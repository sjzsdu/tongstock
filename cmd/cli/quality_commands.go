package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sjzsdu/tongstock/internal/quality"
	"github.com/spf13/cobra"
)

var qualityCmd = &cobra.Command{
	Use:   "quality",
	Short: "质量门: 统一质量检查与端到端验证",
	Long:  `执行统一质量门检查, 覆盖数据质量、回测黄金集、范式阶段门、AI 评测、前向监控和恢复就绪`,
}

var qualityCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "执行全量质量检查",
	RunE:  runQualityCheck,
}

var qualityStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看质量门配置与状态",
	RunE:  runQualityStatus,
}

var qualityReportCmd = &cobra.Command{
	Use:   "report [id]",
	Short: "查看质量报告",
	Args:  cobra.ExactArgs(1),
	RunE:  runQualityReport,
}

var qualityVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "快速验证: 数据质量 + 回测黄金集",
	RunE:  runQualityVerify,
}

func init() {
	qualityCmd.AddCommand(qualityCheckCmd)
	qualityCmd.AddCommand(qualityStatusCmd)
	qualityCmd.AddCommand(qualityReportCmd)
	qualityCmd.AddCommand(qualityVerifyCmd)

	qualityCheckCmd.Flags().String("source-id", "", "来源 ID (如 paradigm 版本 ID)")
	qualityCheckCmd.Flags().String("source-type", "system", "来源类型: paradigm/run/system")
	qualityCheckCmd.Flags().Bool("skip-data", false, "跳过数据质量检查")
	qualityCheckCmd.Flags().Bool("skip-backtest", false, "跳过回测黄金集检查")
	qualityCheckCmd.Flags().Bool("skip-ai", false, "跳过 AI 评测检查")
	qualityCheckCmd.Flags().Bool("skip-recovery", false, "跳过恢复就绪检查")
	qualityCheckCmd.Flags().Bool("json", false, "输出 JSON 格式")
	qualityCheckCmd.Flags().Bool("block", false, "退出码: 有 block 时返回非零")
}

func runQualityCheck(cmd *cobra.Command, args []string) error {
	sourceID, _ := cmd.Flags().GetString("source-id")
	sourceType, _ := cmd.Flags().GetString("source-type")
	skipData, _ := cmd.Flags().GetBool("skip-data")
	skipBacktest, _ := cmd.Flags().GetBool("skip-backtest")
	skipAI, _ := cmd.Flags().GetBool("skip-ai")
	skipRecovery, _ := cmd.Flags().GetBool("skip-recovery")
	jsonOut, _ := cmd.Flags().GetBool("json")
	blockExit, _ := cmd.Flags().GetBool("block")

	if sourceID == "" {
		sourceID = fmt.Sprintf("manual-%d", time.Now().Unix())
	}

	config := quality.DefaultUnifiedGateConfig()
	uqg := quality.NewUnifiedQualityGate(config)

	opts := quality.EvaluateOptions{
		SourceID:      sourceID,
		SourceType:    sourceType,
		RunID:         fmt.Sprintf("run-%d", time.Now().UnixNano()),
		SkipDataQuality:   skipData,
		SkipBacktest:      skipBacktest,
		SkipAI:            skipAI,
		SkipRecovery:      skipRecovery,
		AsOfDate:          time.Now(),
		HasBackup:         true,
		LastBackupTime:    time.Now().Add(-1 * time.Hour),
		CanDegrade:        true,
		ManualOverride:    true,
	}

	report := uqg.Evaluate(opts)

	if jsonOut {
		return printJSONReport(report)
	}

	printQualityReport(report)

	if blockExit && report.Blocked {
		return fmt.Errorf("质量门被阻止: %s", report.Decision)
	}

	return nil
}

func runQualityStatus(cmd *cobra.Command, args []string) error {
	config := quality.DefaultUnifiedGateConfig()
	uqg := quality.NewUnifiedQualityGate(config)

	fmt.Println("=== 质量门配置 ===")
	fmt.Printf("数据质量门:    %s\n", statusText(config.EnableDataQuality))
	fmt.Printf("回测黄金集门:  %s\n", statusText(config.EnableBacktestGolden))
	fmt.Printf("范式阶段门:    %s\n", statusText(config.EnableParadigmStage))
	fmt.Printf("AI 评测门:     %s\n", statusText(config.EnableAIEvaluation))
	fmt.Printf("前向监控门:    %s\n", statusText(config.EnableForwardMonitoring))
	fmt.Printf("恢复就绪门:    %s\n", statusText(config.EnableRecoveryReadiness))
	fmt.Println()
	fmt.Printf("阻止严重问题:  %s\n", statusText(config.BlockOnCritical))
	fmt.Printf("警告提醒:      %s\n", statusText(config.WarnOnWarning))
	fmt.Printf("最低综合分:    %.0f\n", config.MinOverallScore)
	fmt.Printf("最大延迟:      %dms\n", config.MaxAcceptableLatencyMs)

	fmt.Println("\n=== 质量门类型说明 ===")
	fmt.Println("  data_quality       - 检查 K 线数据完整性、异常、时效性")
	fmt.Println("  backtest_golden    - 对比回测结果与黄金集基线")
	fmt.Println("  paradigm_stage     - 验证范式是否满足晋级/保留条件")
	fmt.Println("  ai_evaluation      - 检查 AI 模型准确率、一致性、漂移")
	fmt.Println("  forward_monitoring - 前向漂移检测、衰减监控、健康分")
	fmt.Println("  recovery_readiness - 备份状态、降级能力、恢复步骤")

	_ = uqg
	return nil
}

