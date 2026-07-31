package backtest

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/trading"
)

// MarketBar 是按真实交易日对齐的单日行情。停牌日必须显式保留并标记 Suspended，
// 以免回测把下一个有成交的 K 线误当成“次日”。
type MarketBar struct {
	Code      string        `json:"code"`
	Date      time.Time     `json:"date"`
	Open      float64       `json:"open"`
	High      float64       `json:"high"`
	Low       float64       `json:"low"`
	Close     float64       `json:"close"`
	Volume    float64       `json:"volume"`
	Amount    float64       `json:"amount"`
	Suspended bool          `json:"suspended"`
	Board     trading.Board `json:"board"`
	IPODays   int           `json:"ipo_days"`
}

// MarketBarsFromSnapshot 把不可变真实 K 线快照转换为执行器输入。
// 原始快照没有独立停牌字段，因此仅将成交量为 0 的记录标记为停牌；
// 调用方仍须用交易日历补齐快照中完全缺失的交易日。
func MarketBarsFromSnapshot(bars []paradigm.SnapshotKlineBar, board trading.Board) []MarketBar {
	out := make([]MarketBar, len(bars))
	for i, bar := range bars {
		out[i] = MarketBar{
			Code: bar.Code, Date: bar.Date, Open: bar.Open, High: bar.High,
			Low: bar.Low, Close: bar.Close, Volume: bar.Volume, Amount: bar.Amount,
			Suspended: bar.Volume <= 0, Board: board,
		}
	}
	return out
}

type ParadigmExecutionConfig struct {
	InitialCash     float64
	PositionSize    float64
	Constraints     trading.TradingConstraints
	CostModel       trading.CostModel
	EvaluationStart time.Time
	EvaluationEnd   time.Time
}

func DefaultParadigmExecutionConfig() ParadigmExecutionConfig {
	return ParadigmExecutionConfig{
		InitialCash:  1_000_000,
		PositionSize: 1,
		Constraints:  trading.DefaultTradingConstraints(),
		CostModel:    trading.DefaultCostModel(),
	}
}

type SignalRecord struct {
	Date       time.Time         `json:"date"`
	Side       trading.OrderSide `json:"side"`
	Price      float64           `json:"price"`
	Conditions string            `json:"conditions"`
}

type Rejection struct {
	Order trading.Order      `json:"order"`
	Code  trading.RejectCode `json:"code"`
	Msg   string             `json:"message"`
}

type CompletedTrade struct {
	StockCode         string    `json:"stock_code"`
	BuySignalDate     time.Time `json:"buy_signal_date"`
	BuyExecutionDate  time.Time `json:"buy_execution_date"`
	SellSignalDate    time.Time `json:"sell_signal_date"`
	SellExecutionDate time.Time `json:"sell_execution_date"`
	Quantity          int       `json:"quantity"`
	BuyPrice          float64   `json:"buy_price"`
	SellPrice         float64   `json:"sell_price"`
	GrossPnL          float64   `json:"gross_pnl"`
	NetPnL            float64   `json:"net_pnl"`
	TotalCost         float64   `json:"total_cost"`
}

type EquityPoint struct {
	Date   time.Time `json:"date"`
	Equity float64   `json:"equity"`
}

type ParadigmBacktestResult struct {
	StockCode   string           `json:"stock_code"`
	InitialCash float64          `json:"initial_cash"`
	FinalEquity float64          `json:"final_equity"`
	GrossPnL    float64          `json:"gross_pnl"`
	NetPnL      float64          `json:"net_pnl"`
	TotalCost   float64          `json:"total_cost"`
	Signals     []SignalRecord   `json:"signals"`
	Fills       []trading.Fill   `json:"fills"`
	Rejections  []Rejection      `json:"rejections"`
	Trades      []CompletedTrade `json:"trades"`
	EquityCurve []EquityPoint    `json:"equity_curve"`
}

type pendingOrder struct {
	order trading.Order
	index int
}

