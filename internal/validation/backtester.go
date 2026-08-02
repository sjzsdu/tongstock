package validation

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/trading"
)

// ============================================================================
// MethodBacktester — 用 CompiledMethod 在 K 线上执行确定性回测
// ============================================================================

// BacktestBar 是回测引擎消费的单日行情（与 backtest.MarketBar 对齐但不依赖 paradigm）。
type BacktestBar struct {
	Code      string        `json:"code"`
	Date      string        `json:"date"` // YYYY-MM-DD
	Open      float64       `json:"open"`
	High      float64       `json:"high"`
	Low       float64       `json:"low"`
	Close     float64       `json:"close"`
	Volume    float64       `json:"volume"`
	Amount    float64       `json:"amount"`
	Suspended bool          `json:"suspended"`
	Board     trading.Board `json:"board"`
	PreClose  float64       `json:"pre_close"`
}

// ToMethodsBar 转换为 methods.Bar 供 CompiledMethod.Entry/Exit 使用。
func (b BacktestBar) ToMethodsBar() methods.Bar {
	return methods.Bar{
		Date: b.Date, Open: b.Open, High: b.High, Low: b.Low,
		Close: b.Close, Volume: b.Volume, Amount: b.Amount,
	}
}

// BacktestConfig 回测执行配置。
type BacktestConfig struct {
	InitialCash float64
	PositionPct float64 // 仓位占比 (0,1]，0=满仓
	CostModel   trading.CostModel
	Constraints trading.TradingConstraints
}

// DefaultBacktestConfig 默认严格回测配置。
func DefaultBacktestConfig() BacktestConfig {
	return BacktestConfig{
		InitialCash: 1_000_000,
		PositionPct: 1.0,
		CostModel:   trading.DefaultCostModel(),
		Constraints: trading.DefaultTradingConstraints(),
	}
}

// BacktestResult 单次回测结果。
type BacktestResult struct {
	Trades      []TradeRecord    `json:"trades"`
	EquityCurve []EquityPoint    `json:"equity_curve"`
	Stats       PerformanceStats `json:"stats"`
}

// EquityPoint 资金曲线点。
type EquityPoint struct {
	Date   string  `json:"date"`
	Equity float64 `json:"equity"`
}

// RunBacktest 执行确定性回测。
// 信号逻辑：每个交易日收盘后用 CompiledMethod.Entry/Exit 评估，
// 若触发则在次日开盘执行（T+1），涨跌停/停牌自动拒绝。
func RunBacktest(ctx context.Context, m *methods.CompiledMethod, bars []BacktestBar, cfg BacktestConfig) (*BacktestResult, error) {
	if m == nil || !m.IsExecutable() {
		return nil, fmt.Errorf("method is not executable")
	}
	if len(bars) < 2 {
		return nil, fmt.Errorf("need at least 2 bars, got %d", len(bars))
	}
	if cfg.InitialCash <= 0 {
		return nil, fmt.Errorf("initial cash must be positive")
	}
	if cfg.PositionPct <= 0 || cfg.PositionPct > 1 {
		return nil, fmt.Errorf("position pct must be in (0, 1]")
	}

	cash := cfg.InitialCash
	var (
		trades    []TradeRecord
		equity    []EquityPoint
		position  *positionState
		pending   *pendingOrder
		totalCost float64
	)

	for i := 0; i < len(bars); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bar := bars[i]

		// 1. 在当日开盘执行上一交易日收盘后产生的信号。
		// 订单只在下一根真实 K 线尝试一次；停牌/涨跌停时不伪造成交。
		if pending != nil {
			switch pending.side {
			case trading.OrderBuy:
				if position == nil && !bar.Suspended {
					if pos := tryBuy(cfg, cash, bar, pending.signalDate); pos != nil {
						position = pos
						cash -= pos.entryPrice*float64(pos.quantity) + pos.buyCost
						totalCost += pos.buyCost
					}
				}
			case trading.OrderSell:
				if position != nil && !bar.Suspended {
					if fill := trySell(cfg.CostModel, position, bar); fill != nil {
						gross := (fill.Price - position.entryPrice) * float64(fill.Quantity)
						netPnL := gross - position.buyCost - fill.Cost.Total
						trades = append(trades, makeTrade(position, fill, bar, pending.signalDate, netPnL))
						cash += fill.Price*float64(fill.Quantity) - fill.Cost.Total
						totalCost += fill.Cost.Total
						position = nil
					}
				}
			}
			pending = nil
		}

		// 停牌日没有可信的收盘信号，只按已知价格记录净值。
		if bar.Suspended {
			equity = append(equity, EquityPoint{Date: bar.Date, Equity: cash + positionValue(position, bar.Close)})
			continue
		}
		mBar := bar.ToMethodsBar()
		history := make([]methods.Bar, i+1)
		for j := 0; j <= i; j++ {
			history[j] = bars[j].ToMethodsBar()
		}

		// 2. 收盘后评估信号，仅生成待执行订单，不提前改变持仓。
		if position != nil {
			exitResult, err := m.Exit(mBar, history, &methods.PositionState{
				EntryPrice: position.entryPrice,
				EntryDate:  position.entryDate,
			})
			if err != nil {
				return nil, fmt.Errorf("evaluate exit on %s: %w", bar.Date, err)
			}
			if exitResult.Matched && i+1 < len(bars) {
				pending = &pendingOrder{side: trading.OrderSell, signalDate: bar.Date}
			}
		}

		// 只有本日收盘时仍空仓，才能生成次日买入订单。
		if position == nil {
			entryResult, err := m.Entry(mBar, history)
			if err != nil {
				return nil, fmt.Errorf("evaluate entry on %s: %w", bar.Date, err)
			}
			if entryResult.Matched && i+1 < len(bars) {
				pending = &pendingOrder{side: trading.OrderBuy, signalDate: bar.Date}
			}
		}

		// 3. 记录资金曲线
		equity = append(equity, EquityPoint{Date: bar.Date, Equity: cash + positionValue(position, bar.Close)})
	}

	stats := computeStats(trades, equity, cfg.InitialCash, totalCost)
	return &BacktestResult{
		Trades: trades, EquityCurve: equity, Stats: stats,
	}, nil
}

