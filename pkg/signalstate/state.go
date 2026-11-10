package signalstate

import "time"

// Dim 是一个状态维度。取值固定为四维，顺序由 Dims 给出。
type Dim string

const (
	// DimCross 交叉态：MACD 的 DIF/DEA 相对位置（金叉 / 死叉 / 缠绕）。
	DimCross Dim = "cross"
	// DimOsc 超买超卖态：KDJ 的 J 值位置（超卖 / 超买 / 中间区）。
	DimOsc Dim = "osc"
	// DimBoll 布林位置态：收盘价相对上下轨触发带（突破上轨 / 跌破下轨 / 带内中间区）。
	DimBoll Dim = "boll"
	// DimMA 均线排列态：均线多空排列（多头排列 / 空头排列 / 纠缠）。
	DimMA Dim = "ma"
)

// dimOrder 是四维的规范顺序：序列化、渲染、表格列都按它，保证前后端一致。
var dimOrder = [4]Dim{DimCross, DimOsc, DimBoll, DimMA}

// Dims 返回四维的规范顺序（副本，调用方可随意修改）。
func Dims() [4]Dim { return dimOrder }

// Valid 报告 d 是否是已定义维度。
func (d Dim) Valid() bool {
	for _, known := range dimOrder {
		if d == known {
			return true
		}
	}
	return false
}

// State 是一个维度在某个交易日的状态。
//
// 三态（买入 / 卖出 / 中性）是业务语义的全部取值；StateInsufficient 是独立的
// 第四种技术态，只表示「这一天的这个维度算不出来」（热身不足、停牌无 bar、
// 带宽为 0 等），绝不能被当成中性渲染。把两者混成一个值会让「没数据」悄悄变成
// 「判断为中性」，这正是本包要禁止的。
type State string

const (
	StateBuy          State = "buy"
	StateSell         State = "sell"
	StateNeutral      State = "neutral"
	StateInsufficient State = "insufficient_data"
)

// IsTriState 报告 s 是否属于三态之一（即：是否为一次有效判定）。
func (s State) IsTriState() bool {
	return s == StateBuy || s == StateSell || s == StateNeutral
}

// StateVector 是一只股票某个交易日的状态向量。
type StateVector struct {
	CalcHash string // SHA-256 of definition, causal input prefix and state vector.
	Code     string
	Date     time.Time
	KType    uint8
	// Dims 对每个维度给一个状态；四维必须齐全。数据不足的维度取
	// StateInsufficient，且同一维度在 Insufficient 中为 true。
	Dims map[Dim]State
	// Insufficient 显式标出算不出结果的维度，与 Dims 同步维护。
	Insufficient map[Dim]bool
	// Reasons 记录每个维度的判定依据（含具体数值），用来说明「中性是显式判定
	// 而不是兜底」：每条中性都写清是哪个条件不满足。
	Reasons map[Dim]string
	// AdjustVersion 是本条状态所用 K 线的复权口径，本期固定为 "raw"（未复权）。
	AdjustVersion string
	// CalcVersion 是内核计算版本，参数/算法变更时递增。
	CalcVersion string
	// Close 是当日收盘价（未复权），供「状态 × 股价」对照视图直接使用。
	Close float64
}

// State 返回维度 d 的状态；未定义时返回 StateInsufficient。
func (v StateVector) State(d Dim) State {
	if s, ok := v.Dims[d]; ok {
		return s
	}
	return StateInsufficient
}

// IsInsufficient 报告维度 d 是否属于数据不足态。
func (v StateVector) IsInsufficient(d Dim) bool {
	if v.Insufficient[d] {
		return true
	}
	return v.State(d) == StateInsufficient
}

// HasInsufficient 报告是否至少有一个维度数据不足。
func (v StateVector) HasInsufficient() bool {
	for _, d := range dimOrder {
		if v.IsInsufficient(d) {
			return true
		}
	}
	return false
}
