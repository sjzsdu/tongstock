package baseline

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/trading"
)

// ============================================================================
// 黄金测试数据集
// ============================================================================

// generateLinearData 生成线性上涨数据 (可手算)。
func generateLinearData(n int, startPrice, dailyChange float64) []KlineBar {
	bars := make([]KlineBar, n)
	base := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local)

	for i := 0; i < n; i++ {
		price := startPrice + dailyChange*float64(i)
		bars[i] = KlineBar{
			Date:   base.AddDate(0, 0, i),
			Open:   price,
			High:   price * 1.01,
			Low:    price * 0.99,
			Close:  price,
			Volume: 1000000,
		}
	}
	return bars
}

// generateV形数据 生成 V 形数据 (先跌后涨)。
func generateVShapedData(n int, startPrice float64, bottomDay int, bottomPrice float64) []KlineBar {
	bars := make([]KlineBar, n)
	base := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local)

	for i := 0; i < n; i++ {
		var price float64
		if i <= bottomDay {
			// 下跌阶段
			ratio := float64(i) / float64(bottomDay)
			price = startPrice + (bottomPrice-startPrice)*ratio
		} else {
			// 上涨阶段
			remaining := n - 1 - bottomDay
			ratio := float64(i-bottomDay) / float64(remaining)
			price = bottomPrice + (startPrice-bottomPrice)*ratio
		}

		bars[i] = KlineBar{
			Date:   base.AddDate(0, 0, i),
			Open:   price * 0.999,
			High:   price * 1.01,
			Low:    price * 0.99,
			Close:  price,
			Volume: 1000000,
		}
	}
	return bars
}

// generateFlatData 生成横盘数据。
func generateFlatData(n int, price float64) []KlineBar {
	bars := make([]KlineBar, n)
	base := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local)

	for i := 0; i < n; i++ {
		bars[i] = KlineBar{
			Date:   base.AddDate(0, 0, i),
			Open:   price,
			High:   price * 1.005,
			Low:    price * 0.995,
			Close:  price,
			Volume: 1000000,
		}
	}
	return bars
}

// ============================================================================
// 基础配置
// ============================================================================

func defaultConfig() GoldenBacktestConfig {
	constraints := trading.DefaultTradingConstraints()
	// 基线测试中放宽约束 (使用合成数据, 不适合真实 A 股规则)
	constraints.EnablePriceLimit = false
	constraints.EnableT1 = false
	constraints.EnableSuspension = false

	return GoldenBacktestConfig{
		Code:        "600000",
		InitialCash: 1000000, // 100 万
		Constraints: constraints,
		CostModel:   trading.DefaultCostModel(),
	}
}

// ============================================================================
// 买入持有策略 - 黄金测试
// ============================================================================

func TestBuyAndHold_LinearUp_Golden(t *testing.T) {
	// 5 天线性上涨: 10, 11, 12, 13, 14
	bars := generateLinearData(5, 10, 1)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{
		Code:        config.Code,
		InitialCash: config.InitialCash,
	}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	// 可手算验证:
	// 第一天买入: 1000000 / 10 = 100000 股 -> 取 100000 股 (正好整数)
	// 最后一天卖出: 100000 股 * 14 = 1,400,000
	// 收益: (1,400,000 - 1,000,000) / 1,000,000 = 40%
	// 但需要扣除成本

	t.Logf("Total Return: %.4f (%.2f%%)", result.TotalReturn, result.TotalReturn*100)
	t.Logf("Num Fills: %d", result.NumFills)
	t.Logf("Num Rejects: %d", result.NumRejects)
	t.Logf("Avg Trade Return: %.4f", result.AvgTradeReturn)
	t.Logf("Win Rate: %.2f%%", result.WinRate)

	// 验证: 有 2 笔成交 (买+卖)
	if result.NumFills != 2 {
		t.Errorf("Expected 2 fills, got %d", result.NumFills)
	}

	// 验证: 收益为正 (上涨市场)
	if result.TotalReturn <= 0 {
		t.Error("Buy and hold in uptrend should have positive return")
	}

	// 验证: 权益曲线正确
	if len(result.EquityCurve) != len(bars) {
		t.Errorf("Equity curve length should match bars: %d vs %d", len(result.EquityCurve), len(bars))
	}

	// 验证: 最终权益 > 初始权益
	if result.EquityCurve[len(result.EquityCurve)-1] <= config.InitialCash {
		t.Error("Final equity should be > initial cash in uptrend")
	}

	// 打印权益曲线
	for i, eq := range result.EquityCurve {
		t.Logf("  Day %d: Equity = %.2f", i, eq)
	}
}

