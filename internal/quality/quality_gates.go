// Package quality 实现统一质量门系统, 提供端到端质量验证能力。
//
// # 质量门类型
//
//   数据质量门 (data_quality): 检查 K 线数据完整性、重复、异常价格/成交量、时效性
//   回测黄金集门 (backtest_golden): 对比回测结果与黄金集基线, 检测回归
//   范式阶段门 (paradigm_stage): 验证范式是否满足晋级/保留条件 (分数、通过率)
//   AI 评测门 (ai_evaluation): 检查 AI 模型准确率、一致性、数据漂移
//   前向监控门 (forward_monitoring): 前向漂移检测、衰减监控、健康分
//   恢复就绪门 (recovery_readiness): 备份状态、降级能力、手动覆盖权限
//
// # 使用方式
//
// CLI:
//
//	tongstock quality check [--source-id ID] [--source-type TYPE] [--json] [--block]
//	tongstock quality verify          # 快速验证 (数据质量 + 回测)
//	tongstock quality status          # 查看配置
//
// # 恢复操作流程
//
// 当质量门被阻止 (block) 时, 系统会自动生成恢复计划:
//
//  1. 识别阻止原因: 检查被阻止的门类型和具体消息
//  2. 评估影响范围: 确定受影响的数据/模型/服务范围
//  3. 选择恢复策略:
//     - 有备份 + 可降级 → safe_mode (安全模式运行)
//     - 有备份 + 不可降级 → 恢复后重启
//     - 无备份 + 可降级 → degraded (降级运行, 立即创建备份)
//     - 无备份 + 不可降级 → not_ready (系统不可用, 需要紧急处理)
//  4. 执行恢复步骤: 按照恢复计划中的步骤逐一执行
//  5. 验证恢复: 重新运行 quality check 确认问题解决
//
// # 降级模式
//
//   safe_mode: 仅允许查询, 禁止写入和前向推理
//   readonly:  只读模式, 所有写操作被拒绝
//   no_forward: 禁用前向推理, 仅允许历史数据查询
//
// # 手动覆盖
//
// 当需要在质量门被阻止的情况下继续运行时, 可启用手动覆盖:
//   - 在 EvaluateOptions 中设置 ManualOverride = true
//   - 恢复计划中会记录覆盖操作 (ManualOverrideAllowed = true)
//   - 建议覆盖后立即创建备份并排查根本原因
//
// # 质量评分
//
// 综合分 (0-100) 由各质量门得分加权计算:
//   数据质量: 25% | 回测黄金集: 20% | 范式阶段: 15%
//   AI 评测: 15% | 前向监控: 10% | 恢复就绪: 15%
//
// 最低综合分阈值: 70 (可通过 MinOverallScore 配置调整)
package quality

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"
)

// ============================================================================
// 统一质量门 (UnifiedQualityGate)
// ============================================================================

// GateType 质量门类型
type GateType string

const (
	GateDataQuality       GateType = "data_quality"       // 数据质量门
	GateBacktestGolden    GateType = "backtest_golden"    // 回测黄金集门
	GateParadigmStage     GateType = "paradigm_stage"     // 范式阶段门
	GateAIEvaluation      GateType = "ai_evaluation"      // AI 评测门
	GateForwardMonitoring GateType = "forward_monitoring" // 前向监控门
	GateRecoveryReadiness GateType = "recovery_readiness" // 恢复就绪门
)

// GateStatus 质量门状态
type GateStatus string

const (
	GatePass    GateStatus = "pass"
	GateWarn    GateStatus = "warn"
	GateBlock   GateStatus = "block"
	GateSkipped GateStatus = "skipped"
	GateError   GateStatus = "error"
)