// RunParadigm 仅使用当前及过去的行情计算信号，并在下一条显式市场栏的开盘执行。
func RunParadigm(ctx context.Context, p *paradigms.Paradigm, bars []MarketBar, cfg ParadigmExecutionConfig) (*ParadigmBacktestResult, error) {
	if err := validateExecutionInput(p, bars, cfg); err != nil {
		return nil, err
	}
	engine := trading.NewExecutionEngine(cfg.Constraints, cfg.CostModel)
	engine.SetCash(cfg.InitialCash)
	result := &ParadigmBacktestResult{
		StockCode: bars[0].Code, InitialCash: cfg.InitialCash,
		Signals: []SignalRecord{}, Fills: []trading.Fill{}, Rejections: []Rejection{},
		Trades: []CompletedTrade{}, EquityCurve: []EquityPoint{},
	}
	var pending *pendingOrder
	var openBuy *trading.Fill
	var openBuyOrder *trading.Order

	for i := range bars {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bar := bars[i]
		inEvaluation := evaluationContains(cfg, bar.Date)

		if pending != nil && pending.index == i {
			exec := executeAtOpen(engine, pending.order, bars, i)
			if exec.Rejected {
				result.Rejections = append(result.Rejections, Rejection{
					Order: pending.order, Code: exec.RejectCode, Msg: exec.RejectMsg,
				})
			} else {
				result.Fills = append(result.Fills, *exec.Fill)
				result.TotalCost += exec.Fill.Cost.Total
				if exec.Fill.Side == trading.OrderBuy {
					openBuy = exec.Fill
					buyOrder := pending.order
					openBuyOrder = &buyOrder
				} else if openBuy != nil && openBuyOrder != nil {
					gross := (exec.Fill.Price - openBuy.Price) * float64(exec.Fill.Quantity)
					cost := openBuy.Cost.Total + exec.Fill.Cost.Total
					result.Trades = append(result.Trades, CompletedTrade{
						StockCode: bar.Code, BuySignalDate: openBuyOrder.SignalDate,
						BuyExecutionDate: openBuy.ExecutionDate, SellSignalDate: pending.order.SignalDate,
						SellExecutionDate: exec.Fill.ExecutionDate, Quantity: exec.Fill.Quantity,
						BuyPrice: openBuy.Price, SellPrice: exec.Fill.Price,
						GrossPnL: gross, NetPnL: gross - cost, TotalCost: cost,
					})
					openBuy, openBuyOrder = nil, nil
				}
			}
			pending = nil
		}

		pos, held := engine.GetPosition(bar.Code)
		if pending == nil && !bar.Suspended && inEvaluation {
			frame := indicatorFrame(bars, i)
			var side trading.OrderSide
			var reason string
			var signal bool
			var err error
			if held && pos.Quantity > 0 {
				signal, reason, err = evaluateExit(p.SellConds, frame)
				side = trading.OrderSell
			} else if !held {
				signal, err = evaluateAll(p.BuyConds, frame)
				side = trading.OrderBuy
				reason = "buy_conditions"
			}
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", bar.Code, bar.Date.Format("2006-01-02"), err)
			}
			if signal {
				result.Signals = append(result.Signals, SignalRecord{
					Date: bar.Date, Side: side, Price: bar.Close, Conditions: reason,
				})
				if i+1 >= len(bars) || !evaluationContains(cfg, bars[i+1].Date) {
					order := newOrder(p, bar, side, 0, time.Time{}, reason)
					result.Rejections = append(result.Rejections, Rejection{
						Order: order, Code: trading.RejectMissingMarketData,
						Msg: "信号后缺少下一交易日真实行情，拒绝执行",
					})
				} else {
					qty := pos.Quantity
					if side == trading.OrderBuy {
						qty = affordableQuantity(engine.Cash()*cfg.PositionSize, bars[i+1].Open, cfg)
					}
					order := newOrder(p, bar, side, qty, bars[i+1].Date, reason)
					if side == trading.OrderBuy && qty == 0 {
						result.Rejections = append(result.Rejections, Rejection{
							Order: order, Code: trading.RejectInsufficient,
							Msg: "可用资金不足以在下一交易日开盘价买入最小交易单位",
						})
					} else {
						pending = &pendingOrder{order: order, index: i + 1}
					}
				}
			}
		}

		if inEvaluation {
			equity := engine.Cash()
			if position, ok := engine.GetPosition(bar.Code); ok {
				equity += float64(position.Quantity) * bar.Close
			}
			result.EquityCurve = append(result.EquityCurve, EquityPoint{Date: bar.Date, Equity: equity})
		}
	}

	if len(result.EquityCurve) == 0 {
		return nil, fmt.Errorf("evaluation window contains no real market bars")
	}
	result.FinalEquity = result.EquityCurve[len(result.EquityCurve)-1].Equity
	result.NetPnL = result.FinalEquity - result.InitialCash
	result.GrossPnL = result.NetPnL + result.TotalCost
	return result, nil
}

