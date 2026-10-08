package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/agentservice"
)

// Round-10 characterization: the typed-struct legacy error sites
// (json:"error,omitempty" and friends) plus the Class B error-less 503
// bodies the middleware used to wrap. Each case pins the exact status and
// byte-identical body that ErrorEnvelopeMiddleware emitted. Extras mirror the
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

func TestAgentChatValidationTypedErrorGolden(t *testing.T) {
	// agentState non-nil (no svc needed): the bind/validation branches of
	// handleAgentChat are reachable without a real agent service. Bodies are
	// agentChatResponse error-only — plain envelope bytes.
	router := chatTestDepsRouter(t, func(s *Server) { s.SetChatStore(nil) })

	t.Run("invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/chat", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("missing message", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/chat", `{"message":""}`),
			http.StatusBadRequest)
	})

	t.Run("unknown agent", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/chat", `{"message":"m","agent":"no-such-agent"}`),
			http.StatusBadRequest)
	})
}

func TestAgentChatMinerConflictGolden(t *testing.T) {
	// The miner-blocked 409 needs resolveAgentID to succeed: the miner test
	// fixture wires the scenario default to stock-paradigm-miner.
	server := NewServer(Dependencies{})
	server.SetChatStore(nil)
	server.agentState.defaults = AgentDefaults{Agents: map[string]string{
		agentservice.ScenarioChat: "stock-paradigm-miner",
	}}
	router := serverContractRouter(t, server)

	assertLegacyErrorGolden(t,
		doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/chat",
			`{"message":"直接告诉我稳定收益结论","agent":"stock-paradigm-miner"}`),
		http.StatusConflict)
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

func TestHealthReadyUnavailableProviderSchemaVersionGolden(t *testing.T) {
	// Production sets a non-zero SchemaVersion when the DB ping succeeds
	// before the TDX check fails — the payload then carries five keys and
	// the extras must preserve schema_version (omitempty mirror).
	fixed := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	router := newContractGinEngine(t, Dependencies{Diagnostics: DiagnosticsFunc(func(context.Context) Diagnostics {
		return Diagnostics{
			Status:        "unavailable",
			Service:       "tongstock",
			SchemaVersion: 5,
			Modules:       map[string]ModuleHealth{"app": {Status: "unavailable", Message: "diagnostics not configured"}},
			CheckedAt:     fixed,
		}
	})})
	assertLegacyErrorMixedGolden(t,
		doLegacyErrorRequest(t, router, http.MethodGet, "/health/ready", ""),
		http.StatusServiceUnavailable, map[string]any{
			"status":         "unavailable",
			"service":        "tongstock",
			"schema_version": 5,
			"modules":        map[string]ModuleHealth{"app": {Status: "unavailable", Message: "diagnostics not configured"}},
			"checked_at":     fixed,
		})
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

// TestErrorEnvelopeInvariant is defense-in-depth after the middleware
// retirement: representative >=400 JSON responses must still carry the
// stable envelope (error.code + request_id) straight from the handlers, so
// a future regression cannot silently reintroduce string-error bodies.
func TestErrorEnvelopeInvariant(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})
	chatStoreRouter := chatTestDepsRouter(t, func(s *Server) { s.SetChatStore(nil) })

	for _, tc := range []struct {
		name   string
		router *gin.Engine
		method string
		path   string
		want   string
	}{
		{"chat store unavailable 503", router, http.MethodPost, "/api/agent/chat", "service_unavailable"},
		{"unknown chat session 404", router, http.MethodGet, "/api/agent/chat/session/x", "not_found"},
		{"agent state diagnostics 503", router, http.MethodGet, "/api/agent/state", "service_unavailable"},
		{"paradigm review 500", chatStoreRouter, http.MethodPut, "/api/paradigm/p1/review", "internal_error"},
	} {
		response := doLegacyErrorRequest(t, tc.router, tc.method, tc.path, "{}")
		if response.Code == http.StatusOK || response.Code < 400 {
			t.Fatalf("%s: status = %d, want >= 400", tc.name, response.Code)
		}
		if ct := response.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Fatalf("%s: content-type = %q", tc.name, ct)
		}
		var envelope struct {
			Error struct {
				Code      string `json:"code"`
				RequestID string `json:"request_id"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("%s: body is not an envelope: %v; body = %s", tc.name, err, response.Body.String())
		}
		if envelope.Error.Code != tc.want || envelope.Error.RequestID == "" {
			t.Fatalf("%s: envelope = %+v, want code %q + request_id", tc.name, envelope.Error, tc.want)
		}
	}
}