// UnifiedQualityReport 统一质量报告
type UnifiedQualityReport struct {
	ID         string         `json:"id"`
	RunID      string         `json:"run_id"`
	SourceID   string         `json:"source_id"`
	SourceType string         `json:"source_type"`
	Status     GateStatus     `json:"status"`
	Score      float64        `json:"score"` // 0-100

	Gates      []GateResult   `json:"gates"`
	Summary    GateSummary    `json:"summary"`
	Issues     []GateIssue    `json:"issues"`

	Decision   string         `json:"decision"` // pass / warn / block
	Blocked    bool           `json:"blocked"`

	RecoveryPlan RecoveryPlan `json:"recovery_plan"`
	GeneratedAt  time.Time    `json:"generated_at"`
}

// GateResult 单个质量门结果
type GateResult struct {
	Type       GateType    `json:"type"`
	Name       string      `json:"name"`
	Status     GateStatus  `json:"status"`
	Score      float64     `json:"score"`
	Passed     bool        `json:"passed"`
	Blocked    bool        `json:"blocked"`
	Checks     int         `json:"checks"`
	Failures   int         `json:"failures"`
	LatencyMs  int64       `json:"latency_ms"`
	Message    string      `json:"message"`
	Recommendations []string `json:"recommendations,omitempty"`
}

