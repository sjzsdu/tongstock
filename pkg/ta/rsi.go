package ta

func CalcRSI(klines []KlineInput, period int) []float64 {
	result := make([]float64, len(klines))
	if len(klines) <= period || period <= 0 {
		return result
	}

	length := len(klines)

	// First average gain/loss for initial RSI
	avgGain := 0.0
	avgLoss := 0.0

	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			avgGain += change
		} else {
			avgLoss -= change // change is negative, make it positive
		}
	}

	avgGain /= float64(period)
	avgLoss /= float64(period)

	// First RSI at index 'period'
	if avgLoss == 0 {
		result[period] = 100
	} else {
		rs := avgGain / avgLoss
		result[period] = 100 - 100/(1+rs)
	}

	// Subsequent RSI values using smoothed averages (Wilder's smoothing)
	for i := period + 1; i < length; i++ {
		change := klines[i].Close - klines[i-1].Close

		var gain, loss float64
		if change > 0 {
			gain = change
		} else {
			loss = -change
		}

		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)

		if avgLoss == 0 {
			result[i] = 100
		} else {
			rs := avgGain / avgLoss
			result[i] = 100 - 100/(1+rs)
		}
	}

	return result
}
