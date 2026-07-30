// Package monitoring 实现范式漂移、衰减与风险集中监控的主引擎。
//
// 本模块聚合了:
//   - 分布漂移检测 (drift.go)
//   - 性能衰减检测 (decay.go)
//   - 风险集中监控 (concentration.go)
//   - 预警系统 (alert.go)
//
// 使用方式:
//
//	engine := monitoring.NewMonitorEngine()
//	report := engine.RunMonitoring(ctx)
package monitoring

import (
	"math"
	"time"
)

// ============================================================================
// 监控引擎
// ============================================================================

// MonitorConfig 监控引擎配置
type MonitorConfig struct {
	// 数据源标识
	Source string `json:"source"`

	// 各检测器配置
	DriftConfig        DriftConfig        `json:"drift_config"`
	DecayConfig        DecayConfig        `json:"decay_config"`
	ConcentrationConfig ConcentrationConfig `json:"concentration_config"`
	AlertConfig        AlertConfig        `json:"alert_config"`

	// 监控周期
	MonitoringIntervalDays int `json:"monitoring_interval_days"` // 监控间隔 (天)

	// 数据窗口
	AnalysisWindowDays int `json:"analysis_window_days"` // 分析窗口 (天)
	MaxHistoricalDays  int `json:"max_historical_days"`  // 最大历史回溯 (天)
}

// DriftConfig 漂移检测配置 (简化别名)
type DriftConfig struct {
	KSSignificance        float64 `json:"ks_significance"`
	MinSampleSize         int     `json:"min_sample_size"`
	MeanDriftThreshold    float64 `json:"mean_drift_threshold"`
	WinRateDriftThreshold float64 `json:"win_rate_drift_threshold"`
	VolDriftThreshold     float64 `json:"vol_drift_threshold"`
	MildThreshold         float64 `json:"mild_threshold"`
	ModerateThreshold     float64 `json:"moderate_threshold"`
	SevereThreshold       float64 `json:"severe_threshold"`
}

// NewDefaultMonitorConfig 创建默认监控配置
func NewDefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		Source: "default",
		DriftConfig: DriftConfig{
			KSSignificance:        0.05,
			MinSampleSize:         20,
			MeanDriftThreshold:    0.20,
			WinRateDriftThreshold: 0.15,
			VolDriftThreshold:     0.30,
			MildThreshold:         0.10,
			ModerateThreshold:     0.25,
			SevereThreshold:       0.40,
		},
		DecayConfig:         NewDecayConfig(),
		ConcentrationConfig: NewConcentrationConfig(),
		AlertConfig:         NewAlertConfig(),
		MonitoringIntervalDays: 1,
		AnalysisWindowDays:     60,
		MaxHistoricalDays:      252,
	}
}

// MonitorEngine 监控引擎
type MonitorEngine struct {
	Config           MonitorConfig
	DriftDetector    *DriftDetector
	DecayDetector    *DecayDetector
	ConcentrationMon *ConcentrationMonitor
	AlertEngine      *AlertEngine
}

// NewMonitorEngine 创建监控引擎
func NewMonitorEngine(config MonitorConfig) *MonitorEngine {
	driftDetector := NewDriftDetectorWithConfig(config.DriftConfig)
	decayDetector := NewDecayDetector(config.DecayConfig)
	concMonitor := NewConcentrationMonitor(config.ConcentrationConfig)
	alertEngine := NewAlertEngine(config.AlertConfig)

	return &MonitorEngine{
		Config:           config,
		DriftDetector:    driftDetector,
		DecayDetector:    decayDetector,
		ConcentrationMon: concMonitor,
		AlertEngine:      alertEngine,
	}
}

// NewDriftDetectorWithConfig 使用配置创建漂移检测器
func NewDriftDetectorWithConfig(config DriftConfig) *DriftDetector {
	return &DriftDetector{
		KSSignificance:        config.KSSignificance,
		MinSampleSize:         config.MinSampleSize,
		MeanDriftThreshold:    config.MeanDriftThreshold,
		WinRateDriftThreshold: config.WinRateDriftThreshold,
		VolDriftThreshold:     config.VolDriftThreshold,
		MildThreshold:         config.MildThreshold,
		ModerateThreshold:     config.ModerateThreshold,
		SevereThreshold:       config.SevereThreshold,
	}
}

// MonitorReport 监控报告
type MonitorReport struct {
	Source         string                        `json:"source"`
	GeneratedAt    time.Time                     `json:"generated_at"`
	Period         MonitoringPeriod              `json:"period"`

	// 漂移检测结果
	DriftResults   []DriftDetectionResult        `json:"drift_results"`
	DriftSummary   DriftSummary                  `json:"drift_summary"`

	// 衰减检测结果
	DecayResults   []DecayDetectionResult        `json:"decay_results"`
	DecaySummary   DecaySummary                  `json:"decay_summary"`

	// 集中度结果
	ConcentrationResults []ConcentrationResult    `json:"concentration_results"`
	ConcentrationSummary ConcentrationSummary      `json:"concentration_summary"`

	// 预警
	GeneratedAlerts []Alert                       `json:"generated_alerts"`
	AlertSummary    AlertSummary                  `json:"alert_summary"`

	// 建议
	Recommendations []string                     `json:"recommendations"`
	HealthScore     float64                      `json:"health_score"` // 0-100, 越高越好
}

