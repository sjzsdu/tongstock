package paradigms

import (
	"testing"
	"time"
)

// ============================================================================
// BacktestResult 测试
// ============================================================================

func TestBacktestResultStruct(t *testing.T) {
	bt := &BacktestResult{
		ParadigmID:  "test-001",
		StockCode:   "000001",
		SampleSize:  100,
		WinRate5:    0.55,
		WinRate20:   0.60,
		AvgReturn5:  0.02,
		AvgReturn20: 0.08,
		MaxDrawdown: 0.05,
	}

	if bt.ParadigmID != "test-001" {
		t.Error("ParadigmID mismatch")
	}
	if bt.SampleSize != 100 {
		t.Error("SampleSize mismatch")
	}
}

// ============================================================================
// EvidenceBuilder 测试
// ============================================================================

func TestNewEvidenceBuilder(t *testing.T) {
	builder := NewEvidenceBuilder()
	if builder == nil {
		t.Fatal("NewEvidenceBuilder returned nil")
	}
	if builder.config.SampleOutWeight <= 0 {
		t.Error("config should have positive weights")
	}
}

func TestBuildFromParadigmNilBacktest(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{
		ID:        "p-001",
		Name:      "测试范式",
		StockCode: "000001",
		StockName: "平安银行",
		Side:      "buy",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Source: ParadigmSource{
			Model:        "gpt-4",
			AgentVersion: "stock-paradigm-miner",
			CacheKey:     "cache-key-001",
		},
	}

	card := builder.BuildFromParadigm(p, nil)

	if card == nil {
		t.Fatal("BuildFromParadigm returned nil")
	}
	if card.ParadigmID != "p-001" {
		t.Errorf("ParadigmID = %q, want %q", card.ParadigmID, "p-001")
	}
	if card.ParadigmName != "测试范式" {
		t.Errorf("ParadigmName = %q, want %q", card.ParadigmName, "测试范式")
	}
	if card.StockCode != "000001" {
		t.Errorf("StockCode = %q, want %q", card.StockCode, "000001")
	}

	// Without backtest, sample results should be zero
	if card.InSample.SampleSize != 0 {
		t.Error("InSample should have zero sample size without backtest")
	}

	// Should still have cost analysis
	if card.CostAnalysis.CostRatio <= 0 {
		t.Error("CostAnalysis should have positive cost ratio")
	}

	// Should have drawdown analysis
	if card.DrawdownAnalysis.MaxDrawdown != 0 {
		t.Error("DrawdownAnalysis should have zero max drawdown")
	}
}

