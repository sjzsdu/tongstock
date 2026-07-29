package trading

import (
	"math"
	"testing"
	"time"
)

// ==================== Board Tests ====================

func TestBoard_DailyLimit(t *testing.T) {
	tests := []struct {
		board    Board
		expected float64
	}{
		{BoardMain, 0.10},
		{BoardChiNext, 0.20},
		{BoardSTAR, 0.20},
		{BoardBJ, 0.30},
		{BoardUnknown, 0.10},
	}

	for _, tt := range tests {
		got := tt.board.DailyLimit()
		if got != tt.expected {
			t.Errorf("Board %s: DailyLimit = %f, want %f", tt.board, got, tt.expected)
		}
	}
}

// ==================== TradingConstraints Tests ====================

func TestTradingConstraints_ValidateQuantity(t *testing.T) {
	constraints := DefaultTradingConstraints()

	tests := []struct {
		qty      int
		expected int
		valid    bool
	}{
		{100, 100, true},
		{200, 200, true},
		{150, 100, false}, // 非整手, 向下取整
		{50, 0, false},    // 小于 1 手
		{1000, 1000, true},
		{1050, 1000, false}, // 非整手
		{0, 0, false},
	}

	for _, tt := range tests {
		got, valid := constraints.ValidateQuantity(tt.qty)
		if got != tt.expected || valid != tt.valid {
			t.Errorf("ValidateQuantity(%d) = (%d, %v), want (%d, %v)",
				tt.qty, got, valid, tt.expected, tt.valid)
		}
	}
}

func TestTradingConstraints_ZeroTradeUnit(t *testing.T) {
	c := TradingConstraints{MinTradeUnit: 0}
	got, valid := c.ValidateQuantity(150)
	if !valid {
		t.Error("Zero MinTradeUnit should not validate")
	}
	if got != 150 {
		t.Errorf("Quantity should not be adjusted, got %d", got)
	}
}

// ==================== CostModel Tests ====================

func TestCostModel_BuyCost(t *testing.T) {
	cm := DefaultCostModel()

	cost := cm.CalculateCost(OrderBuy, 10.0, 1000)
	tradeValue := 10000.0

	// 佣金: 10000 * 0.00025 = 2.5 -> min 5
	if cost.Commission < 5.0 {
		t.Errorf("Commission should be at least 5, got %f", cost.Commission)
	}

	// 佣金应该是 5.0 (最低佣金)
	expectedCommission := 5.0
	if math.Abs(cost.Commission-expectedCommission) > 0.01 {
		t.Errorf("Commission = %f, want %f", cost.Commission, expectedCommission)
	}

	// 印花税: 买入不收
	if cost.StampDuty != 0 {
		t.Errorf("Buy stamp duty should be 0, got %f", cost.StampDuty)
	}

	// 过户费: 10000 * 0.00001 = 0.1
	if math.Abs(cost.TransferFee-0.1) > 0.001 {
		t.Errorf("TransferFee = %f, want 0.1", cost.TransferFee)
	}

	// 滑点: 已通过执行价格体现, 成本模型中不再单独计算
	if cost.SlippageCost != 0 {
		t.Errorf("SlippageCost should be 0 (included in execution price), got %f", cost.SlippageCost)
	}

	// 总成本 (不含滑点, 滑点已含在执行价中)
	expectedTotal := expectedCommission + 0 + 0.1
	if math.Abs(cost.Total-expectedTotal) > 0.01 {
		t.Errorf("Total = %f, want %f", cost.Total, expectedTotal)
	}

	_ = tradeValue
}

func TestCostModel_SellCost(t *testing.T) {
	cm := DefaultCostModel()

	cost := cm.CalculateCost(OrderSell, 10.0, 1000)

	// 印花税: 10000 * 0.0005 = 5
	if math.Abs(cost.StampDuty-5.0) > 0.01 {
		t.Errorf("StampDuty = %f, want 5.0", cost.StampDuty)
	}

	// 总成本应该比买入高 (多了印花税)
	buyCost := cm.CalculateCost(OrderBuy, 10.0, 1000)
	if cost.Total <= buyCost.Total {
		t.Error("Sell cost should be higher than buy cost due to stamp duty")
	}
}

