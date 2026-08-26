package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sjzsdu/tongstock/internal/quality"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
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
	qualityCheckCmd.Flags().Bool("skip-paradigm", false, "跳过范式阶段检查")
	qualityCheckCmd.Flags().Bool("skip-forward", false, "跳过前向监控检查")
	qualityCheckCmd.Flags().Bool("json", false, "输出 JSON 格式")
	qualityCheckCmd.Flags().Bool("block", false, "退出码: 有 block 时返回非零")
	qualityCheckCmd.Flags().String("codes", "", "股票代码列表 (逗号分隔, 如 sh000001,sz399001)")
	qualityCheckCmd.Flags().String("kline-type", "day", "K 线类型: day/week/month/5min/15min/30min/60min")
	qualityCheckCmd.Flags().String("start", "", "开始日期 (YYYYMMDD)")
	qualityCheckCmd.Flags().String("end", "", "结束日期 (YYYYMMDD)")
	qualityCheckCmd.Flags().Bool("golden", false, "运行黄金回测测试集")
	qualityCheckCmd.Flags().String("backup-file", "", "实际备份文件路径（用于恢复就绪检查）")
	qualityCheckCmd.Flags().String("degrade-mode", "", "已配置的降级模式: safe_mode/readonly/no_forward")
}

func runQualityCheck(cmd *cobra.Command, args []string) error {
	sourceID, _ := cmd.Flags().GetString("source-id")
	sourceType, _ := cmd.Flags().GetString("source-type")
	skipData, _ := cmd.Flags().GetBool("skip-data")
	skipBacktest, _ := cmd.Flags().GetBool("skip-backtest")
	skipAI, _ := cmd.Flags().GetBool("skip-ai")
	skipRecovery, _ := cmd.Flags().GetBool("skip-recovery")
	skipParadigm, _ := cmd.Flags().GetBool("skip-paradigm")
	skipForward, _ := cmd.Flags().GetBool("skip-forward")
	jsonOut, _ := cmd.Flags().GetBool("json")
	blockExit, _ := cmd.Flags().GetBool("block")
	codesStr, _ := cmd.Flags().GetString("codes")
	ktypeStr, _ := cmd.Flags().GetString("kline-type")
	startDate, _ := cmd.Flags().GetString("start")
	endDate, _ := cmd.Flags().GetString("end")
	runGolden, _ := cmd.Flags().GetBool("golden")
	backupFile, _ := cmd.Flags().GetString("backup-file")
	degradeMode, _ := cmd.Flags().GetString("degrade-mode")

	if sourceID == "" {
		sourceID = fmt.Sprintf("manual-%d", time.Now().Unix())
	}

	config := quality.DefaultUnifiedGateConfig()
	uqg := quality.NewUnifiedQualityGate(config)

	opts := quality.EvaluateOptions{
		SourceID:        sourceID,
		SourceType:      sourceType,
		RunID:           fmt.Sprintf("run-%d", time.Now().UnixNano()),
		SkipDataQuality: skipData,
		SkipBacktest:    skipBacktest,
		SkipAI:          skipAI,
		SkipRecovery:    skipRecovery,
		AsOfDate:        time.Now(),
	}
	if skipParadigm {
		config.EnableParadigmStage = false
	}
	if skipForward {
		config.EnableForwardMonitoring = false
	}
	uqg = quality.NewUnifiedQualityGate(config)

	if !skipRecovery {
		if err := populateRecoveryState(backupFile, degradeMode, &opts); err != nil {
			return err
		}
	}

	// 如果指定了股票代码, 从数据库获取真实 K 线数据
	if codesStr != "" {
		codes := splitCodes(codesStr)
		ktype := tdx.ParseKlineType(ktypeStr)
		dataSource, err := newQualityDataSource()
		if err != nil {
			return fmt.Errorf("连接真实 K 线数据库: %w", err)
		} else if err := dataSource.FetchKlineData(codes, ktype, startDate, endDate, &opts); err != nil {
			return fmt.Errorf("获取真实 K 线数据: %w", err)
		} else {
			fmt.Printf("📊 已获取 %d 只股票的 K 线数据\n", len(codes))
		}
	}

	// 运行黄金回测测试集
	if runGolden && !skipBacktest {
		engine := quality.NewBaselineEngineAdapter()
		gs := quality.DefaultGoldenSet()
		runner := quality.NewGoldenBacktestRunner(engine, gs.Specs)

		btResult, details := runner.RunAll(context.Background())
		opts.BacktestResults = btResult

		fmt.Printf("🏆 黄金回测: %s (%d/%d 通过)\n",
			btResult.Description, btResult.TestCount-btResult.FailCount, btResult.TestCount)
		for _, d := range details {
			status := "✅"
			if !d.Passed {
				status = "❌"
			}
			fmt.Printf("   %s %s: 收益 %.4f (预期 %.4f), 交易 %d 笔\n",
				status, d.SpecID, d.ActualReturn, d.ActualReturn-d.ReturnDiff, d.ActualTrades)
		}
	}

	report := uqg.Evaluate(opts)

	if jsonOut {
		if err := printJSONReport(report); err != nil {
			return err
		}
	} else {
		printQualityReport(report)
	}

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
	return fmt.Errorf("无法查询质量报告 %q：统一质量报告尚未持久化", id)
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
			if gate.Status == quality.GatePass {
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

// newQualityDataSource 创建连接到数据库的质量数据源。
func newQualityDataSource() (*quality.QualityDataSource, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	s, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return nil, err
	}
	store, err := tdx.NewKlineStore(s)
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	adapter := quality.NewTdxKlineAdapter(store)
	return &quality.QualityDataSource{Fetcher: adapter}, nil
}

func populateRecoveryState(backupFile, degradeMode string, opts *quality.EvaluateOptions) error {
	if backupFile != "" {
		info, err := os.Stat(backupFile)
		if err != nil {
			return fmt.Errorf("验证备份文件 %q: %w", backupFile, err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("备份文件 %q 不是非空普通文件", backupFile)
		}
		opts.HasBackup = true
		opts.LastBackupTime = info.ModTime()
	}

	switch degradeMode {
	case "":
	case "safe_mode", "readonly", "no_forward":
		opts.CanDegrade = true
	default:
		return fmt.Errorf("无效降级模式 %q", degradeMode)
	}
	return nil
}

// runQualityDemo 运行端到端可复现演示。
func runQualityDemo(cmd *cobra.Command, args []string) error {
	fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║              质量门端到端可复现演示                              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	config := quality.DefaultUnifiedGateConfig()
	uqg := quality.NewUnifiedQualityGate(config)

	// 1. 准备内置测试 K 线数据 (3 只股票)
	fmt.Println("📌 步骤 1: 准备内置 K 线测试数据")
	klineData := make(map[string][]quality.KlineRecord)
	now := time.Now()

	// sh000001 - 正常数据
	klineData["sh000001"] = generateDemoKline(20, 15.0, 0.3, now)
	// sz399001 - 有一些异常
	klineData["sz399001"] = generateDemoKline(20, 8.0, 0.1, now)
	// bj899050 - 数据质量差 (缺失 + 异常价格)
	klineData["bj899050"] = generateDemoKlineWithIssues(20, 25.0, now)

	for code, records := range klineData {
		fmt.Printf("   %s: %d 条记录\n", code, len(records))
	}

	// 2. 运行黄金回测
	fmt.Println("\n📌 步骤 2: 运行黄金回测测试集")
	engine := quality.NewBaselineEngineAdapter()
	gs := quality.DefaultGoldenSet()
	runner := quality.NewGoldenBacktestRunner(engine, gs.Specs)
	btResult, btDetails := runner.RunAll(context.Background())
	fmt.Printf("   结果: %s\n", btResult.Description)
	for _, d := range btDetails {
		status := "✅"
		if !d.Passed {
			status = "❌"
		}
		fmt.Printf("   %s %s: 收益 %.4f, 交易 %d 笔\n",
			status, d.SpecID, d.ActualReturn, d.ActualTrades)
	}

	// 3. 组装质量门输入
	fmt.Println("\n📌 步骤 3: 组装质量门输入数据")
	expectedDays := make(map[string][]time.Time)
	for code := range klineData {
		expectedDays[code] = generateDemoTradingDays(20, now)
	}

	opts := quality.EvaluateOptions{
		SourceID:        fmt.Sprintf("demo-%d", time.Now().Unix()),
		SourceType:      "demo",
		RunID:           fmt.Sprintf("demo-run-%d", time.Now().UnixNano()),
		KlineData:       klineData,
		ExpectedDays:    expectedDays,
		AsOfDate:        now,
		BacktestResults: btResult,
		ParadigmScore: &quality.ParadigmScoreInput{
			Stage:         "growth",
			Score:         82.5,
			GateThreshold: 70.0,
			Decision:      "advance",
			Transitions:   2,
			EvidenceCount: 8,
		},
		AIEvaluation: &quality.AIEvaluationInput{
			ModelVersion:  "v2.1.0",
			Accuracy:      0.87,
			Consistency:   0.92,
			DriftDetected: false,
			LastEvalDate:  now,
			Passed:        true,
		},
		ForwardReport: &quality.ForwardMonitorInput{
			HealthScore:    0.88,
			DriftDetected:  false,
			DecayDetected:  false,
			AlertCount:     0,
			CriticalAlerts: 0,
			Passed:         true,
		},
		HasBackup:      true,
		LastBackupTime: now.Add(-30 * time.Minute),
		CanDegrade:     true,
		ManualOverride: false,
	}

	// 4. 运行质量门评估
	fmt.Println("\n📌 步骤 4: 运行统一质量门评估")
	report := uqg.Evaluate(opts)

	// 5. 输出结果
	fmt.Println("\n📌 步骤 5: 生成质量报告")
	printQualityReport(report)

	// 6. 输出 JSON 版本
	fmt.Println("\n📌 步骤 6: JSON 报告 (机器可读)")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("JSON 编码失败: %w", err)
	}

	// 总结
	fmt.Println("\n📌 演示完成")
	if report.Blocked {
		return fmt.Errorf("质量门被阻止: %s", report.Decision)
	}
	return nil
}

// generateDemoKline 生成演示用的 K 线数据。
func generateDemoKline(n int, startPrice, step float64, baseDate time.Time) []quality.KlineRecord {
	records := make([]quality.KlineRecord, n)
	for i := 0; i < n; i++ {
		price := startPrice + step*float64(i)
		records[i] = quality.KlineRecord{
			Date:   baseDate.AddDate(0, 0, -n+i),
			Open:   price,
			High:   price + 0.5,
			Low:    price - 0.3,
			Close:  price + step*0.5,
			Volume: float64(10000 + i*500),
			Amount: float64(price * 10000),
		}
	}
	return records
}

// generateDemoKlineWithIssues 生成包含问题的演示 K 线数据。
func generateDemoKlineWithIssues(n int, startPrice float64, baseDate time.Time) []quality.KlineRecord {
	records := make([]quality.KlineRecord, n)
	for i := 0; i < n; i++ {
		price := startPrice + float64(i)*0.5
		// 故意注入一些异常
		if i == 5 {
			price = 9999.0 // 异常高价
		}
		if i == 10 {
			price = -1.0 // 负价格
		}
		records[i] = quality.KlineRecord{
			Date:   baseDate.AddDate(0, 0, -n+i),
			Open:   price,
			High:   price + 0.5,
			Low:    price - 0.3,
			Close:  price,
			Volume: float64(10000 + i*500),
			Amount: float64(price * 10000),
		}
	}
	return records
}

// generateDemoTradingDays 生成演示用的交易日列表。
func generateDemoTradingDays(n int, baseDate time.Time) []time.Time {
	days := make([]time.Time, n)
	for i := 0; i < n; i++ {
		days[i] = baseDate.AddDate(0, 0, -n+i)
	}
	return days
}