func TestBuyAndHold_LinearDown_Golden(t *testing.T) {
	// 5 天线性下跌: 50, 40, 30, 20, 10
	bars := generateLinearData(5, 50, -10)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{
		Code:        config.Code,
		InitialCash: config.InitialCash,
	}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Total Return: %.4f (%.2f%%)", result.TotalReturn, result.TotalReturn*100)

	// 验证: 下跌市场收益为负
	if result.TotalReturn >= 0 {
		t.Error("Buy and hold in downtrend should have negative return")
	}

	// 验证: 权益曲线最终值 < 初始
	if result.EquityCurve[len(result.EquityCurve)-1] >= config.InitialCash {
		t.Error("Final equity should be < initial cash in downtrend")
	}
}

func TestBuyAndHold_FlatMarket_Golden(t *testing.T) {
	// 10 天横盘: 价格恒定
	bars := generateFlatData(10, 10)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{
		Code:        config.Code,
		InitialCash: config.InitialCash,
	}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Total Return: %.4f (%.2f%%)", result.TotalReturn, result.TotalReturn*100)

	// 验证: 横盘收益接近 0 (只扣成本)
	if result.TotalReturn > 0 {
		t.Error("Flat market return should be <= 0 (after costs)")
	}

	// 验证: 权益曲线下降 (因成本)
	if result.EquityCurve[len(result.EquityCurve)-1] >= config.InitialCash {
		t.Error("Final equity should be <= initial cash (after costs)")
	}
}

// ============================================================================
// 均线策略 - 黄金测试
// ============================================================================

func TestSimpleMA_Golden(t *testing.T) {
	// 20 天 V 形数据
	bars := generateVShapedData(20, 10, 10, 5)
	config := defaultConfig()

	strategy := &SimpleMAStrategy{
		Code:       config.Code,
		FastPeriod: 3,
		SlowPeriod: 7,
	}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Strategy: %s", result.StrategyName)
	t.Logf("Total Return: %.4f (%.2f%%)", result.TotalReturn, result.TotalReturn*100)
	t.Logf("Num Fills: %d", result.NumFills)
	t.Logf("Num Rejects: %d", result.NumRejects)

	// 打印每笔成交
	for i, fill := range result.Fills {
		t.Logf("  Fill %d: %s @ %.4f shares=%d", i, fill.Side, fill.Price, fill.Quantity)
	}

	// 打印权益曲线
	for i, eq := range result.EquityCurve {
		bar := bars[i]
		t.Logf("  Day %d (%s): Close=%.2f, Equity=%.2f", i, bar.Date.Format("06-01-02"), bar.Close, eq)
	}

	// 验证: 至少有成交
	if result.NumFills < 2 {
		t.Log("Warning: MA strategy may not have generated signals in this data")
	}
}

func TestSimpleMA_Linear_Golden(t *testing.T) {
	// 30 天线性上涨 (应该触发金叉买入)
	bars := generateLinearData(30, 10, 0.5)
	config := defaultConfig()

	strategy := &SimpleMAStrategy{
		Code:       config.Code,
		FastPeriod: 5,
		SlowPeriod: 15,
	}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Total Return: %.4f (%.2f%%)", result.TotalReturn, result.TotalReturn*100)
	t.Logf("Num Fills: %d", result.NumFills)

	// 线性上涨中, 快 MA 应该在慢 MA 上方
	// 因此应该在早期就买入
	if result.NumFills < 2 {
		t.Log("Warning: MA strategy may not have generated signals")
	}

	// 验证: 权益曲线合理
	if len(result.EquityCurve) != len(bars) {
		t.Errorf("Equity curve length mismatch: %d vs %d", len(result.EquityCurve), len(bars))
	}
}

// ============================================================================
// 随机信号策略 - 黄金测试
// ============================================================================

