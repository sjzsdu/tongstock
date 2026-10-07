package factorlab

import (
	"math"
	"sort"
)

// pearson 是皮尔逊相关系数；退化输入（<2 对、零方差）返回 NaN 语义（ok=false）。
func pearson(x, y []float64) (float64, bool) {
	if len(x) != len(y) || len(x) < 2 {
		return 0, false
	}
	mx, my := mean(x), mean(y)
	num, dx, dy := 0.0, 0.0, 0.0
	for i := range x {
		a, b := x[i]-mx, y[i]-my
		num += a * b
		dx += a * a
		dy += b * b
	}
	if dx <= 0 || dy <= 0 {
		return 0, false
	}
	return num / math.Sqrt(dx*dy), true
}

// rankIC 计算两序列的截面 Spearman 秩相关（RankIC）。
// 样本 <2 或任一侧零离散度时无效（ok=false，该截面不计入统计）。
func rankIC(factorValues, forwardReturns map[string]float64) (float64, bool) {
	if len(factorValues) != len(forwardReturns) || len(factorValues) < minCrossSection {
		return 0, false
	}
	codes := make([]string, 0, len(factorValues))
	for code := range factorValues {
		if _, ok := forwardReturns[code]; !ok {
			return 0, false
		}
		codes = append(codes, code)
	}
	x := make([]float64, 0, len(codes))
	y := make([]float64, 0, len(codes))
	for _, code := range codes {
		x = append(x, factorValues[code])
		y = append(y, forwardReturns[code])
	}
	rx, ok := ranks(x)
	if !ok {
		return 0, false
	}
	ry, ok := ranks(y)
	if !ok {
		return 0, false
	}
	return pearson(rx, ry)
}

// minCrossSection 是有效截面的最小股票数：太少时秩相关无意义。
// methodautomation 的 fail-closed 门槛是 5，这里对齐。
const minCrossSection = 5

// ranks 返回平均秩（并列取平均）；零离散度（全部并列）时 ok=false。
func ranks(values []float64) ([]float64, bool) {
	n := len(values)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return values[idx[a]] < values[idx[b]] })
	out := make([]float64, n)
	i := 0
	for i < n {
		j := i
		for j+1 < n && values[idx[j+1]] == values[idx[i]] {
			j++
		}
		avg := float64(i+j+2) / 2 // 秩从 1 起：i..j 的平均秩
		for k := i; k <= j; k++ {
			out[idx[k]] = avg
		}
		i = j + 1
	}
	// 全部并列 = 零离散度，秩相关无意义。
	for k := 1; k < n; k++ {
		if values[k] != values[0] {
			return out, true
		}
	}
	return nil, false
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func std(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	m := mean(v)
	sum := 0.0
	for _, x := range v {
		sum += (x - m) * (x - m)
	}
	return math.Sqrt(sum / float64(len(v)-1))
}

// zscores 返回截面 z 分（值缺失的股票不在输出里）；零方差时 ok=false。
func zscores(values map[string]float64) (map[string]float64, bool) {
	n := len(values)
	if n < 2 {
		return nil, false
	}
	xs := make([]float64, 0, n)
	for _, v := range values {
		xs = append(xs, v)
	}
	m, s := mean(xs), std(xs)
	if s <= 0 {
		return nil, false
	}
	out := make(map[string]float64, n)
	for code, v := range values {
		out[code] = (v - m) / s
	}
	return out, true
}
