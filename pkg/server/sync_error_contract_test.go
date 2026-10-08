package server

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
)

// Characterization tests for the legacy error sites in sync_handlers.go.
// Each case pins the exact status and byte-identical body that
// ErrorEnvelopeMiddleware emitted for a legacy {"error": ...} response,
// so the later WriteError conversion must not change any observable output.
// Shared golden/router/request helpers live in legacy_error_contract_test.go.

func TestSyncHandlersErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})

	t.Run("sync daily invalid JSON body", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/sync/daily", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("sync daily missing codes", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/sync/daily", `{}`),
			http.StatusBadRequest)
	})

	t.Run("sync daily service unavailable", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/sync/daily", `{"codes":["600000"]}`),
			http.StatusServiceUnavailable)
	})

	t.Run("sync state missing code", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/sync/state", ""),
			http.StatusBadRequest)
	})

	t.Run("sync freshness missing codes", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/sync/freshness", ""),
			http.StatusBadRequest)
	})

	t.Run("kline clean missing code", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/kline/clean", ""),
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
	router := newContractGinEngine(t, Dependencies{})

	t.Run("get settings config load failure", func(t *testing.T) {
		t.Setenv("HOME", uncreatableHomeDir(t))
		response := doLegacyErrorRequest(t, router, http.MethodGet, "/api/settings/indicator", "")
		if response.Code == http.StatusOK {
			// A 200 is only acceptable when param's package-global config was
			// already warmed by an earlier test in this binary (AutoInit then
			// short-circuits before touching HOME, so GetConfig success proves
			// warmth). If GetConfig still fails, the config is cold and the
			// handler should have returned the 500 golden — fail loudly.
			if _, err := param.GetConfig(); err != nil {
				t.Fatalf("handler returned 200 but param config is cold: %v", err)
			}
			t.Skip("param.globalConfig already warmed; GetConfig cannot fail in this process")
		}
		assertLegacyErrorGolden(t, response, http.StatusInternalServerError)
	})

	t.Run("save settings invalid JSON body", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/settings/indicator", `{`),
			http.StatusBadRequest)
	})

	t.Run("save settings invalid payload", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/settings/indicator", `{"defaults":{}}`),
			http.StatusBadRequest)
	})

	t.Run("save settings persistence failure", func(t *testing.T) {
		t.Setenv("HOME", uncreatableHomeDir(t))
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/settings/indicator", `{"defaults":{"ma":[5,10]}}`),
			http.StatusInternalServerError)
	})
}

func TestCleanKlinesPersistenceErrorContract(t *testing.T) {
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "clean.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := tdx.NewServiceWithExecutor(stubExecutor{}, store)
	if err != nil {
		t.Fatal(err)
	}
	// Closing the storage makes every kline-store query fail, which drives
	// handleCleanKlines into its 500 legacy-error branch.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	router := newContractGinEngine(t, Dependencies{StockData: service})
	assertLegacyErrorGolden(t,
		doLegacyErrorRequest(t, router, http.MethodPost, "/api/kline/clean?code=600000", ""),
		http.StatusInternalServerError)
}