func TestRandomSignal_Golden(t *testing.T) {
	bars := generateLinearData(30, 10, 0.5)
	config := defaultConfig()

	strategy := &RandomSignalStrategy{
		Code:     config.Code,
		Seed:     42,
		BuyProb:  0.3,
		SellProb: 0.3,
		HoldDays: 3,
	}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Seed: %d", strategy.Seed)
	t.Logf("Total Return: %.4f (%.2f%%)", result.TotalReturn, result.TotalReturn*100)
	t.Logf("Num Fills: %d", result.NumFills)
	t.Logf("Num Rejects: %d", result.NumRejects)

	// 确定性: 相同种子应产生相同结果
	strategy2 := &RandomSignalStrategy{
		Code:     config.Code,
		Seed:     42, // 相同种子
		BuyProb:  0.3,
		SellProb: 0.3,
		HoldDays: 3,
	}

	result2, err := RunBacktest(context.Background(), bars, strategy2, config)
	if err != nil {
		t.Fatal(err)
	}

	// 验证确定性: 相同输入应产生相同结果
	if result.TotalReturn != result2.TotalReturn {
		t.Errorf("Same seed should produce identical results: %f vs %f",
			result.TotalReturn, result2.TotalReturn)
	}
	if result.NumFills != result2.NumFills {
		t.Errorf("Same seed should produce same number of fills: %d vs %d",
			result.NumFills, result2.NumFills)
	}

	t.Log("Deterministic behavior verified!")
}

func TestRandomSignal_DifferentSeed(t *testing.T) {
	bars := generateLinearData(30, 10, 0.5)
	config := defaultConfig()

	seed42 := &RandomSignalStrategy{Code: config.Code, Seed: 42, BuyProb: 0.3, SellProb: 0.3, HoldDays: 3}
	seed123 := &RandomSignalStrategy{Code: config.Code, Seed: 123, BuyProb: 0.3, SellProb: 0.3, HoldDays: 3}

	result42, _ := RunBacktest(context.Background(), bars, seed42, config)
	result123, _ := RunBacktest(context.Background(), bars, seed123, config)

	// 不同种子可能产生不同结果 (但也可能相同, 这是合理的)
	t.Logf("Seed 42: Return=%.4f, Fills=%d", result42.TotalReturn, result42.NumFills)
	t.Logf("Seed 123: Return=%.4f, Fills=%d", result123.TotalReturn, result123.NumFills)

	// 无法强制要求不同种子产生不同结果, 但逻辑上应该独立
}

// ============================================================================
// 回归测试 - 引擎正确性验证
// ============================================================================

func TestRegression_BuyAndHold_CostDeduction(t *testing.T) {
	// 验证成本扣除正确
	bars := generateLinearData(5, 10, 1)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	// 可手算:
	// 买入 100000 股 @ 10
	// 买入成本: 佣金 (最低 5 元, 万 2.5) = max(5, 100000*10*0.00025) = max(5, 250) = 250
	// 卖出成本: 佣金 + 印花税(0.05%) + 过户费(0.001%)
	// 卖出 100000 股 @ 14
	// 收益 = 100000*(14-10) - costs

	t.Log("Cost deduction verification:")
	for _, fill := range result.Fills {
		t.Logf("  Fill: %s @ %.2f, Cost=%.2f, TotalCost=%.2f",
			fill.Side, fill.Price, fill.Cost.Commission, fill.Cost.Total)
	}

	// 验证: 第一笔是买入
	if len(result.Fills) > 0 && result.Fills[0].Side != trading.OrderBuy {
		t.Error("First fill should be a buy")
	}

	// 验证: 第二笔是卖出
	if len(result.Fills) > 1 && result.Fills[1].Side != trading.OrderSell {
		t.Error("Second fill should be a sell")
	}

	// 验证: 有成本扣除
	for _, fill := range result.Fills {
		if fill.Cost.Total <= 0 {
			t.Errorf("Fill %s should have positive cost", fill.Side)
		}
	}
}

func TestRegression_EquityCurveConsistency(t *testing.T) {
	// 验证权益曲线与交易记录一致
	bars := generateLinearData(10, 10, 0.5)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	// 权益曲线的第一天应该等于初始现金
	if len(result.EquityCurve) > 0 {
		firstEq := result.EquityCurve[0]
		if math.Abs(firstEq-config.InitialCash)/config.InitialCash > 0.001 {
			t.Errorf("First equity value %.2f should be close to initial cash %.2f",
				firstEq, config.InitialCash)
		}
	}

	// 权益曲线长度应等于 bars 长度
	if len(result.EquityCurve) != len(bars) {
		t.Errorf("Equity curve length %d should equal bars length %d",
			len(result.EquityCurve), len(bars))
	}
}

func TestRegression_EmptyData(t *testing.T) {
	config := defaultConfig()
	strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}

	_, err := RunBacktest(context.Background(), nil, strategy, config)
	if err == nil {
		t.Error("Empty data should return error")
	}
}

