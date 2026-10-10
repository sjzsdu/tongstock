package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// Characterization tests for the legacy error sites in forward_handlers.go.
// Each case pins the exact status and byte-identical body that
// ErrorEnvelopeMiddleware emitted for a legacy {"error": ...} response,
// so the later WriteError conversion must not change any observable output.
// The 16 nil-ledger guards must keep returning 404 (status normalization is
// explicitly out of scope), and the 422 site maps through statusError's
// default branch to internal_error, matching today's middleware output.

// nilLedgerForwardRouter registers only the forward routes on a bare Server
// whose ledger is nil: NewServer would silently default it to an in-memory
// ledger, so the guard branches are unreachable through Dependencies.
func nilLedgerForwardRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), Recovery())
	server := &Server{}
	server.registerForwardRunRoutes(router.Group("/api"))
	return router
}

func TestForwardNilLedgerErrorContract(t *testing.T) {
	router := nilLedgerForwardRouter(t)

	t.Run("runs list", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/runs", ""),
			http.StatusNotFound)
	})

	t.Run("run create", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs", `{}`),
			http.StatusNotFound)
	})

	t.Run("run get", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/runs/run-1", ""),
			http.StatusNotFound)
	})

	t.Run("run execute", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs/run-1/execute", `{}`),
			http.StatusNotFound)
	})

	t.Run("run finalize", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs/run-1/finalize", ""),
			http.StatusNotFound)
	})

	t.Run("run signals", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/runs/run-1/signals", ""),
			http.StatusNotFound)
	})

	t.Run("signal get", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/signals/sig-1", ""),
			http.StatusNotFound)
	})

	t.Run("signals list", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/signals", ""),
			http.StatusNotFound)
	})

	t.Run("run equity", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/runs/run-1/equity", ""),
			http.StatusNotFound)
	})

	t.Run("run compare", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/runs/run-1/compare", ""),
			http.StatusNotFound)
	})
}

func TestForwardWarmLedgerErrorContract(t *testing.T) {
	// NewServer defaults Dependencies{} to an empty in-memory ledger, so
	// unknown run/signal ids drive the ledger-lookup 404 branches.
	router := newContractGinEngine(t, Dependencies{})

	t.Run("run get unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/runs/does-not-exist", ""),
			http.StatusNotFound)
	})

	t.Run("run execute unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs/does-not-exist/execute", `{}`),
			http.StatusNotFound)
	})

	t.Run("signal get unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/signals/does-not-exist", ""),
			http.StatusNotFound)
	})

	t.Run("run equity unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/runs/does-not-exist/equity", ""),
			http.StatusNotFound)
	})

	t.Run("run compare unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/runs/does-not-exist/compare", ""),
			http.StatusNotFound)
	})

	t.Run("run finalize unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs/does-not-exist/finalize", ""),
			http.StatusBadRequest)
	})

	t.Run("run create invalid JSON body", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("run create missing paradigm_version_id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs", `{}`),
			http.StatusBadRequest)
	})

	t.Run("run create invalid start_date", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs", `{"paradigm_version_id":"v1","start_date":"2026/01/01"}`),
			http.StatusBadRequest)
	})

	t.Run("signals list missing filter", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/signals", ""),
			http.StatusBadRequest)
	})

	t.Run("signals list invalid date", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/signals?date=2026/13/99", ""),
			http.StatusBadRequest)
	})
}

func TestForwardRunScopedErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})

	// Create a real run so the run-scoped execute/compare branches are
	// reachable; the run id comes back in the response body.
	var runID string
	t.Run("create run", func(t *testing.T) {
		response := doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs",
			`{"paradigm_version_id":"v1","start_date":"2026-01-01"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var body struct {
			Run struct {
				ID string `json:"id"`
			} `json:"run"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Run.ID == "" {
			t.Fatalf("no run id in response: %s", response.Body.String())
		}
		runID = body.Run.ID
	})

	t.Run("execute unknown signal id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs/"+runID+"/execute", `{"signal_id":"does-not-exist"}`),
			http.StatusNotFound)
	})

	t.Run("execute invalid date range", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/forward/runs/"+runID+"/execute", `{"from_date":"bad","to_date":"bad"}`),
			http.StatusBadRequest)
	})

	t.Run("compare without JSON body", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/forward/runs/"+runID+"/compare", ""),
			http.StatusBadRequest)
	})
}
