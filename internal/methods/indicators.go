package methods

import (
	"math"
	"strings"
)

// ComputeBuiltinIndicator 在升序 bars 上计算内建指标，最后一根视为执行日。
//
// 这是执行器（在线判定）与离线特征物化（选股链路）共用的唯一实现，
// 保证「研究时算出的值」与「交易时算出的值」完全一致。
// 返回 false 表示数据不足或指标名不受支持——调用方必须 fail closed，
// 禁止用零值或默认值冒充真实计算。
func ComputeBuiltinIndicator(name string, bars []Bar) (float64, bool) {
	if len(bars) == 0 {
		return 0, false
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	last := bars[len(bars)-1]
	switch lower {
	case "open":
		return last.Open, true
	case "high":
		return last.High, true
	case "low":
		return last.Low, true
	case "close":
		return last.Close, true
	case "volume":
		return last.Volume, true
	case "amount":
		return last.Amount, true
	}
	switch {
	case strings.HasPrefix(lower, "volma"):
		if n, err := parseIntSuffix(lower, len("volma")); err == nil && n > 0 {
			return smaVolume(bars, n)
		}
	case strings.HasPrefix(lower, "volatility"):
		if n, err := parseIntSuffix(lower, len("volatility")); err == nil && n > 1 {
			return rollingVolatility(bars, n)
		}
	case strings.HasPrefix(lower, "prevhigh"):
		if n, err := parseIntSuffix(lower, len("prevhigh")); err == nil && n > 0 {
			return previousCloseExtreme(bars, n, true)
		}
	case strings.HasPrefix(lower, "prevlow"):
		if n, err := parseIntSuffix(lower, len("prevlow")); err == nil && n > 0 {
			return previousCloseExtreme(bars, n, false)
		}
	case strings.HasPrefix(lower, "rsi"):
		if n, err := parseIntSuffix(lower, len("rsi")); err == nil && n > 0 {
			return rsiValue(bars, n)
		}
	case strings.HasPrefix(lower, "ema"):
		if n, err := parseIntSuffix(lower, len("ema")); err == nil && n > 0 {
			return emaValue(bars, n)
		}
	case strings.HasPrefix(lower, "ma"):
		if n, err := parseIntSuffix(lower, len("ma")); err == nil && n > 0 {
			return smaClose(bars, n)
		}
	}
	switch lower {
	case "macd_dif", "dif":
		dif, _, ok := macdValues(bars)
		return dif, ok
	case "macd_dea", "dea":
		_, dea, ok := macdValues(bars)
		return dea, ok
	case "macd_hist", "macd":
		dif, dea, ok := macdValues(bars)
		if !ok {
			return 0, false
		}
		return 2 * (dif - dea), true
	case "return1":
		if len(bars) >= 2 {
			previous := bars[len(bars)-2].Close
			if previous > 0 {
				return (last.Close - previous) / previous, true
			}
		}
	case "gap_pct":
		if len(bars) >= 2 {
			previous := bars[len(bars)-2].Close
			if previous > 0 {
				return (last.Open - previous) / previous, true
			}
		}
	}
	return 0, false
}

// MACD 标准参数 (12, 26, 9)。
const (
	macdFast   = 12
	macdSlow   = 26
	macdSignal = 9
)

// macdValues 返回 (DIF, DEA)。需要至少 macdSlow + macdSignal 根 K 线才能稳定。
func macdValues(bars []Bar) (float64, float64, bool) {
	if len(bars) < macdSlow+macdSignal {
		return 0, 0, false
	}
	closes := make([]float64, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
	}
	fast := emaSeries(closes, macdFast)
	slow := emaSeries(closes, macdSlow)
	difSeries := make([]float64, len(closes))
	for i := range closes {
		difSeries[i] = fast[i] - slow[i]
	}
	deaSeries := emaSeries(difSeries, macdSignal)
	return difSeries[len(closes)-1], deaSeries[len(closes)-1], true
}

// emaSeries 返回与输入等长的 EMA 序列（前值用简单均值播种）。
func emaSeries(values []float64, n int) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 || n <= 0 {
		return out
	}
	k := 2.0 / float64(n+1)
	seed := values[0]
	out[0] = seed
	for i := 1; i < len(values); i++ {
		out[i] = values[i]*k + out[i-1]*(1-k)
	}
	return out
}

func emaValue(bars []Bar, n int) (float64, bool) {
	if len(bars) < n {
		return 0, false
	}
	closes := make([]float64, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
	}
	series := emaSeries(closes, n)
	return series[len(series)-1], true
}

func smaClose(bars []Bar, n int) (float64, bool) {
	if n <= 0 || len(bars) < n {
		return 0, false
	}
	sum := 0.0
	for i := len(bars) - n; i < len(bars); i++ {
		sum += bars[i].Close
	}
	return sum / float64(n), true
}

func smaVolume(bars []Bar, n int) (float64, bool) {
	if n <= 0 || len(bars) < n {
		return 0, false
	}
	var sum float64
	for i := len(bars) - n; i < len(bars); i++ {
		sum += bars[i].Volume
	}
	return sum / float64(n), true
}

// previousCloseExtreme 返回当前 bar 之前 n 根的收盘极值（不含当日，避免前视）。
func previousCloseExtreme(bars []Bar, n int, maximum bool) (float64, bool) {
	if n <= 0 || len(bars) < n+1 {
		return 0, false
	}
	start, end := len(bars)-n-1, len(bars)-1
	value := bars[start].Close
	for i := start + 1; i < end; i++ {
		if maximum && bars[i].Close > value {
			value = bars[i].Close
		}
		if !maximum && bars[i].Close < value {
			value = bars[i].Close
		}
	}
	return value, true
}

func rollingVolatility(bars []Bar, n int) (float64, bool) {
	if n <= 1 || len(bars) < n+1 {
		return 0, false
	}
	returns := make([]float64, 0, n)
	for i := len(bars) - n; i < len(bars); i++ {
		previous := bars[i-1].Close
		if previous <= 0 {
			return 0, false
		}
		returns = append(returns, (bars[i].Close-previous)/previous)
	}
	var mean float64
	for _, value := range returns {
		mean += value
	}
	mean /= float64(len(returns))
	var variance float64
	for _, value := range returns {
		delta := value - mean
		variance += delta * delta
	}
	return math.Sqrt(variance / float64(len(returns)-1)), true
}

func rsiValue(bars []Bar, n int) (float64, bool) {
	if n <= 0 || len(bars) < n+1 {
		return 0, false
	}
	start := len(bars) - n - 1
	var gains, losses float64
	count := 0
	for i := start + 1; i < len(bars); i++ {
		diff := bars[i].Close - bars[i-1].Close
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
		count++
	}
	if count == 0 || losses == 0 {
		if gains == 0 {
			return 50.0, true
		}
		return 100.0, true
	}
	avgGain := gains / float64(count)
	avgLoss := losses / float64(count)
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs), true
}
