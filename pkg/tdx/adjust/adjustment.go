// Package adjust implements price adjustment (复权 / 公司行为 / 价格口径统一)
// for the TongStock paradigm research system.
//
// 设计要点:
//  1. 不复权 (raw): 直接使用交易所原始成交价格, 用于下单、原始数据存储。
//  2. 前复权 (forward): 以"最新"为基准, 历史价格 * Forward 因子, 适合技术指标
//     计算、图形展示、长周期回测。
//  3. 后复权 (backward): 以"最早"为基准, 历史价格 / Backward 因子, 适合累计
//     收益率、长期持有回测。
//
// 因子计算公式:
//   - ExRightPrice = (PrevClose - DividendPerShare + SharesPer10/10 * RightsPrice) /
//     (1 + SharesPer10/10)
//   - Forward  = ExRightPrice / PrevClose  (历史 -> 事件后)
//   - Backward = PrevClose / ExRightPrice  (事件后 -> 历史)
//   - CumForward/CumBackward 为按时间累积的乘积。
//
// 事件来源: TDX XdXr 协议帧 (除权除息、送配股上市、股本变化等)。
package adjust

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Mode 价格口径: 不复权 / 前复权 / 后复权。
type Mode string

const (
	ModeRaw      Mode = "raw"
	ModeForward  Mode = "forward"
	ModeBackward Mode = "backward"
)

// Normalize 归一化非法值为 raw。
func (m Mode) Normalize() Mode {
	switch m {
	case "", ModeRaw:
		return ModeRaw
	case ModeForward:
		return ModeForward
	case ModeBackward:
		return ModeBackward
	default:
		return ModeRaw
	}
}

// String 人类可读描述。
func (m Mode) String() string {
	switch m.Normalize() {
	case ModeRaw:
		return "不复权"
	case ModeForward:
		return "前复权"
	case ModeBackward:
		return "后复权"
	default:
		return "未知"
	}
}

// EventType 除权除息事件的语义分类。
type EventType string

const (
	EventDividend EventType = "dividend" // 纯分红
	EventSplit    EventType = "split"    // 送/转股 (股本扩张)
	EventRights   EventType = "rights"   // 配股 (含配股价)
	EventShrink   EventType = "shrink"   // 缩股
	EventOther    EventType = "other"
)

// Event 一次除权除息/公司行为事件。
type Event struct {
	Date             time.Time
	PrevClose        float64
	DividendPerShare float64
	SharesPer10      float64
	RightsPrice      float64
	ShrinkRatio      float64
	Type             EventType
}

// Factor 一次事件对应的复权因子。
//
// Forward 因子: 事件日之前的价格调整到事件日之后口径 (历史价格 * Forward)
// Backward 因子: 事件日之后的价格调整到事件日之前口径 (历史价格 / Backward)
// CumForward/CumBackward 为累积 (截至该事件日)。
type Factor struct {
	Date        time.Time
	PrevClose   float64
	ExRight     float64 // 除权价 (供审计/测试)
	Forward     float64
	Backward    float64
	CumForward  float64
	CumBackward float64
	Reason      EventType
}

// Kline 通用 K 线视图 (不依赖 protocol 包，便于测试)。
type Kline struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Amount float64
}

// Adjustment 持有计算好的因子列表 (按日期升序)。
type Adjustment struct {
	factors []*Factor
}

// New 从一组按时间升序的事件构建 Adjustment。
func New(events []Event) (*Adjustment, error) {
	if len(events) == 0 {
		return &Adjustment{}, nil
	}

	sorted := make([]Event, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Date.Before(sorted[j].Date)
	})

	factors := make([]*Factor, 0, len(sorted))
	cumForward := 1.0
	cumBackward := 1.0

	for i, ev := range sorted {
		f, err := computeOne(ev, cumForward, cumBackward)
		if err != nil {
			return nil, fmt.Errorf("event[%d] %s: %w", i, ev.Date.Format("2006-01-02"), err)
		}
		cumForward = f.CumForward
		cumBackward = f.CumBackward
		factors = append(factors, f)
	}

	return &Adjustment{factors: factors}, nil
}

