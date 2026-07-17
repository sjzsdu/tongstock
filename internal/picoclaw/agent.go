package picoclaw

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pclogger "github.com/sipeed/picoclaw/pkg/logger"
	pcproviders "github.com/sipeed/picoclaw/pkg/providers"
)

func (rt *Runtime) Run(opt RunOptions) error {
	if rt == nil || rt.Config == nil {
		return fmt.Errorf("picoclaw runtime not loaded")
	}

	resolved, err := rt.ResolveRunOptions(opt)
	if err != nil {
		return err
	}

	cfg := cloneConfig(rt.Config)
	if workspace := resolveRunWorkspace(opt.Workspace); workspace != "" {
		cfg = configureProjectWorkspace(cfg, workspace)
	}
	cfg = prepareConfigForRun(cfg, resolved)
	embeddedAgents := opt.embeddedAgents()
	if err := applyEmbeddedAgentConfigs(cfg, embeddedAgents, resolved.Model); err != nil {
		return err
	}
	configureLogging(opt)
	if str(resolved.Model) != "" {
		cfg.Agents.Defaults.ModelName = str(resolved.Model)
	}

	provider, modelID, err := pcproviders.CreateProvider(cfg)
	if err != nil {
		return fmt.Errorf("create picoclaw provider failed: %w", err)
	}
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}
	if stateful, ok := provider.(pcproviders.StatefulProvider); ok {
		defer stateful.Close()
	}

	loop := newAgentLoop(cfg, provider)
	defer loop.Close()
	if err := registerEmbeddedAgentPrompts(loop, embeddedAgents); err != nil {
		return err
	}

	if text := str(resolved.Message); text != "" {
		resp, err := processDirect(loop, text, resolved.Session, resolved.Agent, DefaultAgent(cfg))
		if err != nil {
			return err
		}
		callBeforeOutput(opt.BeforeOutput)
		fmt.Fprintln(os.Stdout, resp)
		return nil
	}

	return runInteractive(loop, resolved.Session)
}

func configureLogging(opt RunOptions) {
	pclogger.ConfigureFromEnv()
	if opt.Quiet && !opt.Debug {
		pclogger.DisableConsole()
	}
	if opt.Debug {
		pclogger.SetLevel(pclogger.DEBUG)
	}
}

func newAgentLoop(cfg *pcconfig.Config, provider pcproviders.LLMProvider) *pcagent.AgentLoop {
	msgBus := newMessageBus()
	return pcagent.NewAgentLoop(cfg, msgBus, provider)
}

func processDirect(loop *pcagent.AgentLoop, text, session, agentID, defaultAgent string) (string, error) {
	var (
		resp string
		err  error
	)
	if str(agentID) != "" && !strings.EqualFold(str(agentID), defaultAgent) {
		resp, err = loop.ProcessDirectForAgent(context.Background(), text, session, agentID)
	} else {
		resp, err = loop.ProcessDirect(context.Background(), text, session)
	}
	if err != nil {
		return "", fmt.Errorf("process picoclaw message failed: %w", err)
	}
	resp, err = normalizeDirectResponse(resp, nil)
	if err != nil {
		if isEmptyDirectResponseError(err) {
			resp, err = recoverEmptyDirectResponse(loop, session, agentID, defaultAgent)
			if err == nil {
				return resp, nil
			}
		}
		return "", err
	}
	return resp, nil
}

func callBeforeOutput(fn func()) {
	if fn != nil {
		fn()
	}
}

