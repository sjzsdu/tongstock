package agentservice

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/tongstock/internal/picoclaw"
	"github.com/sjzsdu/tongstock/pkg/config"
)

func TestFromAgentConfigResolvesBackendHomeAndPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no user home directory")
	}
	cfg := FromAgentConfig(config.AgentConfig{
		Home:       "~/legacy-home",
		AgentPaths: []string{"~/agents", "relative/agents"},
	})
	if cfg.Backend != config.AgentBackendPicoClaw {
		t.Fatalf("backend = %q, want legacy backend inferred from home", cfg.Backend)
	}
	if cfg.Home != filepath.Join(home, "legacy-home") {
		t.Fatalf("home = %q, want expanded legacy home", cfg.Home)
	}
	want := filepath.Join(home, "agents")
	if len(cfg.AgentPaths) != 2 || cfg.AgentPaths[0] != want || cfg.AgentPaths[1] != "relative/agents" {
		t.Fatalf("agent paths = %#v, want expanded [%q relative/agents]", cfg.AgentPaths, want)
	}
	if cfg.Enabled {
		t.Fatalf("enabled must propagate from config: %+v", cfg)
	}
}

func TestFromAgentConfigFoldsLegacyScenarioFields(t *testing.T) {
	legacy := FromAgentConfig(config.AgentConfig{Agent: "old-chat", StockAgent: "old-stock"})
	if legacy.DefaultAgents[ScenarioChat] != "old-chat" || legacy.DefaultAgents[ScenarioStock] != "old-stock" {
		t.Fatalf("legacy fold = %#v, want both legacy fields mapped", legacy.DefaultAgents)
	}
	explicit := FromAgentConfig(config.AgentConfig{
		Agent:    "legacy-chat",
		Defaults: config.AgentScenarioDefaults{Chat: "new-chat"},
	})
	if explicit.DefaultAgents[ScenarioChat] != "new-chat" {
		t.Fatalf("explicit defaults.chat must win over legacy agent: %#v", explicit.DefaultAgents)
	}
	if _, exists := explicit.DefaultAgents[ScenarioStock]; exists {
		t.Fatalf("unset stock scenario must stay unset before service fallback: %#v", explicit.DefaultAgents)
	}
}

func TestOpenRejectsDisabledAgentConfig(t *testing.T) {
	if _, err := Open(config.AgentConfig{}, Options{}); err != ErrDisabled {
		t.Fatalf("Open(disabled) error = %v, want ErrDisabled", err)
	}
}

func TestNewBuiltinBackendRequiresModel(t *testing.T) {
	_, err := New(Config{Backend: "builtin", Provider: "openai", APIKeyEnv: "MISSING_KEY"}, Options{})
	if err == nil || !strings.Contains(err.Error(), "agent.model") {
		t.Fatalf("New(builtin without model) error = %v, want explicit agent.model error", err)
	}
}

func TestServiceRunResolvesAliasAndStreamsThroughFakeProvider(t *testing.T) {
	provider := newFakeProvider(t)
	defer provider.Close()
	t.Setenv("TEST_AGENT_SERVICE_KEY", "test-key")

	defs := []picoclaw.EmbeddedAgent{
		{ID: "Risk-Reviewer", Name: "Risk Reviewer", Aliases: []string{"risk"}, Prompt: "Review risk."},
	}
	svc, err := New(Config{
		Enabled:       true,
		Backend:       "builtin",
		Provider:      "openai",
		APIBase:       provider.URL,
		APIKeyEnv:     "TEST_AGENT_SERVICE_KEY",
		Model:         "test-model",
		DefaultAgents: map[string]string{ScenarioChat: "Risk-Reviewer"},
		Workspace:     t.TempDir(),
		AgentPaths:    nil,
	}, Options{Lister: func() ([]picoclaw.EmbeddedAgent, error) { return defs, nil }})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	if !svc.Ready() {
		t.Fatal("service not ready after successful construction")
	}
	got := svc.Defaults()
	if got.Agent(ScenarioChat) != "Risk-Reviewer" || got.Model != "test-model" || got.Session != defaultSession {
		t.Fatalf("defaults = %+v", got)
	}
	// The stock scenario must fall back to the chat default so every scene
	// resolves to a usable agent.
	if got.Agent(ScenarioStock) != "Risk-Reviewer" {
		t.Fatalf("stock scenario = %q, want fallback to chat default", got.Agent(ScenarioStock))
	}
	if svc.Workspace() == "" {
		t.Fatal("workspace must not be empty")
	}
	agent, ok := svc.Resolve("RISK")
	if !ok || agent.ID != "Risk-Reviewer" {
		t.Fatalf("alias resolve = (%q, %v), want canonical ID", agent.ID, ok)
	}
	if len(svc.Definitions()) != 1 {
		t.Fatalf("definitions = %d, want 1", len(svc.Definitions()))
	}

	// Blank agent/session/model fall back to defaults and still round-trip.
	resp, err := svc.Run(nil, Request{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(resp, "ok") {
		t.Fatalf("response = %q, want provider content", resp)
	}

	// The legacy adapter path used by method research maps RunOptions to Run.
	resp, err = svc.Process(t.Context(), picoclaw.RunOptions{
		Message: "via adapter", Agent: "Risk-Reviewer", Session: "research", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if !strings.Contains(resp, "ok") {
		t.Fatalf("adapter response = %q, want provider content", resp)
	}

	svc.Close()
	svc.Close() // idempotent
	if svc.Ready() {
		t.Fatal("service still ready after Close")
	}
	// Calls after Close fail instead of panicking.
	if _, err := svc.Run(nil, Request{Prompt: "late"}); err == nil {
		t.Fatal("Run after Close should fail")
	}
}

func newFakeProvider(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if streaming, _ := body["stream"].(bool); streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"content": "ok"},
				"finish_reason": "stop",
			}},
		})
	}))
}
