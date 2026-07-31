// Package monitoring - 监控模块测试
package monitoring

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"
)

// 全局随机源, 确保可复现
var testRng = rand.New(rand.NewSource(42))

// ============================================================================
// KS 检验测试
// ============================================================================

func TestKSTest_SameDistribution(t *testing.T) {
	// 相同分布的样本不应显著偏离
	sample1 := generateNormal(100, 0.01, 0.02)
	sample2 := generateNormal(100, 0.01, 0.02)

	result := KSTest(sample1, sample2, 0.05)
	if result.Statistic < 0 || result.Statistic > 1 {
		t.Errorf("KS statistic should be in [0,1], got %f", result.Statistic)
	}
	if result.PValue < 0 || result.PValue > 1 {
		t.Errorf("p-value should be in [0,1], got %f", result.PValue)
	}
	// 相同分布 p-value 应较大, 不应显著
	if result.IsSignificant {
		t.Log("Warning: same distribution detected as different (may happen with small samples)")
	}
}

func TestKSTest_DifferentDistribution(t *testing.T) {
	// 完全不同的分布应该显著偏离
	sample1 := generateNormal(200, 0.01, 0.02)
	sample2 := generateNormal(200, -0.05, 0.03)

	result := KSTest(sample1, sample2, 0.05)
	if !result.IsSignificant {
		t.Errorf("different distributions should be detected as significant, p=%.4f", result.PValue)
	}
}

func TestKSTest_EmptyInput(t *testing.T) {
	result := KSTest(nil, nil, 0.05)
	if result.Statistic != 0 {
		t.Errorf("empty input should give statistic 0, got %f", result.Statistic)
	}
	if result.PValue != 1 {
		t.Errorf("empty input should give p-value 1, got %f", result.PValue)
	}
}

func TestKSTest_SingleSample(t *testing.T) {
	sample := []float64{0.01}
	result := KSTest(sample, nil, 0.05)
	if result.Statistic != 0 {
		t.Errorf("one empty sample should give statistic 0")
	}
}

// ============================================================================
// 漂移检测测试
// ============================================================================

func TestDriftDetector_MeanDrift(t *testing.T) {
	detector := NewDriftDetector()

	baseline := generateNormal(100, 0.01, 0.02)
	forward := generateNormal(100, 0.02, 0.02) // 均值增加

	result := detector.DetectMeanDrift(baseline, forward)

	if result.Type != DriftMean {
		t.Errorf("expected type %s, got %s", DriftMean, result.Type)
	}
	if result.SampleSize != 100 {
		t.Errorf("expected sample size 100, got %d", result.SampleSize)
	}
	if result.MetricName != "Expected Return" {
		t.Errorf("unexpected metric name: %s", result.MetricName)
	}
}

func TestDriftDetector_WinRateDrift(t *testing.T) {
	detector := NewDriftDetector()

	// 高胜率基准: 大部分为正
	baseline := generateBiased(100, 0.7, 0.02)
	// 低胜率前向: 大部分为负
	forward := generateBiased(100, 0.3, 0.02)

	result := detector.DetectWinRateDrift(baseline, forward)

	if result.Type != DriftWinRate {
		t.Errorf("expected type %s, got %s", DriftWinRate, result.Type)
	}
	if result.OldValue <= result.NewValue {
		t.Errorf("win rate should decrease: baseline=%.3f, forward=%.3f", result.OldValue, result.NewValue)
	}
}

func TestDriftDetector_VolatilityDrift(t *testing.T) {
	detector := NewDriftDetector()

	// 低波动基准
	baseline := generateNormal(100, 0.01, 0.01)
	// 高波动前向
	forward := generateNormal(100, 0.01, 0.05)

	result := detector.DetectVolatilityDrift(baseline, forward)

	if result.Type != DriftVolatility {
		t.Errorf("expected type %s, got %s", DriftVolatility, result.Type)
	}
	if result.NewValue <= result.OldValue {
		t.Errorf("volatility should increase: baseline=%.4f, forward=%.4f", result.OldValue, result.NewValue)
	}
}

