package agentservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/agents"
	"github.com/sjzsdu/tongstock/internal/ai_tools"
	"github.com/sjzsdu/tongstock/internal/llm"
	"github.com/sjzsdu/tongstock/pkg/config"
)

func testConfig() Config {
	return Config{
		Enabled:       true,
		Model:         "default-model",
		Session:       "default-session",
		DefaultAgents: map[string]string{ScenarioChat: "Risk-Reviewer"},
	}
}

func testDefs() []agents.Definition {
	return []agents.Definition{
		{ID: "Risk-Reviewer", Name: "Risk Reviewer", Aliases: []string{"risk"}, Prompt: "Review risk.", Tools: []string{"data_snapshot"}},
	}
}

// newTestService builds a Service around llm.FakeProvider. No runtime is
// constructed and no real provider/HTTP server is involved.
func newTestService(t *testing.T, fake *llm.FakeProvider, defs []agents.Definition, cfg Config) *Service {
	t.Helper()
	svc, err := New(cfg, Options{
		Provider: fake,
		Lister:   func() ([]agents.Definition, error) { return defs, nil },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func TestFromAgentConfigResolvesProviderFieldsAndPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no user home directory")
	}
	cfg := FromAgentConfig(config.AgentConfig{
		Provider:   "deepseek",
		BaseURL:    "https://api.deepseek.com/v1",
		APIKeyEnv:  "DEEPSEEK_API_KEY",
		Model:      "deepseek-chat",
		Timeout:    "90s",
		MaxTokens:  2048,
		Streaming:  true,
		AgentPaths: []string{"~/agents", "relative/agents"},
	})
	if cfg.Provider != "deepseek" || cfg.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("provider fields not carried over: %+v", cfg)
	}
	if cfg.APIKeyEnv != "DEEPSEEK_API_KEY" || cfg.Model != "deepseek-chat" {
		t.Fatalf("model fields not carried over: %+v", cfg)
	}
	if cfg.Timeout != 90*time.Second || cfg.MaxTokens != 2048 || !cfg.Streaming {
		t.Fatalf("provider options not carried over: %+v", cfg)
	}
	want := filepath.Join(home, "agents")
	if len(cfg.AgentPaths) != 2 || cfg.AgentPaths[0] != want || cfg.AgentPaths[1] != "relative/agents" {
		t.Fatalf("agent paths = %#v, want expanded [%q relative/agents]", cfg.AgentPaths, want)
	}
	if cfg.Enabled {
		t.Fatalf("enabled must propagate from config: %+v", cfg)
	}
}

