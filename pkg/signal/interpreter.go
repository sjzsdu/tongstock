package signal

import (
	"fmt"
	"strings"
)

// SignalInterpretation 信号解读结果
type SignalInterpretation struct {
	Summary     string   // 一句话摘要
	Explanation string   // 详细解释
	Suggestions []string  // 操作建议
	RiskLevel   string    // 风险等级：low, medium, high
	Trend       string    // 当前趋势描述
}

// InterpretSignal 解读单个信号
func InterpretSignal(s Signal, trend TrendDirection) SignalInterpretation {
	interpretation := SignalInterpretation{
		RiskLevel: "medium",
	}

	// 设置趋势描述
	switch trend {
	case TrendUptrend:
		interpretation.Trend = "上涨趋势"
	case TrendDowntrend:
		interpretation.Trend = "下跌趋势"
	case TrendSideways:
		interpretation.Trend = "横盘震荡"
	default:
		interpretation.Trend = "趋势不明"
	}

	// 根据信号类型生成解读
	switch s.Type {
	case SignalGoldenCross:
		interpretation = interpretGoldenCross(s, trend)
	case SignalDeathCross:
		interpretation = interpretDeathCross(s, trend)
	case SignalOverbought:
		interpretation = interpretOverbought(s, trend)
	case SignalOversold:
		interpretation = interpretOversold(s, trend)
	case SignalBreakUpper:
		interpretation = interpretBreakUpper(s, trend)
	case SignalBreakLower:
		interpretation = interpretBreakLower(s, trend)
	case SignalBullAlign:
		interpretation = interpretBullAlign(s, trend)
	case SignalBearAlign:
		interpretation = interpretBearAlign(s, trend)
	}

	return interpretation
}

// InterpretAllSignals 解读所有信号，生成综合摘要
func InterpretAllSignals(signals []Signal, trend TrendDirection) string {
	if len(signals) == 0 {
		return "当前无明显技术信号"
	}

	// 统计信号类型
	buySignals := 0
	sellSignals := 0
	neutralSignals := 0

	for _, s := range signals {
		switch s.Type {
		case SignalGoldenCross, SignalOversold, SignalBreakUpper, SignalBullAlign:
			buySignals++
		case SignalDeathCross, SignalOverbought, SignalBreakLower, SignalBearAlign:
			sellSignals++
		default:
			neutralSignals++
		}
	}

	// 生成综合摘要
	var summaryParts []string

	if buySignals > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("发现 %d 个买入信号", buySignals))
	}
	if sellSignals > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("发现 %d 个卖出信号", sellSignals))
	}

	// 添加最强信号
	if len(signals) > 0 {
	 strongest := signals[0]
		for _, s := range signals {
			if s.Strength > strongest.Strength {
				strongest = s
			}
		}
		summaryParts = append(summaryParts, fmt.Sprintf("最强信号：%s %s", strongest.Indicator, strongest.Type))
	}

	return strings.Join(summaryParts, "，")
}

func interpretGoldenCross(s Signal, trend TrendDirection) SignalInterpretation {
	i := SignalInterpretation{
		RiskLevel: "medium",
		Trend:     TrendToString(trend),
	}

	switch s.Indicator {
	case "MACD":
		i.Summary = "MACD 金叉（DIF 上穿 DEA），通常视为趋势转强信号"
		i.Explanation = "MACD 金叉出现在零轴上方时，表示多头力量增强，趋势可能加速上涨；出现在零轴下方时，表示空头力量减弱，可能形成反弹。"
		if trend == TrendUptrend {
			i.Suggestions = []string{"关注成交量是否放大", "可考虑逐步建仓", "注意止损位设置"}
			i.RiskLevel = "low"
		} else {
			i.Suggestions = []string{"谨慎观望，等待趋势确认", "注意是否为假金叉", "关注后续走势"}
			i.RiskLevel = "medium"
		}
	case "KDJ":
		i.Summary = "KDJ 金叉（K 线上穿 D 线），短线转强信号"
		i.Explanation = "KDJ 金叉在低位（20以下）出现时，超卖反弹信号较强；在高位（80以上）出现时，可能为假信号。"
		if s.Strength > 0.7 {
			i.Suggestions = []string{"短线可考虑介入", "快进快出为主", "注意高位风险"}
			i.RiskLevel = "medium"
		} else {
			i.Suggestions = []string{"信号强度较弱，谨慎操作", "结合其他指标确认"}
			i.RiskLevel = "high"
		}
	case "MA":
		i.Summary = "均线金叉，趋势转折信号"
		i.Explanation = "短期均线向上穿越长期均线，表示短期趋势转强。5日线上穿10日线为短线信号，5日线上穿20日线为中线信号。"
		i.Suggestions = []string{"关注均线多头排列形成", "等待趋势确认", "设置止损位"}
	default:
		i.Summary = fmt.Sprintf("%s 金叉，趋势转强信号", s.Indicator)
		i.Explanation = s.Details
		i.Suggestions = []string{"结合其他指标综合判断"}
	}

	return i
}

