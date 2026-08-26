// Package baseline 实现基线策略与黄金回测回归集。
//
// 基线策略用于:
//   - 验证回测引擎的正确性 (可手算核对)
//   - 为候选范式提供比较基准
//   - 每次引擎变更跑回归测试
//
// 包含:
//   - BuyAndHold: 买入持有
//   - RandomSignal: 随机信号
//   - SimpleMA: 简单均线交叉
package baseline

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/sjzsdu/tongstock/internal/trading"
)

// ============================================================================
// 策略接口
// ============================================================================

// KlineBar K线数据。
type KlineBar struct {
	Date   time.Time `json:"date"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume int64     `json:"volume"`
}

// Strategy 基线策略接口。
type Strategy interface {
	// Name 策略名称
	Name() string
	// GenerateSignals 生成交易信号
	// bars: 历史K线, bar: 当前K线
	// 返回: 操作 (buy/sell/hold)
	GenerateSignals(bars []KlineBar) []Signal
}

// Signal 交易信号。
type Signal struct {
	Date   time.Time         `json:"date"`
	Code   string            `json:"code"`
	Side   trading.OrderSide `json:"side"`
	Price  float64           `json:"price"`
	Reason string            `json:"reason"`
}

// ============================================================================
// 买入持有策略
// ============================================================================

// BuyAndHoldStrategy 买入持有策略。
type BuyAndHoldStrategy struct {
	Code        string  `json:"code"`
	InitialCash float64 `json:"initial_cash"`
}

func (s *BuyAndHoldStrategy) Name() string {
	return "buy_and_hold"
}

func (s *BuyAndHoldStrategy) GenerateSignals(bars []KlineBar) []Signal {
	if len(bars) < 1 {
		return nil
	}

	signals := make([]Signal, 0)
	// 第一天买入, 之后持有
	firstBar := bars[0]
	price := firstBar.Close

	// 计算可买股数 (考虑 100 股整数倍)
	shares := int(s.InitialCash/price/100) * 100
	if shares <= 0 {
		shares = 100
	}

	signals = append(signals, Signal{
		Date:   firstBar.Date,
		Code:   s.Code,
		Side:   trading.OrderBuy,
		Price:  price,
		Reason: "initial_buy",
	})

	// 最后一天卖出
	if len(bars) > 1 {
		lastBar := bars[len(bars)-1]
		signals = append(signals, Signal{
			Date:   lastBar.Date,
			Code:   s.Code,
			Side:   trading.OrderSell,
			Price:  lastBar.Close,
			Reason: "final_sell",
		})
	}

	return signals
}

// ============================================================================
// 随机信号策略
// ============================================================================

// RandomSignalStrategy 随机信号策略 (用于验证引擎随机性处理)。
type RandomSignalStrategy struct {
	Code      string  `json:"code"`
	Seed      int64   `json:"seed"`
	BuyProb   float64 `json:"buy_prob"`  // 买入概率
	SellProb  float64 `json:"sell_prob"` // 卖出概率
	HoldDays  int     `json:"hold_days"` // 最少持有天数
	position  bool    // 是否持仓
	lastTrade time.Time
}

func (s *RandomSignalStrategy) Name() string {
	return "random_signal"
}

func (s *RandomSignalStrategy) GenerateSignals(bars []KlineBar) []Signal {
	if len(bars) == 0 {
		return nil
	}

	signals := make([]Signal, 0)

	// 使用确定性伪随机 (基于日期和种子)
	for i, bar := range bars {
		if i == 0 {
			// 第一天买入
			signals = append(signals, Signal{
				Date:   bar.Date,
				Code:   s.Code,
				Side:   trading.OrderBuy,
				Price:  bar.Close,
				Reason: "day_1_buy",
			})
			s.position = true
			s.lastTrade = bar.Date
			continue
		}

		// 使用确定性哈希生成信号
		hash := deterministicHash(s.Seed, bar.Date)

		if s.position {
			// 持仓中: 根据卖出概率决定卖出
			if hash < s.SellProb*1000 {
				// 检查持有天数
				if bar.Date.Sub(s.lastTrade).Hours()/24 >= float64(s.HoldDays) {
					signals = append(signals, Signal{
						Date:   bar.Date,
						Code:   s.Code,
						Side:   trading.OrderSell,
						Price:  bar.Close,
						Reason: "random_sell",
					})
					s.position = false
					s.lastTrade = bar.Date
				}
			}
		} else {
			// 空仓: 根据买入概率决定买入
			if hash < s.BuyProb*1000 {
				signals = append(signals, Signal{
					Date:   bar.Date,
					Code:   s.Code,
					Side:   trading.OrderBuy,
					Price:  bar.Close,
					Reason: "random_buy",
				})
				s.position = true
				s.lastTrade = bar.Date
			}
		}
	}

	// 最后一天强制卖出 (如果还持仓)
	if len(bars) > 1 && s.position {
		lastBar := bars[len(bars)-1]
		// 检查是否已经有卖出信号
		hasSell := false
		for _, sig := range signals {
			if sig.Date.Equal(lastBar.Date) && sig.Side == trading.OrderSell {
				hasSell = true
				break
			}
		}
		if !hasSell {
			signals = append(signals, Signal{
				Date:   lastBar.Date,
				Code:   s.Code,
				Side:   trading.OrderSell,
				Price:  lastBar.Close,
				Reason: "final_sell",
			})
		}
	}

	return signals
}

// deterministicHash 确定性哈希 (0-1000 范围)。
func deterministicHash(seed int64, t time.Time) float64 {
	// 简化的确定性哈希
	h := float64(seed%1000 + int64(t.UnixNano()%1000))
	return math.Mod(h, 1000)
}

// ============================================================================
// 简单均线策略
// ============================================================================

// SimpleMAStrategy 简单均线交叉策略。
type SimpleMAStrategy struct {
	Code       string `json:"code"`
	FastPeriod int    `json:"fast_period"` // 快线周期
	SlowPeriod int    `json:"slow_period"` // 慢线周期
	position   bool
}

func (s *SimpleMAStrategy) Name() string {
	return "simple_ma"
}

func (s *SimpleMAStrategy) GenerateSignals(bars []KlineBar) []Signal {
	if len(bars) < s.SlowPeriod+1 {
		return nil
	}

	signals := make([]Signal, 0)

	for i := s.SlowPeriod; i < len(bars); i++ {
		// 计算快慢均线
		fastMA := calcMA(bars[i-s.FastPeriod+1:i+1], s.FastPeriod)
		slowMA := calcMA(bars[i-s.SlowPeriod+1:i+1], s.SlowPeriod)

		prevFastMA := calcMA(bars[i-s.FastPeriod:i], s.FastPeriod)
		prevSlowMA := calcMA(bars[i-s.SlowPeriod:i], s.SlowPeriod)

		// 金叉: 快线上穿慢线
		if !s.position && prevFastMA <= prevSlowMA && fastMA > slowMA {
			signals = append(signals, Signal{
				Date:   bars[i].Date,
				Code:   s.Code,
				Side:   trading.OrderBuy,
				Price:  bars[i].Close,
				Reason: "ma_golden_cross",
			})
			s.position = true
		}

		// 死叉: 快线下穿慢线
		if s.position && prevFastMA >= prevSlowMA && fastMA < slowMA {
			signals = append(signals, Signal{
				Date:   bars[i].Date,
				Code:   s.Code,
				Side:   trading.OrderSell,
				Price:  bars[i].Close,
				Reason: "ma_death_cross",
			})
			s.position = false
		}
	}

	// 最后一天强制卖出
	if len(bars) > 0 && s.position {
		lastBar := bars[len(bars)-1]
		hasSell := false
		for _, sig := range signals {
			if sig.Date.Equal(lastBar.Date) && sig.Side == trading.OrderSell {
				hasSell = true
				break
			}
		}
		if !hasSell {
			signals = append(signals, Signal{
				Date:   lastBar.Date,
				Code:   s.Code,
				Side:   trading.OrderSell,
				Price:  lastBar.Close,
				Reason: "final_sell",
			})
		}
	}

	return signals
}

// calcMA 计算简单移动平均。
func calcMA(bars []KlineBar, period int) float64 {
	if len(bars) == 0 || period <= 0 {
		return 0
	}

	n := period
	if n > len(bars) {
		n = len(bars)
	}

	sum := 0.0
	for i := len(bars) - n; i < len(bars); i++ {
		sum += bars[i].Close
	}

	return sum / float64(n)
}

// ============================================================================
// 黄金回测引擎
// ============================================================================

// GoldenBacktestConfig 黄金回测配置。
type GoldenBacktestConfig struct {
	Code        string                     `json:"code"`
	InitialCash float64                    `json:"initial_cash"`
	Constraints trading.TradingConstraints `json:"constraints"`
	CostModel   trading.CostModel          `json:"cost_model"`
}

// GoldenBacktestResult 黄金回测结果。
type GoldenBacktestResult struct {
	ID             string          `json:"id"`
	StrategyName   string          `json:"strategy_name"`
	TotalReturn    float64         `json:"total_return"`
	AnnualReturn   float64         `json:"annual_return"`
	NumTrades      int             `json:"num_trades"`
	NumFills       int             `json:"num_fills"`
	NumRejects     int             `json:"num_rejects"`
	AvgTradeReturn float64         `json:"avg_trade_return"`
	WinRate        float64         `json:"win_rate"`
	EquityCurve    []float64       `json:"equity_curve"`
	Fills          []trading.Fill  `json:"fills"`
	Rejects        []trading.Order `json:"rejects"`
	StartDate      time.Time       `json:"start_date"`
	EndDate        time.Time       `json:"end_date"`
	Notes          []string        `json:"notes,omitempty"`
}

// RunBacktest 运行黄金回测。
func RunBacktest(ctx context.Context, bars []KlineBar, strategy Strategy, config GoldenBacktestConfig) (*GoldenBacktestResult, error) {
	if len(bars) == 0 {
		return nil, fmt.Errorf("no data bars provided")
	}

	// 生成信号
	signals := strategy.GenerateSignals(bars)

	// 创建执行引擎
	engine := trading.NewExecutionEngine(config.Constraints, config.CostModel)
	engine.SetCash(config.InitialCash)

	result := &GoldenBacktestResult{
		ID:           fmt.Sprintf("golden-%s-%s", strategy.Name(), bars[0].Date.Format("20060102")),
		StrategyName: strategy.Name(),
		StartDate:    bars[0].Date,
		EndDate:      bars[len(bars)-1].Date,
		EquityCurve:  make([]float64, 0, len(bars)),
	}

	// 设置初始状态
	currentCash := config.InitialCash
	currentShares := 0
	positions := make(map[string]time.Time)

	for i, bar := range bars {
		// 计算前一日收盘价 (用于涨跌停计算)
		var preClose float64
		if i > 0 {
			preClose = bars[i-1].Close
		} else {
			preClose = bar.Close // 第一天, 使用当日收盘价作为参考
		}

		// 更新权益曲线
		equity := currentCash + float64(currentShares)*bar.Close
		result.EquityCurve = append(result.EquityCurve, equity)

		// 查找当天信号
		for _, sig := range signals {
			if sig.Date == bar.Date {
				// 创建订单
				order := trading.Order{
					ID:            fmt.Sprintf("order-%d", i),
					StockCode:     sig.Code,
					Side:          sig.Side,
					Type:          trading.OrderMarket,
					SignalDate:    sig.Date,
					ExecutionDate: sig.Date,
					Reason:        sig.Reason,
				}

				if sig.Side == trading.OrderBuy {
					// 计算可买股数 (保守估计: 考虑滑点和佣金)
					// 注意: 滑点已经被引擎应用到执行价格上, 成本模型中不再单独计算滑点成本
					// 成本 = 执行价 * 数量 + 佣金 + 过户费
					// 佣金: 万 2.5, 过户费: 万 0.1
					priceWithSlippage := sig.Price * (1 + 0.001) // 10 bps 滑点
					estCostRate := 0.00025 + 0.00001             // 佣金 + 过户费
					// 总成本 = priceWithSlippage * qty * (1 + estCostRate)
					// 预留最低佣金 5 元
					availableCash := currentCash - 5
					maxShares := int(availableCash/(priceWithSlippage*(1+estCostRate))/100) * 100
					if maxShares <= 0 {
						maxShares = 100 // 最少买 100 股
					}
					order.Quantity = maxShares
				} else {
					order.Quantity = currentShares
				}

				// 创建快照 (正确使用前一日收盘价计算涨跌停)
				limitUp := preClose * 1.1
				limitDown := preClose * 0.9

				// 对于创业板/科创板, 使用 20% 涨跌停
				if config.Constraints.Board == trading.BoardChiNext ||
					config.Constraints.Board == trading.BoardSTAR {
					limitUp = preClose * 1.2
					limitDown = preClose * 0.8
				}

				snapshot := trading.MarketSnapshot{
					Date:      bar.Date,
					StockCode: sig.Code,
					Open:      bar.Open,
					High:      bar.High,
					Low:       bar.Low,
					Close:     bar.Close,
					PreClose:  preClose,
					Suspended: false,
					LimitUp:   limitUp,
					LimitDown: limitDown,
					Board:     config.Constraints.Board,
				}

				// 执行订单
				execResult := engine.Execute(order, snapshot)

				if execResult.Rejected {
					result.Rejects = append(result.Rejects, order)
					result.Notes = append(result.Notes,
						fmt.Sprintf("Order rejected: %s - %s", order.ID, execResult.RejectMsg))
					continue
				}

				// 更新状态
				if sig.Side == trading.OrderBuy {
					currentShares += execResult.Fill.Quantity
					currentCash -= execResult.Fill.Price * float64(execResult.Fill.Quantity)
					currentCash -= execResult.Fill.Cost.Total
					positions[sig.Code] = bar.Date
				} else {
					currentShares -= execResult.Fill.Quantity
					currentCash += execResult.Fill.Price * float64(execResult.Fill.Quantity)
					currentCash -= execResult.Fill.Cost.Total
				}

				result.Fills = append(result.Fills, *execResult.Fill)
			}
		}
	}

	// 计算汇总指标
	if len(result.EquityCurve) > 0 && result.EquityCurve[0] > 0 {
		result.TotalReturn = (result.EquityCurve[len(result.EquityCurve)-1] - result.EquityCurve[0]) / result.EquityCurve[0]

		// 年化收益 (252 交易日)
		days := float64(len(bars))
		if days > 0 {
			result.AnnualReturn = math.Pow(1+result.TotalReturn, 252/days) - 1
		}
	}

	result.NumFills = len(result.Fills)
	result.NumRejects = len(result.Rejects)
	result.NumTrades = result.NumFills / 2 // 买卖配对

	// 计算交易收益
	if len(result.Fills) > 1 {
		tradeReturns := make([]float64, 0)
		winCount := 0

		for i := 0; i+1 < len(result.Fills); i += 2 {
			if result.Fills[i+1].Price > result.Fills[i].Price {
				winCount++
			}
			ret := (result.Fills[i+1].Price - result.Fills[i].Price) / result.Fills[i].Price
			tradeReturns = append(tradeReturns, ret)
		}

		if len(tradeReturns) > 0 {
			sum := 0.0
			for _, r := range tradeReturns {
				sum += r
			}
			result.AvgTradeReturn = sum / float64(len(tradeReturns))
			result.WinRate = float64(winCount) / float64(len(tradeReturns)) * 100
		}
	}

	return result, nil
}

// ValidateGoldenResult 验证黄金回测结果。
// expected: 预期值快照
func ValidateGoldenResult(result *GoldenBacktestResult, expected GoldenExpectation) (*ValidationReport, error) {
	report := &ValidationReport{
		ResultID: result.ID,
		Passed:   true,
		Checks:   make([]CheckResult, 0),
	}

	// 检查总收益
	report.Checks = append(report.Checks, CheckResult{
		Name:      "total_return",
		Actual:    result.TotalReturn,
		Expected:  expected.TotalReturn,
		Tolerance: expected.Tolerance,
		Passed:    withinTolerance(result.TotalReturn, expected.TotalReturn, expected.Tolerance),
	})

	// 检查交易次数
	report.Checks = append(report.Checks, CheckResult{
		Name:      "num_fills",
		Actual:    float64(result.NumFills),
		Expected:  float64(expected.NumFills),
		Tolerance: 0,
		Passed:    result.NumFills == expected.NumFills,
	})

	// 检查拒绝次数 (可选)
	if expected.CheckRejects {
		report.Checks = append(report.Checks, CheckResult{
			Name:      "num_rejects",
			Actual:    float64(result.NumRejects),
			Expected:  float64(expected.NumRejects),
			Tolerance: 0,
			Passed:    result.NumRejects == expected.NumRejects,
		})
	}

	// 计算总体通过
	for _, check := range report.Checks {
		if !check.Passed {
			report.Passed = false
			report.FailedChecks = append(report.FailedChecks, check)
		}
	}

	return report, nil
}

// GoldenExpectation 黄金预期值。
type GoldenExpectation struct {
	StrategyName string  `json:"strategy_name"`
	TotalReturn  float64 `json:"total_return"`
	NumFills     int     `json:"num_fills"`
	NumRejects   int     `json:"num_rejects"`
	CheckRejects bool    `json:"check_rejects"`
	Tolerance    float64 `json:"tolerance"` // 允许误差 (绝对值)
}

// ValidationReport 验证报告。
type ValidationReport struct {
	ResultID     string        `json:"result_id"`
	Passed       bool          `json:"passed"`
	Checks       []CheckResult `json:"checks"`
	FailedChecks []CheckResult `json:"failed_checks,omitempty"`
}

// CheckResult 单项检查结果。
type CheckResult struct {
	Name      string  `json:"name"`
	Actual    float64 `json:"actual"`
	Expected  float64 `json:"expected"`
	Tolerance float64 `json:"tolerance"`
	Passed    bool    `json:"passed"`
}

// withinTolerance 检查值是否在允许误差内。
func withinTolerance(actual, expected, tolerance float64) bool {
	return math.Abs(actual-expected) <= tolerance
}