// positionState 内部持仓状态
type positionState struct {
	code       string
	quantity   int
	entryPrice float64
	entryDate  string
	buyCost    float64
	signalDate string
}

type pendingOrder struct {
	side       trading.OrderSide
	signalDate string
}

func positionValue(p *positionState, currentPrice float64) float64 {
	if p == nil {
		return 0
	}
	return currentPrice * float64(p.quantity)
}

func tryBuy(cfg BacktestConfig, cash float64, bar BacktestBar, signalDate string) *positionState {
	if cash <= 0 || bar.Open <= 0 {
		return nil
	}
	// 涨跌停检查
	if bar.PreClose > 0 {
		limitUp, _ := trading.CalculateLimits(bar.PreClose, bar.Board)
		if bar.Open >= limitUp {
			return nil // 涨停无法买入
		}
	}
	budget := cash * cfg.PositionPct
	price := bar.Open * (1 + cfg.CostModel.SlippageBps/10_000)
	adjusted := cfg.Constraints.MaxQuantityAtPriceLimit(price, budget)
	for adjusted > 0 {
		cost := cfg.CostModel.CalculateCost(trading.OrderBuy, price, adjusted)
		if price*float64(adjusted)+cost.Total <= budget {
			return &positionState{
				code: bar.Code, quantity: adjusted, entryPrice: price,
				entryDate: bar.Date, buyCost: cost.Total, signalDate: signalDate,
			}
		}
		adjusted -= cfg.Constraints.MinTradeUnit
	}
	return nil
}

func trySell(costModel trading.CostModel, pos *positionState, bar BacktestBar) *trading.Fill {
	if pos == nil || bar.Open <= 0 {
		return nil
	}
	// 涨跌停检查
	if bar.PreClose > 0 {
		_, limitDown := trading.CalculateLimits(bar.PreClose, bar.Board)
		if bar.Open <= limitDown {
			return nil // 跌停无法卖出
		}
	}
	price := bar.Open * (1 - costModel.SlippageBps/10_000)
	cost := costModel.CalculateCost(trading.OrderSell, price, pos.quantity)
	return &trading.Fill{
		StockCode: pos.code, Side: trading.OrderSell, Quantity: pos.quantity,
		Price: price, SignalPrice: price, ExecutionDate: mustParseDate(bar.Date),
		Cost: cost, Status: trading.FillFilled,
	}
}

func makeTrade(pos *positionState, fill *trading.Fill, sellBar BacktestBar, sellSignalDate string, netPnL float64) TradeRecord {
	gross := (fill.Price - pos.entryPrice) * float64(fill.Quantity)
	ret := 0.0
	invested := pos.entryPrice*float64(fill.Quantity) + pos.buyCost
	if invested > 0 {
		ret = netPnL / invested
	}
	return TradeRecord{
		Code: pos.code, BuySignalDate: pos.signalDate, BuyExecutionDate: pos.entryDate,
		SellSignalDate: sellSignalDate, SellExecutionDate: sellBar.Date,
		Quantity: fill.Quantity, BuyPrice: pos.entryPrice, SellPrice: fill.Price,
		GrossPnL: gross, NetPnL: netPnL, TotalCost: pos.buyCost + fill.Cost.Total,
		ReturnPct: ret,
	}
}

func mustParseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// ============================================================================
// 统计指标计算
// ============================================================================

