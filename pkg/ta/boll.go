package ta

import "math"

func CalcBOLL(klines []KlineInput, n int, k float64) *BOLLResult {
	result := &BOLLResult{N: n, K: k}
	if len(klines) < n || n <= 0 {
		return result
	}

	length := len(klines)
	middle := make([]float64, length)
	upper := make([]float64, length)
	lower := make([]float64, length)

	sum := 0.0
	sumSq := 0.0

	for i := 0; i < n-1; i++ {
		sum += klines[i].Close
		sumSq += klines[i].Close * klines[i].Close
	}

	for i := n - 1; i < length; i++ {
		sum += klines[i].Close
		sumSq += klines[i].Close * klines[i].Close

		if i >= n {
			sum -= klines[i-n].Close
			sumSq -= klines[i-n].Close * klines[i-n].Close
		}

		mean := sum / float64(n)
		variance := sumSq/float64(n) - mean*mean
		if variance < 0 {
			variance = 0
		}
		stdDev := math.Sqrt(variance)

		middle[i] = mean
		upper[i] = mean + k*stdDev
		lower[i] = mean - k*stdDev
	}

	result.Middle = middle
	result.Upper = upper
	result.Lower = lower
	return result
}
