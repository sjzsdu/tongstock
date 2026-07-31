package paradigms

import (
	"math"
	"testing"
	"time"
)

// ============================================================================
// 市场状态测试
// ============================================================================

func TestNewMarketState(t *testing.T) {
	state := &MarketState{
		Regime:        RegimeTrendUp,
		Volatility:    0.015,
		Liquidity:     2.0,
		MarketCap:     2000000000000, // 2万亿
		Industry:      "technology",
		Breadth:       0.65,
		TrendStrength: 45,
		Date:          time.Now(),
	}

	if !state.IsTrending() {
		t.Error("expected trending market")
	}
	if state.IsRangeBound() {
		t.Error("should not be range bound")
	}
	if state.IsHighVolatility() {
		t.Error("should not be high volatility")
	}
	if !state.IsLiquid() {
		t.Error("should be liquid")
	}
	if !state.IsLargeCap() {
		t.Error("should be large cap")
	}
	if !state.IsBull() {
		t.Error("should be bull")
	}
}

func TestMarketStateBear(t *testing.T) {
	state := &MarketState{
		Regime: RegimeTrendDown,
		Date:   time.Now(),
	}

	if !state.IsBear() {
		t.Error("expected bear market")
	}
	if state.IsBull() {
		t.Error("should not be bull")
	}
}

func TestMarketStateRange(t *testing.T) {
	state := &MarketState{
		Regime:     RegimeRange,
		Volatility: 0.005,
		Date:       time.Now(),
	}

	if state.IsTrending() {
		t.Error("should not be trending")
	}
	if !state.IsRangeBound() {
		t.Error("expected range bound")
	}
	if !state.IsLowVolatility() {
		t.Error("expected low volatility")
	}
}

// ============================================================================
// 上下文分层引擎测试
// ============================================================================

func TestNewContextLayerEngine(t *testing.T) {
	engine := NewContextLayerEngine()
	if engine == nil {
		t.Fatal("NewContextLayerEngine returned nil")
	}

	layers := engine.GetActiveLayers()
	if len(layers) < 3 {
		t.Errorf("expected at least 3 active layers, got %d", len(layers))
	}
}

func TestAnalyzeTrendingMarket(t *testing.T) {
	engine := NewContextLayerEngine()

	state := &MarketState{
		Regime:        RegimeTrendUp,
		Volatility:    0.015,
		Liquidity:     2.0,
		MarketCap:     2000000000000,
		Industry:      "technology",
		Breadth:       0.65,
		TrendStrength: 45,
		Date:          time.Now(),
	}

	results := engine.Analyze(state)
	if len(results) == 0 {
		t.Error("expected results")
	}

	// 检查趋势层是否匹配
	found := false
	for _, r := range results {
		if r.LayerID == "market_trend" && r.Matched {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected market_trend layer to match for trending market")
	}
}

func TestAnalyzeRangeMarket(t *testing.T) {
	engine := NewContextLayerEngine()

	state := &MarketState{
		Regime:        RegimeRange,
		Volatility:    0.005,
		Liquidity:     0.5,
		MarketCap:     50000000000,
		Industry:      "finance",
		Breadth:       0.35,
		TrendStrength: 15,
		Date:          time.Now(),
	}

	results := engine.Analyze(state)
	if len(results) == 0 {
		t.Error("expected results")
	}

	// 检查流动性层是否不匹配 (低流动性)
	for _, r := range results {
		if r.LayerID == "liquidity" && r.Matched {
			t.Error("liquidity should not match for low liquidity market")
		}
	}
}

func TestMatchContext(t *testing.T) {
	engine := NewContextLayerEngine()

	state := &MarketState{
		Regime:        RegimeTrendUp,
		Volatility:    0.025,
		Liquidity:     3.0,
		MarketCap:     5000000000000,
		Industry:      "technology",
		Breadth:       0.75,
		TrendStrength: 50,
		Date:          time.Now(),
	}

	// 要求趋势成立但高波动不成立
	conditions := map[string]bool{
		"market_trend": true,
		"volatility":   false,
	}

	results := engine.MatchContext(state, conditions)

	// market_trend 应该匹配 (因为 state.TrendStrength=50 > 25)
	found := false
	for _, r := range results {
		if r.LayerID == "market_trend" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected market_trend to match")
	}
}

func TestAddLayer(t *testing.T) {
	engine := NewContextLayerEngine()
	initialCount := len(engine.GetActiveLayers())

	newLayer := LayerDefinition{
		Layer: ContextLayer{
			ID:          "custom_layer",
			Name:        "自定义层",
			Description: "测试添加新层",
			Condition:   "custom > 0",
			Level:       1,
			Enabled:     true,
		},
		EvalFn: func(state *MarketState) bool {
			return state.MarketCap > 50000000000
		},
	}

	engine.AddLayer(newLayer)

	layers := engine.GetActiveLayers()
	if len(layers) != initialCount+1 {
		t.Errorf("expected %d layers, got %d", initialCount+1, len(layers))
	}
}

func TestRemoveLayer(t *testing.T) {
	engine := NewContextLayerEngine()
	initialCount := len(engine.GetActiveLayers())

	err := engine.RemoveLayer("liquidity")
	if err != nil {
		t.Fatalf("RemoveLayer failed: %v", err)
	}

	layers := engine.GetActiveLayers()
	if len(layers) != initialCount-1 {
		t.Errorf("expected %d layers, got %d", initialCount-1, len(layers))
	}
}

func TestRemoveNonExistentLayer(t *testing.T) {
	engine := NewContextLayerEngine()

	err := engine.RemoveLayer("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent layer")
	}
}

