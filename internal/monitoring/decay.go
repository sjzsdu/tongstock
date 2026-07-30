// Package monitoring - 性能衰减检测模块
package monitoring

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// ============================================================================
// 性能衰减检测
// ============================================================================

// DecayType 衰减类型
type DecayType string

const (
	DecaySharpeDecline    DecayType = "sharpe_decline"    // 夏普比率下降
	DecayWinRateDrop      DecayType = "win_rate_drop"      // 胜率下降
	DecayHalfLife         DecayType = "half_life"          // 半衰期衰减
	DecayDrawdownExpansion DecayType = "drawdown_expansion" // 回撤扩大
	DecayCUSUM            DecayType = "cusum"              // CUSUM 持续偏移
	DecayVolatilityIncrease DecayType = "volatility_increase" // 波动率上升
	DecayHitRateDecay     DecayType = "hit_rate_decay"     // 命中率衰减
)

// DecayDetectionResult 衰减检测结果
type DecayDetectionResult struct {
	Type          DecayType `json:"type"`
	IsDecaying    bool      `json:"is_decaying"`
	Severity      string    `json:"severity"` // normal / early / active / critical
	CurrentValue  float64   `json:"current_value"`
	HistoricalAvg float64   `json:"historical_avg"`
	ChangePct     float64   `json:"change_pct"`
	WindowDays    int       `json:"window_days"`
	Confidence    float64   `json:"confidence"` // 0-1, 越高越确定
	Description   string    `json:"description"`
	DetectedAt    time.Time `json:"detected_at"`
}

// DecayConfig 衰减检测配置
type DecayConfig struct {
	// 窗口配置
	ShortWindow  int `json:"short_window"`  // 短期窗口 (天)
	MediumWindow int `json:"medium_window"` // 中期窗口 (天)
	LongWindow   int `json:"long_window"`   // 长期窗口 (天)

	// 衰减阈值
	SharpeDeclineThreshold    float64 `json:"sharpe_decline_threshold"`    // 夏普比率下降阈值
	WinRateDropThreshold      float64 `json:"win_rate_drop_threshold"`      // 胜率下降阈值
	DrawdownExpansionThreshold float64 `json:"drawdown_expansion_threshold"` // 回撤扩大阈值
	HalfLifeDecayThreshold    float64 `json:"half_life_decay_threshold"`    // 半衰期衰减阈值

	// CUSUM 参数
	CUSUMK       float64 `json:"cusum_k"`        // CUSUM 参考值 (允许的偏移)
	CUMUH        float64 `json:"cusum_h"`        // CUSUM 决策阈值

	// 小样本保护
	MinWindowSize int `json:"min_window_size"` // 最小窗口大小 (低于此值不判断)
}

// NewDecayConfig 创建默认衰减检测配置
func NewDecayConfig() DecayConfig {
	return DecayConfig{
		ShortWindow:              10,
		MediumWindow:             30,
		LongWindow:               60,
		SharpeDeclineThreshold:   0.30,
		WinRateDropThreshold:     0.15,
		DrawdownExpansionThreshold: 0.50,
		HalfLifeDecayThreshold:   0.10,
		CUSUMK:                   0.001,
		CUMUH:                    0.01,
		MinWindowSize:            10,
	}
}

// DecayDetector 衰减检测器
type DecayDetector struct {
	Config DecayConfig
}

// NewDecayDetector 创建衰减检测器
func NewDecayDetector(config DecayConfig) *DecayDetector {
	return &DecayDetector{Config: config}
}

// DetectAll 执行全套衰减检测
func (d *DecayDetector) DetectAll(returns []float64, dates []time.Time) []DecayDetectionResult {
	var results []DecayDetectionResult

	if len(returns) < d.Config.MinWindowSize {
		return results
	}

	// 1. 夏普比率衰减
	results = append(results, d.DetectSharpeDecline(returns))

	// 2. 胜率衰减
	results = append(results, d.DetectWinRateDecay(returns))

	// 3. 回撤扩大
	results = append(results, d.DetectDrawdownExpansion(returns))

	// 4. CUSUM 持续偏移
	results = append(results, d.DetectCUSUM(returns))

	// 5. 半衰期估计
	results = append(results, d.DetectHalfLife(returns, dates))

	return results
}

// DetectSharpeDecline 检测夏普比率衰减
func (d *DecayDetector) DetectSharpeDecline(returns []float64) DecayDetectionResult {
	// 计算不同窗口的夏普比率
	shortReturns := lastWindow(returns, d.Config.ShortWindow)
	longReturns := lastWindow(returns, d.Config.LongWindow)

	shortSharpe := annualizedSharpe(shortReturns)
	longSharpe := annualizedSharpe(longReturns)

	changePct := 0.0
	if longSharpe != 0 {
		changePct = (shortSharpe - longSharpe) / math.Abs(longSharpe)
	}

	isDecaying := changePct < -d.Config.SharpeDeclineThreshold
	severity := d.classifyDecaySeverity(changePct, -d.Config.SharpeDeclineThreshold)

	return DecayDetectionResult{
		Type:          DecaySharpeDecline,
		IsDecaying:    isDecaying,
		Severity:      severity,
		CurrentValue:  shortSharpe,
		HistoricalAvg: longSharpe,
		ChangePct:     changePct,
		WindowDays:    d.Config.ShortWindow,
		Confidence:    decayConfidence(len(shortReturns), len(longReturns)),
		Description:   fmt.Sprintf("短窗夏普 %.2f vs 长窗 %.2f (变化 %.1f%%) [%s]",
			shortSharpe, longSharpe, changePct*100, severity),
		DetectedAt:    time.Now(),
	}
}

