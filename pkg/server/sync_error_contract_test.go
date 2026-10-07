package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
)

// Characterization tests for the legacy error sites in sync_handlers.go.
// Each case pins the exact status and byte-identical body that
// ErrorEnvelopeMiddleware emits today for a legacy {"error": ...} response,
// so the later WriteError conversion must not change any observable output.

const syncErrorContractRequestID = "sync-error-contract-req"

func syncErrorGoldenBody(status int) string {
	code, message := statusError(status)
	envelope := map[string]any{"error": APIError{
		Code:      code,
		Message:   message,
		RequestID: syncErrorContractRequestID,
	}}
	data, err := json.Marshal(envelope)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func syncErrorContractRouter(t *testing.T, deps Dependencies) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), ErrorEnvelopeMiddleware(), Recovery())
	NewServer(deps).SetupRoutes(router)
	return router
}

func doSyncErrorRequest(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Request-ID", syncErrorContractRequestID)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertSyncErrorGolden(t *testing.T, response *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, wantStatus, response.Body.String())
	}
	want := syncErrorGoldenBody(wantStatus)
	if got := response.Body.String(); got != want {
		t.Fatalf("body mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestSyncHandlersErrorContract(t *testing.T) {
	router := syncErrorContractRouter(t, Dependencies{})

	t.Run("sync daily invalid JSON body", func(t *testing.T) {
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodPost, "/api/sync/daily", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("sync daily missing codes", func(t *testing.T) {
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodPost, "/api/sync/daily", `{}`),
			http.StatusBadRequest)
	})

	t.Run("sync daily service unavailable", func(t *testing.T) {
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodPost, "/api/sync/daily", `{"codes":["600000"]}`),
			http.StatusServiceUnavailable)
	})

	t.Run("sync state missing code", func(t *testing.T) {
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodGet, "/api/sync/state", ""),
			http.StatusBadRequest)
	})

	t.Run("sync freshness missing codes", func(t *testing.T) {
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodGet, "/api/sync/freshness", ""),
			http.StatusBadRequest)
	})

	t.Run("kline clean missing code", func(t *testing.T) {
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodPost, "/api/kline/clean", ""),
			http.StatusBadRequest)
	})
}

// uncreatableHomeDir returns a HOME path that EnsureHomeDir cannot create:
// an existing regular file, so MkdirAll on it always fails.
func uncreatableHomeDir(t *testing.T) string {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	return blocker
}

func TestSettingsHandlersErrorContract(t *testing.T) {
	router := syncErrorContractRouter(t, Dependencies{})

	// Runs before any success-path settings request so param's cached global
	// config is still unset and GetConfig must hit AutoInit/EnsureHomeDir.
	t.Run("get settings config load failure", func(t *testing.T) {
		t.Setenv("HOME", uncreatableHomeDir(t))
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodGet, "/api/settings/indicator", ""),
			http.StatusInternalServerError)
	})

	t.Run("save settings invalid JSON body", func(t *testing.T) {
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodPut, "/api/settings/indicator", `{`),
			http.StatusBadRequest)
	})

	t.Run("save settings invalid payload", func(t *testing.T) {
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodPut, "/api/settings/indicator", `{"defaults":{}}`),
			http.StatusBadRequest)
	})

	t.Run("save settings persistence failure", func(t *testing.T) {
		t.Setenv("HOME", uncreatableHomeDir(t))
		assertSyncErrorGolden(t,
			doSyncErrorRequest(t, router, http.MethodPut, "/api/settings/indicator", `{"defaults":{"ma":[5,10]}}`),
			http.StatusInternalServerError)
	})
}

// blockingExecutor satisfies tdx.Executor without any network dependency.
type blockingExecutor struct{}

func (blockingExecutor) Do(fn func(c *tdx.Client) error) error                        { return nil }
func (blockingExecutor) DoContext(_ context.Context, _ func(*tdx.Client) error) error { return nil }
func (blockingExecutor) Close() error                                                 { return nil }
func (blockingExecutor) Len() int                                                     { return 0 }
func (blockingExecutor) Status() tdx.ExecutorStatus                                   { return tdx.ExecutorStatus{} }

func TestCleanKlinesPersistenceErrorContract(t *testing.T) {
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "clean.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := tdx.NewServiceWithExecutor(blockingExecutor{}, store)
	if err != nil {
		t.Fatal(err)
	}
	// Closing the storage makes every kline-store query fail, which drives
	// handleCleanKlines into its 500 legacy-error branch.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	router := syncErrorContractRouter(t, Dependencies{StockData: service})
	assertSyncErrorGolden(t,
		doSyncErrorRequest(t, router, http.MethodPost, "/api/kline/clean?code=600000", ""),
		http.StatusInternalServerError)
}
