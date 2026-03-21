package signal

func detectCross(current, prev float64) int {
	if prev <= 0 && current > 0 {
		return 1
	}
	if prev >= 0 && current < 0 {
		return -1
	}
	return 0
}

func detectLineCross(line1, line2 []float64) []int {
	n := min(len(line1), len(line2))
	result := make([]int, n)
	for i := 1; i < n; i++ {
		prev := line1[i-1] - line2[i-1]
		curr := line1[i] - line2[i]
		result[i] = detectCross(curr, prev)
	}
	return result
}
