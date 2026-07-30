// Package ai_critic implements the AI Research Critic — an independent
// review process that checks candidates for data leakage, selection bias,
// insufficient samples, cost sensitivity, concentration, narrative bias,
// and baseline comparison before promotion.
//
// Design goals:
//   - Independent review: AI critic cannot waive hard thresholds
//   - Structured issues: review findings map to experiment metrics
//   - Human-reviewable: conclusions can be manually reviewed and recorded
//   - Seven checks: data leakage, selection bias, sample sufficiency,
//     cost sensitivity, concentration, narrative bias, baseline compare
package ai_critic

import (
	"fmt"
	"time"
)

// ============================================================================
// 审查维度 (7 种)
// ============================================================================

// ReviewDimension 审查维度
type ReviewDimension string

const (
	DimDataLeakage    ReviewDimension = "data_leakage"    // 数据泄漏
	DimSelectionBias  ReviewDimension = "selection_bias"  // 选择偏差
	DimSampleSize     ReviewDimension = "sample_size"     // 样本不足
	DimCostSensitivity ReviewDimension = "cost_sensitivity" // 成本敏感
	DimConcentration  ReviewDimension = "concentration"   // 集中度
	DimNarrativeBias  ReviewDimension = "narrative_bias"  // 叙事后置
	DimBaselineCompare ReviewDimension = "baseline_compare" // 与基线比较
)

// Severity 问题严重程度
type Severity string

const (
	SevCritical Severity = "critical" // 硬门槛: 阻止晋级
	SevHigh     Severity = "high"     // 高: 强烈建议拒绝
	SevMedium   Severity = "medium"   // 中: 需要人工复核
	SevLow      Severity = "low"      // 低: 记录即可
	SevInfo     Severity = "info"     // 信息: 仅提示
)

// HardThresholdSeverities 硬门槛严重程度 — 不可绕过
var HardThresholdSeverities = map[Severity]bool{
	SevCritical: true,
}