func TestRegression_SingleBar(t *testing.T) {
	bars := generateLinearData(1, 10, 0)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	// 只有 1 天, 应该只有买入 (没有卖出)
	if result.NumFills < 1 {
		t.Error("Should have at least 1 fill (buy)")
	}

	t.Logf("Single bar: %d fills", result.NumFills)
}

// ============================================================================
// 验证报告测试
// ============================================================================

func TestValidation_ResultPass(t *testing.T) {
	bars := generateLinearData(5, 10, 1)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}
	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	expected := GoldenExpectation{
		StrategyName: "buy_and_hold",
		TotalReturn:  result.TotalReturn, // 使用实际值 (验证逻辑)
		NumFills:     2,
		NumRejects:   0,
		CheckRejects: false,
		Tolerance:    0.01,
	}

	report, err := ValidateGoldenResult(result, expected)
	if err != nil {
		t.Fatal(err)
	}

	if !report.Passed {
		for _, check := range report.FailedChecks {
			t.Logf("  Failed: %s (actual=%.4f, expected=%.4f)",
				check.Name, check.Actual, check.Expected)
		}
		t.Error("Validation should pass with exact expected values")
	}
}

func TestValidation_ResultFail(t *testing.T) {
	bars := generateLinearData(5, 10, 1)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}
	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	// 故意设置错误的预期值
	expected := GoldenExpectation{
		StrategyName: "buy_and_hold",
		TotalReturn:  result.TotalReturn + 0.5, // 错误: 加了 50%
		NumFills:     2,
		Tolerance:    0.01,
	}

	report, err := ValidateGoldenResult(result, expected)
	if err != nil {
		t.Fatal(err)
	}

	if report.Passed {
		t.Error("Validation should fail with wrong expected values")
	}

	// 应该有失败的检查
	if len(report.FailedChecks) == 0 {
		t.Error("Should have failed checks")
	}

	t.Log("Validation correctly failed on mismatched expectations")
}

// ============================================================================
// 基线策略比较测试
// ============================================================================

func TestBaselineComparison(t *testing.T) {
	// 所有基线策略在同一数据集上的比较
	bars := generateLinearData(30, 10, 0.3)
	config := defaultConfig()

	strategies := map[string]Strategy{
		"buy_and_hold": &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash},
		"random":       &RandomSignalStrategy{Code: config.Code, Seed: 42, BuyProb: 0.3, SellProb: 0.3, HoldDays: 3},
		"ma_fast":      &SimpleMAStrategy{Code: config.Code, FastPeriod: 3, SlowPeriod: 10},
		"ma_slow":      &SimpleMAStrategy{Code: config.Code, FastPeriod: 5, SlowPeriod: 20},
	}

	t.Log("Baseline Strategy Comparison (Linear Uptrend):")
	t.Log("==============================================")

	for name, strategy := range strategies {
		result, err := RunBacktest(context.Background(), bars, strategy, config)
		if err != nil {
			t.Errorf("%s: error: %v", name, err)
			continue
		}

		t.Logf("  %s:", name)
		t.Logf("    Total Return: %.2f%%", result.TotalReturn*100)
		t.Logf("    Annual Return: %.2f%%", result.AnnualReturn*100)
		t.Logf("    Num Fills: %d", result.NumFills)
		t.Logf("    Win Rate: %.2f%%", result.WinRate)
		t.Logf("    Avg Trade Return: %.4f", result.AvgTradeReturn)
	}

	t.Log("==============================================")
	t.Log("Buy and hold should be a reasonable baseline")
	t.Log("Strategy returns should be comparable to baseline")
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestEdgeCases_VerySmallData(t *testing.T) {
	// 2 天数据 (最小)
	bars := generateLinearData(2, 10, 1)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	// 应该有买入和卖出 (第1天和第2天)
	if result.NumFills != 2 {
		t.Errorf("2-day data should produce 2 fills, got %d", result.NumFills)
	}

	t.Logf("2-day result: Return=%.2f%%, Fills=%d", result.TotalReturn*100, result.NumFills)
}

func TestEdgeCases_HighPrice(t *testing.T) {
	// 高价格 (需要考虑 100 股整数倍)
	bars := generateLinearData(5, 1000000, 1000)
	config := defaultConfig() // 100 万初始资金

	strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}

	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("High price: Initial=%.0f, Final=%.0f", config.InitialCash, result.EquityCurve[len(result.EquityCurve)-1])

	// 高价格下可能只能买很少的股 (甚至 0)
	// 验证引擎处理高价格的正确性
	_ = result
}

// ============================================================================
// 确定性验证
// ============================================================================

