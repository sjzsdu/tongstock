// Paper Trading 引擎: 调用已有的 trading.ExecutionEngine 做前向模拟。
//
// 设计原则:
//   - 使用与回测完全一致的 A 股交易约束 (T+1 / 涨跌停 / 最小单位 / 停牌)
//   - 信号先写入 SignalLedger, 再由 PaperTradeEngine 逐条执行
//   - 执行结果回写到账本, 保持原始数据不可变
//   - 支持多股票持仓, 现金管理, 费用扣除
package ledger

import (
	"fmt"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/internal/trading"
)

// PaperTradeEngine 前向 Paper Trading 引擎
type PaperTradeEngine struct {
	mu        sync.Mutex
	ledger    *SignalLedger
	engine    *trading.ExecutionEngine
	runID     string
	executed  map[string]bool
	positions map[string]PositionState
}

// NewPaperTradeEngine 创建前向交易引擎
func NewPaperTradeEngine(
	ledger *SignalLedger,
	runID string,
	constraints trading.TradingConstraints,
	costModel trading.CostModel,
	initialCash float64,
) (*PaperTradeEngine, error) {
	engine := trading.NewExecutionEngine(constraints, costModel)
	run, err := ledger.GetRun(runID)
	if err != nil {
		return nil, err
	}
	cash := run.FinalCash
	if cash == 0 && len(run.EquityCurve) == 0 {
		cash = initialCash
	}
	engine.SetCash(cash)
	positions := clonePositions(run.Positions)
	for code, position := range positions {
		engine.SetPosition(code, position.Quantity, position.BuyDate)
	}
	executed := make(map[string]bool)
	for _, entry := range ledger.ListByRun(runID) {
		if entry.Execution != nil {
			executed[entry.ID] = true
		}
	}

	return &PaperTradeEngine{
		ledger:    ledger,
		engine:    engine,
		runID:     runID,
		executed:  executed,
		positions: positions,
	}, nil
}

// ExecuteSignal 使用执行时由服务端读取的真实行情执行一条信号。
func (pt *PaperTradeEngine) ExecuteSignal(
	entry SignalEntry,
	market ExecutionMarket,
) (*ExecutionRecord, error) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.executed[entry.ID] {
		return nil, fmt.Errorf("signal %s already executed", entry.ID)
	}

	quantity := pt.calcQuantity(entry)

	snapshot := trading.MarketSnapshot{
		Date:           market.Date,
		StockCode:      entry.StockCode,
		Open:           market.Open,
		High:           market.High,
		Low:            market.Low,
		Close:          market.Close,
		ExecutionPrice: market.Open,
		PreClose:       market.PreClose,
		Suspended:      market.Suspended,
		LimitUp:        market.LimitUp,
		LimitDown:      market.LimitDown,
		Board:          trading.Board(market.Board),
	}

	order := trading.Order{
		ID:            entry.ID,
		StockCode:     entry.StockCode,
		Side:          trading.OrderSide(entry.Direction),
		Type:          trading.OrderMarket,
		Quantity:      quantity,
		SignalPrice:   entry.Price,
		SignalDate:    entry.SignalDate,
		ExecutionDate: market.Date,
	}

	result := pt.engine.Execute(order, snapshot)

	exec := &ExecutionRecord{
		Market: market, ExecutedAt: market.Date,
	}

	if result.Rejected {
		exec.Status = "rejected"
		exec.RejectReason = result.RejectMsg
	} else if result.Fill != nil {
		exec.Status = string(result.Fill.Status)
		exec.ExecPrice = result.Fill.Price
		exec.ExecQty = result.Fill.Quantity
		exec.Fee = result.Fill.Cost.Total

		prior := pt.positions[entry.StockCode]
		if entry.Direction == string(trading.OrderBuy) {
			totalQty := prior.Quantity + exec.ExecQty
			totalRawCost := prior.AveragePrice*float64(prior.Quantity) +
				exec.ExecPrice*float64(exec.ExecQty)
			buyDate := prior.BuyDate
			if prior.Quantity == 0 || buyDate.IsZero() {
				buyDate = market.Date
			}
			prior = PositionState{
				StockCode: entry.StockCode, Quantity: totalQty,
				AveragePrice:   totalRawCost / float64(totalQty),
				AccruedBuyFees: prior.AccruedBuyFees + exec.Fee,
				BuyDate:        buyDate, LastPrice: market.Close,
				UpdatedAt: market.Date,
			}
			pt.positions[entry.StockCode] = prior
		} else {
			if prior.Quantity <= 0 {
				return nil, fmt.Errorf("persisted position missing for filled sell %s", entry.ID)
			}
			allocatedBuyFees := prior.AccruedBuyFees * float64(exec.ExecQty) / float64(prior.Quantity)
			exec.GrossPnL = (exec.ExecPrice - prior.AveragePrice) * float64(exec.ExecQty)
			exec.PnL = exec.GrossPnL - allocatedBuyFees - exec.Fee
			prior.Quantity -= exec.ExecQty
			prior.AccruedBuyFees -= allocatedBuyFees
			prior.LastPrice = market.Close
			prior.UpdatedAt = market.Date
			if prior.Quantity <= 0 {
				delete(pt.positions, entry.StockCode)
			} else {
				pt.positions[entry.StockCode] = prior
			}
		}
		if position, ok := pt.positions[entry.StockCode]; ok {
			exec.HoldQty = position.Quantity
			exec.HoldCost = position.AveragePrice
		}
	}

	if position, ok := pt.positions[entry.StockCode]; ok {
		position.LastPrice = market.Close
		pt.positions[entry.StockCode] = position
	}
	point := EquityPoint{Date: market.Date, Cash: pt.engine.Cash()}
	for _, position := range pt.positions {
		point.Value += position.LastPrice * float64(position.Quantity)
	}
	point.Total = point.Cash + point.Value
	if err := pt.ledger.RecordExecutionState(entry.ID, *exec, point.Cash, pt.positions, point); err != nil {
		return nil, fmt.Errorf("failed to update ledger: %w", err)
	}

	pt.executed[entry.ID] = true
	return exec, nil
}

