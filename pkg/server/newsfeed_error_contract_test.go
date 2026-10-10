package server

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// Characterization tests for the legacy error sites in newsfeed_handlers.go.
// Each case pins the exact status and byte-identical body that
// ErrorEnvelopeMiddleware emitted for a legacy {"error": ...} response,
// so the later WriteError conversion must not change any observable output.
// The two dynamic-status sites (stock news / hot topics) always stay >= 400:
// 500 on plain failures, 502 on ErrFetchUnavailable (default-mapped to
// internal_error by statusError, matching today's middleware).

func newsfeedTestStore(t *testing.T) (*storage.Storage, *newsfeed.SQLiteStore) {
	t.Helper()
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "newsfeed.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlite, err := newsfeed.NewStoreWithStorage(store)
	if err != nil {
		t.Fatal(err)
	}
	return store, sqlite
}

// The router helpers below are thin wrappers so each test gets the full
// middleware stack via newContractGinEngine.

func mustNewsfeedStore(t *testing.T) *newsfeed.SQLiteStore {
	t.Helper()
	_, sqlite := newsfeedTestStore(t)
	return sqlite
}

func TestNewsfeedOpenStoreErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{Newsfeed: NewNewsfeedHandler(mustNewsfeedStore(t))})

	t.Run("news item not found", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/item/does-not-exist", ""),
			http.StatusNotFound)
	})

	t.Run("hot event detail not found", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/events/does-not-exist", ""),
			http.StatusNotFound)
	})

	t.Run("stock news invalid code", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/stock/abc", ""),
			http.StatusBadRequest)
	})

	t.Run("hot topics service unavailable", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/topics", ""),
			http.StatusServiceUnavailable)
	})

	t.Run("search missing keyword", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/search", ""),
			http.StatusBadRequest)
	})

	// Note: the "无效的数据源" 400 in handleFetchBrowserNews is unreachable —
	// sources.NewBrowserSource never returns nil — so it has no golden here.

	t.Run("alert rule add invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/news/alerts/rule", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("alert rule update invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/news/alerts/rule/r1", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("alert rule update not found", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/news/alerts/rule/does-not-exist", `{"name":"n"}`),
			http.StatusNotFound)
	})

	t.Run("watchlist set invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/news/alerts/watchlist", `{invalid`),
			http.StatusBadRequest)
	})
}

func TestNewsfeedClosedStoreErrorContract(t *testing.T) {
	store, sqlite := newsfeedTestStore(t)
	handler := NewNewsfeedHandler(sqlite)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	router := newContractGinEngine(t, Dependencies{Newsfeed: handler})

	t.Run("news feed failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/feed", ""),
			http.StatusInternalServerError)
	})

	t.Run("news facets failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/feed/facets", ""),
			http.StatusInternalServerError)
	})

	t.Run("news item failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/item/x", ""),
			http.StatusInternalServerError)
	})

	t.Run("hot events failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/events", ""),
			http.StatusInternalServerError)
	})

	t.Run("hot event detail failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/events/x", ""),
			http.StatusInternalServerError)
	})

	t.Run("refresh events failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/news/events/refresh", ""),
			http.StatusInternalServerError)
	})

	t.Run("stock news store failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/stock/600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("search store failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/search?keyword=x", ""),
			http.StatusInternalServerError)
	})

	t.Run("market sentiment failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/sentiment/market", ""),
			http.StatusInternalServerError)
	})

	t.Run("sentiment trend failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/sentiment/trend", ""),
			http.StatusInternalServerError)
	})

	t.Run("sentiment heatmap failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/sentiment/heatmap", ""),
			http.StatusInternalServerError)
	})

	t.Run("stock sentiment failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/sentiment/stock/600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("alerts failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/alerts", ""),
			http.StatusInternalServerError)
	})

	t.Run("unread alerts failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/alerts/unread", ""),
			http.StatusInternalServerError)
	})

	t.Run("unread count failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/alerts/count", ""),
			http.StatusInternalServerError)
	})

	t.Run("mark alert read failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/news/alerts/a1/read", ""),
			http.StatusInternalServerError)
	})

	t.Run("mark all alerts read failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/news/alerts/read-all", ""),
			http.StatusInternalServerError)
	})
}

func TestNewsfeedStockServiceErrorContract(t *testing.T) {
	// A stock news service over the closed store fails on its first DB
	// access with a non-ErrFetchUnavailable error, so both dynamic-status
	// sites take their 500 branch here. The 502 (ErrFetchUnavailable)
	// branch needs feed fixtures and is covered by inspection.
	store, sqlite := newsfeedTestStore(t)
	handler := NewNewsfeedHandler(sqlite)
	svc, err := newsfeed.NewService(sqlite, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.SetStockNewsService(svc)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	router := newContractGinEngine(t, Dependencies{Newsfeed: handler})

	t.Run("stock news service failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/stock/600000", ""),
			http.StatusInternalServerError)
	})

	t.Run("hot topics service failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/news/topics", ""),
			http.StatusInternalServerError)
	})
}