// MonitoringPeriod 监控周期
type MonitoringPeriod struct {
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	WindowDays int      `json:"window_days"`
}

// DriftSummary 漂移汇总
type DriftSummary struct {
	TotalDetections  int `json:"total_detections"`
	SignificantCount int `json:"significant_count"`
	SevereCount      int `json:"severe_count"`
	AverageDeltaPct  float64 `json:"average_delta_pct"`
	OverallStatus    string `json:"overall_status"` // normal / watch / alert / critical
}

// DecaySummary 衰减汇总
type DecaySummary struct {
	TotalDetections int     `json:"total_detections"`
	DecayingCount   int     `json:"decaying_count"`
	CriticalCount   int     `json:"critical_count"`
	AvgConfidence   float64 `json:"avg_confidence"`
	OverallStatus   string  `json:"overall_status"`
}

// ConcentrationSummary 集中度汇总
type ConcentrationSummary struct {
	TotalDetections int     `json:"total_detections"`
	ConcentratedCount int   `json:"concentrated_count"`
	CriticalCount   int     `json:"critical_count"`
	AvgHHI          float64 `json:"avg_hhi"`
	OverallStatus   string  `json:"overall_status"`
}

// MonitoringInput 监控输入
type MonitoringInput struct {
	// 收益数据
	BaselineReturns []float64   `json:"baseline_returns"` // 历史基准收益
	ForwardReturns  []float64   `json:"forward_returns"`  // 前向实际收益
	ForwardDates    []time.Time `json:"forward_dates"`    // 前向收益对应日期

	// 持仓数据
	Positions []PositionItem `json:"positions"`
}

// RunMonitoring 执行全套监控
func (e *MonitorEngine) RunMonitoring(input MonitoringInput) MonitorReport {
	report := MonitorReport{
		Source:      e.Config.Source,
		GeneratedAt: time.Now(),
		Period: MonitoringPeriod{
			StartDate: time.Now().AddDate(0, 0, -e.Config.AnalysisWindowDays),
			EndDate:   time.Now(),
			WindowDays: e.Config.AnalysisWindowDays,
		},
	}

	// 1. 漂移检测
	report.DriftResults = e.DriftDetector.DetectAll(input.BaselineReturns, input.ForwardReturns)
	report.DriftSummary = summarizeDrift(report.DriftResults)

	// 2. 衰减检测
	report.DecayResults = e.DecayDetector.DetectAll(input.ForwardReturns, input.ForwardDates)
	report.DecaySummary = summarizeDecay(report.DecayResults)

	// 3. 集中度监控
	report.ConcentrationResults = e.ConcentrationMon.MonitorAll(input.Positions)
	report.ConcentrationSummary = summarizeConcentration(report.ConcentrationResults)

	// 4. 生成预警
	report.GeneratedAlerts = e.AlertEngine.GenerateAlerts(
		report.DriftResults,
		report.DecayResults,
		report.ConcentrationResults,
		e.Config.Source,
	)
	report.AlertSummary = e.AlertEngine.GetAlertSummary()

	// 5. 生成建议
	report.Recommendations = generateRecommendations(
		report.DriftSummary,
		report.DecaySummary,
		report.ConcentrationSummary,
		report.AlertSummary,
	)

	// 6. 计算健康分数
	report.HealthScore = calculateHealthScore(
		report.DriftSummary,
		report.DecaySummary,
		report.ConcentrationSummary,
		report.AlertSummary,
	)

	return report
}

// summarizeDrift 汇总漂移检测结果
func summarizeDrift(results []DriftDetectionResult) DriftSummary {
	summary := DriftSummary{TotalDetections: len(results)}
	var totalDelta float64

	for _, r := range results {
		if r.Significant {
			summary.SignificantCount++
		}
		if r.Severity == "severe" {
			summary.SevereCount++
		}
		totalDelta += math.Abs(r.DeltaPct)
	}

	if len(results) > 0 {
		summary.AverageDeltaPct = totalDelta / float64(len(results))
	}

	switch {
	case summary.SevereCount > 0:
		summary.OverallStatus = "critical"
	case summary.SignificantCount > 2:
		summary.OverallStatus = "alert"
	case summary.SignificantCount > 0:
		summary.OverallStatus = "watch"
	default:
		summary.OverallStatus = "normal"
	}

	return summary
}