func validateExecutionInput(p *paradigms.Paradigm, bars []MarketBar, cfg ParadigmExecutionConfig) error {
	if p == nil {
		return fmt.Errorf("paradigm is required")
	}
	if len(p.BuyConds) == 0 {
		return fmt.Errorf("paradigm buy conditions are empty")
	}
	if len(bars) == 0 {
		return fmt.Errorf("real market bars are required")
	}
	if cfg.InitialCash <= 0 || cfg.PositionSize <= 0 || cfg.PositionSize > 1 {
		return fmt.Errorf("invalid cash or position size")
	}
	if !cfg.EvaluationStart.IsZero() && !cfg.EvaluationEnd.IsZero() &&
		cfg.EvaluationStart.After(cfg.EvaluationEnd) {
		return fmt.Errorf("evaluation start is after evaluation end")
	}
	for i, bar := range bars {
		if bar.Code == "" || bar.Date.IsZero() {
			return fmt.Errorf("bar %d has missing identity", i)
		}
		if !bar.Suspended && (bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0) {
			return fmt.Errorf("bar %s has missing real price", bar.Date.Format("2006-01-02"))
		}
		if i > 0 {
			if !bar.Date.After(bars[i-1].Date) {
				return fmt.Errorf("market bars must be strictly increasing")
			}
			if bar.Code != bars[0].Code {
				return fmt.Errorf("market bars contain multiple stock codes")
			}
		}
	}
	for _, group := range [][]paradigms.Condition{p.BuyConds, p.SellConds.TakeProfit, p.SellConds.StopLoss} {
		for _, condition := range group {
			if err := validateCondition(condition); err != nil {
				return err
			}
		}
	}
	return nil
}

func evaluationContains(cfg ParadigmExecutionConfig, date time.Time) bool {
	if !cfg.EvaluationStart.IsZero() && date.Before(cfg.EvaluationStart) {
		return false
	}
	if !cfg.EvaluationEnd.IsZero() && date.After(cfg.EvaluationEnd) {
		return false
	}
	return true
}

func executeAtOpen(engine *trading.ExecutionEngine, order trading.Order, bars []MarketBar, i int) trading.ExecutionResult {
	bar := bars[i]
	preClose := bar.Close
	if i > 0 {
		preClose = bars[i-1].Close
	}
	board := bar.Board
	limitUp, limitDown := trading.CalculateLimits(preClose, board)
	return engine.Execute(order, trading.MarketSnapshot{
		Date: bar.Date, StockCode: bar.Code, Open: bar.Open, High: bar.High,
		Low: bar.Low, Close: bar.Close, ExecutionPrice: bar.Open, PreClose: preClose,
		Suspended: bar.Suspended, LimitUp: limitUp, LimitDown: limitDown,
		Board: board, IPODays: bar.IPODays,
	})
}

func newOrder(p *paradigms.Paradigm, bar MarketBar, side trading.OrderSide, quantity int, executionDate time.Time, reason string) trading.Order {
	id := p.ID
	if id == "" {
		id = "paradigm"
	}
	return trading.Order{
		ID:        fmt.Sprintf("%s-%s-%s", id, side, bar.Date.Format("20060102")),
		StockCode: bar.Code, Side: side, Type: trading.OrderMarket, Quantity: quantity,
		SignalPrice: bar.Close, SignalDate: bar.Date, ExecutionDate: executionDate, Reason: reason,
	}
}

func affordableQuantity(cash, open float64, cfg ParadigmExecutionConfig) int {
	if cash <= 0 || open <= 0 {
		return 0
	}
	slipped := open * (1 + cfg.CostModel.SlippageBps/10000)
	qty := int(cash / slipped)
	unit := cfg.Constraints.MinTradeUnit
	if unit <= 0 {
		unit = 1
	}
	qty = (qty / unit) * unit
	for qty > 0 {
		cost := cfg.CostModel.CalculateCost(trading.OrderBuy, slipped, qty)
		if slipped*float64(qty)+cost.Total <= cash {
			return qty
		}
		qty -= unit
	}
	return 0
}

func evaluateExit(sell paradigms.SellConditions, frame map[string]float64) (bool, string, error) {
	if len(sell.TakeProfit) > 0 {
		ok, err := evaluateAll(sell.TakeProfit, frame)
		if err != nil || ok {
			return ok, "take_profit", err
		}
	}
	if len(sell.StopLoss) > 0 {
		ok, err := evaluateAll(sell.StopLoss, frame)
		if err != nil || ok {
			return ok, "stop_loss", err
		}
	}
	return false, "", nil
}

func evaluateAll(conditions []paradigms.Condition, frame map[string]float64) (bool, error) {
	if len(conditions) == 0 {
		return false, nil
	}
	for _, condition := range conditions {
		ok, available, err := evaluateCondition(condition, frame)
		if err != nil {
			return false, err
		}
		if !available || !ok {
			return false, nil
		}
	}
	return true, nil
}

func validateCondition(c paradigms.Condition) error {
	op := strings.ToLower(strings.TrimSpace(c.Operator))
	switch op {
	case "gt", ">", "lt", "<", "near", "between", "cross_above", "cross_below":
	default:
		return fmt.Errorf("unsupported condition operator %q", c.Operator)
	}
	if normalizeIndicator(c.Indicator) == "" {
		return fmt.Errorf("condition indicator is empty")
	}
	if strings.TrimSpace(c.Value) == "" {
		return fmt.Errorf("condition value is empty")
	}
	return nil
}

