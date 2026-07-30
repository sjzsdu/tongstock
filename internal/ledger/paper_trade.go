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
	mu       sync.Mutex
	ledger   *SignalLedger
	engine   *trading.ExecutionEngine
	runID    string
	executed map[string]bool
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
	engine.SetCash(initialCash)

	return &PaperTradeEngine{
		ledger:   ledger,
		engine:   engine,
		runID:    runID,
		executed: make(map[string]bool),
	}, nil
}

// ExecuteSignal 执行一条信号 (遵守 A 股约束)
func (pt *PaperTradeEngine) ExecuteSignal(entry SignalEntry) (*ExecutionRecord, error) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.executed[entry.ID] {
		return nil, fmt.Errorf("signal %s already executed", entry.ID)
	}

	quantity := pt.calcQuantity(entry)

	snapshot := trading.MarketSnapshot{
		Date:      entry.ExecutionDate,
		StockCode: entry.StockCode,
		Open:      entry.Price,
		High:      entry.Price,
		Low:       entry.Price,
		Close:     entry.Price,
		PreClose:  entry.PreClose,
		Suspended: entry.Suspended,
		LimitUp:   entry.LimitUp,
		LimitDown: entry.LimitDown,
		Board:     trading.Board(entry.Board),
	}

	order := trading.Order{
		ID:            entry.ID,
		StockCode:     entry.StockCode,
		Side:          trading.OrderSide(entry.Direction),
		Type:          trading.OrderMarket,
		Quantity:      quantity,
		SignalDate:    entry.SignalDate,
		ExecutionDate: entry.ExecutionDate,
	}

	result := pt.engine.Execute(order, snapshot)

	exec := &ExecutionRecord{
		ExecutedAt: entry.ExecutionDate,
	}

	if result.Rejected {
		exec.Status = "rejected"
		exec.RejectReason = result.RejectMsg
	} else if result.Fill != nil {
		exec.Status = string(result.Fill.Status)
		exec.ExecPrice = result.Fill.Price
		exec.ExecQty = result.Fill.Quantity
		exec.Fee = result.Fill.Cost.Total

		pos, _ := pt.engine.GetPosition(entry.StockCode)
		exec.HoldQty = pos.Quantity
		exec.HoldCost = float64(pos.BuyDate.Unix())

		if entry.Direction == "sell" {
			exec.PnL = exec.ExecPrice*float64(exec.ExecQty) - exec.Fee
		}
	}

	if err := pt.ledger.UpdateExecution(entry.ID, *exec); err != nil {
		return nil, fmt.Errorf("failed to update ledger: %w", err)
	}

	pt.executed[entry.ID] = true
	return exec, nil
}

// ExecuteAllPending 执行账本中该 run 的所有未执行信号
func (pt *PaperTradeEngine) ExecuteAllPending() (int, int, error) {
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

		exec, err := pt.ExecuteSignal(entry)
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
func (pt *PaperTradeEngine) ExecuteByDate(from, to time.Time) (int, int, error) {
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

		exec, err := pt.ExecuteSignal(entry)
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
	pt.mu.Lock()
	defer pt.mu.Unlock()

	entries := pt.ledger.ListByRun(pt.runID)
	var cash float64
	positions := make(map[string]PositionSnapshot)
	var points []EquityPoint

	run, err := pt.ledger.GetRun(pt.runID)
	if err == nil {
		cash = run.InitialCash
	}

	for _, e := range entries {
		if e.Execution == nil {
			continue
		}
		switch e.Execution.Status {
		case "filled", "partial":
			if e.Direction == "buy" {
				cash -= e.Execution.ExecPrice*float64(e.Execution.ExecQty) + e.Execution.Fee
				positions[e.StockCode] = PositionSnapshot{
					Quantity: e.Execution.ExecQty,
					Cost:     e.Execution.ExecPrice,
					Date:     e.ExecutionDate,
				}
			} else {
				cash += e.Execution.ExecPrice*float64(e.Execution.ExecQty) - e.Execution.Fee
				if pos, ok := positions[e.StockCode]; ok {
					pos.Quantity -= e.Execution.ExecQty
					if pos.Quantity <= 0 {
						delete(positions, e.StockCode)
					} else {
						positions[e.StockCode] = pos
					}
				}
			}
		}

		var positionValue float64
		for _, pos := range positions {
			positionValue += e.Price * float64(pos.Quantity)
		}

		points = append(points, EquityPoint{
			Date:  e.SignalDate,
			Cash:  cash,
			Value: positionValue,
			Total: cash + positionValue,
		})
	}

	return points
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
