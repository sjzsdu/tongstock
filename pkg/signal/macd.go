package signal

import (
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"math"
)

func detectMACDSignals(code string, klines []ta.KlineInput, macd *ta.MACDResult) []Signal {
	var signals []Signal
	if len(macd.DIF) == 0 {
		return signals
	}

	crosses := detectLineCross(macd.DIF, macd.DEA)
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
			Indicator: "MACD",
			Details:   fmt.Sprintf("DIF(%.2f) DEA(%.2f)", macd.DIF[i], macd.DEA[i]),
			Strength:  math.Abs(macd.Hist[i]),
		})
	}
	return signals
}