func evaluateCondition(c paradigms.Condition, frame map[string]float64) (bool, bool, error) {
	leftName := normalizeIndicator(c.Indicator)
	left, ok := frame[leftName]
	if !ok {
		return false, false, nil
	}
	op := strings.ToLower(strings.TrimSpace(c.Operator))
	if op == "between" {
		lo, hi, err := parseRange(c.Value)
		return left >= lo && left <= hi, true, err
	}
	right, rightName, ok := resolveValue(c.Value, frame)
	if !ok {
		return false, false, fmt.Errorf("condition value %q is neither an indicator nor a number", c.Value)
	}
	switch op {
	case "gt", ">":
		return left > right, true, nil
	case "lt", "<":
		return left < right, true, nil
	case "near":
		if right == 0 {
			return left == 0, true, nil
		}
		return math.Abs(left-right)/math.Abs(right) <= 0.03, true, nil
	case "cross_above", "cross_below":
		prevLeft, leftOK := frame["prev_"+leftName]
		prevRight, rightOK := frame["prev_"+rightName]
		if !rightOK {
			prevRight = right
			rightOK = true
		}
		if !leftOK || !rightOK {
			return false, false, nil
		}
		if op == "cross_above" {
			return prevLeft <= prevRight && left > right, true, nil
		}
		return prevLeft >= prevRight && left < right, true, nil
	default:
		return false, false, fmt.Errorf("unsupported condition operator %q", c.Operator)
	}
}

func normalizeIndicator(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".", "_")
	value = strings.ReplaceAll(value, " ", "")
	value = strings.Trim(value, "\"'`：:")
	switch value {
	case "price", "current", "当前价", "收盘价", "closeprice":
		return "close"
	case "成交量", "vol":
		return "volume"
	case "dif", "macd_dif":
		return "macd_dif"
	case "rsi", "rsi6", "rsi14":
		return "rsi14"
	default:
		return value
	}
}

func resolveValue(raw string, frame map[string]float64) (float64, string, bool) {
	name := normalizeIndicator(raw)
	if value, ok := frame[name]; ok {
		return value, name, true
	}
	value, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(raw), "%"), 64)
	return value, name, err == nil
}

func parseRange(raw string) (float64, float64, error) {
	raw = strings.ReplaceAll(raw, "至", "~")
	raw = strings.ReplaceAll(raw, "-", "~")
	parts := strings.Split(raw, "~")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range %q", raw)
	}
	lo, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(parts[0]), "%"), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q", raw)
	}
	hi, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(parts[1]), "%"), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q", raw)
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi, nil
}

func indicatorFrame(bars []MarketBar, index int) map[string]float64 {
	frame := make(map[string]float64)
	addIndicators := func(prefix string, at int) {
		if at < 0 {
			return
		}
		frame[prefix+"close"] = bars[at].Close
		frame[prefix+"volume"] = bars[at].Volume
		for _, period := range []int{5, 10, 20, 60} {
			if value, ok := movingAverage(bars, at, period, false); ok {
				frame[fmt.Sprintf("%sma%d", prefix, period)] = value
			}
		}
		if value, ok := movingAverage(bars, at, 20, true); ok {
			frame[prefix+"avg_volume_20"] = value
		}
		if value, ok := rsi(bars, at, 14); ok {
			frame[prefix+"rsi14"] = value
		}
		if value, ok := macdDIF(bars, at); ok {
			frame[prefix+"macd_dif"] = value
		}
	}
	addIndicators("", index)
	addIndicators("prev_", index-1)
	return frame
}

func movingAverage(bars []MarketBar, index, period int, volume bool) (float64, bool) {
	if index+1 < period {
		return 0, false
	}
	var sum float64
	for i := index - period + 1; i <= index; i++ {
		if volume {
			sum += bars[i].Volume
		} else {
			sum += bars[i].Close
		}
	}
	return sum / float64(period), true
}

func rsi(bars []MarketBar, index, period int) (float64, bool) {
	if index < period {
		return 0, false
	}
	var gains, losses float64
	for i := index - period + 1; i <= index; i++ {
		change := bars[i].Close - bars[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}
	if losses == 0 {
		return 100, true
	}
	rs := gains / losses
	return 100 - 100/(1+rs), true
}

func macdDIF(bars []MarketBar, index int) (float64, bool) {
	if index < 25 {
		return 0, false
	}
	return ema(bars, index, 12) - ema(bars, index, 26), true
}

func ema(bars []MarketBar, index, period int) float64 {
	value := bars[0].Close
	alpha := 2.0 / float64(period+1)
	for i := 1; i <= index; i++ {
		value = alpha*bars[i].Close + (1-alpha)*value
	}
	return value
}
