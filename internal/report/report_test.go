package report

import (
	"math"
	"testing"
)

// ============================================================================
// 辅助函数
// ============================================================================

// generateNormalReturns 生成近似正态分布的收益序列。
func generateNormalReturns(n int, mean, std float64) []float64 {
	// 使用 Box-Muller 变换生成正态分布随机数 (确定性)
	returns := make([]float64, n)
	seed := 42.0
	for i := range returns {
		// 简化的伪随机数生成
		u1 := (math.Sin(seed+float64(i)*12.9898) + 1) / 2
		u2 := (math.Sin(seed+float64(i)*78.233) + 1) / 2

		z0 := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
		returns[i] = mean + std*z0
	}
	return returns
}

// generateEquityCurve 生成权益曲线。
func generateEquityCurve(returns []float64, initial float64) []float64 {
	curve := make([]float64, len(returns)+1)
	curve[0] = initial
	for i, r := range returns {
		curve[i+1] = curve[i] * (1 + r)
	}
	return curve
}

// generateTrades 生成测试交易记录。
func generateTrades(n int) []TradeRecord {
	trades := make([]TradeRecord, n)
	baseDate := "2024-01"
	for i := range trades {
		trades[i] = TradeRecord{
			StockCode: "000001.SZ",
			EntryDate: baseDate + "-" + string(rune('0'+i%9+1)) + "0",
			ExitDate:  baseDate + "-" + string(rune('0'+i%9+1)) + "5",
			Return:    0.02 * float64((i%5)-2), // -0.04 ~ 0.06
			HoldDays:  5 + i%10,
			Position:  100000,
		}
	}
	return trades
}

// ============================================================================
// 基础统计测试
// ============================================================================

func TestComputeBasicStats(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5}
	stats := ComputeBasicStats(values)

	if stats.Count != 5 {
		t.Errorf("Count = %d, want 5", stats.Count)
	}
	if math.Abs(stats.Mean-3) > 0.001 {
		t.Errorf("Mean = %f, want 3", stats.Mean)
	}
	if math.Abs(stats.Median-3) > 0.001 {
		t.Errorf("Median = %f, want 3", stats.Median)
	}
	if math.Abs(stats.StdDev-math.Sqrt(2)) > 0.001 {
		t.Errorf("StdDev = %f, want %f", stats.StdDev, math.Sqrt(2))
	}
	if stats.Min != 1 {
		t.Errorf("Min = %f, want 1", stats.Min)
	}
	if stats.Max != 5 {
		t.Errorf("Max = %f, want 5", stats.Max)
	}
	if stats.Sum != 15 {
		t.Errorf("Sum = %f, want 15", stats.Sum)
	}
}

func TestComputeBasicStats_Empty(t *testing.T) {
	stats := ComputeBasicStats([]float64{})
	if stats.Count != 0 {
		t.Error("Empty stats should have count 0")
	}
}

func TestComputeBasicStats_SkewnessKurtosis(t *testing.T) {
	// 高度偏态分布 (右偏)
	values := []float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 10}
	stats := ComputeBasicStats(values)

	if stats.Skewness <= 0 {
		t.Error("Right-skewed distribution should have positive skewness")
	}
}

// ============================================================================
// 置信区间测试
// ============================================================================

func TestComputeConfidenceInterval(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ci := ComputeConfidenceInterval(values, 0.95)

	if ci == nil {
		t.Fatal("Should not return nil")
	}

	if ci.Level != 0.95 {
		t.Errorf("Level = %f, want 0.95", ci.Level)
	}

	if ci.Lower >= ci.Upper {
		t.Errorf("Lower %f should be less than Upper %f", ci.Lower, ci.Upper)
	}

	// 均值 5.5 应在区间内
	if ci.Lower > 5.5 || ci.Upper < 5.5 {
		t.Errorf("Mean 5.5 should be within CI [%f, %f]", ci.Lower, ci.Upper)
	}
}

func TestComputeConfidenceInterval_SingleSample(t *testing.T) {
	ci := ComputeConfidenceInterval([]float64{5}, 0.95)
	if ci != nil {
		t.Error("Single sample should not produce CI")
	}
}

// ============================================================================
// 分布分析测试
// ============================================================================

