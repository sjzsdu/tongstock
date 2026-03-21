package signal

import (
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/ta"
)

func detectRSISignals(code string, klines []ta.KlineInput, rsi map[string][]float64) []Signal {
	var signals []Signal

	for period, values := range rsi {
		for i, val := range values {
			if val == 0 {
				continue
			}
			if val > 80 {
				signals = append(signals, Signal{
					Code:      code,
					Date:      klines[i].Time,
					Type:      SignalOverbought,
					Indicator: "RSI" + period,
					Details:   fmt.Sprintf("RSI%s=%.2f", period, val),
					Strength:  (val - 80) / 20,
				})
			}
			if val < 20 {
				signals = append(signals, Signal{
					Code:      code,
					Date:      klines[i].Time,
					Type:      SignalOversold,
					Indicator: "RSI" + period,
					Details:   fmt.Sprintf("RSI%s=%.2f", period, val),
					Strength:  (20 - val) / 20,
				})
			}
		}
	}
	return signals
}
