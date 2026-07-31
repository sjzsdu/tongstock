// Package review 实现定期复盘与研究反馈机制。
//
// 核心能力:
//   - 结构化复盘报告生成 (review.go)
//   - 失败原因分类与根因分析 (failure_analysis.go)
//   - 研究反馈闭环: 新假设回流而非静默调参 (feedback.go)
package review

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// ============================================================================
// 复盘类型定义
// ============================================================================

// ReviewPeriod 复盘周期
type ReviewPeriod string

const (
	ReviewWeekly    ReviewPeriod = "weekly"
	ReviewMonthly   ReviewPeriod = "monthly"
	ReviewQuarterly ReviewPeriod = "quarterly"
)

// ReviewStatus 复盘状态
type ReviewStatus string

const (
	ReviewDraft     ReviewStatus = "draft"
	ReviewComplete  ReviewStatus = "completed"
	ReviewPublished ReviewStatus = "published"
)

// ReviewType 复盘类型
type ReviewType string

const (
	ReviewPostMortem      ReviewType = "post_mortem"      // 事后复盘
	ReviewRetrospective   ReviewType = "retrospective"    // 定期回顾
	ReviewParamAudit      ReviewType = "param_audit"      // 参数审计
	ReviewFailureAnalysis ReviewType = "failure_analysis" // 失败分析
)

// ReviewPriority 复盘优先级
type ReviewPriority string

const (
	PriorityHigh   ReviewPriority = "high"
	PriorityMedium ReviewPriority = "medium"
	PriorityLow    ReviewPriority = "low"
)

// ============================================================================
// 核心数据结构
// ============================================================================

// ReviewReport 结构化复盘报告
type ReviewReport struct {
	ID         string         `json:"id"`
	Type       ReviewType     `json:"type"`
	Period     ReviewPeriod   `json:"period"`
	Status     ReviewStatus   `json:"status"`
	Priority   ReviewPriority `json:"priority"`
	SourceID   string         `json:"source_id"`   // paradigm_version_id 或 run_id
	SourceType string         `json:"source_type"` // paradigm / run / system

	// 时间范围
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	GeneratedAt time.Time `json:"generated_at"`

	// 核心发现
	Findings  []ReviewFinding  `json:"findings"`
	Failures  []FailureEvent   `json:"failures"`
	Decisions []ReviewDecision `json:"decisions"`

	// 统计摘要
	Stats ReviewStats `json:"stats"`

	// 结构化输出
	ExecutiveSummary string       `json:"executive_summary"`
	ActionItems      []ActionItem `json:"action_items"`
	OpenQuestions    []string     `json:"open_questions"`
	Recommendations  []string     `json:"recommendations"`
	LessonsLearned   []string     `json:"lessons_learned"`

	// 反馈
	FeedbackGenerated bool   `json:"feedback_generated"`
	FeedbackID        string `json:"feedback_id,omitempty"`

	// 操作人
	Author     string `json:"author"`
	ReviewedBy string `json:"reviewed_by,omitempty"`
	ApprovedBy string `json:"approved_by,omitempty"`
}

// ReviewFinding 复盘发现
type ReviewFinding struct {
	ID          string    `json:"id"`
	Category    string    `json:"category"` // performance / execution / market / data / model
	Severity    string    `json:"severity"` // info / warning / critical
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Evidence    []string  `json:"evidence"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Timestamp   time.Time `json:"timestamp"`
}

// ReviewStats 复盘统计
type ReviewStats struct {
	TotalSignals      int     `json:"total_signals"`
	ExecutedSignals   int     `json:"executed_signals"`
	FailedSignals     int     `json:"failed_signals"`
	UnexecutedSignals int     `json:"unexecuted_signals"`
	ExecutionRate     float64 `json:"execution_rate"`
	WinRate           float64 `json:"win_rate"`
	AvgReturn         float64 `json:"avg_return"`
	TotalPnL          float64 `json:"total_pnl"`
	MaxDrawdown       float64 `json:"max_drawdown"`
	SharpeRatio       float64 `json:"sharpe_ratio"`
	StatusChanges     int     `json:"status_changes"`
	ParamChanges      int     `json:"param_changes"`
	DataQualityScore  float64 `json:"data_quality_score"`
}

// ReviewDecision 复盘决策记录
type ReviewDecision struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // promote / degrade / suspend / reject / keep / revise
	TargetID   string    `json:"target_id"`
	Reason     string    `json:"reason"`
	Rationale  string    `json:"rationale"`
	ApprovedBy string    `json:"approved_by"`
	NewVersion string    `json:"new_version,omitempty"`
	OldVersion string    `json:"old_version,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ActionItem 行动项
type ActionItem struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Priority         string    `json:"priority"`
	Assignee         string    `json:"assignee"`
	DueDate          time.Time `json:"due_date"`
	Status           string    `json:"status"` // todo / in_progress / done
	RelatedFindingID string    `json:"related_finding_id"`
}