type ExecutionMarketLoader func(SignalEntry) (ExecutionMarket, error)

// ExecuteAllPending 使用调用方在执行时加载的市场数据执行所有待处理信号。
func (pt *PaperTradeEngine) ExecuteAllPending(loadMarket ExecutionMarketLoader) (int, int, error) {
	entries := pt.ledger.ListByRun(pt.runID)
	executed := 0
	rejected := 0

	for _, entry := range entries {
		if pt.executed[entry.ID] {
			continue
		}
		if entry.Execution != nil {
			continue
		}

		market, err := loadMarket(entry)
		if err != nil {
			return executed, rejected, fmt.Errorf("loading execution market for %s: %w", entry.ID, err)
		}
		exec, err := pt.ExecuteSignal(entry, market)
		if err != nil {
			return executed, rejected, fmt.Errorf("executing signal %s: %w", entry.ID, err)
		}
		executed++
		if exec.Status == "rejected" {
			rejected++
		}
	}

	return executed, rejected, nil
}

// ExecuteByDate 执行指定日期范围内的信号
func (pt *PaperTradeEngine) ExecuteByDate(
	from, to time.Time,
	loadMarket ExecutionMarketLoader,
) (int, int, error) {
	entries := pt.ledger.ListByRun(pt.runID)
	executed := 0
	rejected := 0

	for _, entry := range entries {
		if entry.SignalDate.Before(from) || entry.SignalDate.After(to) {
			continue
		}
		if pt.executed[entry.ID] {
			continue
		}
		if entry.Execution != nil {
			continue
		}

		market, err := loadMarket(entry)
		if err != nil {
			return executed, rejected, fmt.Errorf("loading execution market for %s: %w", entry.ID, err)
		}
		exec, err := pt.ExecuteSignal(entry, market)
		if err != nil {
			return executed, rejected, fmt.Errorf("executing signal %s: %w", entry.ID, err)
		}
		executed++
		if exec.Status == "rejected" {
			rejected++
		}
	}

	return executed, rejected, nil
}

// GetEquityCurve 获取权益曲线
func (pt *PaperTradeEngine) GetEquityCurve() []EquityPoint {
	run, err := pt.ledger.GetRun(pt.runID)
	if err != nil {
		return nil
	}
	return append([]EquityPoint(nil), run.EquityCurve...)
}

// calcQuantity 基于方向和可用资金/持仓计算下单数量
func (pt *PaperTradeEngine) calcQuantity(entry SignalEntry) int {
	run, err := pt.ledger.GetRun(pt.runID)
	if err != nil {
		return 100
	}

	if entry.Direction == "buy" {
		positionSize := run.InitialCash * 0.10
		if entry.Price > 0 {
			qty := int(positionSize / entry.Price)
			qty = (qty / 100) * 100
			if qty < 100 {
				qty = 100
			}
			return qty
		}
		return 100
	}

	pos, ok := pt.engine.GetPosition(entry.StockCode)
	if ok && pos.Quantity > 0 {
		return pos.Quantity
	}
	return 0
}

// PositionSnapshot 持仓快照
type PositionSnapshot struct {
	Quantity int       `json:"quantity"`
	Cost     float64   `json:"cost"`
	Date     time.Time `json:"date"`
}

// EquityPoint 权益曲线点
type EquityPoint struct {
	Date  time.Time `json:"date"`
	Cash  float64   `json:"cash"`
	Value float64   `json:"position_value"`
	Total float64   `json:"total_equity"`
}
