package statesnapshot

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/signalstate"
)

// Optional acceptance check against an already-backfilled database. Always open
// it read-only: no migrations, backfills, refreshes or changes to real data.
func TestScreenRealSnapshotParity(t *testing.T) {
	dsn := os.Getenv("TONGSTOCK_SNAPSHOT_PARITY_DSN")
	if dsn == "" {
		t.Skip("set TONGSTOCK_SNAPSHOT_PARITY_DSN to a file: SQLite URI (mode=ro)")
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	store := Store{DB: db}
	var date string
	if err = db.QueryRow(`SELECT MAX(trade_date) FROM signal_state_daily WHERE ktype=9`).Scan(&date); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT DISTINCT code FROM signal_state_daily WHERE ktype=9 AND trade_date=? ORDER BY code`, date)
	if err != nil {
		t.Fatal(err)
	}
	codes := []string{}
	for rows.Next() {
		var code string
		if err = rows.Scan(&code); err != nil {
			t.Fatal(err)
		}
		codes = append(codes, code)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, tc := range []struct {
		name string
		f    StateFilter
		sql  string
	}{
		{"cross_buy_and_ma_buy", StateFilter{Conditions: []Condition{{Dim: "cross", State: "buy"}, {Dim: "ma", State: "buy"}}}, ` AND MAX(CASE WHEN dim='cross' THEN state END)='buy' AND MAX(CASE WHEN dim='ma' THEN state END)='buy'`},
		{"explicit_neutral", StateFilter{Conditions: []Condition{{Dim: "cross", State: "buy"}, {Dim: "ma", State: "buy"}, {Dim: "osc", State: "neutral"}}}, ` AND MAX(CASE WHEN dim='cross' THEN state END)='buy' AND MAX(CASE WHEN dim='ma' THEN state END)='buy' AND MAX(CASE WHEN dim='osc' THEN state END)='neutral'`},
		{"sell_to_buy_5_days", StateFilter{Transition: &Transition{Dim: "cross", From: []signalstate.State{"sell"}, To: "buy", WithinDays: 5}}, ` AND code IN (SELECT code FROM (SELECT code,trade_date,state,insufficient,LAG(state) OVER(PARTITION BY code ORDER BY trade_date) AS prev,LAG(insufficient) OVER(PARTITION BY code ORDER BY trade_date) AS prev_bad FROM signal_state_daily WHERE ktype=9 AND dim='cross' AND trade_date<=?) WHERE trade_date>=? AND state='buy' AND prev='sell' AND insufficient=0 AND prev_bad=0)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := time.Now()
			out, err := store.Screen(ctx, codes, date, tc.f)
			if err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(started)
			query := `SELECT code FROM signal_state_daily WHERE ktype=9 AND trade_date=? GROUP BY code HAVING COUNT(*)=4 AND MAX(insufficient)=0` + tc.sql + ` ORDER BY code`
			args := []any{date}
			if tc.f.Transition != nil {
				d, _ := time.Parse("20060102", date)
				args = append(args, date, d.AddDate(0, 0, -4).Format("20060102"))
			}
			rows, err := db.QueryContext(ctx, query, args...)
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
			if len(expected) != len(out.Results) {
				t.Fatalf("hits %d != SQL %d", len(out.Results), len(expected))
			}
			for i, r := range out.Results {
				if r.Code != expected[i] {
					t.Fatalf("row %d %s != SQL %s", i, r.Code, expected[i])
				}
			}
			t.Logf("date=%s pool=%d hits=%d insufficient=%d query=%s SQL parity=exact", date, len(codes), len(out.Results), out.InsufficientCount, elapsed)
		})
	}
}
