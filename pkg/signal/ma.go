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

	ma5 := ma["5"]
	ma10 := ma["10"]
	ma20 := ma["20"]
	if ma5 == nil || ma10 == nil || ma20 == nil {
		return signals
	}
	n := min(len(ma5), min(len(ma10), len(ma20)))
	for i := 0; i < n; i++ {
		if ma5[i] == 0 || ma10[i] == 0 || ma20[i] == 0 {
			continue
		}
		if ma5[i] > ma10[i] && ma10[i] > ma20[i] {
			signals = append(signals, Signal{
				Code:      code,
				Date:      klines[i].Time,
				Type:      SignalBullAlign,
				Indicator: "MA",
				Details:   fmt.Sprintf("MA5(%.2f) > MA10(%.2f) > MA20(%.2f)", ma5[i], ma10[i], ma20[i]),
				Strength:  (ma5[i] - ma20[i]) / ma20[i],
			})
		}
		if ma5[i] < ma10[i] && ma10[i] < ma20[i] {
			signals = append(signals, Signal{
				Code:      code,
				Date:      klines[i].Time,
				Type:      SignalBearAlign,
				Indicator: "MA",
				Details:   fmt.Sprintf("MA5(%.2f) < MA10(%.2f) < MA20(%.2f)", ma5[i], ma10[i], ma20[i]),
				Strength:  (ma20[i] - ma5[i]) / ma20[i],
			})
		}
	}
	return signals
}
