package ta

func CalcVolumeRatio(klines []KlineInput, avgPeriod int) *VolumeRatioResult {
	result := &VolumeRatioResult{
		Current: 0,
		Avg5:    0,
		Ratio:   0,
		Signal:  "normal",
	}

	if len(klines) == 0 {
		return result
	}

	n := len(klines)
	result.Current = klines[n-1].Volume

	if n < avgPeriod {
		result.Avg5 = result.Current
		result.Ratio = 1.0
	} else {
		sum := 0.0
		for i := n - avgPeriod; i < n; i++ {
			sum += klines[i].Volume
		}
		result.Avg5 = sum / float64(avgPeriod)
		if result.Avg5 > 0 {
			result.Ratio = result.Current / result.Avg5
		}
	}

	if result.Ratio > 2.0 {
		result.Signal = "very_active"
	} else if result.Ratio > 1.5 {
		result.Signal = "active"
	} else if result.Ratio < 0.5 {
		result.Signal = "inactive"
	} else {
		result.Signal = "normal"
	}

	return result
}
