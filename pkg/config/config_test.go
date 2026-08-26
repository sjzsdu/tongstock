package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAgentConfigEffectiveBackend(t *testing.T) {
	tests := []struct {
		name string
		cfg  AgentConfig
		want string
	}{
		{name: "native default", cfg: AgentConfig{}, want: AgentBackendBuiltin},
		{name: "explicit native", cfg: AgentConfig{Backend: " BUILTIN "}, want: AgentBackendBuiltin},
		{name: "explicit native wins over legacy paths", cfg: AgentConfig{Backend: "builtin", Home: "~/.picoclaw"}, want: AgentBackendBuiltin},
		{name: "explicit legacy", cfg: AgentConfig{Backend: "picoclaw"}, want: AgentBackendPicoClaw},
		{name: "legacy home migration", cfg: AgentConfig{Home: "~/.picoclaw"}, want: AgentBackendPicoClaw},
		{name: "legacy config migration", cfg: AgentConfig{Config: "~/.picoclaw/config.json"}, want: AgentBackendPicoClaw},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.EffectiveBackend(); got != tt.want {
				t.Fatalf("EffectiveBackend() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultConfigTemplateIsValidYAML(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(defaultConfigTemplate()), &cfg); err != nil {
		t.Fatalf("default config template is invalid: %v", err)
	}
}
