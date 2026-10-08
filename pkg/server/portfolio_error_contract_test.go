package server

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/history"
	"github.com/sjzsdu/tongstock/pkg/stockinfo"
	"github.com/sjzsdu/tongstock/pkg/stockpool"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/trading"
	"github.com/sjzsdu/tongstock/pkg/watchlist"
)

// Characterization tests for the legacy error sites in portfolio_handlers.go.
// Each case pins the exact status and byte-identical body that
// ErrorEnvelopeMiddleware emits today for a legacy {"error": ...} response,
// so the later WriteError conversion must not change any observable output.
// The "trading store not initialized" 500s are the current contract and keep
// their 500 status (normalization is out of scope).

// closedPortfolioRouter builds every portfolio store over one sqlite storage
// and then closes it: every store query afterwards fails, which drives the
// handlers into their legacy 500 branches deterministically.
func closedPortfolioRouter(t *testing.T) *gin.Engine {
	t.Helper()
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "portfolio.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	historyStore, err := history.New(store)
	if err != nil {
		t.Fatal(err)
	}
	watchlistStore, err := watchlist.New(store)
	if err != nil {
		t.Fatal(err)
	}
	stockpoolStore, err := stockpool.New(store)
	if err != nil {
		t.Fatal(err)
	}
	tradingStore, err := trading.New(store)
	if err != nil {
		t.Fatal(err)
	}
	stockinfoStore, err := stockinfo.New(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return newContractGinEngine(t, Dependencies{
		History:   historyStore,
		Watchlist: watchlistStore,
		StockPool: stockpoolStore,
		Trading:   tradingStore,
		StockInfo: stockinfoStore,
	})
}

func TestPortfolioClosedStoreErrorContract(t *testing.T) {
	router := closedPortfolioRouter(t)

	t.Run("history list failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/history", ""),
			http.StatusInternalServerError)
	})
	t.Run("history add invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/history", `{invalid`),
			http.StatusBadRequest)
	})
	t.Run("history add missing code", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/history", `{}`),
			http.StatusBadRequest)
	})
	t.Run("history add persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/history", `{"code":"600000"}`),
			http.StatusInternalServerError)
	})
	t.Run("history delete persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodDelete, "/api/history/600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("watchlist list failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/watchlist", ""),
			http.StatusInternalServerError)
	})
	t.Run("watchlist add invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/watchlist", `{invalid`),
			http.StatusBadRequest)
	})
	t.Run("watchlist add missing code", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/watchlist", `{}`),
			http.StatusBadRequest)
	})
	t.Run("watchlist add persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/watchlist", `{"code":"600000","name":"浦发银行"}`),
			http.StatusInternalServerError)
	})
	t.Run("watchlist delete persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodDelete, "/api/watchlist/600000", ""),
			http.StatusInternalServerError)
	})
	t.Run("watchlist note invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/watchlist/600000/note", `{invalid`),
			http.StatusBadRequest)
	})
	t.Run("watchlist note persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/watchlist/600000/note", `{"note":"x"}`),
			http.StatusInternalServerError)
	})
	t.Run("watchlist group invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/watchlist/600000/group", `{invalid`),
			http.StatusBadRequest)
	})
	t.Run("watchlist group persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/watchlist/600000/group", `{"group":"g"}`),
			http.StatusInternalServerError)
	})
	t.Run("watchlist groups failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/watchlist/groups", ""),
			http.StatusInternalServerError)
	})

	t.Run("stockpool list failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stockpool", ""),
			http.StatusInternalServerError)
	})
	t.Run("stockpool upsert invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/stockpool", `{invalid`),
			http.StatusBadRequest)
	})
	t.Run("stockpool upsert missing id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/stockpool", `{}`),
			http.StatusBadRequest)
	})
	t.Run("stockpool upsert missing name", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/stockpool", `{"id":"p1"}`),
			http.StatusBadRequest)
	})
	t.Run("stockpool upsert persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/stockpool", `{"id":"p1","name":"n"}`),
			http.StatusInternalServerError)
	})
	t.Run("stockpool delete persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodDelete, "/api/stockpool/p1", ""),
			http.StatusInternalServerError)
	})

	t.Run("trade create invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/trades", `{invalid`),
			http.StatusBadRequest)
	})
	t.Run("trade create persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/trades", `{"code":"600000","action":"buy","price":10}`),
			http.StatusInternalServerError)
	})
	t.Run("trade list failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/trades", ""),
			http.StatusInternalServerError)
	})
	t.Run("trade list by codes failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/trades?codes=600000", ""),
			http.StatusInternalServerError)
	})
	t.Run("trade positions failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/trades/positions", ""),
			http.StatusInternalServerError)
	})
	t.Run("trade delete persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodDelete, "/api/trades/1", ""),
			http.StatusInternalServerError)
	})

	t.Run("stockinfo list failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stockinfo", ""),
			http.StatusInternalServerError)
	})
	t.Run("stockinfo get not found", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stockinfo/600000", ""),
			http.StatusNotFound)
	})
	t.Run("stockinfo count failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/stockinfo/count", ""),
			http.StatusInternalServerError)
	})
}

func TestPortfolioNilTradingStoreErrorContract(t *testing.T) {
	// No Trading dependency: the "trading store not initialized" 500s and
	// the invalid-id 400 fire before any store access.
	router := newContractGinEngine(t, Dependencies{})

	t.Run("trade create store not initialized", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/trades", `{"code":"600000","action":"buy","price":10}`),
			http.StatusInternalServerError)
	})
	t.Run("trade list store not initialized", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/trades", ""),
			http.StatusInternalServerError)
	})
	t.Run("trade positions store not initialized", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/trades/positions", ""),
			http.StatusInternalServerError)
	})
	t.Run("trade delete invalid id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodDelete, "/api/trades/abc", ""),
			http.StatusBadRequest)
	})
	t.Run("trade delete store not initialized", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodDelete, "/api/trades/1", ""),
			http.StatusInternalServerError)
	})
}
