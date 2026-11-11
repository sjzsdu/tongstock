package agents

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed embedded
var embeddedFS embed.FS

// Definition is TongStock's own agent definition. It deliberately lives in
// internal/agents rather than internal/llm: the LLM abstraction holds no
// agent/session concepts, and business code must not depend on a vendor
// Agent/Session type. It replaces the legacy vendor EmbeddedAgent type
// one-for-one, keeping every field
// (ID/Name/Description/Prompt/Soul/Aliases/Skills/Tools/NoHistory) so the
// embedded Markdown definitions and the user extension point are unchanged.
// Prompt comes from the Markdown body, not the YAML frontmatter.
type Definition struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Prompt      string   `yaml:"-"`
	Soul        string   `yaml:"soul"`
	Aliases     []string `yaml:"aliases"`
	Skills      []string `yaml:"skills"`
	Tools       []string `yaml:"tools"`
	NoHistory   bool     `yaml:"no_history"`
}

// Resolve resolves a definition by canonical ID or alias. Matching is
// whitespace-tolerant and case-insensitive, while the returned definition
// always carries the canonical ID declared by the agent. It is the
// TongStock-owned replacement for the legacy SDK resolver and imports no SDK.
func Resolve(defs []Definition, id string) (Definition, bool) {
	want := strings.TrimSpace(id)
	if want == "" {
		return Definition{}, false
	}
	for _, def := range defs {
		if strings.EqualFold(strings.TrimSpace(def.ID), want) {
			return def, true
		}
		for _, alias := range def.Aliases {
			if strings.EqualFold(strings.TrimSpace(alias), want) {
				return def, true
			}
		}
	}
	return Definition{}, false
}

func All() ([]Definition, error) {
	return List()
}

func List() ([]Definition, error) {
	paths, err := embeddedAgentPaths()
	if err != nil {
		return nil, err
	}
	agents := make([]Definition, 0, len(paths))
	for _, path := range paths {
		agent, err := loadMarkdownAgentFS(embeddedFS, path)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return mergeAgents(agents), nil
}

// ListWithPaths returns built-in agents plus user supplied Markdown agents.
// A path may point to a single file or a directory. External definitions with
// the same ID intentionally replace built-ins, which provides a small and
// stable extension point without requiring changes to the server runtime.
func ListWithPaths(customPaths []string) ([]Definition, error) {
	agents, err := List()
	if err != nil {
		return nil, err
	}
	for _, path := range customPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		resolvedPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("resolve agent path %s failed: %w", path, err)
		}
		info, err := os.Stat(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("read agent path %s failed: %w", path, err)
		}
		if !info.IsDir() {
			agent, err := loadMarkdownAgentFile(resolvedPath)
			if err != nil {
				return nil, err
			}
			agents = append(agents, agent)
			continue
		}
		err = filepath.WalkDir(resolvedPath, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
				return nil
			}
			agent, err := loadMarkdownAgentFile(filePath)
			if err != nil {
				return err
			}
			agents = append(agents, agent)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("read agent directory %s failed: %w", path, err)
		}
	}
	return mergeAgents(agents), nil
}

func mergeAgents(items []Definition) []Definition {
	agents := make([]Definition, 0, len(items))
	indexByID := map[string]int{}
	for _, agent := range items {
		key := strings.ToLower(agent.ID)
		if idx, exists := indexByID[key]; exists {
			log.Printf("agent %q overridden by later definition", agent.ID)
			agents[idx] = agent
		} else {
			indexByID[key] = len(agents)
			agents = append(agents, agent)
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents
}

func Get(id string) (Definition, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Definition{}, fmt.Errorf("agent id is required")
	}
	agents, err := List()
	if err != nil {
		return Definition{}, err
	}
	for _, agent := range agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return Definition{}, fmt.Errorf("embedded agent %q not found", id)
}

func embeddedAgentPaths() ([]string, error) {
	paths := make([]string, 0)
	err := fs.WalkDir(embeddedFS, "embedded", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read embedded agents failed: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func loadMarkdownAgentFS(source fs.ReadFileFS, path string) (Definition, error) {
	data, err := source.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read agent %s failed: %w", path, err)
	}
	return parseMarkdownAgent(path, data)
}

func loadMarkdownAgentFile(path string) (Definition, error) {
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return Definition{}, fmt.Errorf("agent file %s must use the .md extension", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read agent %s failed: %w", path, err)
	}
	return parseMarkdownAgent(path, data)
}

func parseMarkdownAgent(path string, data []byte) (Definition, error) {
	meta, body, err := splitFrontMatter(string(data))
	if err != nil {
		return Definition{}, fmt.Errorf("parse agent %s failed: %w", path, err)
	}
	var def Definition
	if err := yaml.Unmarshal([]byte(meta), &def); err != nil {
		return Definition{}, fmt.Errorf("parse agent %s frontmatter failed: %w", path, err)
	}
	def.ID = strings.TrimSpace(def.ID)
	def.Name = strings.TrimSpace(def.Name)
	if def.ID == "" {
		return Definition{}, fmt.Errorf("agent %s missing id", path)
	}
	if def.Name == "" {
		def.Name = def.ID
	}
	def.Description = strings.TrimSpace(def.Description)
	def.Prompt = strings.TrimSpace(body)
	def.Soul = strings.TrimSpace(def.Soul)
	def.Aliases = compactStrings(def.Aliases)
	def.Skills = compactStrings(def.Skills)
	def.Tools = compactStrings(def.Tools)
	return def, nil
}

func splitFrontMatter(content string) (string, string, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---\n") {
		return "", "", fmt.Errorf("missing YAML frontmatter")
	}
	rest := strings.TrimPrefix(content, "---\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("unterminated YAML frontmatter")
	}
	meta := rest[:idx]
	body := strings.TrimPrefix(rest[idx+len("\n---"):], "\n")
	return meta, body, nil
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
