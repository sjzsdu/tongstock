// Package review 实现定期复盘与研究反馈机制。
// feedback.go 负责将复盘发现转化为新研究假设的反馈闭环。
package review

import (
	"fmt"
	"sort"
	"time"
)

// ============================================================================
// 反馈闭环类型
// ============================================================================

// FeedbackStatus 反馈状态
type FeedbackStatus string

const (
	FeedbackPending     FeedbackStatus = "pending"     // 待处理
	FeedbackInProgress  FeedbackStatus = "in_progress" // 进行中
	FeedbackValidated   FeedbackStatus = "validated"   // 已验证
	FeedbackRejected    FeedbackStatus = "rejected"    // 已拒绝
	FeedbackImplemented FeedbackStatus = "implemented" // 已实施
	FeedbackArchived    FeedbackStatus = "archived"    // 已归档
)

// FeedbackPriority 反馈优先级
type FeedbackPriority string

const (
	FeedbackP0 FeedbackPriority = "P0" // 紧急: 必须立即处理
	FeedbackP1 FeedbackPriority = "P1" // 高: 尽快处理
	FeedbackP2 FeedbackPriority = "P2" // 中: 计划处理
	FeedbackP3 FeedbackPriority = "P3" // 低: 有空处理
)

// FeedbackType 反馈类型
type FeedbackType string

const (
	FeedbackHypothesis     FeedbackType = "hypothesis"      // 新假设
	FeedbackParamUpdate    FeedbackType = "param_update"    // 参数更新
	FeedbackStrategyRev    FeedbackType = "strategy_rev"    // 策略修订
	FeedbackDataFix        FeedbackType = "data_fix"        // 数据修复
	FeedbackProcessImprove FeedbackType = "process_improve" // 流程改进
	FeedbackModelRetrain   FeedbackType = "model_retrain"   // 模型重训练
)

// ResearchFeedback 研究反馈项
type ResearchFeedback struct {
	ID       string           `json:"id"`
	Type     FeedbackType     `json:"type"`
	Status   FeedbackStatus   `json:"status"`
	Priority FeedbackPriority `json:"priority"`

	// 来源
	SourceReviewID  string `json:"source_review_id"`
	SourceFindingID string `json:"source_finding_id,omitempty"`
	SourceFailureID string `json:"source_failure_id,omitempty"`
	SourcePatternID string `json:"source_pattern_id,omitempty"`

	// 内容
	Title          string `json:"title"`
	Description    string `json:"description"`
	ExpectedImpact string `json:"expected_impact"`
	ValidationPlan string `json:"validation_plan"`

	// 关联对象
	TargetParadigmID string `json:"target_paradigm_id,omitempty"`
	TargetVersion    string `json:"target_version,omitempty"`
	NewVersion       string `json:"new_version,omitempty"`

	// 生命周期
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ValidatedAt     *time.Time `json:"validated_at,omitempty"`
	ValidatedResult string     `json:"validated_result,omitempty"`

	// 评估
	EffortEstimate   string  `json:"effort_estimate,omitempty"`   // quick / moderate / extensive
	ImpactScore      float64 `json:"impact_score,omitempty"`      // 0-100
	FeasibilityScore float64 `json:"feasibility_score,omitempty"` // 0-100

	// 日志
	History  []FeedbackHistory `json:"history,omitempty"`
	Author   string            `json:"author"`
	Assignee string            `json:"assignee,omitempty"`
}

// FeedbackHistory 反馈变更历史
type FeedbackHistory struct {
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Note      string    `json:"note,omitempty"`
	OldStatus string    `json:"old_status,omitempty"`
	NewStatus string    `json:"new_status,omitempty"`
}

// FeedbackPortfolio 反馈组合
type FeedbackPortfolio struct {
	ID              string             `json:"id"`
	GeneratedAt     time.Time          `json:"generated_at"`
	TotalCount      int                `json:"total_count"`
	PendingCount    int                `json:"pending_count"`
	HighPriority    int                `json:"high_priority"`
	Items           []ResearchFeedback `json:"items"`
	Recommendations []string           `json:"recommendations"`
}

// ============================================================================
// 反馈生成器
// ============================================================================

// FeedbackGenerator 反馈生成器
type FeedbackGenerator struct {
	idCounter int
}

// NewFeedbackGenerator 创建反馈生成器
func NewFeedbackGenerator() *FeedbackGenerator {
	return &FeedbackGenerator{}
}

