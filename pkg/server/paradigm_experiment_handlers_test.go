package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/backtest"
	"github.com/sjzsdu/tongstock/internal/experiment"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestParadigmBacktestAPIRealSQLiteEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, paradigmStore, api := newParadigmExperimentTestServer(t, 140)
	if err := paradigmStore.Save(testAPIParadigm()); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	api.SetupParadigmRoutes(&router.RouterGroup)
	body := bytes.NewBufferString(`{"paradigm_id":"p-api-real"}`)
	request := httptest.NewRequest(http.MethodPost, "/paradigm/backtest", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result paradigmBacktestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ExperimentID == "" || result.RunID == "" || result.SnapshotID == "" {
		t.Fatalf("persistent identities missing: %+v", result)
	}
	if result.ConfigHash == "" || result.ResultHash == "" || result.Metrics == nil {
		t.Fatalf("reproducible result metadata missing: %+v", result)
	}
	if len(result.SegmentedMetric) == 0 || string(result.SegmentedMetric) == "[]" {
		t.Fatalf("segmented metrics missing: %s", result.SegmentedMetric)
	}

	registry, err := experiment.NewSQLiteRegistry(store)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := registry.GetByID(result.ExperimentID)
	if err != nil {
		t.Fatalf("load persisted experiment: %v", err)
	}
	if persisted.Config.DataSnapshotID != result.SnapshotID {
		t.Fatalf("snapshot binding = %q, want %q", persisted.Config.DataSnapshotID, result.SnapshotID)
	}
	runs, err := registry.ListRuns(result.ExperimentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || artifactNamed(runs[0].Artifacts, "transactions") == nil {
		t.Fatalf("persisted transaction evidence missing: %+v", runs)
	}

	evidenceRequest := httptest.NewRequest(http.MethodGet,
		"/paradigm/experiments/"+result.ExperimentID, nil)
	evidenceResponse := httptest.NewRecorder()
	router.ServeHTTP(evidenceResponse, evidenceRequest)
	if evidenceResponse.Code != http.StatusOK {
		t.Fatalf("evidence status = %d, body = %s", evidenceResponse.Code, evidenceResponse.Body.String())
	}
	if !strings.Contains(evidenceResponse.Body.String(), `"name":"transactions"`) {
		t.Fatalf("transaction artifact not retrievable by experiment_id: %s", evidenceResponse.Body.String())
	}

	cardRequest := httptest.NewRequest(http.MethodGet,
		"/paradigm/p-api-real/evidence?experiment_id="+result.ExperimentID, nil)
	cardResponse := httptest.NewRecorder()
	router.ServeHTTP(cardResponse, cardRequest)
	if cardResponse.Code != http.StatusOK {
		t.Fatalf("evidence card status = %d, body = %s", cardResponse.Code, cardResponse.Body.String())
	}
	var card paradigms.EvidenceCard
	if err := json.Unmarshal(cardResponse.Body.Bytes(), &card); err != nil {
		t.Fatal(err)
	}
	if !card.Available || card.ExperimentID != result.ExperimentID ||
		card.SnapshotID != result.SnapshotID || card.RunID != result.RunID {
		t.Fatalf("evidence provenance is incomplete: %+v", card)
	}
	if card.Lineage == nil || card.Lineage.SourceHash == "" ||
		card.Lineage.ResultHash != result.ResultHash {
		t.Fatalf("evidence lineage is not tied to frozen inputs: %+v", card.Lineage)
	}
	if len(card.TradeSamples) == 0 {
		t.Fatal("real completed trades missing from evidence")
	}
	transactionArtifact := artifactNamed(runs[0].Artifacts, "transactions")
	var persistedTransactions []evidenceTransactionSegment
	if err := json.Unmarshal(transactionArtifact.Content, &persistedTransactions); err != nil {
		t.Fatal(err)
	}
	var persistedTrades []backtest.CompletedTrade
	for _, segment := range persistedTransactions {
		persistedTrades = append(persistedTrades, segment.Trades...)
	}
	if len(card.TradeSamples) != len(persistedTrades) {
		t.Fatalf("evidence trades = %d, persisted trades = %d",
			len(card.TradeSamples), len(persistedTrades))
	}
	if card.TradeSamples[0].NetPnL != persistedTrades[0].NetPnL ||
		card.TradeSamples[0].BuyPrice != persistedTrades[0].BuyPrice {
		t.Fatalf("evidence trade does not match persisted artifact: %+v vs %+v",
			card.TradeSamples[0], persistedTrades[0])
	}
	for _, trade := range card.TradeSamples {
		if trade.BuyExecutionDate.IsZero() || trade.SellExecutionDate.IsZero() ||
			trade.Quantity <= 0 || trade.BuyPrice <= 0 || trade.SellPrice <= 0 {
			t.Fatalf("fabricated or incomplete trade projection: %+v", trade)
		}
	}
	if card.RobustnessScore != nil || card.ParamSensitivity != nil {
		t.Fatalf("unmeasured robustness values must remain unavailable: %+v", card)
	}
	if strings.Contains(strings.ToLower(cardResponse.Body.String()), "synthetic") ||
		strings.Contains(cardResponse.Body.String(), "合成") {
		t.Fatalf("synthetic evidence leaked into response: %s", cardResponse.Body.String())
	}
	if card.PromotionEligible {
		t.Fatal("missing robustness and parameter sweep evidence must block promotion")
	}

	transitionRequest := httptest.NewRequest(http.MethodPost, "/paradigm/p-api-real/transition",
		bytes.NewBufferString(`{"to":"verified","reason":"should be blocked"}`))
	transitionRequest.Header.Set("Content-Type", "application/json")
	transitionResponse := httptest.NewRecorder()
	router.ServeHTTP(transitionResponse, transitionRequest)
	if transitionResponse.Code != http.StatusConflict {
		t.Fatalf("incomplete evidence transition status = %d, body = %s",
			transitionResponse.Code, transitionResponse.Body.String())
	}
}

func TestParadigmBacktestAPIInsufficientRealDataReturns4xx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, paradigmStore, api := newParadigmExperimentTestServer(t, 20)
	if err := paradigmStore.Save(testAPIParadigm()); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.SetupParadigmRoutes(&router.RouterGroup)
	request := httptest.NewRequest(http.MethodPost, "/paradigm/backtest",
		bytes.NewBufferString(`{"paradigm_id":"p-api-real"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code < 400 || response.Code >= 500 {
		t.Fatalf("status = %d, want 4xx; body = %s", response.Code, response.Body.String())
	}
	var failure map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(fmt.Sprint(failure["error"])) == "" {
		t.Fatalf("clear error missing: %s", response.Body.String())
	}
	if _, exists := failure["metrics"]; exists {
		t.Fatalf("insufficient data must not return a zero-valued report: %s", response.Body.String())
	}
}

func TestParadigmEvidenceWithoutExperimentIsExplicitlyUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, paradigmStore, api := newParadigmExperimentTestServer(t, 140)
	if err := paradigmStore.Save(testAPIParadigm()); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.SetupParadigmRoutes(&router.RouterGroup)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/paradigm/p-api-real/evidence", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var card paradigms.EvidenceCard
	if err := json.Unmarshal(response.Body.Bytes(), &card); err != nil {
		t.Fatal(err)
	}
	if card.Available || card.PromotionEligible || len(card.UnavailableReasons) == 0 {
		t.Fatalf("missing evidence must be explicit and block promotion: %+v", card)
	}
	if card.InSample != nil || card.CostAnalysis != nil || len(card.TradeSamples) != 0 {
		t.Fatalf("missing evidence emitted zero-valued or synthetic facts: %+v", card)
	}
}

func newParadigmExperimentTestServer(t *testing.T, barCount int) (*storage.Storage, *paradigms.Store, *Server) {
	t.Helper()
	store, err := storage.New(storage.Config{
		Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "paradigm-api.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	date := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	for inserted := 0; inserted < barCount; date = date.AddDate(0, 0, 1) {
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		price := 10 + float64(inserted)*0.02
		if _, err := store.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`,
			"600001", date.Format("20060102"), price-0.01, price+0.08,
			price-0.08, price, 100000+float64(inserted), price*100000); err != nil {
			t.Fatal(err)
		}
		inserted++
	}
	paradigmStore, err := paradigms.NewStoreWithStorage("", store)
	if err != nil {
		t.Fatal(err)
	}
	return store, paradigmStore, func() *Server {
		api := NewServer(Dependencies{Storage: store})
		api.SetParadigmStore(paradigmStore)
		return api
	}()
}

func testAPIParadigm() *paradigms.Paradigm {
	return &paradigms.Paradigm{
		ID: "p-api-real", StockCode: "600001", Name: "真实 SQLite API 范式",
		BuyConds: []paradigms.Condition{
			{Indicator: "close", Operator: "gt", Value: "0"},
		},
		SellConds: paradigms.SellConditions{
			TakeProfit: []paradigms.Condition{
				{Indicator: "close", Operator: "gt", Value: "0"},
			},
		},
	}
}

func artifactNamed(artifacts []experiment.Artifact, name string) *experiment.Artifact {
	for i := range artifacts {
		if artifacts[i].Name == name {
			return &artifacts[i]
		}
	}
	return nil
}