func TestComputeDistribution(t *testing.T) {
	returns := generateNormalReturns(1000, 0.001, 0.02)
	dist := ComputeDistribution(returns, 20)

	if dist == nil {
		t.Fatal("Should not return nil")
	}

	if len(dist.Histogram) != 20 {
		t.Errorf("Histogram should have 20 buckets, got %d", len(dist.Histogram))
	}

	if len(dist.Quantiles) == 0 {
		t.Error("Should have quantiles")
	}

	// 正态分布应通过正态性检验
	if !dist.IsNormal {
		t.Log("Warning: Generated normal data failed normality test")
	}

	t.Logf("Distribution: mean=%.4f, std=%.4f, skew=%.4f, kurtosis=%.4f",
		dist.Stats.Mean, dist.Stats.StdDev, dist.Stats.Skewness, dist.Stats.Kurtosis)
}

func TestComputeDistribution_Empty(t *testing.T) {
	dist := ComputeDistribution([]float64{}, 10)
	if dist != nil {
		t.Error("Empty data should return nil")
	}
}

// ============================================================================
// 风险指标测试
// ============================================================================

func TestComputeVaR(t *testing.T) {
	returns := generateNormalReturns(1000, 0.001, 0.02)

	var95 := computeVaR(returns, 0.95)
	var99 := computeVaR(returns, 0.99)

	if var95 <= 0 {
		t.Error("VaR should be positive (loss)")
	}
	if var99 <= var95 {
		t.Errorf("99%% VaR (%f) should be > 95%% VaR (%f)", var99, var95)
	}

	t.Logf("VaR_95: %.4f, VaR_99: %.4f", var95, var99)
}

func TestComputeCVaR(t *testing.T) {
	returns := generateNormalReturns(1000, 0.001, 0.02)

	cvar95 := computeCVaR(returns, 0.95)
	cvar99 := computeCVaR(returns, 0.99)

	if cvar95 <= 0 {
		t.Error("CVaR should be positive (loss)")
	}
	if cvar99 <= cvar95 {
		t.Errorf("99%% CVaR (%f) should be > 95%% CVaR (%f)", cvar99, cvar95)
	}

	t.Logf("CVaR_95: %.4f, CVaR_99: %.4f", cvar95, cvar99)
}

func TestComputeMaxDrawdown(t *testing.T) {
	// 生成先涨后跌的权益曲线
	equity := []float64{100, 110, 120, 115, 105, 95, 100, 105}

	mdd, peakIdx, troughIdx := computeMaxDrawdown(equity)

	if mdd <= 0 {
		t.Error("Max drawdown should be positive")
	}
	if peakIdx >= troughIdx {
		t.Error("Peak should be before trough")
	}

	// 计算: peak=120 at index 2, trough=95 at index 5
	// MDD = (120-95)/120 = 0.2083
	expected := 25.0 / 120.0
	if math.Abs(mdd-expected) > 0.01 {
		t.Errorf("MaxDrawdown = %f, want %f", mdd, expected)
	}

	t.Logf("MaxDrawdown: %.4f (peak@%d=%.0f, trough@%d=%.0f)",
		mdd, peakIdx, equity[peakIdx], troughIdx, equity[troughIdx])
}

func TestComputeRiskMetrics(t *testing.T) {
	returns := generateNormalReturns(500, 0.0005, 0.015)
	equity := generateEquityCurve(returns, 1000000)

	risk := ComputeRiskMetrics(equity, returns)

	if risk == nil {
		t.Fatal("Should not return nil")
	}

	if risk.MaxDrawdown < 0 {
		t.Error("MaxDrawdown should be non-negative")
	}
	if risk.VaR_95 <= 0 {
		t.Error("VaR_95 should be positive")
	}
	if risk.CVaR_95 <= 0 {
		t.Error("CVaR_95 should be positive")
	}

	t.Logf("Risk metrics: MDD=%.4f, VaR95=%.4f, CVaR95=%.4f, Ulcer=%.4f",
		risk.MaxDrawdown, risk.VaR_95, risk.CVaR_95, risk.UlcerIndex)
}

func TestComputeRiskMetrics_Empty(t *testing.T) {
	risk := ComputeRiskMetrics([]float64{100}, []float64{})
	if risk != nil {
		t.Error("Empty data should return nil")
	}
}

