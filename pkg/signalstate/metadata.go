package signalstate

// Direction 是状态的方向语义。前端只读它，不得自己推导买卖方向：
// 「突破上轨 = 买、跌破下轨 = 卖」这类口径一旦出现在前端，就会像旧的
// web/src/types/screen.ts / SIGNAL_OPTIONS 那样和后端漂移。
type Direction string

const (
	DirectionBuy          Direction = "buy"
	DirectionSell         Direction = "sell"
	DirectionNeutral      Direction = "neutral"
	DirectionInsufficient Direction = "insufficient_data"
)

// DirectionOf 是方向口径的唯一实现：三态的方向由这里定死。
func DirectionOf(state State) Direction {
	switch state {
	case StateBuy:
		return DirectionBuy
	case StateSell:
		return DirectionSell
	case StateNeutral:
		return DirectionNeutral
	default:
		return DirectionInsufficient
	}
}

// 配色口径（A股语义：买/涨红、卖/跌绿），前端不再定义任何状态颜色。
// 中性取低饱和石板灰，让买卖两态在图上跳出来；数据不足取琥珀色，
// 与中性是两种完全不同的视觉，避免「没数据」被看成「中性」。
const (
	ColorBuy          = "#ef4444"
	ColorSell         = "#22c55e"
	ColorNeutral      = "#475569"
	ColorInsufficient = "#f59e0b"
)

// StateMeta 描述一个状态在界面上的标签、方向与颜色。
type StateMeta struct {
	State       State     `json:"state"`
	Label       string    `json:"label"`
	Direction   Direction `json:"direction"`
	Color       string    `json:"color"`
	Description string    `json:"description"`
}

// DimMeta 描述一个维度：它是什么、四态各自的标签与颜色。
type DimMeta struct {
	Dim         Dim         `json:"dim"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	States      []StateMeta `json:"states"`
}

func stateMeta(state State, label, description string) StateMeta {
	return StateMeta{
		State:       state,
		Label:       label,
		Direction:   DirectionOf(state),
		Color:       colorOf(state),
		Description: description,
	}
}

func colorOf(state State) string {
	switch state {
	case StateBuy:
		return ColorBuy
	case StateSell:
		return ColorSell
	case StateNeutral:
		return ColorNeutral
	default:
		return ColorInsufficient
	}
}

// insufficientMeta 是四维共用的数据不足态（不作为三态之一展示）。
func insufficientMeta() StateMeta {
	return stateMeta(StateInsufficient, "数据不足", "该维度当天算不出来（热身不足或停牌无 bar），不属于中性，也不参与筛选")
}

// dimMetaTable 是四维标签的唯一事实源，键取自 dimOrder。
var dimMetaTable = map[Dim]DimMeta{
	DimCross: {
		Dim:   DimCross,
		Label: "交叉态",
		Description: "MACD 的 DIF 与 DEA 的相对位置。DIF 在 DEA 上方为金叉态，下方为死叉态，" +
			"两者差值落在缠绕阈值带内时为缠绕（中性）。",
		States: []StateMeta{
			stateMeta(StateBuy, "金叉", "DIF 站在 DEA 上方且差值超过缠绕阈值"),
			stateMeta(StateSell, "死叉", "DIF 落在 DEA 下方且差值超过缠绕阈值"),
			stateMeta(StateNeutral, "缠绕", "DIF 与 DEA 的差值落在缠绕阈值带内，方向未定"),
			insufficientMeta(),
		},
	},
	DimOsc: {
		Dim:   DimOsc,
		Label: "超买超卖态",
		Description: "KDJ 的 J 值位置。J 低于超卖阈值为超卖，高于超买阈值为超买，" +
			"两者之间为中间区（中性）。",
		States: []StateMeta{
			stateMeta(StateBuy, "超卖", "J 值跌到超卖阈值以下，短线跌过头"),
			stateMeta(StateSell, "超买", "J 值涨到超买阈值以上，短线涨过头"),
			stateMeta(StateNeutral, "中间区", "J 值位于超卖与超买阈值之间"),
			insufficientMeta(),
		},
	},
	DimBoll: {
		Dim:   DimBoll,
		Label: "布林位置态",
		Description: "收盘价相对布林上下轨的位置。进入上轨触发带（上轨附近区域）为买入态，" +
			"进入下轨触发带（下轨附近区域）为卖出态，带内中间区域为中性。",
		States: []StateMeta{
			stateMeta(StateBuy, "突破上轨", "收盘价落在上轨触发带内，即突破点上下的区域"),
			stateMeta(StateSell, "跌破下轨", "收盘价落在下轨触发带内，即跌破点上下的区域"),
			stateMeta(StateNeutral, "带内中间区", "收盘价在上下轨之间且不在两侧触发带内"),
			insufficientMeta(),
		},
	},
	DimMA: {
		Dim:   DimMA,
		Label: "均线排列态",
		Description: "均线多空排列。短周期在上、长周期在下且间距足够为多头排列，" +
			"反之为空头排列，其余为均线纠缠（中性）。",
		States: []StateMeta{
			stateMeta(StateBuy, "多头排列", "短周期均线依次高于长周期均线，且间距达标"),
			stateMeta(StateSell, "空头排列", "短周期均线依次低于长周期均线，且间距达标"),
			stateMeta(StateNeutral, "纠缠", "均线未形成单边排列，或排列间距过小"),
			insufficientMeta(),
		},
	},
}

// Metadata 按 dimOrder 返回四维的标签/方向/配色元数据。接口与前端都从这里取，
// 保证「买卖方向、三态文案、配色」只有后端一处来源。
func Metadata() []DimMeta {
	out := make([]DimMeta, 0, len(dimOrder))
	for _, dim := range dimOrder {
		meta := dimMetaTable[dim]
		meta.States = append([]StateMeta(nil), meta.States...)
		out = append(out, meta)
	}
	return out
}

// MetadataFor 返回单个维度的元数据。
func MetadataFor(dim Dim) (DimMeta, bool) {
	meta, ok := dimMetaTable[dim]
	if !ok {
		return DimMeta{}, false
	}
	meta.States = append([]StateMeta(nil), meta.States...)
	return meta, true
}

// StateMetaFor 返回某维度某状态的元数据（含标签、方向、颜色）。
func StateMetaFor(dim Dim, state State) (StateMeta, bool) {
	meta, ok := dimMetaTable[dim]
	if !ok {
		return StateMeta{}, false
	}
	for _, s := range meta.States {
		if s.State == state {
			return s, true
		}
	}
	return StateMeta{}, false
}
