package evidence

import (
	"math"
	"testing"
)

func TestHoldingPeriodToType(t *testing.T) {
	tests := []struct {
		input    string
		expected HoldingPeriodType
	}{
		{"intraday", HoldingIntraday},
		{"日内", HoldingIntraday},
		{"t+0", HoldingIntraday},
		{"short", HoldingShort},
		{"1-5", HoldingShort},
		{"1-5天", HoldingShort},
		{"medium", HoldingMedium},
		{"5-20", HoldingMedium},
		{"5-20天", HoldingMedium},
		{"long", HoldingLong},
		{"20+", HoldingLong},
		{"20+天", HoldingLong},
		{"unknown", HoldingUndefined},
	}

	for _, tc := range tests {
		result := HoldingPeriodToType(tc.input)
		if result != tc.expected {
			t.Errorf("HoldingPeriodToType(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
}

func TestHoldingPeriodToMinSample(t *testing.T) {
	tests := []struct {
		hp      HoldingPeriodType
		minSize int
	}{
		{HoldingIntraday, 100},
		{HoldingShort, 50},
		{HoldingMedium, 30},
		{HoldingLong, 20},
		{HoldingUndefined, 30},
	}

	for _, tc := range tests {
		result := HoldingPeriodToMinSample(tc.hp)
		if result != tc.minSize {
			t.Errorf("HoldingPeriodToMinSample(%v) = %d, want %d", tc.hp, result, tc.minSize)
		}
	}
}

func TestHoldingPeriodToMaxDrawdown(t *testing.T) {
	tests := []struct {
		hp    HoldingPeriodType
		maxDD float64
	}{
		{HoldingIntraday, 5.0},
		{HoldingShort, 10.0},
		{HoldingMedium, 15.0},
		{HoldingLong, 20.0},
		{HoldingUndefined, 15.0},
	}

	for _, tc := range tests {
		result := HoldingPeriodToMaxDrawdown(tc.hp)
		if result != tc.maxDD {
			t.Errorf("HoldingPeriodToMaxDrawdown(%v) = %.2f, want %.2f", tc.hp, result, tc.maxDD)
		}
	}
}

func TestComputeMetrics(t *testing.T) {
	returns := []float64{2.5, -1.0, 3.0, -0.5, 1.5, 4.0, -2.0, 0.5, 2.0, -1.5}
	m := ComputeMetrics(returns, "short", 0.001)

	if m.SampleSize != 10 {
		t.Errorf("SampleSize = %d, want 10", m.SampleSize)
	}

	if m.AvgReturn <= 0 {
		t.Errorf("AvgReturn should be positive, got %.2f", m.AvgReturn)
	}

	if m.WinRate <= 0 {
		t.Errorf("WinRate should be positive, got %.2f", m.WinRate)
	}

	// Verify net return accounts for costs.
	grossExpected := 2.5 - 1.0 + 3.0 - 0.5 + 1.5 + 4.0 - 2.0 + 0.5 + 2.0 - 1.5
	netExpected := grossExpected - 0.001*2*100*10 // cost * 2 sides * 10 trades
	if math.Abs(m.GrossReturn-grossExpected) > 0.01 {
		t.Errorf("GrossReturn = %.2f, want %.2f", m.GrossReturn, grossExpected)
	}
	if math.Abs(m.NetReturn-netExpected) > 0.01 {
		t.Errorf("NetReturn = %.2f, want %.2f", m.NetReturn, netExpected)
	}
}

func TestComputeMetricsEmpty(t *testing.T) {
	m := ComputeMetrics([]float64{}, "short", 0.001)
	if m.SampleSize != 0 {
		t.Errorf("SampleSize = %d, want 0", m.SampleSize)
	}
}

func TestConfidenceInterval(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ci := confidenceInterval(values, 0.95)

	if ci[0] >= ci[1] {
		t.Errorf("CI lower bound should be less than upper bound: [%f, %f]", ci[0], ci[1])
	}

	// Mean of 1..10 is 5.5.
	if ci[0] > 5.5 || ci[1] < 5.5 {
		t.Errorf("Mean 5.5 should be within CI [%f, %f]", ci[0], ci[1])
	}
}

func TestTopNConcentration(t *testing.T) {
	// All-positive returns so concentration is a meaningful fraction of total.
	returns := []float64{10, 8, 3, 2, 1, 0.5, 0.5, 0.3, 0.2, 0.1}
	total := 10 + 8 + 3 + 2 + 1 + 0.5 + 0.5 + 0.3 + 0.2 + 0.1 // = 25.6

	// Top 20% (2 trades) contributes (10+8)/total.
	top20 := topNConcentration(returns, 0.2)
	expected20 := (10 + 8) / total * 100
	if math.Abs(top20-expected20) > 0.01 {
		t.Errorf("Top20Concentration = %.2f, want %.2f", top20, expected20)
	}

	// Top 10% (1 trade) contributes 10/total.
	top10 := topNConcentration(returns, 0.1)
	expected10 := 10 / total * 100
	if math.Abs(top10-expected10) > 0.01 {
		t.Errorf("Top10Concentration = %.2f, want %.2f", top10, expected10)
	}

	// Top 10 must be <= Top 20.
	if top10 > top20 {
		t.Errorf("Top10 (%.2f) should be <= Top20 (%.2f)", top10, top20)
	}
}

func TestTopNConcentrationZeroTotal(t *testing.T) {
	// When total is zero, concentration should be zero to avoid division by zero.
	returns := []float64{1, -1, 2, -2}
	conc := topNConcentration(returns, 0.5)
	if conc != 0 {
		t.Errorf("Concentration with zero total should be 0, got %.2f", conc)
	}
}

func TestMaxDrawdown(t *testing.T) {
	// Returns: 0 -> 1 -> 3 -> 2 -> 4 -> 6 -> 5 -> 3 -> 8 -> 10
	returns := []float64{1, 2, -1, 2, 2, -1, -2, 5, 2}
	mdd := maxDrawdown(returns)

	// Cumulative: 0, 1, 3, 2, 4, 6, 5, 3, 8, 10
	// Peak: 0, 1, 3, 3, 4, 6, 6, 6, 8, 10
	// DD: 0, 0, 0, 1, 0, 0, 1, 3, 0, 0
	expected := 3.0 // Peak at 6, trough at 3.
	if math.Abs(mdd-expected) > 0.01 {
		t.Errorf("MaxDrawdown = %.2f, want %.2f", mdd, expected)
	}
}

func TestMetricsHoldingPeriodType(t *testing.T) {
	m := Metrics{HoldingPeriod: "short"}
	if m.HoldingPeriodType() != HoldingShort {
		t.Errorf("HoldingPeriodType() = %v, want %v", m.HoldingPeriodType(), HoldingShort)
	}
}

func TestMetricsMinSampleSize(t *testing.T) {
	tests := []struct {
		hp      string
		minSize int
	}{
		{"intraday", 100},
		{"short", 50},
		{"medium", 30},
		{"long", 20},
		{"", 30}, // undefined defaults to 30
	}

	for _, tc := range tests {
		m := Metrics{HoldingPeriod: tc.hp}
		result := m.MinSampleSize()
		if result != tc.minSize {
			t.Errorf("MinSampleSize(HoldingPeriod=%q) = %d, want %d", tc.hp, result, tc.minSize)
		}
	}
}

func TestMetricsMaxDrawdownLimit(t *testing.T) {
	tests := []struct {
		hp    string
		maxDD float64
	}{
		{"intraday", 5.0},
		{"short", 10.0},
		{"medium", 15.0},
		{"long", 20.0},
		{"", 15.0},
	}

	for _, tc := range tests {
		m := Metrics{HoldingPeriod: tc.hp}
		result := m.MaxDrawdownLimit()
		if result != tc.maxDD {
			t.Errorf("MaxDrawdownLimit(HoldingPeriod=%q) = %.2f, want %.2f", tc.hp, result, tc.maxDD)
		}
	}
}

func TestCheckAdmissionPass(t *testing.T) {
	// A passing scenario: enough sample, positive returns, good CI, etc.
	evidence := Evidence{
		ParadigmID: "test-paradigm",
		Metrics: Metrics{
			SampleSize:         60,
			HoldingPeriod:      "short",
			NetReturn:          15.0,
			AvgReturn:          1.5,
			MaxDrawdown:        8.0,
			WinRate:            55.0,
			ConfidenceInterval: [2]float64{0.5, 2.5},
			ConfidenceLevel:    0.95,
			Top20Concentration: 55.0,
			Top10Concentration: 30.0,
			ParamSensitivity:   0.3,
			RiskRewardRatio:    1.0,
		},
		Windows: []WindowResult{
			{NetReturn: 5.0, WinRate: 60},
			{NetReturn: 8.0, WinRate: 55},
			{NetReturn: 6.0, WinRate: 58},
		},
		Regimes: []RegimeResult{
			{Regime: "bull", NetReturn: 10.0, WinRate: 60},
			{Regime: "bear", NetReturn: 2.0, WinRate: 45},
			{Regime: "range", NetReturn: 3.0, WinRate: 50},
		},
	}

	result := evidence.CheckAdmission()

	if !result.Eligible {
		t.Errorf("Expected eligible, got ineligible. MustFix: %v", result.MustFix)
	}

	if result.Level != LevelGold && result.Level != LevelPlatinum {
		t.Errorf("Expected at least Gold level, got %v", result.Level)
	}
}

func TestCheckAdmissionBlocker(t *testing.T) {
	// A failing scenario with a blocker.
	evidence := Evidence{
		ParadigmID:    "test-paradigm",
		HasFutureData: true,
		Metrics: Metrics{
			SampleSize:    60,
			HoldingPeriod: "short",
			NetReturn:     15.0,
		},
	}

	result := evidence.CheckAdmission()

	if result.Eligible {
		t.Error("Expected ineligible due to future data blocker")
	}

	found := false
	for _, m := range result.MustFix {
		if m == "禁止使用未来数据" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected blocker message about future data, got: %v", result.MustFix)
	}
}

func TestCheckAdmissionSurvivorshipBias(t *testing.T) {
	evidence := Evidence{
		ParadigmID:          "test-paradigm",
		HasSurvivorshipBias: true,
		Metrics: Metrics{
			SampleSize:    100,
			HoldingPeriod: "intraday",
			NetReturn:     20.0,
		},
	}

	result := evidence.CheckAdmission()

	if result.Eligible {
		t.Error("Expected ineligible due to survivorship bias")
	}
}

func TestCheckAdmissionInsufficientSample(t *testing.T) {
	evidence := Evidence{
		ParadigmID: "test-paradigm",
		Metrics: Metrics{
			SampleSize:    10, // Too low for "short"
			HoldingPeriod: "short",
			NetReturn:     5.0,
		},
	}

	result := evidence.CheckAdmission()

	found := false
	for _, m := range result.MustFix {
		if m == "样本量不足：10 < 50" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected sample size error, got: %v", result.MustFix)
	}
}

func TestCheckAdmissionNegativeCI(t *testing.T) {
	evidence := Evidence{
		ParadigmID: "test-paradigm",
		Metrics: Metrics{
			SampleSize:         60,
			HoldingPeriod:      "short",
			NetReturn:          5.0,
			ConfidenceInterval: [2]float64{-2.0, 3.0}, // Negative lower bound
			ConfidenceLevel:    0.95,
		},
	}

	result := evidence.CheckAdmission()

	found := false
	for _, m := range result.MustFix {
		if m == "95%置信区间下界必须大于0" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected CI error, got: %v", result.MustFix)
	}
}

func TestCheckAdmissionHighConcentration(t *testing.T) {
	evidence := Evidence{
		ParadigmID: "test-paradigm",
		Metrics: Metrics{
			SampleSize:         60,
			HoldingPeriod:      "short",
			NetReturn:          5.0,
			Top20Concentration: 85.0, // Too high
		},
		Windows: []WindowResult{
			{NetReturn: 5.0}, {NetReturn: 4.0}, {NetReturn: 6.0},
		},
		Regimes: []RegimeResult{
			{Regime: "bull", NetReturn: 5.0},
			{Regime: "bear", NetReturn: 3.0},
		},
	}

	result := evidence.CheckAdmission()

	found := false
	for _, m := range result.MustFix {
		if m == "收益集中度过高：Top20%贡献85%收益" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected concentration error, got: %v", result.MustFix)
	}
}

func TestCheckAdmissionLowWindows(t *testing.T) {
	evidence := Evidence{
		ParadigmID: "test-paradigm",
		Metrics: Metrics{
			SampleSize:    60,
			HoldingPeriod: "short",
			NetReturn:     5.0,
		},
		Windows: []WindowResult{
			{NetReturn: 5.0}, {NetReturn: 4.0}, // Only 2 windows
		},
		Regimes: []RegimeResult{
			{Regime: "bull", NetReturn: 5.0},
			{Regime: "bear", NetReturn: 3.0},
		},
	}

	result := evidence.CheckAdmission()

	found := false
	for _, m := range result.MustFix {
		if m == "滚动窗口数量不足（至少3个）" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected windows error, got: %v", result.MustFix)
	}
}

func TestCheckAdmissionNoPositiveRegimes(t *testing.T) {
	evidence := Evidence{
		ParadigmID: "test-paradigm",
		Metrics: Metrics{
			SampleSize:    60,
			HoldingPeriod: "short",
			NetReturn:     5.0,
		},
		Windows: []WindowResult{
			{NetReturn: 5.0}, {NetReturn: 4.0}, {NetReturn: 6.0},
		},
		Regimes: []RegimeResult{
			{Regime: "bull", NetReturn: -2.0},
			{Regime: "bear", NetReturn: -1.0},
			{Regime: "range", NetReturn: -3.0},
		},
	}

	result := evidence.CheckAdmission()

	found := false
	for _, m := range result.MustFix {
		if m == "所有市场状态下收益均为负" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected negative regimes error, got: %v", result.MustFix)
	}
}

func TestCheckAdmissionHighParamSensitivity(t *testing.T) {
	evidence := Evidence{
		ParadigmID: "test-paradigm",
		Metrics: Metrics{
			SampleSize:       60,
			HoldingPeriod:    "short",
			NetReturn:        5.0,
			ParamSensitivity: 1.5, // Too high
		},
		Windows: []WindowResult{
			{NetReturn: 5.0}, {NetReturn: 4.0}, {NetReturn: 6.0},
		},
		Regimes: []RegimeResult{
			{Regime: "bull", NetReturn: 5.0},
			{Regime: "bear", NetReturn: 3.0},
		},
	}

	result := evidence.CheckAdmission()

	found := false
	for _, m := range result.MustFix {
		if m == "参数敏感性过高：1.50 > 1.0" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected sensitivity error, got: %v", result.MustFix)
	}
}

func TestDefaultCostPerTrade(t *testing.T) {
	cost := DefaultCostPerTrade()
	if cost <= 0 || cost > 0.01 {
		t.Errorf("DefaultCostPerTrade = %.4f, expected ~0.0013", cost)
	}
}

func TestMetricsRequiredRiskRewardRatio(t *testing.T) {
	tests := []struct {
		hp  string
		rrr float64
	}{
		{"intraday", 0.5},
		{"short", 0.8},
		{"medium", 1.0},
		{"long", 1.2},
		{"", 1.0},
	}

	for _, tc := range tests {
		m := Metrics{HoldingPeriod: tc.hp}
		result := m.RequiredRiskRewardRatio()
		if result != tc.rrr {
			t.Errorf("RequiredRiskRewardRatio(HoldingPeriod=%q) = %.2f, want %.2f", tc.hp, result, tc.rrr)
		}
	}
}