// ============================================================================
// 复盘生成器
// ============================================================================

// ReviewInput 复盘输入
type ReviewInput struct {
	SourceID    string       `json:"source_id"`
	SourceType  string       `json:"source_type"`
	Type        ReviewType   `json:"type"`
	Period      ReviewPeriod `json:"period"`
	PeriodStart time.Time    `json:"period_start"`
	PeriodEnd   time.Time    `json:"period_end"`
	Author      string       `json:"author"`

	// 原始数据
	SignalCount      int       `json:"signal_count"`
	ExecutedCount    int       `json:"executed_count"`
	FailedCount      int       `json:"failed_count"`
	UnexecutedCount  int       `json:"unexecuted_count"`
	Returns          []float64 `json:"returns"`
	PnL              float64   `json:"pnl"`
	StatusChanges    int       `json:"status_changes"`
	ParamChanges     int       `json:"param_changes"`
	DataQualityScore float64   `json:"data_quality_score"`

	// 上下文
	Failures      []FailureEvent   `json:"failures,omitempty"`
	Decisions     []ReviewDecision `json:"decisions,omitempty"`
	ExtraFindings []ReviewFinding  `json:"extra_findings,omitempty"`
}

// ReviewGenerator 复盘生成器
type ReviewGenerator struct {
	IDCounter int
}

// NewReviewGenerator 创建复盘生成器
func NewReviewGenerator() *ReviewGenerator {
	return &ReviewGenerator{IDCounter: 0}
}

// GenerateReview 生成结构化复盘报告
func (g *ReviewGenerator) GenerateReview(input ReviewInput) *ReviewReport {
	g.IDCounter++
	report := &ReviewReport{
		ID:         fmt.Sprintf("review-%d", g.IDCounter),
		Type:       input.Type,
		Period:     input.Period,
		Status:     ReviewDraft,
		Priority:   g.determinePriority(input),
		SourceID:   input.SourceID,
		SourceType: input.SourceType,

		PeriodStart: input.PeriodStart,
		PeriodEnd:   input.PeriodEnd,
		GeneratedAt: time.Now(),

		Failures:  input.Failures,
		Decisions: input.Decisions,
		Author:    input.Author,
	}

	// 1. 生成统计
	report.Stats = g.computeStats(input)

	// 2. 生成发现
	report.Findings = g.generateFindings(input, report.Stats)

	// 3. 生成执行摘要
	report.ExecutiveSummary = g.generateExecutiveSummary(report)

	// 4. 生成行动项
	report.ActionItems = g.generateActionItems(report.Findings)

	// 5. 生成开放问题
	report.OpenQuestions = g.generateOpenQuestions(report)

	// 6. 生成建议
	report.Recommendations = g.generateRecommendations(report)

	// 7. 生成经验教训
	report.LessonsLearned = g.extractLessons(report)

	return report
}

// determinePriority 根据数据确定复盘优先级
func (g *ReviewGenerator) determinePriority(input ReviewInput) ReviewPriority {
	// 严重失败 -> 高优先级
	if len(input.Failures) > 0 {
		hasCritical := false
		for _, f := range input.Failures {
			if f.Severity == "critical" {
				hasCritical = true
				break
			}
		}
		if hasCritical {
			return PriorityHigh
		}
		if len(input.Failures) >= 3 {
			return PriorityHigh
		}
		return PriorityMedium
	}

	// 低胜率 -> 中优先级
	if len(input.Returns) > 0 {
		wr := winRate(input.Returns)
		if wr < 0.3 {
			return PriorityHigh
		}
		if wr < 0.45 {
			return PriorityMedium
		}
	}

	// 高执行失败率 -> 中优先级
	if input.ExecutedCount > 0 {
		failRate := float64(input.FailedCount) / float64(input.ExecutedCount)
		if failRate > 0.2 {
			return PriorityMedium
		}
	}

	return PriorityLow
}

