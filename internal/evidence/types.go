// Package evidence defines the admission standards and evidence requirements
// for promoting a trading paradigm from candidate to validated status.
package evidence

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// HoldingPeriodType groups strategy holding periods into coarse categories.
type HoldingPeriodType string

const (
	HoldingIntraday  HoldingPeriodType = "intraday"
	HoldingShort     HoldingPeriodType = "short"   // 1-5 days
	HoldingMedium    HoldingPeriodType = "medium"  // 5-20 days
	HoldingLong      HoldingPeriodType = "long"    // 20+ days
	HoldingUndefined HoldingPeriodType = "undefined"
)

// HoldingPeriodToType maps a holding-period label to its coarse category.
// It accepts both English and Chinese labels used in the codebase.
func HoldingPeriodToType(hp string) HoldingPeriodType {
	switch hp {
	case "intraday", "日内", "t+0":
		return HoldingIntraday
	case "short", "1-5", "1-5天":
		return HoldingShort
	case "medium", "5-20", "5-20天":
		return HoldingMedium
	case "long", "20+", "20+天":
		return HoldingLong
	default:
		return HoldingUndefined
	}
}

// HoldingPeriodToMinSample maps a holding-period type to its minimum required sample size.
func HoldingPeriodToMinSample(hp HoldingPeriodType) int {
	switch hp {
	case HoldingIntraday:
		return 100
	case HoldingShort:
		return 50
	case HoldingMedium:
		return 30
	case HoldingLong:
		return 20
	default:
		return 30
	}
}

// HoldingPeriodToMaxDrawdown maps a holding-period type to its maximum allowed drawdown in percent.
func HoldingPeriodToMaxDrawdown(hp HoldingPeriodType) float64 {
	switch hp {
	case HoldingIntraday:
		return 5.0
	case HoldingShort:
		return 10.0
	case HoldingMedium:
		return 15.0
	case HoldingLong:
		return 20.0
	default:
		return 15.0
	}
}

// MarketRegime describes the market condition during a sub-period.
type MarketRegime string

const (
	RegimeBull      MarketRegime = "bull"
	RegimeBear      MarketRegime = "bear"
	RegimeRange     MarketRegime = "range"
	RegimeVolatile  MarketRegime = "volatile"
	RegimeUnknown   MarketRegime = "unknown"
)

// Metrics holds the mandatory evidence metrics for a paradigm.
type Metrics struct {
	SampleSize         int        `json:"sample_size"`          // total number of completed trades
	HoldingPeriod      string     `json:"holding_period"`       // raw label, e.g. "short"
	GrossReturn        float64    `json:"gross_return"`         // total gross return (%)
	NetReturn          float64    `json:"net_return"`           // post-cost expected return (%)
	AvgReturn          float64    `json:"avg_return"`           // average return per trade (%)
	MedianReturn       float64    `json:"median_return"`        // median return per trade (%)
	MaxDrawdown        float64    `json:"max_drawdown"`         // maximum drawdown (%)
	StdDev             float64    `json:"std_dev"`              // return standard deviation
	WinRate            float64    `json:"win_rate"`             // win rate (%)
	ConfidenceInterval [2]float64 `json:"confidence_interval"`  // 95% CI [lower, upper]
	ConfidenceLevel    float64    `json:"confidence_level"`     // e.g. 0.95
	Top20Concentration float64    `json:"top20_concentration"`  // % of returns contributed by top 20%
	Top10Concentration float64    `json:"top10_concentration"`  // % of returns contributed by top 10%
	ParamSensitivity   float64    `json:"param_sensitivity"`    // sensitivity coefficient
	RiskRewardRatio    float64    `json:"risk_reward_ratio"`    // avg return / |max drawdown|
	SharpeRatio        float64    `json:"sharpe_ratio"`         // annualised if possible
}

