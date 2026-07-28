package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/newsfeed"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/watchlist"
)

func TestVerticalSliceKeyHandlers(t *testing.T) {
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "slices.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	watchlistStore, err := watchlist.New(store)
	if err != nil {
		t.Fatal(err)
	}
	newsStore, err := newsfeed.NewStoreWithStorage(store)
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), ErrorEnvelopeMiddleware(), Recovery())
	NewServer(Dependencies{
		Watchlist: watchlistStore,
		Newsfeed:  NewNewsfeedHandler(newsStore),
	}).SetupRoutes(router)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "analysis validation", method: http.MethodGet, path: "/api/indicator", wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "sync validation", method: http.MethodPost, path: "/api/sync/daily", body: `{}`, wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "settings validation", method: http.MethodPut, path: "/api/settings/indicator", body: `{`, wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "portfolio write", method: http.MethodPost, path: "/api/watchlist", body: `{"code":"600000","name":"浦发银行","group":"银行"}`, wantStatus: http.StatusOK},
		{name: "portfolio read", method: http.MethodGet, path: "/api/watchlist", wantStatus: http.StatusOK},
		{name: "news read", method: http.MethodGet, path: "/api/news/feed", wantStatus: http.StatusOK},
		{name: "agent diagnostics", method: http.MethodGet, path: "/api/agent/diagnose", wantStatus: http.StatusOK},
		{name: "paradigm list", method: http.MethodGet, path: "/api/paradigm/list", wantStatus: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantCode != "" {
				var envelope ErrorEnvelope
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Error.Code != test.wantCode {
					t.Fatalf("error code = %q, want %q; body = %s", envelope.Error.Code, test.wantCode, response.Body.String())
				}
			}
		})
	}
}