func computeStats(trades []TradeRecord, equity []EquityPoint, initialCash float64, totalCost float64) PerformanceStats {
	stats := PerformanceStats{}
	if len(trades) == 0 && len(equity) == 0 {
		return stats
	}
	// 交易级
	stats.TotalTrades = len(trades)
	wins, losses := 0, 0
	totalWin, totalLoss := 0.0, 0.0
	totalHoldDays := 0
	totalTradeValue := 0.0
	for _, t := range trades {
		totalTradeValue += t.BuyPrice * float64(t.Quantity)
		if t.NetPnL > 0 {
			wins++
			totalWin += t.NetPnL
		} else {
			losses++
			totalLoss += math.Abs(t.NetPnL)
		}
		holdDays := daysBetween(t.BuyExecutionDate, t.SellExecutionDate)
		totalHoldDays += holdDays
	}
	if stats.TotalTrades > 0 {
		stats.WinRate = float64(wins) / float64(stats.TotalTrades)
		stats.AvgHoldDays = float64(totalHoldDays) / float64(stats.TotalTrades)
	}
	if wins > 0 {
		stats.AvgWin = totalWin / float64(wins)
	}
	if losses > 0 {
		stats.AvgLoss = totalLoss / float64(losses)
	}
	if totalLoss > 0 {
		stats.ProfitFactor = totalWin / totalLoss
	}

	// 资金曲线级
	if len(equity) > 0 && initialCash > 0 {
		final := equity[len(equity)-1].Equity
		stats.TotalReturn = (final - initialCash) / initialCash
		stats.MaxDrawdown = maxDrawdown(equity)
		days := len(equity)
		if days > 1 {
			stats.AnnualReturn = annualizeReturn(stats.TotalReturn, days)
			stats.SharpeRatio = sharpeRatio(equity)
			stats.SortinoRatio = sortinoRatio(equity)
		}
		if stats.MaxDrawdown != 0 {
			stats.CalmarRatio = stats.AnnualReturn / math.Abs(stats.MaxDrawdown)
		}
	}

	// 成本
	stats.TotalCost = totalCost
	if totalTradeValue > 0 {
		stats.CostRatio = totalCost / totalTradeValue
	}

	// 置信区间 (简化: 用交易收益率的标准差)
	if stats.TotalTrades >= 2 {
		returns := make([]float64, len(trades))
		for i, t := range trades {
			returns[i] = t.ReturnPct
		}
		mean, std := meanStd(returns)
		ci := 1.96 * std / math.Sqrt(float64(len(returns)))
		stats.ReturnCI95Low = mean - ci
		stats.ReturnCI95High = mean + ci
	}

	return stats
}

func maxDrawdown(equity []EquityPoint) float64 {
	if len(equity) == 0 {
		return 0
	}
	peak := equity[0].Equity
	maxDD := 0.0
	for _, p := range equity {
		if p.Equity > peak {
			peak = p.Equity
		}
		if peak > 0 {
			dd := (peak - p.Equity) / peak
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return maxDD
}

func annualizeReturn(totalReturn float64, days int) float64 {
	if days <= 0 {
		return 0
	}
	years := float64(days) / 252.0
	if years <= 0 {
		return 0
	}
	if totalReturn <= -1 {
		return -1
	}
	return math.Pow(1+totalReturn, 1/years) - 1
}

func sharpeRatio(equity []EquityPoint) float64 {
	if len(equity) < 2 {
		return 0
	}
	returns := make([]float64, 0, len(equity)-1)
	for i := 1; i < len(equity); i++ {
		if equity[i-1].Equity > 0 {
			returns = append(returns, (equity[i].Equity-equity[i-1].Equity)/equity[i-1].Equity)
		}
	}
	mean, std := meanStd(returns)
	if std == 0 {
		return 0
	}
	return mean / std * math.Sqrt(252)
}

func sortinoRatio(equity []EquityPoint) float64 {
	if len(equity) < 2 {
		return 0
	}
	returns := make([]float64, 0, len(equity)-1)
	for i := 1; i < len(equity); i++ {
		if equity[i-1].Equity > 0 {
			r := (equity[i].Equity - equity[i-1].Equity) / equity[i-1].Equity
			returns = append(returns, r)
		}
	}
	mean, _ := meanStd(returns)
	downsideReturns := make([]float64, 0)
	for _, r := range returns {
		if r < 0 {
			downsideReturns = append(downsideReturns, r)
		}
	}
	if len(downsideReturns) == 0 {
		return 0
	}
	_, downsideStd := meanStd(downsideReturns)
	if downsideStd == 0 {
		return 0
	}
	return mean / downsideStd * math.Sqrt(252)
}

func meanStd(data []float64) (float64, float64) {
	if len(data) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	mean := sum / float64(len(data))
	variance := 0.0
	for _, v := range data {
		variance += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(variance / float64(len(data)))
}

func daysBetween(a, b string) int {
	ta, err := time.Parse("2006-01-02", a)
	if err != nil {
		return 0
	}
	tb, err := time.Parse("2006-01-02", b)
	if err != nil {
		return 0
	}
	d := int(tb.Sub(ta).Hours() / 24)
	if d < 0 {
		return -d
	}
	return d
}

// SortedTrades 按买入日期排序返回副本。
func SortedTrades(trades []TradeRecord) []TradeRecord {
	out := make([]TradeRecord, len(trades))
	copy(out, trades)
	sort.Slice(out, func(i, j int) bool {
		return out[i].BuyExecutionDate < out[j].BuyExecutionDate
	})
	return out
}