// computeStats 计算统计数据
func (g *ReviewGenerator) computeStats(input ReviewInput) ReviewStats {
	stats := ReviewStats{
		TotalSignals:      input.SignalCount,
		ExecutedSignals:   input.ExecutedCount,
		FailedSignals:     input.FailedCount,
		UnexecutedSignals: input.UnexecutedCount,
		StatusChanges:     input.StatusChanges,
		ParamChanges:      input.ParamChanges,
		DataQualityScore:  input.DataQualityScore,
		TotalPnL:          input.PnL,
	}

	// 执行率
	if input.SignalCount > 0 {
		stats.ExecutionRate = float64(input.ExecutedCount) / float64(input.SignalCount)
	}

	// 胜率与收益
	if len(input.Returns) > 0 {
		stats.WinRate = winRate(input.Returns)
		stats.AvgReturn = mean(input.Returns)
		stats.SharpeRatio = annualizedSharpe(input.Returns)
		stats.MaxDrawdown = maxDrawdown(input.Returns)
	}

	return stats
}

// generateFindings 生成复盘发现
func (g *ReviewGenerator) generateFindings(input ReviewInput, stats ReviewStats) []ReviewFinding {
	var findings []ReviewFinding
	idx := 0

	// 1. 胜率/收益发现
	if stats.WinRate < 0.4 {
		idx++
		findings = append(findings, ReviewFinding{
			ID:          fmt.Sprintf("finding-%d", idx),
			Category:    "performance",
			Severity:    "critical",
			Title:       "胜率显著低于预期",
			Description: fmt.Sprintf("当前胜率 %.1f%%, 低于预期 50%%, 需要检查模型假设是否仍然成立", stats.WinRate*100),
			Evidence:    []string{fmt.Sprintf("Period: %s ~ %s", input.PeriodStart.Format("2006-01-02"), input.PeriodEnd.Format("2006-01-02"))},
			Metric:      "win_rate",
			Value:       stats.WinRate,
			Threshold:   0.5,
			Timestamp:   time.Now(),
		})
	} else if stats.WinRate < 0.45 {
		idx++
		findings = append(findings, ReviewFinding{
			ID:          fmt.Sprintf("finding-%d", idx),
			Category:    "performance",
			Severity:    "warning",
			Title:       "胜率低于正常水平",
			Description: fmt.Sprintf("当前胜率 %.1f%%, 略低于基准, 需要持续监控", stats.WinRate*100),
			Metric:      "win_rate",
			Value:       stats.WinRate,
			Threshold:   0.5,
			Timestamp:   time.Now(),
		})
	}

	// 2. 执行率发现
	if stats.ExecutionRate < 0.8 && input.SignalCount > 0 {
		idx++
		findings = append(findings, ReviewFinding{
			ID:          fmt.Sprintf("finding-%d", idx),
			Category:    "execution",
			Severity:    "warning",
			Title:       "信号执行率偏低",
			Description: fmt.Sprintf("执行率 %.1f%%, 有 %d 条信号未成交", stats.ExecutionRate*100, input.UnexecutedCount),
			Metric:      "execution_rate",
			Value:       stats.ExecutionRate,
			Threshold:   0.8,
			Timestamp:   time.Now(),
		})
	}

	// 3. 高回撤发现
	if stats.MaxDrawdown > 0.1 {
		idx++
		findings = append(findings, ReviewFinding{
			ID:          fmt.Sprintf("finding-%d", idx),
			Category:    "performance",
			Severity:    "critical",
			Title:       "最大回撤超过阈值",
			Description: fmt.Sprintf("最大回撤 %.2f%%, 超过 10%% 阈值, 需要评估风险控制", stats.MaxDrawdown*100),
			Metric:      "max_drawdown",
			Value:       stats.MaxDrawdown,
			Threshold:   0.1,
			Timestamp:   time.Now(),
		})
	}

	// 4. 数据质量发现
	if input.DataQualityScore < 0.8 {
		severity := "warning"
		if input.DataQualityScore < 0.5 {
			severity = "critical"
		}
		idx++
		findings = append(findings, ReviewFinding{
			ID:          fmt.Sprintf("finding-%d", idx),
			Category:    "data",
			Severity:    severity,
			Title:       "数据质量偏低",
			Description: fmt.Sprintf("数据质量分数 %.1f, 部分数据可能缺失或异常", input.DataQualityScore),
			Metric:      "data_quality",
			Value:       input.DataQualityScore,
			Threshold:   0.8,
			Timestamp:   time.Now(),
		})
	}

	// 5. 参数频繁修改
	if input.ParamChanges > 3 {
		idx++
		findings = append(findings, ReviewFinding{
			ID:          fmt.Sprintf("finding-%d", idx),
			Category:    "model",
			Severity:    "warning",
			Title:       "参数在周期内频繁修改",
			Description: fmt.Sprintf("本周期内参数修改 %d 次, 建议评估是否存在过拟合风险", input.ParamChanges),
			Metric:      "param_changes",
			Value:       float64(input.ParamChanges),
			Threshold:   3,
			Timestamp:   time.Now(),
		})
	}

	// 附加发现
	findings = append(findings, input.ExtraFindings...)

	// 按严重度排序
	sort.Slice(findings, func(i, j int) bool {
		severityOrder := map[string]int{"critical": 3, "warning": 2, "info": 1}
		return severityOrder[findings[i].Severity] > severityOrder[findings[j].Severity]
	})

	return findings
}