func TestCostModel_HighValueTrade(t *testing.T) {
	cm := DefaultCostModel()

	// 大额交易: 佣金超过最低
	cost := cm.CalculateCost(OrderBuy, 100.0, 10000) // 1,000,000 元
	expectedCommission := 1000000 * 0.00025          // 250 元

	if math.Abs(cost.Commission-expectedCommission) > 0.01 {
		t.Errorf("Commission = %f, want %f", cost.Commission, expectedCommission)
	}

	// 总费率约为: 0.025% + 0.001% = 0.026% (滑点已含在执行价中)
	rate := cost.Total / 1000000.0 * 100
	if rate < 0.01 || rate > 0.1 {
		t.Errorf("Total rate = %f%%, expected ~0.03%%", rate)
	}
}

func TestCostModel_MinCommission(t *testing.T) {
	cm := DefaultCostModel()

	// 小交易: 佣金不足 5 元
	cost := cm.CalculateCost(OrderBuy, 1.0, 100) // 100 元
	if cost.Commission != 5.0 {
		t.Errorf("Min commission should be 5, got %f", cost.Commission)
	}
}

func TestCostModel_NoStampDuty(t *testing.T) {
	cm := DefaultCostModel()
	cm.EnableStampDuty = false

	cost := cm.CalculateCost(OrderSell, 10.0, 10000)
	if cost.StampDuty != 0 {
		t.Error("Stamp duty should be disabled")
	}
}

func TestCostModel_GrossToNet(t *testing.T) {
	cm := DefaultCostModel()

	gross := 10000.0
	tradeValue := 100000.0

	// 买入: 净收益 = 毛收益 - 买入成本
	net := cm.GrossToNet(gross, tradeValue, OrderBuy)
	if net >= gross {
		t.Error("Net should be less than gross after costs")
	}

	// 卖出: 净收益更低 (多了印花税)
	netSell := cm.GrossToNet(gross, tradeValue, OrderSell)
	if netSell >= net {
		t.Error("Sell net should be lower than buy net due to stamp duty")
	}
}

// ==================== Price Limit Tests ====================

func TestCalculateLimits(t *testing.T) {
	tests := []struct {
		preClose float64
		board    Board
		wantUp   float64
		wantDown float64
	}{
		{10.0, BoardMain, 11.0, 9.0},
		{10.0, BoardChiNext, 12.0, 8.0},
		{10.0, BoardSTAR, 12.0, 8.0},
		{10.0, BoardBJ, 13.0, 7.0},
		{9.99, BoardMain, 10.99, 9.0}, // 精度测试
	}

	for _, tt := range tests {
		up, down := CalculateLimits(tt.preClose, tt.board)
		if math.Abs(up-tt.wantUp) > 0.01 {
			t.Errorf("LimitUp(%.2f, %s) = %.2f, want %.2f", tt.preClose, tt.board, up, tt.wantUp)
		}
		if math.Abs(down-tt.wantDown) > 0.01 {
			t.Errorf("LimitDown(%.2f, %s) = %.2f, want %.2f", tt.preClose, tt.board, down, tt.wantDown)
		}
	}
}

// ==================== ExecutionEngine Tests ====================

func TestExecutionEngine_BuySuccess(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	engine.SetCash(100000)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Open:      10.0,
		High:      10.5,
		Low:       9.8,
		Close:     10.2,
		PreClose:  10.0,
		Suspended: false,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	order := Order{
		ID:            "test-buy",
		StockCode:     "600000",
		Side:          OrderBuy,
		Type:          OrderMarket,
		Quantity:      1000,
		SignalDate:    date,
		ExecutionDate: date,
	}

	result := engine.Execute(order, snapshot)

	if result.Rejected {
		t.Fatalf("Order rejected: %s - %s", result.RejectCode, result.RejectMsg)
	}

	if result.Fill == nil {
		t.Fatal("Fill should not be nil")
	}

	if result.Fill.Quantity != 1000 {
		t.Errorf("Fill quantity = %d, want 1000", result.Fill.Quantity)
	}

	// 成交价应含滑点 (买入价格上浮)
	expectedPrice := roundPrice(10.2 * (1 + 10.0/10000.0)) // 10bps = 0.1%
	if math.Abs(result.Fill.Price-expectedPrice) > 0.01 {
		t.Errorf("Fill price = %.2f, want %.2f (with slippage)", result.Fill.Price, expectedPrice)
	}

	// 持仓已更新
	pos, ok := engine.GetPosition("600000")
	if !ok {
		t.Fatal("Position should exist after buy")
	}
	if pos.Quantity != 1000 {
		t.Errorf("Position qty = %d, want 1000", pos.Quantity)
	}
}