// GenerateFromReview 从复盘报告生成反馈
func (fg *FeedbackGenerator) GenerateFromReview(report *ReviewReport) FeedbackPortfolio {
	fg.idCounter++

	var items []ResearchFeedback

	// 1. 从关键发现生成反馈
	for _, finding := range report.Findings {
		if finding.Severity == "critical" {
			feedback := fg.createFromFinding(report, finding)
			items = append(items, feedback)
		}
	}

	// 2. 从失败事件生成反馈
	for _, failure := range report.Failures {
		feedback := fg.createFromFailure(report, failure)
		items = append(items, feedback)
	}

	// 3. 从行动项生成反馈
	for _, action := range report.ActionItems {
		if action.Priority == "critical" {
			feedback := fg.createFromAction(report, action)
			items = append(items, feedback)
		}
	}

	// 4. 去重 (基于目标范式+类型)
	items = fg.deduplicate(items)

	// 5. 排序 (按优先级+影响分)
	sort.Slice(items, func(i, j int) bool {
		pOrder := map[FeedbackPriority]int{FeedbackP0: 0, FeedbackP1: 1, FeedbackP2: 2, FeedbackP3: 3}
		if pOrder[items[i].Priority] != pOrder[items[j].Priority] {
			return pOrder[items[i].Priority] < pOrder[items[j].Priority]
		}
		return items[i].ImpactScore > items[j].ImpactScore
	})

	// 构建反馈组合
	pendingCount := 0
	highPriority := 0
	for _, item := range items {
		if item.Status == FeedbackPending {
			pendingCount++
		}
		if item.Priority == FeedbackP0 || item.Priority == FeedbackP1 {
			highPriority++
		}
	}

	portfolio := FeedbackPortfolio{
		ID:              fmt.Sprintf("feedback-portfolio-%d", fg.idCounter),
		GeneratedAt:     time.Now(),
		TotalCount:      len(items),
		PendingCount:    pendingCount,
		HighPriority:    highPriority,
		Items:           items,
		Recommendations: fg.buildPortfolioRecommendations(report, items),
	}

	return portfolio
}

// createFromFinding 从发现生成反馈
func (fg *FeedbackGenerator) createFromFinding(report *ReviewReport, finding ReviewFinding) ResearchFeedback {
	fg.idCounter++

	priority := fg.estimatePriority(finding.Severity, finding.Category)
	feedbackType := fg.deriveFeedbackType(finding.Category, finding.Metric)

	feedback := ResearchFeedback{
		ID:               fmt.Sprintf("feedback-finding-%d", fg.idCounter),
		Type:             feedbackType,
		Status:           FeedbackPending,
		Priority:         priority,
		SourceReviewID:   report.ID,
		SourceFindingID:  finding.ID,
		Title:            fmt.Sprintf("[%s] %s", finding.Category, finding.Title),
		Description:      finding.Description,
		ExpectedImpact:   fg.estimateImpact(finding),
		ValidationPlan:   fg.suggestValidation(feedbackType),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		EffortEstimate:   fg.estimateEffort(feedbackType),
		ImpactScore:      fg.computeImpactScore(finding),
		Author:           report.Author,
		TargetParadigmID: report.SourceID,
		TargetVersion:    report.SourceID,
	}

	return feedback
}

// createFromFailure 从失败事件生成反馈
func (fg *FeedbackGenerator) createFromFailure(report *ReviewReport, failure FailureEvent) ResearchFeedback {
	fg.idCounter++

	priority := fg.failureToPriority(failure)
	feedbackType := fg.failureToFeedbackType(failure)

	description := failure.Description
	if failure.RootCause != "" {
		description = fmt.Sprintf("%s\n\n根因: %s", description, failure.RootCause)
	}

	feedback := ResearchFeedback{
		ID:               fmt.Sprintf("feedback-failure-%d", fg.idCounter),
		Type:             feedbackType,
		Status:           FeedbackPending,
		Priority:         priority,
		SourceReviewID:   report.ID,
		SourceFailureID:  failure.ID,
		Title:            fmt.Sprintf("[失败分析] %s", failure.Title),
		Description:      description,
		ExpectedImpact:   fmt.Sprintf("修复 %s 类别失败, 减少同类问题", failure.Category),
		ValidationPlan:   fg.suggestValidation(feedbackType),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		EffortEstimate:   fg.estimateEffort(feedbackType),
		ImpactScore:      fg.computeFailureImpact(failure),
		Author:           report.Author,
		TargetParadigmID: failure.ParadigmID,
	}

	return feedback
}