// generateExecutiveSummary 生成执行摘要
func (g *ReviewGenerator) generateExecutiveSummary(report *ReviewReport) string {
	if len(report.Findings) == 0 && len(report.Failures) == 0 {
		return fmt.Sprintf("本周期 %s 运行正常, 共产生 %d 条信号, 执行率 %.1f%%, 胜率 %.1f%%, 夏普 %.2f",
			report.Period, report.Stats.TotalSignals, report.Stats.ExecutionRate*100,
			report.Stats.WinRate*100, report.Stats.SharpeRatio)
	}

	summary := fmt.Sprintf("【%s复盘】%s %s 发现 %d 项问题, %d 次失败事件。",
		report.Period, report.SourceType, report.SourceID, len(report.Findings), len(report.Failures))

	// 列出关键发现
	for _, f := range report.Findings {
		if f.Severity == "critical" {
			summary += fmt.Sprintf("\n⚠️ %s: %s", f.Title, f.Description)
		}
	}

	summary += fmt.Sprintf("\n📊 统计: 信号 %d, 执行 %d, 胜率 %.1f%%, 夏普 %.2f, 回撤 %.2f%%",
		report.Stats.TotalSignals, report.Stats.ExecutedSignals,
		report.Stats.WinRate*100, report.Stats.SharpeRatio, report.Stats.MaxDrawdown*100)

	return summary
}

// generateActionItems 生成行动项
func (g *ReviewGenerator) generateActionItems(findings []ReviewFinding) []ActionItem {
	var items []ActionItem
	idx := 0

	for _, f := range findings {
		if f.Severity == "critical" || f.Severity == "warning" {
			idx++
			items = append(items, ActionItem{
				ID:               fmt.Sprintf("action-%d", idx),
				Title:            fmt.Sprintf("[%s] %s", f.Category, f.Title),
				Description:      f.Description,
				Priority:         f.Severity,
				Assignee:         "research-team",
				DueDate:          time.Now().AddDate(0, 0, 7),
				Status:           "todo",
				RelatedFindingID: f.ID,
			})
		}
	}

	return items
}

// generateOpenQuestions 生成开放问题
func (g *ReviewGenerator) generateOpenQuestions(report *ReviewReport) []string {
	var questions []string

	if report.Stats.MaxDrawdown > 0.05 {
		questions = append(questions,
			"当前风险控制是否充分? 是否需要调整止损策略?")
	}

	if report.Stats.WinRate < 0.5 {
		questions = append(questions,
			"低胜率是因为市场环境变化还是模型退化? 是否需要重新训练?")
	}

	if report.Stats.ExecutionRate < 0.9 {
		questions = append(questions,
			"信号未执行的主要原因是什么? 是否因为 T+1 限制、涨跌停还是流动性?")
	}

	if len(report.Decisions) > 0 {
		questions = append(questions,
			fmt.Sprintf("本周期的 %d 项决策是否经过充分验证? 决策后果如何评估?", len(report.Decisions)))
	}

	if report.Stats.ParamChanges > 0 {
		questions = append(questions,
			fmt.Sprintf("本周期的 %d 次参数修改是否产生了新版本? 是否记录了变更原因?", report.Stats.ParamChanges))
	}

	return questions
}

