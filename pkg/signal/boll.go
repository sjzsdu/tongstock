package signal

import (
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/ta"
)

func detectBOLLSignals(code string, klines []ta.KlineInput, boll *ta.BOLLResult) []Signal {
	var signals []Signal
	for i := range klines {
		close := klines[i].Close
		if close > boll.Upper[i] && boll.Upper[i] > 0 {
			signals = append(signals, Signal{
				Code:      code,
				Date:      klines[i].Time,
				Type:      SignalBreakUpper,
				Indicator: "BOLL",
				Details:   fmt.Sprintf("Close(%.2f) > Upper(%.2f)", close, boll.Upper[i]),
				Strength:  (close - boll.Upper[i]) / boll.Upper[i],
			})
		}
		if close < boll.Lower[i] && boll.Lower[i] > 0 {
			signals = append(signals, Signal{
				Code:      code,
				Date:      klines[i].Time,
				Type:      SignalBreakLower,
				Indicator: "BOLL",
				Details:   fmt.Sprintf("Close(%.2f) < Lower(%.2f)", close, boll.Lower[i]),
				Strength:  (boll.Lower[i] - close) / boll.Lower[i],
			})
		}
	}
	return signals
}
