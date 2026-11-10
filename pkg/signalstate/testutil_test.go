package signalstate_test

import (
	"math"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"github.com/sjzsdu/tongstock/pkg/ta"
)

// syntheticKlines 生成一条确定性的价格路径：慢周期趋势（约 180 日一轮）叠加
// 快周期波动（约 23 日一轮）。不含随机数，同样的 n 永远得到同样的序列，
// 测试失败可原样复现。
func syntheticKlines(n int) []ta.KlineInput {
	start := time.Date(2019, 1, 2, 0, 0, 0, 0, time.UTC)
	out := make([]ta.KlineInput, 0, n)
	prev := 10.0
	for i := 0; i < n; i++ {
		wave := 1 + 0.30*math.Sin(2*math.Pi*float64(i)/180) + 0.09*math.Sin(2*math.Pi*float64(i)/23)
		c := 10 * wave
		high := math.Max(prev, c) * 1.012
		low := math.Min(prev, c) * 0.988
		out = append(out, ta.KlineInput{
			Time: start.AddDate(0, 0, i), Open: prev, High: high, Low: low,
			Close: c, Volume: 1000 + float64(i), Amount: c * 1000,
		})
		prev = c
	}
	return out
}

// sameVector 逐字段比较两个状态向量（map 顺序无关）。
func sameVector(t *testing.T, want, got signalstate.StateVector, where string) {
	t.Helper()
	if !want.Date.Equal(got.Date) {
		t.Fatalf("%s: date mismatch: want %s got %s", where, want.Date, got.Date)
	}
	if want.Code != got.Code {
		t.Fatalf("%s: code mismatch: want %s got %s", where, want.Code, got.Code)
	}
	if want.AdjustVersion != got.AdjustVersion || want.CalcVersion != got.CalcVersion {
		t.Fatalf("%s: version mismatch: want %s/%s got %s/%s",
			where, want.AdjustVersion, want.CalcVersion, got.AdjustVersion, got.CalcVersion)
	}
	for _, dim := range signalstate.Dims() {
		if want.State(dim) != got.State(dim) {
			t.Fatalf("%s: dim %s state mismatch: want %s got %s", where, dim, want.State(dim), got.State(dim))
		}
		if want.IsInsufficient(dim) != got.IsInsufficient(dim) {
			t.Fatalf("%s: dim %s insufficient flag mismatch", where, dim)
		}
		if want.Reasons[dim] != got.Reasons[dim] {
			t.Fatalf("%s: dim %s reason mismatch:\n want %q\n got  %q", where, dim, want.Reasons[dim], got.Reasons[dim])
		}
	}
}

// countStates 统计四维各态的出现次数。
func countStates(series []signalstate.StateVector) map[signalstate.Dim]map[signalstate.State]int {
	counts := make(map[signalstate.Dim]map[signalstate.State]int, 4)
	for _, dim := range signalstate.Dims() {
		counts[dim] = map[signalstate.State]int{}
	}
	for _, v := range series {
		for _, dim := range signalstate.Dims() {
			counts[dim][v.State(dim)]++
		}
	}
	return counts
}
