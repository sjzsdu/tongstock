// Package monitoring 实现范式漂移、衰减与风险集中监控。
//
// 核心能力:
//   - drift.go: 分布漂移检测 (KS 检验、胜率漂移、均值漂移)
//   - decay.go: 性能衰减检测 (CUSUM、滚动夏普、半衰期)
//   - concentration.go: 风险集中监控 (赫芬达尔指数、行业/个股集中度)
//   - alert.go: 预警阈值与状态机
//   - monitor.go: 主监控引擎
package monitoring

import (
	"fmt"
	"math"
	"sort"
)

// ============================================================================
// Kolmogorov-Smirnov 检验
// ============================================================================

// KSResult KS 检验结果
type KSResult struct {
	Statistic   float64 `json:"statistic"`    // KS 统计量 D
	PValue      float64 `json:"p_value"`      // 近似 p-value
	SampleSize1 int     `json:"sample_size1"` // 第一组样本量
	SampleSize2 int     `json:"sample_size2"` // 第二组样本量
	IsSignificant bool  `json:"is_significant"` // 是否统计显著
	Alpha       float64 `json:"alpha"`        // 显著性水平
}

// KSTest 执行两样本 Kolmogorov-Smirnov 检验
// 比较 forward returns 与 baseline returns 的分布差异
func KSTest(sample1, sample2 []float64, alpha float64) KSResult {
	result := KSResult{
		SampleSize1: len(sample1),
		SampleSize2: len(sample2),
		Alpha:       alpha,
	}

	n1 := len(sample1)
	n2 := len(sample2)

	if n1 == 0 || n2 == 0 {
		result.Statistic = 0
		result.PValue = 1
		result.IsSignificant = false
		return result
	}

	// 排序两个样本
	sorted1 := make([]float64, n1)
	copy(sorted1, sample1)
	sort.Float64s(sorted1)

	sorted2 := make([]float64, n2)
	copy(sorted2, sample2)
	sort.Float64s(sorted2)

	// 计算所有唯一值作为检查点
	allValues := make([]float64, 0, n1+n2)
	allValues = append(allValues, sorted1...)
	allValues = append(allValues, sorted2...)
	sort.Float64s(allValues)

	// 去除重复值
	unique := make([]float64, 0, len(allValues))
	for i, v := range allValues {
		if i == 0 || v != allValues[i-1] {
			unique = append(unique, v)
		}
	}

	// 计算经验累积分布函数 (ECDF) 的最大差
	var maxDiff float64
	for _, x := range unique {
		ecdf1 := ecdf(sorted1, x)
		ecdf2 := ecdf(sorted2, x)
		diff := math.Abs(ecdf1 - ecdf2)
		if diff > maxDiff {
			maxDiff = diff
		}
	}

	result.Statistic = maxDiff

	// 近似 p-value 计算 (基于渐近公式)
	// lambda = (sqrt(n1*n2/(n1+n2)) + 0.12 + 0.11/sqrt(n1*n2/(n1+n2))) * maxDiff
	sqrtN := math.Sqrt(float64(n1*n2) / float64(n1+n2))
	lambda := (sqrtN + 0.12 + 0.11/sqrtN) * maxDiff

	// Kolmogorov 分布近似
	result.PValue = ksPValue(lambda)

	// 判断是否显著
	result.IsSignificant = result.PValue < alpha

	return result
}

// ecdf 计算经验累积分布函数
func ecdf(sorted []float64, x float64) float64 {
	count := 0
	for _, v := range sorted {
		if v <= x {
			count++
		} else {
			break
		}
	}
	return float64(count) / float64(len(sorted))
}

// ksPValue 近似计算 KS 检验的 p-value
func ksPValue(lambda float64) float64 {
	if lambda <= 0 {
		return 1
	}
	// 使用 Kolmogorov 近似公式
	sum := 0.0
	for k := 1; k <= 100; k++ {
		sum += math.Pow(-1, float64(k-1)) * math.Exp(-2*lambda*lambda*float64(k*k))
	}
	p := 2.0 * sum
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	return p
}

// ============================================================================
// 分布漂移检测
// ============================================================================

// DriftType 漂移类型
type DriftType string

const (
	DriftDistribution   DriftType = "distribution"   // 整体分布漂移
	DriftMean           DriftType = "mean"           // 均值漂移
	DriftWinRate        DriftType = "win_rate"       // 胜率漂移
	DriftVolatility     DriftType = "volatility"     // 波动率漂移
	DriftSkewness       DriftType = "skewness"       // 偏度漂移
	DriftKurtosis       DriftType = "kurtosis"       // 峰度漂移
	DriftCoverage       DriftType = "coverage"       // 数据覆盖率漂移
)

