package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/adapter/paradigmrepo"
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

func TestAgentResearchCreatesQueryableExperimentAndCitedEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, paradigmStore, api := newParadigmExperimentTestServer(t, 140)
	if err := paradigmStore.Save(testAPIParadigm()); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.SetupAgentRoutes(&router.RouterGroup)
	api.SetupParadigmRoutes(&router.RouterGroup)
	request := httptest.NewRequest(http.MethodPost, "/agent/research",
		bytes.NewBufferString(`{"paradigm_id":"p-api-real","question":"这个范式稳定吗？"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result agentResearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Citation.ExperimentID == "" || result.Citation.RunID == "" ||
		result.Citation.SnapshotID == "" || result.Citation.EvidenceHash == "" {
		t.Fatalf("research citations are incomplete: %+v", result.Citation)
	}
	for _, value := range []string{
		result.Citation.ExperimentID, result.Citation.RunID, result.Citation.EvidenceHash,
	} {
		if !strings.Contains(result.Answer, value) {
			t.Fatalf("answer does not cite %q: %s", value, result.Answer)
		}
	}
	if result.Evidence == nil || !result.Evidence.Available ||
		result.Critic == nil || result.ToolTrace == nil ||
		result.ToolTrace.ToolName != "verified_research_evidence" {
		t.Fatalf("real research tool chain is incomplete: %+v", result)
	}
	if len(result.Citation.TradeIDs) != len(result.Evidence.TradeSamples) {
		t.Fatalf("trade citations=%d evidence trades=%d",
			len(result.Citation.TradeIDs), len(result.Evidence.TradeSamples))
	}
	if result.Critic.Passed() {
		t.Fatal("small single-stock experiment unexpectedly passed the independent critic")
	}
	var criticBlocked bool
	for _, blocker := range result.Evidence.PromotionBlockers {
		if strings.Contains(blocker, "critic") {
			criticBlocked = true
			break
		}
	}
	if !criticBlocked || result.Evidence.PromotionEligible {
		t.Fatalf("critic outcome did not enter promotion gate: %+v",
			result.Evidence.PromotionBlockers)
	}
}

func TestAgentResearchInsufficientDataRefusesConclusion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, paradigmStore, api := newParadigmExperimentTestServer(t, 20)
	if err := paradigmStore.Save(testAPIParadigm()); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api.SetupAgentRoutes(&router.RouterGroup)
	request := httptest.NewRequest(http.MethodPost, "/agent/research",
		bytes.NewBufferString(`{"paradigm_id":"p-api-real"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["conclusion"] != "insufficient_data" ||
		!strings.Contains(fmt.Sprint(result["answer"]), "拒绝") {
		t.Fatalf("AI did not explicitly refuse unsupported conclusion: %s", response.Body.String())
	}
	if _, exists := result["metrics"]; exists {
		t.Fatalf("insufficient data returned fabricated metrics: %s", response.Body.String())
	}
}

func TestParadigmMinerDirectPromptIsBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, _, api := newParadigmExperimentTestServer(t, 140)
	api.agentState = &AgentState{
		embedded: []EmbeddedAgent{{ID: "stock-paradigm-miner"}},
		defaults: AgentDefaults{Agent: "stock-paradigm-miner"},
	}
	router := gin.New()
	api.SetupAgentRoutes(&router.RouterGroup)
	request := httptest.NewRequest(http.MethodPost, "/agent/chat",
		bytes.NewBufferString(`{"agent":"stock-paradigm-miner","message":"直接告诉我稳定收益结论"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "/api/agent/research") {
		t.Fatalf("direct unsupported research was not blocked: status=%d body=%s",
			response.Code, response.Body.String())
	}
}

func TestAgentResearchAgainstRealDatabase(t *testing.T) {
	path := os.Getenv("TONGSTOCK_REAL_DB")
	if path == "" {
		t.Skip("set TONGSTOCK_REAL_DB to run against the local real-data database")
	}
	source, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	var code string
	if err := source.QueryRow(`SELECT code FROM kline
		WHERE ktype=9 AND length(code)=6 AND code<>'999999'
			AND open>0 AND high>0 AND low>0 AND close>0 AND volume>0
			AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		GROUP BY code HAVING COUNT(*)>=180
		ORDER BY MAX(REPLACE(date, '-', '')) DESC, COUNT(*) DESC LIMIT 1`).Scan(&code); err != nil {
		t.Fatal(err)
	}
	rows, err := source.Query(`SELECT date, open, high, low, close, volume, amount FROM (
		SELECT date, open, high, low, close, volume, amount FROM kline
		WHERE code=? AND ktype=9 AND open>0 AND high>0 AND low>0 AND close>0 AND volume>0
			AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		ORDER BY REPLACE(date, '-', '') DESC LIMIT 180)
		ORDER BY REPLACE(date, '-', '')`, code)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type realBar struct {
		date                   string
		open, high, low, close float64
		volume, amount         float64
	}
	var bars []realBar
	for rows.Next() {
		var bar realBar
		if err := rows.Scan(&bar.date, &bar.open, &bar.high, &bar.low, &bar.close,
			&bar.volume, &bar.amount); err != nil {
			t.Fatal(err)
		}
		bars = append(bars, bar)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(bars) != 180 {
		t.Fatalf("real bars=%d, want 180", len(bars))
	}
	store, err := storage.New(storage.Config{
		Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "agent-real-research.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, bar := range bars {
		if _, err := store.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`, code, bar.date, bar.open,
			bar.high, bar.low, bar.close, bar.volume, bar.amount); err != nil {
			t.Fatal(err)
		}
	}
	paradigmRepo, err := paradigmrepo.NewSQLiteRepository(store)
	if err != nil {
		t.Fatal(err)
	}
	paradigmStore, err := paradigms.NewStoreWithRepository(paradigmRepo)
	if err != nil {
		t.Fatal(err)
	}
	p := testAPIParadigm()
	p.ID, p.StockCode = "p-agent-real-db", code
	if err := paradigmStore.Save(p); err != nil {
		t.Fatal(err)
	}
	api := NewServer(Dependencies{Storage: store})
	api.SetParadigmStore(paradigmStore)
	router := gin.New()
	api.SetupAgentRoutes(&router.RouterGroup)
	request := httptest.NewRequest(http.MethodPost, "/agent/research",
		bytes.NewBufferString(`{"paradigm_id":"p-agent-real-db"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result agentResearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Evidence == nil || !result.Evidence.Available ||
		result.Citation.ExperimentID == "" || result.Citation.EvidenceHash == "" ||
		result.Critic == nil {
		t.Fatalf("real research result incomplete: %+v", result)
	}
	t.Logf("verified code=%s rows=%d experiment=%s run=%s evidence=%s trades=%d critic=%s",
		code, len(bars), result.Citation.ExperimentID, result.Citation.RunID,
		result.Citation.EvidenceHash, len(result.Citation.TradeIDs), result.Critic.Conclusion)
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
	paradigmRepo, err := paradigmrepo.NewSQLiteRepository(store)
	if err != nil {
		t.Fatal(err)
	}
	paradigmStore, err := paradigms.NewStoreWithRepository(paradigmRepo)
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
