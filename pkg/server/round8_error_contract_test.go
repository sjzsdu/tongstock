package server

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// Characterization tests for the legacy error sites in agent_handlers.go
// (chat session persistence), monitoring_handlers.go (alert ack/resolve)
// and strategy_handlers.go. Each case pins the exact status and
// byte-identical body that ErrorEnvelopeMiddleware emits today. The SSE
// chat-stream path is exempt from the middleware and stays untouched, and
// the empty-alert-id guards are unreachable through gin path routing.

func chatTestServer(t *testing.T, closeStorage bool) *Server {
	t.Helper()
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "chat.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	chatStore, err := NewChatStoreWithStorage(filepath.Join(t.TempDir(), "chat-files"), store)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Dependencies{Storage: store})
	server.SetChatStore(chatStore)
	if closeStorage {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return server
}

func serverContractRouter(t *testing.T, server *Server) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestID(), ErrorEnvelopeMiddleware(), Recovery())
	server.SetupRoutes(engine)
	return engine
}

func TestAgentChatNilStoreErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})

	t.Run("chat save store unavailable", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/chat/session/save", `{}`),
			http.StatusServiceUnavailable)
	})

	t.Run("chat get store unavailable", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/agent/chat/session/x", ""),
			http.StatusNotFound)
	})
}

func TestAgentChatStoreErrorContract(t *testing.T) {
	router := serverContractRouter(t, chatTestServer(t, true))

	t.Run("chat save invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/chat/session/save", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("chat save persistence failure", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/agent/chat/session/save",
				`{"id":"c1","messages":[]}`),
			http.StatusInternalServerError)
	})

	t.Run("chat get unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodGet, "/api/agent/chat/session/missing", ""),
			http.StatusNotFound)
	})
}

func TestMonitoringAlertErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})

	t.Run("alert ack unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/monitoring/alerts/does-not-exist/ack", ""),
			http.StatusNotFound)
	})

	t.Run("alert resolve unknown id", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/monitoring/alerts/does-not-exist/resolve", ""),
			http.StatusNotFound)
	})
}

func TestStrategyErrorContract(t *testing.T) {
	router := newContractGinEngine(t, Dependencies{})

	t.Run("overnight invalid JSON", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/strategy/overnight", `{invalid`),
			http.StatusBadRequest)
	})

	t.Run("overnight missing codes", func(t *testing.T) {
		assertLegacyErrorGolden(t,
			doLegacyErrorRequest(t, router, http.MethodPost, "/api/strategy/overnight", `{}`),
			http.StatusBadRequest)
	})
}
