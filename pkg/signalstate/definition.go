package signalstate

import (
	"sort"

	"github.com/sjzsdu/tongstock/pkg/ta"
)

// CalcVersion 是内核计算版本。阈值/算法口径改动时必须递增，让落库或缓存的
// 旧结果能被识别出来（本期不落库，但口径版本先留好）。
const CalcVersion = "state-v1"

// AdjustRaw 是本期唯一的复权口径：未复权原始价（raw）。
//
// 选择 raw 的理由：状态刻画的是「当时看到的价格与指标」，前复权会随着新的
// 除权事件不断改写历史价，同一根历史 bar 的状态会随时间漂移，回溯结果不可
// 复现。每条状态带都带上口径版本，后续若引入复权口径才能和 raw 区分开。
const AdjustRaw = "raw"

// Definition 是一个维度集合的完整参数：指标周期 + 三态判定阈值。
//
// 所有阈值都有默认值（见 DefaultDefinition），且都能在 configs/params.yaml 的
// state 段覆盖（见 pkg/param.ResolveStateDefinition）。集中放一份，后续各期
// （全量回溯、筛选）共用同一份定义，避免口径分叉。
type Definition struct {
	// MACD 交叉维度的指标周期。
	MACD ta.MACDConfig `json:"macd"`
	// KDJ 超买超卖维度的指标周期。
	KDJ ta.KDJConfig `json:"kdj"`
	// BOLL 布林位置维度的指标周期。
	BOLL ta.BOLLConfig `json:"boll"`
	// MAPeriods 均线排列维度使用的周期，按周期升序（如 [5,10,20,60]）。
	MAPeriods []int `json:"ma_periods"`

	// CrossWrapRatio 定义「缠绕」带：|DIF-DEA| / 收盘价 ≤ 该比例时判定为缠绕（中性）。
	//
	// 用占收盘价的比例而不是绝对差值，是因为 MACD 差值的量纲跟股价同单位：
	// 10 元的票差 0.02 与 100 元的票差 0.2 是同一个强弱。默认 0.002（0.2%）。
	CrossWrapRatio float64 `json:"cross_wrap_ratio"`

	// OscOversoldJ 是超卖阈值：J ≤ 该值判定为超卖（买入态）。默认 0。
	OscOversoldJ float64 `json:"osc_oversold_j"`
	// OscOverboughtJ 是超买阈值：J ≥ 该值判定为超买（卖出态）。默认 100。
	OscOverboughtJ float64 `json:"osc_overbought_j"`

	// BollZoneRatio 是布林触发带的宽度占布林带宽（上轨-下轨）的比例，
	// 默认 0.15：以 2σ 带宽计，触发带约 0.6σ，即「突破点附近」的区域。
	// 可调范围被限制在 (0, 0.5)，否则上下触发带会重叠。
	BollZoneRatio float64 `json:"boll_zone_ratio"`

	// MAMinSpreadRatio 是均线排列的最小间距（最短均线减最长均线，占收盘价比例）。
	// 小于该值时即便顺序正确也算「纠缠」（中性）：默认 0.001（0.1%）。
	MAMinSpreadRatio float64 `json:"ma_min_spread_ratio"`
}

// 默认阈值的集中定义（单一事实源）：params.yaml 里的 state 段只是覆盖入口，
// 没配到的字段一律回落到这里。
const (
	DefaultCrossWrapRatio   = 0.002
	DefaultOscOversoldJ     = 0.0
	DefaultOscOverboughtJ   = 100.0
	DefaultBollZoneRatio    = 0.15
	DefaultMAMinSpreadRatio = 0.001
	// maxBollZoneRatio 保证上下触发带不重叠（各占带宽比例 < 50%）。
	maxBollZoneRatio = 0.499
)

// defaultMAPeriods 是均线排列维度的默认周期。
var defaultMAPeriods = []int{5, 10, 20, 60}

// DefaultDefinition 返回带全部默认值的定义。
func DefaultDefinition() Definition {
	return Definition{
		MACD:             ta.MACDConfig{Fast: 12, Slow: 26, Signal: 9},
		KDJ:              ta.KDJConfig{N: 9, M1: 3, M2: 3},
		BOLL:             ta.BOLLConfig{N: 20, K: 2.0},
		MAPeriods:        append([]int(nil), defaultMAPeriods...),
		CrossWrapRatio:   DefaultCrossWrapRatio,
		OscOversoldJ:     DefaultOscOversoldJ,
		OscOverboughtJ:   DefaultOscOverboughtJ,
		BollZoneRatio:    DefaultBollZoneRatio,
		MAMinSpreadRatio: DefaultMAMinSpreadRatio,
	}
}