// createFromAction 从行动项生成反馈
func (fg *FeedbackGenerator) createFromAction(report *ReviewReport, action ActionItem) ResearchFeedback {
	fg.idCounter++

	feedbackType := FeedbackProcessImprove
	if contains(action.Title, "model") {
		feedbackType = FeedbackModelRetrain
	} else if contains(action.Title, "data") {
		feedbackType = FeedbackDataFix
	}

	feedback := ResearchFeedback{
		ID:             fmt.Sprintf("feedback-action-%d", fg.idCounter),
		Type:           feedbackType,
		Status:         FeedbackPending,
		Priority:       FeedbackP1,
		SourceReviewID: report.ID,
		Title:          action.Title,
		Description:    action.Description,
		ExpectedImpact: "解决当前问题, 提升系统稳定性",
		ValidationPlan: fg.suggestValidation(feedbackType),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		EffortEstimate: fg.estimateEffort(feedbackType),
		ImpactScore:    70,
		Author:         report.Author,
	}

	return feedback
}

// deduplicate 去重
func (fg *FeedbackGenerator) deduplicate(items []ResearchFeedback) []ResearchFeedback {
	seen := make(map[string]bool)
	var result []ResearchFeedback

	for _, item := range items {
		key := fmt.Sprintf("%s-%s-%s", item.Type, item.TargetParadigmID, item.Title)
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}

	return result
}

// buildPortfolioRecommendations 构建反馈组合建议
func (fg *FeedbackGenerator) buildPortfolioRecommendations(report *ReviewReport, items []ResearchFeedback) []string {
	var recs []string

	if len(items) == 0 {
		recs = append(recs, "本周期无关键反馈, 保持现有策略")
		return recs
	}

	// 按类型统计
	typeCounts := make(map[FeedbackType]int)
	for _, item := range items {
		typeCounts[item.Type]++
	}

	// 最优先处理的类型
	var maxType FeedbackType
	maxCount := 0
	for t, c := range typeCounts {
		if c > maxCount {
			maxCount = c
			maxType = t
		}
	}

	switch maxType {
	case FeedbackHypothesis:
		recs = append(recs, "多个关键发现指向新假设生成, 建议组织专项头脑风暴")
	case FeedbackModelRetrain:
		recs = append(recs, "模型退化问题集中, 建议安排重训练计划")
	case FeedbackParamUpdate:
		recs = append(recs, "参数需要系统性更新, 建议启动参数扫描和验证")
	case FeedbackStrategyRev:
		recs = append(recs, "多个策略需要修订, 建议重新评估各策略的市场适配性")
	case FeedbackDataFix:
		recs = append(recs, "数据质量问题突出, 建议优先修复数据管道")
	}

	// 高优先级建议
	p0Count := 0
	for _, item := range items {
		if item.Priority == FeedbackP0 {
			p0Count++
		}
	}
	if p0Count > 0 {
		recs = append(recs, fmt.Sprintf("有 %d 项 P0 级反馈, 建议列入紧急议程", p0Count))
	}

	// 版本管理建议
	if report.Stats.ParamChanges > 0 {
		recs = append(recs, "本周期有参数修改, 任何修改必须产生新版本并记录验证结果")
	}

	return recs
}

// ============================================================================
// 反馈辅助方法
// ============================================================================

func (fg *FeedbackGenerator) estimatePriority(severity, category string) FeedbackPriority {
	if severity == "critical" {
		return FeedbackP0
	}
	if category == "data" || category == "model" {
		return FeedbackP1
	}
	return FeedbackP2
}

func (fg *FeedbackGenerator) deriveFeedbackType(category, metric string) FeedbackType {
	switch {
	case category == "performance" && (metric == "win_rate" || metric == "sharpe_ratio"):
		return FeedbackHypothesis
	case category == "performance" && metric == "max_drawdown":
		return FeedbackStrategyRev
	case category == "execution":
		return FeedbackProcessImprove
	case category == "data":
		return FeedbackDataFix
	case category == "model":
		return FeedbackModelRetrain
	default:
		return FeedbackHypothesis
	}
}

func (fg *FeedbackGenerator) estimateImpact(finding ReviewFinding) string {
	switch finding.Severity {
	case "critical":
		return fmt.Sprintf("影响重大: %s 指标偏离 %.2f 标准差, 需要立即处理", finding.Metric, finding.Value)
	case "warning":
		return fmt.Sprintf("中等影响: %s 指标接近阈值, 需要关注", finding.Metric)
	default:
		return fmt.Sprintf("轻微影响: %s 指标在正常范围, 持续监控", finding.Metric)
	}
}

func (fg *FeedbackGenerator) suggestValidation(feedbackType FeedbackType) string {
	switch feedbackType {
	case FeedbackHypothesis:
		return "在历史数据上回测验证新假设, 对比基准策略表现"
	case FeedbackModelRetrain:
		return "使用最新数据重训练模型, 在样本外时间窗口验证"
	case FeedbackParamUpdate:
		return "参数扫描 + 滚动窗口验证, 选择鲁棒性最好的参数组合"
	case FeedbackStrategyRev:
		return "修订策略后, 进行 A/B 测试对比新旧策略"
	case FeedbackDataFix:
		return "修复后验证数据完整性, 对比修复前后的模型表现差异"
	case FeedbackProcessImprove:
		return "流程变更后, 监控关键指标的变化趋势"
	default:
		return "制定具体验证计划, 记录验证结果"
	}
}