func TestSetThreshold(t *testing.T) {
	engine := NewContextLayerEngine()

	engine.SetThreshold("trend_strength", 50)
	if engine.thresholds["trend_strength"] != 50 {
		t.Errorf("expected threshold 50, got %f", engine.thresholds["trend_strength"])
	}
}

// ============================================================================
// 环境画像测试
// ============================================================================

func TestNewEnvironmentProfile(t *testing.T) {
	profile := NewEnvironmentProfile()
	if profile == nil {
		t.Fatal("NewEnvironmentProfile returned nil")
	}
	if len(profile.LayerResults) != 0 {
		t.Error("expected empty layer results")
	}
	if len(profile.Performance) != 0 {
		t.Error("expected empty performance")
	}
}

func TestAddLayerResult(t *testing.T) {
	profile := NewEnvironmentProfile()

	result := LayerResult{
		LayerID:   "market_trend",
		LayerName: "市场趋势",
		Level:     1,
		Matched:   true,
	}

	profile.AddLayerResult(result)
	if len(profile.LayerResults) != 1 {
		t.Errorf("expected 1 layer result, got %d", len(profile.LayerResults))
	}
}

func TestAddPerformance(t *testing.T) {
	profile := NewEnvironmentProfile()

	perf := LayerPerformance{
		LayerID:     "market_trend",
		LayerName:   "市场趋势",
		Count:       50,
		WinRate:     0.55,
		AvgReturn:   0.08,
		SharpeRatio: 1.5,
		SampleSize:  252,
		Confidence:  0.8,
	}

	profile.AddPerformance(perf)
	if len(profile.Performance) != 1 {
		t.Errorf("expected 1 performance, got %d", len(profile.Performance))
	}
}

func TestFindBestEnvironment(t *testing.T) {
	profile := NewEnvironmentProfile()

	profile.AddPerformance(LayerPerformance{
		LayerID:     "trend_up",
		LayerName:   "上升趋势",
		SharpeRatio: 2.0,
		SampleSize:  50,
	})

	profile.AddPerformance(LayerPerformance{
		LayerID:     "range",
		LayerName:   "震荡",
		SharpeRatio: 0.5,
		SampleSize:  100,
	})

	profile.AddPerformance(LayerPerformance{
		LayerID:     "trend_down",
		LayerName:   "下降趋势",
		SharpeRatio: -1.0,
		SampleSize:  30,
	})

	best := profile.FindBestEnvironment()
	if best == nil {
		t.Fatal("expected best environment")
	}
	if best.LayerID != "trend_up" {
		t.Errorf("expected trend_up as best, got %s", best.LayerID)
	}
}

func TestFindWorstEnvironment(t *testing.T) {
	profile := NewEnvironmentProfile()

	profile.AddPerformance(LayerPerformance{
		LayerID:     "trend_up",
		LayerName:   "上升趋势",
		SharpeRatio: 2.0,
	})

	profile.AddPerformance(LayerPerformance{
		LayerID:     "range",
		LayerName:   "震荡",
		SharpeRatio: 0.5,
	})

	profile.AddPerformance(LayerPerformance{
		LayerID:     "trend_down",
		LayerName:   "下降趋势",
		SharpeRatio: -1.0,
	})

	worst := profile.FindWorstEnvironment()
	if worst == nil {
		t.Fatal("expected worst environment")
	}
	if worst.LayerID != "trend_down" {
		t.Errorf("expected trend_down as worst, got %s", worst.LayerID)
	}
}

