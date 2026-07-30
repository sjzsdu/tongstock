package ai_quality

import (
	"fmt"
	"time"
)

// ============================================================================
// 回归门 (Regression Gate)
// ============================================================================

// GateStatus 门状态
type GateStatus string

const (
	GatePass    GateStatus = "pass"    // 全部通过
	GateWarning GateStatus = "warning" // 有警告但无阻止
	GateBlock   GateStatus = "block"   // 有关键错误, 阻止发布
)

// TestResult 单个测试用例结果
type TestResult struct {
	TestCaseID string       `json:"test_case_id"`
	Category   TestCategory `json:"category"`
	Passed     bool         `json:"passed"`
	Score      float64      `json:"score"`            // 0-1
	Issues     []string     `json:"issues,omitempty"` // 发现的问题
	DurationMs int64        `json:"duration_ms"`
}

// GateReport 回归门报告
type GateReport struct {
	ID            string       `json:"id"`
	Status        GateStatus   `json:"status"`
	Model         string       `json:"model"`
	ModelVersion  string       `json:"model_version"`
	PromptVersion string       `json:"prompt_version"`
	TestSuiteVer  string       `json:"test_suite_version"`
	Results       []TestResult `json:"results"`
	TotalTests    int          `json:"total_tests"`
	PassedCount   int          `json:"passed_count"`
	FailedCount   int          `json:"failed_count"`
	CriticalFail  int          `json:"critical_fail"`
	Score         float64      `json:"score"` // 0-1 总体评分
	StartedAt     time.Time    `json:"started_at"`
	CompletedAt   time.Time    `json:"completed_at"`
	DurationMs    int64        `json:"duration_ms"`
	// 版本变化记录
	VersionChanges []VersionChange `json:"version_changes,omitempty"`
}

// VersionChange 版本变化记录
type VersionChange struct {
	Field     string    `json:"field"`
	OldValue  string    `json:"old_value"`
	NewValue  string    `json:"new_value"`
	ChangedAt time.Time `json:"changed_at"`
	Impact    string    `json:"impact"` // "critical", "warning", "info"
}

// Passed 是否通过
func (r *GateReport) Passed() bool {
	return r.Status == GatePass || r.Status == GateWarning
}

// ============================================================================
// 回归门配置
// ============================================================================

// RegressionGateConfig 回归门配置
type RegressionGateConfig struct {
	// 关键测试用例失败是否阻止发布
	BlockOnCriticalFail bool `json:"block_on_critical_fail"` // 默认 true
	// 最低通过分数
	MinPassScore float64 `json:"min_pass_score"` // 默认 0.90
	// 允许的最大失败数量
	MaxFailures int `json:"max_failures"` // 默认 2
	// AI 相关版本
	Model            string `json:"model"`
	ModelVersion     string `json:"model_version"`
	PromptVersion    string `json:"prompt_version"`
	TestSuiteVersion string `json:"test_suite_version"`
}

// DefaultGateConfig 默认回归门配置
func DefaultGateConfig() RegressionGateConfig {
	return RegressionGateConfig{
		BlockOnCriticalFail: true,
		MinPassScore:        0.90,
		MaxFailures:         2,
		Model:               "test-model",
		ModelVersion:        "0.1",
		PromptVersion:       "v1",
		TestSuiteVersion:    "1.0",
	}
}

// ============================================================================
// 回归门实现
// ============================================================================

// RegressionGate 回归门
type RegressionGate struct {
	config  RegressionGateConfig
	library *TestCaseLibrary
	runner  TestRunner
	history []GateReport
}

// TestRunner 测试运行器接口
type TestRunner interface {
	// RunTestCase 运行单个测试用例, 返回结果
	RunTestCase(tc TestCase) TestResult
	// VersionInfo 返回版本信息
	VersionInfo() (model, modelVersion, promptVersion string)
}