func TestDriftDetector_DetectAll(t *testing.T) {
	detector := NewDriftDetector()

	baseline := generateNormal(100, 0.01, 0.02)
	forward := generateNormal(100, 0.02, 0.025)

	results := detector.DetectAll(baseline, forward)

	expectedTypes := map[DriftType]bool{
		DriftDistribution: true,
		DriftMean:         true,
		DriftWinRate:      true,
		DriftVolatility:   true,
		DriftSkewness:     true,
	}

	for _, r := range results {
		if !expectedTypes[r.Type] {
			t.Errorf("unexpected drift type %s", r.Type)
		}
	}

	// 检查所有结果都有样本量
	for _, r := range results {
		if r.SampleSize < 0 {
			t.Errorf("negative sample size for type %s", r.Type)
		}
	}
}

func TestDriftDetector_InsufficientData(t *testing.T) {
	detector := NewDriftDetector()
	detector.MinSampleSize = 50

	baseline := generateNormal(10, 0.01, 0.02)
	forward := generateNormal(10, 0.01, 0.02)

	results := detector.DetectAll(baseline, forward)

	for _, r := range results {
		if r.Severity != "insufficient_data" {
			t.Errorf("expected insufficient_data severity, got %s", r.Severity)
		}
	}
}

// ============================================================================
// 衰减检测测试
// ============================================================================

func TestDecayDetector_SharpeDecline(t *testing.T) {
	config := NewDecayConfig()
	detector := NewDecayDetector(config)

	// 前期表现好, 后期表现差
	returns := []float64{}
	for i := 0; i < 60; i++ {
		if i < 40 {
			returns = append(returns, 0.01+randn()*0.02) // 正收益
		} else {
			returns = append(returns, -0.005+randn()*0.02) // 负收益
		}
	}

	result := detector.DetectSharpeDecline(returns)

	if result.Type != DecaySharpeDecline {
		t.Errorf("expected type %s, got %s", DecaySharpeDecline, result.Type)
	}
	if result.WindowDays != config.ShortWindow {
		t.Errorf("expected window %d, got %d", config.ShortWindow, result.WindowDays)
	}
	if result.CurrentValue == 0 && result.HistoricalAvg != 0 {
		t.Log("Short-window Sharpe is 0 due to insufficient data")
	}
}

func TestDecayDetector_WinRateDecay(t *testing.T) {
	config := NewDecayConfig()
	detector := NewDecayDetector(config)

	returns := []float64{}
	for i := 0; i < 60; i++ {
		if i < 40 {
			returns = append(returns, 0.01+randn()*0.02)
		} else {
			returns = append(returns, -0.005+randn()*0.02)
		}
	}

	result := detector.DetectWinRateDecay(returns)

	if result.Type != DecayWinRateDrop {
		t.Errorf("expected type %s, got %s", DecayWinRateDrop, result.Type)
	}
}

func TestDecayDetector_CUSUM(t *testing.T) {
	config := NewDecayConfig()
	detector := NewDecayDetector(config)

	// 持续偏移的数据
	returns := make([]float64, 100)
	for i := range returns {
		returns[i] = -0.002 + randn()*0.02 // 持续负偏移
	}

	result := detector.DetectCUSUM(returns)

	if result.Type != DecayCUSUM {
		t.Errorf("expected type %s, got %s", DecayCUSUM, result.Type)
	}
	if result.HistoricalAvg < 0 {
		t.Errorf("historical avg should not be negative")
	}
}

func TestDecayDetector_DetectAll(t *testing.T) {
	config := NewDecayConfig()
	detector := NewDecayDetector(config)

	returns := make([]float64, 100)
	dates := make([]time.Time, 100)
	for i := range returns {
		returns[i] = (0.01 - float64(i)*0.0002) + randn()*0.02
		dates[i] = time.Now().AddDate(0, 0, -100+i)
	}

	results := detector.DetectAll(returns, dates)

	if len(results) == 0 {
		t.Error("expected some decay results")
	}

	// 检查结果类型
	resultTypes := map[DecayType]bool{}
	for _, r := range results {
		resultTypes[r.Type] = true
	}

	expectedTypes := []DecayType{DecaySharpeDecline, DecayWinRateDrop, DecayDrawdownExpansion, DecayCUSUM, DecayHalfLife}
	for _, et := range expectedTypes {
		if !resultTypes[et] {
			t.Errorf("expected decay type %s not found", et)
		}
	}
}

