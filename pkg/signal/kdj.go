package signal

import (
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"math"
)

func detectKDJSignals(code string, klines []ta.KlineInput, kdj *ta.KDJResult) []Signal {
	var signals []Signal
	if len(kdj.K) == 0 {
		return signals
	}

	crosses := detectLineCross(kdj.K, kdj.D)
	for i, c := range crosses {
		if c == 0 {
			continue
		}
		st := SignalGoldenCross
		if c == -1 {
			st = SignalDeathCross
		}
		signals = append(signals, Signal{
			Code:      code,
			Date:      klines[i].Time,
			Type:      st,
			Indicator: "KDJ",
			Details:   fmt.Sprintf("K(%.2f) D(%.2f) J(%.2f)", kdj.K[i], kdj.D[i], kdj.J[i]),
			Strength:  math.Abs(kdj.J[i]-50) / 50,
		})
	}

	for i, jVal := range kdj.J {
		if jVal > 100 {
			signals = append(signals, Signal{
				Code:      code,
				Date:      klines[i].Time,
				Type:      SignalOverbought,
				Indicator: "KDJ",
				Details:   fmt.Sprintf("J=%.2f", jVal),
				Strength:  (jVal - 100) / 100,
			})
		}
		if jVal < 0 {
			signals = append(signals, Signal{
				Code:      code,
				Date:      klines[i].Time,
				Type:      SignalOversold,
				Indicator: "KDJ",
				Details:   fmt.Sprintf("J=%.2f", jVal),
				Strength:  (-jVal) / 100,
			})
		}
	}
	return signals
}
