package ta

// SMA calculates Simple Moving Average for the given period.
// Returns a slice of float64 of the same length as input klines.
// Values before period-1 are 0 (not enough data yet).
func SMA(klines []KlineInput, period int) []float64 {
	result := make([]float64, len(klines))
	if len(klines) < period || period <= 0 {
		return result
	}

	sum := 0.0
	for i := 0; i < period-1; i++ {
		sum += klines[i].Close
	}

	for i := period - 1; i < len(klines); i++ {
		sum += klines[i].Close
		if i >= period {
			sum -= klines[i-period].Close
		}
		result[i] = sum / float64(period)
	}

	return result
}

// EMA calculates Exponential Moving Average for the given period.
// Returns a slice of float64 of the same length as input klines.
// First EMA value is the SMA of the first 'period' values.
func EMA(klines []KlineInput, period int) []float64 {
	result := make([]float64, len(klines))
	if len(klines) < period || period <= 0 {
		return result
	}

	multiplier := 2.0 / float64(period+1)

	// Initialize with SMA of first period
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	result[period-1] = sum / float64(period)

	// Calculate EMA
	for i := period; i < len(klines); i++ {
		result[i] = (klines[i].Close-result[i-1])*multiplier + result[i-1]
	}

	return result
}