func interpretDeathCross(s Signal, trend TrendDirection) SignalInterpretation {
	i := SignalInterpretation{
		RiskLevel: "medium",
		Trend:     TrendToString(trend),
	}

	switch s.Indicator {
	case "MACD":
		i.Summary = "MACD 死叉（DIF 下穿 DEA），趋势转弱信号"
		i.Explanation = "MACD 死叉出现在零轴上方时，表示多头力量减弱，可能形成回调；出现在零轴下方时，表示空头力量增强，趋势可能加速下跌。"
		if trend == TrendDowntrend {
			i.Suggestions = []string{"注意风险控制", "考虑减仓或止损", "等待企稳信号"}
			i.RiskLevel = "high"
		} else {
			i.Suggestions = []string{"关注是否为技术性回调", "观察成交量变化", "等待趋势确认"}
			i.RiskLevel = "medium"
		}
	case "KDJ":
		i.Summary = "KDJ 死叉（K 线下穿 D 线），短线转弱信号"
		i.Explanation = "KDJ 死叉在高位（80以上）出现时，超买回调信号较强；在低位（20以下）出现时，可能为假信号。"
		i.Suggestions = []string{"短线注意风险", "高位死叉需警惕", "关注后续走势"}
		i.RiskLevel = "medium"
	case "MA":
		i.Summary = "均线死叉，趋势转折信号"
		i.Explanation = "短期均线向下穿越长期均线，表示短期趋势转弱。5日线下穿10日线为短线信号，5日线下穿20日线为中线信号。"
		i.Suggestions = []string{"注意均线空头排列形成", "考虑风险控制", "等待企稳信号"}
	default:
		i.Summary = fmt.Sprintf("%s 死叉，趋势转弱信号", s.Indicator)
		i.Explanation = s.Details
		i.Suggestions = []string{"结合其他指标综合判断"}
	}

	return i
}

func interpretOverbought(s Signal, trend TrendDirection) SignalInterpretation {
	i := SignalInterpretation{
		RiskLevel: "high",
		Trend:     TrendToString(trend),
	}

	switch s.Indicator {
	case "KDJ":
		i.Summary = "KDJ 超买（K/D/J 值超过80），短线风险增加"
		i.Explanation = "KDJ 超买区域表示短期上涨过快，存在回调风险。但强势股可能持续超买状态，需结合趋势判断。"
		if trend == TrendUptrend {
			i.Suggestions = []string{"强势股可能持续超买", "关注成交量变化", "注意回调风险"}
			i.RiskLevel = "medium"
		} else {
			i.Suggestions = []string{"超买后回调概率大", "谨慎追高", "等待回调企稳"}
			i.RiskLevel = "high"
		}
	case "RSI":
		i.Summary = "RSI 超买（RSI 值超过70），短期风险增加"
		i.Explanation = "RSI 超买表示短期上涨动能过强，可能面临回调。RSI 超过80时风险更高。"
		i.Suggestions = []string{"注意回调风险", "观察RSI是否回落", "结合成交量判断"}
	default:
		i.Summary = fmt.Sprintf("%s 超买，短期风险增加", s.Indicator)
		i.Explanation = s.Details
		i.Suggestions = []string{"结合其他指标综合判断"}
	}

	return i
}

