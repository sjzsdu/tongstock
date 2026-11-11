package statesnapshot

import (
	"context"
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"math"
	"testing"
	"time"
)

func fixture(t *testing.T) (Store, *storage.Storage, []ta.KlineInput) {
	t.Helper()
	db, err := storage.New(storage.Config{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var bars []ta.KlineInput
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tx, err := db.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 90; i++ {
		// Gaps model suspensions/weekends: only actual bars get states.
		price := 10 + math.Sin(float64(i)*0.6) + float64(i)*0.02
		b := ta.KlineInput{Time: start.AddDate(0, 0, i*2), Open: price, High: price + 1, Low: price - 1, Close: price, Volume: 100, Amount: 1000}
		bars = append(bars, b)
		_, err = storage.AppendKline(context.Background(), tx, storage.KlineBar{Code: "000001", KType: Day, Date: b.Time.Format("20060102"), Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume, Amount: b.Amount}, "test", start)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return Store{db.DB()}, db, bars
}
func TestBackfillReferenceIdempotenceAndCausality(t *testing.T) {
	s, _, bars := fixture(t)
	ctx := context.Background()
	r, err := s.Run(ctx, Options{Mode: "full", Verify: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Changed != 360 || r.Verified != 360 {
		t.Fatalf("%+v", r)
	}
	rows, err := s.Series(ctx, "000001", "", "")
	if err != nil {
		t.Fatal(err)
	}
	def := param.ResolveStateDefinition("000001")
	for i, b := range bars {
		v := signalstate.Compute("000001", bars[:i+1], def)
		// Compute uses unspecified KType=0; series with that same KType must hash identically.
		if got := signalstate.ComputeSeries("000001", 0, bars, def)[i].CalcHash; got != v.CalcHash {
			t.Fatalf("future changed %d", i)
		}
		v = signalstate.ComputeSeries("000001", Day, bars[:i+1], def)[i]
		for _, row := range rows[i*4 : i*4+4] {
			if row.Date != b.Time.Format("20060102") || row.CalcHash != v.CalcHash || row.State != v.State(row.Dim) {
				t.Fatalf("row mismatch: %+v", row)
			}
		}
	}
	for _, mode := range []string{"full", "incremental"} {
		r, err = s.Run(ctx, Options{Mode: mode, Verify: true})
		if err != nil || r.Changed != 0 {
			t.Fatalf("rerun %s: %+v %v", mode, r, err)
		}
	}
	if !rows[0].Insufficient || rows[0].State != signalstate.StateInsufficient {
		t.Fatal("warmup lost")
	}
}
func TestIncrementalGapsCorrectionsAndWithdrawals(t *testing.T) {
	s, db, bars := fixture(t)
	ctx := context.Background()
	if _, err := s.Run(ctx, Options{Mode: "full"}); err != nil {
		t.Fatal(err)
	}
	// Losing one dimension of an old day is repaired without a watermark gap.
	if _, err := db.DB().Exec(`DELETE FROM signal_state_daily WHERE trade_date=? AND dim='ma'`, bars[10].Time.Format("20060102")); err != nil {
		t.Fatal(err)
	}
	r, err := s.Run(ctx, Options{Mode: "incremental", Verify: true})
	if err != nil || r.Changed != 1 {
		t.Fatalf("repair %+v %v", r, err)
	}
	// Consecutive daily runs append only their four new rows, even across gaps.
	for i := 0; i < 5; i++ {
		date := bars[len(bars)-1].Time.AddDate(0, 0, 2+i*2).Format("20060102")
		if _, err := db.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount) VALUES('000001',9,?,10,11,9,10,100,1000)`, date); err != nil {
			t.Fatal(err)
		}
		r, err = s.Run(ctx, Options{Mode: "incremental", Verify: true})
		if err != nil || r.Changed != 4 {
			t.Fatalf("day %d: %+v %v", i, r, err)
		}
	}
	before, _ := s.Series(ctx, "000001", "", "")
	tx, _ := db.DB().Begin()
	b := bars[12]
	if _, err := storage.AppendKline(ctx, tx, storage.KlineBar{Code: "000001", KType: Day, Date: b.Time.Format("20060102"), Open: b.Open, High: b.High + 1, Low: b.Low, Close: b.Close, Volume: b.Volume, Amount: b.Amount}, "correction", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	r, err = s.Run(ctx, Options{Mode: "incremental", Verify: true})
	if err != nil || r.Changed == 0 {
		t.Fatalf("revision %+v %v", r, err)
	}
	after, _ := s.Series(ctx, "000001", "", "")
	if before[0].CalcHash != after[0].CalcHash || before[12*4].CalcHash == after[12*4].CalcHash {
		t.Fatal("prefix traceability broken")
	}
	if _, err := db.DB().Exec(`DELETE FROM kline WHERE date=?`, bars[5].Time.Format("20060102")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Run(ctx, Options{Mode: "incremental", Verify: true}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.At(ctx, bars[5].Time.Format("20060102"), nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("withdrawn: %v %v", rows, err)
	}
}
func TestAllMarketNoCapAndFromSeed(t *testing.T) {
	s, db, bars := fixture(t)
	ctx := context.Background()
	tx, _ := db.DB().Begin()
	for i := 0; i < 501; i++ {
		if _, err := tx.Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount) VALUES(?,9,'20240101',10,11,9,10,100,1000)`, fmt.Sprintf("%06d", i+600000)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	r, err := s.Run(ctx, Options{Mode: "full", Verify: true})
	if err != nil || r.Codes != 502 {
		t.Fatalf("market cap %+v %v", r, err)
	}
	from := bars[70].Time.Format("20060102")
	if _, err = db.DB().Exec(`DELETE FROM signal_state_daily WHERE code='000001'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Run(ctx, Options{Codes: []string{"000001"}, From: from, Mode: "full", Verify: true}); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Series(ctx, "000001", "", "")
	if len(rows) != 20*4 || rows[0].Date != from {
		t.Fatalf("from: %d", len(rows))
	}
	v := signalstate.ComputeSeries("000001", Day, bars, param.ResolveStateDefinition("000001"))[70]
	if rows[0].CalcHash != v.CalcHash {
		t.Fatal("from reset warmup")
	}
	// A subsequent increment heals the deliberately omitted prefix as well.
	if _, err = s.Run(ctx, Options{Codes: []string{"000001"}, Mode: "incremental", Verify: true}); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Series(ctx, "000001", "", "")
	if len(rows) != 360 {
		t.Fatal("prefix gap")
	}
}

func TestDefinitionChangeRebuildsEntireHistoryAndCancellationRollsBack(t *testing.T) {
	s, db, bars := fixture(t)
	ctx := context.Background()
	if _, err := s.Run(ctx, Options{Mode: "full"}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Series(ctx, "000001", "", "")
	// Simulate an older recorded definition. Even with --from, a parameter
	// migration must not bless the old prefix as if it used the new definition.
	if _, err := db.DB().Exec(`UPDATE signal_state_seed SET definition='{}'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE signal_state_daily SET calc_hash='old'`); err != nil {
		t.Fatal(err)
	}
	r, err := s.Run(ctx, Options{Mode: "full", From: bars[70].Time.Format("20060102"), Verify: true})
	if err != nil || r.Changed != 360 {
		t.Fatalf("definition rebuild %+v %v", r, err)
	}
	after, _ := s.Series(ctx, "000001", "", "")
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("definition replay row %d", i)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.Run(canceled, Options{Codes: []string{"000001"}, Mode: "full"}); err == nil {
		t.Fatal("canceled run succeeded")
	}
	after, _ = s.Series(ctx, "000001", "", "")
	if len(after) != 360 {
		t.Fatal("cancel changed coverage")
	}
}

func TestIncrementalSuffixUsesExactReferenceSeed(t *testing.T) {
	s, _, bars := fixture(t)
	def := param.ResolveStateDefinition("000001")
	full := signalstate.ComputeSeries("000001", Day, bars, def)
	for _, start := range []int{0, 1, 34, 70, 89, 90} {
		suffix := signalstate.ComputeSeriesFrom("000001", Day, bars, def, start)
		if len(suffix) != len(full)-start {
			t.Fatal("wrong suffix length")
		}
		for i, v := range suffix {
			if v.CalcHash != full[i+start].CalcHash {
				t.Fatalf("suffix drift at %d", i+start)
			}
		}
	}
	if _, err := s.Run(context.Background(), Options{Mode: "full"}); err != nil {
		t.Fatal(err)
	}
}

func TestAllMarketRemovesACompletelyWithdrawnSymbol(t *testing.T) {
	s, db, _ := fixture(t)
	ctx := context.Background()
	if _, err := s.Run(ctx, Options{Mode: "full"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DELETE FROM kline`); err != nil {
		t.Fatal(err)
	}
	r, err := s.Run(ctx, Options{Mode: "incremental", Verify: true})
	if err != nil || r.Changed != 360 {
		t.Fatalf("withdrawal %+v %v", r, err)
	}
	rows, err := s.Series(ctx, "000001", "", "")
	if err != nil || len(rows) != 0 {
		t.Fatalf("phantom state rows: %d %v", len(rows), err)
	}
}