// Empty 返回一个没有任何事件的空调整 (恒等映射)。
func Empty() *Adjustment { return &Adjustment{} }

// Factors 返回完整因子列表 (拷贝)。
func (a *Adjustment) Factors() []*Factor {
	out := make([]*Factor, len(a.factors))
	copy(out, a.factors)
	return out
}

// Adjust 把 (不复权) 原始价格调整为目标口径。
//
// 规则 (事件日为口径切换点, 非对称边界):
//   - raw: 直接返回 rawPrice
//   - forward (前复权, 以"最新"为基准):
//   - 对"日期 ≤ 最近一次事件"的价格, 用该事件的 CumForward 折算到最新口径.
//   - 最近一次事件之后的不复权价格已经是最新口径, 不再调整.
//   - backward (后复权, 以"最早"为基准):
//   - 对"日期 > 最早一次事件"的价格, 用该事件的 CumBackward 折算到最早口径.
//   - 最早一次事件当日及之前的不复权价格已经是最早口径, 不再调整.
//
// 直观:
//   - Forward: 事件日前一天的收盘 (10 元) 折算到事件日后口径 = 9 元, 使
//     "事件日开盘 (9 元) 与前一日收盘 (9 元)" 在同一口径下连续.
//   - Backward: 事件日后一天的收盘 (10 元) 折算到事件日前口径 = 20 元, 使
//     "事件日前收盘 (20 元) 与后一日收盘 (20 元)" 在同一口径下连续.
//
// 调用约定: date 应为该价格"所属的交易日" (K 线日期).
func (a *Adjustment) Adjust(date time.Time, rawPrice float64, mode Mode) float64 {
	mode = mode.Normalize()
	if mode == ModeRaw || rawPrice <= 0 {
		return rawPrice
	}
	if len(a.factors) == 0 {
		return rawPrice
	}

	if mode == ModeForward {
		// 前复权: 以"最新"为基准. 把事件之前的价格折算到事件之后口径.
		// idx = 第一个 Date 严格晚于 date 的事件.
		//   - idx == 0: date 早于所有事件 → 应用第一个事件的 CumForward.
		//   - 0 < idx < len: date 在事件 idx-1 与 idx 之间 → 应用事件 idx-1 的 CumForward.
		//   - idx == len: date 晚于所有事件 → 已是最新口径, 不再调整.
		idx := sort.Search(len(a.factors), func(i int) bool {
			return a.factors[i].Date.After(date)
		})
		if idx == len(a.factors) {
			return rawPrice
		}
		// 事件当日 (date == factors[idx].Date) 视为已在最新口径: 不再调整.
		if !a.factors[idx].Date.After(date) {
			return rawPrice
		}
		if idx == 0 {
			return rawPrice * a.factors[0].CumForward
		}
		return rawPrice * a.factors[idx-1].CumForward
	}

	// ModeBackward
	// 后复权: 以"最早"为基准. 把事件之后的价格折算到事件之前口径.
	// idx = 第一个 Date 严格晚于 date 的事件.
	//   - idx == len: date 晚于所有事件 → 应用最后一个事件的 CumBackward.
	//   - 0 < idx < len: date 在事件 idx-1 与 idx 之间 → 应用事件 idx 的 CumBackward.
	//   - idx == 0: date 早于所有事件 → 已是最早口径, 不再调整.
	idx := sort.Search(len(a.factors), func(i int) bool {
		return a.factors[i].Date.After(date)
	})
	if idx == 0 {
		return rawPrice
	}
	// 事件当日 (date == factors[idx-1].Date) 视为已在最早口径: 不再调整.
	if !date.After(a.factors[idx-1].Date) {
		return rawPrice
	}
	if idx == len(a.factors) {
		return rawPrice * a.factors[len(a.factors)-1].CumBackward
	}
	return rawPrice * a.factors[idx].CumBackward
}