func TestExecutionEngine_SellSuccess(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)

	engine.SetPosition("600000", 1000, date.AddDate(0, 0, -1))

	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Close:     10.5,
		PreClose:  10.0,
		Suspended: false,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	order := Order{
		ID:        "test-sell",
		StockCode: "600000",
		Side:      OrderSell,
		Quantity:  1000,
	}

	result := engine.Execute(order, snapshot)

	if result.Rejected {
		t.Fatalf("Order rejected: %s - %s", result.RejectCode, result.RejectMsg)
	}

	// 卖出后持仓应为 0
	_, ok := engine.GetPosition("600000")
	if ok {
		t.Error("Position should be closed after sell")
	}
}

func TestExecutionEngine_T1Restriction(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)

	// 当日买入, 当日卖出 (T+1 违规)
	engine.SetPosition("600000", 1000, date) // 买入日期 = 当天

	snapshot := MarketSnapshot{
		Date:      date, // 同一天
		StockCode: "600000",
		Close:     10.5,
		PreClose:  10.0,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	order := Order{
		ID:        "test-t1",
		StockCode: "600000",
		Side:      OrderSell,
		Quantity:  1000,
	}

	result := engine.Execute(order, snapshot)

	if !result.Rejected {
		t.Error("T+1 violation should be rejected")
	}
	if result.RejectCode != RejectT1Restriction {
		t.Errorf("RejectCode = %s, want T1", result.RejectCode)
	}
}

func TestExecutionEngine_T1NextDay(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	buyDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	sellDate := time.Date(2024, 1, 16, 0, 0, 0, 0, time.Local) // T+1

	engine.SetPosition("600000", 1000, buyDate)

	snapshot := MarketSnapshot{
		Date:      sellDate,
		StockCode: "600000",
		Close:     10.5,
		PreClose:  10.2,
		LimitUp:   11.22,
		LimitDown: 9.18,
		Board:     BoardMain,
	}

	order := Order{
		ID:        "test-t1-ok",
		StockCode: "600000",
		Side:      OrderSell,
		Quantity:  1000,
	}

	result := engine.Execute(order, snapshot)

	if result.Rejected {
		t.Errorf("T+1 should be allowed on next day: %s", result.RejectMsg)
	}
}

func TestExecutionEngine_PriceLimitBuy(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	engine.SetCash(100000)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Close:     11.0, // 涨停价
		PreClose:  10.0,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	order := Order{
		ID:        "test-limit-up",
		StockCode: "600000",
		Side:      OrderBuy,
		Quantity:  1000,
	}

	result := engine.Execute(order, snapshot)

	if !result.Rejected {
		t.Error("Buy at limit up should be rejected")
	}
	if result.RejectCode != RejectPriceLimit {
		t.Errorf("RejectCode = %s, want price_limit", result.RejectCode)
	}
}

func TestExecutionEngine_PriceLimitSell(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	engine.SetPosition("600000", 1000, date.AddDate(0, 0, -1))

	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Close:     9.0, // 跌停价
		PreClose:  10.0,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	order := Order{
		ID:        "test-limit-down",
		StockCode: "600000",
		Side:      OrderSell,
		Quantity:  1000,
	}

	result := engine.Execute(order, snapshot)

	if !result.Rejected {
		t.Error("Sell at limit down should be rejected")
	}
}

func TestExecutionEngine_Suspended(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	engine.SetCash(100000)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Close:     10.0,
		PreClose:  10.0,
		Suspended: true,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	order := Order{
		ID:        "test-susp",
		StockCode: "600000",
		Side:      OrderBuy,
		Quantity:  1000,
	}

	result := engine.Execute(order, snapshot)

	if !result.Rejected {
		t.Error("Suspended stock should be rejected")
	}
	if result.RejectCode != RejectSuspended {
		t.Errorf("RejectCode = %s, want suspended", result.RejectCode)
	}
}