func cloneConfig(cfg *pcconfig.Config) *pcconfig.Config {
	if cfg == nil {
		return nil
	}
	cp := *cfg
	if len(cfg.Agents.List) > 0 {
		cp.Agents.List = append([]pcconfig.AgentConfig(nil), cfg.Agents.List...)
		for i := range cfg.Agents.List {
			if cfg.Agents.List[i].Model != nil {
				modelCopy := *cfg.Agents.List[i].Model
				cp.Agents.List[i].Model = &modelCopy
			}
		}
	}
	if len(cfg.ModelList) > 0 {
		cp.ModelList = make([]*pcconfig.ModelConfig, len(cfg.ModelList))
		for i := range cfg.ModelList {
			if cfg.ModelList[i] != nil {
				modelCopy := *cfg.ModelList[i]
				cp.ModelList[i] = &modelCopy
			}
		}
	}
	if len(cfg.Agents.Defaults.ModelFallbacks) > 0 {
		cp.Agents.Defaults.ModelFallbacks = append([]string(nil), cfg.Agents.Defaults.ModelFallbacks...)
	}
	return &cp
}

func configureProjectWorkspace(cfg *pcconfig.Config, workspace string) *pcconfig.Config {
	if cfg == nil || workspace == "" {
		return cfg
	}
	result := cloneConfig(cfg)
	setAgentWorkspaces(result, workspace)
	result.Agents.Defaults.RestrictToWorkspace = false
	result.Agents.Defaults.AllowReadOutsideWorkspace = true
	for _, path := range workspaceAccessPaths(workspace) {
		result.Tools.AllowReadPaths = append(result.Tools.AllowReadPaths, path)
		result.Tools.AllowWritePaths = append(result.Tools.AllowWritePaths, path)
	}
	return result
}

func workspaceAccessPaths(workspace string) []string {
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	if workspace == "" {
		return nil
	}
	paths := []string{workspace}
	if filepath.Base(workspace) == ".tongstock" {
		parent := filepath.Dir(workspace)
		if parent != "" && parent != workspace {
			paths = append(paths, parent)
		}
	}
	return paths
}

func resolveRunWorkspace(workspace string) string {
	if workspace = str(workspace); workspace != "" {
		return workspace
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

func setAgentWorkspaces(cfg *pcconfig.Config, workspace string) {
	if cfg == nil || workspace == "" {
		return
	}
	cfg.Agents.Defaults.Workspace = workspace
	for i := range cfg.Agents.List {
		cfg.Agents.List[i].Workspace = workspace
	}
}

func prepareConfigForRun(cfg *pcconfig.Config, opt ResolvedRunOptions) *pcconfig.Config {
	if cfg == nil {
		return nil
	}
	if str(opt.Model) != "" {
		cfg.Agents.Defaults.ModelName = str(opt.Model)
		for i := range cfg.Agents.List {
			cfg.Agents.List[i].Model = &pcconfig.AgentModelConfig{Primary: str(opt.Model)}
		}
	}
	selected := str(opt.Agent)
	if selected == "" {
		selected = DefaultAgent(cfg)
	}
	if strings.EqualFold(selected, DefaultAgent(cfg)) {
		return cfg
	}
	for i := range cfg.Agents.List {
		item := &cfg.Agents.List[i]
		if !strings.EqualFold(str(item.ID), selected) {
			continue
		}
		item.Default = true
		if str(opt.Model) != "" {
			item.Model = &pcconfig.AgentModelConfig{Primary: str(opt.Model)}
		}
		cfg.Agents.List = []pcconfig.AgentConfig{*item}
		cfg.Agents.Dispatch = nil
		return cfg
	}
	return cfg
}

func runInteractive(loop *pcagent.AgentLoop, sessionKey string) error {
	fmt.Fprintln(os.Stdout, "Interactive mode. Type exit or quit to leave.")
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, "> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(os.Stdout)
				return nil
			}
			return fmt.Errorf("read input failed: %w", err)
		}
		input := str(line)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			return nil
		}
		resp, err := loop.ProcessDirect(context.Background(), input, sessionKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		resp, err = normalizeDirectResponse(resp, nil)
		if err != nil {
			if isEmptyDirectResponseError(err) {
				resp, err = recoverEmptyDirectResponse(loop, sessionKey, "", "")
				if err == nil {
					fmt.Fprintln(os.Stdout, resp)
					continue
				}
			}
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		fmt.Fprintln(os.Stdout, resp)
	}
}
