package signal

import (
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/ta"
)

func detectMASignals(code string, klines []ta.KlineInput, ma map[string][]float64) []Signal {
	var signals []Signal

	periods := []string{"5", "10", "20", "60"}
	for i := 0; i < len(periods)-1; i++ {
		line1, ok1 := ma[periods[i]]
		line2, ok2 := ma[periods[i+1]]
		if !ok1 || !ok2 {
			continue
		}
		crosses := detectLineCross(line1, line2)
		if len(crosses) > len(klines) {
			crosses = crosses[:len(klines)]
		}
		for j, c := range crosses {
			if c == 0 {
				continue
			}
			st := SignalGoldenCross
			if c == -1 {
				st = SignalDeathCross
			}
			signals = append(signals, Signal{
				Code:      code,
				Date:      klines[j].Time,
				Type:      st,
				Indicator: fmt.Sprintf("MA%s/MA%s", periods[i], periods[i+1]),
				Details:   fmt.Sprintf("MA%s(%.2f) MA%s(%.2f)", periods[i], line1[j], periods[i+1], line2[j]),
				Strength:  0.5,
			})
		}
	}

	makeMA := func(maMap map[string][]float64) (isBull, isBear bool, details string, strength float64) {
		ma5 := maMap["5"]
		ma10 := maMap["10"]
		ma20 := maMap["20"]
		ma60 := maMap["60"]

		idx := min(len(klines), len(ma5), len(ma10), len(ma20)) - 1
		if ma5 == nil || ma10 == nil || ma20 == nil || idx < 0 {
			return
		}

		if ma5[idx] == 0 || ma10[idx] == 0 || ma20[idx] == 0 {
			return
		}

		isBull = ma5[idx] > ma10[idx] && ma10[idx] > ma20[idx]
		if isBull && ma60 != nil && len(ma60) > idx && ma60[idx] > 0 {
			isBull = isBull && ma20[idx] > ma60[idx]
		}

		isBear = ma5[idx] < ma10[idx] && ma10[idx] < ma20[idx]
		if isBear && ma60 != nil && len(ma60) > idx && ma60[idx] > 0 {
			isBear = isBear && ma20[idx] < ma60[idx]
		}

		if isBull {
			details = fmt.Sprintf("MA5(%.2f) > MA10(%.2f) > MA20(%.2f)", ma5[idx], ma10[idx], ma20[idx])
			if ma60 != nil && len(ma60) > idx && ma60[idx] > 0 {
				details += fmt.Sprintf(" > MA60(%.2f)", ma60[idx])
			}
			strength = (ma5[idx] - ma20[idx]) / ma20[idx]
		}
		if isBear {
			details = fmt.Sprintf("MA5(%.2f) < MA10(%.2f) < MA20(%.2f)", ma5[idx], ma10[idx], ma20[idx])
			if ma60 != nil && len(ma60) > idx && ma60[idx] > 0 {
				details += fmt.Sprintf(" < MA60(%.2f)", ma60[idx])
			}
			strength = (ma20[idx] - ma5[idx]) / ma20[idx]
		}
		return
	}

	isBull, isBear, details, strength := makeMA(ma)
	lastIdx := len(klines) - 1
	if lastIdx < 0 {
		return signals
	}

	if isBull {
		signals = append(signals, Signal{
			Code:      code,
			Date:      klines[lastIdx].Time,
			Type:      SignalBullAlign,
			Indicator: "MA",
			Details:   details,
			Strength:  strength,
		})
	}
	if isBear {
		signals = append(signals, Signal{
			Code:      code,
			Date:      klines[lastIdx].Time,
			Type:      SignalBearAlign,
			Indicator: "MA",
			Details:   details,
			Strength:  strength,
		})
	}

	return signals
}