// DetectWinRateDecay 检测胜率衰减
func (d *DecayDetector) DetectWinRateDecay(returns []float64) DecayDetectionResult {
	shortReturns := lastWindow(returns, d.Config.ShortWindow)
	mediumReturns := lastWindow(returns, d.Config.MediumWindow)

	shortWR := winRate(shortReturns)
	mediumWR := winRate(mediumReturns)

	changePct := 0.0
	if mediumWR > 0 {
		changePct = (shortWR - mediumWR) / mediumWR
	}

	isDecaying := changePct < -d.Config.WinRateDropThreshold
	severity := d.classifyDecaySeverity(changePct, -d.Config.WinRateDropThreshold)

	return DecayDetectionResult{
		Type:          DecayWinRateDrop,
		IsDecaying:    isDecaying,
		Severity:      severity,
		CurrentValue:  shortWR,
		HistoricalAvg: mediumWR,
		ChangePct:     changePct,
		WindowDays:    d.Config.ShortWindow,
		Confidence:    decayConfidence(len(shortReturns), len(mediumReturns)),
		Description:   fmt.Sprintf("短窗胜率 %.1f%% vs 中窗 %.1f%% (变化 %.1f%%) [%s]",
			shortWR*100, mediumWR*100, changePct*100, severity),
		DetectedAt:    time.Now(),
	}
}

// DetectDrawdownExpansion 检测回撤扩大
func (d *DecayDetector) DetectDrawdownExpansion(returns []float64) DecayDetectionResult {
	shortReturns := lastWindow(returns, d.Config.ShortWindow)
	longReturns := lastWindow(returns, d.Config.LongWindow)

	shortMaxDD := maxDrawdown(shortReturns)
	longMaxDD := maxDrawdown(longReturns)

	changePct := 0.0
	if longMaxDD > 0 {
		changePct = (shortMaxDD - longMaxDD) / longMaxDD
	}

	isDecaying := changePct > d.Config.DrawdownExpansionThreshold
	severity := d.classifyDecaySeverity(changePct, d.Config.DrawdownExpansionThreshold)

	return DecayDetectionResult{
		Type:          DecayDrawdownExpansion,
		IsDecaying:    isDecaying,
		Severity:      severity,
		CurrentValue:  shortMaxDD,
		HistoricalAvg: longMaxDD,
		ChangePct:     changePct,
		WindowDays:    d.Config.ShortWindow,
		Confidence:    decayConfidence(len(shortReturns), len(longReturns)),
		Description:   fmt.Sprintf("短窗最大回撤 %.2f%% vs 长窗 %.2f%% (变化 %.1f%%) [%s]",
			shortMaxDD*100, longMaxDD*100, changePct*100, severity),
		DetectedAt:    time.Now(),
	}
}

// DetectCUSUM 使用 CUSUM 检测持续偏移
func (d *DecayDetector) DetectCUSUM(returns []float64) DecayDetectionResult {
	if len(returns) < d.Config.MinWindowSize*2 {
		return DecayDetectionResult{
			Type:        DecayCUSUM,
			IsDecaying:  false,
			Severity:    "insufficient_data",
			Description: "样本量不足以执行 CUSUM 检测",
			DetectedAt:  time.Now(),
		}
	}

	// 计算均值作为参考值
	mu0 := mean(returns)
	if mu0 == 0 {
		mu0 = 0.001 // 避免零均值
	}

	// CUSUM 上边界检测
	var sH float64
	var maxSH float64
	var peakIdx int

	for i, r := range returns {
		z := (r - mu0)
		sH = math.Max(0, sH+z-d.Config.CUSUMK)
		if sH > maxSH {
			maxSH = sH
			peakIdx = i
		}
	}

	// CUSUM 下边界检测
	var sL float64
	var maxSL float64
	for _, r := range returns {
		z := (r - mu0)
		sL = math.Max(0, sL-z-d.Config.CUSUMK)
		if sL > maxSL {
			maxSL = sL
		}
	}

	// 判断是否触发 CUSUM 警报
	maxCS := math.Max(maxSH, maxSL)
	isDecaying := maxCS > d.Config.CUMUH

	severity := "normal"
	if maxCS > d.Config.CUMUH*2 {
		severity = "critical"
	} else if maxCS > d.Config.CUMUH*1.5 {
		severity = "active"
	} else if isDecaying {
		severity = "early"
	}

	changePct := 0.0
	if len(returns) > 0 && peakIdx > 0 {
		prePeak := mean(returns[:peakIdx])
		postPeak := mean(returns[peakIdx:])
		if prePeak != 0 {
			changePct = (postPeak - prePeak) / math.Abs(prePeak)
		}
	}

	return DecayDetectionResult{
		Type:          DecayCUSUM,
		IsDecaying:    isDecaying,
		Severity:      severity,
		CurrentValue:  maxCS,
		HistoricalAvg: d.Config.CUMUH,
		ChangePct:     changePct,
		WindowDays:    len(returns),
		Confidence:    math.Min(1.0, float64(maxCS)/d.Config.CUMUH),
		Description:   fmt.Sprintf("CUSUM 统计量 %.4f (阈值 %.4f), %s",
			maxCS, d.Config.CUMUH, severity),
		DetectedAt:    time.Now(),
	}
}

