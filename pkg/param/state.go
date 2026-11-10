package param

import (
	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"github.com/sjzsdu/tongstock/pkg/ta"
)

// StateThresholds 是「三态状态模型」的阈值配置，对应 params.yaml 的 state 段。
//
// 指标周期（MACD/KDJ/BOLL 周期、均线周期）不在这里重复配置：它们来自同一份
// defaults / categories / overrides，由 Resolve 解析，状态模型直接用同一套参数，
// 避免「图上指标的 MACD」和「状态判定用的 MACD」算的不是一回事。
//
// 字段取值约定：0 或缺省 = 用 signalstate 的默认值。默认值集中在
// signalstate.DefaultDefinition，本结构只做覆盖入口。
type StateThresholds struct {
	Cross *CrossThresholds `yaml:"cross,omitempty" json:"cross,omitempty"`
	Osc   *OscThresholds   `yaml:"osc,omitempty" json:"osc,omitempty"`
	Boll  *BollThresholds  `yaml:"boll,omitempty" json:"boll,omitempty"`
	MA    *MAThresholds    `yaml:"ma,omitempty" json:"ma,omitempty"`
}

// CrossThresholds 是交叉态（MACD 缠绕）的阈值。
type CrossThresholds struct {
	// WrapRatio = |DIF-DEA| / 收盘价 的缠绕带半宽，默认 0.002（0.2%）。
	WrapRatio float64 `yaml:"wrap_ratio,omitempty" json:"wrap_ratio,omitempty"`
}

// OscThresholds 是超买超卖态（KDJ J 值）的阈值。
type OscThresholds struct {
	// OversoldJ：J ≤ 该值为超卖（买入态），默认 0。
	OversoldJ float64 `yaml:"oversold_j,omitempty" json:"oversold_j,omitempty"`
	// OverboughtJ：J ≥ 该值为超买（卖出态），默认 100。
	OverboughtJ float64 `yaml:"overbought_j,omitempty" json:"overbought_j,omitempty"`
}

// BollThresholds 是布林位置态的阈值。
type BollThresholds struct {
	// ZoneRatio：以上轨/下轨为中心的触发带宽度占布林带宽（上轨-下轨）的比例，
	// 默认 0.15。用户原话「突破的那个点的附近区域算交易机会」，调大 = 附近区域更宽。
	ZoneRatio float64 `yaml:"zone_ratio,omitempty" json:"zone_ratio,omitempty"`
}

// MAThresholds 是均线排列态的阈值。
type MAThresholds struct {
	// MinSpreadRatio：排列的最小间距（最短均线-最长均线占收盘价比例），
	// 默认 0.001（0.1%）。小于该值即便顺序正确也算纠缠（中性）。
	MinSpreadRatio float64 `yaml:"min_spread_ratio,omitempty" json:"min_spread_ratio,omitempty"`
}

// DefaultStateThresholds 返回与 signalstate 默认值一致的阈值（用于生成默认配置文件）。
func DefaultStateThresholds() *StateThresholds {
	def := signalstate.DefaultDefinition()
	return &StateThresholds{
		Cross: &CrossThresholds{WrapRatio: def.CrossWrapRatio},
		Osc:   &OscThresholds{OversoldJ: def.OscOversoldJ, OverboughtJ: def.OscOverboughtJ},
		Boll:  &BollThresholds{ZoneRatio: def.BollZoneRatio},
		MA:    &MAThresholds{MinSpreadRatio: def.MAMinSpreadRatio},
	}
}

// Normalized 返回一份合法化的阈值副本：非法/缺省字段回落到默认值，越界值被夹紧
// （例如 zone_ratio 超过 0.5 会让上下触发带重叠）。设置接口保存配置时走它，
// 避免把 0 或负数写进 params.yaml 之后状态计算变成常数或除零。
func (p *StateThresholds) Normalized() *StateThresholds {
	def := p.applyTo(signalstate.DefaultDefinition()).Normalized()
	return &StateThresholds{
		Cross: &CrossThresholds{WrapRatio: def.CrossWrapRatio},
		Osc:   &OscThresholds{OversoldJ: def.OscOversoldJ, OverboughtJ: def.OscOverboughtJ},
		Boll:  &BollThresholds{ZoneRatio: def.BollZoneRatio},
		MA:    &MAThresholds{MinSpreadRatio: def.MAMinSpreadRatio},
	}
}

// ResolveStateDefinition 解析某只股票的三态定义：指标周期取自 Resolve（三层覆盖），
// 阈值取 state 段覆盖（未配置的字段回落到默认值），最后统一归一化。
//
// 这是「状态定义」的唯一入口：接口、CLI、后续各期回溯都用它，保证口径一致。
func ResolveStateDefinition(code string) signalstate.Definition {
	return stateDefinitionFrom(resolveStateConfig(), Resolve(code, DetectCategory(code)))
}

func resolveStateConfig() *StateThresholds {
	if globalConfig == nil {
		LoadDefault()
	}
	if globalConfig == nil {
		return nil
	}
	return globalConfig.State
}

func stateDefinitionFrom(thresholds *StateThresholds, indicators *ta.IndicatorConfig) signalstate.Definition {
	def := signalstate.DefaultDefinition().WithIndicatorConfig(indicators)
	if thresholds != nil {
		def = thresholds.applyTo(def)
	}
	return def.Normalized()
}

// applyTo 把配置里的阈值覆盖到定义上（0 值视为未配置，保留默认值）。
func (p *StateThresholds) applyTo(def signalstate.Definition) signalstate.Definition {
	if p == nil {
		return def
	}
	if p.Cross != nil && p.Cross.WrapRatio > 0 {
		def.CrossWrapRatio = p.Cross.WrapRatio
	}
	if p.Osc != nil {
		if p.Osc.OversoldJ != 0 {
			def.OscOversoldJ = p.Osc.OversoldJ
		}
		if p.Osc.OverboughtJ > 0 {
			def.OscOverboughtJ = p.Osc.OverboughtJ
		}
	}
	if p.Boll != nil && p.Boll.ZoneRatio > 0 {
		def.BollZoneRatio = p.Boll.ZoneRatio
	}
	if p.MA != nil && p.MA.MinSpreadRatio > 0 {
		def.MAMinSpreadRatio = p.MA.MinSpreadRatio
	}
	return def
}