// ReviewIssue 单个审查问题
type ReviewIssue struct {
	ID              string            `json:"id"`
	Dimension       ReviewDimension   `json:"dimension"`
	Severity        Severity          `json:"severity"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	Evidence        string            `json:"evidence,omitempty"`        // 支持证据
	Recommendation  string            `json:"recommendation"`             // 建议
	MetricName      string            `json:"metric_name,omitempty"`     // 关联的实验指标
	MetricValue     float64           `json:"metric_value,omitempty"`    // 指标值
	MetricThreshold  float64           `json:"metric_threshold,omitempty"` // 指标阈值
	IsHardThreshold bool              `json:"is_hard_threshold"`         // 是否为硬门槛
	CreatedAt       time.Time         `json:"created_at"`
}

// IsHardThreshold 是否硬门槛
func (r *ReviewIssue) IsHardThresholdIssue() bool {
	return HardThresholdSeverities[r.Severity] || r.IsHardThreshold
}

// ReviewConclusion 审查结论
type ReviewConclusion string

const (
	ConclusionPass           ReviewConclusion = "pass"           // 通过 (可晋级)
	ConclusionPassWithNotes  ReviewConclusion = "pass_notes"     // 有条件通过 (附建议)
	ConclusionFail            ReviewConclusion = "fail"           // 未通过 (需修正)
	ConclusionBlock          ReviewConclusion = "block"          // 阻止 (硬门槛未过)
	ConclusionNeedsReview    ReviewConclusion = "needs_review"   // 需要人工复核
)

// ReviewOutcome 审查结果
type ReviewOutcome struct {
	ID           string           `json:"id"`
	TargetID     string           `json:"target_id"`     // 被审查对象 ID
	TargetType   string           `json:"target_type"`   // "candidate", "paradigm", "experiment"
	Conclusion   ReviewConclusion `json:"conclusion"`
	Issues       []ReviewIssue    `json:"issues"`
	HardBlocked  bool             `json:"hard_blocked"`
	ReviewedBy   string           `json:"reviewed_by"`   // 审查者
	ReviewedAt   time.Time        `json:"reviewed_at"`
	// 人工复核
	HumanReview  *HumanReviewRecord `json:"human_review,omitempty"`
	// 摘要
	Summary     string           `json:"summary,omitempty"`
}

// Passed 审查是否通过
func (o *ReviewOutcome) Passed() bool {
	return o.Conclusion == ConclusionPass || o.Conclusion == ConclusionPassWithNotes
}

// HasHardBlock 是否有硬门槛阻止
func (o *ReviewOutcome) HasHardBlock() bool {
	for _, issue := range o.Issues {
		if issue.IsHardThresholdIssue() {
			return true
		}
	}
	return false
}

// GetHardBlockingIssues 获取硬门槛问题
func (o *ReviewOutcome) GetHardBlockingIssues() []ReviewIssue {
	var blocked []ReviewIssue
	for _, issue := range o.Issues {
		if issue.IsHardThresholdIssue() {
			blocked = append(blocked, issue)
		}
	}
	return blocked
}

// GetCriticalIssues 获取所有严重问题
func (o *ReviewOutcome) GetCriticalIssues() []ReviewIssue {
	var critical []ReviewIssue
	for _, issue := range o.Issues {
		if issue.Severity == SevCritical || issue.Severity == SevHigh {
			critical = append(critical, issue)
		}
	}
	return critical
}

// HumanReviewRecord 人工复核记录
type HumanReviewRecord struct {
	ReviewerID   string   `json:"reviewer_id"`
	Decision     string   `json:"decision"`   // "approved", "rejected", "waived", "deferred"
	Notes        string   `json:"notes"`
	WaivedIssues []string `json:"waived_issues,omitempty"` // 被豁免的问题 ID
	ReviewedAt   time.Time `json:"reviewed_at"`
}

// IsValid 人工复核记录是否有效
func (h *HumanReviewRecord) IsValid() bool {
	validDecisions := map[string]bool{"approved": true, "rejected": true, "waived": true, "deferred": true}
	return h.ReviewerID != "" && validDecisions[h.Decision]
}

// ============================================================================
// 审查输入: 被审查对象的元数据
// ============================================================================

// ReviewInput 审查输入
type ReviewInput struct {
	TargetID   string         `json:"target_id"`
	TargetType string         `json:"target_type"`
	// 实验指标
	Metrics    map[string]float64 `json:"metrics"`
	// 实验配置
	Config     ReviewConfig   `json:"config"`
	// 实验结果
	Results    ReviewResults  `json:"results"`
}

// ReviewConfig 审查用实验配置
type ReviewConfig struct {
	SplitType      string  `json:"split_type"`       // "fixed", "rolling", "expanding"
	TrainRatio     float64 `json:"train_ratio"`      // 训练集比例
	ValidRatio     float64 `json:"valid_ratio"`      // 验证集比例
	EmbargoDays    int     `json:"embargo_days"`     // 隔离期 (天)
	PurgeDays      int     `json:"purge_days"`       // 清洗期 (天)
	FeatureCount   int     `json:"feature_count"`    // 使用特征数
	FeatureIDs     []string `json:"feature_ids"`      // 特征 ID 列表
	DataSnapshotID string  `json:"data_snapshot_id"` // 数据快照 ID
}

// ReviewResults 审查用实验结果
type ReviewResults struct {
	SampleSize        int     `json:"sample_size"`
	SharpeRatio       float64 `json:"sharpe_ratio"`
	SortinoRatio      float64 `json:"sortino_ratio"`
	MaxDrawdown       float64 `json:"max_drawdown"`
	TotalReturn       float64 `json:"total_return"`
	WinRate           float64 `json:"win_rate"`
	TotalTrades       int     `json:"total_trades"`
	ProfitFactor      float64 `json:"profit_factor"`
	GrossReturn       float64 `json:"gross_return"`
	NetReturn         float64 `json:"net_return"`
	CostRatio         float64 `json:"cost_ratio"`         // 成本占比
	MaxPositionWeight float64 `json:"max_position_weight"` // 最大单票仓位
	Concentration     float64 `json:"concentration"`      // 集中度指数
	BaselineReturn    float64 `json:"baseline_return"`    // 基线收益 (如沪深 300)
	BaselineSharpe    float64 `json:"baseline_sharpe"`    // 基线夏普
}

// ============================================================================
// 工厂方法
// ============================================================================

// NewReviewIssue 创建审查问题
func NewReviewIssue(id string, dim ReviewDimension, sev Severity, title, description, recommendation string) *ReviewIssue {
	return &ReviewIssue{
		ID:              id,
		Dimension:       dim,
		Severity:        sev,
		Title:           title,
		Description:     description,
		Recommendation:  recommendation,
		IsHardThreshold: sev == SevCritical,
		CreatedAt:       time.Now(),
	}
}

// SetMetric 关联实验指标
func (r *ReviewIssue) SetMetric(name string, value, threshold float64) *ReviewIssue {
	r.MetricName = name
	r.MetricValue = value
	r.MetricThreshold = threshold
	return r
}

// NewReviewOutcome 创建审查结果
func NewReviewOutcome(targetID, targetType, reviewer string) *ReviewOutcome {
	return &ReviewOutcome{
		ID:         fmt.Sprintf("review-%s-%d", targetID, time.Now().UnixNano()),
		TargetID:   targetID,
		TargetType: targetType,
		Conclusion: ConclusionNeedsReview,
		Issues:     make([]ReviewIssue, 0),
		ReviewedBy: reviewer,
		ReviewedAt: time.Now(),
	}
}

// AddIssue 添加审查问题
func (o *ReviewOutcome) AddIssue(issue ReviewIssue) *ReviewOutcome {
	o.Issues = append(o.Issues, issue)
	return o
}

// AddIssueQuick 快速添加
func (o *ReviewOutcome) AddIssueQuick(dim ReviewDimension, sev Severity, title, desc, rec string) *ReviewOutcome {
	issue := NewReviewIssue(
		fmt.Sprintf("issue-%s-%d", dim, len(o.Issues)),
		dim, sev, title, desc, rec,
	)
	o.Issues = append(o.Issues, *issue)
	return o
}

// Finalize 完成审查, 计算结论
func (o *ReviewOutcome) Finalize() *ReviewOutcome {
	o.Conclusion = o.computeConclusion()
	o.HardBlocked = o.HasHardBlock()
	o.Summary = o.generateSummary()
	o.ReviewedAt = time.Now()
	return o
}

// computeConclusion 计算审查结论
func (o *ReviewOutcome) computeConclusion() ReviewConclusion {
	// 硬门槛: 任何 critical 问题 → block
	for _, issue := range o.Issues {
		if issue.IsHardThresholdIssue() {
			return ConclusionBlock
		}
	}

	// 有 high 级: fail
	highCount := 0
	mediumCount := 0
	for _, issue := range o.Issues {
		switch issue.Severity {
		case SevHigh:
			highCount++
		case SevMedium:
			mediumCount++
		}
	}

	if highCount > 0 {
		return ConclusionFail
	}
	if mediumCount > 0 {
		return ConclusionNeedsReview
	}

	// 全部通过 (包括 low/info)
	if len(o.Issues) > 0 {
		return ConclusionPassWithNotes
	}
	return ConclusionPass
}

// generateSummary 生成摘要
func (o *ReviewOutcome) generateSummary() string {
	counts := make(map[Severity]int)
	for _, issue := range o.Issues {
		counts[issue.Severity]++
	}

	summary := fmt.Sprintf("审查结论: %s | 问题总数: %d", o.Conclusion, len(o.Issues))
	if counts[SevCritical] > 0 {
		summary += fmt.Sprintf(" | 严重: %d", counts[SevCritical])
	}
	if counts[SevHigh] > 0 {
		summary += fmt.Sprintf(" | 高: %d", counts[SevHigh])
	}
	if counts[SevMedium] > 0 {
		summary += fmt.Sprintf(" | 中: %d", counts[SevMedium])
	}

	return summary
}

// ============================================================================
// 人工复核方法
// ============================================================================

// WaiveIssue 豁免某个问题
func (o *ReviewOutcome) WaiveIssue(reviewerID string, issueID string, reason string) *ReviewOutcome {
	if o.HumanReview == nil {
		o.HumanReview = &HumanReviewRecord{
			ReviewerID:   reviewerID,
			Decision:     "waived",
			Notes:        "",
			WaivedIssues: make([]string, 0),
			ReviewedAt:   time.Now(),
		}
	}
	o.HumanReview.WaivedIssues = append(o.HumanReview.WaivedIssues, issueID)
	o.HumanReview.Notes += fmt.Sprintf("[豁免 %s] %s; ", issueID, reason)
	return o
}

// Approve 人工批准
func (o *ReviewOutcome) Approve(reviewerID, notes string) *ReviewOutcome {
	o.HumanReview = &HumanReviewRecord{
		ReviewerID: reviewerID,
		Decision:   "approved",
		Notes:      notes,
		ReviewedAt: time.Now(),
	}
	return o
}

// Reject 人工拒绝
func (o *ReviewOutcome) Reject(reviewerID, notes string) *ReviewOutcome {
	o.HumanReview = &HumanReviewRecord{
		ReviewerID: reviewerID,
		Decision:   "rejected",
		Notes:      notes,
		ReviewedAt: time.Now(),
	}
	return o
}

// Defer 延后处理
func (o *ReviewOutcome) Defer(reviewerID, notes string) *ReviewOutcome {
	o.HumanReview = &HumanReviewRecord{
		ReviewerID: reviewerID,
		Decision:   "deferred",
		Notes:      notes,
		ReviewedAt: time.Now(),
	}
	return o
}
