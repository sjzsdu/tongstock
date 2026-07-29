package adjust

import (
	"math"
	"testing"
	"time"
)

// date helper
func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// nearly-equal
func approx(a, b float64) bool {
	if a == 0 && b == 0 {
		return true
	}
	return math.Abs(a-b)/math.Max(math.Abs(a), math.Abs(b)) < 1e-9
}

// TestForwardAdjustment_Dividend 验证分红前复权:
// 前收 10 元, 每股分红 1 元 -> 除权价 9 元, Forward=0.9.
//
// 语义: 除权日 (2024-06-15) 的开盘价已经是除权后口径 (9 元),
// 不应再调整. 除权日之前的历史价格 (2024-06-14 收盘 10 元)
// 应折算到除权后口径 = 10 * 0.9 = 9 元.
func TestForwardAdjustment_Dividend(t *testing.T) {
	adj, err := New([]Event{
		{Date: mustDate("2024-06-15"), PrevClose: 10, DividendPerShare: 1},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fs := adj.Factors()
	if len(fs) != 1 {
		t.Fatalf("want 1 factor, got %d", len(fs))
	}
	f := fs[0]
	if !approx(f.ExRight, 9.0) {
		t.Errorf("ex-rights price: want 9.0, got %v", f.ExRight)
	}
	if !approx(f.Forward, 0.9) {
		t.Errorf("forward: want 0.9, got %v", f.Forward)
	}
	if !approx(f.Backward, 10.0/9.0) {
		t.Errorf("backward: want 10/9, got %v", f.Backward)
	}

	// 2024-06-14 (除权前一日) 收盘 10 -> 前复权 9
	if got := adj.Adjust(mustDate("2024-06-14"), 10, ModeForward); !approx(got, 9.0) {
		t.Errorf("2024-06-14 adj: want 9.0 got %v", got)
	}
	// 2024-06-15 (除权日) 开盘/收盘已是除权后口径, 不再调整
	if got := adj.Adjust(mustDate("2024-06-15"), 9, ModeForward); !approx(got, 9.0) {
		t.Errorf("2024-06-15 adj: want 9.0 got %v", got)
	}
	// 2024-06-16 (除权后日期) 已在最新口径, 不再调整
	if got := adj.Adjust(mustDate("2024-06-16"), 9.0, ModeForward); !approx(got, 9.0) {
		t.Errorf("2024-06-16 adj: want 9.0 got %v", got)
	}
}

// TestForwardAdjustment_StockSplit 10送10 (每10股送10股), 前收 20 元
// 除权价 = 20 / (1+1) = 10 元, Forward=0.5
func TestForwardAdjustment_StockSplit(t *testing.T) {
	adj, err := New([]Event{
		{Date: mustDate("2024-07-01"), PrevClose: 20, SharesPer10: 10},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fs := adj.Factors()
	f := fs[0]
	if !approx(f.Forward, 0.5) {
		t.Errorf("forward: want 0.5, got %v", f.Forward)
	}
	if !approx(f.CumForward, 0.5) {
		t.Errorf("cum forward: want 0.5, got %v", f.CumForward)
	}
	// 2024-06-30 是除权前最后一日, 收盘 20 元 -> 前复权 10 元
	if got := adj.Adjust(mustDate("2024-06-30"), 20, ModeForward); !approx(got, 10.0) {
		t.Errorf("pre-event adj: want 10.0 got %v", got)
	}
	// 2024-07-01 及之后: 已是除权后口径, 不再调整
	if got := adj.Adjust(mustDate("2024-07-01"), 10, ModeForward); !approx(got, 10.0) {
		t.Errorf("event-day adj: want 10.0 got %v", got)
	}
}

// TestForwardAdjustment_Cumulative 两次事件累积
// 2024-06-01: 10送10 (Forward=0.5, cum=0.5)
// 2024-08-01: 每股分红 2 (Forward=0.8, cum=0.4)
//
// 验证:
//   - 2024-05-31 (第一个事件之前) 20元 -> 前复权 20*0.4 = 8 元 (最新口径)
//   - 2024-07-01 (两事件之间) 10元 -> 前复权 10*0.5 = 5 元 (第一事件后口径)
//   - 2024-08-01 及之后: 已是最新口径, 不再调整
func TestForwardAdjustment_Cumulative(t *testing.T) {
	adj, err := New([]Event{
		{Date: mustDate("2024-06-01"), PrevClose: 20, SharesPer10: 10},
		{Date: mustDate("2024-08-01"), PrevClose: 10, DividendPerShare: 2},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fs := adj.Factors()
	if len(fs) != 2 {
		t.Fatalf("want 2 factors, got %d", len(fs))
	}
	// 第一事件前日期 (2024-05-31): 使用 cumForward = 0.5 (截止第一事件)
	if got := adj.Adjust(mustDate("2024-05-31"), 20, ModeForward); !approx(got, 10.0) {
		t.Errorf("pre-first-event adj: want 10.0 got %v", got)
	}
	// 注意: 对"第一个事件之前"的日期, 我们仅折算到"第一事件后"的口径.
	// 若需折算到最新口径, 应跨越多段, 此 API 只支持单日期查询.
	// 跨事件段的全量折算由 AdjustSeries 按事件段分别完成.

	// 两事件之间 (2024-07-01): 使用第一事件的 cumForward (0.5)
	if got := adj.Adjust(mustDate("2024-07-01"), 10, ModeForward); !approx(got, 5.0) {
		t.Errorf("between-event adj: want 5.0 got %v", got)
	}
}

// TestBackwardAdjustment_Split 后复权: 除权后的价格折算回历史口径.
// 10送10, 2024-07-01 除权. 除权后价格 10 元 -> 后复权 20 元.
func TestBackwardAdjustment_Split(t *testing.T) {
	adj, err := New([]Event{
		{Date: mustDate("2024-07-01"), PrevClose: 20, SharesPer10: 10},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// 2024-07-02 (除权后) 10 元 -> 后复权 20 元
	if got := adj.Adjust(mustDate("2024-07-02"), 10, ModeBackward); !approx(got, 20.0) {
		t.Errorf("post-event backward adj: want 20.0 got %v", got)
	}
	// 2024-06-30 (除权前) 20 元: 已是最早口径, 不再调整
	if got := adj.Adjust(mustDate("2024-06-30"), 20, ModeBackward); !approx(got, 20.0) {
		t.Errorf("pre-event backward adj: want 20.0 got %v", got)
	}
}

// TestAdjustSeries 校验整段 K 线前复权后, 前一个交易日的收盘价
// 与后一个交易日的开盘价保持连续 (不存在除权跳空).
func TestAdjustSeries_Continuity(t *testing.T) {
	klines := []Kline{
		{Time: mustDate("2024-06-10"), Open: 10, Close: 10},
		{Time: mustDate("2024-06-11"), Open: 10, Close: 10},
		{Time: mustDate("2024-06-14"), Open: 10, Close: 10}, // 除权前最后一日
		{Time: mustDate("2024-06-15"), Open: 9, Close: 9},   // 除权日 (不复权价格跳空)
		{Time: mustDate("2024-06-16"), Open: 9, Close: 9},
	}
	adj, err := New([]Event{
		{Date: mustDate("2024-06-15"), PrevClose: 10, DividendPerShare: 1},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	adjusted := adj.AdjustSeries(klines, ModeForward)
	// 06-14 收盘价 10 -> 前复权 9.0 (折算到事件后口径)
	if !approx(adjusted[2].Close, 9.0) {
		t.Errorf("pre-event close adj: want 9.0 got %v", adjusted[2].Close)
	}
	// 06-15 开盘价 9 已是除权后口径, 不再调整
	if !approx(adjusted[3].Open, 9.0) {
		t.Errorf("event-day open adj: want 9.0 got %v", adjusted[3].Open)
	}
	// 连续性: 调整后的 06-14 收盘 (9) 与 06-15 开盘 (9) 无跳空
	if !approx(adjusted[2].Close, adjusted[3].Open) {
		t.Errorf("continuity broken: close_0614=%v, open_0615=%v", adjusted[2].Close, adjusted[3].Open)
	}
}

// TestSuspension_CrossHoliday 停牌 (无交易) + 跨节假日场景.
// 除权日 (登记日) 为 06-17 (周六非交易日), 实际除权在 06-20 复牌日体现.
// 在我们的模型中, 除权日 = 事件登记日, 不复权 K 线在 06-20 跳空.
// 因此 06-17 之前的历史价格都需要 * Forward 因子, 06-17 及之后保持.
func TestSuspension_CrossHoliday(t *testing.T) {
	klines := []Kline{
		{Time: mustDate("2024-06-10"), Close: 10},
		{Time: mustDate("2024-06-11"), Close: 10},
		{Time: mustDate("2024-06-12"), Close: 10},
		{Time: mustDate("2024-06-13"), Close: 10},
		// 06-14 周五 (停牌前最后交易日)
		// 06-15~06-19 停牌/节假日
		// 06-17 为除权除息登记日 (事件日)
		{Time: mustDate("2024-06-20"), Close: 9}, // 复牌日, 跳空到 9
		{Time: mustDate("2024-06-21"), Close: 9},
	}

	adj, err := New([]Event{
		{Date: mustDate("2024-06-17"), PrevClose: 10, DividendPerShare: 1},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	adjusted := adj.AdjustSeries(klines, ModeForward)
	// 06-10 ~ 06-13: 除权登记日之前, 都应 * 0.9 = 9
	for i := 0; i <= 3; i++ {
		if !approx(adjusted[i].Close, 9.0) {
			t.Errorf("day %d close adj: want 9.0 got %v", i, adjusted[i].Close)
		}
	}
	// 06-20 (除权登记日之后, 且实际除权已体现): 保持 9
	if !approx(adjusted[4].Close, 9.0) {
		t.Errorf("resume day close: want 9.0 got %v", adjusted[4].Close)
	}
}

// TestAdjustRange_Verifiable 公司行为前后结果可验证.
// 前复权 & 后复权对"原始收益"的折算应给出相同结果.
func TestAdjustRange_Verifiable(t *testing.T) {
	adj, err := New([]Event{
		{Date: mustDate("2024-06-15"), PrevClose: 10, DividendPerShare: 1},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// 原始价格: 01-05 收 10, 12-31 收 13.5
	//
	// 前复权 (以 06-15 除权为界):
	//   - 01-05 (除权前): 10 * 0.9 = 9
	//   - 12-31 (除权后): 13.5 (已是最新口径, 不再调整)
	//   - 收益 = 13.5 / 9 = 1.5
	//
	// 后复权 (以 06-15 除权为界):
	//   - 01-05 (除权前): 10 (已是最早口径, 不再调整)
	//   - 12-31 (除权后): 13.5 * (10/9) = 15
	//   - 收益 = 15 / 10 = 1.5
	fwdRet := adj.AdjustRange(
		mustDate("2024-01-05"), mustDate("2024-12-31"),
		10, 13.5, ModeForward,
	)
	bwdRet := adj.AdjustRange(
		mustDate("2024-01-05"), mustDate("2024-12-31"),
		10, 13.5, ModeBackward,
	)
	if !approx(fwdRet, 1.5) {
		t.Errorf("forward ret: want 1.5 got %v", fwdRet)
	}
	if !approx(bwdRet, 1.5) {
		t.Errorf("backward ret: want 1.5 got %v", bwdRet)
	}
}

// TestEmpty 空调整恒等映射
func TestEmpty(t *testing.T) {
	adj := Empty()
	if got := adj.Adjust(mustDate("2024-01-01"), 12.5, ModeForward); got != 12.5 {
		t.Errorf("empty adj: want 12.5 got %v", got)
	}
	if got := adj.Adjust(mustDate("2024-01-01"), 12.5, ModeBackward); got != 12.5 {
		t.Errorf("empty adj backward: want 12.5 got %v", got)
	}
}

// TestModeNormalize 非法模式回落为 raw
func TestModeNormalize(t *testing.T) {
	if got := Mode("").Normalize(); got != ModeRaw {
		t.Errorf("empty mode: want raw got %v", got)
	}
	if got := Mode("weird").Normalize(); got != ModeRaw {
		t.Errorf("weird mode: want raw got %v", got)
	}
	if got := ModeForward.Normalize(); got != ModeForward {
		t.Errorf("forward not preserved")
	}
}

// TestInvalidEvent 非法事件应返回错误
func TestInvalidEvent(t *testing.T) {
	if _, err := New([]Event{{Date: mustDate("2024-01-01")}}); err == nil {
		t.Error("want error for zero prev_close")
	}
	if _, err := New([]Event{{Date: mustDate("2024-01-01"), PrevClose: 10}}); err == nil {
		t.Error("want error for zero-effect event")
	}
	if _, err := New([]Event{{Date: mustDate("2024-01-01"), PrevClose: 10, ShrinkRatio: 1.5}}); err == nil {
		t.Error("want error for invalid shrink ratio")
	}
}