// An unparsable timeout is rejected by config.Load; a programmatic caller that
// bypasses Load must still get a usable service with the adapter default.
func TestFromAgentConfigIgnoresUnparsableTimeout(t *testing.T) {
	cfg := FromAgentConfig(config.AgentConfig{Timeout: "eventually"})
	if cfg.Timeout != 0 {
		t.Fatalf("timeout = %v, want adapter default 0", cfg.Timeout)
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

func TestNewRequiresModelWhenNoProviderInjected(t *testing.T) {
	_, err := New(Config{Provider: "openai", APIKeyEnv: "MISSING_KEY"}, Options{})
	if err == nil || !strings.Contains(err.Error(), "agent.model") {
		t.Fatalf("New(no provider, no model) error = %v, want explicit agent.model error", err)
	}
}

// Without an injected provider, New builds an OpenAI-compatible provider from
// Config. This covers the production construction path without any network I/O.
func TestNewBuildsProviderFromConfig(t *testing.T) {
	t.Setenv("WAN66_AGENTSERVICE_TEST_KEY", "placeholder-not-a-secret")
	svc, err := New(Config{
		Model:     "test-model",
		BaseURL:   "https://example.com/v1",
		APIKeyEnv: "WAN66_AGENTSERVICE_TEST_KEY",
	}, Options{Lister: func() ([]agents.Definition, error) { return testDefs(), nil }})
	if err != nil {
		t.Fatalf("New(config-driven): %v", err)
	}
	defer svc.Close()
	if !svc.Ready() {
		t.Fatal("service not ready after config-driven construction")
	}
	if got := svc.provider.Name(); got != "openai-compatible" {
		t.Fatalf("provider name = %q, want openai-compatible", got)
	}
}

// Defaults, scenario fallback, alias resolution, definition copy and the
// Ready/Close lifecycle all stay unchanged after the provider migration.
func TestDefaultsResolveDefinitionsAndLifecycle(t *testing.T) {
	cfg := testConfig()
	cfg.Session = "" // exercise the package default
	svc := newTestService(t, llm.NewFakeProvider("fake"), testDefs(), cfg)
	defer svc.Close()

	got := svc.Defaults()
	if got.Agent(ScenarioChat) != "Risk-Reviewer" || got.Model != "default-model" || got.Session != defaultSession {
		t.Fatalf("defaults = %+v", got)
	}
	if got.Agent(ScenarioStock) != "Risk-Reviewer" {
		t.Fatalf("stock scenario = %q, want fallback to chat default", got.Agent(ScenarioStock))
	}
	if svc.Workspace() == "" {
		t.Fatal("workspace must not be empty")
	}
	if !svc.Ready() {
		t.Fatal("service not ready after successful construction")
	}
	if agent, ok := svc.Resolve("RISK"); !ok || agent.ID != "Risk-Reviewer" {
		t.Fatalf("alias resolve = (%q, %v), want canonical ID", agent.ID, ok)
	}
	if len(svc.Definitions()) != 1 {
		t.Fatalf("definitions = %d, want 1", len(svc.Definitions()))
	}

	svc.Close()
	svc.Close() // idempotent
	if svc.Ready() {
		t.Fatal("service still ready after Close")
	}
	if _, err := svc.Run(nil, Request{Prompt: "late"}); err == nil {
		t.Fatal("Run after Close should fail")
	}
}

// Blank agent/session/model fall back to the service defaults (Defaults +
// Config), which is the existing contract.
func TestRunResolvesDefaultsForEmptyAgentSessionModel(t *testing.T) {
	fake := llm.NewFakeProvider("fake")
	fake.QueueComplete(llm.Response{Text: "ok"}, nil)
	svc := newTestService(t, fake, testDefs(), testConfig())
	defer svc.Close()

	resp, err := svc.Run(nil, Request{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(resp, "ok") {
		t.Fatalf("response = %q, want provider content", resp)
	}
	req, ok := fake.LastRequest()
	if !ok {
		t.Fatal("no request recorded")
	}
	if req.Model != "default-model" {
		t.Fatalf("model = %q, want default-model", req.Model)
	}
	if req.SessionID != "default-session" {
		t.Fatalf("session = %q, want default-session", req.SessionID)
	}
	if req.Metadata["agent"] != "Risk-Reviewer" {
		t.Fatalf("agent metadata = %q, want default chat agent", req.Metadata["agent"])
	}
	if len(req.Messages) == 0 || req.Messages[len(req.Messages)-1].Content != "hello" || req.Messages[len(req.Messages)-1].Role != llm.RoleUser {
		t.Fatalf("unexpected messages: %+v", req.Messages)
	}
}

// chat and research call sites can still select distinct model/session values;
// the service passes them through to llm.Request unchanged.
func TestRunChatAndResearchSelectDifferentModelSession(t *testing.T) {
	fake := llm.NewFakeProvider("fake")
	fake.QueueComplete(llm.Response{Text: "chat-ok"}, nil)
	fake.QueueComplete(llm.Response{Text: "research-ok"}, nil)
	svc := newTestService(t, fake, testDefs(), testConfig())
	defer svc.Close()

	if _, err := svc.Run(context.Background(), Request{Agent: "Risk-Reviewer", Session: "chat-sess", Model: "chat-model", Prompt: "p1"}); err != nil {
		t.Fatalf("chat Run: %v", err)
	}
	if _, err := svc.Run(context.Background(), Request{Agent: "risk", Session: "research-sess", Model: "research-model", Prompt: "p2"}); err != nil {
		t.Fatalf("research Run: %v", err)
	}
	reqs := fake.Requests()
	if len(reqs) != 2 {
		t.Fatalf("got %d requests, want 2", len(reqs))
	}
	if reqs[0].Model != "chat-model" || reqs[0].SessionID != "chat-sess" {
		t.Fatalf("chat request = %+v", reqs[0])
	}
	if reqs[1].Model != "research-model" || reqs[1].SessionID != "research-sess" {
		t.Fatalf("research request = %+v", reqs[1])
	}
}

// Stream text deltas are translated into OnDelta in order at the service
// layer; the provider never receives an OnDelta callback.
func TestRunStreamDeltasForwardedToOnDeltaInOrder(t *testing.T) {
	fake := llm.NewFakeProvider("fake")
	fake.QueueStream([]llm.Event{
		{Type: llm.EventTypeTextDelta, Text: "Hel"},
		{Type: llm.EventTypeTextDelta, Text: "lo"},
		{Type: llm.EventTypeDone, Usage: &llm.Usage{TotalTokens: 3}},
	}, nil)
	svc := newTestService(t, fake, testDefs(), testConfig())
	defer svc.Close()

	var deltas []string
	resp, err := svc.Run(context.Background(), Request{
		Prompt:  "hi",
		OnDelta: func(s string) { deltas = append(deltas, s) },
	})
	if err != nil {
		t.Fatalf("Run(stream): %v", err)
	}
	if resp != "Hello" {
		t.Fatalf("response = %q, want Hello", resp)
	}
	if len(deltas) != 2 || deltas[0] != "Hel" || deltas[1] != "lo" {
		t.Fatalf("OnDelta deltas = %#v, want [Hel lo] in order", deltas)
	}
	if _, ok := fake.LastRequest(); !ok {
		t.Fatal("streaming request was not recorded")
	}
}

// The context passed to Run is bound to the provider stream, so a client
// disconnect (cancel) cancels the upstream and stops delivery.
func TestRunClientDisconnectCancelsUpstream(t *testing.T) {
	fake := llm.NewFakeProvider("fake")
	fake.QueueStream([]llm.Event{
		{Type: llm.EventTypeTextDelta, Text: "a"},
		{Type: llm.EventTypeTextDelta, Text: "b"},
		{Type: llm.EventTypeTextDelta, Text: "c"},
	}, nil)
	svc := newTestService(t, fake, testDefs(), testConfig())
	defer svc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var deltas []string
	_, err := svc.Run(ctx, Request{
		Prompt: "hi",
		OnDelta: func(s string) {
			deltas = append(deltas, s)
			cancel() // simulate client disconnect after the first delta
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run after disconnect error = %v, want context.Canceled", err)
	}
	if len(deltas) != 1 || deltas[0] != "a" {
		t.Fatalf("deltas after disconnect = %#v, want only [a] (upstream cancelled)", deltas)
	}
}

// Provider failures map onto the stable llm sentinels via errors.Is.
func TestRunProviderErrorsMapToStableSentinels(t *testing.T) {
	cases := []struct {
		code llm.ErrorCode
		want error
	}{
		{llm.CodeAuth, llm.ErrAuth},
		{llm.CodeRateLimited, llm.ErrRateLimited},
		{llm.CodeTimeout, llm.ErrTimeout},
		{llm.CodeInvalidResponse, llm.ErrInvalidResponse},
		{llm.CodeToolCallFailed, llm.ErrToolCallFailed},
	}
	for _, c := range cases {
		fake := llm.NewFakeProvider("fake")
		fake.QueueComplete(llm.Response{}, &llm.ProviderError{Provider: "fake", Code: c.code, Message: "short failure"})
		svc := newTestService(t, fake, testDefs(), testConfig())
		_, err := svc.Run(context.Background(), Request{Prompt: "hi"})
		if !errors.Is(err, c.want) {
			t.Fatalf("code %s: err = %v, want errors.Is(..., %v)", c.code, err, c.want)
		}
		svc.Close()
	}
}

// The allowlist is exactly the read-only (PermRead) set in internal/ai_tools —
// no extra tool can ever be advertised.
func TestReadOnlyToolAllowlistMatchesAiTools(t *testing.T) {
	readOnly := []ai_tools.Tool{
		ai_tools.NewDataSnapshotTool(nil),
		ai_tools.NewFeatureQueryTool(nil),
		ai_tools.NewExperimentReportTool(nil),
		ai_tools.NewParadigmVersionTool(nil),
	}
	if len(readOnlyToolDefs) != len(readOnly) {
		t.Fatalf("allowlist has %d tools, want exactly %d PermRead tools", len(readOnlyToolDefs), len(readOnly))
	}
	for _, tool := range readOnly {
		if _, ok := readOnlyToolDefs[tool.Name()]; !ok {
			t.Fatalf("read-only tool %q missing from allowlist", tool.Name())
		}
		hasRead := false
		for _, p := range tool.Permissions() {
			if p == ai_tools.PermRead {
				hasRead = true
			}
		}
		if !hasRead {
			t.Fatalf("tool %q in allowlist is not PermRead", tool.Name())
		}
	}
}

// The legacy runtime could enable write tools from a `tools:` list; the new
// registration must never surface any of them, only read-only subsets.
func TestToolDefinitionsNeverRegisterWriteTools(t *testing.T) {
	writeTools := []string{
		"exec", "bash", "shell", "write_file", "edit_file", "append_file",
		"spawn", "subagent", "read_file", "list_dir", "web_search", "web_fetch",
		"skills", "find_skills",
	}
	if got := toolDefinitions(writeTools); len(got) != 0 {
		t.Fatalf("write-only declarations produced tools: %+v", got)
	}
	mixed := toolDefinitions([]string{
		"data_snapshot", "exec", "feature_query", "write_file",
		"experiment_report", "paradigm_version", "spawn", "DATA_SNAPSHOT",
	})
	var names []string
	for _, d := range mixed {
		names = append(names, d.Name)
	}
	want := []string{"data_snapshot", "feature_query", "experiment_report", "paradigm_version"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tool names = %v, want read-only subset %v", names, want)
	}
	for _, n := range names {
		for _, w := range writeTools {
			if strings.EqualFold(n, w) {
				t.Fatalf("write tool %q leaked into registration", n)
			}
		}
	}
}

// End-to-end: an agent declaring write tools yields an llm.Request.Tools that
// contains only the read-only subset.
func TestRunAdvertisesOnlyReadOnlyTools(t *testing.T) {
	defs := []agents.Definition{
		{ID: "A", Name: "A", Tools: []string{"data_snapshot", "exec", "write_file", "feature_query"}},
	}
	cfg := testConfig()
	cfg.DefaultAgents = map[string]string{ScenarioChat: "A"}
	fake := llm.NewFakeProvider("fake")
	fake.QueueComplete(llm.Response{Text: "ok"}, nil)
	svc := newTestService(t, fake, defs, cfg)
	defer svc.Close()

	if _, err := svc.Run(context.Background(), Request{Prompt: "hi"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	req, _ := fake.LastRequest()
	if len(req.Tools) != 2 {
		t.Fatalf("advertised tools = %+v, want 2 read-only tools", req.Tools)
	}
	for _, td := range req.Tools {
		if td.Name != "data_snapshot" && td.Name != "feature_query" {
			t.Fatalf("non-read-only tool advertised: %q", td.Name)
		}
	}
}

// Process still adapts the legacy options shape to Run (used by method
// research / adapters), now with the TongStock-owned RunOptions.
func TestProcessAdaptsRunOptions(t *testing.T) {
	fake := llm.NewFakeProvider("fake")
	fake.QueueComplete(llm.Response{Text: "ok"}, nil)
	svc := newTestService(t, fake, testDefs(), testConfig())
	defer svc.Close()

	resp, err := svc.Process(context.Background(), RunOptions{
		Message: "via adapter", Agent: "Risk-Reviewer", Session: "research", Model: "adapter-model",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if !strings.Contains(resp, "ok") {
		t.Fatalf("adapter response = %q, want provider content", resp)
	}
	req, _ := fake.LastRequest()
	if req.Model != "adapter-model" || req.SessionID != "research" {
		t.Fatalf("adapter request = %+v, want model/session passed through", req)
	}
}