// NewRegressionGate 创建回归门
func NewRegressionGate(config RegressionGateConfig, library *TestCaseLibrary, runner TestRunner) *RegressionGate {
	return &RegressionGate{
		config:  config,
		library: library,
		runner:  runner,
		history: make([]GateReport, 0),
	}
}

// Run 执行回归门
func (g *RegressionGate) Run() *GateReport {
	started := time.Now()

	// 获取版本信息
	model, modelVer, promptVer := g.runner.VersionInfo()

	report := &GateReport{
		ID:            fmt.Sprintf("gate-%d", started.UnixNano()),
		Status:        GatePass,
		Model:         model,
		ModelVersion:  modelVer,
		PromptVersion: promptVer,
		TestSuiteVer:  g.config.TestSuiteVersion,
		Results:       make([]TestResult, 0),
		TotalTests:    g.library.Count(),
		StartedAt:     started,
	}

	// 检查版本变化
	g.checkVersionChanges(report)

	// 运行所有测试
	for _, tc := range g.library.GetCases() {
		result := g.runner.RunTestCase(tc)
		report.Results = append(report.Results, result)

		if result.Passed {
			report.PassedCount++
		} else {
			report.FailedCount++
			if tc.IsCritical {
				report.CriticalFail++
			}
		}
	}

	// 计算总体评分
	report.Score = g.calculateScore(report)

	// 判定门状态
	report.Status = g.determineStatus(report)

	// 完成
	report.CompletedAt = time.Now()
	report.DurationMs = report.CompletedAt.Sub(report.StartedAt).Milliseconds()

	// 记录历史
	g.history = append(g.history, *report)

	return report
}

// GetHistory 获取历史报告
func (g *RegressionGate) GetHistory() []GateReport {
	return g.history
}

// GetLatestReport 获取最新报告
func (g *RegressionGate) GetLatestReport() *GateReport {
	if len(g.history) == 0 {
		return nil
	}
	return &g.history[len(g.history)-1]
}

// ============================================================================
// 内部方法
// ============================================================================

// calculateScore 计算总体评分 (0-1 范围)
func (g *RegressionGate) calculateScore(report *GateReport) float64 {
	if report.TotalTests == 0 {
		return 0.0
	}

	totalScore := 0.0
	for _, result := range report.Results {
		if result.Passed {
			totalScore += result.Score
		} else {
			totalScore += 0.0 // 失败得 0 分
		}
	}

	return totalScore / float64(report.TotalTests)
}

// determineStatus 判定门状态
func (g *RegressionGate) determineStatus(report *GateReport) GateStatus {
	// 关键错误阻止发布
	if g.config.BlockOnCriticalFail && report.CriticalFail > 0 {
		return GateBlock
	}

	// 超过最大失败数
	if report.FailedCount > g.config.MaxFailures {
		return GateBlock
	}

	// 分数低于阈值
	if report.Score < g.config.MinPassScore {
		return GateWarning
	}

	// 有失败但不严重
	if report.FailedCount > 0 {
		return GateWarning
	}

	return GatePass
}

// checkVersionChanges 检查版本变化
func (g *RegressionGate) checkVersionChanges(report *GateReport) {
	// 与最近一次运行比较版本
	if len(g.history) == 0 {
		return
	}

	last := g.history[len(g.history)-1]

	checks := []struct {
		field    string
		current  string
		previous string
		impact   string
	}{
		{"model", report.Model, last.Model, "critical"},
		{"model_version", report.ModelVersion, last.ModelVersion, "critical"},
		{"prompt_version", report.PromptVersion, last.PromptVersion, "warning"},
		{"test_suite_version", report.TestSuiteVer, last.TestSuiteVer, "info"},
	}

	for _, check := range checks {
		if check.current != check.previous {
			report.VersionChanges = append(report.VersionChanges, VersionChange{
				Field:     check.field,
				OldValue:  check.previous,
				NewValue:  check.current,
				ChangedAt: time.Now(),
				Impact:    check.impact,
			})
		}
	}
}

