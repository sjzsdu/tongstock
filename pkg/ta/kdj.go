package ta

import "math"

func CalcKDJ(klines []KlineInput, n, m1, m2 int) *KDJResult {
	result := &KDJResult{N: n, M1: m1, M2: m2}
	if len(klines) < n || n <= 0 {
		return result
	}

	length := len(klines)
	rsv := make([]float64, length)
	k := make([]float64, length)
	d := make([]float64, length)
	j := make([]float64, length)

	for i := n - 1; i < length; i++ {
		highest := -math.MaxFloat64
		lowest := math.MaxFloat64

		for p := i - n + 1; p <= i; p++ {
			if klines[p].High > highest {
				highest = klines[p].High
			}
			if klines[p].Low < lowest {
				lowest = klines[p].Low
			}
		}

		if highest == lowest {
			rsv[i] = 50.0
		} else {
			rsv[i] = (klines[i].Close - lowest) / (highest - lowest) * 100
		}
	}

	kMultiplier := 1.0 / float64(m1)
	dMultiplier := 1.0 / float64(m2)

	k[n-1] = rsv[n-1]
	d[n-1] = k[n-1]

	for i := n; i < length; i++ {
		k[i] = (1-kMultiplier)*k[i-1] + kMultiplier*rsv[i]
		d[i] = (1-dMultiplier)*d[i-1] + dMultiplier*k[i]
	}

	for i := 0; i < length; i++ {
		j[i] = 3*k[i] - 2*d[i]
	}

	result.K = k
	result.D = d
	result.J = j
	return result
}
