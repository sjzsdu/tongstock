// Package trading 实现符合 A 股交易规则的成交与成本模型。
//
// 核心能力:
//   - 可配置的成本模型 (佣金/印花税/过户费/滑点)
//   - A 股交易约束检查 (涨跌停/停牌/T+1/最小交易单位)
//   - 订单执行引擎 (信号下单时点/不可成交处理)
//   - 毛收益与净收益分离报告
package trading

import (
	"fmt"
	"time"
)

// ============================================================================
// 股票板块与涨跌停幅度
// ============================================================================

// Board 股票板块, 决定涨跌停幅度。
type Board string

const (
	// BoardMain 主板: ±10% 涨跌停
	BoardMain Board = "main"
	// BoardChiNext 创业板: ±20% 涨跌停
	BoardChiNext Board = "chinext"
	// BoardSTAR 科创板: ±20% 涨跌停
	BoardSTAR Board = "star"
	// BoardBJ 北交所: ±30% 涨跌停
	BoardBJ Board = "bj"
	// BoardUnknown 未知板块: 按主板处理
	BoardUnknown Board = ""
)

// DailyLimit 返回板块的每日涨跌停百分比。
func (b Board) DailyLimit() float64 {
	switch b {
	case BoardChiNext, BoardSTAR:
		return 0.20
	case BoardBJ:
		return 0.30
	default:
		return 0.10
	}
}

// ============================================================================
// 交易约束
// ============================================================================

// TradingConstraints A 股交易约束配置。
type TradingConstraints struct {
	// Board 股票板块, 决定涨跌停幅度
	Board Board `json:"board"`
	// MinTradeUnit 最小交易单位 (股数), 默认 100
	MinTradeUnit int `json:"min_trade_unit"`
	// EnableT1 是否启用 T+1 约束
	EnableT1 bool `json:"enable_t_1"`
	// EnablePriceLimit 是否启用涨跌停约束
	EnablePriceLimit bool `json:"enable_price_limit"`
	// EnableSuspension 是否启用品牌/停牌检查
	EnableSuspension bool `json:"enable_suspension"`
	// IPODays 上市天数, 前 N 天无涨跌停限制 (0 = 不特殊处理)
	IPODays int `json:"ipo_days"`
}

// DefaultTradingConstraints 返回默认 A 股交易约束。
func DefaultTradingConstraints() TradingConstraints {
	return TradingConstraints{
		Board:            BoardMain,
		MinTradeUnit:     100,
		EnableT1:         true,
		EnablePriceLimit: true,
		EnableSuspension: true,
		IPODays:          0,
	}
}

// ValidateQuantity 检查交易数量是否满足最小交易单位要求。
// 返回修正后的数量 (向下取整到整手) 和是否通过校验。
func (c TradingConstraints) ValidateQuantity(quantity int) (int, bool) {
	if c.MinTradeUnit <= 0 {
		return quantity, true
	}
	// 向下取整到整手
	adjusted := (quantity / c.MinTradeUnit) * c.MinTradeUnit
	if adjusted <= 0 {
		return 0, false
	}
	return adjusted, adjusted == quantity
}

// MaxQuantityAtPriceLimit 在涨停价计算最大可买数量 (受涨跌停价格约束)。
// price: 当前价格, limitPrice: 涨停价, availableCash: 可用资金
func (c TradingConstraints) MaxQuantityAtPriceLimit(price, availableCash float64) int {
	if price <= 0 || availableCash <= 0 {
		return 0
	}
	// 预估成本价 = 价格 * (1 + 费率)
	estimatedCost := price * 1.003 // 预估 0.3% 总费率
	maxShares := int(availableCash / estimatedCost)
	adjusted, _ := c.ValidateQuantity(maxShares)
	return adjusted
}

// ============================================================================
// 成本模型
// ============================================================================

