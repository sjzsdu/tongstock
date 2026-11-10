package signalstate_test

import (
	"fmt"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/signalstate"
)

// TestComputeSeriesIsDeterministic 验证：固定 code + 相同 K 线重复调用，逐日结果
// 完全一致（内核不允许 time.Now / 网络 / map 迭代顺序影响结果）。
func TestComputeSeriesIsDeterministic(t *testing.T) {
	klines := syntheticKlines(600)
	def := signalstate.DefaultDefinition()

	first := signalstate.ComputeSeries("000001", 9, klines, def)
	for i := range first {
		if first[i].KType != 9 {
			t.Fatalf("bar %d: ktype not stamped: %d", i, first[i].KType)
		}
	}
	for round := 0; round < 3; round++ {
		again := signalstate.ComputeSeries("000001", 9, klines, def)
		if len(again) != len(first) {
			t.Fatalf("round %d: series length changed: %d vs %d", round, len(again), len(first))
		}
		for i := range first {
			sameVector(t, first[i], again[i], fmt.Sprintf("round %d bar %d", round, i))
		}
	}
}

// TestComputeHasNoLookAhead 验证截断对拍：对 klines[0..t] 单次计算，必须与全序列
// 第 t 根的结果逐字段相同 —— 追加 t 之后的数据不得改变 t 的状态。
func TestComputeHasNoLookAhead(t *testing.T) {
	klines := syntheticKlines(420)
	def := signalstate.DefaultDefinition()
	full := signalstate.ComputeSeries("600000", 9, klines, def)

	for tIdx := 0; tIdx < len(klines); tIdx++ {
		prefix := signalstate.Compute("600000", klines[:tIdx+1], def)
		sameVector(t, full[tIdx], prefix, fmt.Sprintf("prefix len %d", tIdx+1))
	}
}

// TestAllDimsExposeAllThreeStates 验证：4 个维度的三态都实际出现过，且「中性」
// 非空 —— 说明中性是显式判定出来的，而不是没算出来的兜底。
func TestAllDimsExposeAllThreeStates(t *testing.T) {
	klines := syntheticKlines(600)
	def := signalstate.DefaultDefinition()
	series := signalstate.ComputeSeries("000001", 9, klines, def)

	for _, v := range series {
		for _, dim := range signalstate.Dims() {
			state := v.State(dim)
			switch state {
			case signalstate.StateNeutral:
				if v.Reasons[dim] == "" {
					t.Fatalf("neutral on %s/%s has no reason：中性必须是显式判定",
						v.Date.Format("2006-01-02"), dim)
				}
				if v.Insufficient[dim] {
					t.Fatalf("neutral on %s/%s marked insufficient", v.Date.Format("2006-01-02"), dim)
				}
			case signalstate.StateInsufficient:
				if !v.Insufficient[dim] {
					t.Fatalf("insufficient on %s/%s missing Insufficient flag", v.Date.Format("2006-01-02"), dim)
				}
			case signalstate.StateBuy, signalstate.StateSell:
				if v.Insufficient[dim] {
					t.Fatalf("dim %s flagged insufficient but state is %s", dim, state)
				}
			default:
				t.Fatalf("dim %s unknown state %q", dim, state)
			}
			if v.Reasons[dim] == "" {
				t.Fatalf("dim %s on %s has empty reason", dim, v.Date.Format("2006-01-02"))
			}
		}
	}

	counts := countStates(series)
	for _, dim := range signalstate.Dims() {
		for _, state := range []signalstate.State{signalstate.StateBuy, signalstate.StateSell, signalstate.StateNeutral} {
			if counts[dim][state] == 0 {
				t.Errorf("dim %s never took state %s (counts=%v)，三态必须都实际出现过", dim, state, counts[dim])
			}
		}
		// 热身区之外不允许出现 insufficient_data：只有门槛之前的 bar 才算不出来。
		for idx, v := range series {
			if v.State(dim) == signalstate.StateInsufficient && idx >= def.MinBarsFor(dim)-1 {
				t.Errorf("dim %s still insufficient at bar %d (need %d)", dim, idx, def.MinBarsFor(dim))
			}
		}
	}
}

// TestInsufficientDataIsNotNeutral 验证：热身不足时四维是独立的
// insufficient_data 态，不会退化成中性。
func TestInsufficientDataIsNotNeutral(t *testing.T) {
	def := signalstate.DefaultDefinition()
	klines := syntheticKlines(600)
	series := signalstate.ComputeSeries("000001", 9, klines, def)

	for _, dim := range signalstate.Dims() {
		for idx := 0; idx < def.MinBarsFor(dim)-1; idx++ {
			v := series[idx]
			if v.State(dim) != signalstate.StateInsufficient {
				t.Fatalf("bar %d dim %s: want insufficient_data got %s", idx, dim, v.State(dim))
			}
			if v.State(dim).IsTriState() {
				t.Fatalf("bar %d dim %s: insufficient_data must not be one of the three states", idx, dim)
			}
			if !v.Insufficient[dim] {
				t.Fatalf("bar %d dim %s: insufficient flag not set", idx, dim)
			}
		}
	}

	empty := signalstate.Compute("000001", nil, def)
	if !empty.HasInsufficient() {
		t.Fatal("empty klines must produce insufficient_data")
	}
	for _, dim := range signalstate.Dims() {
		if empty.State(dim) == signalstate.StateNeutral {
			t.Fatalf("empty klines dim %s rendered as neutral", dim)
		}
	}
}

