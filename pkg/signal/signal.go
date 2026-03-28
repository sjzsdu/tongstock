package signal

import (
	"math"
	"time"

	"github.com/sjzsdu/tongstock/pkg/ta"
)

// TrendDirection 表示趋势方向
type TrendDirection int

const (
	TrendUnknown TrendDirection = iota
	TrendUptrend    // 上涨趋势
	TrendDowntrend  // 下跌趋势
	TrendSideways   // 横盘震荡
)

// detectTrend 判断当前的趋势方向
// 基于均线排列和价格位置来判断
func detectTrend(klines []ta.KlineInput, ma map[string][]float64) TrendDirection {
	if len(klines) < 20 || ma["20"] == nil {
		return TrendUnknown
	}

	// 取最近几天的数据进行判断
	n := min(5, len(klines))
	startIdx := len(klines) - n

	// 判断均线排列
	ma5 := ma["5"]
	ma10 := ma["10"]
	ma20 := ma["20"]

	if ma5 == nil || ma10 == nil || ma20 == nil {
		return TrendUnknown
	}

	// 统计最近N天多头排列的天数
	bullDays := 0
	bearDays := 0
	for i := startIdx; i < len(klines); i++ {
		if ma5[i] > ma10[i] && ma10[i] > ma20[i] && ma20[i] > 0 {
			bullDays++
		}
		if ma5[i] < ma10[i] && ma10[i] < ma20[i] && ma20[i] > 0 {
			bearDays++
		}
	}

	// 如果多数天数呈现多头排列，判断为上涨趋势
	if bullDays >= n-1 {
		return TrendUptrend
	}
	// 如果多数天数呈现空头排列，判断为下跌趋势
	if bearDays >= n-1 {
		return TrendDowntrend
	}

	// 判断价格与均线的位置关系
	lastIdx := len(klines) - 1
	price := klines[lastIdx].Close
	ma20Val := ma20[lastIdx]

	if ma20Val > 0 {
		// 价格在20日均线上方，且均线向上
		if price > ma20Val && ma20[lastIdx] > ma20[max(0, lastIdx-5)] {
			return TrendUptrend
		}
		// 价格在20日均线下方，且均线向下
		if price < ma20Val && ma20[lastIdx] < ma20[max(0, lastIdx-5)] {
			return TrendDowntrend
		}
	}

	return TrendSideways
}

// shouldGenerateSignal 判断在给定趋势下是否应该生成信号
// uptrend: 只生成金叉(买入)
// downtrend: 只生成死叉(卖出)
// sideways: 不生成交叉信号
func shouldGenerateSignal(signalType SignalType, trend TrendDirection) bool {
	// 超买超卖和突破信号不受趋势限制
	switch signalType {
	case SignalOverbought, SignalOversold, SignalBreakUpper, SignalBreakLower:
		return true
	case SignalGoldenCross:
		return trend == TrendUptrend
	case SignalDeathCross:
		return trend == TrendDowntrend
	case SignalBullAlign, SignalBearAlign:
		return true
	}
	return false
}

// filterSignalsByTrend 根据趋势过滤信号
func filterSignalsByTrend(signals []Signal, trend TrendDirection) []Signal {
	if trend == TrendUnknown || trend == TrendSideways {
		// 震荡行情中，只保留超买超卖和突破信号，过滤掉金叉死叉
		var filtered []Signal
		for _, s := range signals {
			if shouldGenerateSignal(s.Type, trend) {
				filtered = append(filtered, s)
			}
		}
		return filtered
	}
	return signals
}

type SignalType string

const (
	SignalGoldenCross SignalType = "金叉"
	SignalDeathCross  SignalType = "死叉"
	SignalOverbought  SignalType = "超买"
	SignalOversold    SignalType = "超卖"
	SignalBreakUpper  SignalType = "突破上轨"
	SignalBreakLower  SignalType = "跌破下轨"
	SignalBullAlign   SignalType = "多头排列"
	SignalBearAlign   SignalType = "空头排列"
)

type Signal struct {
	Code      string
	Date      time.Time
	Type      SignalType
	Indicator string
	Details   string
	Strength  float64
}

type DetectOptions struct {
	EnableMACD bool
	EnableKDJ  bool
	EnableBOLL bool
	EnableMA   bool
	EnableRSI  bool
}

func DefaultDetectOptions() *DetectOptions {
	return &DetectOptions{
		EnableMACD: true,
		EnableKDJ:  true,
		EnableBOLL: true,
		EnableMA:   true,
		EnableRSI:  true,
	}
}

var _ = math.MaxInt32
