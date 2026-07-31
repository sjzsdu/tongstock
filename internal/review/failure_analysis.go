// Package review 实现定期复盘与研究反馈机制。
// failure_analysis.go 负责失败事件的分类、根因分析和模式识别。
package review

import (
	"fmt"
	"sort"
	"time"
)

// ============================================================================
// 失败事件分类
// ============================================================================

// FailureCategory 失败类别
type FailureCategory string

const (
	FailureModelDegradation FailureCategory = "model_degradation" // 模型退化
	FailureMarketRegime     FailureCategory = "market_regime"     // 市场切换
	FailureDataQuality      FailureCategory = "data_quality"      // 数据质量
	FailureExecution        FailureCategory = "execution"         // 执行失败
	FailureRiskManagement   FailureCategory = "risk_management"   // 风险管理
	FailureLiquidity        FailureCategory = "liquidity"         // 流动性
	FailureUserDecision     FailureCategory = "user_decision"     // 用户决策
	FailureSystemError      FailureCategory = "system_error"      // 系统错误
	FailureOverfitting      FailureCategory = "overfitting"       // 过拟合
	FailureParameterDrift   FailureCategory = "parameter_drift"   // 参数漂移
)

// FailureSeverity 失败严重度
type FailureSeverity string

const (
	SeverityInfo         FailureSeverity = "info"
	SeverityWarning      FailureSeverity = "warning"
	SeverityCritical     FailureSeverity = "critical"
	SeverityCatastrophic FailureSeverity = "catastrophic"
)

// FailureEvent 失败事件
type FailureEvent struct {
	ID          string          `json:"id"`
	Category    FailureCategory `json:"category"`
	Severity    FailureSeverity `json:"severity"`
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Description string          `json:"description"`

	// 关联对象
	SourceID   string `json:"source_id"`
	SourceType string `json:"source_type"`
	ParadigmID string `json:"paradigm_id,omitempty"`
	SignalID   string `json:"signal_id,omitempty"`
	OrderID    string `json:"order_id,omitempty"`

	// 量化特征
	Metric    string  `json:"metric"`
	Expected  float64 `json:"expected"`
	Actual    float64 `json:"actual"`
	Deviation float64 `json:"deviation"`

	// 时间
	DetectedAt time.Time  `json:"detected_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Duration   int64      `json:"duration_minutes,omitempty"`

	// 根因分析
	RootCause           string   `json:"root_cause,omitempty"`
	ContributingFactors []string `json:"contributing_factors,omitempty"`
	Correction          string   `json:"correction,omitempty"`

	// 状态
	Status          string `json:"status"` // open / investigating / resolved / mitigated
	RelatedReviewID string `json:"related_review_id,omitempty"`
}

// FailureAnalysisResult 失败分析结果
type FailureAnalysisResult struct {
	TotalFailures     int                     `json:"total_failures"`
	OpenFailures      int                     `json:"open_failures"`
	ByCategory        map[FailureCategory]int `json:"by_category"`
	BySeverity        map[FailureSeverity]int `json:"by_severity"`
	Patterns          []FailurePattern        `json:"patterns"`
	TopCauses         []RootCauseItem         `json:"top_causes"`
	FailureRate       float64                 `json:"failure_rate"`
	MeanTimeToDetect  time.Duration           `json:"mean_time_to_detect"`
	MeanTimeToResolve time.Duration           `json:"mean_time_to_resolve"`
	RecurringFailures int                     `json:"recurring_failures"`
}

// FailurePattern 失败模式
type FailurePattern struct {
	ID              string    `json:"id"`
	PatternType     string    `json:"pattern_type"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	OccurrenceCount int       `json:"occurrence_count"`
	AffectedAreas   []string  `json:"affected_areas"`
	DetectedPattern []float64 `json:"detected_pattern"`
	Severity        string    `json:"severity"`
	Recommendation  string    `json:"recommendation"`
}