func TestDecayDetector_InsufficientData(t *testing.T) {
	config := NewDecayConfig()
	config.MinWindowSize = 100
	detector := NewDecayDetector(config)

	returns := make([]float64, 5)
	dates := make([]time.Time, 5)
	for i := range returns {
		returns[i] = randn() * 0.02
		dates[i] = time.Now().AddDate(0, 0, -5+i)
	}

	results := detector.DetectAll(returns, dates)
	if len(results) != 0 {
		t.Errorf("expected no results with insufficient data, got %d", len(results))
	}
}

// ============================================================================
// 集中度监控测试
// ============================================================================

func TestConcentrationMonitor_HHI(t *testing.T) {
	config := NewConcentrationConfig()
	monitor := NewConcentrationMonitor(config)

	// 高度集中 (单一标的)
	concentrated := []PositionItem{
		{Code: "000001", Weight: 0.90},
		{Code: "000002", Weight: 0.10},
	}

	result := monitor.MonitorPositionConcentration(concentrated)

	if result.HHI < 0 {
		t.Errorf("HHI should not be negative")
	}
	if result.HHI < 0.5 {
		t.Errorf("concentrated portfolio should have high HHI, got %.3f", result.HHI)
	}
	if !result.IsConcentrated {
		t.Error("concentrated portfolio should be flagged")
	}
}

func TestConcentrationMonitor_Diversified(t *testing.T) {
	config := NewConcentrationConfig()
	monitor := NewConcentrationMonitor(config)

	// 分散组合
	diversified := []PositionItem{}
	for i := 1; i <= 20; i++ {
		code := formatCode(i)
		diversified = append(diversified, PositionItem{
			Code:   code,
			Weight: 0.05,
		})
	}

	result := monitor.MonitorPositionConcentration(diversified)

	if result.HHI > 0.1 {
		t.Errorf("diversified portfolio should have low HHI, got %.3f", result.HHI)
	}
	if result.IsConcentrated {
		t.Error("diversified portfolio should not be flagged")
	}
	if result.Severity != "normal" {
		t.Errorf("diversified portfolio should have normal severity, got %s", result.Severity)
	}
}

func TestConcentrationMonitor_Industry(t *testing.T) {
	config := NewConcentrationConfig()
	monitor := NewConcentrationMonitor(config)

	// 单行业集中
	positions := []PositionItem{
		{Code: "000001", Industry: "金融", Weight: 0.40},
		{Code: "000002", Industry: "金融", Weight: 0.35},
		{Code: "000003", Industry: "金融", Weight: 0.25},
	}

	result := monitor.MonitorIndustryConcentration(positions)

	if result.IsConcentrated {
		t.Log("Industry concentration flagged as expected")
	}
	if result.EffectiveCount > 1.5 {
		t.Errorf("expected effective count ~1.0, got %.1f", result.EffectiveCount)
	}
}

func TestConcentrationMonitor_Empty(t *testing.T) {
	config := NewConcentrationConfig()
	monitor := NewConcentrationMonitor(config)

	result := monitor.MonitorPositionConcentration(nil)
	if result.Description == "" {
		t.Error("expected description for empty portfolio")
	}
}

func TestConcentrationMonitor_MonitorAll(t *testing.T) {
	config := NewConcentrationConfig()
	monitor := NewConcentrationMonitor(config)

	positions := []PositionItem{
		{Code: "000001", Industry: "金融", Weight: 0.50},
		{Code: "000002", Industry: "消费", Weight: 0.30},
		{Code: "000003", Industry: "科技", Weight: 0.20},
	}

	results := monitor.MonitorAll(positions)

	if len(results) < 3 {
		t.Errorf("expected at least 3 result types, got %d", len(results))
	}

	resultTypes := map[ConcentrationType]bool{}
	for _, r := range results {
		resultTypes[r.Type] = true
	}
	if !resultTypes[ConcentrationStock] {
		t.Error("missing stock concentration result")
	}
}