// AdjustSeries 把一段 K 线从不复权转换为目标口径。
// 日期顺序应为升序；否则结果不符合直觉。
func (a *Adjustment) AdjustSeries(klines []Kline, mode Mode) []Kline {
	mode = mode.Normalize()
	out := make([]Kline, len(klines))
	for i, k := range klines {
		out[i] = k
		out[i].Open = a.Adjust(k.Time, k.Open, mode)
		out[i].High = a.Adjust(k.Time, k.High, mode)
		out[i].Low = a.Adjust(k.Time, k.Low, mode)
		out[i].Close = a.Adjust(k.Time, k.Close, mode)
	}
	return out
}

// AdjustRange 返回 [from, to] 闭区间 (按原始不复权价格) 在目标口径下的收益比值。
// 用于公司行为前后结果可验证的测试。
func (a *Adjustment) AdjustRange(from, to time.Time, rawFrom, rawTo float64, mode Mode) float64 {
	adjFrom := a.Adjust(from, rawFrom, mode)
	adjTo := a.Adjust(to, rawTo, mode)
	if adjFrom == 0 {
		return 0
	}
	return adjTo / adjFrom
}

// computeOne 计算单次事件的复权因子。
func computeOne(ev Event, prevCumForward, prevCumBackward float64) (*Factor, error) {
	if ev.PrevClose <= 0 {
		return nil, errors.New("prev_close must be positive")
	}
	if ev.ShrinkRatio < 0 || ev.ShrinkRatio > 1 {
		return nil, fmt.Errorf("invalid shrink_ratio: %v", ev.ShrinkRatio)
	}

	var exRightPrice float64
	var reason EventType

	switch {
	case ev.ShrinkRatio > 0:
		exRightPrice = ev.PrevClose / ev.ShrinkRatio
		reason = EventShrink
	case ev.SharesPer10 == 0 && ev.DividendPerShare > 0:
		exRightPrice = ev.PrevClose - ev.DividendPerShare
		reason = EventDividend
	case ev.SharesPer10 > 0 && ev.RightsPrice > 0:
		r := ev.SharesPer10 / 10.0
		exRightPrice = (ev.PrevClose + r*ev.RightsPrice) / (1 + r)
		reason = EventRights
	case ev.SharesPer10 > 0 && ev.RightsPrice <= 0:
		r := ev.SharesPer10 / 10.0
		exRightPrice = (ev.PrevClose - ev.DividendPerShare) / (1 + r)
		reason = EventSplit
	case ev.DividendPerShare > 0 || ev.SharesPer10 > 0:
		r := ev.SharesPer10 / 10.0
		exRightPrice = (ev.PrevClose - ev.DividendPerShare + r*ev.RightsPrice) / (1 + r)
		if r > 0 && ev.RightsPrice > 0 {
			reason = EventRights
		} else if r > 0 {
			reason = EventSplit
		} else {
			reason = EventDividend
		}
	default:
		return nil, errors.New("no adjustment effect (zero dividend / split / rights / shrink)")
	}

	if exRightPrice <= 0 {
		return nil, fmt.Errorf("invalid ex-rights price: %v (prev_close=%v)", exRightPrice, ev.PrevClose)
	}

	forward := exRightPrice / ev.PrevClose
	backward := ev.PrevClose / exRightPrice

	cumForward := prevCumForward * forward
	cumBackward := prevCumBackward * backward

	return &Factor{
		Date:        ev.Date,
		PrevClose:   ev.PrevClose,
		ExRight:     exRightPrice,
		Forward:     forward,
		Backward:    backward,
		CumForward:  cumForward,
		CumBackward: cumBackward,
		Reason:      reason,
	}, nil
}
