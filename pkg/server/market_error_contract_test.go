package server

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
)

// Characterization tests for the legacy error sites in market_handlers.go
// and the resolveStockCodeOrRespond helper in server.go. Each case pins the
// exact status and byte-identical body that ErrorEnvelopeMiddleware emitted
// today for a legacy {"error": ...} response, so the WriteError conversion
// must not change any observable output. The helper is shared by market and
// analysis handlers, so its conversion covers both callers.

func failingMarketService(t *testing.T) *tdx.Service {
	t.Helper()
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "market.db"),
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

func TestMarketHandlersErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{StockData: failingMarketService(t)})

	t.Run("quotes missing codes", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/quotes", ""),
			http.StatusBadRequest)
	})

	t.Run("codes fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/codes", ""),
			http.StatusInternalServerError)
	})

	t.Run("codes list fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/codes/list", ""),
			http.StatusInternalServerError)
	})

	t.Run("index kline failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/index", ""),
			http.StatusInternalServerError)
	})

	t.Run("minute failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/minute?code=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("trade failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/trade?code=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("auction failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/auction?code=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("xdxr failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/xdxr?code=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("finance trends invalid mode", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/finance/trends?code=600000&mode=bad", ""),
			http.StatusBadRequest)
	})

	t.Run("finance trends fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/finance/trends?code=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("finance metrics fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/finance/metrics?code=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("company catalogue failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/company?code=600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("company content missing code", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/company/content", ""),
			http.StatusBadRequest)
	})

	t.Run("company content missing block and filename", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/company/content?code=600000", ""),
			http.StatusBadRequest)
	})

	t.Run("company content fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/company/content?code=600000&filename=gsgl.dat", ""),
			http.StatusInternalServerError)
	})

	t.Run("block fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/block", ""),
			http.StatusInternalServerError)
	})

	t.Run("block list fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/block/list", ""),
			http.StatusInternalServerError)
	})

	t.Run("block show fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/block/show?name=x", ""),
			http.StatusInternalServerError)
	})

	t.Run("block show missing name and code", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/block/show", ""),
			http.StatusBadRequest)
	})

	t.Run("count fetch failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/count", ""),
			http.StatusInternalServerError)
	})
}

func TestResolveStockCodeErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{StockData: failingMarketService(t)})

	t.Run("missing code parameter", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/auction", ""),
			http.StatusBadRequest)
	})

	t.Run("search index failure", func(t *testing.T) {
		// A non-6-digit query falls through to the search index, whose
		// build fails on the failing service — the helper's 500 branch.
		// The same helper serves analysis handlers (handleIndicator etc.).
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/auction?code=abc", ""),
			http.StatusInternalServerError)
	})
}