// CostModel A 股交易成本模型。
type CostModel struct {
	// CommissionRate 佣金率 (单边), 默认 0.00025 (万 2.5)
	CommissionRate float64 `json:"commission_rate"`
	// MinCommission 最低佣金 (元), 默认 5 元
	MinCommission float64 `json:"min_commission"`
	// StampDutyRate 印花税率 (仅卖出), 默认 0.0005 (万 5)
	StampDutyRate float64 `json:"stamp_duty_rate"`
	// TransferFeeRate 过户费率 (双边), 默认 0.00001 (万 0.1)
	TransferFeeRate float64 `json:"transfer_fee_rate"`
	// SlippageBps 滑点 (基点), 单边, 默认 10 bps (0.1%)
	SlippageBps float64 `json:"slippage_bps"`
	// EnableStampDuty 是否征收印花税 (可配置回测)
	EnableStampDuty bool `json:"enable_stamp_duty"`
}

// DefaultCostModel 返回默认 A 股成本模型。
func DefaultCostModel() CostModel {
	return CostModel{
		CommissionRate:  0.00025, // 万 2.5
		MinCommission:   5.0,     // 最低 5 元
		StampDutyRate:   0.0005,  // 万 5 (仅卖出)
		TransferFeeRate: 0.00001, // 万 0.1 (双边)
		SlippageBps:     10.0,    // 10 bps 滑点 (由引擎应用到执行价格, 成本模型中不再单独计算)
		EnableStampDuty: true,
	}
}

// TradingCost 单笔交易的成本明细。
type TradingCost struct {
	Commission   float64 `json:"commission"`    // 佣金
	StampDuty    float64 `json:"stamp_duty"`    // 印花税 (仅卖出)
	TransferFee  float64 `json:"transfer_fee"`  // 过户费
	SlippageCost float64 `json:"slippage_cost"` // 滑点成本
	Total        float64 `json:"total"`         // 总成本
}

// CalculateCost 计算单笔交易的成本。
// 注意: 滑点已经由引擎应用到执行价格上, 此函数不再单独计算滑点成本
// action: buy/sell, price: 成交价 (已含滑点), quantity: 数量
func (cm CostModel) CalculateCost(action OrderSide, price float64, quantity int) TradingCost {
	tradeValue := price * float64(quantity)

	var cost TradingCost

	// 佣金 (最低 5 元)
	cost.Commission = tradeValue * cm.CommissionRate
	if cost.Commission < cm.MinCommission && tradeValue > 0 {
		cost.Commission = cm.MinCommission
	}

	// 印花税 (仅卖出)
	if action == OrderSell && cm.EnableStampDuty {
		cost.StampDuty = tradeValue * cm.StampDutyRate
	}

	// 过户费 (双边)
	cost.TransferFee = tradeValue * cm.TransferFeeRate

	// 滑点成本为 0 (已通过执行价格体现)
	cost.SlippageCost = 0

	// 总成本 = 佣金 + 印花税 + 过户费 (滑点已含在执行价中)
	cost.Total = cost.Commission + cost.StampDuty + cost.TransferFee

	return cost
}

// GrossToNet 将毛收益转换为净收益 (扣除双边成本)。
// grossPnL: 毛收益 (元)
// tradeValue: 交易金额 (元)
// action: buy/sell
func (cm CostModel) GrossToNet(grossPnL, tradeValue float64, action OrderSide) float64 {
	if tradeValue <= 0 {
		return grossPnL
	}
	// 近似: 双边成本 = 2 * 佣金 + 印花税 (卖出) + 2 * 过户费 + 2 * 滑点
	roundTripCost := tradeValue * (2*cm.CommissionRate + 2*cm.TransferFeeRate + 2*cm.SlippageBps/10000.0)
	if action == OrderSell && cm.EnableStampDuty {
		roundTripCost += tradeValue * cm.StampDutyRate
	}
	return grossPnL - roundTripCost
}

// ============================================================================
// 订单与成交
// ============================================================================

// OrderSide 订单方向。
type OrderSide string

const (
	OrderBuy  OrderSide = "buy"
	OrderSell OrderSide = "sell"
)

// OrderType 订单类型。
type OrderType string

const (
	// OrderMarket 市价单: 按当前市场价格立即成交
	OrderMarket OrderType = "market"
	// OrderLimit 限价单: 按指定价格或更优价格成交
	OrderLimit OrderType = "limit"
	// OrderStop 止损单: 价格触及触发价后转为市价
	OrderStop OrderType = "stop"
)

