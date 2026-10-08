package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Round-10 characterization: the typed-struct legacy error sites
// (json:"error,omitempty" and friends) plus the Class B error-less 503
// bodies the middleware used to wrap. Each case pins the exact status and
// byte-identical body that ErrorEnvelopeMiddleware emits. Extras mirror the
// old struct's omitempty serialization exactly — only the keys the legacy
// body actually carried.

func chatTestDepsRouter(t *testing.T, mutate func(*Server)) *gin.Engine {
	t.Helper()
	server := NewServer(Dependencies{})
	if mutate != nil {
		mutate(server)
	}
	return serverContractRouter(t, server)
}

func TestAgentChatTypedErrorGolden(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})

	// agentChatResponse error-only body: {"error": ...} — plain WriteError.
	assertLegacyErrorGolden(t,
		doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/chat", `{}`),
		http.StatusServiceUnavailable)
}

func TestAgentStateClassBGolden(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})

	// Class B: the legacy body carries no error key; the middleware adds the
	// envelope and preserves the serialized payload keys.
	assertLegacyErrorMixedGolden(t,
		doLegacyErrorRequest(t, router, http.MethodGet, "/api/agent/state", ""),
		http.StatusServiceUnavailable, map[string]any{
			"started_at": "",
			"workspace":  "",
			"defaults":   AgentDefaults{},
			"agents":     []agentInfo{},
		})
}

func TestAgentDebateTypedErrorGolden(t *testing.T) {
	nilStateRouter := newContractGinEngine(t, Dependencies{})
	router := chatTestDepsRouter(t, func(s *Server) { s.SetChatStore(nil) })

	// agentDebateResponse error bodies always serialize stock_code, topic
	// and participants (non-omitempty), never stock_name/summary.
	debateExtras := map[string]any{
		"stock_code":   "",
		"topic":        "",
		"participants": nil,
	}

	t.Run("store unavailable", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, nilStateRouter, http.MethodPost, "/api/agent/debate", `{}`),
			http.StatusInternalServerError, debateExtras)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/debate", `{invalid`),
			http.StatusBadRequest, debateExtras)
	})

	t.Run("missing stock_code", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/debate", `{}`),
			http.StatusBadRequest, debateExtras)
	})

	t.Run("unknown agent", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/debate",
				`{"stock_code":"600000","agents":["no-such-agent"]}`),
			http.StatusBadRequest, debateExtras)
	})
}

func TestParadigmEvaluateTypedErrorGolden(t *testing.T) {
	// The store-nil 500 fires before the store matters; the 400s need the
	// store present so the bind/validation branches are reached.
	storeRouter := chatTestDepsRouter(t, func(s *Server) { s.SetChatStore(nil); s.SetParadigmStore(paradigmTestStore(t, false)) })
	nilRouter := newContractGinEngine(t, Dependencies{})

	evaluateExtras := map[string]any{
		"stock_code": "",
		"conditions": nil,
	}

	t.Run("store not initialized", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, nilRouter, http.MethodPost, "/api/paradigm/evaluate", `{}`),
			http.StatusInternalServerError, evaluateExtras)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, storeRouter, http.MethodPost, "/api/paradigm/evaluate", `{invalid`),
			http.StatusBadRequest, evaluateExtras)
	})

	t.Run("missing stock_code", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, storeRouter, http.MethodPost, "/api/paradigm/evaluate", `{}`),
			http.StatusBadRequest, evaluateExtras)
	})
}

func TestParadigmAnalyzeTypedErrorGolden(t *testing.T) {
	nilStateRouter := newContractGinEngine(t, Dependencies{})
	nilStoreRouter := chatTestDepsRouter(t, func(s *Server) { s.SetChatStore(nil) })
	storeRouter := chatTestDepsRouter(t, func(s *Server) { s.SetChatStore(nil); s.SetParadigmStore(paradigmTestStore(t, false)) })

	analyzeExtras := map[string]any{
		"stock_code": "",
		"agent_text": "",
	}

	t.Run("agent not initialized", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, nilStateRouter, http.MethodPost, "/api/paradigm/analyze", `{}`),
			http.StatusInternalServerError, analyzeExtras)
	})

	t.Run("paradigm store not initialized", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, nilStoreRouter, http.MethodPost, "/api/paradigm/analyze", `{}`),
			http.StatusInternalServerError, analyzeExtras)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, storeRouter, http.MethodPost, "/api/paradigm/analyze", `{invalid`),
			http.StatusBadRequest, analyzeExtras)
	})

	t.Run("missing stock_code", func(t *testing.T) {
		assertLegacyErrorMixedGolden(t,
			doLegacyErrorRequest(t, storeRouter, http.MethodPost, "/api/paradigm/analyze", `{}`),
			http.StatusBadRequest, analyzeExtras)
	})
}

func TestHealthReadyClassBGolden(t *testing.T) {
	// Diagnostics nil: the handler writes the unavailable Diagnostics payload
	// (with a wall-clock CheckedAt, hence the parse-based byte pinning).
	router := newContractGinEngine(t, Dependencies{})
	response := doLegacyErrorRequest(t, router, http.MethodGet, "/health/ready", "")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	extras := parsedErrorExtras(t, response)
	assertLegacyErrorMixedGolden(t, response, http.StatusServiceUnavailable, extras)

	if extras["status"] != "unavailable" || extras["service"] != "tongstock" {
		t.Fatalf("unexpected diagnostics payload: %s", response.Body.String())
	}
	modules, ok := extras["modules"].(map[string]any)
	if !ok || modules["app"] == nil {
		t.Fatalf("missing app module health: %s", response.Body.String())
	}
}

func TestHealthReadyUnavailableProviderGolden(t *testing.T) {
	// A Diagnostics provider reporting unavailable drives the second 503
	// branch; the fixed CheckedAt makes a static golden possible.
	fixed := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	router := newContractGinEngine(t, Dependencies{Diagnostics: DiagnosticsFunc(func(context.Context) Diagnostics {
		return Diagnostics{
			Status:    "unavailable",
			Service:   "tongstock",
			Modules:   map[string]ModuleHealth{"app": {Status: "unavailable", Message: "diagnostics not configured"}},
			CheckedAt: fixed,
		}
	})})
	assertLegacyErrorMixedGolden(t,
		doLegacyErrorRequest(t, router, http.MethodGet, "/health/ready", ""),
		http.StatusServiceUnavailable, map[string]any{
			"status":     "unavailable",
			"service":    "tongstock",
			"modules":    map[string]ModuleHealth{"app": {Status: "unavailable", Message: "diagnostics not configured"}},
			"checked_at": fixed,
		})
}