// WindowResult records performance over an independent rolling window.
type WindowResult struct {
	WindowID    string  `json:"window_id"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
	SampleSize  int     `json:"sample_size"`
	NetReturn   float64 `json:"net_return"`
	WinRate     float64 `json:"win_rate"`
	MaxDrawdown float64 `json:"max_drawdown"`
}

// RegimeResult records performance within a market regime.
type RegimeResult struct {
	Regime     string  `json:"regime"`
	SampleSize int     `json:"sample_size"`
	NetReturn  float64 `json:"net_return"`
	WinRate    float64 `json:"win_rate"`
	Count      int     `json:"count"`
}

// Evidence is the full evidence report for a paradigm.
type Evidence struct {
	ParadigmID  string          `json:"paradigm_id"`
	GeneratedAt time.Time       `json:"generated_at"`
	DataVersion string          `json:"data_version"`
	Metrics     Metrics         `json:"metrics"`
	Windows     []WindowResult  `json:"windows"`
	Regimes     []RegimeResult  `json:"regimes"`

	// Red flags that block promotion regardless of metrics.
	HasFutureData      bool `json:"has_future_data"`
	HasSurvivorshipBias bool `json:"has_survivorship_bias"`
	SelectiveReporting bool `json:"selective_reporting"`
	Overfitting        bool `json:"overfitting"`
}

// AdmissionLevel is the classification assigned after evaluation.
type AdmissionLevel string

const (
	LevelPlatinum AdmissionLevel = "platinum"
	LevelGold     AdmissionLevel = "gold"
	LevelSilver   AdmissionLevel = "silver"
	LevelBronze   AdmissionLevel = "bronze"
)

// AdmissionResult is the outcome of an admission check.
type AdmissionResult struct {
	Eligible    bool             `json:"eligible"`
	Level       AdmissionLevel   `json:"level"`
	Score       float64          `json:"score"`
	Reasons     []string         `json:"reasons"`
	MustFix     []string         `json:"must_fix"`
	Warnings    []string         `json:"warnings"`
	Suggestions []string         `json:"suggestions"`
}

// HoldingPeriodType returns the coarse holding-period type from metrics.
func (m Metrics) HoldingPeriodType() HoldingPeriodType {
	return HoldingPeriodToType(m.HoldingPeriod)
}

// MinSampleSize returns the minimum required sample size for the metrics.
func (m Metrics) MinSampleSize() int {
	return HoldingPeriodToMinSample(m.HoldingPeriodType())
}

// MaxDrawdownLimit returns the maximum allowed drawdown for the metrics.
func (m Metrics) MaxDrawdownLimit() float64 {
	return HoldingPeriodToMaxDrawdown(m.HoldingPeriodType())
}

// RequiredRiskRewardRatio returns the minimum risk-reward ratio for the metrics.
func (m Metrics) RequiredRiskRewardRatio() float64 {
	switch m.HoldingPeriodType() {
	case HoldingIntraday:
		return 0.5
	case HoldingShort:
		return 0.8
	case HoldingMedium:
		return 1.0
	case HoldingLong:
		return 1.2
	default:
		return 1.0
	}
}

// CheckAdmission evaluates the evidence against the admission rules.
func (e *Evidence) CheckAdmission() *AdmissionResult {
	var blockers, mustFix, warnings, suggestions []string
	score := 100.0

	// Red flags: any blocker makes the paradigm ineligible.
	if e.HasFutureData {
		blockers = append(blockers, "禁止使用未来数据")
	}
	if e.HasSurvivorshipBias {
		blockers = append(blockers, "存在幸存者偏差")
	}
	if e.SelectiveReporting {
		blockers = append(blockers, "存在选择性报告")
	}
	if e.Overfitting {
		blockers = append(blockers, "存在过拟合嫌疑")
	}

	m := e.Metrics

	// Net return must be positive after costs.
	if m.NetReturn <= 0 {
		mustFix = append(mustFix, "成本后期望收益必须大于0")
		score -= 30
	}

	// Sample size requirement.
	if m.SampleSize < m.MinSampleSize() {
		mustFix = append(mustFix, fmt.Sprintf("样本量不足：%d < %d", m.SampleSize, m.MinSampleSize()))
		score -= 20
	}

	// Confidence interval lower bound must be positive.
	if m.ConfidenceLevel <= 0 {
		m.ConfidenceLevel = 0.95
	}
	if m.ConfidenceInterval[0] <= 0 {
		mustFix = append(mustFix, "95%置信区间下界必须大于0")
		score -= 25
	}

	// Max drawdown must be within limit.
	if m.MaxDrawdown > m.MaxDrawdownLimit() {
		mustFix = append(mustFix, fmt.Sprintf("最大回撤超过限制：%.2f%% > %.2f%%", m.MaxDrawdown, m.MaxDrawdownLimit()))
		score -= 15
	}

	// Risk-reward ratio.
	if m.RiskRewardRatio < m.RequiredRiskRewardRatio() {
		mustFix = append(mustFix, fmt.Sprintf("收益风险比低于要求：%.2f < %.2f", m.RiskRewardRatio, m.RequiredRiskRewardRatio()))
		score -= 15
	}

	// Rolling windows.
	if len(e.Windows) < 3 {
		mustFix = append(mustFix, "滚动窗口数量不足（至少3个）")
		score -= 20
	} else {
		positiveWindows := 0
		windowReturns := make([]float64, 0, len(e.Windows))
		for _, w := range e.Windows {
			if w.NetReturn > 0 {
				positiveWindows++
			}
			windowReturns = append(windowReturns, w.NetReturn)
		}
		positiveRatio := float64(positiveWindows) / float64(len(e.Windows))
		if positiveRatio < 0.7 {
			mustFix = append(mustFix, fmt.Sprintf("正收益窗口比例不足：%.0f%% < 70%%", positiveRatio*100))
			score -= 15
		}
		if std, mean := stdDev(windowReturns), mean(windowReturns); mean != 0 && std/mean > 1.5 {
			warnings = append(warnings, "滚动窗口收益波动较大")
			score -= 10
		}
	}

	// Market regimes.
	if len(e.Regimes) < 2 {
		mustFix = append(mustFix, "至少覆盖2种市场状态")
		score -= 15
	} else {
		positiveRegimes := 0
		for _, r := range e.Regimes {
			if r.NetReturn > 0 {
				positiveRegimes++
			}
		}
		if positiveRegimes == 0 {
			mustFix = append(mustFix, "所有市场状态下收益均为负")
			score -= 15
		} else if float64(positiveRegimes)/float64(len(e.Regimes)) < 0.5 {
			warnings = append(warnings, "超过半数市场状态收益为负")
			score -= 10
		}
	}

	// Return concentration.
	if m.Top20Concentration > 80 {
		mustFix = append(mustFix, fmt.Sprintf("收益集中度过高：Top20%%贡献%.0f%%收益", m.Top20Concentration))
		score -= 15
	} else if m.Top20Concentration > 60 {
		warnings = append(warnings, fmt.Sprintf("收益集中度偏高：Top20%%贡献%.0f%%收益", m.Top20Concentration))
		score -= 10
	}

	// Parameter sensitivity.
	if m.ParamSensitivity > 1.0 {
		mustFix = append(mustFix, fmt.Sprintf("参数敏感性过高：%.2f > 1.0", m.ParamSensitivity))
		score -= 15
	} else if m.ParamSensitivity > 0.5 {
		warnings = append(warnings, "参数敏感性偏高")
		score -= 5
	}

	// Win-rate-only warning.
	if m.WinRate > 60 && m.NetReturn <= 0 {
		warnings = append(warnings, "胜率高但收益非正，避免只看胜率")
		score -= 10
	}

	if score < 0 {
		score = 0
	}

	eligible := len(blockers) == 0 && len(mustFix) == 0

	// Determine admission level.
	level := LevelBronze
	if eligible {
		if score >= 90 {
			level = LevelPlatinum
		} else if score >= 75 {
			level = LevelGold
		} else if score >= 60 {
			level = LevelSilver
		}
	}

	if eligible && score >= 90 {
		suggestions = append(suggestions, "可考虑进入产品级验证")
	} else if eligible {
		suggestions = append(suggestions, "可进入模拟前向运行阶段")
	} else {
		suggestions = append(suggestions, "修复must_fix项后重新验证")
	}

	return &AdmissionResult{
		Eligible:    eligible,
		Level:       level,
		Score:       math.Round(score*100) / 100,
		Reasons:     warnings,
		MustFix:     append(blockers, mustFix...),
		Warnings:    warnings,
		Suggestions: suggestions,
	}
}

// ComputeMetrics derives the required evidence metrics from a raw return series.
// Costs are expressed as a fraction, e.g. 0.001 = 0.1% per trade side.
// holdingPeriod is the raw label used to determine thresholds.
func ComputeMetrics(returns []float64, holdingPeriod string, costPerTrade float64) Metrics {
	if len(returns) == 0 {
		return Metrics{HoldingPeriod: holdingPeriod}
	}

	netReturns := make([]float64, len(returns))
	for i, r := range returns {
		// Cost is applied to each side (entry + exit), so subtract twice.
		netReturns[i] = r - costPerTrade*2*100
	}

	meanNet := mean(netReturns)
	stdNet := stdDev(netReturns)
	medianNet := median(netReturns)

	grossReturn := sum(returns)
	netReturn := sum(netReturns)

	positive := 0
	for _, r := range netReturns {
		if r > 0 {
			positive++
		}
	}
	winRate := float64(positive) / float64(len(netReturns)) * 100

	ci := confidenceInterval(netReturns, 0.95)

	top20 := topNConcentration(netReturns, 0.2)
	top10 := topNConcentration(netReturns, 0.1)

	mdd := maxDrawdown(netReturns)

	var rr float64
	if mdd != 0 {
		rr = math.Abs(meanNet) / math.Abs(mdd) * 100
	}

	var sharpe float64
	if stdNet != 0 {
		sharpe = meanNet / stdNet
	}

	return Metrics{
		SampleSize:         len(returns),
		HoldingPeriod:      holdingPeriod,
		GrossReturn:        grossReturn,
		NetReturn:          netReturn,
		AvgReturn:          meanNet,
		MedianReturn:       medianNet,
		MaxDrawdown:        mdd,
		StdDev:             stdNet,
		WinRate:            winRate,
		ConfidenceInterval: ci,
		ConfidenceLevel:    0.95,
		Top20Concentration: top20,
		Top10Concentration: top10,
		RiskRewardRatio:    rr,
		SharpeRatio:        sharpe,
	}
}

// DefaultCostPerTrade returns a sensible default round-trip cost per trade in percent points.
// It uses: commission 0.025% (each side) + stamp tax 0.1% (sell) + slippage 0.05% per side.
// This returns the per-side cost as a fraction.
func DefaultCostPerTrade() float64 {
	// commission 0.025% + slippage 0.05% = 0.075% per side
	// stamp tax 0.1% only on sell is approximated by increasing the per-side cost slightly.
	return 0.0013
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var s float64
	for _, v := range values {
		s += v
	}
	return s / float64(len(values))
}

func stdDev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	m := mean(values)
	var s float64
	for _, v := range values {
		d := v - m
		s += d * d
	}
	return math.Sqrt(s / float64(len(values)))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func sum(values []float64) float64 {
	var s float64
	for _, v := range values {
		s += v
	}
	return s
}

// confidenceInterval returns a percentile-based bootstrap-style confidence interval.
// For small samples it uses the normal approximation via standard error.
func confidenceInterval(values []float64, level float64) [2]float64 {
	if len(values) == 0 {
		return [2]float64{0, 0}
	}
	m := mean(values)
	sd := stdDev(values)
	if sd == 0 {
		return [2]float64{m, m}
	}
	// Normal approximation confidence interval.
	alpha := 1 - level
	z := normalZ(1 - alpha/2)
	se := sd / math.Sqrt(float64(len(values)))
	return [2]float64{m - z*se, m + z*se}
}

// normalZ returns an approximate z-score for the given cumulative probability.
func normalZ(p float64) float64 {
	// Abramowitz and Stegun approximation for the inverse normal CDF.
	if p <= 0 || p >= 1 {
		return 0
	}
	// Use a lookup for the common 95% case for simplicity and stability.
	if math.Abs(p-0.975) < 1e-6 {
		return 1.96
	}
	// Fallback: rough approximation using error function inverse.
	return math.Sqrt(2) * inverseErf(2*p-1)
}

// inverseErf returns an approximation of the inverse error function for |x| < 1.
func inverseErf(x float64) float64 {
	// Hastings approximation.
	sign := 1.0
	if x < 0 {
		sign = -1.0
		x = -x
	}
	if x >= 1 {
		return 0
	}
	a := 0.147
	t := 2.0 / (math.Pi*a) + math.Log(1-x*x)/2.0
	u := math.Log(1-x*x)/a + t*t/4.0
	return sign * math.Sqrt(math.Sqrt(u) - t/2.0)
}

// topNConcentration returns the percentage of total return contributed by the top fraction of trades.
// fraction must be in (0,1].
func topNConcentration(returns []float64, fraction float64) float64 {
	if len(returns) == 0 || fraction <= 0 {
		return 0
	}
	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })

	total := sum(sorted)
	if total == 0 {
		return 0
	}
	n := int(math.Ceil(float64(len(sorted)) * fraction))
	if n > len(sorted) {
		n = len(sorted)
	}
	topSum := sum(sorted[:n])
	return topSum / total * 100
}

// maxDrawdown computes the maximum peak-to-trough drawdown as a percentage.
// values are returns in percent.
func maxDrawdown(returns []float64) float64 {
	var peak, maxDD float64
	cumulative := 0.0
	for _, r := range returns {
		cumulative += r
		if cumulative > peak {
			peak = cumulative
		}
		dd := peak - cumulative
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}
