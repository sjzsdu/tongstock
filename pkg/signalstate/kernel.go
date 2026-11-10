package signalstate

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sjzsdu/tongstock/pkg/ta"
)

// indicators 是一次性算好的指标输出，四个维度共用。全部指标都是因果的
// （只用 klines[0..i]），所以对全序列算一次与对每个前缀各算一次，同一根 bar
// 上的取值完全相同 —— 这正是「无未来函数」的判据，见 kernel_causality_test.go。
type indicators struct {
	macd *ta.MACDResult
	kdj  *ta.KDJResult
	boll *ta.BOLLResult
	ma   map[string][]float64
}

// Compute 返回 klines 最后一根 bar 的状态向量。
//
// 这是唯一的纯内核：只读入参，不读时间、不联网、不落库。对 klines[0..t] 调用
// 得到的向量，与对更长序列调用后取第 t 根得到的向量必须逐字段相同（追加
// 未来数据不得改写历史状态）。
func Compute(code string, klines []ta.KlineInput, def Definition) StateVector {
	if len(klines) == 0 {
		return emptyVector(code, def)
	}
	series := computeSeries(code, 0, klines, def)
	return series[len(series)-1]
}

// ComputeSeries 逐日返回状态向量，长度与 klines 一致（第 i 个对应 klines[i]）。
// 指标只算一次、逐 bar 判定，因此单股 8000+ 根日线也是毫秒级。
func ComputeSeries(code string, ktype uint8, klines []ta.KlineInput, def Definition) []StateVector {
	return computeSeries(code, ktype, klines, def)
}

func computeSeries(code string, ktype uint8, klines []ta.KlineInput, def Definition) []StateVector {
	def = def.Normalized()
	out := make([]StateVector, len(klines))
	if len(klines) == 0 {
		return out
	}
	ind := calcIndicators(klines, def)
	for i := range klines {
		out[i] = classify(code, ktype, klines, ind, def, i)
	}
	return out
}

func calcIndicators(klines []ta.KlineInput, def Definition) *indicators {
	macd, kdj, boll := def.MACD, def.KDJ, def.BOLL
	res := ta.Calculate(klines, &ta.IndicatorConfig{
		MA:   def.MAPeriods,
		MACD: &macd,
		KDJ:  &kdj,
		BOLL: &boll,
	})
	return &indicators{macd: res.MACD, kdj: res.KDJ, boll: res.BOLL, ma: res.MA}
}

func emptyVector(code string, def Definition) StateVector {
	v := newVector(code, 0, ta.KlineInput{}, def)
	for _, dim := range dimOrder {
		v.set(dim, StateInsufficient, "没有任何 K 线数据（停牌或本地无该股票日线）")
	}
	return v
}

func newVector(code string, ktype uint8, bar ta.KlineInput, def Definition) StateVector {
	return StateVector{
		Code:          code,
		Date:          bar.Time,
		KType:         ktype,
		Dims:          make(map[Dim]State, len(dimOrder)),
		Insufficient:  make(map[Dim]bool, len(dimOrder)),
		Reasons:       make(map[Dim]string, len(dimOrder)),
		AdjustVersion: AdjustRaw,
		CalcVersion:   CalcVersion,
		Close:         bar.Close,
	}
}

func (v *StateVector) set(dim Dim, state State, reason string) {
	v.Dims[dim] = state
	v.Reasons[dim] = reason
	if state == StateInsufficient {
		v.Insufficient[dim] = true
		return
	}
	delete(v.Insufficient, dim)
}