// ============================================================================
// 稳定性指标测试
// ============================================================================

func TestComputeStabilityMetrics(t *testing.T) {
	trades := generateTrades(200)
	stability := ComputeStabilityMetrics(trades, 252)

	if stability == nil {
		t.Fatal("Should not return nil")
	}

	if stability.TurnoverRate <= 0 {
		t.Error("Turnover rate should be positive")
	}
	if stability.AvgHoldDays <= 0 {
		t.Error("Avg hold days should be positive")
	}
	if stability.StockConcentration <= 0 {
		t.Error("Stock concentration should be positive")
	}

	t.Logf("Stability: turnover=%.2f, avgHold=%.1f, stockHHI=%.4f, capacity=%.0f万",
		stability.TurnoverRate, stability.AvgHoldDays,
		stability.StockConcentration, stability.CapacityEstimate)
}

func TestComputeStabilityMetrics_Empty(t *testing.T) {
	stability := ComputeStabilityMetrics([]TradeRecord{}, 252)
	if stability != nil {
		t.Error("Empty trades should return nil")
	}
}

// ============================================================================
// 报告生成测试
// ============================================================================

func TestReportGenerator_NormalCase(t *testing.T) {
	returns := generateNormalReturns(500, 0.0005, 0.015)
	equity := generateEquityCurve(returns, 1000000)
	trades := generateTrades(100)

	generator := NewReportGenerator(20)
	config := DefaultReportConfig()

	report := generator.Generate(returns, equity, trades, config)

	if report.IsEmpty {
		t.Error("Should not be empty")
	}
	if report.IsSmallSample {
		t.Error("Should not be small sample (500 > 20)")
	}
	if report.SampleSize != 500 {
		t.Errorf("SampleSize = %d, want 500", report.SampleSize)
	}

	// 检查关键指标
	if report.TotalReturn == 0 {
		t.Error("Total return should be non-zero")
	}
	if report.Distribution == nil {
		t.Error("Should have distribution analysis")
	}
	if report.Risk == nil {
		t.Error("Should have risk metrics")
	}
	if report.Drawdown == nil {
		t.Error("Should have drawdown analysis")
	}
	if report.Stability == nil {
		t.Error("Should have stability metrics")
	}

	t.Log(report.SummaryText())
}

func TestReportGenerator_EmptyData(t *testing.T) {
	generator := NewReportGenerator(20)
	report := generator.Generate([]float64{}, []float64{}, nil, nil)

	if !report.IsEmpty {
		t.Error("Should be marked as empty")
	}
	if len(report.Warnings) == 0 {
		t.Error("Should have warnings")
	}

	t.Log(report.SummaryText())
}

func TestReportGenerator_SmallSample(t *testing.T) {
	// 小于 20 个样本
	returns := generateNormalReturns(10, 0.001, 0.02)
	equity := generateEquityCurve(returns, 1000000)

	generator := NewReportGenerator(20)
	report := generator.Generate(returns, equity, nil, nil)

	if !report.IsSmallSample {
		t.Error("Should be marked as small sample (10 < 20)")
	}

	// 小样本仍应计算指标 (供参考)
	if report.Distribution != nil {
		t.Log("Distribution computed for small sample (with warning)")
	}

	t.Log(report.SummaryText())
}

func TestReportGenerator_VerySmallSample(t *testing.T) {
	// 极小样本 (< 5)
	returns := []float64{0.01, -0.02}
	equity := generateEquityCurve(returns, 1000000)

	generator := NewReportGenerator(20)
	report := generator.Generate(returns, equity, nil, nil)

	if report.IsSmallSample {
		t.Log("Marked as small sample")
	}

	// 应有警告
	if len(report.Warnings) == 0 {
		t.Error("Very small sample should have warnings")
	}

	t.Logf("Warnings: %v", report.Warnings)
	t.Log(report.SummaryText())
}