// summarizeDecay 汇总衰减检测结果
func summarizeDecay(results []DecayDetectionResult) DecaySummary {
	summary := DecaySummary{TotalDetections: len(results)}
	var totalConfidence float64

	for _, r := range results {
		if r.IsDecaying {
			summary.DecayingCount++
		}
		if r.Severity == "critical" {
			summary.CriticalCount++
		}
		totalConfidence += r.Confidence
	}

	if len(results) > 0 {
		summary.AvgConfidence = totalConfidence / float64(len(results))
	}

	switch {
	case summary.CriticalCount > 0:
		summary.OverallStatus = "critical"
	case summary.DecayingCount >= 3:
		summary.OverallStatus = "alert"
	case summary.DecayingCount > 0:
		summary.OverallStatus = "watch"
	default:
		summary.OverallStatus = "normal"
	}

	return summary
}

// summarizeConcentration 汇总集中度结果
func summarizeConcentration(results []ConcentrationResult) ConcentrationSummary {
	summary := ConcentrationSummary{TotalDetections: len(results)}
	var totalHHI float64

	for _, r := range results {
		if r.IsConcentrated {
			summary.ConcentratedCount++
		}
		if r.Severity == "critical" {
			summary.CriticalCount++
		}
		totalHHI += r.HHI
	}

	if len(results) > 0 {
		summary.AvgHHI = totalHHI / float64(len(results))
	}

	switch {
	case summary.CriticalCount > 0:
		summary.OverallStatus = "critical"
	case summary.ConcentratedCount > 1:
		summary.OverallStatus = "alert"
	case summary.ConcentratedCount > 0:
		summary.OverallStatus = "watch"
	default:
		summary.OverallStatus = "normal"
	}

	return summary
}

// generateRecommendations 生成监控建议
func generateRecommendations(
	drift DriftSummary,
	decay DecaySummary,
	conc ConcentrationSummary,
	alerts AlertSummary,
) []string {
	var recommendations []string

	// 漂移建议
	switch drift.OverallStatus {
	case "critical":
		recommendations = append(recommendations,
			"【漂移严重】前向收益分布与基准差异显著, 建议暂停策略并重新评估模型假设")
	case "alert":
		recommendations = append(recommendations,
			"【漂移预警】多项分布指标显著偏移, 建议检查市场环境变化并调整策略参数")
	case "watch":
		recommendations = append(recommendations,
			"【漂移观察】部分指标出现偏移迹象, 建议密切监控后续变化")
	}

	// 衰减建议
	switch decay.OverallStatus {
	case "critical":
		recommendations = append(recommendations,
			"【衰减严重】策略核心指标持续恶化, 建议立即降低仓位或切换策略")
	case "alert":
		recommendations = append(recommendations,
			"【衰减预警】多项性能指标下降, 建议考虑参数优化或切换至备选方案")
	case "watch":
		recommendations = append(recommendations,
			"【衰减观察】部分性能指标出现下降趋势, 建议关注后续表现")
	}

	// 集中度建议
	switch conc.OverallStatus {
	case "critical":
		recommendations = append(recommendations,
			"【集中风险极高】持仓过度集中, 建议立即分散至更多标的")
	case "alert":
		recommendations = append(recommendations,
			"【集中风险偏高】存在行业/个股集中风险, 建议增加标的多样性")
	case "watch":
		recommendations = append(recommendations,
			"【集中风险关注】持仓集中度接近阈值, 建议关注变化")
	}

	// 预警汇总
	if alerts.CriticalCount > 0 {
		recommendations = append(recommendations,
			"【系统预警】存在 "+itoa(alerts.CriticalCount)+" 条严重级别预警, 需要立即处理")
	}

	return recommendations
}

// calculateHealthScore 计算健康分数 (0-100)
func calculateHealthScore(
	drift DriftSummary,
	decay DecaySummary,
	conc ConcentrationSummary,
	alerts AlertSummary,
) float64 {
	// 基础分 100
	score := 100.0

	// 漂移扣分
	switch drift.OverallStatus {
	case "critical":
		score -= 30
	case "alert":
		score -= 15
	case "watch":
		score -= 5
	}
	score -= float64(drift.SignificantCount) * 2

	// 衰减扣分
	switch decay.OverallStatus {
	case "critical":
		score -= 25
	case "alert":
		score -= 12
	case "watch":
		score -= 4
	}
	score -= float64(decay.DecayingCount) * 3

	// 集中度扣分
	switch conc.OverallStatus {
	case "critical":
		score -= 20
	case "alert":
		score -= 10
	case "watch":
		score -= 3
	}

	// 预警扣分
	score -= float64(alerts.CriticalCount) * 5
	score -= float64(alerts.DangerCount) * 3
	score -= float64(alerts.WarningCount) * 1

	// 限制范围
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

// itoa 整数转字符串 (避免 fmt 导入冲突)
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
