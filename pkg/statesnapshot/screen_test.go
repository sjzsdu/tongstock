package statesnapshot

import (
	"context"
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"testing"
)

func TestScreenANDNeutralAndTransition(t *testing.T) {
	db, err := storage.New(storage.Config{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	put := func(code, date string, cross, ma signalstate.State, bad bool) {
		t.Helper()
		for _, d := range signalstate.Dims() {
			v := signalstate.StateNeutral
			if d == signalstate.DimCross {
				v = cross
			}
			if d == signalstate.DimMA {
				v = ma
			}
			if bad {
				v = signalstate.StateInsufficient
			}
			if _, e := db.DB().Exec(`INSERT INTO signal_state_daily VALUES(?,?,9,?,?,?,'raw','test','hash')`, code, date, d, v, bad); e != nil {
				t.Fatal(e)
			}
		}
	}
	put("a", "20240101", signalstate.StateSell, signalstate.StateBuy, false)
	put("a", "20240103", signalstate.StateBuy, signalstate.StateBuy, false)
	put("b", "20240103", signalstate.StateBuy, signalstate.StateSell, false)
	put("c", "20240103", signalstate.StateBuy, signalstate.StateBuy, true)
	s := Store{DB: db.DB()}
	f := StateFilter{Conditions: []Condition{{Dim: "cross", State: "buy"}, {Dim: "ma", State: "buy"}, {Dim: "osc", State: "neutral"}}}
	out, e := s.Screen(context.Background(), []string{"a", "b", "c"}, "20240103", f)
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Results) != 1 || out.Results[0].Code != "a" || out.InsufficientCount != 1 {
		t.Fatalf("%+v", out)
	}
	f.Transition = &Transition{Dim: "cross", From: []signalstate.State{"sell", "neutral"}, To: "buy", WithinDays: 5}
	out, e = s.Screen(context.Background(), []string{"a", "b", "c"}, "20240103", f)
	if e != nil || len(out.Results) != 1 {
		t.Fatalf("%+v %v", out, e)
	}
	// An already-buy state is not a transition, and a later day cannot see a future transition.
	out, e = s.Screen(context.Background(), []string{"a"}, "20240101", f)
	if e != nil || len(out.Results) != 0 {
		t.Fatalf("%+v %v", out, e)
	}
	f.Transition.WithinDays = 1
	out, e = s.Screen(context.Background(), []string{"a"}, "20240110", f)
	if e != nil || len(out.Results) != 0 {
		t.Fatalf("%+v %v", out, e)
	}
}

func TestScreenPoolAndSQLParity(t *testing.T) {
	db, err := storage.New(storage.Config{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	codes := []string{}
	for i := 0; i < 5252; i++ {
		code := fmt.Sprintf("%06d", i)
		codes = append(codes, code)
		for j, d := range signalstate.Dims() {
			state := []signalstate.State{signalstate.StateBuy, signalstate.StateSell, signalstate.StateNeutral}[(i+j)%3]
			bad := i%71 == 0
			if bad {
				state = signalstate.StateInsufficient
			}
			if _, err = tx.Exec(`INSERT INTO signal_state_daily VALUES(?,'20240103',9,?,?,?,'raw','test','hash')`, code, d, state, bad); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	filter := StateFilter{Conditions: []Condition{{Dim: "cross", State: "buy"}, {Dim: "ma", State: "buy"}, {Dim: "osc", State: "sell"}}}
	out, err := (Store{DB: db.DB()}).Screen(context.Background(), append(codes, "absent"), "", filter)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.DB().Query(`SELECT c.code FROM signal_state_daily c JOIN signal_state_daily m ON c.code=m.code AND c.trade_date=m.trade_date AND m.dim='ma' JOIN signal_state_daily o ON c.code=o.code AND c.trade_date=o.trade_date AND o.dim='osc' WHERE c.dim='cross' AND c.state='buy' AND m.state='buy' AND o.state='sell' ORDER BY c.code`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	expected := []string{}
	for rows.Next() {
		var code string
		if err = rows.Scan(&code); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, code)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != len(expected) || len(expected) <= 500 || out.MissingCount != 1 || out.InsufficientCount != 74 || out.Date != "20240103" {
		t.Fatalf("hits=%d expected=%d excluded=%d missing=%d date=%s", len(out.Results), len(expected), out.InsufficientCount, out.MissingCount, out.Date)
	}
	for i, r := range out.Results {
		if r.Code != expected[i] || len(r.States) != 4 {
			t.Fatalf("row %d: %+v != %s", i, r, expected[i])
		}
	}
}

func TestScreenTransitionWindowAndInsufficientPredecessor(t *testing.T) {
	db, err := storage.New(storage.Config{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, entry := range []struct{ code, date, state string }{
		{"edge", "20240101", "sell"}, {"edge", "20240103", "buy"}, {"edge", "20240107", "buy"}, {"edge", "20240108", "sell"},
		{"gap", "20240101", "sell"}, {"gap", "20240102", "insufficient_data"}, {"gap", "20240103", "buy"}, {"gap", "20240107", "buy"},
		{"steady", "20240101", "buy"}, {"steady", "20240107", "buy"},
	} {
		for _, d := range signalstate.Dims() {
			state := entry.state
			if d != signalstate.DimCross {
				state = "neutral"
			}
			if _, err = db.DB().Exec(`INSERT INTO signal_state_daily VALUES(?,?,9,?,?,?,'raw','test','hash')`, entry.code, entry.date, d, state, state == "insufficient_data"); err != nil {
				t.Fatal(err)
			}
		}
	}
	filter := StateFilter{Transition: &Transition{Dim: "cross", From: []signalstate.State{"sell"}, To: "buy", WithinDays: 5}}
	s := Store{DB: db.DB()}
	out, err := s.Screen(context.Background(), []string{"edge", "gap", "steady"}, "20240107", filter)
	if err != nil || len(out.Results) != 1 || out.Results[0].Code != "edge" {
		t.Fatalf("%+v %v", out, err)
	}
	filter.Transition.WithinDays = 4
	out, err = s.Screen(context.Background(), []string{"edge"}, "20240107", filter)
	if err != nil || len(out.Results) != 0 {
		t.Fatalf("out of window: %+v %v", out, err)
	}
	// No future sell->buy observation is visible in the January 1 query.
	filter.Transition.WithinDays = 5
	out, err = s.Screen(context.Background(), []string{"edge"}, "20240101", filter)
	if err != nil || len(out.Results) != 0 {
		t.Fatalf("lookahead: %+v %v", out, err)
	}
}

func TestScreenCoverageReasons(t *testing.T) {
	db, err := storage.New(storage.Config{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	put := func(code string, dims int, insufficient bool) {
		t.Helper()
		for i, d := range signalstate.Dims() {
			if i >= dims {
				continue
			}
			state := signalstate.StateNeutral
			if insufficient {
				state = signalstate.StateInsufficient
			}
			if _, err := db.DB().Exec(`INSERT INTO signal_state_daily VALUES(?,'20240103',9,?,?,?,'raw','test','hash')`, code, d, state, insufficient); err != nil {
				t.Fatal(err)
			}
		}
	}
	put("a", 4, false) // 已参与筛选
	put("b", 4, true)  // 有状态行但四维不完整
	put("e", 3, false) // 缺一维
	// c 当日有行情但状态未生成；d 当日无行情（停牌/未上市/已退市）。
	for _, code := range []string{"a", "b", "c", "e"} {
		if _, err := db.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount) VALUES(?,9,'20240103',1,1,1,1,1,1)`, code); err != nil {
			t.Fatal(err)
		}
	}
	pool := []string{"a", "b", "c", "d", "e", "a"}
	out, err := (Store{DB: db.DB()}).Screen(context.Background(), pool, "20240103", StateFilter{Conditions: []Condition{}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Coverage.Total != 5 || out.Coverage.Covered != 1 || !out.Coverage.TradeDateExists || out.Coverage.FirstDate != "20240103" || out.Coverage.LastDate != "20240103" {
		t.Fatalf("coverage=%+v", out.Coverage)
	}
	reasons := map[string]string{}
	for _, item := range out.Coverage.Skipped {
		reasons[item.Code] = item.Reason
	}
	want := map[string]string{"b": SkipInsufficientHistory, "c": SkipNotComputed, "d": SkipNoQuote, "e": SkipInsufficientHistory}
	if len(reasons) != len(want) {
		t.Fatalf("skipped=%+v", out.Coverage.Skipped)
	}
	for code, reason := range want {
		if reasons[code] != reason {
			t.Fatalf("%s: got %q want %q (all=%+v)", code, reasons[code], reason, out.Coverage.Skipped)
		}
	}

	// 该日整库没有状态（周末/未来日）：不逐只归类，覆盖为 0。
	out, err = (Store{DB: db.DB()}).Screen(context.Background(), []string{"a", "b"}, "20240106", StateFilter{Conditions: []Condition{}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Coverage.TradeDateExists || out.Coverage.Total != 2 || out.Coverage.Covered != 0 || len(out.Coverage.Skipped) != 0 || out.MissingCount != 2 {
		t.Fatalf("weekend coverage=%+v missing=%d", out.Coverage, out.MissingCount)
	}
	if out.Coverage.FirstDate != "20240103" || out.Coverage.LastDate != "20240103" {
		t.Fatalf("range=%+v", out.Coverage)
	}
}