func TestHerfindahlIndex(t *testing.T) {
	// 完全均匀分布: HHI = n * (1/n)^2 = 1/n
	uniform := map[string]float64{
		"a": 0.25, "b": 0.25, "c": 0.25, "d": 0.25,
	}
	hhi := herfindahlIndex(uniform)
	expected := 0.25 // 4 * 0.25^2 = 0.25
	if math.Abs(hhi-expected) > 0.001 {
		t.Errorf("uniform HHI: expected %.4f, got %.4f", expected, hhi)
	}

	// 完全集中: HHI = 1
	concentrated := map[string]float64{"a": 1.0}
	hhi = herfindahlIndex(concentrated)
	if math.Abs(hhi-1.0) > 0.001 {
		t.Errorf("concentrated HHI: expected 1.0, got %.4f", hhi)
	}
}

// ============================================================================
// 预警系统测试
// ============================================================================

func TestAlertEngine_AlertGeneration(t *testing.T) {
	config := NewAlertConfig()
	engine := NewAlertEngine(config)

	driftResults := []DriftDetectionResult{
		{
			Type:        DriftMean,
			Significant: true,
			Severity:    "moderate",
			MetricName:  "Expected Return",
			OldValue:    0.01,
			NewValue:    -0.01,
			DeltaPct:    -2.0,
			Description: "期望收益大幅下降",
		},
	}

	decayResults := []DecayDetectionResult{
		{
			Type:        DecaySharpeDecline,
			IsDecaying:  true,
			Severity:    "active",
			Description: "夏普比率显著下降",
		},
	}

	concResults := []ConcentrationResult{
		{
			Type:           ConcentrationStock,
			IsConcentrated: true,
			Severity:       "warning",
			Description:    "个股集中度偏高",
		},
	}

	alerts := engine.GenerateAlerts(driftResults, decayResults, concResults, "test-source")

	if len(alerts) == 0 {
		t.Error("expected alerts to be generated")
	}

	for _, a := range alerts {
		if a.Source != "test-source" {
			t.Errorf("alert source mismatch: %s", a.Source)
		}
		if a.CreatedAt.IsZero() {
			t.Error("alert should have creation time")
		}
	}
}

func TestAlertEngine_AckAndResolve(t *testing.T) {
	config := NewAlertConfig()
	engine := NewAlertEngine(config)

	driftResults := []DriftDetectionResult{
		{
			Type:        DriftMean,
			Significant: true,
			Severity:    "severe",
			MetricName:  "Test",
			DeltaPct:    -0.5,
			Description: "严重漂移",
		},
	}

	alerts := engine.GenerateAlerts(driftResults, nil, nil, "test")
	if len(alerts) == 0 {
		t.Fatal("expected at least one alert")
	}

	alertID := alerts[0].ID

	// 确认预警
	err := engine.AcknowledgeAlert(alertID, "admin")
	if err != nil {
		t.Errorf("failed to acknowledge alert: %v", err)
	}

	// 再次确认应该失败 (已不是 active)
	err = engine.AcknowledgeAlert(alertID, "admin")
	if err == nil {
		t.Error("should not be able to acknowledge non-active alert")
	}

	// 解决预警
	err = engine.ResolveAlert(alertID)
	if err != nil {
		t.Errorf("failed to resolve alert: %v", err)
	}
}

func TestAlertEngine_GetByStatus(t *testing.T) {
	config := NewAlertConfig()
	engine := NewAlertEngine(config)

	// 创建一些预警
	driftResults := []DriftDetectionResult{
		{
			Type:        DriftMean,
			Significant: true,
			Severity:    "mild",
			MetricName:  "A",
			Description: "Test A",
		},
		{
			Type:        DriftWinRate,
			Significant: true,
			Severity:    "moderate",
			MetricName:  "B",
			Description: "Test B",
		},
	}

	engine.GenerateAlerts(driftResults, nil, nil, "source1")
	engine.GenerateAlerts(nil, nil, nil, "source2")

	active := engine.GetActiveAlerts()
	if len(active) < 2 {
		t.Errorf("expected at least 2 active alerts, got %d", len(active))
	}

	bySource := engine.GetAlertsBySource("source1")
	if len(bySource) < 2 {
		t.Errorf("expected at least 2 alerts for source1, got %d", len(bySource))
	}

	byLevel := engine.GetAlertsByLevel(AlertLevelWarning)
	for _, a := range byLevel {
		if a.Level != AlertLevelWarning {
			t.Error("wrong level returned")
		}
	}
}