// Order 回测订单。
type Order struct {
	ID            string    `json:"id"`
	StockCode     string    `json:"stock_code"`
	Side          OrderSide `json:"side"`
	Type          OrderType `json:"type"`
	Quantity      int       `json:"quantity"`
	LimitPrice    float64   `json:"limit_price,omitempty"`
	SignalDate    time.Time `json:"signal_date"`    // 信号产生日期
	ExecutionDate time.Time `json:"execution_date"` // 预期执行日期
	Reason        string    `json:"reason"`
}

// Fill 成交记录。
type Fill struct {
	ID            string      `json:"id"`
	OrderID       string      `json:"order_id"`
	StockCode     string      `json:"stock_code"`
	Side          OrderSide   `json:"side"`
	Quantity      int         `json:"quantity"`
	Price         float64     `json:"price"`        // 实际成交价 (含滑点后)
	SignalPrice   float64     `json:"signal_price"` // 信号价格 (理想价格)
	ExecutionDate time.Time   `json:"execution_date"`
	Cost          TradingCost `json:"cost"`
	Status        FillStatus  `json:"status"`
	RejectReason  string      `json:"reject_reason,omitempty"`
}

// FillStatus 成交状态。
type FillStatus string

const (
	FillFilled    FillStatus = "filled"    // 完全成交
	FillPartial   FillStatus = "partial"   // 部分成交
	FillRejected  FillStatus = "rejected"  // 被拒绝 (不可成交)
	FillCancelled FillStatus = "cancelled" // 已取消
)

// ============================================================================
// 市场数据 (用于执行决策)
// ============================================================================