func TestBuildFromParadigmWithBacktest(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{
		ID:        "p-002",
		Name:      "多头突破",
		StockCode: "600519",
		StockName: "贵州茅台",
		Side:      "buy",
		Invalid:   []string{"跌破MA60", "MACD继续死叉"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Source: ParadigmSource{
			Model:        "gpt-4",
			AgentVersion: "stock-paradigm-miner",
			CacheKey:     "cache-key-002",
		},
	}

	bt := &BacktestResult{
		ParadigmID:  "p-002",
		StockCode:   "600519",
		SampleSize:  200,
		WinRate5:    0.45,
		WinRate10:   0.50,
		WinRate20:   0.55,
		AvgReturn5:  0.015,
		AvgReturn10: 0.035,
		AvgReturn20: 0.08,
		MaxDrawdown: 0.08,
	}

	card := builder.BuildFromParadigm(p, bt)

	if card == nil {
		t.Fatal("BuildFromParadigm returned nil")
	}

	// 样本内结果
	if card.InSample.SampleSize != 200 {
		t.Errorf("InSample.SampleSize = %d, want 200", card.InSample.SampleSize)
	}
	if card.InSample.WinRate != 0.45 {
		t.Errorf("InSample.WinRate = %f, want 0.45", card.InSample.WinRate)
	}
	if card.InSample.AnnualReturn <= 0 {
		t.Error("InSample.AnnualReturn should be positive")
	}

	// 样本外结果
	if card.OutOfSample.WinRate != 0.55 {
		t.Errorf("OutOfSample.WinRate = %f, want 0.55", card.OutOfSample.WinRate)
	}

	// 成本分析
	if card.CostAnalysis.GrossReturn <= 0 {
		t.Error("CostAnalysis.GrossReturn should be positive")
	}
	if card.CostAnalysis.CostRatio != 0.15 {
		t.Errorf("CostAnalysis.CostRatio = %f, want 0.15", card.CostAnalysis.CostRatio)
	}

	// 回撤分析
	if card.DrawdownAnalysis.MaxDrawdown != 0.08 {
		t.Errorf("DrawdownAnalysis.MaxDrawdown = %f, want 0.08", card.DrawdownAnalysis.MaxDrawdown)
	}

	// 反证: 短周期胜率低于长周期
	if len(card.CounterEvidence) < 1 {
		t.Error("Should have counter evidence for win rate difference")
	}

	// 反证: 否定条件
	hasInvalidation := false
	for _, ce := range card.CounterEvidence {
		if ce.Type == "invalidation_rule" {
			hasInvalidation = true
			break
		}
	}
	if !hasInvalidation {
		t.Error("Should have invalidation rule counter evidence")
	}

	// 数据血缘
	if card.Lineage.DataSource == "" {
		t.Error("Lineage.DataSource should not be empty")
	}
	if card.Lineage.VersionID != "p-002" {
		t.Errorf("Lineage.VersionID = %q, want %q", card.Lineage.VersionID, "p-002")
	}
	if len(card.Lineage.ReviewHistory) < 1 {
		t.Error("Should have at least one review record")
	}
}

func TestBuildFromParadigmWithHighDrawdown(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{
		ID:   "p-003",
		Side: "buy",
	}

	// 低收益 + 高回撤 → 回撤收益比 > 1
	bt := &BacktestResult{
		ParadigmID:  "p-003",
		SampleSize:  50,
		AvgReturn5:  0.001, // 0.1% 收益
		AvgReturn20: 0.005, // 0.5% 收益
		MaxDrawdown: 0.20,  // 20% 回撤 > 10% 且 >> 收益
	}

	card := builder.BuildFromParadigm(p, bt)

	// 应该有回撤反证
	hasHighDD := false
	for _, ce := range card.CounterEvidence {
		if ce.Type == "risk_case" {
			hasHighDD = true
			break
		}
	}
	if !hasHighDD {
		t.Error("Should have risk counter evidence for high drawdown")
	}

	// 回撤收益比 = 0.20 / (0.001*252/5) = 0.20 / 0.0504 ≈ 3.97 > 1
	if card.DrawdownAnalysis.Warning == "" {
		t.Error("Should have drawdown warning for high DD/return ratio")
	}

	// 风险标记
	hasDDWarning := false
	for _, rf := range card.RiskFlags {
		if rf.Category == "drawdown" {
			hasDDWarning = true
			break
		}
	}
	if !hasDDWarning {
		t.Error("Should have drawdown risk flag")
	}
}

// ============================================================================
// 置信区间测试
// ============================================================================

func TestComputeConfidenceIntervalSmallSample(t *testing.T) {
	builder := NewEvidenceBuilder()
	sr := SampleResult{
		SampleSize:   15,
		AnnualReturn: 0.05,
	}

	ci := builder.computeConfidenceInterval(sr)

	if ci.SampleSize != 15 {
		t.Errorf("CI.SampleSize = %d, want 15", ci.SampleSize)
	}
	// 样本量 < 30 应该有备注
	if len(ci.Notes) < 1 {
		t.Error("Should have notes for small sample size")
	}
}

func TestComputeConfidenceIntervalNegativeReturn(t *testing.T) {
	builder := NewEvidenceBuilder()
	sr := SampleResult{
		SampleSize:   100,
		AnnualReturn: -0.05,
	}

	ci := builder.computeConfidenceInterval(sr)

	if ci.Significant {
		t.Error("Negative return should not be significant")
	}
	hasNegativeNote := false
	for _, n := range ci.Notes {
		if n == "样本外收益为负，策略表现低于随机" {
			hasNegativeNote = true
			break
		}
	}
	if !hasNegativeNote {
		t.Error("Should have negative return note")
	}
}

func TestComputeConfidenceIntervalLargeSample(t *testing.T) {
	builder := NewEvidenceBuilder()
	sr := SampleResult{
		SampleSize:   200,
		AnnualReturn: 0.10,
	}

	ci := builder.computeConfidenceInterval(sr)

	if ci.SampleSize != 200 {
		t.Errorf("CI.SampleSize = %d, want 200", ci.SampleSize)
	}
	if ci.CI95Lower > ci.CI95Upper {
		t.Error("Lower bound should be <= upper bound")
	}
	if ci.MeanReturn != 0.10 {
		t.Errorf("MeanReturn = %f, want 0.10", ci.MeanReturn)
	}
}

// ============================================================================
// 成本分析测试
// ============================================================================

func TestComputeCostAnalysis(t *testing.T) {
	builder := NewEvidenceBuilder()
	sr := SampleResult{
		TotalReturn: 0.10,
		TradesCount: 100,
	}

	cost := builder.computeCostAnalysis(sr)

	if cost.GrossReturn != 0.10 {
		t.Errorf("GrossReturn = %f, want 0.10", cost.GrossReturn)
	}
	if cost.CostRatio != 0.15 {
		t.Errorf("CostRatio = %f, want 0.15", cost.CostRatio)
	}
	if cost.NetRetention != 0.85 {
		t.Errorf("NetRetention = %f, want 0.85", cost.NetRetention)
	}
	if cost.NetReturn >= cost.GrossReturn {
		t.Error("NetReturn should be less than GrossReturn")
	}
}

func TestComputeCostAnalysisZeroTrades(t *testing.T) {
	builder := NewEvidenceBuilder()
	sr := SampleResult{
		TotalReturn: 0.10,
		TradesCount: 0,
	}

	cost := builder.computeCostAnalysis(sr)

	// Should not divide by zero
	if cost.CostPerTrade == 0 {
		t.Error("CostPerTrade should handle zero trades")
	}
}

// ============================================================================
// 回撤收益比测试
// ============================================================================

func TestComputeDrawdownRatio(t *testing.T) {
	builder := NewEvidenceBuilder()

	tests := []struct {
		maxDD        float64
		annualReturn float64
		expected     float64
	}{
		{0.05, 0.10, 0.5},
		{0.10, 0.05, 2.0},
		{0.05, 0, 0}, // 零收益避免除零
		{0, 0.10, 0}, // 零回撤
	}

	for _, tt := range tests {
		result := builder.computeDrawdownRatio(tt.maxDD, tt.annualReturn)
		if result != tt.expected {
			t.Errorf("computeDrawdownRatio(%f, %f) = %f, want %f",
				tt.maxDD, tt.annualReturn, result, tt.expected)
		}
	}
}

// ============================================================================
// 反证生成测试
// ============================================================================

func TestGenerateCounterEvidenceWinRateDifference(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{ID: "test"}
	bt := &BacktestResult{
		SampleSize: 50,
		WinRate5:   0.40,
		WinRate20:  0.55,
	}

	ev := builder.generateCounterEvidence(p, bt)

	found := false
	for _, e := range ev {
		if e.Type == "fail_case" && e.Severity == "medium" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Should find fail_case for win rate difference")
	}
}

func TestGenerateCounterEvidenceNoBacktest(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{ID: "test"}

	ev := builder.generateCounterEvidence(p, nil)

	// 没有回测数据时不应该有除了失效规则之外的反证
	for _, e := range ev {
		if e.Type != "invalidation_rule" {
			t.Errorf("Unexpected counter evidence type: %s", e.Type)
		}
	}
}

// ============================================================================
// 风险标记测试
// ============================================================================

func TestGenerateRiskFlagsHighDrawdown(t *testing.T) {
	builder := NewEvidenceBuilder()
	card := &EvidenceCard{
		DrawdownAnalysis: DrawdownInfo{MaxDrawdown: 0.20},
		CostAnalysis:     CostBreakdown{CostRatio: 0.15},
		Concentration:    ConcentrationInfo{MaxPositionWeight: 0.05},
		ConfidenceInterval: CIResult{
			Significant: true,
			SampleSize:  100,
		},
		InSample: SampleResult{SampleSize: 100},
	}

	flags := builder.generateRiskFlags(card)

	hasDD := false
	for _, f := range flags {
		if f.Category == "drawdown" {
			hasDD = true
			if f.Level != "critical" {
				t.Errorf("Drawdown risk level = %q, want critical", f.Level)
			}
		}
	}
	if !hasDD {
		t.Error("Should have drawdown risk flag for high drawdown")
	}
}

func TestGenerateRiskFlagsHighCost(t *testing.T) {
	builder := NewEvidenceBuilder()
	card := &EvidenceCard{
		DrawdownAnalysis: DrawdownInfo{MaxDrawdown: 0.05},
		CostAnalysis:     CostBreakdown{CostRatio: 0.40},
		Concentration:    ConcentrationInfo{MaxPositionWeight: 0.05},
		ConfidenceInterval: CIResult{
			Significant: true,
			SampleSize:  100,
		},
		InSample: SampleResult{SampleSize: 100},
	}

	flags := builder.generateRiskFlags(card)

	hasCost := false
	for _, f := range flags {
		if f.Category == "cost" {
			hasCost = true
			if f.Level != "high" {
				t.Errorf("Cost risk level = %q, want high", f.Level)
			}
		}
	}
	if !hasCost {
		t.Error("Should have cost risk flag for high cost ratio")
	}
}

func TestGenerateRiskFlagsLowSample(t *testing.T) {
	builder := NewEvidenceBuilder()
	card := &EvidenceCard{
		DrawdownAnalysis: DrawdownInfo{MaxDrawdown: 0.05},
		CostAnalysis:     CostBreakdown{CostRatio: 0.15},
		Concentration:    ConcentrationInfo{MaxPositionWeight: 0.05},
		ConfidenceInterval: CIResult{
			Significant: true,
			SampleSize:  10,
		},
		InSample: SampleResult{SampleSize: 10},
	}

	flags := builder.generateRiskFlags(card)

	hasSample := false
	for _, f := range flags {
		if f.Category == "sample_size" {
			hasSample = true
		}
	}
	if !hasSample {
		t.Error("Should have sample size risk flag")
	}
}

// ============================================================================
// 完整流程测试
// ============================================================================

func TestFullPipeline(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{
		ID:           "p-full",
		Name:         "完整测试范式",
		StockCode:    "000001",
		StockName:    "平安银行",
		Side:         "buy",
		Confirm:      []string{"成交量放大"},
		Invalid:      []string{"跌破MA60"},
		ReviewStatus: "reviewed",
		ReviewRating: 4,
		ReviewNote:   "表现良好",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Source: ParadigmSource{
			Model:        "gpt-4",
			AgentVersion: "stock-paradigm-miner",
			CacheKey:     "full-cache-key",
		},
	}

	bt := &BacktestResult{
		ParadigmID:  "p-full",
		StockCode:   "000001",
		SampleSize:  150,
		WinRate5:    0.50,
		WinRate10:   0.55,
		WinRate20:   0.60,
		AvgReturn5:  0.02,
		AvgReturn10: 0.04,
		AvgReturn20: 0.10,
		MaxDrawdown: 0.06,
	}

	card := builder.BuildFromParadigm(p, bt)

	// 验证所有主要区块都有数据
	if card.InSample.SampleSize != 150 {
		t.Error("InSample.SampleSize should be 150")
	}
	if card.OutOfSample.SampleSize != 150 {
		t.Error("OutOfSample.SampleSize should be 150")
	}
	if card.ConfidenceInterval.SampleSize != 150 {
		t.Error("ConfidenceInterval.SampleSize should be 150")
	}
	if card.CostAnalysis.CostRatio != 0.15 {
		t.Error("CostRatio should be 0.15")
	}
	if card.DrawdownAnalysis.MaxDrawdown != 0.06 {
		t.Error("MaxDrawdown should be 0.06")
	}

	// 反证不为空 (至少有失效规则)
	if len(card.CounterEvidence) == 0 {
		t.Error("CounterEvidence should not be empty")
	}

	// 风险标记
	// 样本量 > 30, 回撤 < 15%, 成本 < 30%, 集中度正常 → 可能没有风险标记
	// 但也可能有统计显著性等

	// 血缘
	if card.Lineage.VersionID != "p-full" {
		t.Error("Lineage.VersionID should be p-full")
	}
	if len(card.Lineage.ReviewHistory) < 2 {
		t.Error("Should have at least 2 review records (create + review)")
	}

	// 评分和阶段门
	if card.RobustnessScore == nil {
		t.Error("RobustnessScore should not be nil")
	}
	if card.RobustnessScore.OverallScore < 0 {
		t.Error("OverallScore should be non-negative")
	}
	if card.StageGateDecision == nil {
		t.Error("StageGateDecision should not be nil")
	}

	// 验证阶段门决策有效
	validStages := map[string]bool{"reject": true, "observe": true, "promote": true}
	if !validStages[card.StageGateDecision.Stage] {
		t.Errorf("Invalid stage: %s", card.StageGateDecision.Stage)
	}
}

func TestEvidenceCardImmutability(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{ID: "imm", Side: "buy"}
	bt := &BacktestResult{SampleSize: 100}

	card1 := builder.BuildFromParadigm(p, bt)
	card2 := builder.BuildFromParadigm(p, bt)

	// 两次调用应该产生不同的时间戳
	if card1.GeneratedAt.Equal(card2.GeneratedAt) {
		t.Log("Note: GeneratedAt may be equal if called within same nanosecond")
	}

	// 但结构应该一致
	if card1.ParadigmID != card2.ParadigmID {
		t.Error("Cards should have same ParadigmID")
	}
}

// ============================================================================
// Edge case 测试
// ============================================================================

func TestBuildFromParadigmEmptyParadigm(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{}

	card := builder.BuildFromParadigm(p, nil)

	if card == nil {
		t.Fatal("Should not return nil for empty paradigm")
	}
	if card.ParadigmID != "" {
		t.Error("Empty paradigm should have empty ID")
	}
	if card.InSample.SampleSize != 0 {
		t.Error("Should have zero sample size")
	}
}

func TestBuildFromParadigmLargeValues(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{ID: "large", Side: "buy"}

	bt := &BacktestResult{
		SampleSize:  10000,
		WinRate5:    0.80,
		WinRate20:   0.90,
		AvgReturn5:  1.0,  // 100% return
		AvgReturn20: 5.0,  // 500% return
		MaxDrawdown: 0.50, // 50% drawdown
	}

	card := builder.BuildFromParadigm(p, bt)

	if card == nil {
		t.Fatal("Should not return nil")
	}
	// 年化收益应该基于 252 天
	expectedAnnual := 5.0 * 252 / 20
	if card.OutOfSample.AnnualReturn != expectedAnnual {
		t.Errorf("AnnualReturn = %f, want %f", card.OutOfSample.AnnualReturn, expectedAnnual)
	}
}

func TestCounterEvidenceWithNoInvalidationAndPositiveWinRateDiff(t *testing.T) {
	builder := NewEvidenceBuilder()
	p := &Paradigm{ID: "no-inv", Side: "buy"}

	bt := &BacktestResult{
		SampleSize: 50,
		WinRate5:   0.60,
		WinRate20:  0.40, // 短周期胜率高于长周期 → 无反证
	}

	ev := builder.generateCounterEvidence(p, bt)

	// 短周期胜率高于长周期时不应有 fail_case
	for _, e := range ev {
		if e.Type == "fail_case" {
			t.Error("Should not have fail_case when short win rate > long win rate")
		}
	}
}
