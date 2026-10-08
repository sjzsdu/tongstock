package server

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// Characterization tests for the convertible legacy error sites in the
// paradigm cluster (paradigm_handlers.go, paradigm_experiment_handlers.go,
// agent_research_handlers.go). Each case pins the exact status and
// byte-identical body that ErrorEnvelopeMiddleware emitted (retired in round 11). The three
// parked mixed-key sites (409 + promotion_blockers/evidence, and the two
// 422 body-building sites) are intentionally NOT covered here — they stay
// legacy until WriteErrorWithDetails exists (round 9).

type failingSaveRepo struct {
	loaded []*paradigms.Paradigm
}

func (r *failingSaveRepo) LoadAll() ([]*paradigms.Paradigm, error) { return r.loaded, nil }
func (r *failingSaveRepo) Save(*paradigms.Paradigm) error          { return errors.New("persist failed") }
func (r *failingSaveRepo) Delete(string) error                     { return nil }

func paradigmTestStore(t *testing.T, failSave bool) *paradigms.Store {
	t.Helper()
	repo := &failingSaveRepo{loaded: []*paradigms.Paradigm{{ID: "p1", Name: "n"}}}
	var store *paradigms.Store
	var err error
	if failSave {
		store, err = paradigms.NewStoreWithRepository(repo)
	} else {
		store, err = paradigms.NewStore()
	}
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestParadigmNilStoreErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})

	t.Run("review store not initialized", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/paradigm/p1/review", `{}`),
			http.StatusInternalServerError)
	})

	t.Run("delete store not initialized", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodDelete, "/api/paradigm/p1", ""),
			http.StatusInternalServerError)
	})

	t.Run("backtest storage not initialized", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/paradigm/backtest", `{}`),
			http.StatusServiceUnavailable)
	})

	t.Run("research tools not initialized", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/research", `{}`),
			http.StatusServiceUnavailable)
	})
}

func TestParadigmReviewErrorContract(t *testing.T) {
	server := NewServer(Dependencies{})
	server.SetParadigmStore(paradigmTestStore(t, true))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), Recovery())
	server.SetupRoutes(router)

	t.Run("review unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/paradigm/missing/review", `{}`),
			http.StatusNotFound)
	})

	t.Run("delete unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodDelete, "/api/paradigm/missing", ""),
			http.StatusNotFound)
	})

	t.Run("review invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/paradigm/p1/review", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("review evidence unavailable", func(t *testing.T) {
		// Verifying without an experiment registry fails the evidence check
		// (single-key 409 branch; the mixed-key 409 stays parked).
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/paradigm/p1/review", `{"review_status":"verified"}`),
			http.StatusConflict)
	})

	t.Run("review persist failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPut, "/api/paradigm/p1/review", `{"review_status":"reviewed"}`),
			http.StatusInternalServerError)
	})
}

func TestParadigmValidationErrorContract(t *testing.T) {
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "paradigm-validation.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	server := NewServer(Dependencies{Storage: store})
	server.SetParadigmStore(paradigmTestStore(t, false))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), Recovery())
	server.SetupRoutes(router)

	t.Run("backtest invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/paradigm/backtest", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("backtest missing paradigm_id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/paradigm/backtest", `{}`),
			http.StatusBadRequest)
	})

	t.Run("research invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/research", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("research missing paradigm_id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/research", `{}`),
			http.StatusBadRequest)
	})
}