func runQualityReport(cmd *cobra.Command, args []string) error {
	id := args[0]
	fmt.Printf("查询质量报告: %s\n", id)
	fmt.Println("注意: 当前报告存储为内存模式, 使用 'quality check --json' 生成报告")
	return nil
}

func runQualityVerify(cmd *cobra.Command, args []string) error {
	config := quality.DefaultUnifiedGateConfig()
	uqg := quality.NewUnifiedQualityGate(config)

	opts := quality.EvaluateOptions{
		SourceID:   fmt.Sprintf("verify-%d", time.Now().Unix()),
		SourceType: "system",
		RunID:      fmt.Sprintf("verify-run-%d", time.Now().UnixNano()),
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
	}

	report := uqg.Evaluate(opts)

	// 只显示数据质量和回测门
	fmt.Println("=== 快速验证 ===")
	verified := 0
	for _, gate := range report.Gates {
		if gate.Type == quality.GateDataQuality || gate.Type == quality.GateBacktestGolden {
			printGateResult(gate)
			if gate.Status == quality.GatePass || gate.Status == quality.GateSkipped {
				verified++
			}
		}
	}

	if verified >= 2 {
		fmt.Println("✅ 快速验证通过")
		return nil
	}
	fmt.Println("❌ 快速验证未通过")
	return fmt.Errorf("验证失败")
}

func printQualityReport(report *quality.UnifiedQualityReport) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                     统一质量门报告                              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  ID:        %s\n", report.ID)
	fmt.Printf("  来源:      %s (%s)\n", report.SourceID, report.SourceType)
	fmt.Printf("  时间:      %s\n", report.GeneratedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	// 状态行
	statusIcon := "✅"
	statusColor := "\033[32m"
	if report.Status == quality.GateBlock {
		statusIcon = "❌"
		statusColor = "\033[31m"
	} else if report.Status == quality.GateWarn {
		statusIcon = "⚠️"
		statusColor = "\033[33m"
	}
	fmt.Printf("  状态: %s%s\033[0m %s (综合分 %.1f)\n", statusColor, statusIcon, report.Status, report.Score)
	fmt.Println()

	// 各质量门结果
	fmt.Println("  质量门结果:")
	fmt.Println("  ┌──────────────────┬────────┬──────┬──────┬────────────┐")
	fmt.Println("  │ 名称             │ 状态   │ 分数 │ 检查 │ 信息       │")
	fmt.Println("  ├──────────────────┼────────┼──────┼──────┼────────────┤")
	for _, gate := range report.Gates {
		fmt.Printf("  │ %-16s │ %-6s │ %5.1f │ %4d │ %-10s │\n",
			gate.Name, gate.Status, gate.Score, gate.Checks, truncateStr(gate.Message, 10))
	}
	fmt.Println("  └──────────────────┴────────┴──────┴──────┴────────────┘")
	fmt.Println()

	// 汇总
	summary := report.Summary
	fmt.Printf("  通过: %d/%d  |  警告: %d  |  阻止: %d  |  跳过: %d  |  问题: %d\n",
		summary.PassedGates, summary.TotalGates, summary.WarnedGates,
		summary.BlockedGates, summary.SkippedGates, summary.TotalIssues)
	fmt.Println()

	// 问题列表
	if len(report.Issues) > 0 {
		fmt.Println("  发现的问题:")
		for _, issue := range report.Issues {
			icon := "⚠️"
			if issue.Severity == quality.SeverityCritical {
				icon = "❌"
			}
			fmt.Printf("    %s [%s] %s: %s\n", icon, issue.GateType, issue.Title, issue.Message)
		}
		fmt.Println()
	}

	// 恢复计划
	if report.RecoveryPlan.Status != "" {
		plan := report.RecoveryPlan
		fmt.Println("  恢复计划:")
		fmt.Printf("    状态: %s  |  备份: %s  |  降级: %s\n",
			plan.Status, statusText(plan.BackupExists), statusText(plan.CanDegrade))
		if len(plan.RecoverySteps) > 0 {
			fmt.Println("    恢复步骤:")
			for _, step := range plan.RecoverySteps {
				fmt.Printf("      %s\n", step)
			}
		}
		fmt.Printf("    预估时间: %d 分钟\n", plan.EstimatedTimeMs/60000)
		fmt.Println()
	}

	// 建议
	for _, gate := range report.Gates {
		if len(gate.Recommendations) > 0 {
			fmt.Printf("  [%s] 建议:\n", gate.Name)
			for _, rec := range gate.Recommendations {
				fmt.Printf("    - %s\n", rec)
			}
		}
	}

	fmt.Println()
	fmt.Printf("  %s\n", report.SummaryString())
	fmt.Println()
}

func printGateResult(gate quality.GateResult) {
	statusIcon := "✅"
	if gate.Blocked {
		statusIcon = "❌"
	} else if gate.Status == quality.GateWarn {
		statusIcon = "⚠️"
	}
	fmt.Printf("  %s %s: %s (分数 %.1f)\n", statusIcon, gate.Name, gate.Message, gate.Score)
}

func printJSONReport(report *quality.UnifiedQualityReport) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func statusText(v bool) string {
	if v {
		return "启用"
	}
	return "禁用"
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
