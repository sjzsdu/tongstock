package picoclaw

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultAPIKeyEnv(t *testing.T) {
	tests := map[string]string{
		" openAI ":    "OPENAI_API_KEY",
		"ANTHROPIC":   "ANTHROPIC_API_KEY",
		"DeepSeek":    "DEEPSEEK_API_KEY",
		"openrouter":  "OPENROUTER_API_KEY",
		"ZHIPU":       "ZHIPU_API_KEY",
		" ollama ":    "",
		"unknown-api": "",
	}
	for provider, want := range tests {
		t.Run(strings.TrimSpace(provider), func(t *testing.T) {
			if got := defaultAPIKeyEnv(provider); got != want {
				t.Fatalf("defaultAPIKeyEnv(%q) = %q, want %q", provider, got, want)
			}
		})
	}
}

func TestLoadBuiltinBuildsRuntimeWithoutPicoClawConfigFile(t *testing.T) {
	t.Setenv("TEST_AGENT_KEY", "secret")
	home := t.TempDir()
	rt, err := Load(Options{
		Backend:   "builtin",
		Home:      home,
		Provider:  "openai",
		Model:     "gpt-test",
		APIBase:   "http://localhost:11434/v1",
		APIKeyEnv: "TEST_AGENT_KEY",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rt.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty", rt.ConfigPath)
	}
	if rt.Config.Agents.Defaults.ModelName != "gpt-test" {
		t.Fatalf("default model = %q", rt.Config.Agents.Defaults.ModelName)
	}
	if got := rt.Config.Agents.Defaults.Workspace; got != filepath.Join(home, "workspace") {
		t.Fatalf("workspace = %q", got)
	}
	if len(rt.Config.ModelList) != 1 || rt.Config.ModelList[0].APIKey() != "secret" {
		t.Fatalf("unexpected model config: %#v", rt.Config.ModelList)
	}
}

func TestLoadBuiltinRequiresModel(t *testing.T) {
	_, err := Load(Options{Backend: "builtin"})
	if err == nil || !strings.Contains(err.Error(), "agent.model") {
		t.Fatalf("error = %v, want explicit agent.model diagnostic", err)
	}
}

func TestLoadBuiltinRequiresProvider(t *testing.T) {
	_, err := Load(Options{Backend: "builtin", Model: "test-model"})
	if err == nil || !strings.Contains(err.Error(), "agent.provider") {
		t.Fatalf("error = %v, want explicit agent.provider diagnostic", err)
	}
}

func TestLoadBuiltinRequiresRemoteProviderAPIKeyConfiguration(t *testing.T) {
	_, err := Load(Options{Backend: "builtin", Provider: "remote-compatible", Model: "test-model"})
	if err == nil || !strings.Contains(err.Error(), "agent.api_key_env") {
		t.Fatalf("error = %v, want api_key_env diagnostic", err)
	}
}

func TestLoadBuiltinRejectsMissingAPIKeyEnvironmentValue(t *testing.T) {
	t.Setenv("MISSING_AGENT_KEY", "")
	_, err := Load(Options{
		Backend: "builtin", Provider: "openai", Model: "test-model", APIKeyEnv: "MISSING_AGENT_KEY",
	})
	if err == nil || !strings.Contains(err.Error(), "MISSING_AGENT_KEY") {
		t.Fatalf("error = %v, want missing environment variable diagnostic", err)
	}
}

func TestLoadBuiltinAllowsKeylessLocalProviders(t *testing.T) {
	for _, provider := range []string{"ollama", "vllm", "lmstudio", "gpt4free", "claude-cli", "codex-cli"} {
		t.Run(provider, func(t *testing.T) {
			rt, err := Load(Options{Backend: "builtin", Home: t.TempDir(), Provider: provider, Model: "test-model"})
			if err != nil {
				t.Fatal(err)
			}
			if got := rt.Config.ModelList[0].APIKey(); got != "" {
				t.Fatalf("APIKey() = %q, want empty", got)
			}
		})
	}
}