func interpretOversold(s Signal, trend TrendDirection) SignalInterpretation {
	i := SignalInterpretation{
		RiskLevel: "medium",
		Trend:     TrendToString(trend),
	}

	switch s.Indicator {
	case "KDJ":
		i.Summary = "KDJ 超卖（K/D/J 值低于20），短线反弹机会"
		i.Explanation = "KDJ 超卖区域表示短期下跌过度，存在反弹机会。但弱势股可能持续超卖状态，需结合趋势判断。"
		if trend == TrendDowntrend {
			i.Suggestions = []string{"弱势股可能持续超卖", "谨慎抄底", "等待企稳信号"}
			i.RiskLevel = "high"
		} else {
			i.Suggestions = []string{"超卖后反弹概率大", "可考虑轻仓试探", "注意止损位"}
			i.RiskLevel = "medium"
		}
	case "RSI":
		i.Summary = "RSI 超卖（RSI 值低于30），短期反弹机会"
		i.Explanation = "RSI 超卖表示短期下跌动能过强，可能形成反弹。RSI 低于20时反弹概率更高。"
		i.Suggestions = []string{"关注反弹机会", "观察RSI是否回升", "结合成交量判断"}
	default:
		i.Summary = fmt.Sprintf("%s 超卖，短期反弹机会", s.Indicator)
		i.Explanation = s.Details
		i.Suggestions = []string{"结合其他指标综合判断"}
	}

	return i
}

func interpretBreakUpper(s Signal, trend TrendDirection) SignalInterpretation {
	i := SignalInterpretation{
		RiskLevel: "medium",
		Trend:     TrendToString(trend),
	}

	i.Summary = "突破布林上轨，强势上涨信号"
	i.Explanation = "股价突破布林带上轨，表示短期上涨动能强劲。突破后可能继续上涨，也可能回落至上轨附近。"
	if trend == TrendUptrend {
		i.Suggestions = []string{"强势突破，可能持续上涨", "关注成交量配合", "注意回调风险"}
		i.RiskLevel = "low"
	} else {
		i.Suggestions = []string{"突破可能为假突破", "观察是否有效站稳", "谨慎追高"}
		i.RiskLevel = "medium"
	}

	return i
}

func interpretBreakLower(s Signal, trend TrendDirection) SignalInterpretation {
	i := SignalInterpretation{
		RiskLevel: "high",
		Trend:     TrendToString(trend),
	}

	i.Summary = "跌破布林下轨，弱势下跌信号"
	i.Explanation = "股价跌破布林带下轨，表示短期下跌动能强劲。跌破后可能继续下跌，也可能反弹至下轨附近。"
	if trend == TrendDowntrend {
		i.Suggestions = []string{"弱势跌破，可能持续下跌", "注意风险控制", "等待企稳信号"}
		i.RiskLevel = "high"
	} else {
		i.Suggestions = []string{"跌破可能为假跌破", "观察是否有效跌破", "关注反弹机会"}
		i.RiskLevel = "medium"
	}

	return i
}

func interpretBullAlign(s Signal, trend TrendDirection) SignalInterpretation {
	i := SignalInterpretation{
		RiskLevel: "low",
		Trend:     TrendToString(trend),
	}

	i.Summary = "均线多头排列，趋势向好"
	i.Explanation = "短期均线在中期均线之上，中期均线在长期均线之上，形成多头排列，表示趋势向好。"
	if trend == TrendUptrend {
		i.Suggestions = []string{"趋势明确向好", "可考虑持股待涨", "关注均线支撑"}
		i.RiskLevel = "low"
	} else {
		i.Suggestions = []string{"多头排列初形成", "等待趋势确认", "注意回调风险"}
		i.RiskLevel = "medium"
	}

	return i
}

func interpretBearAlign(s Signal, trend TrendDirection) SignalInterpretation {
	i := SignalInterpretation{
		RiskLevel: "high",
		Trend:     TrendToString(trend),
	}

	i.Summary = "均线空头排列，趋势偏弱"
	i.Explanation = "短期均线在中期均线之下，中期均线在长期均线之下，形成空头排列，表示趋势偏弱。"
	if trend == TrendDowntrend {
		i.Suggestions = []string{"趋势明确偏弱", "注意风险控制", "等待企稳信号"}
		i.RiskLevel = "high"
	} else {
		i.Suggestions = []string{"空头排列初形成", "等待趋势确认", "关注均线压力"}
		i.RiskLevel = "medium"
	}

	return i
}

// TrendToString 将趋势转换为字符串描述
func TrendToString(trend TrendDirection) string {
	switch trend {
	case TrendUptrend:
		return "上涨趋势"
	case TrendDowntrend:
		return "下跌趋势"
	case TrendSideways:
		return "横盘震荡"
	default:
		return "趋势不明"
	}
}