// GateIssue 质量门发现的问题
type GateIssue struct {
	ID        string    `json:"id"`
	GateType  GateType  `json:"gate_type"`
	Severity  Severity  `json:"severity"`
	Category  string    `json:"category"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Context   string    `json:"context,omitempty"`
}

// GateSummary 质量门汇总
type GateSummary struct {
	TotalGates     int   `json:"total_gates"`
	PassedGates    int   `json:"passed_gates"`
	WarnedGates    int   `json:"warned_gates"`
	BlockedGates   int   `json:"blocked_gates"`
	SkippedGates   int   `json:"skipped_gates"`
	TotalIssues    int   `json:"total_issues"`
	CriticalIssues int   `json:"critical_issues"`
	WarningIssues  int   `json:"warning_issues"`
	TotalLatencyMs int64 `json:"total_latency_ms"`
}

// RecoveryPlan 恢复计划
type RecoveryPlan struct {
	Status           string   `json:"status"`           // ready / degraded / not_ready
	BackupExists     bool     `json:"backup_exists"`
	LastBackupAt     string   `json:"last_backup_at,omitempty"`
	RecoverySteps    []string `json:"recovery_steps"`
	EstimatedTimeMs   int64   `json:"estimated_time_ms"`
	CanDegrade       bool     `json:"can_degrade"`
	DegradeMode      string   `json:"degrade_mode,omitempty"` // safe_mode / readonly / no_forward
	ManualOverrideAllowed bool `json:"manual_override_allowed"`
}

// ============================================================================
// 统一质量门评估器
// ============================================================================

// UnifiedQualityGate 统一质量门评估器
type UnifiedQualityGate struct {
	config          UnifiedGateConfig
	qualityChecker  *QualityChecker
	enabledGates    map[GateType]bool
}

// UnifiedGateConfig 统一质量门配置
type UnifiedGateConfig struct {
	EnableDataQuality       bool  `json:"enable_data_quality"`
	EnableBacktestGolden    bool  `json:"enable_backtest_golden"`
	EnableParadigmStage     bool  `json:"enable_paradigm_stage"`
	EnableAIEvaluation      bool  `json:"enable_ai_evaluation"`
	EnableForwardMonitoring bool  `json:"enable_forward_monitoring"`
	EnableRecoveryReadiness bool  `json:"enable_recovery_readiness"`

	BlockOnCritical   bool  `json:"block_on_critical"`
	WarnOnWarning     bool  `json:"warn_on_warning"`
	MinOverallScore   float64 `json:"min_overall_score"`

	MaxAcceptableLatencyMs int64 `json:"max_acceptable_latency_ms"`
}

// DefaultUnifiedGateConfig 返回默认配置
func DefaultUnifiedGateConfig() UnifiedGateConfig {
	return UnifiedGateConfig{
		EnableDataQuality:       true,
		EnableBacktestGolden:    true,
		EnableParadigmStage:     true,
		EnableAIEvaluation:      true,
		EnableForwardMonitoring: true,
		EnableRecoveryReadiness: true,
		BlockOnCritical:         true,
		WarnOnWarning:           true,
		MinOverallScore:         70.0,
		MaxAcceptableLatencyMs:  3000,
	}
}

// NewUnifiedQualityGate 创建统一质量门评估器
func NewUnifiedQualityGate(config UnifiedGateConfig) *UnifiedQualityGate {
	qg := &UnifiedQualityGate{
		config:         config,
		qualityChecker: NewQualityChecker(DefaultQualityGateConfig()),
		enabledGates: map[GateType]bool{
			GateDataQuality:       config.EnableDataQuality,
			GateBacktestGolden:    config.EnableBacktestGolden,
			GateParadigmStage:     config.EnableParadigmStage,
			GateAIEvaluation:      config.EnableAIEvaluation,
			GateForwardMonitoring: config.EnableForwardMonitoring,
			GateRecoveryReadiness: config.EnableRecoveryReadiness,
		},
	}
	return qg
}

// EvaluateOptions 评估选项
type EvaluateOptions struct {
	SourceID          string
	SourceType        string
	RunID             string
	SkipDataQuality   bool
	SkipBacktest      bool
	SkipAI            bool
	SkipRecovery      bool

	// 数据质量门输入
	KlineData         map[string][]KlineRecord
	ExpectedDays      map[string][]time.Time
	AsOfDate          time.Time

	// 回测黄金集输入
	BacktestResults   *BacktestGoldenResult

	// 范式阶段门输入
	ParadigmScore     *ParadigmScoreInput

	// AI 评测输入
	AIEvaluation      *AIEvaluationInput

	// 前向监控输入
	ForwardReport     *ForwardMonitorInput

	// 恢复检查
	HasBackup         bool
	LastBackupTime    time.Time
	CanDegrade        bool
	ManualOverride    bool
}

// BacktestGoldenResult 回测黄金集结果
type BacktestGoldenResult struct {
	TestPassed     bool
	TestCount      int
	FailCount      int
	TestHash       string
	GoldenHash     string
	Regressed      bool
	Description    string
}

// ParadigmScoreInput 范式阶段门输入
type ParadigmScoreInput struct {
	Stage          string
	Score          float64
	GateThreshold  float64
	Decision       string
	Transitions    int
	EvidenceCount  int
}

// AIEvaluationInput AI 评测输入
type AIEvaluationInput struct {
	ModelVersion   string
	Accuracy       float64
	Consistency    float64
	DriftDetected  bool
	LastEvalDate   time.Time
	Passed         bool
}

// ForwardMonitorInput 前向监控输入
type ForwardMonitorInput struct {
	HealthScore    float64
	DriftDetected  bool
	DecayDetected  bool
	AlertCount     int
	CriticalAlerts int
	Passed         bool
}

// Evaluate 执行统一质量门评估
func (uqg *UnifiedQualityGate) Evaluate(opts EvaluateOptions) *UnifiedQualityReport {
	report := &UnifiedQualityReport{
		ID:         fmt.Sprintf("uqr-%d", time.Now().UnixNano()),
		RunID:      opts.RunID,
		SourceID:   opts.SourceID,
		SourceType: opts.SourceType,
		GeneratedAt: time.Now(),
	}

	// 1. 数据质量门
	if uqg.enabledGates[GateDataQuality] && !opts.SkipDataQuality {
		gateResult := uqg.evaluateDataQuality(opts)
		report.Gates = append(report.Gates, gateResult)
		uqg.appendGateIssues(report, gateResult)
	}

	// 2. 回测黄金集门
	if uqg.enabledGates[GateBacktestGolden] && !opts.SkipBacktest {
		gateResult := uqg.evaluateBacktestGolden(opts)
		report.Gates = append(report.Gates, gateResult)
		uqg.appendGateIssues(report, gateResult)
	}

	// 3. 范式阶段门
	if uqg.enabledGates[GateParadigmStage] {
		gateResult := uqg.evaluateParadigmStage(opts)
		report.Gates = append(report.Gates, gateResult)
		uqg.appendGateIssues(report, gateResult)
	}

	// 4. AI 评测门
	if uqg.enabledGates[GateAIEvaluation] && !opts.SkipAI {
		gateResult := uqg.evaluateAIEvaluation(opts)
		report.Gates = append(report.Gates, gateResult)
		uqg.appendGateIssues(report, gateResult)
	}

	// 5. 前向监控门
	if uqg.enabledGates[GateForwardMonitoring] {
		gateResult := uqg.evaluateForwardMonitoring(opts)
		report.Gates = append(report.Gates, gateResult)
		uqg.appendGateIssues(report, gateResult)
	}

	// 6. 恢复就绪门
	if uqg.enabledGates[GateRecoveryReadiness] && !opts.SkipRecovery {
		gateResult := uqg.evaluateRecoveryReadiness(opts)
		report.Gates = append(report.Gates, gateResult)
		uqg.appendGateIssues(report, gateResult)
	}

	// 汇总
	report.Summary = uqg.computeSummary(report)
	report.Score = uqg.computeOverallScore(report)
	report.Status = uqg.determineOverallStatus(report, opts)
	report.Decision = string(report.Status)
	report.Blocked = report.Status == GateBlock
	report.RecoveryPlan = uqg.buildRecoveryPlan(opts, report)

	return report
}

// ============================================================================
// 各质量门评估实现
// ============================================================================

func (uqg *UnifiedQualityGate) evaluateDataQuality(opts EvaluateOptions) GateResult {
	start := time.Now()
	result := GateResult{
		Type:   GateDataQuality,
		Name:   "数据质量门",
		Checks: 0,
	}

	if len(opts.KlineData) == 0 {
		result.Status = GateSkipped
		result.Message = "无数据质量检查输入, 跳过"
		return result
	}

	totalIssues := 0
	criticalIssues := 0

	for code, records := range opts.KlineData {
		expectedDays := opts.ExpectedDays[code]
		report := uqg.qualityChecker.CheckKline(code, records, expectedDays, opts.AsOfDate)
		totalIssues += len(report.Issues)
		criticalIssues += report.Summary.CriticalCount
		result.Checks++
		result.Failures += report.Summary.CriticalCount + report.Summary.WarningCount

		for _, issue := range report.Issues {
			if issue.Severity == SeverityCritical {
				result.Recommendations = append(result.Recommendations,
					fmt.Sprintf("[%s] %s: %s", code, issue.Metric, issue.Description))
			}
		}
	}

	switch {
	case criticalIssues > 0 && uqg.config.BlockOnCritical:
		result.Status = GateBlock
		result.Passed = false
		result.Blocked = true
		result.Score = 30.0
		result.Message = fmt.Sprintf("数据质量存在 %d 个严重问题, 阻止运行", criticalIssues)
	case totalIssues > 0:
		result.Status = GateWarn
		result.Passed = true
		result.Score = 70.0
		result.Message = fmt.Sprintf("数据质量存在 %d 个问题 (含 %d 个严重), 警告", totalIssues, criticalIssues)
	default:
		result.Status = GatePass
		result.Passed = true
		result.Score = 100.0
		result.Message = "数据质量检查通过"
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

func (uqg *UnifiedQualityGate) evaluateBacktestGolden(opts EvaluateOptions) GateResult {
	start := time.Now()
	result := GateResult{
		Type: GateBacktestGolden,
		Name: "回测黄金集门",
	}

	if opts.BacktestResults == nil {
		result.Status = GateSkipped
		result.Message = "无回测结果, 跳过"
		return result
	}

	result.Checks = opts.BacktestResults.TestCount
	result.Failures = opts.BacktestResults.FailCount

	// 哈希比对
	if opts.BacktestResults.TestHash != "" && opts.BacktestResults.GoldenHash != "" {
		if opts.BacktestResults.TestHash != opts.BacktestResults.GoldenHash {
			opts.BacktestResults.Regressed = true
		}
	}

	switch {
	case opts.BacktestResults.Regressed:
		result.Status = GateBlock
		result.Passed = false
		result.Blocked = true
		result.Score = 20.0
		result.Message = "回测结果与黄金集不一致, 可能存在回归"
		result.Recommendations = append(result.Recommendations,
			"1. 立即检查代码变更\n2. 对比参数差异\n3. 验证数据版本")
	case !opts.BacktestResults.TestPassed:
		result.Status = GateBlock
		result.Passed = false
		result.Blocked = true
		result.Score = 40.0
		result.Message = fmt.Sprintf("回测失败: %d/%d 测试用例未通过", opts.BacktestResults.FailCount, opts.BacktestResults.TestCount)
	default:
		result.Status = GatePass
		result.Passed = true
		result.Score = 100.0
		result.Message = fmt.Sprintf("回测黄金集通过 (%d/%d)", opts.BacktestResults.TestCount-opts.BacktestResults.FailCount, opts.BacktestResults.TestCount)
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

func (uqg *UnifiedQualityGate) evaluateParadigmStage(opts EvaluateOptions) GateResult {
	start := time.Now()
	result := GateResult{
		Type: GateParadigmStage,
		Name: "范式阶段门",
	}

	if opts.ParadigmScore == nil {
		result.Status = GateSkipped
		result.Message = "无范式评分输入, 跳过"
		return result
	}

	result.Checks = 1
	result.Score = opts.ParadigmScore.Score

	switch {
	case opts.ParadigmScore.Score < opts.ParadigmScore.GateThreshold:
		result.Status = GateBlock
		result.Passed = false
		result.Blocked = true
		result.Failures = 1
		result.Message = fmt.Sprintf("范式得分 %.2f 低于门控阈值 %.2f, 需降级或淘汰",
			opts.ParadigmScore.Score, opts.ParadigmScore.GateThreshold)
		result.Recommendations = append(result.Recommendations,
			"1. 检查范式假设是否仍然成立\n2. 评估市场环境变化\n3. 考虑降级或淘汰")
	case opts.ParadigmScore.Decision == "reject":
		result.Status = GateBlock
		result.Passed = false
		result.Blocked = true
		result.Failures = 1
		result.Message = "范式阶段决策为拒绝"
	default:
		result.Status = GatePass
		result.Passed = true
		result.Message = fmt.Sprintf("范式阶段 '%s' 通过 (得分 %.2f)",
			opts.ParadigmScore.Stage, opts.ParadigmScore.Score)
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

func (uqg *UnifiedQualityGate) evaluateAIEvaluation(opts EvaluateOptions) GateResult {
	start := time.Now()
	result := GateResult{
		Type: GateAIEvaluation,
		Name: "AI 评测门",
	}

	if opts.AIEvaluation == nil {
		result.Status = GateSkipped
		result.Message = "无 AI 评测结果, 跳过"
		return result
	}

	result.Checks = 3
	result.Failures = 0

	if opts.AIEvaluation.Accuracy < 0.7 {
		result.Failures++
	}
	if opts.AIEvaluation.Consistency < 0.8 {
		result.Failures++
	}
	if opts.AIEvaluation.DriftDetected {
		result.Failures++
	}

	score := (opts.AIEvaluation.Accuracy + opts.AIEvaluation.Consistency) / 2 * 100
	if !opts.AIEvaluation.Passed {
		score -= 30
	}
	result.Score = score

	switch {
	case !opts.AIEvaluation.Passed || opts.AIEvaluation.DriftDetected:
		result.Status = GateBlock
		result.Passed = false
		result.Blocked = true
		result.Message = "AI 评测未通过或检测到模型漂移"
		if opts.AIEvaluation.DriftDetected {
			result.Recommendations = append(result.Recommendations,
				"模型检测到漂移, 需要重新训练或切换模型版本")
		}
	default:
		result.Status = GatePass
		result.Passed = true
		result.Message = fmt.Sprintf("AI 评测通过 (准确率 %.1f%%, 一致性 %.1f%%)",
			opts.AIEvaluation.Accuracy*100, opts.AIEvaluation.Consistency*100)
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

func (uqg *UnifiedQualityGate) evaluateForwardMonitoring(opts EvaluateOptions) GateResult {
	start := time.Now()
	result := GateResult{
		Type: GateForwardMonitoring,
		Name: "前向监控门",
	}

	if opts.ForwardReport == nil {
		result.Status = GateSkipped
		result.Message = "无前向监控结果, 跳过"
		return result
	}

	result.Checks = 4
	result.Failures = 0

	if opts.ForwardReport.DriftDetected {
		result.Failures++
		result.Recommendations = append(result.Recommendations, "检测到分布漂移")
	}
	if opts.ForwardReport.DecayDetected {
		result.Failures++
		result.Recommendations = append(result.Recommendations, "检测到性能衰减")
	}
	if opts.ForwardReport.CriticalAlerts > 0 {
		result.Failures++
	}

	result.Score = opts.ForwardReport.HealthScore

	switch {
	case !opts.ForwardReport.Passed || opts.ForwardReport.CriticalAlerts > 0:
		result.Status = GateBlock
		result.Passed = false
		result.Blocked = true
		result.Message = fmt.Sprintf("前向监控存在 %d 个严重告警", opts.ForwardReport.CriticalAlerts)
	case opts.ForwardReport.DriftDetected || opts.ForwardReport.DecayDetected:
		result.Status = GateWarn
		result.Passed = true
		result.Message = "前向监控检测到异常趋势, 需关注"
	default:
		result.Status = GatePass
		result.Passed = true
		result.Message = fmt.Sprintf("前向监控通过 (健康分 %.1f)", opts.ForwardReport.HealthScore)
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

func (uqg *UnifiedQualityGate) evaluateRecoveryReadiness(opts EvaluateOptions) GateResult {
	start := time.Now()
	result := GateResult{
		Type: GateRecoveryReadiness,
		Name: "恢复就绪门",
		Checks: 3,
	}

	blocked := false
	result.Failures = 0

	// 检查备份
	if !opts.HasBackup {
		result.Failures++
		result.Recommendations = append(result.Recommendations,
			"无有效备份, 建议立即创建备份")
	}

	// 检查降级能力
	if !opts.CanDegrade {
		result.Failures++
		result.Recommendations = append(result.Recommendations,
			"无法降级运行, 建议实现降级模式")
	}

	// 检查手动覆盖
	if !opts.ManualOverride {
		result.Failures++
	}

	score := 100.0
	if !opts.HasBackup {
		score -= 40
	}
	if !opts.CanDegrade {
		score -= 30
	}
	if opts.ManualOverride {
		score += 20
	}
	if score > 100 {
		score = 100
	}
	result.Score = score

	switch {
	case score < 60:
		result.Status = GateBlock
		result.Passed = false
		result.Blocked = true
		blocked = true
		result.Message = "系统不具备基本恢复能力"
	case score < 80:
		result.Status = GateWarn
		result.Passed = true
		result.Message = "恢复能力不完善, 建议加强"
	default:
		result.Status = GatePass
		result.Passed = true
		result.Message = "恢复就绪检查通过"
	}

	_ = blocked
	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

// ============================================================================
// 汇总与决策
// ============================================================================

func (uqg *UnifiedQualityGate) computeSummary(report *UnifiedQualityReport) GateSummary {
	summary := GateSummary{
		TotalGates: len(report.Gates),
	}

	for _, gate := range report.Gates {
		switch gate.Status {
		case GatePass:
			summary.PassedGates++
		case GateWarn:
			summary.WarnedGates++
		case GateBlock:
			summary.BlockedGates++
		case GateSkipped:
			summary.SkippedGates++
		}
		summary.TotalLatencyMs += gate.LatencyMs
	}

	summary.TotalIssues = len(report.Issues)
	for _, issue := range report.Issues {
		if issue.Severity == SeverityCritical {
			summary.CriticalIssues++
		} else if issue.Severity == SeverityWarning {
			summary.WarningIssues++
		}
	}

	return summary
}

func (uqg *UnifiedQualityGate) computeOverallScore(report *UnifiedQualityReport) float64 {
	if len(report.Gates) == 0 {
		return 0
	}

	totalScore := 0.0
	weights := map[GateType]float64{
		GateDataQuality:       25.0,
		GateBacktestGolden:    20.0,
		GateParadigmStage:     15.0,
		GateAIEvaluation:      15.0,
		GateForwardMonitoring: 10.0,
		GateRecoveryReadiness: 15.0,
	}
	totalWeight := 0.0

	for _, gate := range report.Gates {
		if gate.Status == GateSkipped {
			continue
		}
		weight := weights[gate.Type]
		totalScore += gate.Score * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 0
	}
	result := totalScore / totalWeight
	if result > 100 {
		result = 100
	}
	return result
}

func (uqg *UnifiedQualityGate) determineOverallStatus(report *UnifiedQualityReport, opts EvaluateOptions) GateStatus {
	// 有手动覆盖时不阻止
	if opts.ManualOverride {
		return GatePass
	}

	// 任一关键门阻止则整体阻止
	hasBlocked := false
	for _, gate := range report.Gates {
		if gate.Blocked {
			hasBlocked = true
			break
		}
	}

	if hasBlocked {
		return GateBlock
	}

	// 分数低于阈值也判定为阻止
	if report.Score < uqg.config.MinOverallScore {
		return GateBlock
	}

	// 有警告但不阻止
	for _, gate := range report.Gates {
		if gate.Status == GateWarn {
			return GateWarn
		}
	}

	if report.Summary.CriticalIssues > 0 && uqg.config.BlockOnCritical {
		return GateBlock
	}

	return GatePass
}

func (uqg *UnifiedQualityGate) appendGateIssues(report *UnifiedQualityReport, gate GateResult) {
	if gate.Status == GatePass {
		return
	}

	severity := SeverityWarning
	if gate.Blocked {
		severity = SeverityCritical
	}

	report.Issues = append(report.Issues, GateIssue{
		ID:       fmt.Sprintf("issue-%s-%d", gate.Type, len(report.Issues)),
		GateType: gate.Type,
		Severity: severity,
		Category: string(gate.Type),
		Title:    fmt.Sprintf("[%s] %s", gate.Name, gate.Status),
		Message:  gate.Message,
	})
}

// ============================================================================
// 恢复计划
// ============================================================================

func (uqg *UnifiedQualityGate) buildRecoveryPlan(opts EvaluateOptions, report *UnifiedQualityReport) RecoveryPlan {
	plan := RecoveryPlan{
		Status:               "ready",
		CanDegrade:           opts.CanDegrade,
		ManualOverrideAllowed: opts.ManualOverride,
	}

	// 检查报告状态
	if report.Status == GateBlock {
		if opts.CanDegrade {
			plan.Status = "degraded"
			plan.DegradeMode = "safe_mode"
		} else {
			plan.Status = "not_ready"
		}
	}

	// 备份状态
	if opts.HasBackup {
		plan.BackupExists = true
		if !opts.LastBackupTime.IsZero() {
			plan.LastBackupAt = opts.LastBackupTime.Format("2006-01-02 15:04:05")
		}
	}

	// 恢复步骤
	plan.RecoverySteps = uqg.generateRecoverySteps(report, opts)

	// 估计恢复时间
	plan.EstimatedTimeMs = uqg.estimateRecoveryTime(report)

	return plan
}

func (uqg *UnifiedQualityGate) generateRecoverySteps(report *UnifiedQualityReport, opts EvaluateOptions) []string {
	var steps []string

	// 基础恢复步骤
	if report.Status == GateBlock {
		steps = append(steps,
			"1. 确认问题范围和影响",
			"2. 启动恢复流程评估",
		)

		// 根据具体门失败生成步骤
		for _, gate := range report.Gates {
			if gate.Blocked {
				switch gate.Type {
				case GateDataQuality:
					steps = append(steps,
						fmt.Sprintf("- [数据质量] %s", gate.Message),
						"  - 检查数据源连接",
						"  - 重新拉取数据",
						"  - 验证数据完整性",
					)
				case GateBacktestGolden:
					steps = append(steps,
						fmt.Sprintf("- [回测黄金集] %s", gate.Message),
						"  - 对比当前代码与黄金集基线",
						"  - 检查参数变更",
						"  - 重新运行基准测试",
					)
				case GateParadigmStage:
					steps = append(steps,
						fmt.Sprintf("- [范式阶段] %s", gate.Message),
						"  - 重新计算范式评分",
						"  - 检查生命周期状态",
						"  - 评估是否需要降级",
					)
				case GateAIEvaluation:
					steps = append(steps,
						fmt.Sprintf("- [AI 评测] %s", gate.Message),
						"  - 检查模型版本",
						"  - 运行全套评测",
						"  - 考虑重新训练",
					)
				case GateForwardMonitoring:
					steps = append(steps,
						fmt.Sprintf("- [前向监控] %s", gate.Message),
						"  - 检查最近的监控报告",
						"  - 分析漂移/衰减原因",
						"  - 考虑暂停交易",
					)
				}
			}
		}

		steps = append(steps,
			"N. 验证恢复是否成功",
			"N+1. 更新操作日志",
		)
	}

	// 降级步骤
	if opts.CanDegrade && report.Status == GateBlock {
		steps = append(steps,
			"--- 降级模式 ---",
			"D1. 切换至只读/安全模式",
			"D2. 暂停新交易开仓",
			"D3. 保留现有仓位监控",
			"D4. 通知相关人员",
		)
	}

	// 备份恢复步骤
	if !opts.HasBackup {
		steps = append(steps,
			"--- 数据安全 ---",
			"B1. 创建紧急备份",
			"B2. 保存当前状态快照",
		)
	}

	return steps
}

func (uqg *UnifiedQualityGate) estimateRecoveryTime(report *UnifiedQualityReport) int64 {
	baseTime := int64(5 * 60 * 1000) // 5 分钟基础

	blockedGates := 0
	for _, gate := range report.Gates {
		if gate.Blocked {
			blockedGates++
		}
	}

	// 每个被阻止的门增加 3-5 分钟
	additionalTime := int64(blockedGates * 4 * 60 * 1000)

	return baseTime + additionalTime
}

// ============================================================================
// 辅助函数
// ============================================================================

// ComputeGateHash 计算报告哈希
func (r *UnifiedQualityReport) ComputeGateHash() string {
	data := fmt.Sprintf("%s|%s|%s|%d|%f",
		r.ID, r.SourceID, r.Status, len(r.Gates), r.Score)
	h := md5.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}

// SummaryString 返回可读摘要
func (r *UnifiedQualityReport) SummaryString() string {
	statusIcon := map[GateStatus]string{
		GatePass:  "✅",
		GateWarn:  "⚠️",
		GateBlock: "❌",
		GateError: "🛑",
	}

	return fmt.Sprintf("%s 质量门: %s (分数 %.1f, 通过 %d/%d, 问题 %d 项)",
		statusIcon[r.Status], r.Status, r.Score,
		r.Summary.PassedGates, r.Summary.TotalGates,
		r.Summary.TotalIssues)
}

// PassPercentage 返回通过率百分比
func (r *UnifiedQualityReport) PassPercentage() float64 {
	if r.Summary.TotalGates == 0 {
		return 0
	}
	return float64(r.Summary.PassedGates) / float64(r.Summary.TotalGates) * 100
}