func TestAlertEngine_Summary(t *testing.T) {
	config := NewAlertConfig()
	engine := NewAlertEngine(config)

	driftResults := []DriftDetectionResult{
		{
			Type: DriftMean, Significant: true, Severity: "severe",
			MetricName: "Test", DeltaPct: -0.5, Description: "严重",
		},
	}

	engine.GenerateAlerts(driftResults, nil, nil, "test")

	summary := engine.GetAlertSummary()
	if summary.TotalAlerts == 0 {
		t.Error("summary should have alerts")
	}
	if summary.CriticalCount == 0 {
		t.Error("severe drift should generate critical alert")
	}
}

func TestSortAlerts(t *testing.T) {
	alerts := []Alert{
		{ID: "1", Level: AlertLevelInfo, CreatedAt: time.Now()},
		{ID: "2", Level: AlertLevelCritical, CreatedAt: time.Now()},
		{ID: "3", Level: AlertLevelWarning, CreatedAt: time.Now()},
		{ID: "4", Level: AlertLevelDanger, CreatedAt: time.Now()},
	}

	sorted := SortAlerts(alerts)

	if sorted[0].Level != AlertLevelCritical {
		t.Errorf("first alert should be critical, got %s", sorted[0].Level)
	}
	if sorted[1].Level != AlertLevelDanger {
		t.Errorf("second alert should be danger, got %s", sorted[1].Level)
	}
}

// ============================================================================
// 监控引擎测试
// ============================================================================

func TestMonitorEngine_RunMonitoring(t *testing.T) {
	config := NewDefaultMonitorConfig()
	engine := NewMonitorEngine(config)

	input := MonitoringInput{
		BaselineReturns: generateNormal(200, 0.01, 0.02),
		ForwardReturns:  generateNormal(60, 0.008, 0.025),
		ForwardDates:    generateDates(60),
		Positions: []PositionItem{
			{Code: "000001", Industry: "金融", Weight: 0.30},
			{Code: "000002", Industry: "消费", Weight: 0.25},
			{Code: "000003", Industry: "科技", Weight: 0.20},
			{Code: "000004", Industry: "医药", Weight: 0.15},
			{Code: "000005", Industry: "能源", Weight: 0.10},
		},
	}

	report := engine.RunMonitoring(input)

	if report.Source != config.Source {
		t.Errorf("source mismatch: %s", report.Source)
	}
	if report.GeneratedAt.IsZero() {
		t.Error("report should have generation time")
	}
	if len(report.DriftResults) == 0 {
		t.Error("expected drift results")
	}
	if len(report.DecayResults) == 0 {
		t.Error("expected decay results")
	}
	if len(report.ConcentrationResults) == 0 {
		t.Error("expected concentration results")
	}
	if report.HealthScore < 0 || report.HealthScore > 100 {
		t.Errorf("health score out of range [0,100]: %.1f", report.HealthScore)
	}
}

func TestMonitorEngine_ReportRecommendations(t *testing.T) {
	config := NewDefaultMonitorConfig()
	engine := NewMonitorEngine(config)

	// 故意构造一个"坏"的场景
	input := MonitoringInput{
		BaselineReturns: generateNormal(200, 0.02, 0.02),
		ForwardReturns:  generateNormal(60, -0.02, 0.05),
		ForwardDates:    generateDates(60),
		Positions: []PositionItem{
			{Code: "000001", Industry: "金融", Weight: 0.90},
			{Code: "000002", Industry: "金融", Weight: 0.10},
		},
	}

	report := engine.RunMonitoring(input)

	if len(report.Recommendations) == 0 {
		t.Log("No recommendations (may be normal for this data)")
	}
}

func TestMonitorEngine_HealthScoreRange(t *testing.T) {
	config := NewDefaultMonitorConfig()
	engine := NewMonitorEngine(config)

	input := MonitoringInput{
		BaselineReturns: generateNormal(100, 0.01, 0.02),
		ForwardReturns:  generateNormal(30, 0.01, 0.02),
		ForwardDates:    generateDates(30),
		Positions: []PositionItem{
			{Code: "000001", Industry: "A", Weight: 0.25},
			{Code: "000002", Industry: "B", Weight: 0.25},
			{Code: "000003", Industry: "C", Weight: 0.25},
			{Code: "000004", Industry: "D", Weight: 0.25},
		},
	}

	report := engine.RunMonitoring(input)

	if report.HealthScore < 0 || report.HealthScore > 100 {
		t.Errorf("health score should be in [0,100], got %.1f", report.HealthScore)
	}
}

