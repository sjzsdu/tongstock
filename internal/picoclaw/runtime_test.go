package picoclaw

import (
	"path/filepath"
	"testing"
)

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
	if _, err := Load(Options{Backend: "builtin"}); err == nil {
		t.Fatal("expected model validation error")
	}
}