// classify 判定第 i 根 bar 的四维状态。每个维度都走「先判能否算、再显式判定
// 三态」两步，中性只可能来自明确的 else 分支（且写明原因），不存在「其他情况
// 一律算中性」的兜底。
func classify(code string, ktype uint8, klines []ta.KlineInput, ind *indicators, def Definition, i int) StateVector {
	bar := klines[i]
	v := newVector(code, ktype, bar, def)
	bars := i + 1

	for _, dim := range dimOrder {
		if need := def.minBars(dim); bars < need {
			v.set(dim, StateInsufficient, fmt.Sprintf(
				"指标热身不足：该维度需要 %d 根K线，当前仅 %d 根", need, bars))
			continue
		}
		switch dim {
		case DimCross:
			judgeCross(&v, ind, def, i, bar.Close)
		case DimOsc:
			judgeOsc(&v, ind, def, i)
		case DimBoll:
			judgeBoll(&v, ind, def, i, bar.Close)
		case DimMA:
			judgeMA(&v, ind, def, i, bar.Close)
		}
	}
	return v
}

func judgeCross(v *StateVector, ind *indicators, def Definition, i int, close float64) {
	if ind.macd == nil || i >= len(ind.macd.DIF) || i >= len(ind.macd.DEA) {
		v.set(DimCross, StateInsufficient, "MACD 输出长度不足，无法取到当日 DIF/DEA")
		return
	}
	if !(close > 0) {
		v.set(DimCross, StateInsufficient, "当日收盘价非正，无法按价格归一化缠绕阈值")
		return
	}
	dif, dea := ind.macd.DIF[i], ind.macd.DEA[i]
	diff := dif - dea
	band := def.CrossWrapRatio * close
	switch {
	case diff > band:
		v.set(DimCross, StateBuy, fmt.Sprintf(
			"DIF %.4f 在 DEA %.4f 上方，差值 %+.4f 超过缠绕带 ±%.4f（收盘价 %.2f × %.3f）",
			dif, dea, diff, band, close, def.CrossWrapRatio))
	case diff < -band:
		v.set(DimCross, StateSell, fmt.Sprintf(
			"DIF %.4f 在 DEA %.4f 下方，差值 %+.4f 低于缠绕带 -%.4f（收盘价 %.2f × %.3f）",
			dif, dea, diff, band, close, def.CrossWrapRatio))
	default:
		v.set(DimCross, StateNeutral, fmt.Sprintf(
			"DIF %.4f 与 DEA %.4f 的差值 %+.4f 落在缠绕带 ±%.4f 内（收盘价 %.2f × %.3f）→ DIF/DEA 缠绕",
			dif, dea, diff, band, close, def.CrossWrapRatio))
	}
}

func judgeOsc(v *StateVector, ind *indicators, def Definition, i int) {
	if ind.kdj == nil || i >= len(ind.kdj.J) || i < def.KDJ.N-1 {
		v.set(DimOsc, StateInsufficient, "KDJ 输出长度不足，无法取到当日 J 值")
		return
	}
	j := ind.kdj.J[i]
	switch {
	case j <= def.OscOversoldJ:
		v.set(DimOsc, StateBuy, fmt.Sprintf(
			"J %.1f ≤ 超卖阈值 %.1f → 超卖", j, def.OscOversoldJ))
	case j >= def.OscOverboughtJ:
		v.set(DimOsc, StateSell, fmt.Sprintf(
			"J %.1f ≥ 超买阈值 %.1f → 超买", j, def.OscOverboughtJ))
	default:
		v.set(DimOsc, StateNeutral, fmt.Sprintf(
			"J %.1f 落在中间区 (%.1f, %.1f)，既未超卖也未超买",
			j, def.OscOversoldJ, def.OscOverboughtJ))
	}
}

