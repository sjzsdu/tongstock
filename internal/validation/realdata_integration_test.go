package validation_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sjzsdu/tongstock/internal/adapter/validationrepo"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// TestValidationFactoryAgainstRealDatabase 只使用本地真实 A 股日线。
// 它把最小必要数据复制到隔离库，验证冻结、回测、critic、
// EvidenceBundle 以及同快照重跑哈希，不会修改用户的行情库。
func TestValidationFactoryAgainstRealDatabase(t *testing.T) {
	path := os.Getenv("TONGSTOCK_REAL_DB")
	if path == "" {
		t.Skip("set TONGSTOCK_REAL_DB to run the real-data validation integration test")
	}
	source, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	var code string
	err = source.QueryRow(`SELECT code FROM kline
		WHERE ktype = 9 AND length(code) = 6 AND code <> '999999'
		AND open > 0 AND high > 0 AND low > 0 AND close BETWEEN 1 AND 100 AND volume > 0
		AND length(REPLACE(date, '-', '')) = 8
		AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		GROUP BY code HAVING COUNT(*) >= 360
		ORDER BY MAX(REPLACE(date, '-', '')) DESC, COUNT(*) DESC LIMIT 1`).Scan(&code)
	if err != nil {
		t.Fatalf("select real A-share series: %v", err)
	}

	rows, err := source.Query(`SELECT date, open, high, low, close, volume, amount FROM (
		SELECT date, open, high, low, close, volume, amount FROM kline
		WHERE code = ? AND ktype = 9 AND open > 0 AND high > 0 AND low > 0
		AND close BETWEEN 1 AND 100 AND volume > 0
		AND length(REPLACE(date, '-', '')) = 8
		AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		ORDER BY REPLACE(date, '-', '') DESC LIMIT 360)
		ORDER BY REPLACE(date, '-', '')`, code)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	type realBar struct {
		date                   string
		open, high, low, close float64
		volume, amount         float64
	}
	realBars := make([]realBar, 0, 360)
	for rows.Next() {
		var bar realBar
		if err := rows.Scan(&bar.date, &bar.open, &bar.high, &bar.low, &bar.close, &bar.volume, &bar.amount); err != nil {
			t.Fatal(err)
		}
		realBars = append(realBars, bar)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(realBars) != 360 {
		t.Fatalf("real bars=%d, want 360", len(realBars))
	}

	tempStore, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "validation-real.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer tempStore.Close()
	for _, bar := range realBars {
		_, err := tempStore.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`, code, strings.ReplaceAll(bar.date, "-", ""),
			bar.open, bar.high, bar.low, bar.close, bar.volume, bar.amount)
		if err != nil {
			t.Fatal(err)
		}
	}
	start := parseRealDate(t, realBars[0].date)
	end := parseRealDate(t, realBars[len(realBars)-1].date)
	snapshot := &paradigm.DatasetSnapshot{
		ID: "validation-real-integration", Version: "v1", Universe: []string{code},
		DateRange: paradigm.DateRange{Start: start.Format("2006-01-02"), End: end.Format("2006-01-02")},
		Market:    "A", PriceAdjustment: paradigm.PriceRaw,
	}
	snapshotStore := paradigm.NewDatasetSnapshotStore(tempStore)
	if err := snapshotStore.CreateKlineSnapshot(snapshot, validationrepo.KlineTypeDaily); err != nil {
		t.Fatal(err)
	}
	method, _, err := methods.Compile(methods.DemoBreakout())
	if err != nil {
		t.Fatal(err)
	}
	factory, err := validation.NewFactory(validation.FactoryDeps{
		Method: method, Bars: validationrepo.New(tempStore), Benchmark: validationrepo.NewBenchmark(tempStore),
	})
	if err != nil {
		t.Fatal(err)
	}
	job := validation.ValidationJob{
		MethodHash: method.ContentHash, MethodName: method.Name, SnapshotID: snapshot.ID,
		StockCode: code, DateStart: snapshot.DateRange.Start, DateEnd: snapshot.DateRange.End,
		SplitType: "fixed", DiscoveryTrials: 3, InitialCash: 1_000_000,
	}
	first, err := factory.Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	second, err := factory.Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if first.ResultHash == "" || first.ResultHash != second.ResultHash {
		t.Fatalf("real-data result is not reproducible: %s vs %s", first.ResultHash, second.ResultHash)
	}
	if first.SnapshotID != snapshot.ID || len(first.Segments) < 3 || first.BonferroniAlpha == 0 {
		t.Fatalf("incomplete evidence bundle: %+v", first)
	}
	t.Logf("validated real code=%s bars=%d range=%s..%s hash=%s confidence=%s",
		code, len(realBars), snapshot.DateRange.Start, snapshot.DateRange.End, first.ResultHash, first.Confidence)
}

func parseRealDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("20060102", strings.ReplaceAll(value, "-", ""))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
