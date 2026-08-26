package quality

import (
	"context"

	"github.com/sjzsdu/tongstock/internal/baseline"
	"github.com/sjzsdu/tongstock/internal/trading"
)

// BaselineEngineAdapter 将 baseline 黄金回测引擎适配为 BacktestEngine 接口。
type BaselineEngineAdapter struct {
	initialCash float64
	constraints trading.TradingConstraints
	costModel   trading.CostModel
}

// NewBaselineEngineAdapter 创建默认的 baseline 引擎适配器。
func NewBaselineEngineAdapter() *BaselineEngineAdapter {
	return &BaselineEngineAdapter{
		initialCash: 100000.0,
		constraints: trading.DefaultTradingConstraints(),
		costModel:   trading.DefaultCostModel(),
	}
}

// Run 实现 BacktestEngine 接口，将 KlineRecord 转换为 baseline.KlineBar 后执行回测。
func (a *BaselineEngineAdapter) Run(ctx context.Context, bars []KlineRecord, strategyName string) (BacktestRunResult, error) {
	if len(bars) == 0 {
		return BacktestRunResult{}, nil
	}

	// 转换数据格式
	baselineBars := make([]baseline.KlineBar, len(bars))
	for i, b := range bars {
		baselineBars[i] = baseline.KlineBar{
			Date:   b.Date,
			Open:   b.Open,
			High:   b.High,
			Low:    b.Low,
			Close:  b.Close,
			Volume: int64(b.Volume),
		}
	}

	// 选择策略
	var strategy baseline.Strategy
	switch strategyName {
	case "buy_and_hold":
		strategy = &baseline.BuyAndHoldStrategy{
			Code:        "sh000001",
			InitialCash: a.initialCash,
		}
	default:
		strategy = &baseline.BuyAndHoldStrategy{
			Code:        "sh000001",
			InitialCash: a.initialCash,
		}
	}

	config := baseline.GoldenBacktestConfig{
		Code:        "sh000001",
		InitialCash: a.initialCash,
		Constraints: a.constraints,
		CostModel:   a.costModel,
	}

	result, err := baseline.RunBacktest(ctx, baselineBars, strategy, config)
	if err != nil {
		return BacktestRunResult{}, err
	}

	return BacktestRunResult{
		TotalReturn:  result.TotalReturn,
		AnnualReturn: result.AnnualReturn,
		NumTrades:    result.NumTrades,
		WinRate:      result.WinRate,
		EquityCurve:  result.EquityCurve,
	}, nil
}
