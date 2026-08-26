package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListWithPathsAddsAndOverridesAgents(t *testing.T) {
	dir := t.TempDir()
	custom := `---
id: custom-agent
name: 自定义 Agent
description: custom
tools: [web_search]
---
Custom prompt.
`
	override := `---
id: stock-analyst
name: 自定义个股分析师
---
Override prompt.
`
	if err := os.WriteFile(filepath.Join(dir, "custom.md"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "override.md"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ListWithPaths([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]string, len(got))
	for _, agent := range got {
		byID[agent.ID] = agent.Name
	}
	if byID["custom-agent"] != "自定义 Agent" {
		t.Fatalf("custom agent not loaded: %#v", byID)
	}
	if byID["stock-analyst"] != "自定义个股分析师" {
		t.Fatalf("built-in agent was not overridden: %#v", byID)
	}
}

func TestListWithPathsRejectsMissingPath(t *testing.T) {
	if _, err := ListWithPaths([]string{filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("expected missing path error")
	}
}