func TestGetFavorableEnvironments(t *testing.T) {
	profile := NewEnvironmentProfile()

	profile.AddPerformance(LayerPerformance{
		LayerID:     "good",
		LayerName:   "有利环境",
		SharpeRatio: 2.0,
		SampleSize:  50,
	})

	profile.AddPerformance(LayerPerformance{
		LayerID:     "bad",
		LayerName:   "不利环境",
		SharpeRatio: 0.1,
		SampleSize:  30,
	})

	favorable := profile.GetFavorableEnvironments(0.5)
	if len(favorable) != 1 {
		t.Errorf("expected 1 favorable environment, got %d", len(favorable))
	}
	if favorable[0].LayerID != "good" {
		t.Error("expected good environment")
	}
}

func TestGetUnfavorableEnvironments(t *testing.T) {
	profile := NewEnvironmentProfile()

	profile.AddPerformance(LayerPerformance{
		LayerID:     "good",
		LayerName:   "有利环境",
		SharpeRatio: 2.0,
		SampleSize:  50,
	})

	profile.AddPerformance(LayerPerformance{
		LayerID:     "bad",
		LayerName:   "不利环境",
		SharpeRatio: -0.5,
		SampleSize:  30,
	})

	unfavorable := profile.GetUnfavorableEnvironments(0)
	if len(unfavorable) != 1 {
		t.Errorf("expected 1 unfavorable environment, got %d", len(unfavorable))
	}
	if unfavorable[0].LayerID != "bad" {
		t.Error("expected bad environment")
	}
}

// ============================================================================
// 分层报告测试
// ============================================================================

func TestGenerateLayeredReport(t *testing.T) {
	layers := []LayerPerformance{
		{
			LayerID:     "trend_up",
			LayerName:   "上升趋势",
			Count:       100,
			WinRate:     0.6,
			AvgReturn:   0.12,
			MaxDrawdown: 0.08,
			SharpeRatio: 1.8,
			SampleSize:  200,
			Confidence:  0.85,
		},
		{
			LayerID:     "range",
			LayerName:   "震荡",
			Count:       80,
			WinRate:     0.5,
			AvgReturn:   0.03,
			MaxDrawdown: 0.05,
			SharpeRatio: 0.5,
			SampleSize:  150,
			Confidence:  0.6,
		},
		{
			LayerID:     "trend_down",
			LayerName:   "下降趋势",
			Count:       50,
			WinRate:     0.4,
			AvgReturn:   -0.05,
			MaxDrawdown: 0.15,
			SharpeRatio: -0.8,
			SampleSize:  100,
			Confidence:  0.7,
		},
	}

	report := GenerateLayeredReport(layers, "测试分层报告")
	if report == nil {
		t.Fatal("GenerateLayeredReport returned nil")
	}

	if report.Summary.BestLayer != "上升趋势" {
		t.Errorf("expected best layer 上升趋势, got %s", report.Summary.BestLayer)
	}

	if report.Summary.WorstLayer != "下降趋势" {
		t.Errorf("expected worst layer 下降趋势, got %s", report.Summary.WorstLayer)
	}

	if report.Summary.ValidCount == 0 {
		t.Error("expected some valid layers")
	}
}

func TestGenerateLayeredReportEmpty(t *testing.T) {
	report := GenerateLayeredReport([]LayerPerformance{}, "空报告")
	if report == nil {
		t.Fatal("GenerateLayeredReport returned nil")
	}
	if len(report.Layers) != 0 {
		t.Error("expected empty layers")
	}
}

func TestLayerPerformanceIsValid(t *testing.T) {
	valid := LayerPerformance{
		SampleSize: 200,
		Confidence: 0.85,
	}
	if !valid.IsValid() {
		t.Error("expected valid performance")
	}

	invalid := LayerPerformance{
		SampleSize: 10,
		Confidence: 0.3,
	}
	if invalid.IsValid() {
		t.Error("expected invalid performance")
	}
}

// ============================================================================
// 未来数据检查测试
// ============================================================================

func TestNewLookAheadValidator(t *testing.T) {
	validator := NewLookAheadValidator()
	if validator == nil {
		t.Fatal("NewLookAheadValidator returned nil")
	}
}

func TestValidateContextDataNoLeak(t *testing.T) {
	validator := NewLookAheadValidator()

	stateDate := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	dataDate := time.Date(2023, 5, 31, 0, 0, 0, 0, time.UTC)

	err := validator.ValidateContextData(stateDate, dataDate)
	if err != nil {
		t.Errorf("expected no error for past data: %v", err)
	}
}

