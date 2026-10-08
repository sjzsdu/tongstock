package server

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/pkg/cache"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

func storageNew(t *testing.T) (*storage.Storage, error) {
	t.Helper()
	return storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "round9.db"),
	})
}

// Round-9 characterization: the parked mixed-key legacy error sites. Each
// case pins the exact status and byte-identical body that
// ErrorEnvelopeMiddleware emits when it merges a legacy {"error": ...} body
// with extra top-level keys. Sites whose payloads contain wall-clock
// timestamps (monitoring input status, paradigm evidence card) are pinned
// through a parse-then-remarshal round trip: the response body must equal
// the test-local sorted-key re-marshal of its own parsed values — the same
// transformation WriteErrorWithDetails must reproduce — plus semantic
// assertions on the extra values. Fully deterministic sites use static
// goldens.

func paradigmMixedRouter(t *testing.T) *gin.Engine {
	t.Helper()
	store, err := storageNew(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedParadigmKlines(t, store)
	server := NewServer(Dependencies{Storage: store})
	paradigmStore := paradigmTestStore(t, false)
	if err := paradigmStore.Save(&paradigms.Paradigm{ID: "p1", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := paradigmStore.Save(&paradigms.Paradigm{
		ID: "p2", Name: "runnable", StockCode: "600000", StockName: "浦发银行",
		BuyConds: []paradigms.Condition{{Indicator: "MA20", Operator: "gt", Value: "10"}},
	}); err != nil {
		t.Fatal(err)
	}
	server.SetParadigmStore(paradigmStore)
	return serverContractRouter(t, server)
}

// seedParadigmKlines inserts a small real-kline series for 600000 so
// resolveParadigmSnapshot can freeze a snapshot offline. The series is
// deliberately short — under the default split's MinTrainSize — so the
// experiment run fails after the experiment is created, driving the
// conditional-keys 422 branch.
func seedParadigmKlines(t *testing.T, store *storage.Storage) {
	t.Helper()
	dates := []string{"20260105", "20260106", "20260107", "20260108",
		"20260109", "20260112", "20260113", "20260114", "20260115", "20260116"}
	for i, date := range dates {
		close := 10.0 + float64(i)*0.1
		if _, err := store.DB().Exec(
			`INSERT INTO kline (code, ktype, date, open, high, low, close, volume, amount)
			 VALUES ('600000', 9, ?, ?, ?, ?, ?, 1000, ?)`,
			date, close, close+0.1, close-0.1, close, 1000.0*close); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParadigmMixedConflictGolden(t *testing.T) {
	router := paradigmMixedRouter(t)

	// Verifying without any persisted experiment yields the unavailable
	// evidence card: a 409 whose body merges the error envelope with
	// promotion_blockers and the full evidence struct.
	response := doLegacyErrorRequest(t, router, http.MethodPut, "/api/paradigm/p1/review", `{"review_status":"verified"}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	extras := parsedErrorExtras(t, response)
	assertLegacyErrorMixedGolden(t, response, http.StatusConflict, extras)

	blockers, ok := extras["promotion_blockers"].([]any)
	if !ok || len(blockers) == 0 {
		t.Fatalf("missing promotion_blockers: %s", response.Body.String())
	}
	evidence, ok := extras["evidence"].(map[string]any)
	if !ok {
		t.Fatalf("missing evidence object: %s", response.Body.String())
	}
	if evidence["promotion_eligible"] != false {
		t.Fatalf("evidence should not be promotion-eligible: %s", response.Body.String())
	}
	if evidence["available"] != false {
		t.Fatalf("evidence should be unavailable: %s", response.Body.String())
	}
}

func TestParadigmExperiment422Golden(t *testing.T) {
	router := paradigmMixedRouter(t)

	// Reduced variant: the paradigm has no executable buy conditions, so the
	// experiment never starts and the body carries only the error key.
	assertLegacyErrorGolden(t,
		doLegacyErrorRequest(t, router, http.MethodPost, "/api/paradigm/backtest", `{"paradigm_id":"p1"}`),
		http.StatusUnprocessableEntity)

	// Full-keys variant: with buy conditions and a seeded real-kline series
	// the snapshot freezes and the experiment is created, then the run fails
	// (the series is deliberately shorter than the split's MinTrainSize), so
	// the body must carry all three conditional keys through the merge.
	response := doLegacyErrorRequest(t, router, http.MethodPost, "/api/paradigm/backtest", `{"paradigm_id":"p2"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	extras := parsedErrorExtras(t, response)
	for _, key := range []string{"experiment_id", "snapshot_id", "run_id"} {
		if _, ok := extras[key]; !ok {
			t.Fatalf("missing conditional key %q: %s", key, response.Body.String())
		}
	}
	for key := range extras {
		if key != "experiment_id" && key != "snapshot_id" && key != "run_id" {
			t.Fatalf("unexpected extra key %q: %s", key, response.Body.String())
		}
	}
	assertLegacyErrorMixedGolden(t, response, http.StatusUnprocessableEntity, extras)
}

func TestAgentResearch422Golden(t *testing.T) {
	router := paradigmMixedRouter(t)

	// The verified-research pipeline fails closed for a paradigm without a
	// runnable experiment: fixed conclusion/answer keys plus the error.
	response := doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/research", `{"paradigm_id":"p1"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	extras := parsedErrorExtras(t, response)
	assertLegacyErrorMixedGolden(t, response, http.StatusUnprocessableEntity, extras)

	if extras["conclusion"] != "insufficient_data" {
		t.Fatalf("conclusion = %v, want insufficient_data", extras["conclusion"])
	}
	if extras["answer"] == "" || extras["answer"] == nil {
		t.Fatalf("missing answer: %s", response.Body.String())
	}
}

func TestAgentResearch422ConditionalKeysGolden(t *testing.T) {
	router := paradigmMixedRouter(t)

	// With a runnable paradigm the research pipeline reaches the same
	// failed-run 422 and must carry its conditional experiment/run keys
	// through the merge (nit 2: same fixture as the experiment full-keys).
	response := doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/research", `{"paradigm_id":"p2"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	extras := parsedErrorExtras(t, response)
	for _, key := range []string{"conclusion", "answer", "experiment_id", "run_id"} {
		if _, ok := extras[key]; !ok {
			t.Fatalf("missing key %q: %s", key, response.Body.String())
		}
	}
	for key := range extras {
		if key != "conclusion" && key != "answer" && key != "experiment_id" && key != "run_id" {
			t.Fatalf("unexpected extra key %q: %s", key, response.Body.String())
		}
	}
	assertLegacyErrorMixedGolden(t, response, http.StatusUnprocessableEntity, extras)
	if extras["conclusion"] != "insufficient_data" {
		t.Fatalf("conclusion = %v, want insufficient_data", extras["conclusion"])
	}
}

func TestMonitoringReportUnavailableGolden(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})

	// Both call sites route through monitoringReportUnavailable: a 404
	// merging the error envelope with available=false and the input
	// diagnostics struct (which contains a wall-clock timestamp, hence the
	// parse-based pinning).
	t.Run("report", func(t *testing.T) {
		response := doLegacyErrorRequest(t, router, http.MethodGet, "/api/monitoring/report", "")
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		extras := parsedErrorExtras(t, response)
		assertLegacyErrorMixedGolden(t, response, http.StatusNotFound, extras)

		if extras["available"] != false {
			t.Fatalf("available = %v, want false", extras["available"])
		}
		input, ok := extras["input"].(map[string]any)
		if !ok || input["source"] != "none" {
			t.Fatalf("missing input diagnostics: %s", response.Body.String())
		}
	})

	t.Run("report refresh", func(t *testing.T) {
		response := doLegacyErrorRequest(t, router, http.MethodPost, "/api/monitoring/report/refresh", "")
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		extras := parsedErrorExtras(t, response)
		assertLegacyErrorMixedGolden(t, response, http.StatusNotFound, extras)
	})
}

func TestStockSearchTypedErrorGolden(t *testing.T) {
	// Seed the code cache inside the shared sqlite DB so FetchCodes resolves
	// offline for all three exchanges (the search index needs every source
	// to succeed). The failing executor stays unused for cached codes.
	store, err := storageNew(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cacheInstance, err := cache.NewSQLiteCacheWithDB(store.DB())
	if err != nil {
		t.Fatal(err)
	}
	codeStore, err := tdx.NewCodeStore(cacheInstance)
	if err != nil {
		t.Fatal(err)
	}
	if err := codeStore.SaveCodes([]*protocol.CodeItem{
		{Code: "600000", Name: "浦发银行"},
		{Code: "600001", Name: "兴业银行"},
	}, protocol.ExchangeSH); err != nil {
		t.Fatal(err)
	}
	if err := codeStore.SaveCodes([]*protocol.CodeItem{
		{Code: "000001", Name: "深发展"},
	}, protocol.ExchangeSZ); err != nil {
		t.Fatal(err)
	}
	if err := codeStore.SaveCodes([]*protocol.CodeItem{
		{Code: "430047", Name: "诺思兰德"},
	}, protocol.ExchangeBJ); err != nil {
		t.Fatal(err)
	}

	service, err := tdx.NewServiceWithExecutor(stubExecutor{doErr: errors.New("connection closed")}, store)
	if err != nil {
		t.Fatal(err)
	}
	router := newContractGinEngine(t, Dependencies{StockData: service})

	// Deterministic 404: no matches at all — empty matches slice pinned
	// statically, including the int total round-tripping to 0.
	assertLegacyErrorMixedGolden(t,
		doLegacyErrorRequest(t, router, http.MethodGet, "/api/indicator?code=88888", ""),
		http.StatusNotFound, map[string]any{
			"query":   "88888",
			"total":   0,
			"matches": []stockSearchMatch{},
		})

	// Ambiguous 409: a prefix matching several seeded codes carries the
	// non-empty matches struct slice (sorted-key re-marshal probe).
	response := doLegacyErrorRequest(t, router, http.MethodGet, "/api/indicator?code=%E9%93%B6%E8%A1%8C", "")
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	extras := parsedErrorExtras(t, response)
	assertLegacyErrorMixedGolden(t, response, http.StatusConflict, extras)

	if extras["total"] != float64(2) {
		t.Fatalf("total = %v, want 2 (int round-tripped to float64)", extras["total"])
	}
	matches, ok := extras["matches"].([]any)
	if !ok || len(matches) != 2 {
		t.Fatalf("expected 2 matches: %s", response.Body.String())
	}
}
