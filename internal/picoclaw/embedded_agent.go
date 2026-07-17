package picoclaw

import (
	"context"
	"fmt"
	"strings"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
)

const embeddedPromptSourceID pcagent.PromptSourceID = "tongstock.embedded.agent"

type embeddedAgentPromptContributor struct {
	name    string
	content string
}

func (c embeddedAgentPromptContributor) PromptSource() pcagent.PromptSourceDescriptor {
	return pcagent.PromptSourceDescriptor{
		ID:              embeddedPromptSourceID,
		Owner:           "tongstock",
		Description:     "Embedded TongStock agent instructions",
		Allowed:         []pcagent.PromptPlacement{{Layer: pcagent.PromptLayerInstruction, Slot: pcagent.PromptSlotWorkspace}},
		StableByDefault: true,
	}
}

func (c embeddedAgentPromptContributor) ContributePrompt(context.Context, pcagent.PromptBuildRequest) ([]pcagent.PromptPart, error) {
	content := str(c.content)
	if content == "" {
		return nil, nil
	}
	return []pcagent.PromptPart{{
		ID:      "tongstock.embedded.agent",
		Layer:   pcagent.PromptLayerInstruction,
		Slot:    pcagent.PromptSlotWorkspace,
		Source:  pcagent.PromptSource{ID: embeddedPromptSourceID, Name: c.name},
		Title:   c.name,
		Content: content,
		Stable:  true,
	}}, nil
}

func applyEmbeddedAgentConfig(cfg *pcconfig.Config, agent *EmbeddedAgent, model string) error {
	if cfg == nil || agent == nil {
		return nil
	}
	id := str(agent.ID)
	if id == "" {
		return fmt.Errorf("embedded agent id required")
	}
	workspace := str(cfg.Agents.Defaults.Workspace)
	if workspace == "" {
		workspace = Workspace(cfg)
	}
	name := str(agent.Name)
	if name == "" {
		name = id
	}
	var agentModel *pcconfig.AgentModelConfig
	if primaryModel := str(model); primaryModel != "" {
		agentModel = &pcconfig.AgentModelConfig{Primary: primaryModel}
	}
	removeAgentByID(cfg, id)
	cfg.Agents.List = append(cfg.Agents.List, pcconfig.AgentConfig{
		ID:        id,
		Name:      name,
		Workspace: workspace,
		Model:     agentModel,
		Skills:    append([]string(nil), agent.Skills...),
		NoHistory: agent.NoHistory,
	})
	applyEmbeddedAgentTools(cfg, agent.Tools)
	return nil
}

func applyEmbeddedAgentTools(cfg *pcconfig.Config, tools []string) {
	if cfg == nil {
		return
	}
	for _, tool := range tools {
		switch strings.ToLower(strings.TrimSpace(tool)) {
		case "skills":
			cfg.Tools.Skills.Enabled = true
		case "find_skills":
			cfg.Tools.FindSkills.Enabled = true
		case "web", "web_search":
			cfg.Tools.Web.Enabled = true
		case "web_fetch":
			cfg.Tools.WebFetch.Enabled = true
		case "exec", "bash", "shell":
			cfg.Tools.Exec.Enabled = true
		case "read_file":
			cfg.Tools.ReadFile.Enabled = true
		case "write_file":
			cfg.Tools.WriteFile.Enabled = true
		case "edit_file":
			cfg.Tools.EditFile.Enabled = true
		case "append_file":
			cfg.Tools.AppendFile.Enabled = true
		case "list_dir":
			cfg.Tools.ListDir.Enabled = true
		case "spawn":
			cfg.Tools.Spawn.Enabled = true
		case "subagent":
			cfg.Tools.Subagent.Enabled = true
		}
	}
}

func applyEmbeddedAgentConfigs(cfg *pcconfig.Config, agents []EmbeddedAgent, model string) error {
	for i := range agents {
		if err := applyEmbeddedAgentConfig(cfg, &agents[i], model); err != nil {
			return err
		}
	}
	return nil
}

func registerEmbeddedAgentPrompt(loop *pcagent.AgentLoop, agent *EmbeddedAgent) error {
	if loop == nil || agent == nil {
		return nil
	}
	id := str(agent.ID)
	if id == "" {
		return fmt.Errorf("embedded agent id required")
	}
	instance, ok := loop.GetRegistry().GetAgent(id)
	if !ok || instance == nil || instance.ContextBuilder == nil {
		return fmt.Errorf("embedded agent %q not registered", id)
	}
	content := str(agent.Prompt)
	if soul := str(agent.Soul); soul != "" {
		content = str(content + "\n\n## SOUL.md\n\n" + soul)
	}
	return instance.ContextBuilder.RegisterPromptContributor(embeddedAgentPromptContributor{
		name:    str(agent.Name),
		content: content,
	})
}

func registerEmbeddedAgentPrompts(loop *pcagent.AgentLoop, agents []EmbeddedAgent) error {
	for i := range agents {
		if err := registerEmbeddedAgentPrompt(loop, &agents[i]); err != nil {
			return err
		}
	}
	return nil
}

func removeAgentByID(cfg *pcconfig.Config, id string) {
	if cfg == nil {
		return
	}
	out := cfg.Agents.List[:0]
	for _, item := range cfg.Agents.List {
		if !strings.EqualFold(str(item.ID), id) {
			out = append(out, item)
		}
	}
	cfg.Agents.List = out
}