func (fg *FeedbackGenerator) estimateEffort(feedbackType FeedbackType) string {
	switch feedbackType {
	case FeedbackDataFix:
		return "quick"
	case FeedbackProcessImprove:
		return "quick"
	case FeedbackParamUpdate:
		return "moderate"
	case FeedbackStrategyRev:
		return "moderate"
	case FeedbackHypothesis:
		return "moderate"
	case FeedbackModelRetrain:
		return "extensive"
	default:
		return "moderate"
	}
}

func (fg *FeedbackGenerator) computeImpactScore(finding ReviewFinding) float64 {
	score := 0.0

	switch finding.Severity {
	case "critical":
		score += 50
	case "warning":
		score += 30
	case "info":
		score += 10
	}

	// 指标偏离程度
	if finding.Threshold != 0 {
		deviation := finding.Value / finding.Threshold
		if deviation > 2 {
			score += 30
		} else if deviation > 1.5 {
			score += 20
		} else if deviation > 1 {
			score += 10
		}
	}

	// 关键类别加成
	categoryBoost := map[string]float64{
		"performance": 10,
		"model":       15,
		"data":        10,
		"execution":   5,
		"market":      8,
		"risk":        12,
	}
	score += categoryBoost[finding.Category]

	return min(score, 100)
}

func (fg *FeedbackGenerator) computeFailureImpact(failure FailureEvent) float64 {
	score := 0.0

	severityScore := map[FailureSeverity]float64{
		SeverityCatastrophic: 80,
		SeverityCritical:     60,
		SeverityWarning:      30,
		SeverityInfo:         10,
	}
	score += severityScore[failure.Severity]

	// 偏差程度
	if failure.Deviation > 0.5 {
		score += 20
	} else if failure.Deviation > 0.3 {
		score += 10
	}

	return min(score, 100)
}

func (fg *FeedbackGenerator) failureToPriority(failure FailureEvent) FeedbackPriority {
	switch failure.Severity {
	case SeverityCatastrophic:
		return FeedbackP0
	case SeverityCritical:
		return FeedbackP0
	case SeverityWarning:
		return FeedbackP1
	default:
		return FeedbackP2
	}
}

func (fg *FeedbackGenerator) failureToFeedbackType(failure FailureEvent) FeedbackType {
	switch failure.Category {
	case FailureModelDegradation, FailureParameterDrift, FailureOverfitting:
		return FeedbackModelRetrain
	case FailureMarketRegime:
		return FeedbackStrategyRev
	case FailureDataQuality:
		return FeedbackDataFix
	case FailureExecution, FailureLiquidity:
		return FeedbackProcessImprove
	case FailureRiskManagement:
		return FeedbackStrategyRev
	case FailureUserDecision:
		return FeedbackHypothesis
	default:
		return FeedbackProcessImprove
	}
}

// ============================================================================
// 反馈操作
// ============================================================================

// UpdateFeedback 更新反馈状态
func (fg *FeedbackGenerator) UpdateFeedback(feedback *ResearchFeedback, newStatus FeedbackStatus, note, actor string) {
	oldStatus := feedback.Status
	feedback.Status = newStatus
	feedback.UpdatedAt = time.Now()

	feedback.History = append(feedback.History, FeedbackHistory{
		Timestamp: time.Now(),
		Actor:     actor,
		Action:    "status_change",
		Note:      note,
		OldStatus: string(oldStatus),
		NewStatus: string(newStatus),
	})

	if newStatus == FeedbackValidated {
		now := time.Now()
		feedback.ValidatedAt = &now
	}
}

// RejectFeedback 拒绝反馈
func (fg *FeedbackGenerator) RejectFeedback(feedback *ResearchFeedback, reason, actor string) {
	fg.UpdateFeedback(feedback, FeedbackRejected, reason, actor)
}

// ValidateFeedback 验证反馈
func (fg *FeedbackGenerator) ValidateFeedback(feedback *ResearchFeedback, result, actor string) {
	fg.UpdateFeedback(feedback, FeedbackValidated, result, actor)
	feedback.ValidatedResult = result
}

// ImplementFeedback 实施反馈 (创建新版本)
func (fg *FeedbackGenerator) ImplementFeedback(feedback *ResearchFeedback, newVersion, actor string) {
	fg.UpdateFeedback(feedback, FeedbackImplemented,
		fmt.Sprintf("已创建新版本: %s", newVersion), actor)
	feedback.NewVersion = newVersion
}

// ============================================================================
// 辅助函数
// ============================================================================

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
