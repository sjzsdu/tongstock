// Package report 统一收益、风险与稳定性分析报告生成。
//
// 该包与 evidence 包的指标标准保持一致，提供更丰富的分析维度：
//   - 收益分布: 偏度/峰度/分位数
//   - 尾部风险: VaR/CVaR/ES
//   - 最大回撤分析
//   - 稳定性指标: 换手率/容量/集中度
//   - 小样本/空样本安全处理
package report

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// ============================================================================
// 基础统计
// ============================================================================

// BasicStats 基础统计量。
type BasicStats struct {
	Count    int     `json:"count"`
	Mean     float64 `json:"mean"`
	Median   float64 `json:"median"`
	StdDev   float64 `json:"std_dev"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Skewness float64 `json:"skewness"`
	Kurtosis float64 `json:"kurtosis"`
	Sum      float64 `json:"sum"`
}

// DistributionAnalysis 收益分布分析。
type DistributionAnalysis struct {
	Stats     BasicStats         `json:"stats"`
	Histogram []Bucket           `json:"histogram,omitempty"`
	Quantiles map[string]float64 `json:"quantiles,omitempty"`
	IsNormal  bool               `json:"is_normal"` // 是否接近正态分布
}

// Bucket 直方图桶。
type Bucket struct {
	Range     [2]float64 `json:"range"`
	Count     int        `json:"count"`
	Frequency float64    `json:"frequency"`
}

// ConfidenceInterval 置信区间。
type ConfidenceInterval struct {
	Level  float64 `json:"level"`
	Lower  float64 `json:"lower"`
	Upper  float64 `json:"upper"`
	Mean   float64 `json:"mean"`
	StdErr float64 `json:"std_err"`
}

// ============================================================================
// 风险指标
// ============================================================================

// RiskMetrics 风险指标。
type RiskMetrics struct {
	MaxDrawdown         float64 `json:"max_drawdown"`               // 最大回撤
	MaxDrawdownDuration int     `json:"max_drawdown_duration_days"` // 最大回撤持续天数
	CurrentDrawdown     float64 `json:"current_drawdown"`           // 当前回撤
	VaR_95              float64 `json:"var_95"`                     // 95% VaR
	VaR_99              float64 `json:"var_99"`                     // 99% VaR
	CVaR_95             float64 `json:"cvar_95"`                    // 95% CVaR (Expected Shortfall)
	CVaR_99             float64 `json:"cvar_99"`                    // 99% CVaR
	DownsideDeviation   float64 `json:"downside_deviation"`         // 下行偏差
	UlcerIndex          float64 `json:"ulcer_index"`                // Ulcer Index (回撤深度和持续时间)
	TailRiskRatio       float64 `json:"tail_risk_ratio"`            // 尾部风险比率: 5%最坏/平均
}

// DrawdownAnalysis 最大回撤详细分析。
type DrawdownAnalysis struct {
	MaxDrawdown     float64    `json:"max_drawdown"`
	MaxDDStart      *time.Time `json:"max_dd_start,omitempty"`
	MaxDDEnd        *time.Time `json:"max_dd_end,omitempty"`
	MaxDDDuration   int        `json:"max_dd_duration_days"`
	AvgDrawdown     float64    `json:"avg_drawdown"`
	DrawdownFreq    float64    `json:"drawdown_frequency"` // 回撤频率
	RecoveryDays    []int      `json:"recovery_days,omitempty"`
	CurrentDrawdown float64    `json:"current_drawdown"`
	CurrentDDBDays  int        `json:"current_dd_days"`
}

// ============================================================================
// 稳定性指标
// ============================================================================

// StabilityMetrics 稳定性指标。
type StabilityMetrics struct {
	TurnoverRate       float64 `json:"turnover_rate"`       // 换手率
	AvgHoldDays        float64 `json:"avg_hold_days"`       // 平均持有天数
	TradeFrequency     float64 `json:"trade_frequency"`     // 交易频率 (天/笔)
	StockConcentration float64 `json:"stock_concentration"` // 股票集中度
	DateConcentration  float64 `json:"date_concentration"`  // 日期集中度
	TopPctContrib      float64 `json:"top_pct_contrib"`     // Top 贡献百分比
	CapacityEstimate   float64 `json:"capacity_estimate"`   // 容量估算 (万元)
	SharpeStability    float64 `json:"sharpe_stability"`    // 夏普稳定性 (滚动夏普的变异系数)
}

// ============================================================================
// 综合报告
// ============================================================================

// UnifiedReport 统一分析报告。
type UnifiedReport struct {
	GeneratedAt   time.Time `json:"generated_at"`
	SampleSize    int       `json:"sample_size"`
	IsEmpty       bool      `json:"is_empty"`        // 是否空样本
	IsSmallSample bool      `json:"is_small_sample"` // 是否小样本 (<20)

	// 收益指标
	TotalReturn  float64 `json:"total_return"`
	AnnualReturn float64 `json:"annual_return,omitempty"`
	AvgReturn    float64 `json:"avg_return"`
	WinRate      float64 `json:"win_rate"`
	ProfitFactor float64 `json:"profit_factor"`
	SharpeRatio  float64 `json:"sharpe_ratio,omitempty"`
	SortinoRatio float64 `json:"sortino_ratio,omitempty"`

	// 分布分析
	Distribution  *DistributionAnalysis `json:"distribution,omitempty"`
	ConfidenceInt *ConfidenceInterval   `json:"confidence_interval,omitempty"`

	// 风险分析
	Risk     *RiskMetrics      `json:"risk,omitempty"`
	Drawdown *DrawdownAnalysis `json:"drawdown,omitempty"`

	// 稳定性
	Stability *StabilityMetrics `json:"stability,omitempty"`

	// 警告信息
	Warnings []string `json:"warnings,omitempty"`
	Notes    []string `json:"notes,omitempty"`
}

// ============================================================================
// 计算函数
// ============================================================================

// ComputeBasicStats 计算基础统计量。
func ComputeBasicStats(values []float64) BasicStats {
	if len(values) == 0 {
		return BasicStats{}
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := float64(len(values))
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / n

	// 方差
	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= n
	stdDev := math.Sqrt(variance)

	// 中位数
	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 && len(sorted) > 1 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}

	// 偏度
	skewness := 0.0
	if stdDev > 0 && n > 2 {
		m3 := 0.0
		for _, v := range values {
			diff := (v - mean) / stdDev
			m3 += diff * diff * diff
		}
		skewness = m3 / n
	}

	// 峰度
	kurtosis := 0.0
	if stdDev > 0 && n > 3 {
		m4 := 0.0
		for _, v := range values {
			diff := (v - mean) / stdDev
			m4 += diff * diff * diff * diff
		}
		kurtosis = m4/n - 3 // 超额峰度
	}

	return BasicStats{
		Count:    len(values),
		Mean:     mean,
		Median:   median,
		StdDev:   stdDev,
		Min:      sorted[0],
		Max:      sorted[len(sorted)-1],
		Skewness: skewness,
		Kurtosis: kurtosis,
		Sum:      sum,
	}
}

// ComputeConfidenceInterval 计算置信区间 (t-distribution 近似)。
func ComputeConfidenceInterval(values []float64, confidence float64) *ConfidenceInterval {
	if len(values) < 2 {
		return nil
	}

	n := float64(len(values))
	stats := ComputeBasicStats(values)

	// 标准误
	stdErr := stats.StdDev / math.Sqrt(n)

	// t 值近似 (简化: 使用 z 值)
	z := normalInvCDF(1 - (1-confidence)/2)

	lower := stats.Mean - z*stdErr
	upper := stats.Mean + z*stdErr

	return &ConfidenceInterval{
		Level:  confidence,
		Lower:  lower,
		Upper:  upper,
		Mean:   stats.Mean,
		StdErr: stdErr,
	}
}

// ComputeDistribution 计算分布分析。
func ComputeDistribution(values []float64, numBuckets int) *DistributionAnalysis {
	if len(values) == 0 {
		return nil
	}

	stats := ComputeBasicStats(values)

	// 分位数
	quantiles := map[string]float64{}
	quantilePoints := []float64{0.05, 0.10, 0.25, 0.50, 0.75, 0.90, 0.95}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	for _, q := range quantilePoints {
		quantiles[fmt.Sprintf("p%.0f", q*100)] = percentile(sorted, q)
	}

	// 直方图
	if numBuckets <= 0 {
		numBuckets = 10
	}
	buckets := make([]Bucket, numBuckets)
	if stats.Max > stats.Min {
		bucketWidth := (stats.Max - stats.Min) / float64(numBuckets)
		counts := make([]int, numBuckets)

		for _, v := range values {
			idx := int((v - stats.Min) / bucketWidth)
			if idx >= numBuckets {
				idx = numBuckets - 1
			}
			counts[idx]++
		}

		for i := 0; i < numBuckets; i++ {
			buckets[i] = Bucket{
				Range: [2]float64{
					stats.Min + float64(i)*bucketWidth,
					stats.Min + float64(i+1)*bucketWidth,
				},
				Count:     counts[i],
				Frequency: float64(counts[i]) / float64(len(values)),
			}
		}
	}

	// 正态性检验 (简化: 基于偏度和峰度)
	isNormal := math.Abs(stats.Skewness) < 1.0 && math.Abs(stats.Kurtosis) < 3.0

	return &DistributionAnalysis{
		Stats:     stats,
		Histogram: buckets,
		Quantiles: quantiles,
		IsNormal:  isNormal,
	}
}

// ComputeRiskMetrics 计算风险指标。
func ComputeRiskMetrics(equityCurve []float64, dailyReturns []float64) *RiskMetrics {
	if len(equityCurve) < 2 || len(dailyReturns) == 0 {
		return nil
	}

	// 最大回撤
	mdd, _, _ := computeMaxDrawdown(equityCurve)

	// VaR
	var95 := computeVaR(dailyReturns, 0.95)
	var99 := computeVaR(dailyReturns, 0.99)

	// CVaR
	cvar95 := computeCVaR(dailyReturns, 0.95)
	cvar99 := computeCVaR(dailyReturns, 0.99)

	// 下行偏差
	downsideDev := computeDownsideDeviation(dailyReturns)

	// Ulcer Index
	ulcerIndex := computeUlcerIndex(equityCurve)

	// 尾部风险比率
	tailRatio := 0.0
	stats := ComputeBasicStats(dailyReturns)
	if stats.Mean != 0 {
		worstReturns := make([]float64, 0)
		sorted := make([]float64, len(dailyReturns))
		copy(sorted, dailyReturns)
		sort.Float64s(sorted)

		tailCount := int(math.Ceil(float64(len(dailyReturns)) * 0.05))
		if tailCount > 0 && len(sorted) > tailCount {
			worstReturns = sorted[:tailCount]
			avgWorst := sum(worstReturns) / float64(tailCount)
			if avgWorst != 0 {
				tailRatio = avgWorst / math.Abs(stats.Mean)
			}
		}
	}

	return &RiskMetrics{
		MaxDrawdown:       mdd,
		VaR_95:            var95,
		VaR_99:            var99,
		CVaR_95:           cvar95,
		CVaR_99:           cvar99,
		DownsideDeviation: downsideDev,
		UlcerIndex:        ulcerIndex,
		TailRiskRatio:     tailRatio,
	}
}

// ============================================================================
// 辅助计算函数
// ============================================================================

// computeMaxDrawdown 计算最大回撤。
func computeMaxDrawdown(equityCurve []float64) (float64, int, int) {
	if len(equityCurve) < 2 {
		return 0, 0, 0
	}

	peak := equityCurve[0]
	maxDD := 0.0
	peakIdx := 0
	troughIdx := 0

	for i := 1; i < len(equityCurve); i++ {
		if equityCurve[i] > peak {
			peak = equityCurve[i]
			peakIdx = i
		}
		dd := (peak - equityCurve[i]) / peak
		if dd > maxDD {
			maxDD = dd
			troughIdx = i
		}
	}

	return maxDD, peakIdx, troughIdx
}

// computeVaR 计算 Value at Risk (历史模拟法)。
func computeVaR(returns []float64, confidence float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Float64s(sorted)

	// VaR = -quantile (置信水平下的最大损失)
	idx := int((1 - confidence) * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	if idx < 0 {
		idx = 0
	}

	return -sorted[idx] // 转换为正数表示损失
}

// computeCVaR 计算条件 VaR (Expected Shortfall)。
func computeCVaR(returns []float64, confidence float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Float64s(sorted)

	idx := int((1 - confidence) * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	if idx < 0 {
		idx = 0
	}

	// CVaR = 平均超过 VaR 的损失
	tailReturns := sorted[:idx+1]
	if len(tailReturns) == 0 {
		return 0
	}

	avgLoss := 0.0
	for _, r := range tailReturns {
		avgLoss += math.Abs(r)
	}
	return avgLoss / float64(len(tailReturns))
}

// computeDownsideDeviation 计算下行偏差。
func computeDownsideDeviation(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	sum := 0.0
	for _, r := range returns {
		if r < 0 {
			sum += r * r
		}
	}
	return math.Sqrt(sum / float64(len(returns)))
}

// computeUlcerIndex 计算 Ulcer Index。
func computeUlcerIndex(equityCurve []float64) float64 {
	if len(equityCurve) < 2 {
		return 0
	}

	peak := equityCurve[0]
	sumDD := 0.0

	for i := 1; i < len(equityCurve); i++ {
		if equityCurve[i] > peak {
			peak = equityCurve[i]
		}
		dd := (peak - equityCurve[i]) / peak
		sumDD += dd * dd
	}

	return math.Sqrt(sumDD / float64(len(equityCurve)-1))
}

// computeDrawdownSeries 计算回撤序列。
func computeDrawdownSeries(equityCurve []float64) []float64 {
	if len(equityCurve) < 2 {
		return nil
	}

	peak := equityCurve[0]
	dds := make([]float64, len(equityCurve))
	dds[0] = 0

	for i := 1; i < len(equityCurve); i++ {
		if equityCurve[i] > peak {
			peak = equityCurve[i]
		}
		dds[i] = (peak - equityCurve[i]) / peak
	}

	return dds
}

// percentile 计算百分位数。
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return sorted[lower]
	}

	weight := idx - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// normalInvCDF 标准正态分布逆CDF (近似)。
func normalInvCDF(p float64) float64 {
	if p <= 0 || p >= 1 {
		return 0
	}

	// Rational approximation (Abramowitz and Stegun)
	if p < 0.5 {
		return -normalInvCDF(1 - p)
	}

	t := math.Sqrt(-2 * math.Log(1-p))
	c0 := 2.515517
	c1 := 0.802853
	c2 := 0.010328
	d1 := 1.432788
	d2 := 0.189269
	d3 := 0.001308

	return t - (c0+c1*t+c2*t*t)/(1+d1*t+d2*t*t+d3*t*t*t)
}

// sum 求和。
func sum(values []float64) float64 {
	s := 0.0
	for _, v := range values {
		s += v
	}
	return s
}