// WithIndicatorConfig 用统一的指标参数（param.Resolve 的结果）覆盖 MACD/KDJ/BOLL
// 周期与均线周期，其余阈值保持不变。指标周期只有一份来源，避免状态模型与图表
// 指标各用一套参数。
func (d Definition) WithIndicatorConfig(cfg *ta.IndicatorConfig) Definition {
	if cfg == nil {
		return d.Normalized()
	}
	if cfg.MACD != nil && cfg.MACD.Fast > 0 && cfg.MACD.Slow > 0 && cfg.MACD.Signal > 0 {
		d.MACD = *cfg.MACD
	}
	if cfg.KDJ != nil && cfg.KDJ.N > 0 {
		d.KDJ = *cfg.KDJ
	}
	if cfg.BOLL != nil && cfg.BOLL.N > 0 && cfg.BOLL.K > 0 {
		d.BOLL = *cfg.BOLL
	}
	if len(cfg.MA) > 0 {
		d.MAPeriods = append([]int(nil), cfg.MA...)
	}
	return d.Normalized()
}

// Normalized 把非法/缺省字段回落到默认值，并保证参数内部自洽。内核入口会先调用
// 它，所以调用方传零值 Definition 也能得到确定结果，而不是除零或空排列。
func (d Definition) Normalized() Definition {
	def := DefaultDefinition()

	if d.MACD.Fast <= 0 || d.MACD.Slow <= 0 || d.MACD.Signal <= 0 || d.MACD.Fast >= d.MACD.Slow {
		d.MACD = def.MACD
	}
	if d.KDJ.N <= 0 {
		d.KDJ = def.KDJ
	}
	if d.KDJ.M1 <= 0 {
		d.KDJ.M1 = 3
	}
	if d.KDJ.M2 <= 0 {
		d.KDJ.M2 = 3
	}
	if d.BOLL.N <= 0 {
		d.BOLL.N = def.BOLL.N
	}
	if d.BOLL.K <= 0 {
		d.BOLL.K = def.BOLL.K
	}

	d.MAPeriods = normalizeMAPeriods(d.MAPeriods)

	if !(d.CrossWrapRatio > 0) {
		d.CrossWrapRatio = def.CrossWrapRatio
	}
	if d.OscOverboughtJ <= 0 {
		d.OscOverboughtJ = def.OscOverboughtJ
	}
	if d.OscOversoldJ >= d.OscOverboughtJ {
		d.OscOversoldJ = def.OscOversoldJ
		d.OscOverboughtJ = def.OscOverboughtJ
	}
	if !(d.BollZoneRatio > 0) {
		d.BollZoneRatio = def.BollZoneRatio
	}
	if d.BollZoneRatio > maxBollZoneRatio {
		d.BollZoneRatio = maxBollZoneRatio
	}
	if d.MAMinSpreadRatio < 0 {
		d.MAMinSpreadRatio = def.MAMinSpreadRatio
	}
	return d
}

// normalizeMAPeriods 去重、升序，并要求至少 2 个周期（1 个周期谈不上「排列」）。
func normalizeMAPeriods(periods []int) []int {
	seen := make(map[int]bool, len(periods))
	out := make([]int, 0, len(periods))
	for _, p := range periods {
		if p <= 0 || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Ints(out)
	if len(out) < 2 {
		return append([]int(nil), defaultMAPeriods...)
	}
	return out
}

// MinBarsFor 返回某维度需要多少根 bar 才能判定：不足即 insufficient_data
// （而不是中性）。接口把它作为预热提示返回给前端。
func (d Definition) MinBarsFor(dim Dim) int {
	return d.Normalized().minBars(dim)
}

// minBars 返回某维度从第几根 bar 起才可判定：返回需要的 bar 数量，不足即
// insufficient_data（而不是中性）。
func (d Definition) minBars(dim Dim) int {
	switch dim {
	case DimCross:
		// DIF 从 slow-1 起有效，DEA 需要再从有效 DIF 里取 signal 个做 SMA 初始化，
		// 因此 DEA 的首个有效下标是 slow+signal-2。这里按 slow+signal 要求，
		// 与 ta.CalcMACD 的守卫（len < slow+signal 时直接返回空结果）一致：
		// 否则会出现「内核声称可判定、指标其实没算」的日子（截断对拍测试会抓到）。
		return d.MACD.Slow + d.MACD.Signal
	case DimOsc:
		return d.KDJ.N
	case DimBoll:
		return d.BOLL.N
	case DimMA:
		longest := 0
		for _, p := range d.MAPeriods {
			if p > longest {
				longest = p
			}
		}
		return longest
	default:
		return 0
	}
}
