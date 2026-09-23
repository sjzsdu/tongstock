package main

import (
	"fmt"
	"strings"

	"github.com/sjzsdu/tongstock/internal/adapter/methodresearchai"
	"github.com/sjzsdu/tongstock/internal/adapter/methodresearchrepo"
	"github.com/sjzsdu/tongstock/internal/agentservice"
	"github.com/sjzsdu/tongstock/internal/methodresearch"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/spf13/cobra"
)

var (
	methodResearchName  string
	methodResearchURL   string
	methodResearchText  string
	methodResearchCode  string
	methodResearchModel string
)

var methodResearchCmd = &cobra.Command{
	Use: "research", Short: "由 AI 查找真实来源、拆分冲突规则并编译可验证方法", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		input, err := researchCLIInput()
		if err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		// 统一的 agent 服务负责启用检查、运行时加载和 agent 定义（含
		// agent_paths），与 Server 共用同一段配置。
		svc, err := agentservice.Open(cfg.Agent, agentservice.Options{})
		if err != nil {
			return err
		}
		defer svc.Close()
		agent, ok := svc.Resolve("method-source-researcher")
		if !ok {
			return fmt.Errorf("embedded agent %q not found", "method-source-researcher")
		}
		model := firstNonEmptyResearch(methodResearchModel, cfg.Agent.Model)
		session := firstNonEmptyResearch(cfg.Agent.Session, "method-source-research")
		provider, err := methodresearchai.New(svc.Process, agent.ID, model, session, svc.Definitions())
		if err != nil {
			return err
		}
		store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
		if err != nil {
			return err
		}
		defer store.Close()
		repo, err := methodresearchrepo.New(store)
		if err != nil {
			return err
		}
		researcher, err := methodresearch.New(provider, repo)
		if err != nil {
			return err
		}
		result, err := researcher.Run(cmd.Context(), input)
		if err != nil {
			return err
		}
		return printValidationJSON(result)
	},
}

func researchCLIInput() (methodresearch.ResearchInput, error) {
	values := []struct {
		kind  methodresearch.InputKind
		value string
	}{{methodresearch.InputName, methodResearchName}, {methodresearch.InputURL, methodResearchURL}, {methodresearch.InputText, methodResearchText}}
	var selected *struct {
		kind  methodresearch.InputKind
		value string
	}
	for i := range values {
		if strings.TrimSpace(values[i].value) != "" {
			if selected != nil {
				return methodresearch.ResearchInput{}, fmt.Errorf("--name、--url、--text 必须且只能提供一个")
			}
			selected = &values[i]
		}
	}
	if selected == nil {
		return methodresearch.ResearchInput{}, fmt.Errorf("--name、--url、--text 必须且只能提供一个")
	}
	return methodresearch.ResearchInput{Kind: selected.kind, Value: strings.TrimSpace(selected.value), StockCode: strings.TrimSpace(methodResearchCode)}, nil
}

func firstNonEmptyResearch(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func init() {
	methodResearchCmd.Flags().StringVar(&methodResearchName, "name", "", "方法名称，例如：杨永兴隔夜套利法")
	methodResearchCmd.Flags().StringVar(&methodResearchURL, "url", "", "方法资料 URL（AI 会交叉核验）")
	methodResearchCmd.Flags().StringVar(&methodResearchText, "text", "", "方法自然语言描述（AI 会查找来源）")
	methodResearchCmd.Flags().StringVar(&methodResearchCode, "code", "", "可选真实股票代码；存在时自动创建验证交接")
	methodResearchCmd.Flags().StringVar(&methodResearchModel, "model", "", "可选 AI 模型覆盖")
	methodCmd.AddCommand(methodResearchCmd)
}
