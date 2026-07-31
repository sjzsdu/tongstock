package backtest

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/trading"
)

// TestRunParadigmAgainstRealDatabase is opt-in because it reads the user's
// local TongStock database. It never creates or substitutes market rows.
func TestRunParadigmAgainstRealDatabase(t *testing.T) {
	path := os.Getenv("TONGSTOCK_REAL_DB")
	if path == "" {
		t.Skip("set TONGSTOCK_REAL_DB to run against the local real-data database")
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var code string
	var ktype int
	err = db.QueryRow(`
		SELECT code, ktype
		FROM kline
		WHERE length(code) = 6 AND code <> '999999'
			AND open > 0 AND high > 0 AND low > 0 AND close > 0 AND close < 10000 AND volume > 0
			AND length(REPLACE(date, '-', '')) = 8
			AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		GROUP BY code, ktype
		HAVING COUNT(*) >= 40
		ORDER BY MAX(REPLACE(date, '-', '')) DESC, COUNT(*) DESC
		LIMIT 1`).Scan(&code, &ktype)
	if err != nil {
		t.Fatalf("select real K-line series: %v", err)
	}

	rows, err := db.Query(`
		SELECT date, open, high, low, close, volume, amount
		FROM (
			SELECT date, open, high, low, close, volume, amount
			FROM kline
			WHERE code = ? AND ktype = ?
				AND open > 0 AND high > 0 AND low > 0 AND close > 0 AND close < 10000 AND volume > 0
				AND length(REPLACE(date, '-', '')) = 8
				AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
			ORDER BY REPLACE(date, '-', '') DESC
			LIMIT 40
		)
		ORDER BY REPLACE(date, '-', '')`, code, ktype)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var bars []MarketBar
	for rows.Next() {
		var date string
		var bar MarketBar
		if err := rows.Scan(&date, &bar.Open, &bar.High, &bar.Low, &bar.Close, &bar.Volume, &bar.Amount); err != nil {
			t.Fatal(err)
		}
		bar.Date, err = time.Parse("20060102", strings.ReplaceAll(date, "-", ""))
		if err != nil {
			t.Fatalf("parse real K-line date %q: %v", date, err)
		}
		bar.Code = code
		bar.Board = boardForCode(code)
		bars = append(bars, bar)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(bars) != 40 {
		t.Fatalf("real K-line row count = %d, want 40", len(bars))
	}

	p := &paradigms.Paradigm{
		ID: "real-db-smoke",
		BuyConds: []paradigms.Condition{
			{Indicator: "close", Operator: "gt", Value: "0"},
		},
	}
	result, err := RunParadigm(context.Background(), p, bars, DefaultParadigmExecutionConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 1 {
		t.Fatalf("real-data fills = %d, want 1: %+v", len(result.Fills), result.Rejections)
	}
	fill := result.Fills[0]
	executionIndex := -1
	for i := range bars {
		if bars[i].Date == fill.ExecutionDate {
			executionIndex = i
			break
		}
	}
	if executionIndex < 1 {
		t.Fatalf("fill execution date is not a non-first real market bar: %+v", fill)
	}
	signalBar, executionBar := bars[executionIndex-1], bars[executionIndex]
	if fill.SignalPrice != signalBar.Close {
		t.Fatalf("real-data signal price %.2f, want previous real close %.2f", fill.SignalPrice, signalBar.Close)
	}
	wantPrice := roundCents(executionBar.Open * (1 + DefaultParadigmExecutionConfig().CostModel.SlippageBps/10000))
	if fill.Price != wantPrice {
		t.Fatalf("real-data execution price %.2f, want next real open %.2f with slippage = %.2f",
			fill.Price, executionBar.Open, wantPrice)
	}
	t.Logf("verified code=%s ktype=%d range=%s..%s signal=%s execution=%s open=%.2f fill=%.2f",
		code, ktype, bars[0].Date.Format("2006-01-02"), bars[len(bars)-1].Date.Format("2006-01-02"),
		signalBar.Date.Format("2006-01-02"), fill.ExecutionDate.Format("2006-01-02"), executionBar.Open, fill.Price)
}

func boardForCode(code string) trading.Board {
	switch {
	case strings.HasPrefix(code, "300"), strings.HasPrefix(code, "301"):
		return trading.BoardChiNext
	case strings.HasPrefix(code, "688"), strings.HasPrefix(code, "689"):
		return trading.BoardSTAR
	case strings.HasPrefix(code, "4"), strings.HasPrefix(code, "8"), strings.HasPrefix(code, "9"):
		return trading.BoardBJ
	default:
		return trading.BoardMain
	}
}

func roundCents(value float64) float64 {
	return float64(int64(value*100+0.5)) / 100
}