func TestExecutionEngine_InsufficientCash(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	engine.SetCash(100) // 资金极少

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Close:     10.0,
		PreClose:  10.0,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	order := Order{
		ID:        "test-poor",
		StockCode: "600000",
		Side:      OrderBuy,
		Quantity:  1000,
	}

	result := engine.Execute(order, snapshot)

	if !result.Rejected {
		t.Error("Insufficient cash should be rejected")
	}
	if result.RejectCode != RejectInsufficient {
		t.Errorf("RejectCode = %s, want insufficient", result.RejectCode)
	}
}

func TestExecutionEngine_InsufficientPosition(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Close:     10.0,
		PreClose:  10.0,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	// 无持仓直接卖出
	order := Order{
		ID:        "test-nopos",
		StockCode: "600000",
		Side:      OrderSell,
		Quantity:  1000,
	}

	result := engine.Execute(order, snapshot)

	if !result.Rejected {
		t.Error("Sell without position should be rejected")
	}
	if result.RejectCode != RejectInsufficient {
		t.Errorf("RejectCode = %s, want insufficient", result.RejectCode)
	}
}

func TestExecutionEngine_InvalidQuantity(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	engine.SetCash(100000)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Close:     10.0,
		PreClose:  10.0,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	// 数量为 0
	order := Order{
		ID:        "test-zero",
		StockCode: "600000",
		Side:      OrderBuy,
		Quantity:  0,
	}

	result := engine.Execute(order, snapshot)

	if !result.Rejected {
		t.Error("Zero quantity should be rejected")
	}
}

func TestExecutionEngine_IpoExempt(t *testing.T) {
	constraints := DefaultTradingConstraints()
	constraints.IPODays = 5 // 上市 5 天内无涨跌停
	engine := NewExecutionEngine(constraints, DefaultCostModel())
	engine.SetCash(100000)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "300001",
		Close:     50.0, // 涨停价 (相对前一天 +10%)
		PreClose:  45.0,
		LimitUp:   49.5,
		LimitDown: 40.5,
		Board:     BoardChiNext,
		IPODays:   3, // 上市第 3 天
	}

	order := Order{
		ID:        "test-ipo",
		StockCode: "300001",
		Side:      OrderBuy,
		Quantity:  100,
	}

	result := engine.Execute(order, snapshot)

	// IPO 前 N 天不受涨跌停限制
	if result.Rejected {
		t.Errorf("IPO stock should not be limited: %s", result.RejectMsg)
	}
}

func TestExecutionEngine_SlippageDirection(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	engine.SetCash(100000)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Close:     10.0,
		PreClose:  10.0,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	// 买入: 价格应上浮
	buyOrder := Order{ID: "buy", StockCode: "600000", Side: OrderBuy, Quantity: 100}
	buyResult := engine.Execute(buyOrder, snapshot)
	if buyResult.Fill.Price <= 10.0 {
		t.Error("Buy price should be higher than close (slippage up)")
	}

	// 卖出: 价格应下浮
	engine2 := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	engine2.SetPosition("600000", 100, date.AddDate(0, 0, -1))
	sellOrder := Order{ID: "sell", StockCode: "600000", Side: OrderSell, Quantity: 100}
	sellResult := engine2.Execute(sellOrder, snapshot)
	if sellResult.Fill.Price >= 10.0 {
		t.Error("Sell price should be lower than close (slippage down)")
	}
}

func TestExecutionEngine_CashUpdate(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	initialCash := 100000.0
	engine.SetCash(initialCash)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date:      date,
		StockCode: "600000",
		Close:     10.0,
		PreClose:  10.0,
		LimitUp:   11.0,
		LimitDown: 9.0,
		Board:     BoardMain,
	}

	order := Order{ID: "test", StockCode: "600000", Side: OrderBuy, Quantity: 1000}
	result := engine.Execute(order, snapshot)

	if result.Rejected {
		t.Fatalf("Order rejected: %s", result.RejectMsg)
	}

	// 买入后资金应减少
	if engine.cash >= initialCash {
		t.Error("Cash should decrease after buy")
	}

	// 买入金额 ≈ 1000 * 10.0 * (1 + 0.001) = 10010 (含滑点)
	// 再加成本
	expectedCash := initialCash - result.Fill.Price*1000 - result.Fill.Cost.Total
	if math.Abs(engine.cash-expectedCash) > 0.01 {
		t.Errorf("Cash = %.2f, want %.2f", engine.cash, expectedCash)
	}
}

