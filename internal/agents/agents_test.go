package agents

import (
	"os"
	"path/filepath"
	"testing"

	pcwrap "github.com/sjzsdu/tongstock/internal/picoclaw"
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

func TestListWithPathsIgnoresEmptyEntries(t *testing.T) {
	want, err := List()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ListWithPaths([]string{"", "   "})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("len(ListWithPaths(empty)) = %d, want %d", len(got), len(want))
	}
}

func TestListWithPathsLaterPathsAndFilesOverrideEarlierDefinitions(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	writeAgentFile(t, filepath.Join(firstDir, "a.md"), "ordered-agent", "first path")
	writeAgentFile(t, filepath.Join(secondDir, "a.md"), "ordered-agent", "second path early file")
	writeAgentFile(t, filepath.Join(secondDir, "z.md"), "ordered-agent", "second path late file")

	got, err := ListWithPaths([]string{firstDir, secondDir})
	if err != nil {
		t.Fatal(err)
	}
	if name := agentName(got, "ordered-agent"); name != "second path late file" {
		t.Fatalf("ordered-agent name = %q, want later file from later path", name)
	}
}

func TestListWithPathsFollowsSymlinkDirectory(t *testing.T) {
	target := t.TempDir()
	writeAgentFile(t, filepath.Join(target, "linked.md"), "linked-agent", "linked")
	link := filepath.Join(t.TempDir(), "agents-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	got, err := ListWithPaths([]string{link})
	if err != nil {
		t.Fatal(err)
	}
	if name := agentName(got, "linked-agent"); name != "linked" {
		t.Fatalf("linked-agent name = %q, want linked", name)
	}
}

func TestListWithPathsValidatesSingleFileExtensionCaseInsensitively(t *testing.T) {
	dir := t.TempDir()
	upper := filepath.Join(dir, "agent.MD")
	writeAgentFile(t, upper, "upper-agent", "upper")
	if _, err := ListWithPaths([]string{upper}); err != nil {
		t.Fatalf("uppercase Markdown extension should be accepted: %v", err)
	}

	invalid := filepath.Join(dir, "agent.txt")
	writeAgentFile(t, invalid, "text-agent", "text")
	if _, err := ListWithPaths([]string{invalid}); err == nil {
		t.Fatal("expected non-Markdown single file to be rejected")
	}
}

func writeAgentFile(t *testing.T, path, id, name string) {
	t.Helper()
	content := "---\nid: " + id + "\nname: " + name + "\n---\nPrompt.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func agentName(items []pcwrap.EmbeddedAgent, id string) string {
	for _, item := range items {
		if item.ID == id {
			return item.Name
		}
	}
	return ""
}
