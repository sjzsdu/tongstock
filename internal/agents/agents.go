package agents

import (
	"embed"
	"fmt"
	"io/fs"
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
	indexByID := map[string]int{}
	for _, path := range paths {
		agent, err := loadMarkdownAgent(path)
		if err != nil {
			return nil, err
		}
		if idx, exists := indexByID[agent.ID]; exists {
			agents[idx] = agent
		} else {
			indexByID[agent.ID] = len(agents)
			agents = append(agents, agent)
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents, nil
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

func loadMarkdownAgent(path string) (pcwrap.EmbeddedAgent, error) {
	data, err := embeddedFS.ReadFile(path)
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("read embedded agent %s failed: %w", path, err)
	}
	meta, body, err := splitFrontMatter(string(data))
	if err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("parse embedded agent %s failed: %w", path, err)
	}
	var def definition
	if err := yaml.Unmarshal([]byte(meta), &def); err != nil {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("parse embedded agent %s frontmatter failed: %w", path, err)
	}
	def.ID = strings.TrimSpace(def.ID)
	def.Name = strings.TrimSpace(def.Name)
	if def.ID == "" {
		return pcwrap.EmbeddedAgent{}, fmt.Errorf("embedded agent %s missing id", path)
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