// DriftDetectionResult 漂移检测结果
type DriftDetectionResult struct {
	Type        DriftType   `json:"type"`
	Significant bool        `json:"significant"`
	Severity    string      `json:"severity"` // normal / mild / moderate / severe
	MetricName  string      `json:"metric_name"`
	OldValue    float64     `json:"old_value"`
	NewValue    float64     `json:"new_value"`
	Delta       float64     `json:"delta"`
	DeltaPct    float64     `json:"delta_pct"`
	PValue      float64     `json:"p_value"`
	Threshold   float64     `json:"threshold"`
	SampleSize  int         `json:"sample_size"`
	Description string      `json:"description"`
}

// DriftDetector 漂移检测器
type DriftDetector struct {
	// KS 检验参数
	KSSignificance float64 `json:"ks_significance"` // KS 检验显著性水平
	MinSampleSize  int     `json:"min_sample_size"`  // 最小样本量 (低于此值不触发结论)

	// 均值漂移阈值
	MeanDriftThreshold    float64 `json:"mean_drift_threshold"`    // 均值变化超过此比例触发预警
	WinRateDriftThreshold float64 `json:"win_rate_drift_threshold"` // 胜率变化阈值
	VolDriftThreshold     float64 `json:"vol_drift_threshold"`     // 波动率变化阈值

	// 严重度阈值
	MildThreshold     float64 `json:"mild_threshold"`     // 轻度: 偏离但尚未严重
	ModerateThreshold float64 `json:"moderate_threshold"` // 中度: 显著偏离
	SevereThreshold   float64 `json:"severe_threshold"`   // 严重: 高度异常
}

// NewDriftDetector 创建默认漂移检测器
func NewDriftDetector() *DriftDetector {
	return &DriftDetector{
		KSSignificance:        0.05,
		MinSampleSize:         20,
		MeanDriftThreshold:    0.20,  // 均值变化 > 20% 触发
		WinRateDriftThreshold: 0.15,  // 胜率变化 > 15pp 触发
		VolDriftThreshold:     0.30,  // 波动率变化 > 30% 触发
		MildThreshold:         0.10,
		ModerateThreshold:     0.25,
		SevereThreshold:       0.40,
	}
}

// DetectAll 执行全套漂移检测
// baseline: 历史基准收益序列
// forward: 前向实际收益序列
func (d *DriftDetector) DetectAll(baseline, forward []float64) []DriftDetectionResult {
	var results []DriftDetectionResult

	// 1. KS 分布漂移
	results = append(results, d.DetectDistributionDrift(baseline, forward)...)

	// 2. 均值漂移
	results = append(results, d.DetectMeanDrift(baseline, forward))

	// 3. 胜率漂移
	results = append(results, d.DetectWinRateDrift(baseline, forward))

	// 4. 波动率漂移
	results = append(results, d.DetectVolatilityDrift(baseline, forward))

	// 5. 偏度漂移
	results = append(results, d.DetectSkewnessDrift(baseline, forward))

	// 过滤掉因样本量不足而无法判断的结果
	filtered := make([]DriftDetectionResult, 0, len(results))
	for _, r := range results {
		if r.SampleSize >= d.MinSampleSize {
			filtered = append(filtered, r)
		} else {
			// 小样本: 标记为不显著, 不触发过度结论
			r.Significant = false
			r.Severity = "insufficient_data"
			r.Description = fmt.Sprintf("样本量 %d < %d, 结论仅供参考", r.SampleSize, d.MinSampleSize)
			filtered = append(filtered, r)
		}
	}

	return filtered
}

// DetectDistributionDrift 检测整体分布漂移
func (d *DriftDetector) DetectDistributionDrift(baseline, forward []float64) []DriftDetectionResult {
	results := make([]DriftDetectionResult, 0)

	ks := KSTest(baseline, forward, d.KSSignificance)
	severity := d.classifySeverity(ks.Statistic)

	results = append(results, DriftDetectionResult{
		Type:        DriftDistribution,
		Significant: ks.IsSignificant,
		Severity:    severity,
		MetricName:  "KS Statistic",
		OldValue:    ks.Statistic,
		NewValue:    ks.Statistic,
		Delta:       ks.Statistic,
		DeltaPct:    ks.Statistic,
		PValue:      ks.PValue,
		Threshold:   d.KSSignificance,
		SampleSize:  min(len(baseline), len(forward)),
		Description: d.describeKSResult(ks),
	})

	return results
}