// MarketSnapshot 某时点的市场快照。
type MarketSnapshot struct {
	Date      time.Time `json:"date"`
	StockCode string    `json:"stock_code"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	PreClose  float64   `json:"pre_close"`  // 前收盘价
	Suspended bool      `json:"suspended"`  // 是否停牌
	LimitUp   float64   `json:"limit_up"`   // 涨停价
	LimitDown float64   `json:"limit_down"` // 跌停价
	Board     Board     `json:"board"`
	IPODays   int       `json:"ipo_days"` // 上市天数
}

// CalculateLimits 基于前收盘价计算涨跌停价格。
func CalculateLimits(preClose float64, board Board) (limitUp, limitDown float64) {
	limit := board.DailyLimit()
	limitUp = roundPrice(preClose * (1 + limit))
	limitDown = roundPrice(preClose * (1 - limit))
	if limitDown < 0 {
		limitDown = 0
	}
	return
}

// roundPrice 按 A 股最小价格单位 (0.01 元) 四舍五入。
func roundPrice(price float64) float64 {
	return float64(int64(price*100+0.5)) / 100.0
}

// ============================================================================
// 执行结果
// ============================================================================

// ExecutionResult 订单执行结果。
type ExecutionResult struct {
	Order      Order      `json:"order"`
	Fill       *Fill      `json:"fill,omitempty"`
	Rejected   bool       `json:"rejected"`
	RejectCode RejectCode `json:"reject_code,omitempty"`
	RejectMsg  string     `json:"reject_msg,omitempty"`
}

// RejectCode 拒绝原因代码。
type RejectCode string

const (
	RejectPriceLimit    RejectCode = "price_limit"  // 涨跌停无法成交
	RejectSuspended     RejectCode = "suspended"    // 停牌
	RejectT1Restriction RejectCode = "t_1"          // T+1 限制
	RejectInvalidQty    RejectCode = "invalid_qty"  // 数量不合法
	RejectInsufficient  RejectCode = "insufficient" // 资金或持仓不足
	RejectZeroQuantity  RejectCode = "zero_qty"     // 数量为 0
	RejectBoardLimit    RejectCode = "board_limit"  // 板块涨跌停
)

func (r RejectCode) String() string {
	switch r {
	case RejectPriceLimit:
		return "价格触及涨跌停, 无法成交"
	case RejectSuspended:
		return "股票停牌, 无法交易"
	case RejectT1Restriction:
		return "T+1 限制, 当日不可卖出"
	case RejectInvalidQty:
		return "交易数量不符合最小单位"
	case RejectInsufficient:
		return "资金或持仓不足"
	case RejectZeroQuantity:
		return "交易数量为 0"
	case RejectBoardLimit:
		return "板块涨跌停限制"
	default:
		return string(r)
	}
}

// ============================================================================
// 执行引擎
// ============================================================================

// ExecutionEngine 订单执行引擎。
type ExecutionEngine struct {
	constraints TradingConstraints
	costModel   CostModel
	// 持仓记录: stock -> {quantity, buyDate}
	positions map[string]Position
	// 可用资金
	cash float64
}

// Position 持仓信息。
type Position struct {
	Quantity int       `json:"quantity"`
	BuyDate  time.Time `json:"buy_date"` // 最早买入日期 (用于 T+1 检查)
}

// NewExecutionEngine 创建执行引擎。
func NewExecutionEngine(constraints TradingConstraints, costModel CostModel) *ExecutionEngine {
	return &ExecutionEngine{
		constraints: constraints,
		costModel:   costModel,
		positions:   make(map[string]Position),
		cash:        0,
	}
}

// SetCash 设置可用资金。
func (e *ExecutionEngine) SetCash(cash float64) {
	e.cash = cash
}

// SetPosition 设置持仓。
func (e *ExecutionEngine) SetPosition(stockCode string, quantity int, buyDate time.Time) {
	e.positions[stockCode] = Position{Quantity: quantity, BuyDate: buyDate}
}

// GetPosition 获取持仓。
func (e *ExecutionEngine) GetPosition(stockCode string) (Position, bool) {
	p, ok := e.positions[stockCode]
	return p, ok
}

// Execute 执行订单, 返回执行结果。
func (e *ExecutionEngine) Execute(order Order, snapshot MarketSnapshot) ExecutionResult {
	result := ExecutionResult{Order: order}

	// Step 1: 数量校验
	adjustedQty, valid := e.constraints.ValidateQuantity(order.Quantity)
	if !valid && adjustedQty == 0 {
		result.Rejected = true
		result.RejectCode = RejectZeroQuantity
		result.RejectMsg = RejectZeroQuantity.String()
		return result
	}
	if !valid {
		// 非整手: 调整数量并继续 (发出警告)
		order.Quantity = adjustedQty
	}
	if adjustedQty <= 0 {
		result.Rejected = true
		result.RejectCode = RejectZeroQuantity
		result.RejectMsg = "调整后数量为 0"
		return result
	}

	// Step 2: 停牌检查
	if e.constraints.EnableSuspension && snapshot.Suspended {
		result.Rejected = true
		result.RejectCode = RejectSuspended
		result.RejectMsg = RejectSuspended.String()
		return result
	}

	// Step 3: 涨跌停检查 (非 IPO 前 N 天)
	inIPO := e.constraints.IPODays > 0 && snapshot.IPODays <= e.constraints.IPODays
	if e.constraints.EnablePriceLimit && !inIPO {
		if order.Side == OrderBuy {
			if snapshot.Close >= snapshot.LimitUp {
				result.Rejected = true
				result.RejectCode = RejectPriceLimit
				result.RejectMsg = fmt.Sprintf("涨停价 %.2f, 无法买入", snapshot.LimitUp)
				return result
			}
		} else if order.Side == OrderSell {
			if snapshot.Close <= snapshot.LimitDown {
				result.Rejected = true
				result.RejectCode = RejectPriceLimit
				result.RejectMsg = fmt.Sprintf("跌停价 %.2f, 无法卖出", snapshot.LimitDown)
				return result
			}
		}
	}

	// Step 4: T+1 检查
	if e.constraints.EnableT1 && order.Side == OrderSell {
		pos, ok := e.positions[order.StockCode]
		if !ok || pos.Quantity < adjustedQty {
			result.Rejected = true
			result.RejectCode = RejectInsufficient
			result.RejectMsg = "持仓不足或无持仓"
			return result
		}
		// 检查买入日期 (T+1: 买入次日才能卖出)
		if !pos.BuyDate.IsZero() && snapshot.Date.Before(pos.BuyDate.AddDate(0, 0, 1)) {
			result.Rejected = true
			result.RejectCode = RejectT1Restriction
			result.RejectMsg = fmt.Sprintf("T+1 限制: 买入于 %s, 最早 %s 可卖出",
				pos.BuyDate.Format("2006-01-02"),
				pos.BuyDate.AddDate(0, 0, 1).Format("2006-01-02"))
			return result
		}
	}

	// Step 5: 资金检查 (买入)
	if order.Side == OrderBuy {
		estPrice := snapshot.Close * (1 + e.costModel.SlippageBps/10000.0)
		estCost := e.costModel.CalculateCost(OrderBuy, estPrice, adjustedQty)
		required := estPrice*float64(adjustedQty) + estCost.Total
		if required > e.cash {
			result.Rejected = true
			result.RejectCode = RejectInsufficient
			result.RejectMsg = fmt.Sprintf("资金不足: 需要 %.2f, 可用 %.2f", required, e.cash)
			return result
		}
	}

	// Step 6: 计算实际成交价 (含滑点)
	execPrice := e.applySlippage(snapshot.Close, order.Side)

	// Step 7: 扣减成本
	cost := e.costModel.CalculateCost(order.Side, execPrice, adjustedQty)

	// Step 8: 生成成交
	fill := &Fill{
		ID:            fmt.Sprintf("fill-%s-%s", order.ID, snapshot.Date.Format("20060102")),
		OrderID:       order.ID,
		StockCode:     order.StockCode,
		Side:          order.Side,
		Quantity:      adjustedQty,
		Price:         execPrice,
		SignalPrice:   snapshot.Close,
		ExecutionDate: snapshot.Date,
		Cost:          cost,
		Status:        FillFilled,
	}

	// Step 9: 更新持仓和资金
	if order.Side == OrderBuy {
		e.cash -= execPrice*float64(adjustedQty) + cost.Total
		existing, ok := e.positions[order.StockCode]
		if ok && !existing.BuyDate.IsZero() {
			// 加权平均买入日期
			totalQty := existing.Quantity + adjustedQty
			avgDate := weightedDate(existing.BuyDate, existing.Quantity, snapshot.Date, adjustedQty)
			e.positions[order.StockCode] = Position{Quantity: totalQty, BuyDate: avgDate}
		} else {
			e.positions[order.StockCode] = Position{Quantity: adjustedQty, BuyDate: snapshot.Date}
		}
	} else {
		e.cash += execPrice*float64(adjustedQty) - cost.Total
		if pos, ok := e.positions[order.StockCode]; ok {
			remaining := pos.Quantity - adjustedQty
			if remaining <= 0 {
				delete(e.positions, order.StockCode)
			} else {
				e.positions[order.StockCode] = Position{Quantity: remaining, BuyDate: pos.BuyDate}
			}
		}
	}

	result.Fill = fill
	return result
}

// applySlippage 应用滑点: 买入价格上浮, 卖出价格下浮。
func (e *ExecutionEngine) applySlippage(price float64, side OrderSide) float64 {
	slippageMult := e.costModel.SlippageBps / 10000.0
	if side == OrderBuy {
		return roundPrice(price * (1 + slippageMult))
	}
	return roundPrice(price * (1 - slippageMult))
}

// weightedDate 计算加权平均日期。
func weightedDate(d1 time.Time, q1 int, d2 time.Time, q2 int) time.Time {
	// 简化处理: 按数量加权, 以天为单位
	total := float64(q1 + q2)
	if total == 0 {
		return d2
	}
	days1 := float64(d1.Unix())
	days2 := float64(d2.Unix())
	weighted := int64((days1*float64(q1) + days2*float64(q2)) / total)
	return time.Unix(weighted, 0)
}

// ============================================================================
// 回测报告
// ============================================================================

// BacktestReport 回测报告 (毛收益与净收益分离)。
type BacktestReport struct {
	StockCode   string         `json:"stock_code"`
	StartDate   time.Time      `json:"start_date"`
	EndDate     time.Time      `json:"end_date"`
	InitialCash float64        `json:"initial_cash"`
	FinalValue  float64        `json:"final_value"`
	GrossPnL    float64        `json:"gross_pnl"`   // 毛收益 (不含成本)
	NetPnL      float64        `json:"net_pnl"`     // 净收益 (扣成本后)
	TotalCost   float64        `json:"total_cost"`  // 总成本
	TotalFills  int            `json:"total_fills"` // 成交笔数
	WinRate     float64        `json:"win_rate"`    // 胜率
	Config      BacktestConfig `json:"config"`      // 回测配置
}

// BacktestConfig 回测配置 (可复现)。
type BacktestConfig struct {
	StockCode        string  `json:"stock_code"`
	StartDate        string  `json:"start_date"`
	EndDate          string  `json:"end_date"`
	InitialCash      float64 `json:"initial_cash"`
	PositionSize     float64 `json:"position_size"` // 单笔仓位比例 (0-1)
	CommissionRate   float64 `json:"commission_rate"`
	StampDutyRate    float64 `json:"stamp_duty_rate"`
	SlippageBps      float64 `json:"slippage_bps"`
	EnableT1         bool    `json:"enable_t_1"`
	EnablePriceLimit bool    `json:"enable_price_limit"`
	Board            Board   `json:"board"`
	SignalTiming     string  `json:"signal_timing"` // close_to_open / same_bar
}

// NewBacktestConfig 默认回测配置。
func NewBacktestConfig(stockCode, startDate, endDate string) BacktestConfig {
	return BacktestConfig{
		StockCode:        stockCode,
		StartDate:        startDate,
		EndDate:          endDate,
		InitialCash:      1000000.0,
		PositionSize:     0.1,
		CommissionRate:   0.00025,
		StampDutyRate:    0.0005,
		SlippageBps:      10.0,
		EnableT1:         true,
		EnablePriceLimit: true,
		Board:            BoardMain,
		SignalTiming:     "close_to_open",
	}
}

// Validate 验证回测配置。
func (c BacktestConfig) Validate() error {
	if c.InitialCash <= 0 {
		return fmt.Errorf("初始资金必须大于 0")
	}
	if c.PositionSize <= 0 || c.PositionSize > 1 {
		return fmt.Errorf("仓位比例必须在 (0, 1] 之间")
	}
	if c.CommissionRate < 0 || c.CommissionRate > 0.1 {
		return fmt.Errorf("佣金率不合法")
	}
	if c.SlippageBps < 0 || c.SlippageBps > 500 {
		return fmt.Errorf("滑点不合法")
	}
	return nil
}

// SignalTimingStrategy 信号下单时点策略。
type SignalTimingStrategy string

const (
	// SignalNextOpen 信号收盘产生, 次日开盘下单
	SignalNextOpen SignalTimingStrategy = "next_open"
	// SignalSameBarOpen 信号产生后当根 bar 开盘价成交
	SignalSameBarOpen SignalTimingStrategy = "same_bar_open"
	// SignalNextClose 信号产生后次日收盘成交
	SignalNextClose SignalTimingStrategy = "next_close"
)

// OrderFromSignal 根据信号生成订单。
func OrderFromSignal(stockCode string, side OrderSide, quantity int, signalDate time.Time, strategy SignalTimingStrategy) Order {
	var execDate time.Time
	switch strategy {
	case SignalSameBarOpen:
		execDate = signalDate
	default:
		execDate = signalDate.AddDate(0, 0, 1)
		// 简化: 不考虑节假日, 实际使用需要跳过非交易日
	}

	return Order{
		ID:            fmt.Sprintf("ord-%s-%s-%s", stockCode, side, signalDate.Format("20060102")),
		StockCode:     stockCode,
		Side:          side,
		Type:          OrderMarket,
		Quantity:      quantity,
		SignalDate:    signalDate,
		ExecutionDate: execDate,
		Reason:        string(strategy),
	}
}