// ==================== BacktestReport Tests ====================

func TestBacktestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  BacktestConfig
		wantErr bool
	}{
		{
			"valid",
			NewBacktestConfig("600000", "2024-01-01", "2024-12-31"),
			false,
		},
		{
			"zero cash",
			func() BacktestConfig {
				c := NewBacktestConfig("600000", "2024-01-01", "2024-12-31")
				c.InitialCash = 0
				return c
			}(),
			true,
		},
		{
			"invalid position size",
			func() BacktestConfig {
				c := NewBacktestConfig("600000", "2024-01-01", "2024-12-31")
				c.PositionSize = 0
				return c
			}(),
			true,
		},
		{
			"position size > 1",
			func() BacktestConfig {
				c := NewBacktestConfig("600000", "2024-01-01", "2024-12-31")
				c.PositionSize = 1.5
				return c
			}(),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ==================== OrderFromSignal Tests ====================

func TestOrderFromSignal_NextOpen(t *testing.T) {
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	order := OrderFromSignal("600000", OrderBuy, 1000, date, SignalNextOpen)

	if !order.ExecutionDate.Equal(date.AddDate(0, 0, 1)) {
		t.Error("ExecutionDate should be next day for next_open strategy")
	}
	if order.SignalDate != date {
		t.Error("SignalDate should be the original date")
	}
}

func TestOrderFromSignal_SameBarOpen(t *testing.T) {
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	order := OrderFromSignal("600000", OrderBuy, 1000, date, SignalSameBarOpen)

	if !order.ExecutionDate.Equal(date) {
		t.Error("ExecutionDate should be same day for same_bar_open strategy")
	}
}

// ==================== A 股特殊场景集成测试 ====================

func TestExecutionEngine_FullRoundTrip(t *testing.T) {
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	engine.SetCash(1000000)

	// 模拟一次完整的买入-卖出流程
	buyDate := time.Date(2024, 1, 10, 0, 0, 0, 0, time.Local)
	sellDate := time.Date(2024, 1, 12, 0, 0, 0, 0, time.Local)

	// Day 1: 买入
	buySnapshot := MarketSnapshot{
		Date: buyDate, StockCode: "600000",
		Close: 10.0, PreClose: 9.8,
		LimitUp: 10.78, LimitDown: 8.82,
		Board: BoardMain,
	}
	buyOrder := Order{ID: "b1", StockCode: "600000", Side: OrderBuy, Quantity: 10000}
	buyResult := engine.Execute(buyOrder, buySnapshot)
	if buyResult.Rejected {
		t.Fatalf("Buy failed: %s", buyResult.RejectMsg)
	}

	day1Cash := engine.cash
	pos1, _ := engine.GetPosition("600000")

	// Day 1 same day: T+1 限制, 不可卖出 (买入当日)
	t1Snapshot := MarketSnapshot{
		Date:      buyDate, // 同一天
		StockCode: "600000", Close: 10.5, PreClose: 10.0,
		LimitUp: 11.0, LimitDown: 9.0, Board: BoardMain,
	}
	t1Order := Order{ID: "s-t1", StockCode: "600000", Side: OrderSell, Quantity: 10000}
	t1Result := engine.Execute(t1Order, t1Snapshot)
	if !t1Result.Rejected || t1Result.RejectCode != RejectT1Restriction {
		t.Error("T+1 should block sell on same day as buy")
	}

	// Day 3: T+1 允许, 可以卖出 (买入后第 2 个交易日)
	sellSnapshot := MarketSnapshot{
		Date: sellDate, StockCode: "600000",
		Close: 11.0, PreClose: 10.5,
		LimitUp: 11.55, LimitDown: 9.45,
		Board: BoardMain,
	}
	sellOrder := Order{ID: "s1", StockCode: "600000", Side: OrderSell, Quantity: 10000}
	sellResult := engine.Execute(sellOrder, sellSnapshot)
	if sellResult.Rejected {
		t.Fatalf("Sell failed: %s", sellResult.RejectMsg)
	}

	// 卖出后资金应增加
	if engine.cash <= day1Cash {
		t.Error("Cash should increase after profitable sell")
	}

	// 持仓清空
	_, ok := engine.GetPosition("600000")
	if ok {
		t.Error("Position should be fully closed")
	}

	// 计算盈亏
	buyCost := buyResult.Fill.Cost.Total
	sellCost := sellResult.Fill.Cost.Total
	grossPnL := (sellResult.Fill.Price - buyResult.Fill.Price) * 10000
	netPnL := grossPnL - buyCost - sellCost

	if netPnL <= 0 {
		t.Logf("Net P&L = %.2f (costs: buy=%.2f, sell=%.2f)", netPnL, buyCost, sellCost)
	}

	_ = pos1
}

func TestExecutionEngine_PartialFillNotSupported(t *testing.T) {
	// 当前实现只支持全部成交或拒绝, 但数量可以被调整
	engine := NewExecutionEngine(DefaultTradingConstraints(), DefaultCostModel())
	engine.SetCash(100000)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	snapshot := MarketSnapshot{
		Date: date, StockCode: "600000",
		Close: 10.0, PreClose: 10.0,
		LimitUp: 11.0, LimitDown: 9.0, Board: BoardMain,
	}

	// 150 股 -> 调整为 100 股
	order := Order{ID: "partial", StockCode: "600000", Side: OrderBuy, Quantity: 150}
	result := engine.Execute(order, snapshot)

	if result.Rejected {
		t.Fatalf("Should not be rejected: %s", result.RejectMsg)
	}
	if result.Fill.Quantity != 100 {
		t.Errorf("Quantity should be adjusted to 100, got %d", result.Fill.Quantity)
	}
}

func TestExecutionEngine_ChiNextLimitUp(t *testing.T) {
	constraints := DefaultTradingConstraints()
	constraints.Board = BoardChiNext
	engine := NewExecutionEngine(constraints, DefaultCostModel())
	engine.SetCash(100000)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	// 创业板: 20% 涨跌停
	snapshot := MarketSnapshot{
		Date: date, StockCode: "300001",
		Close: 12.0, PreClose: 10.0, // 涨 20% -> 涨停
		LimitUp: 12.0, LimitDown: 8.0,
		Board: BoardChiNext,
	}

	order := Order{ID: "cyb", StockCode: "300001", Side: OrderBuy, Quantity: 100}
	result := engine.Execute(order, snapshot)

	if !result.Rejected {
		t.Error("ChiNext stock at 20% limit up should be rejected")
	}
}

func TestCostModel_EdgeCases(t *testing.T) {
	cm := DefaultCostModel()

	// 零交易
	cost := cm.CalculateCost(OrderBuy, 10.0, 0)
	if cost.Total != 0 {
		t.Error("Zero quantity should have zero cost")
	}

	// 零价格
	cost = cm.CalculateCost(OrderBuy, 0, 100)
	if cost.Total != 0 {
		t.Error("Zero price should have zero cost")
	}

	// 极小交易
	cost = cm.CalculateCost(OrderBuy, 0.01, 1)
	if cost.Commission < 5.0 {
		t.Error("Min commission should apply")
	}
}

func TestRoundPrice(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{10.123, 10.12},
		{10.125, 10.13},
		{10.127, 10.13},
		{9.999, 10.00},
		{0.001, 0.00},
	}

	for _, tt := range tests {
		got := roundPrice(tt.input)
		if math.Abs(got-tt.expected) > 0.001 {
			t.Errorf("roundPrice(%.3f) = %.2f, want %.2f", tt.input, got, tt.expected)
		}
	}
}

func TestRejectCode_String(t *testing.T) {
	tests := []struct {
		code RejectCode
		want string
	}{
		{RejectPriceLimit, "价格触及涨跌停, 无法成交"},
		{RejectSuspended, "股票停牌, 无法交易"},
		{RejectT1Restriction, "T+1 限制, 当日不可卖出"},
		{RejectInvalidQty, "交易数量不符合最小单位"},
		{RejectInsufficient, "资金或持仓不足"},
		{RejectZeroQuantity, "交易数量为 0"},
		{RejectBoardLimit, "板块涨跌停限制"},
	}

	for _, tt := range tests {
		got := tt.code.String()
		if got != tt.want {
			t.Errorf("RejectCode(%s).String() = %q, want %q", tt.code, got, tt.want)
		}
	}
}
