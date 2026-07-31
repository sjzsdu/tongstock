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