// DetectHalfLife 估计策略收益的半衰期
func (d *DecayDetector) DetectHalfLife(returns []float64, dates []time.Time) DecayDetectionResult {
	if len(returns) < d.Config.MinWindowSize*2 {
		return DecayDetectionResult{
			Type:        DecayHalfLife,
			IsDecaying:  false,
			Severity:    "insufficient_data",
			Description: "样本量不足以估计半衰期",
			DetectedAt:  time.Now(),
		}
	}

	// 使用指数加权平均估计有效样本量 (半衰期)
	// EWMA: y_t = alpha * x_t + (1-alpha) * y_{t-1}
	// 半衰期: h = -log(0.5) / (-log(1-alpha))
	// 简化: 使用不同 alpha 值比较预测误差

	// 分半段比较
	mid := len(returns) / 2
	firstHalf := returns[:mid]
	secondHalf := returns[mid:]

	// 使用第一半的均值预测第二半
	predictedFirst := mean(firstHalf)
	actualSecond := mean(secondHalf)

	// 预测误差
	forecastError := math.Abs(actualSecond - predictedFirst)
	relativeError := 0.0
	if math.Abs(actualSecond) > 0.001 {
		relativeError = forecastError / math.Abs(actualSecond)
	}

	// 估计半衰期 (简化模型)
	// 半衰期越短，策略衰减越快
	halfLifeEstimate := float64(len(returns)) / math.Max(1, relativeError*5)
	if halfLifeEstimate > float64(len(returns)) {
		halfLifeEstimate = float64(len(returns))
	}

	isDecaying := halfLifeEstimate < float64(d.Config.MediumWindow)
	severity := "normal"
	if halfLifeEstimate < float64(d.Config.ShortWindow) {
		severity = "critical"
	} else if isDecaying {
		severity = "active"
	}

	return DecayDetectionResult{
		Type:          DecayHalfLife,
		IsDecaying:    isDecaying,
		Severity:      severity,
		CurrentValue:  halfLifeEstimate,
		HistoricalAvg: float64(d.Config.LongWindow),
		ChangePct:     -relativeError,
		WindowDays:    len(returns),
		Confidence:    math.Min(1.0, forecastError/(math.Abs(actualSecond)+0.001)),
		Description:   fmt.Sprintf("估计半衰期 %.0f 天, 预测误差 %.1f%% [%s]",
			halfLifeEstimate, relativeError*100, severity),
		DetectedAt:    time.Now(),
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// classifyDecaySeverity 分类衰减严重度
func (d *DecayDetector) classifyDecaySeverity(changePct, threshold float64) string {
	ratio := 0.0
	if threshold != 0 {
		ratio = changePct / threshold
	}

	switch {
	case ratio < 2:
		return "critical"
	case ratio < 1.5:
		return "active"
	case ratio < 1:
		return "early"
	default:
		return "normal"
	}
}

// decayConfidence 基于样本量计算置信度
func decayConfidence(n1, n2 int) float64 {
	minN := math.Min(float64(n1), float64(n2))
	// 置信度 = sqrt(n / 50), 上限 1.0
	conf := math.Sqrt(minN / 50.0)
	if conf > 1 {
		conf = 1
	}
	return conf
}

// lastWindow 取最后 n 个元素
func lastWindow(data []float64, n int) []float64 {
	if len(data) <= n {
		return data
	}
	return data[len(data)-n:]
}

// annualizedSharpe 年化夏普比率
// 假设日收益率, 年化因子 sqrt(252)
func annualizedSharpe(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	m := mean(returns)
	sd := standardDeviation(returns)
	if sd == 0 {
		return 0
	}
	return m / sd * math.Sqrt(252)
}

// maxDrawdown 计算最大回撤
func maxDrawdown(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	// 计算累计收益
	cumulative := make([]float64, len(returns))
	cumulative[0] = 1 + returns[0]
	for i := 1; i < len(returns); i++ {
		cumulative[i] = cumulative[i-1] * (1 + returns[i])
	}

	// 计算从峰值的最大回撤
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

// sortFloats 排序辅助
func sortFloats(data []float64) []float64 {
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)
	return sorted
}