func TestValidateContextDataLeak(t *testing.T) {
	validator := NewLookAheadValidator()

	stateDate := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	dataDate := time.Date(2023, 6, 2, 0, 0, 0, 0, time.UTC) // 未来数据

	err := validator.ValidateContextData(stateDate, dataDate)
	if err == nil {
		t.Error("expected error for future data leak")
	}
}

func TestValidateContextDataWithLookAhead(t *testing.T) {
	validator := NewLookAheadValidator()
	validator.SetMaxLookAhead(24 * time.Hour) // 允许 1 天的前视期

	stateDate := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	dataDate := time.Date(2023, 6, 1, 12, 0, 0, 0, time.UTC) // 同一天内

	err := validator.ValidateContextData(stateDate, dataDate)
	if err != nil {
		t.Errorf("expected no error with look-ahead: %v", err)
	}
}

func TestValidateNoPostHocOpt(t *testing.T) {
	validator := NewLookAheadValidator()

	layers := []LayerDefinition{
		{
			Layer: ContextLayer{
				ID:        "test_layer",
				Condition: "test > 0",
			},
			EvalFn: func(state *MarketState) bool { return true },
		},
	}

	state := &MarketState{
		Date: time.Now(),
	}

	err := validator.ValidateNoPostHocOpt(layers, state)
	if err != nil {
		t.Errorf("expected no error: %v", err)
	}
}

func TestValidateNoPostHocOptMissingCondition(t *testing.T) {
	validator := NewLookAheadValidator()

	layers := []LayerDefinition{
		{
			Layer: ContextLayer{
				ID:        "test_layer",
				Condition: "", // 缺少条件
			},
			EvalFn: func(state *MarketState) bool { return true },
		},
	}

	state := &MarketState{
		Date: time.Now(),
	}

	err := validator.ValidateNoPostHocOpt(layers, state)
	if err == nil {
		t.Error("expected error for missing condition")
	}
}

func TestSetForbidPostHocOpt(t *testing.T) {
	validator := NewLookAheadValidator()
	validator.SetForbidPostHocOpt(false)

	// 允许事后最优检查
	layers := []LayerDefinition{
		{
			Layer: ContextLayer{
				ID:        "test_layer",
				Condition: "",
			},
			EvalFn: func(state *MarketState) bool { return true },
		},
	}

	state := &MarketState{
		Date: time.Now(),
	}

	err := validator.ValidateNoPostHocOpt(layers, state)
	if err != nil {
		t.Errorf("expected no error when post-hoc is allowed: %v", err)
	}
}

// ============================================================================
// 实用函数测试
// ============================================================================

func TestCalculateADX(t *testing.T) {
	highs := []float64{100, 102, 105, 103, 108, 110, 112, 115, 118, 120}
	lows := []float64{98, 100, 103, 101, 106, 108, 110, 113, 116, 118}

	adx := CalculateADX(highs, lows, 5)
	if adx < 0 || adx > 100 {
		t.Errorf("ADX should be between 0 and 100, got %f", adx)
	}
}

func TestCalculateADXInsufficientData(t *testing.T) {
	highs := []float64{100, 102}
	lows := []float64{98, 100}

	adx := CalculateADX(highs, lows, 5)
	if adx != 0 {
		t.Errorf("expected 0 for insufficient data, got %f", adx)
	}
}

func TestCalculateVolatility(t *testing.T) {
	returns := []float64{0.01, -0.02, 0.03, -0.01, 0.02}

	vol := CalculateVolatility(returns)
	if vol <= 0 {
		t.Error("expected positive volatility")
	}
}

func TestCalculateVolatilityEmpty(t *testing.T) {
	vol := CalculateVolatility([]float64{})
	if vol != 0 {
		t.Errorf("expected 0 for empty data, got %f", vol)
	}
}

func TestCalculateBreadth(t *testing.T) {
	returns := []float64{0.01, -0.02, 0.03, -0.01, 0.02, 0.01, -0.01}

	breadth := CalculateBreadth(returns)
	if breadth < 0 || breadth > 1 {
		t.Errorf("breadth should be between 0 and 1, got %f", breadth)
	}

	// 4 out of 7 are positive
	if math.Abs(breadth-4.0/7.0) > 0.01 {
		t.Errorf("expected breadth ~0.57, got %f", breadth)
	}
}

func TestCalculateBreadthEmpty(t *testing.T) {
	breadth := CalculateBreadth([]float64{})
	if breadth != 0 {
		t.Errorf("expected 0 for empty data, got %f", breadth)
	}
}