// generateRecommendations 生成建议
func (g *ReviewGenerator) generateRecommendations(report *ReviewReport) []string {
	var recs []string

	hasCritical := false
	hasWarning := false
	for _, f := range report.Findings {
		if f.Severity == "critical" {
			hasCritical = true
		}
		if f.Severity == "warning" {
			hasWarning = true
		}
	}

	if hasCritical {
		recs = append(recs,
			"【高优先级】存在严重问题, 建议立即审查相关模型和数据管道")
	}

	if report.Stats.WinRate < 0.4 && report.Stats.ExecutedSignals > 0 {
		recs = append(recs,
			"胜率持续偏低, 建议: 1) 检查市场环境是否切换 2) 评估模型是否过拟合 3) 考虑降低仓位或切换备选策略")
	}

	if report.Stats.ExecutionRate < 0.8 {
		recs = append(recs,
			"执行率偏低, 建议: 1) 优化信号触发时点 2) 考虑流动性更好的标的 3) 放宽执行价格容忍度")
	}

	if report.Stats.MaxDrawdown > 0.1 {
		recs = append(recs,
			"回撤超过阈值, 建议: 1) 检查止损是否及时 2) 评估持仓集中度 3) 考虑波动率调整仓位")
	}

	if report.Stats.DataQualityScore < 0.7 {
		recs = append(recs,
			"数据质量偏低, 建议: 1) 检查数据源完整性 2) 增加数据校验 3) 建立数据质量监控告警")
	}

	if len(recs) == 0 && !hasWarning {
		recs = append(recs, "本周期无明显问题, 继续保持当前策略")
	}

	return recs
}

// extractLessons 提取经验教训
func (g *ReviewGenerator) extractLessons(report *ReviewReport) []string {
	var lessons []string

	// 从成功中学习
	if report.Stats.WinRate > 0.55 && report.Stats.SharpeRatio > 1.5 {
		lessons = append(lessons,
			fmt.Sprintf("高胜率(%.1f%%)和高夏普(%.2f)表明策略在当前市场环境下表现良好, 应记录该环境特征",
				report.Stats.WinRate*100, report.Stats.SharpeRatio))
	}

	// 从失败中学习
	failureCats := make(map[string]int)
	for _, f := range report.Failures {
		failureCats[string(f.Category)]++
	}
	for cat, count := range failureCats {
		lessons = append(lessons,
			fmt.Sprintf("在 %s 类别发生 %d 次失败, 需要针对性改进", cat, count))
	}

	// 从决策中学习
	decisionTypes := make(map[string]int)
	for _, d := range report.Decisions {
		decisionTypes[d.Type]++
	}
	if len(decisionTypes) > 0 {
		for dt, count := range decisionTypes {
			lessons = append(lessons,
				fmt.Sprintf("本周期执行了 %d 次 '%s' 决策, 建议复盘决策依据和结果", count, dt))
		}
	}

	if len(lessons) == 0 {
		lessons = append(lessons, "本周期无明显经验教训, 持续积累数据")
	}

	return lessons
}

// ============================================================================
// 辅助函数
// ============================================================================

func mean(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

func winRate(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	wins := 0
	for _, v := range data {
		if v > 0 {
			wins++
		}
	}
	return float64(wins) / float64(len(data))
}

func annualizedSharpe(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	m := mean(returns)
	variance := 0.0
	for _, r := range returns {
		diff := r - m
		variance += diff * diff
	}
	variance /= float64(len(returns) - 1)
	sd := math.Sqrt(variance)
	if sd == 0 {
		return 0
	}
	return m / sd * math.Sqrt(252)
}

func maxDrawdown(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	cumulative := make([]float64, len(returns))
	cumulative[0] = 1 + returns[0]
	for i := 1; i < len(returns); i++ {
		cumulative[i] = cumulative[i-1] * (1 + returns[i])
	}
	peak := cumulative[0]
	var maxDD float64
	for _, v := range cumulative {
		if v > peak {
			peak = v
		}
		dd := (peak - v) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}
