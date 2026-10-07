package factorlab

import (
	"github.com/sjzsdu/tongstock/internal/validation"
)

// BuiltinFactors 是首批可解释横截面因子（先验方向来自 A 股常见实证，
// 但是否显著完全由数据说话）。全部只用现有 K 线可算的量价信息。
func BuiltinFactors() []Factor {
	return []Factor{
		{
			Key:         "reversal_1d",
			Name:        "1日反转",
			Description: "当日收益（较昨收）：A股短周期反转效应（涨多了跌），方向由数据决定",
			Prior:       -1,
			Compute: func(history []validation.BacktestBar) (float64, bool) {
				if len(history) == 0 {
					return 0, false
				}
				last := history[len(history)-1]
				if last.Close <= 0 {
					return 0, false
				}
				if last.PreClose > 0 {
					return last.Close/last.PreClose - 1, true
				}
				if len(history) < 2 || history[len(history)-2].Close <= 0 {
					return 0, false
				}
				return last.Close/history[len(history)-2].Close - 1, true
			},
		},
		{
			Key:         "up_days_ratio_20d",
			Name:        "20日上涨天数占比",
			Description: "近20个交易日上涨天数占比：连续性/情绪宽度，方向由数据决定",
			Prior:       1,
			Compute: func(history []validation.BacktestBar) (float64, bool) {
				if len(history) < 21 {
					return 0, false
				}
				up := 0
				for i := len(history) - 20; i < len(history); i++ {
					if history[i-1].Close > 0 && history[i].Close > history[i-1].Close {
						up++
					}
				}
				return float64(up) / 20, true
			},
		},
		{
			Key:         "price_level",
			Name:        "价格水平",
			Description: "当日收盘价绝对水平：低价股效应（A股低价股短线弹性大），方向由数据决定",
			Prior:       -1,
			Compute: func(history []validation.BacktestBar) (float64, bool) {
				if len(history) == 0 || history[len(history)-1].Close <= 0 {
					return 0, false
				}
				return history[len(history)-1].Close, true
			},
		},
		{
			Key:         "momentum_5d",
			Name:        "5日动量",
			Description: "近5个交易日收益：短周期动量/延续效应",
			Prior:       1,
			Compute: func(history []validation.BacktestBar) (float64, bool) {
				if len(history) < 6 {
					return 0, false
				}
				base := history[len(history)-6].Close
				if base <= 0 {
					return 0, false
				}
				return history[len(history)-1].Close/base - 1, true
			},
		},
		{
			Key:         "momentum_20d",
			Name:        "20日动量",
			Description: "近20个交易日收益：中期动量/延续效应",
			Prior:       1,
			Compute: func(history []validation.BacktestBar) (float64, bool) {
				if len(history) < 21 {
					return 0, false
				}
				base := history[len(history)-21].Close
				if base <= 0 {
					return 0, false
				}
				return history[len(history)-1].Close/base - 1, true
			},
		},
		{
			Key:         "volume_surge",
			Name:        "量比激增",
			Description: "当日成交量相对20日均量的放大倍数：资金关注度",
			Prior:       1,
			Compute: func(history []validation.BacktestBar) (float64, bool) {
				if len(history) < 21 || history[len(history)-1].Volume <= 0 {
					return 0, false
				}
				sum := 0.0
				for _, b := range history[len(history)-21 : len(history)-1] {
					sum += b.Volume
				}
				avg := sum / 20
				if avg <= 0 {
					return 0, false
				}
				return history[len(history)-1].Volume / avg, true
			},
		},
		{
			Key:         "volatility_20d",
			Name:        "20日波动率",
			Description: "近20个交易日日收益标准差：低波动溢价/风险控制",
			Prior:       -1,
			Compute: func(history []validation.BacktestBar) (float64, bool) {
				if len(history) < 21 {
					return 0, false
				}
				prev := history[len(history)-21].Close
				if prev <= 0 {
					return 0, false
				}
				rets := make([]float64, 0, 20)
				for _, b := range history[len(history)-20:] {
					if prev <= 0 {
						return 0, false
					}
					rets = append(rets, b.Close/prev-1)
					prev = b.Close
				}
				return std(rets), true
			},
		},
		{
			Key:         "close_position",
			Name:        "收盘位置",
			Description: "收盘价在当日高低区间的位置：日内买盘强度",
			Prior:       1,
			Compute: func(history []validation.BacktestBar) (float64, bool) {
				if len(history) == 0 {
					return 0, false
				}
				last := history[len(history)-1]
				if last.High-last.Low <= 0 {
					return 0, false
				}
				return (last.Close - last.Low) / (last.High - last.Low), true
			},
		},
		{
			Key:         "turnover_level",
			Name:        "成交额水平",
			Description: "当日成交额：流动性与关注度（大额成交兼顾流动性与机构参与）",
			Prior:       1,
			Compute: func(history []validation.BacktestBar) (float64, bool) {
				if len(history) == 0 || history[len(history)-1].Amount <= 0 {
					return 0, false
				}
				return history[len(history)-1].Amount, true
			},
		},
	}
}
