package agents

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pcwrap "github.com/sjzsdu/tongstock/internal/picoclaw"
	"gopkg.in/yaml.v3"
)

//go:embed embedded
var embeddedFS embed.FS

type definition struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Soul        string   `yaml:"soul"`
	Aliases     []string `yaml:"aliases"`
	Skills      []string `yaml:"skills"`
	Tools       []string `yaml:"tools"`
	NoHistory   bool     `yaml:"no_history"`
}

func All() ([]pcwrap.EmbeddedAgent, error) {
	return List()
}

func List() ([]pcwrap.EmbeddedAgent, error) {
	paths, err := embeddedAgentPaths()
	if err != nil {
		return nil, err
	}
	agents := make([]pcwrap.EmbeddedAgent, 0, len(paths))
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
func ListWithPaths(customPaths []string) ([]pcwrap.EmbeddedAgent, error) {
	agents, err := List()
	if err != nil {
		return nil, err
	}
	for _, path := range customPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("read agent path %s failed: %w", path, err)
		}
		if !info.IsDir() {
			agent, err := loadMarkdownAgentFile(path)
			if err != nil {
				return nil, err
			}
			agents = append(agents, agent)
			continue
		}
		err = filepath.WalkDir(path, func(filePath string, entry fs.DirEntry, walkErr error) error {
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

func mergeAgents(items []pcwrap.EmbeddedAgent) []pcwrap.EmbeddedAgent {
	agents := make([]pcwrap.EmbeddedAgent, 0, len(items))
	indexByID := map[string]int{}
	for _, agent := range items {
		key := strings.ToLower(agent.ID)
		if idx, exists := indexByID[key]; exists {
			agents[idx] = agent
		} else {
			indexByID[key] = len(agents)
			agents = append(agents, agent)
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents
}

func Get(id string) (pcwrap.EmbeddedAgent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("agent id is required")
	}
	agents, err := List()
	if err != nil {
		return pcwrap.EmbeddedAgent{}, err
	}
	for _, agent := range agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return pcwrap.EmbeddedAgent{}, fmt.Errorf("embedded agent %q not found", id)
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

func loadMarkdownAgentFS(source fs.ReadFileFS, path string) (pcwrap.EmbeddedAgent, error) {
	data, err := source.ReadFile(path)
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("read agent %s failed: %w", path, err)
	}
	return parseMarkdownAgent(path, data)
}

func loadMarkdownAgentFile(path string) (pcwrap.EmbeddedAgent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("read agent %s failed: %w", path, err)
	}
	return parseMarkdownAgent(path, data)
}

func parseMarkdownAgent(path string, data []byte) (pcwrap.EmbeddedAgent, error) {
	meta, body, err := splitFrontMatter(string(data))
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("parse agent %s failed: %w", path, err)
	}
	var def definition
	if err := yaml.Unmarshal([]byte(meta), &def); err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("parse agent %s frontmatter failed: %w", path, err)
	}
	def.ID = strings.TrimSpace(def.ID)
	def.Name = strings.TrimSpace(def.Name)
	if def.ID == "" {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("agent %s missing id", path)
	}
	if def.Name == "" {
		def.Name = def.ID
	}
	return pcwrap.EmbeddedAgent{
		ID:          def.ID,
		Name:        def.Name,
		Description: strings.TrimSpace(def.Description),
		Prompt:      strings.TrimSpace(body),
		Soul:        strings.TrimSpace(def.Soul),
		Aliases:     compactStrings(def.Aliases),
		Skills:      compactStrings(def.Skills),
		Tools:       compactStrings(def.Tools),
		NoHistory:   def.NoHistory,
	}, nil
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
