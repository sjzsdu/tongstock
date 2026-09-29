package main

import (
	"strings"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/config"
)

func TestUnifiedServiceCommandsAreRegistered(t *testing.T) {
	for _, name := range []string{"serve", "server"} {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("rootCmd.Find(%q) error = %v", name, err)
		}
		if cmd == rootCmd || cmd.Name() != "serve" {
			t.Fatalf("command %q is not registered (resolved to %q)", name, cmd.Name())
		}
	}

	// Subcommands must resolve through the legacy alias too.
	for _, name := range []string{"serve status", "server status"} {
		args := strings.Fields(name)
		cmd, _, err := rootCmd.Find(args)
		if err != nil {
			t.Fatalf("rootCmd.Find(%q) error = %v", name, err)
		}
		if cmd.Name() != "status" {
			t.Fatalf("subcommand %q resolved to %q, want status", name, cmd.Name())
		}
	}
}

func TestBaseURLFor(t *testing.T) {
	tests := []struct {
		bind string
		port int
		want string
	}{
		{"127.0.0.1", 8106, "http://127.0.0.1:8106"},
		{"0.0.0.0", 6565, "http://127.0.0.1:6565"},
		{"", 0, "http://127.0.0.1:8106"},
		{"::", 8106, "http://127.0.0.1:8106"},
		{"192.168.1.10", 9000, "http://192.168.1.10:9000"},
	}
	for _, tt := range tests {
		cfg := config.DefaultConfig()
		cfg.Server.BindAddress = tt.bind
		cfg.Server.Port = tt.port
		if got := baseURLFor(cfg); got != tt.want {
			t.Errorf("baseURLFor(bind=%q, port=%d) = %q, want %q", tt.bind, tt.port, got, tt.want)
		}
	}
}