func judgeBoll(v *StateVector, ind *indicators, def Definition, i int, close float64) {
	if ind.boll == nil || i >= len(ind.boll.Upper) || i >= len(ind.boll.Lower) {
		v.set(DimBoll, StateInsufficient, "BOLL 输出长度不足，无法取到当日上下轨")
		return
	}
	upper, lower := ind.boll.Upper[i], ind.boll.Lower[i]
	width := upper - lower
	if !(width > 0) {
		v.set(DimBoll, StateInsufficient, fmt.Sprintf(
			"布林带宽为 0（%d 日窗口内收盘价无波动），无法划定触发带", def.BOLL.N))
		return
	}
	zone := def.BollZoneRatio * width
	buyEdge := upper - zone
	sellEdge := lower + zone
	switch {
	case close >= buyEdge:
		v.set(DimBoll, StateBuy, fmt.Sprintf(
			"收盘价 %.2f ≥ 上轨触发带下沿 %.2f（上轨 %.2f - %.3f×带宽 %.2f）→ 突破或贴近上轨",
			close, buyEdge, upper, def.BollZoneRatio, width))
	case close <= sellEdge:
		v.set(DimBoll, StateSell, fmt.Sprintf(
			"收盘价 %.2f ≤ 下轨触发带上沿 %.2f（下轨 %.2f + %.3f×带宽 %.2f）→ 跌破或贴近下轨",
			close, sellEdge, lower, def.BollZoneRatio, width))
	default:
		v.set(DimBoll, StateNeutral, fmt.Sprintf(
			"收盘价 %.2f 位于下轨触发带上沿 %.2f 与上轨触发带下沿 %.2f 之间 → 带内中间区",
			close, sellEdge, buyEdge))
	}
}

func judgeMA(v *StateVector, ind *indicators, def Definition, i int, close float64) {
	if !(close > 0) {
		v.set(DimMA, StateInsufficient, "当日收盘价非正，无法按价格归一化均线间距")
		return
	}
	values := make([]float64, len(def.MAPeriods))
	for idx, period := range def.MAPeriods {
		series, ok := ind.ma[strconv.Itoa(period)]
		if !ok || i >= len(series) {
			v.set(DimMA, StateInsufficient, fmt.Sprintf("MA%d 无输出，无法判断均线排列", period))
			return
		}
		if !(series[i] > 0) {
			v.set(DimMA, StateInsufficient, fmt.Sprintf("MA%d 尚未成形（当日取值非正）", period))
			return
		}
		values[idx] = series[i]
	}

	descending, ascending := true, true
	for idx := 0; idx < len(values)-1; idx++ {
		if values[idx] <= values[idx+1] {
			descending = false
		}
		if values[idx] >= values[idx+1] {
			ascending = false
		}
	}
	// spread 用「最短周期均线 - 最长周期均线」占收盘价的比例度量排列的陡峭程度。
	spread := (values[0] - values[len(values)-1]) / close
	need := def.MAMinSpreadRatio

	switch {
	case descending && spread >= need:
		v.set(DimMA, StateBuy, fmt.Sprintf(
			"%s 依次递减，间距 %.3f%% ≥ %.3f%%，收盘价 %.2f → 多头排列",
			maLabel(def.MAPeriods, values), spread*100, need*100, close))
	case ascending && -spread >= need:
		v.set(DimMA, StateSell, fmt.Sprintf(
			"%s 依次递增，间距 %.3f%% ≥ %.3f%%，收盘价 %.2f → 空头排列",
			maLabel(def.MAPeriods, values), -spread*100, need*100, close))
	case descending || ascending:
		v.set(DimMA, StateNeutral, fmt.Sprintf(
			"%s 顺序已排列但间距 %.3f%% < %.3f%%（收盘价 %.2f）→ 均线纠缠",
			maLabel(def.MAPeriods, values), abs(spread)*100, need*100, close))
	default:
		v.set(DimMA, StateNeutral, fmt.Sprintf(
			"%s 顺序未形成单边排列（收盘价 %.2f）→ 均线纠缠", maLabel(def.MAPeriods, values), close))
	}
}

// maLabel 把当日各周期均线值排成 "MA5 10.30 / MA10 10.28 / ..." 便于肉眼核对。
func maLabel(periods []int, values []float64) string {
	var b strings.Builder
	for idx, period := range periods {
		if idx > 0 {
			b.WriteString(" / ")
		}
		fmt.Fprintf(&b, "MA%d %.2f", period, values[idx])
	}
	return b.String()
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