// RootCauseItem 根因项目
type RootCauseItem struct {
	Factor     string    `json:"factor"`
	Count      int       `json:"count"`
	Percentage float64   `json:"percentage"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
}

// ============================================================================
// 失败分析器
// ============================================================================

// FailureAnalyzer 失败分析器
type FailureAnalyzer struct {
	idCounter int
}

// NewFailureAnalyzer 创建失败分析器
func NewFailureAnalyzer() *FailureAnalyzer {
	return &FailureAnalyzer{}
}

// ClassifyFailure 分类失败事件
func (fa *FailureAnalyzer) ClassifyFailure(event FailureEvent) FailureCategory {
	// 基于特征自动分类
	switch {
	case event.Deviation > 0.5 && event.Category == "":
		return FailureModelDegradation
	case event.Category == "":
		switch event.Metric {
		case "signal_deviation", "prediction_error":
			return FailureModelDegradation
		case "market_state_change", "volatility_spike":
			return FailureMarketRegime
		case "data_missing", "data_corruption", "data_delay":
			return FailureDataQuality
		case "execution_price", "execution_volume", "order_rejected":
			return FailureExecution
		case "max_drawdown", "position_concentration":
			return FailureRiskManagement
		case "spread", "volume_shrink":
			return FailureLiquidity
		case "user_override", "manual_intervention":
			return FailureUserDecision
		case "api_error", "calculation_error", "timeout":
			return FailureSystemError
		case "in_sample_high", "out_of_sample_low":
			return FailureOverfitting
		case "param_shift", "model_drift":
			return FailureParameterDrift
		}
	}

	if event.Category != "" {
		return event.Category
	}
	return FailureSystemError
}

// AnalyzeFailures 分析一批失败事件
func (fa *FailureAnalyzer) AnalyzeFailures(events []FailureEvent) FailureAnalysisResult {
	result := FailureAnalysisResult{
		TotalFailures: len(events),
		ByCategory:    make(map[FailureCategory]int),
		BySeverity:    make(map[FailureSeverity]int),
		TopCauses:     []RootCauseItem{},
	}

	if len(events) == 0 {
		return result
	}

	// 统计分类和严重度
	categoryCounts := make(map[FailureCategory]int)
	severityCounts := make(map[FailureSeverity]int)
	causeCounts := make(map[string]*RootCauseItem)

	var totalDetectTime, totalResolveTime time.Duration
	openCount := 0
	recurringCount := 0

	for _, e := range events {
		// 自动分类
		if e.Category == "" {
			e.Category = fa.ClassifyFailure(e)
		}

		categoryCounts[e.Category]++
		severityCounts[e.Severity]++

		if e.Status == "open" || e.Status == "investigating" {
			openCount++
		}

		// 根因统计
		if e.RootCause != "" {
			item, exists := causeCounts[e.RootCause]
			if !exists {
				item = &RootCauseItem{
					Factor:    e.RootCause,
					FirstSeen: e.DetectedAt,
				}
				causeCounts[e.RootCause] = item
			}
			item.Count++
			item.LastSeen = e.DetectedAt
		}

		// 重复发生检测
		for _, f := range events {
			if f.ID != e.ID && f.RootCause == e.RootCause &&
				(f.DetectedAt.Sub(e.DetectedAt).Hours() < 24 &&
					e.DetectedAt.Sub(f.DetectedAt).Hours() < 24) {
				recurringCount++
				break
			}
		}
	}

	// 时间统计
	var detectCount, resolveCount int
	for _, e := range events {
		if e.Duration > 0 {
			totalDetectTime += time.Duration(e.Duration) * time.Minute
			detectCount++
		}
		if e.ResolvedAt != nil && !e.ResolvedAt.IsZero() {
			totalResolveTime += e.ResolvedAt.Sub(e.DetectedAt)
			resolveCount++
		}
	}

	// 填充结果
	result.OpenFailures = openCount
	result.ByCategory = categoryCounts
	result.BySeverity = severityCounts
	result.RecurringFailures = recurringCount / 2

	if detectCount > 0 {
		result.MeanTimeToDetect = totalDetectTime / time.Duration(detectCount)
	}
	if resolveCount > 0 {
		result.MeanTimeToResolve = totalResolveTime / time.Duration(resolveCount)
	}

	// 根因排名
	for _, item := range causeCounts {
		item.Percentage = float64(item.Count) / float64(len(events)) * 100
		result.TopCauses = append(result.TopCauses, *item)
	}
	sort.Slice(result.TopCauses, func(i, j int) bool {
		return result.TopCauses[i].Count > result.TopCauses[j].Count
	})

	// 失败模式识别
	result.Patterns = fa.detectPatterns(events)

	return result
}

// detectPatterns 检测失败模式
func (fa *FailureAnalyzer) detectPatterns(events []FailureEvent) []FailurePattern {
	var patterns []FailurePattern

	// 模式 1: 同类连续失败
	consecutiveByCategory := make(map[FailureCategory]int)
	for i := 1; i < len(events); i++ {
		if events[i].Category == events[i-1].Category &&
			events[i].DetectedAt.Sub(events[i-1].DetectedAt).Hours() < 24 {
			consecutiveByCategory[events[i].Category]++
		}
	}
	for cat, count := range consecutiveByCategory {
		if count >= 2 {
			patterns = append(patterns, FailurePattern{
				ID:              fmt.Sprintf("pattern-consecutive-%s", cat),
				PatternType:     "consecutive_failure",
				Name:            fmt.Sprintf("连续 %s 失败", cat),
				Description:     fmt.Sprintf("在 24 小时内连续发生 %d 次同类失败, 可能表明系统性问题", count),
				OccurrenceCount: count,
				AffectedAreas:   []string{string(cat)},
				Severity:        "critical",
				Recommendation:  fmt.Sprintf("立即检查 %s 相关的系统性问题, 考虑暂停相关交易", cat),
			})
		}
	}

	// 模式 2: 严重度升级
	for i := 1; i < len(events); i++ {
		sevOrder := map[FailureSeverity]int{
			SeverityInfo: 1, SeverityWarning: 2,
			SeverityCritical: 3, SeverityCatastrophic: 4,
		}
		if sevOrder[events[i].Severity] > sevOrder[events[i-1].Severity] &&
			events[i].Category == events[i-1].Category {
			patterns = append(patterns, FailurePattern{
				ID:              fmt.Sprintf("pattern-escalation-%s-%d", events[i].Category, i),
				PatternType:     "severity_escalation",
				Name:            fmt.Sprintf("%s 严重度升级", events[i].Category),
				Description:     fmt.Sprintf("失败严重度从 %s 升级到 %s, 需要立即关注", events[i-1].Severity, events[i].Severity),
				OccurrenceCount: 1,
				AffectedAreas:   []string{string(events[i].Category)},
				Severity:        "critical",
				Recommendation:  "立即排查根因, 考虑暂停相关策略",
			})
		}
	}

	// 模式 3: 周期性失败
	byHour := make(map[int]int)
	for _, e := range events {
		h := e.DetectedAt.Hour()
		byHour[h]++
	}
	maxCount := 0
	maxHour := 0
	for h, c := range byHour {
		if c > maxCount {
			maxCount = c
			maxHour = h
		}
	}
	if maxCount >= 3 {
		patterns = append(patterns, FailurePattern{
			ID:              fmt.Sprintf("pattern-periodic-hour-%d", maxHour),
			PatternType:     "periodic_failure",
			Name:            fmt.Sprintf("每日 %d 点高发失败", maxHour),
			Description:     fmt.Sprintf("在每日 %d 点附近发生 %d 次失败, 可能与结算、数据更新等定时任务有关", maxHour, maxCount),
			OccurrenceCount: maxCount,
			Severity:        "warning",
			Recommendation:  fmt.Sprintf("检查 %d 点附近的定时任务、数据刷新等环节", maxHour),
		})
	}

	// 模式 4: 同一范式反复失败
	paradigmFailures := make(map[string]int)
	for _, e := range events {
		if e.ParadigmID != "" {
			paradigmFailures[e.ParadigmID]++
		}
	}
	for pid, count := range paradigmFailures {
		if count >= 2 {
			patterns = append(patterns, FailurePattern{
				ID:              fmt.Sprintf("pattern-paradigm-%s", pid),
				PatternType:     "recurring_paradigm_failure",
				Name:            fmt.Sprintf("范式 %s 反复失败", pid),
				Description:     fmt.Sprintf("范式 %s 已发生 %d 次失败, 建议深入审查", pid, count),
				OccurrenceCount: count,
				AffectedAreas:   []string{pid},
				Severity:        "warning",
				Recommendation:  fmt.Sprintf("对范式 %s 进行专项复盘, 考虑重新验证或降级", pid),
			})
		}
	}

	return patterns
}

// SuggestRootCause 基于特征推测根因
func (fa *FailureAnalyzer) SuggestRootCause(event FailureEvent) string {
	switch {
	case event.Deviation > 0.3 && event.Metric == "signal_deviation":
		return "模型预测与实际显著偏离, 可能是市场环境变化或模型退化"
	case event.Category == FailureMarketRegime:
		return "市场状态发生切换, 原模型假设的市场条件不再成立"
	case event.Category == FailureDataQuality:
		return "数据管道问题, 可能缺少数据源、延迟或不一致"
	case event.Category == FailureExecution:
		return "执行层面问题, 可能是流动性不足、价格限制或交易时段限制"
	case event.Category == FailureRiskManagement:
		return "风险控制失效, 可能是止损过宽、仓位过大或集中度超限"
	case event.Category == FailureOverfitting:
		return "模型过拟合, 样本外表现显著低于样本内"
	case event.Category == FailureParameterDrift:
		return "参数漂移, 模型关键参数随时间偏离最优值"
	default:
		return "待人工分析根因"
	}
}

// RankFailureSeverity 对失败事件进行严重度排名
func (fa *FailureAnalyzer) RankFailureSeverity(events []FailureEvent) []FailureEvent {
	severityOrder := map[FailureSeverity]int{
		SeverityCatastrophic: 4,
		SeverityCritical:     3,
		SeverityWarning:      2,
		SeverityInfo:         1,
	}

	sorted := make([]FailureEvent, len(events))
	copy(sorted, events)

	sort.Slice(sorted, func(i, j int) bool {
		return severityOrder[sorted[i].Severity] > severityOrder[sorted[j].Severity]
	})

	return sorted
}