// DetectMeanDrift 检测均值漂移
func (d *DriftDetector) DetectMeanDrift(baseline, forward []float64) DriftDetectionResult {
	baseMean := mean(baseline)
	fwdMean := mean(forward)
	deltaPct := relativeChange(baseMean, fwdMean)
	severity := d.classifySeverity(deltaPct)

	return DriftDetectionResult{
		Type:        DriftMean,
		Significant: math.Abs(deltaPct) > d.MeanDriftThreshold,
		Severity:    severity,
		MetricName:  "Expected Return",
		OldValue:    baseMean,
		NewValue:    fwdMean,
		Delta:       fwdMean - baseMean,
		DeltaPct:    deltaPct,
		PValue:      tTestPValue(baseline, forward),
		Threshold:   d.MeanDriftThreshold,
		SampleSize:  min(len(baseline), len(forward)),
		Description: d.describeMeanDrift(baseMean, fwdMean, deltaPct),
	}
}

// DetectWinRateDrift 检测胜率漂移
func (d *DriftDetector) DetectWinRateDrift(baseline, forward []float64) DriftDetectionResult {
	baseWR := winRate(baseline)
	fwdWR := winRate(forward)
	deltaPP := (fwdWR - baseWR) * 100 // 百分点差
	deltaPct := relativeChange(baseWR, fwdWR)
	severity := d.classifySeverity(math.Abs(deltaPct))

	significant := math.Abs(deltaPP) > d.WinRateDriftThreshold*100
	pValue := binomialTestPValue(baseWR, len(baseline), fwdWR, len(forward))

	return DriftDetectionResult{
		Type:        DriftWinRate,
		Significant: significant,
		Severity:    severity,
		MetricName:  "Win Rate",
		OldValue:    baseWR,
		NewValue:    fwdWR,
		Delta:       deltaPP,
		DeltaPct:    deltaPct,
		PValue:      pValue,
		Threshold:   d.WinRateDriftThreshold,
		SampleSize:  min(len(baseline), len(forward)),
		Description: d.describeWinRateDrift(baseWR, fwdWR, deltaPP),
	}
}

// DetectVolatilityDrift 检测波动率漂移
func (d *DriftDetector) DetectVolatilityDrift(baseline, forward []float64) DriftDetectionResult {
	baseVol := standardDeviation(baseline)
	fwdVol := standardDeviation(forward)
	deltaPct := relativeChange(baseVol, fwdVol)
	severity := d.classifySeverity(math.Abs(deltaPct))

	return DriftDetectionResult{
		Type:        DriftVolatility,
		Significant: math.Abs(deltaPct) > d.VolDriftThreshold,
		Severity:    severity,
		MetricName:  "Volatility",
		OldValue:    baseVol,
		NewValue:    fwdVol,
		Delta:       fwdVol - baseVol,
		DeltaPct:    deltaPct,
		PValue:      0.05, // 简化
		Threshold:   d.VolDriftThreshold,
		SampleSize:  min(len(baseline), len(forward)),
		Description: d.describeVolDrift(baseVol, fwdVol, deltaPct),
	}
}