// ============================================================================
// 统计函数测试
// ============================================================================

func TestStatFunctions(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}

	if m := mean(data); m != 3.0 {
		t.Errorf("mean: expected 3.0, got %.2f", m)
	}

	if v := variance(data); v < 1.0 || v > 3.0 {
		t.Errorf("variance: expected ~2.5, got %.2f", v)
	}

	if sd := standardDeviation(data); sd < 1.5 || sd > 2.0 {
		t.Errorf("std dev: expected ~1.58, got %.2f", sd)
	}

	wr := winRate([]float64{0.01, -0.01, 0.02, -0.02, 0.03})
	if wr < 0.5 || wr > 0.7 {
		t.Errorf("win rate: expected ~0.6, got %.2f", wr)
	}
}

func TestEmptyStatFunctions(t *testing.T) {
	if mean(nil) != 0 {
		t.Error("empty mean should be 0")
	}
	if standardDeviation(nil) != 0 {
		t.Error("empty std should be 0")
	}
	if winRate(nil) != 0 {
		t.Error("empty win rate should be 0")
	}
	if skewness(nil) != 0 {
		t.Error("empty skewness should be 0")
	}
}

func TestRelativeChange(t *testing.T) {
	if rc := relativeChange(0.1, 0.12); math.Abs(rc-0.2) > 0.01 {
		t.Errorf("relative change: expected 0.2, got %.4f", rc)
	}
	if rc := relativeChange(0.1, 0.08); math.Abs(rc-(-0.2)) > 0.01 {
		t.Errorf("relative change: expected -0.2, got %.4f", rc)
	}
}

func TestMaxDrawdown(t *testing.T) {
	returns := []float64{0.10, 0.05, -0.20, 0.10} // 峰值后回撤
	dd := maxDrawdown(returns)
	if dd < 0.15 || dd > 0.25 {
		t.Errorf("max drawdown: expected ~0.17, got %.4f", dd)
	}
}

func TestAnnualizedSharpe(t *testing.T) {
	// 零波动: 夏普为 0
	if s := annualizedSharpe([]float64{0.01, 0.01, 0.01}); s != 0 {
		t.Error("zero volatility should give Sharpe 0")
	}

	// 正收益正波动: 正夏普
	returns := generateNormal(50, 0.01, 0.02)
	s := annualizedSharpe(returns)
	if s <= 0 {
		t.Errorf("positive mean with positive vol should give positive Sharpe, got %.2f", s)
	}
}

// ============================================================================
// 数据生成辅助
// ============================================================================

func generateNormal(n int, mean_, std float64) []float64 {
	result := make([]float64, n)
	for i := range result {
		result[i] = mean_ + randn()*std
	}
	return result
}

func generateBiased(n int, winProb float64, std float64) []float64 {
	result := make([]float64, n)
	for i := range result {
		if randFloat() < winProb {
			result[i] = 0.01 + randn()*std
		} else {
			result[i] = -0.01 + randn()*std
		}
	}
	return result
}

func generateDates(n int) []time.Time {
	dates := make([]time.Time, n)
	base := time.Now()
	for i := range dates {
		dates[i] = base.AddDate(0, 0, -n+i)
	}
	return dates
}

// randn 生成标准正态分布随机数 (近似)
func randn() float64 {
	// Box-Muller transform
	u1 := testRng.Float64()
	u2 := testRng.Float64()
	if u1 < 1e-10 {
		u1 = 1e-10
	}
	z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	return z
}

// randFloat 生成 [0,1) 随机数
func randFloat() float64 {
	return testRng.Float64()
}

// formatCode 格式化股票代码
func formatCode(n int) string {
	if n <= 0 || n >= 1000000 {
		return fmt.Sprintf("%06d", n)
	}
	padded := fmt.Sprintf("%06d", n)
	return padded
}