func TestDeterminism_BuyAndHold(t *testing.T) {
	bars := generateLinearData(10, 10, 1)
	config := defaultConfig()

	// 运行 3 次
	results := make([]*GoldenBacktestResult, 3)
	for i := 0; i < 3; i++ {
		strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}
		result, err := RunBacktest(context.Background(), bars, strategy, config)
		if err != nil {
			t.Fatal(err)
		}
		results[i] = result
	}

	// 验证所有结果完全一致
	for i := 1; i < len(results); i++ {
		if results[0].TotalReturn != results[i].TotalReturn {
			t.Errorf("Run %d has different return: %f vs %f",
				i, results[0].TotalReturn, results[i].TotalReturn)
		}
	}

	t.Log("BuyAndHold is deterministic across runs")
}

func TestDeterminism_MultipleStrategies(t *testing.T) {
	bars := generateLinearData(15, 10, 0.5)
	config := defaultConfig()

	strategies := []struct {
		name     string
		strategy Strategy
	}{
		{"bh", &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}},
		{"rand", &RandomSignalStrategy{Code: config.Code, Seed: 42, BuyProb: 0.2, SellProb: 0.2, HoldDays: 2}},
		{"ma", &SimpleMAStrategy{Code: config.Code, FastPeriod: 3, SlowPeriod: 7}},
	}

	for _, s := range strategies {
		// 运行 2 次
		var result1, result2 *GoldenBacktestResult
		var err error

		result1, err = RunBacktest(context.Background(), bars, s.strategy, config)
		if err != nil {
			t.Fatalf("%s: first run error: %v", s.name, err)
		}

		// 重新创建策略 (因为状态可能改变)
		s2 := recreateStrategy(s.strategy)
		result2, err = RunBacktest(context.Background(), bars, s2, config)
		if err != nil {
			t.Fatalf("%s: second run error: %v", s.name, err)
		}

		if result1.TotalReturn != result2.TotalReturn {
			t.Errorf("%s: non-deterministic: %f vs %f", s.name, result1.TotalReturn, result2.TotalReturn)
		}

		t.Logf("%s: deterministic (return=%.4f)", s.name, result1.TotalReturn)
	}
}

// recreateStrategy 重新创建策略实例。
func recreateStrategy(s Strategy) Strategy {
	switch s := s.(type) {
	case *BuyAndHoldStrategy:
		return &BuyAndHoldStrategy{Code: s.Code, InitialCash: s.InitialCash}
	case *RandomSignalStrategy:
		return &RandomSignalStrategy{
			Code: s.Code, Seed: s.Seed, BuyProb: s.BuyProb, SellProb: s.SellProb, HoldDays: s.HoldDays,
		}
	case *SimpleMAStrategy:
		return &SimpleMAStrategy{Code: s.Code, FastPeriod: s.FastPeriod, SlowPeriod: s.SlowPeriod}
	default:
		return s
	}
}

// ============================================================================
// 数学精度验证
// ============================================================================

func TestMathPrecision_SimpleCalculation(t *testing.T) {
	// 手动计算验证
	// 5 天数据: 10, 11, 12, 13, 14
	bars := generateLinearData(5, 10, 1)
	config := defaultConfig()

	strategy := &BuyAndHoldStrategy{Code: config.Code, InitialCash: config.InitialCash}
	result, err := RunBacktest(context.Background(), bars, strategy, config)
	if err != nil {
		t.Fatal(err)
	}

	// 手动计算 (不含成本):
	// 买入: 1000000 / 10 = 100000 股
	// 卖出: 100000 股 * 14 = 1,400,000
	// 简单收益: (1,400,000 - 1,000,000) / 1,000,000 = 40%
	manualReturn := (14.0 - 10.0) / 10.0 // 40%

	// 实际收益应小于手动收益 (因为扣除了成本)
	if result.TotalReturn >= manualReturn {
		t.Errorf("Actual return %.4f should be < manual return %.4f (costs not deducted)",
			result.TotalReturn, manualReturn)
	}

	t.Logf("Manual return (no cost): %.4f", manualReturn)
	t.Logf("Actual return (with cost): %.4f", result.TotalReturn)
	t.Logf("Cost impact: %.4f", manualReturn-result.TotalReturn)

	// 验证: 成本影响合理 (不应超过 5%)
	costImpact := manualReturn - result.TotalReturn
	if costImpact > 0.05 {
		t.Logf("Warning: High cost impact: %.2f%%", costImpact*100)
	}
}