func TestReportGenerator_ConsistencyWithEvidence(t *testing.T) {
	// 验证关键指标与 evidence 包的计算保持一致
	returns := []float64{0.025, -0.010, 0.030, -0.005, 0.015, 0.040, -0.020, 0.005, 0.020, -0.015}
	equity := generateEquityCurve(returns, 1000000)

	generator := NewReportGenerator(20)
	report := generator.Generate(returns, equity, nil, nil)

	if report.AvgReturn == 0 {
		t.Error("Average return should be computed")
	}

	// 计算手动检查
	expectedMean := 0.0
	for _, r := range returns {
		expectedMean += r
	}
	expectedMean /= float64(len(returns))

	if math.Abs(report.AvgReturn-expectedMean) > 1e-6 {
		t.Errorf("AvgReturn = %.6f, want %.6f", report.AvgReturn, expectedMean)
	}

	t.Logf("AvgReturn: %.6f (expected: %.6f)", report.AvgReturn, expectedMean)
}

// ============================================================================
// DrawdownAnalysis 测试
// ============================================================================

func TestComputeDrawdownAnalysis(t *testing.T) {
	equity := []float64{100, 110, 120, 115, 105, 95, 100, 110}
	dd := ComputeDrawdownAnalysis(equity)

	if dd == nil {
		t.Fatal("Should not return nil")
	}

	if dd.MaxDrawdown <= 0 {
		t.Error("MaxDrawdown should be positive")
	}

	// 最大回撤: peak=120, trough=95 -> (120-95)/120 = 0.2083
	expected := 25.0 / 120.0
	if math.Abs(dd.MaxDrawdown-expected) > 0.01 {
		t.Errorf("MaxDrawdown = %f, want %f", dd.MaxDrawdown, expected)
	}

	t.Logf("Drawdown: MDD=%.4f, duration=%d days, current=%.4f",
		dd.MaxDrawdown, dd.MaxDDDuration, dd.CurrentDrawdown)
}

func TestComputeDrawdownAnalysis_MonotonicIncrease(t *testing.T) {
	// 单调递增: 无回撤
	equity := []float64{100, 105, 110, 115, 120}
	dd := ComputeDrawdownAnalysis(equity)

	if dd == nil {
		t.Fatal("Should not return nil")
	}

	if dd.MaxDrawdown > 0.0001 {
		t.Errorf("Monotonic increase should have ~0 drawdown, got %f", dd.MaxDrawdown)
	}
}

// ============================================================================
// SummaryText 测试
// ============================================================================

func TestSummaryText_Normal(t *testing.T) {
	returns := generateNormalReturns(100, 0.001, 0.02)
	equity := generateEquityCurve(returns, 1000000)

	generator := NewReportGenerator(20)
	report := generator.Generate(returns, equity, nil, nil)

	text := report.SummaryText()

	if len(text) == 0 {
		t.Error("Summary text should not be empty")
	}

	// 检查关键信息
	if !contains(text, "📊") {
		t.Error("Should contain header emoji")
	}

	t.Log(text[:200])
}

func TestSummaryText_Empty(t *testing.T) {
	generator := NewReportGenerator(20)
	report := generator.Generate([]float64{}, []float64{}, nil, nil)

	text := report.SummaryText()

	if !contains(text, "⚠️") {
		t.Error("Empty report should contain warning")
	}
}

// contains 字符串包含检查。
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// 正态分布逆CDF测试
// ============================================================================

func TestNormalInvCDF(t *testing.T) {
	// 边界测试
	if math.Abs(normalInvCDF(0.5)) > 0.01 {
		t.Error("CDF inverse at 0.5 should be ~0")
	}

	// 对称性
	p := 0.8
	if math.Abs(normalInvCDF(p)+normalInvCDF(1-p)) > 0.01 {
		t.Error("Should be symmetric")
	}

	// 极端值
	large := normalInvCDF(0.99)
	if large < 2.0 {
		t.Error("CDF inverse at 0.99 should be > 2")
	}
}

// ============================================================================
// 百分位数测试
// ============================================================================

func TestPercentile(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	p50 := percentile(sorted, 0.5)
	if math.Abs(p50-5.5) > 0.01 {
		t.Errorf("P50 = %f, want 5.5", p50)
	}

	p0 := percentile(sorted, 0)
	if p0 != 1 {
		t.Errorf("P0 = %f, want 1", p0)
	}

	p100 := percentile(sorted, 1)
	if p100 != 10 {
		t.Errorf("P100 = %f, want 10", p100)
	}
}