// ============================================================================
// 内置测试运行器
// ============================================================================

// DefaultTestRunner 默认测试运行器 (使用 ai_critic 进行审查)
type DefaultTestRunner struct {
	model         string
	modelVersion  string
	promptVersion string
	// 审查函数: 可以注入不同的批评者
	ReviewFunc func(input TestCaseInput) ReviewOutcome
}

// NewDefaultTestRunner 创建默认测试运行器
func NewDefaultTestRunner(model, modelVersion, promptVersion string) *DefaultTestRunner {
	return &DefaultTestRunner{
		model:         model,
		modelVersion:  modelVersion,
		promptVersion: promptVersion,
		ReviewFunc:    DefaultReviewFunc,
	}
}

// RunTestCase 运行单个测试用例
func (r *DefaultTestRunner) RunTestCase(tc TestCase) TestResult {
	start := time.Now()

	// 执行审查
	outcome := r.ReviewFunc(tc.Input)

	// 评估是否通过
	passed := r.evaluateOutcome(tc, outcome)

	// 计算分数
	score := r.calculateOutcomeScore(tc, outcome)

	// 收集问题
	var issues []string
	for _, issue := range outcome.Issues {
		issues = append(issues, fmt.Sprintf("%s: %s (severity=%s)", issue.Dimension, issue.Title, issue.Severity))
	}

	return TestResult{
		TestCaseID: tc.ID,
		Category:   tc.Category,
		Passed:     passed,
		Score:      score,
		Issues:     issues,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// VersionInfo 返回版本信息
func (r *DefaultTestRunner) VersionInfo() (model, modelVersion, promptVersion string) {
	return r.model, r.modelVersion, r.promptVersion
}

// evaluateOutcome 评估测试用例结果
func (r *DefaultTestRunner) evaluateOutcome(tc TestCase, outcome ReviewOutcome) bool {
	// 检查结论是否在期望列表中
	if !stringInSlice(string(outcome.Conclusion), tc.Expectation.ExpectConclusions) {
		return false
	}

	// 检查必须存在的维度
	for _, dim := range tc.Expectation.MustHave {
		if !dimensionInOutcome(dim, outcome) {
			return false
		}
	}

	// 检查禁止存在的维度
	for _, dim := range tc.Expectation.MustNotHave {
		if dimensionInOutcome(dim, outcome) {
			return false
		}
	}

	// 检查维度数量
	for dim, minCount := range tc.Expectation.ExpectDimensionCounts {
		count := countDimensionInOutcome(dim, outcome)
		if count < minCount {
			return false
		}
	}

	return true
}

// calculateOutcomeScore 计算结果分数 (0-1)
func (r *DefaultTestRunner) calculateOutcomeScore(tc TestCase, outcome ReviewOutcome) float64 {
	if len(tc.Expectation.ExpectConclusions) == 0 {
		return 0.5
	}

	if stringInSlice(string(outcome.Conclusion), tc.Expectation.ExpectConclusions) {
		return 0.8
	}

	return 0.2
}

// ============================================================================
// 审查函数 (默认实现)
// ============================================================================

// ReviewOutcome 审查结果 (与 ai_critic.ReviewOutcome 兼容的简化版)
type ReviewOutcome struct {
	Conclusion   string        `json:"conclusion"`
	Issues       []ReviewIssue `json:"issues"`
	HasHardBlock bool          `json:"has_hard_block"`
}

// ReviewIssue 审查问题
type ReviewIssue struct {
	Dimension string `json:"dimension"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
}

// DefaultReviewFunc 默认审查函数 (启发式规则实现)
func DefaultReviewFunc(input TestCaseInput) ReviewOutcome {
	var issues []ReviewIssue
	hasHardBlock := false

	// 1. 数据泄漏检查
	if input.EmbargoDays < 5 {
		issues = append(issues, ReviewIssue{
			Dimension: "data_leakage",
			Severity:  "critical",
			Title:     "隔离期过短",
		})
		hasHardBlock = true
	}
	if input.PurgeDays < 3 {
		issues = append(issues, ReviewIssue{
			Dimension: "data_leakage",
			Severity:  "critical",
			Title:     "清洗期过短",
		})
		hasHardBlock = true
	}

	// 2. 样本量检查
	if input.SampleSize < 30 {
		issues = append(issues, ReviewIssue{
			Dimension: "sample_size",
			Severity:  "critical",
			Title:     "样本量不足",
		})
		hasHardBlock = true
	}
	if input.TotalTrades < 10 {
		issues = append(issues, ReviewIssue{
			Dimension: "sample_size",
			Severity:  "critical",
			Title:     "交易次数过少",
		})
		hasHardBlock = true
	}

	// 3. 成本检查
	if input.CostRatio > 0.30 {
		issues = append(issues, ReviewIssue{
			Dimension: "cost_sensitivity",
			Severity:  "critical",
			Title:     "成本占比过高",
		})
		hasHardBlock = true
	}

	// 4. 选择偏差检查
	if input.FeatureCount > 20 {
		issues = append(issues, ReviewIssue{
			Dimension: "selection_bias",
			Severity:  "medium",
			Title:     "特征数量过多",
		})
	}

	// 5. 集中度检查
	if input.MaxPositionWeight > 0.15 {
		issues = append(issues, ReviewIssue{
			Dimension: "concentration",
			Severity:  "high",
			Title:     "单票权重过高",
		})
	}
	if input.Concentration > 0.50 {
		issues = append(issues, ReviewIssue{
			Dimension: "concentration",
			Severity:  "high",
			Title:     "集中度过高",
		})
	}

	// 6. 叙事偏差检查
	if input.SharpeRatio > 5.0 {
		issues = append(issues, ReviewIssue{
			Dimension: "narrative_bias",
			Severity:  "high",
			Title:     "夏普比率异常高",
		})
	}
	if input.WinRate > 0.85 {
		issues = append(issues, ReviewIssue{
			Dimension: "narrative_bias",
			Severity:  "medium",
			Title:     "胜率异常高",
		})
	}

	// 7. 基线比较检查
	if input.TotalReturn > 0 && input.BaselineReturn > input.TotalReturn {
		issues = append(issues, ReviewIssue{
			Dimension: "baseline_compare",
			Severity:  "critical",
			Title:     "策略跑输基准",
		})
		hasHardBlock = true
	} else if input.TotalReturn-input.BaselineReturn < 0.02 {
		issues = append(issues, ReviewIssue{
			Dimension: "baseline_compare",
			Severity:  "medium",
			Title:     "超额收益不足",
		})
	}

	// 计算结论
	conclusion := computeConclusion(issues, hasHardBlock)

	return ReviewOutcome{
		Conclusion:   conclusion,
		Issues:       issues,
		HasHardBlock: hasHardBlock,
	}
}

// computeConclusion 计算审查结论
func computeConclusion(issues []ReviewIssue, hasHardBlock bool) string {
	if hasHardBlock {
		return "block"
	}

	hasHigh := false
	hasMedium := false
	for _, issue := range issues {
		switch issue.Severity {
		case "high":
			hasHigh = true
		case "medium":
			hasMedium = true
		}
	}

	if hasHigh {
		return "fail"
	}
	if hasMedium {
		return "needs_review"
	}
	if len(issues) > 0 {
		return "pass_notes"
	}
	return "pass"
}

// ============================================================================
// 辅助函数
// ============================================================================

func stringInSlice(s string, slice []string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func dimensionInOutcome(dim string, outcome ReviewOutcome) bool {
	for _, issue := range outcome.Issues {
		if issue.Dimension == dim {
			return true
		}
	}
	return false
}

func countDimensionInOutcome(dim string, outcome ReviewOutcome) int {
	count := 0
	for _, issue := range outcome.Issues {
		if issue.Dimension == dim {
			count++
		}
	}
	return count
}