// TestMinBarsGatePerDimension 验证每个维度的热身门槛：门槛前一根是 insufficient，
// 门槛那一根起必须给出三态之一。
func TestMinBarsGatePerDimension(t *testing.T) {
	def := signalstate.DefaultDefinition()
	klines := syntheticKlines(200)
	series := signalstate.ComputeSeries("000001", 9, klines, def)

	for _, dim := range signalstate.Dims() {
		need := def.MinBarsFor(dim)
		if need <= 1 {
			t.Fatalf("dim %s has no warmup requirement: %d", dim, need)
		}
		if got := series[need-2].State(dim); got != signalstate.StateInsufficient {
			t.Fatalf("dim %s at bar %d (need %d): want insufficient got %s", dim, need-2, need, got)
		}
		if got := series[need-1].State(dim); got == signalstate.StateInsufficient {
			t.Fatalf("dim %s at bar %d (need %d): still insufficient", dim, need-1, need)
		}
	}
}

// TestBollZoneRatioIsWired 验证布林触发带宽可配：带宽比例为 0 时只有真正穿越
// 轨道才算买卖，放大到 0.4 后触发带变宽，买卖态天数应严格增加。
func TestBollZoneRatioIsWired(t *testing.T) {
	klines := syntheticKlines(600)

	narrow := signalstate.DefaultDefinition()
	narrow.BollZoneRatio = 0
	wide := signalstate.DefaultDefinition()
	wide.BollZoneRatio = 0.4

	triggered := func(def signalstate.Definition) int {
		total := 0
		for _, v := range signalstate.ComputeSeries("000001", 9, klines, def) {
			switch v.State(signalstate.DimBoll) {
			case signalstate.StateBuy, signalstate.StateSell:
				total++
			}
		}
		return total
	}

	narrowCount, wideCount := triggered(narrow), triggered(wide)
	if wideCount <= narrowCount {
		t.Fatalf("widening boll zone must trigger more bars: narrow=%d wide=%d", narrowCount, wideCount)
	}
}

// TestDirectionIsFrozenForBoll 验证「突破上轨 = 买、跌破下轨 = 卖」已写死在枚举里。
func TestDirectionIsFrozenForBoll(t *testing.T) {
	buy, ok := signalstate.StateMetaFor(signalstate.DimBoll, signalstate.StateBuy)
	if !ok {
		t.Fatal("boll buy meta missing")
	}
	sell, ok := signalstate.StateMetaFor(signalstate.DimBoll, signalstate.StateSell)
	if !ok {
		t.Fatal("boll sell meta missing")
	}
	if buy.Label != "突破上轨" || buy.Direction != signalstate.DirectionBuy {
		t.Fatalf("boll buy frozen as %q/%q", buy.Label, buy.Direction)
	}
	if sell.Label != "跌破下轨" || sell.Direction != signalstate.DirectionSell {
		t.Fatalf("boll sell frozen as %q/%q", sell.Label, sell.Direction)
	}
}

// TestNormalizedClampsParameters 验证参数兜底：零值定义与越界阈值都回落到合法
// 默认值，避免除零或上下触发带重叠。
func TestNormalizedClampsParameters(t *testing.T) {
	var zero signalstate.Definition
	got := zero.Normalized()
	def := signalstate.DefaultDefinition()
	if got.BollZoneRatio != def.BollZoneRatio || got.CrossWrapRatio != def.CrossWrapRatio {
		t.Fatalf("zero definition not normalized: %+v", got)
	}
	if got.MACD != def.MACD || got.KDJ != def.KDJ || got.BOLL != def.BOLL {
		t.Fatalf("zero definition indicator periods not normalized: %+v", got)
	}

	over := def
	over.BollZoneRatio = 3
	if clamped := over.Normalized().BollZoneRatio; clamped >= 0.5 {
		t.Fatalf("boll zone ratio not clamped: %v", clamped)
	}

	inverted := def
	inverted.OscOversoldJ, inverted.OscOverboughtJ = 120, -20
	if fixed := inverted.Normalized(); fixed.OscOversoldJ >= fixed.OscOverboughtJ {
		t.Fatalf("inverted osc thresholds not fixed: %+v", fixed)
	}
}
