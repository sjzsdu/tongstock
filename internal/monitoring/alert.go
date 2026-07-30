// Package monitoring - 预警系统模块
package monitoring

import (
	"fmt"
	"sort"
	"time"
)

// ============================================================================
// 预警系统
// ============================================================================

// AlertLevel 预警级别
type AlertLevel string

const (
	AlertLevelInfo    AlertLevel = "info"    // 信息 (仅供参考)
	AlertLevelWarning AlertLevel = "warning" // 警告 (需要关注)
	AlertLevelDanger  AlertLevel = "danger"  // 危险 (需要行动)
	AlertLevelCritical AlertLevel = "critical" // 严重 (立即行动)
)

// AlertCategory 预警分类
type AlertCategory string

const (
	CategoryDrift        AlertCategory = "drift"
	CategoryDecay        AlertCategory = "decay"
	CategoryConcentration AlertCategory = "concentration"
	CategorySystem       AlertCategory = "system"
)

// AlertStatus 预警状态
type AlertStatus string

const (
	AlertStatusActive    AlertStatus = "active"
	AlertStatusAcked     AlertStatus = "acknowledged"
	AlertStatusResolved  AlertStatus = "resolved"
	AlertStatusSuppressed AlertStatus = "suppressed"
)

// Alert 预警事件
type Alert struct {
	ID          string        `json:"id"`
	Category    AlertCategory `json:"category"`
	Level       AlertLevel    `json:"level"`
	Status      AlertStatus   `json:"status"`
	Title       string        `json:"title"`
	Message     string        `json:"message"`
	Source      string        `json:"source"`      // 数据源标识 (paradigm_version_id 等)
	MetricName  string        `json:"metric_name"`
	MetricValue float64       `json:"metric_value"`
	Threshold   float64       `json:"threshold"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	AckedAt     *time.Time    `json:"acked_at,omitempty"`
	ResolvedAt  *time.Time    `json:"resolved_at,omitempty"`
	AckedBy     string        `json:"acked_by,omitempty"`
}

// AlertConfig 预警配置
type AlertConfig struct {
	// 预警冷却 (同一 source + metric 在冷却期内不重复触发)
	CooldownMinutes int `json:"cooldown_minutes"`

	// 升级规则
	PromoteToDangerThreshold  int `json:"promote_to_danger_threshold"`  // N 次 warning 升级为 danger
	PromoteToCriticalThreshold int `json:"promote_to_critical_threshold"` // N 次 danger 升级为 critical

	// 抑制规则
	AutoSuppressMinutes int `json:"auto_suppress_minutes"` // 自动抑制级别以下的预警
}

// NewAlertConfig 创建默认预警配置
func NewAlertConfig() AlertConfig {
	return AlertConfig{
		CooldownMinutes:           30,
		PromoteToDangerThreshold:  3,
		PromoteToCriticalThreshold: 5,
		AutoSuppressMinutes:       60,
	}
}

// AlertEngine 预警引擎
type AlertEngine struct {
	Config AlertConfig
	alerts []Alert
	counter int
}

// NewAlertEngine 创建预警引擎
func NewAlertEngine(config AlertConfig) *AlertEngine {
	return &AlertEngine{
		Config: config,
		alerts: make([]Alert, 0),
	}
}

// GenerateAlerts 从漂移/衰减/集中度结果生成预警
func (e *AlertEngine) GenerateAlerts(
	driftResults []DriftDetectionResult,
	decayResults []DecayDetectionResult,
	concResults []ConcentrationResult,
	source string,
) []Alert {
	var newAlerts []Alert

	// 从漂移结果生成预警
	for _, r := range driftResults {
		alert := e.alertFromDrift(r, source)
		if alert != nil {
			newAlerts = append(newAlerts, *alert)
		}
	}

	// 从衰减结果生成预警
	for _, r := range decayResults {
		alert := e.alertFromDecay(r, source)
		if alert != nil {
			newAlerts = append(newAlerts, *alert)
		}
	}

	// 从集中度结果生成预警
	for _, r := range concResults {
		alert := e.alertFromConcentration(r, source)
		if alert != nil {
			newAlerts = append(newAlerts, *alert)
		}
	}

	// 应用冷却和升级规则
	for i := range newAlerts {
		e.applyAlertRules(&newAlerts[i])
		e.alerts = append(e.alerts, newAlerts[i])
	}

	return newAlerts
}

// alertFromDrift 从漂移检测结果生成预警
func (e *AlertEngine) alertFromDrift(r DriftDetectionResult, source string) *Alert {
	level := driftAlertLevel(r)
	if level == "" {
		return nil
	}

	e.counter++
	alert := Alert{
		ID:         fmt.Sprintf("drift-%d", e.counter),
		Category:   CategoryDrift,
		Level:      level,
		Status:     AlertStatusActive,
		Title:      driftAlertTitle(r),
		Message:    r.Description,
		Source:     source,
		MetricName: r.MetricName,
		MetricValue: r.NewValue,
		Threshold:  r.Threshold,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Metadata: map[string]interface{}{
			"drift_type":   r.Type,
			"severity":     r.Severity,
			"old_value":    r.OldValue,
			"new_value":    r.NewValue,
			"delta_pct":    r.DeltaPct,
			"p_value":      r.PValue,
			"sample_size":  r.SampleSize,
		},
	}
	return &alert
}

// driftAlertLevel 根据漂移结果确定预警级别
func driftAlertLevel(r DriftDetectionResult) AlertLevel {
	switch r.Severity {
	case "severe":
		return AlertLevelCritical
	case "moderate":
		return AlertLevelDanger
	case "mild":
		return AlertLevelWarning
	case "normal":
		if r.Significant {
			return AlertLevelInfo
		}
		return ""
	default:
		return ""
	}
}

// driftAlertTitle 生成漂移预警标题
func driftAlertTitle(r DriftDetectionResult) string {
	titles := map[DriftType]string{
		DriftDistribution: "分布漂移检测",
		DriftMean:         "期望收益漂移",
		DriftWinRate:      "胜率漂移",
		DriftVolatility:   "波动率漂移",
		DriftSkewness:     "收益分布偏度变化",
		DriftKurtosis:     "收益分布峰度变化",
		DriftCoverage:     "数据覆盖率变化",
	}
	if title, ok := titles[r.Type]; ok {
		return title
	}
	return "漂移检测"
}

// alertFromDecay 从衰减检测结果生成预警
func (e *AlertEngine) alertFromDecay(r DecayDetectionResult, source string) *Alert {
	if !r.IsDecaying && r.Severity == "normal" {
		return nil
	}

	level := decayAlertLevel(r)
	if level == "" {
		return nil
	}

	e.counter++
	alert := Alert{
		ID:         fmt.Sprintf("decay-%d", e.counter),
		Category:   CategoryDecay,
		Level:      level,
		Status:     AlertStatusActive,
		Title:      decayAlertTitle(r),
		Message:    r.Description,
		Source:     source,
		MetricName: string(r.Type),
		MetricValue: r.CurrentValue,
		Threshold:  r.HistoricalAvg,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Metadata: map[string]interface{}{
			"decay_type":   r.Type,
			"severity":     r.Severity,
			"change_pct":   r.ChangePct,
			"confidence":   r.Confidence,
			"current":      r.CurrentValue,
			"historical":   r.HistoricalAvg,
		},
	}
	return &alert
}

// decayAlertLevel 根据衰减结果确定预警级别
func decayAlertLevel(r DecayDetectionResult) AlertLevel {
	switch r.Severity {
	case "critical":
		return AlertLevelCritical
	case "active":
		return AlertLevelDanger
	case "early":
		return AlertLevelWarning
	case "normal":
		if r.IsDecaying {
			return AlertLevelInfo
		}
		return ""
	default:
		return ""
	}
}

// decayAlertTitle 生成衰减预警标题
func decayAlertTitle(r DecayDetectionResult) string {
	titles := map[DecayType]string{
		DecaySharpeDecline:     "夏普比率衰减",
		DecayWinRateDrop:       "胜率衰减",
		DecayHalfLife:          "策略半衰期缩短",
		DecayDrawdownExpansion: "回撤扩大",
		DecayCUSUM:             "持续偏移检测",
		DecayVolatilityIncrease: "波动率上升",
		DecayHitRateDecay:      "命中率衰减",
	}
	if title, ok := titles[r.Type]; ok {
		return title
	}
	return "性能衰减检测"
}

// alertFromConcentration 从集中度结果生成预警
func (e *AlertEngine) alertFromConcentration(r ConcentrationResult, source string) *Alert {
	if !r.IsConcentrated && r.Severity == "normal" {
		return nil
	}

	level := concAlertLevel(r)
	if level == "" {
		return nil
	}

	e.counter++
	alert := Alert{
		ID:         fmt.Sprintf("conc-%d", e.counter),
		Category:   CategoryConcentration,
		Level:      level,
		Status:     AlertStatusActive,
		Title:      concAlertTitle(r),
		Message:    r.Description,
		Source:     source,
		MetricName: string(r.Type),
		MetricValue: r.HHI,
		Threshold:  r.Threshold,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Metadata: map[string]interface{}{
			"conc_type":       r.Type,
			"severity":        r.Severity,
			"hhi":             r.HHI,
			"effective_count": r.EffectiveCount,
			"top_contributor": r.TopContributor,
			"top_weight":      r.TopWeight,
		},
	}
	return &alert
}

// concAlertLevel 根据集中度结果确定预警级别
func concAlertLevel(r ConcentrationResult) AlertLevel {
	switch r.Severity {
	case "critical":
		return AlertLevelCritical
	case "warning":
		return AlertLevelWarning
	case "normal":
		if r.IsConcentrated {
			return AlertLevelInfo
		}
		return ""
	default:
		return ""
	}
}

// concAlertTitle 生成集中度预警标题
func concAlertTitle(r ConcentrationResult) string {
	titles := map[ConcentrationType]string{
		ConcentrationPosition:   "持仓集中度",
		ConcentrationIndustry:   "行业集中度",
		ConcentrationStock:      "个股集中度",
		ConcentrationSector:     "板块集中度",
		ConcentrationCorrelation: "相关性集中度",
	}
	if title, ok := titles[r.Type]; ok {
		return title + "预警"
	}
	return "集中度预警"
}

// applyAlertRules 应用预警规则 (冷却期、升级)
func (e *AlertEngine) applyAlertRules(alert *Alert) {
	// 检查冷却期: 相同 source + metric 最近的 active 预警
	cutoff := time.Now().Add(-time.Duration(e.Config.CooldownMinutes) * time.Minute)
	for _, existing := range e.alerts {
		if existing.Source == alert.Source &&
			existing.MetricName == alert.MetricName &&
			existing.Status == AlertStatusActive &&
			existing.CreatedAt.After(cutoff) {
			// 在冷却期内: 合并为现有预警的更新, 不创建新预警
			alert.Status = AlertStatusSuppressed
			alert.Title = ""
			alert.Message = ""
			return
		}
	}
}

// AcknowledgeAlert 确认预警
func (e *AlertEngine) AcknowledgeAlert(id, ackedBy string) error {
	for i := range e.alerts {
		if e.alerts[i].ID == id && e.alerts[i].Status == AlertStatusActive {
			now := time.Now()
			e.alerts[i].Status = AlertStatusAcked
			e.alerts[i].AckedAt = &now
			e.alerts[i].AckedBy = ackedBy
			e.alerts[i].UpdatedAt = now
			return nil
		}
	}
	return fmt.Errorf("alert %s not found or not active", id)
}

// ResolveAlert 解决预警
func (e *AlertEngine) ResolveAlert(id string) error {
	for i := range e.alerts {
		if e.alerts[i].ID == id {
			now := time.Now()
			e.alerts[i].Status = AlertStatusResolved
			e.alerts[i].ResolvedAt = &now
			e.alerts[i].UpdatedAt = now
			return nil
		}
	}
	return fmt.Errorf("alert %s not found", id)
}

// GetActiveAlerts 获取活跃预警
func (e *AlertEngine) GetActiveAlerts() []Alert {
	var active []Alert
	for _, a := range e.alerts {
		if a.Status == AlertStatusActive {
			active = append(active, a)
		}
	}
	return active
}

// GetAlertsBySource 获取指定 source 的预警
func (e *AlertEngine) GetAlertsBySource(source string, statuses ...AlertStatus) []Alert {
	var result []Alert
	for _, a := range e.alerts {
		if a.Source == source {
			if len(statuses) == 0 {
				result = append(result, a)
			} else {
				for _, s := range statuses {
					if a.Status == s {
						result = append(result, a)
						break
					}
				}
			}
		}
	}
	return result
}

// GetAlertsByLevel 获取指定级别的预警
func (e *AlertEngine) GetAlertsByLevel(level AlertLevel) []Alert {
	var result []Alert
	for _, a := range e.alerts {
		if a.Level == level && a.Status == AlertStatusActive {
			result = append(result, a)
		}
	}
	return result
}

// GetAlertSummary 获取预警汇总
func (e *AlertEngine) GetAlertSummary() AlertSummary {
	summary := AlertSummary{}
	summary.TotalAlerts = len(e.alerts)

	for _, a := range e.alerts {
		switch a.Status {
		case AlertStatusActive:
			summary.ActiveCount++
		case AlertStatusAcked:
			summary.AckedCount++
		case AlertStatusResolved:
			summary.ResolvedCount++
		case AlertStatusSuppressed:
			summary.SuppressedCount++
		}

		switch a.Level {
		case AlertLevelCritical:
			summary.CriticalCount++
		case AlertLevelDanger:
			summary.DangerCount++
		case AlertLevelWarning:
			summary.WarningCount++
		case AlertLevelInfo:
			summary.InfoCount++
		}
	}

	return summary
}

// AlertSummary 预警汇总
type AlertSummary struct {
	TotalAlerts      int `json:"total_alerts"`
	ActiveCount      int `json:"active_count"`
	AckedCount       int `json:"acked_count"`
	ResolvedCount    int `json:"resolved_count"`
	SuppressedCount  int `json:"suppressed_count"`
	CriticalCount    int `json:"critical_count"`
	DangerCount      int `json:"danger_count"`
	WarningCount     int `json:"warning_count"`
	InfoCount        int `json:"info_count"`
}

// SortAlerts 按级别和时间排序预警
func SortAlerts(alerts []Alert) []Alert {
	sorted := make([]Alert, len(alerts))
	copy(sorted, alerts)

	levelOrder := map[AlertLevel]int{
		AlertLevelCritical: 4,
		AlertLevelDanger:   3,
		AlertLevelWarning:  2,
		AlertLevelInfo:     1,
	}

	sort.Slice(sorted, func(i, j int) bool {
		if levelOrder[sorted[i].Level] != levelOrder[sorted[j].Level] {
			return levelOrder[sorted[i].Level] > levelOrder[sorted[j].Level]
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	return sorted
}
