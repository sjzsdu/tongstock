package ta

// CalcMACD computes MACD using standard EMA-based definition.
// fast, slow, signal are the EMA periods for the fast line, slow line, and signal line respectively.
// Returns a MACDResult containing DIF, DEA, Hist and the used parameters.
func CalcMACD(klines []KlineInput, fast, slow, signal int) *MACDResult {
	// Basic guard: if insufficient data to compute MACD, return with meta parameters
	if len(klines) < slow+signal {
		return &MACDResult{Fast: fast, Slow: slow, Signal: signal}
	}

	// Step 1: Calculate fast and slow EMA
	emaFast := EMA(klines, fast)
	emaSlow := EMA(klines, slow)

	n := len(klines)
	dif := make([]float64, n)

	// DIF = EMA(fast) - EMA(slow)
	// Only valid from 'slow-1' onwards (when slow EMA starts)
	for i := slow - 1; i < n; i++ {
		dif[i] = emaFast[i] - emaSlow[i]
	}

	// Step 2: Calculate DEA = EMA of DIF
	// We'll compute EMA over the valid DIF region, and store results into dea aligned with the original index.
	dea := make([]float64, n)

	// valid start index for DIF values
	validStart := slow - 1
	validDIF := make([]float64, 0, n-validStart)
	for i := validStart; i < n; i++ {
		validDIF = append(validDIF, dif[i])
	}

	if len(validDIF) >= signal {
		// Initialize using SMA of the first 'signal' values
		sum := 0.0
		for i := 0; i < signal; i++ {
			sum += validDIF[i]
		}
		dea[validStart+signal-1] = sum / float64(signal)

		multiplier := 2.0 / float64(signal+1)
		// Compute subsequent EMA values over the remaining validDIF
		for i := signal; i < len(validDIF); i++ {
			idx := validStart + i
			dea[idx] = (validDIF[i]-dea[idx-1])*multiplier + dea[idx-1]
		}
	}

	// Step 3: Histogram = DIF - DEA
	hist := make([]float64, n)
	for i := 0; i < n; i++ {
		hist[i] = dif[i] - dea[i]
	}

	return &MACDResult{
		DIF:    dif,
		DEA:    dea,
		Hist:   hist,
		Fast:   fast,
		Slow:   slow,
		Signal: signal,
	}
}
