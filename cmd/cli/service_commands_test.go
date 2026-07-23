package main

import "testing"

func TestUnifiedServiceCommandsAreRegistered(t *testing.T) {
	for _, name := range []string{"server", "menubar"} {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("rootCmd.Find(%q) error = %v", name, err)
		}
		if cmd == rootCmd || cmd.Name() != name {
			t.Fatalf("command %q is not registered", name)
		}
	}
}