// DetectSkewnessDrift 检测偏度漂移
func (d *DriftDetector) DetectSkewnessDrift(baseline, forward []float64) DriftDetectionResult {
	baseSkew := skewness(baseline)
	fwdSkew := skewness(forward)
	delta := fwdSkew - baseSkew
	deltaPct := relativeChange(math.Abs(baseSkew), math.Abs(fwdSkew))
	severity := d.classifySeverity(math.Abs(deltaPct))

	return DriftDetectionResult{
		Type:        DriftSkewness,
		Significant: math.Abs(delta) > 0.5,
		Severity:    severity,
		MetricName:  "Skewness",
		OldValue:    baseSkew,
		NewValue:    fwdSkew,
		Delta:       delta,
		DeltaPct:    deltaPct,
		PValue:      0.1,
		Threshold:   0.5,
		SampleSize:  min(len(baseline), len(forward)),
		Description: d.describeSkewnessDrift(baseSkew, fwdSkew),
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// classifySeverity 根据变化幅度分类严重度
func (d *DriftDetector) classifySeverity(changePct float64) string {
	absChange := math.Abs(changePct)
	switch {
	case absChange > d.SevereThreshold:
		return "severe"
	case absChange > d.ModerateThreshold:
		return "moderate"
	case absChange > d.MildThreshold:
		return "mild"
	default:
		return "normal"
	}
}

// describeKSResult 描述 KS 检验结果
func (d *DriftDetector) describeKSResult(ks KSResult) string {
	if !ks.IsSignificant {
		return fmt.Sprintf("KS=%.4f, p=%.4f: 前向分布与基准分布无显著差异", ks.Statistic, ks.PValue)
	}
	severity := d.classifySeverity(ks.Statistic)
	return fmt.Sprintf("KS=%.4f, p=%.4f [%s]: 前向分布与基准分布存在显著差异", ks.Statistic, ks.PValue, severity)
}

// describeMeanDrift 描述均值漂移
func (d *DriftDetector) describeMeanDrift(old, new, deltaPct float64) string {
	dir := "上升"
	if deltaPct < 0 {
		dir = "下降"
	}
	severity := d.classifySeverity(deltaPct)
	return fmt.Sprintf("期望收益 %s: 基准 %.4f → 前向 %.4f (%.1f%%) [%s]", dir, old, new, math.Abs(deltaPct)*100, severity)
}

// describeWinRateDrift 描述胜率漂移
func (d *DriftDetector) describeWinRateDrift(old, new, deltaPP float64) string {
	dir := "上升"
	if deltaPP < 0 {
		dir = "下降"
	}
	severity := d.classifySeverity(math.Abs(deltaPP) / 100)
	return fmt.Sprintf("胜率 %s: 基准 %.1f%% → 前向 %.1f%% (%.1fpp) [%s]", dir, old*100, new*100, math.Abs(deltaPP), severity)
}

// describeVolDrift 描述波动率漂移
func (d *DriftDetector) describeVolDrift(old, new, deltaPct float64) string {
	dir := "增加"
	if deltaPct < 0 {
		dir = "减少"
	}
	severity := d.classifySeverity(deltaPct)
	return fmt.Sprintf("波动率 %s: 基准 %.4f → 前向 %.4f (%.1f%%) [%s]", dir, old, new, math.Abs(deltaPct)*100, severity)
}

// describeSkewnessDrift 描述偏度漂移
func (d *DriftDetector) describeSkewnessDrift(old, new float64) string {
	oldLabel := "对称"
	if old > 0.5 {
		oldLabel = "右偏"
	} else if old < -0.5 {
		oldLabel = "左偏"
	}
	newLabel := "对称"
	if new > 0.5 {
		newLabel = "右偏"
	} else if new < -0.5 {
		newLabel = "左偏"
	}
	return fmt.Sprintf("偏度变化: 基准 %s(%.2f) → 前向 %s(%.2f)", oldLabel, old, newLabel, new)
}

// ============================================================================
// 统计辅助函数
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

func variance(data []float64) float64 {
	if len(data) < 2 {
		return 0
	}
	m := mean(data)
	var sum float64
	for _, v := range data {
		diff := v - m
		sum += diff * diff
	}
	return sum / float64(len(data)-1)
}

func standardDeviation(data []float64) float64 {
	return math.Sqrt(variance(data))
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

func skewness(data []float64) float64 {
	n := len(data)
	if n < 3 {
		return 0
	}
	m := mean(data)
	sd := standardDeviation(data)
	if sd == 0 {
		return 0
	}
	var sum float64
	for _, v := range data {
		sum += math.Pow((v-m)/sd, 3)
	}
	return float64(n) / (float64(n-1) * float64(n-2)) * sum
}

func relativeChange(old, new float64) float64 {
	if old == 0 {
		if new == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return (new - old) / math.Abs(old)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// tTestPValue 近似 t 检验 p-value
func tTestPValue(sample1, sample2 []float64) float64 {
	n1 := len(sample1)
	n2 := len(sample2)
	if n1 < 2 || n2 < 2 {
		return 1
	}
	m1 := mean(sample1)
	m2 := mean(sample2)
	v1 := variance(sample1)
	v2 := variance(sample2)

	se := math.Sqrt(v1/float64(n1) + v2/float64(n2))
	if se == 0 {
		return 1
	}
	tStat := math.Abs(m1 - m2) / se

	// 近似自由度
	df := v1/float64(n1) + v2/float64(n2)
	df = (df * df) / ((v1/float64(n1))*(v1/float64(n1))/float64(n1-1) + (v2/float64(n2))*(v2/float64(n2))/float64(n2-1))
	if df < 1 {
		df = 1
	}

	// 近似 p-value (使用指数衰减)
	pValue := math.Exp(-0.7 * tStat)
	if pValue < 0 {
		pValue = 0
	}
	if pValue > 1 {
		pValue = 1
	}
	return pValue
}

// binomialTestPValue 近似二项检验 p-value
func binomialTestPValue(p1 float64, n1 int, p2 float64, n2 int) float64 {
	if n1 < 5 || n2 < 5 {
		return 1
	}
	pooled := (p1*float64(n1) + p2*float64(n2)) / float64(n1+n2)
	se := math.Sqrt(pooled * (1 - pooled) * (1.0/float64(n1) + 1.0/float64(n2)))
	if se == 0 {
		return 1
	}
	z := math.Abs(p1 - p2) / se
	// 近似 p-value
	pValue := math.Exp(-0.5 * z * z)
	if pValue < 0 {
		pValue = 0
	}
	return pValue
}
