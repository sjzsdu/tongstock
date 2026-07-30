package quality

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"
)

// GoldenTestSpec 黄金回测试验规格。
// 每一条规格定义了一组输入数据和预期输出，用于验证回测引擎的正确性。
type GoldenTestSpec struct {
	ID           string    `json:"id"`
	Description  string    `json:"description"`
	InputData    []KlineRecord `json:"input_data"`
	StrategyName string    `json:"strategy_name"`
	ExpectedReturn    float64 `json:"expected_return"`
	Tolerance         float64 `json:"tolerance"` // 允许的误差范围
	ExpectedTrades    int     `json:"expected_trades"`
	ExpectedWinRate   float64 `json:"expected_win_rate"`
}

// GoldenTestResult 单个黄金测试用例的执行结果。
type GoldenTestResult struct {
	SpecID     string  `json:"spec_id"`
	Passed     bool    `json:"passed"`
	ActualReturn    float64 `json:"actual_return"`
	ActualTrades    int     `json:"actual_trades"`
	ActualWinRate   float64 `json:"actual_win_rate"`
	ReturnDiff      float64 `json:"return_diff"`
	Message         string  `json:"message"`
}

// BacktestEngine 回测引擎抽象接口。
// 由实际回测实现（如 baseline.RunBacktest）来满足。
type BacktestEngine interface {
	Run(ctx context.Context, bars []KlineRecord, strategyName string) (BacktestRunResult, error)
}

// BacktestRunResult 单次回测执行结果。
type BacktestRunResult struct {
	TotalReturn  float64 `json:"total_return"`
	AnnualReturn float64 `json:"annual_return"`
	NumTrades    int     `json:"num_trades"`
	WinRate      float64 `json:"win_rate"`
	EquityCurve  []float64 `json:"equity_curve"`
}

// GoldenBacktestRunner 黄金回测运行器。
// 执行一组黄金测试用例，对比实际回测结果与预期值。
type GoldenBacktestRunner struct {
	engine  BacktestEngine
	specs   []GoldenTestSpec
}

// NewGoldenBacktestRunner 创建黄金回测运行器。
func NewGoldenBacktestRunner(engine BacktestEngine, specs []GoldenTestSpec) *GoldenBacktestRunner {
	return &GoldenBacktestRunner{engine: engine, specs: specs}
}

// RunAll 运行所有黄金测试用例，返回聚合结果供质量门使用。
func (r *GoldenBacktestRunner) RunAll(ctx context.Context) (*BacktestGoldenResult, []GoldenTestResult) {
	var testResults []GoldenTestResult
	passed := 0
	failed := 0

	for _, spec := range r.specs {
		tr := GoldenTestResult{
			SpecID: spec.ID,
			Message: spec.Description,
		}

		result, err := r.engine.Run(ctx, spec.InputData, spec.StrategyName)
		if err != nil {
			tr.Passed = false
			tr.Message = fmt.Sprintf("%s: 执行错误: %v", spec.Description, err)
			failed++
			testResults = append(testResults, tr)
			continue
		}

		tr.ActualReturn = result.TotalReturn
		tr.ActualTrades = result.NumTrades
		tr.ActualWinRate = result.WinRate
		tr.ReturnDiff = result.TotalReturn - spec.ExpectedReturn

		returnOk := abs(tr.ReturnDiff) <= spec.Tolerance
		tradesOk := spec.ExpectedTrades <= 0 || result.NumTrades == spec.ExpectedTrades
		winRateOk := spec.ExpectedWinRate <= 0 || abs(result.WinRate-spec.ExpectedWinRate) <= 0.01

		tr.Passed = returnOk && tradesOk && winRateOk
		if tr.Passed {
			passed++
		} else {
			failed++
			tr.Message = fmt.Sprintf("%s: 实际收益 %.4f (预期 %.4f, 差 %.4f), 交易数 %d (预期 %d), 胜率 %.2f (预期 %.2f)",
				spec.Description, result.TotalReturn, spec.ExpectedReturn, tr.ReturnDiff,
				result.NumTrades, spec.ExpectedTrades, result.WinRate, spec.ExpectedWinRate)
		}

		testResults = append(testResults, tr)
	}

	// 计算测试哈希（用于回归检测）
	hash := r.computeHash(testResults)

	return &BacktestGoldenResult{
		TestPassed:  failed == 0,
		TestCount:   len(r.specs),
		FailCount:   failed,
		TestHash:    hash,
		GoldenHash:  "", // 黄金哈希由 GoldenSet 提供
		Regressed:   false,
		Description: fmt.Sprintf("黄金回测: %d/%d 通过", passed, len(r.specs)),
	}, testResults
}

// computeHash 计算测试结果的哈希值，用于回归检测。
func (r *GoldenBacktestRunner) computeHash(results []GoldenTestResult) string {
	h := sha256.New()
	for _, res := range results {
		fmt.Fprintf(h, "%s:%v:%.6f:%d:%.4f",
			res.SpecID, res.Passed, res.ActualReturn, res.ActualTrades, res.ActualWinRate)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// GoldenSet 黄金测试集定义。
// 包含预设的测试规格和基准哈希。
type GoldenSet struct {
	ID        string           `json:"id"`
	Version   string           `json:"version"`
	Specs     []GoldenTestSpec `json:"specs"`
	GoldenHash string          `json:"golden_hash"` // 基准哈希，用于回归检测
	CreatedAt time.Time        `json:"created_at"`
}

// DefaultGoldenSet 返回默认黄金测试集。
func DefaultGoldenSet() *GoldenSet {
	return &GoldenSet{
		ID:      "default-golden",
		Version: "1.0",
		Specs: []GoldenTestSpec{
			{
				ID: "buy_and_hold_5day",
				Description: "买入持有 5 天 (价格线性上涨)",
				InputData: generateLinearBars(5, 10.0, 0.5),
				StrategyName: "buy_and_hold",
				ExpectedReturn: 0.1928,
				Tolerance:      0.001,
				ExpectedTrades: 1,
			},
			{
				ID: "buy_and_hold_10day",
				Description: "买入持有 10 天 (价格线性上涨)",
				InputData: generateLinearBars(10, 10.0, 0.3),
				StrategyName: "buy_and_hold",
				ExpectedReturn: 0.2634,
				Tolerance:      0.001,
				ExpectedTrades: 1,
			},
			{
				ID: "buy_and_hold_flat",
				Description: "买入持有 (价格不变)",
				InputData: generateLinearBars(5, 10.0, 0.0),
				StrategyName: "buy_and_hold",
				ExpectedReturn: -0.0012,
				Tolerance:      0.005,
				ExpectedTrades: 1,
			},
			{
				ID: "buy_and_hold_declining",
				Description: "买入持有 (价格线性下跌)",
				InputData: generateLinearBars(5, 10.0, -0.3),
				StrategyName: "buy_and_hold",
				ExpectedReturn: -0.1225,
				Tolerance:      0.01,
				ExpectedTrades: 1,
			},
		},
		GoldenHash:  "", // 需要通过首次运行生成
		CreatedAt:   time.Now(),
	}
}

// generateLinearBars 生成线性趋势的 K 线数据。
func generateLinearBars(n int, startPrice, step float64) []KlineRecord {
	bars := make([]KlineRecord, n)
	for i := 0; i < n; i++ {
		price := startPrice + step*float64(i)
		bars[i] = KlineRecord{
			Date:  time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.Local),
			Open:  price,
			High:  price + 0.5,
			Low:   price - 0.3,
			Close: price + step*0.5,
			Volume: 10000,
		}
	}
	return bars
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
