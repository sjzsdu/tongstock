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

func agentName(items []Definition, id string) string {
	for _, item := range items {
		if item.ID == id {
			return item.Name
		}
	}
	return ""
}

// Definition is the TongStock-owned replacement for picoclaw.EmbeddedAgent:
// all frontmatter fields plus the Markdown body prompt are preserved.
func TestDefinitionKeepsAllFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "full.md")
	content := `---
id: full-agent
name: Full Agent
description: a fully specified agent
soul: be concise
aliases: [fa, 全量]
skills: [research]
tools: [data_snapshot]
no_history: true
---

The body prompt.
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := ListWithPaths([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	def, ok := Resolve(list, "FA") // alias, case-insensitive
	if !ok {
		t.Fatal("full-agent not resolvable by alias")
	}
	if def.ID != "full-agent" || def.Name != "Full Agent" || def.Description != "a fully specified agent" {
		t.Fatalf("identity fields = %+v", def)
	}
	if def.Prompt != "The body prompt." || def.Soul != "be concise" {
		t.Fatalf("prompt/soul = %q/%q", def.Prompt, def.Soul)
	}
	if len(def.Aliases) != 2 || len(def.Skills) != 1 || len(def.Tools) != 1 || !def.NoHistory {
		t.Fatalf("list/bool fields = %+v", def)
	}
}

// Resolve matches canonical IDs and aliases case-insensitively and returns the
// canonical definition; unknown/blank IDs do not resolve.
func TestResolve(t *testing.T) {
	defs := []Definition{
		{ID: "Risk-Reviewer", Name: "Risk Reviewer", Aliases: []string{"risk", "风险"}},
		{ID: "Stock-Analyst", Name: "Stock Analyst"},
	}
	for _, id := range []string{"RISK-REVIEWER", "  risk  ", "风险"} {
		got, ok := Resolve(defs, id)
		if !ok || got.ID != "Risk-Reviewer" {
			t.Fatalf("Resolve(%q) = (%q, %v), want canonical ID", id, got.ID, ok)
		}
	}
	for _, id := range []string{"", "   ", "missing"} {
		if _, ok := Resolve(defs, id); ok {
			t.Fatalf("Resolve(%q) unexpectedly resolved", id)
		}
	}
}

func TestListWithPathsLoadsAliasesFromSingleMarkdownFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-agent.md")
	content := `---
id: custom-risk
name: Custom Risk
aliases: [risk, 风险]
tools: [web_search]
---

Review risks.
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	list, err := ListWithPaths([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range list {
		if agent.ID != "custom-risk" {
			continue
		}
		if len(agent.Aliases) != 2 || agent.Aliases[0] != "risk" || agent.Prompt != "Review risks." {
			t.Fatalf("unexpected single-file agent: %#v", agent)
		}
		return
	}
	t.Fatal("single-file custom agent was not loaded")
}
