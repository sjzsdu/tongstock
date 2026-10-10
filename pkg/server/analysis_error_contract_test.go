package server

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
)

// Characterization tests for the legacy error sites in analysis_handlers.go.
// Each case pins the exact status and byte-identical body that
// ErrorEnvelopeMiddleware emitted for a legacy {"error": ...} response,
// so the later WriteError conversion must not change any observable output.
// None of these paths touch param's package-global config (reviewer nit 3).

func TestAnalysisHandlersErrorContract(t *testing.T) {
	// Nil service: every case below fails before the service is touched,
	// or (search/search-index) fails inside getStockSearchIndex's nil guard.
	router := newContractGinEngine(t, Dependencies{})

	t.Run("screen missing codes", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/screen", ""),
			http.StatusBadRequest)
	})

	t.Run("signal analysis missing code", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/signal-analysis", ""),
			http.StatusBadRequest)
	})

	t.Run("stock search missing query", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stocks/search", ""),
			http.StatusBadRequest)
	})

	t.Run("stock search unavailable", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stocks/search?q=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("stock search index unavailable", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stocks/search-index", ""),
			http.StatusInternalServerError)
	})

	t.Run("stock compare missing code", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stock/compare", ""),
			http.StatusBadRequest)
	})
}

// failingAnalysisService builds a tdx.Service over closed sqlite storage so
// kline-store reads fail, with a failing executor for client-backed calls.
func failingAnalysisService(t *testing.T) *tdx.Service {
	t.Helper()
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "analysis.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := tdx.NewServiceWithExecutor(stubExecutor{doErr: errors.New("connection closed")}, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return service
}

func TestAnalysisFetchFailureErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{StockData: failingAnalysisService(t)})

	t.Run("indicator fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/indicator?code=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("signal analysis fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/signal-analysis?code=600000", ""),
			http.StatusInternalServerError)
	})
}

func TestStockCompareQuoteErrorContract(t *testing.T) {
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "compare.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	t.Run("compare quote fetch failure", func(t *testing.T) {
		failing, err := tdx.NewServiceWithExecutor(stubExecutor{doErr: errors.New("connection closed")}, store)
		if err != nil {
			t.Fatal(err)
		}
		router := newContractGinEngine(t, Dependencies{StockData: failing})
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stock/compare?code=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("compare quote not found", func(t *testing.T) {
		// A no-op executor returns zero quotes without error, driving
		// handleStockCompare into its 404 legacy branch.
		quiet, err := tdx.NewServiceWithExecutor(stubExecutor{}, store)
		if err != nil {
			t.Fatal(err)
		}
		router := newContractGinEngine(t, Dependencies{StockData: quiet})
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stock/compare?code=600000", ""),
			http.StatusNotFound)
	})
}
