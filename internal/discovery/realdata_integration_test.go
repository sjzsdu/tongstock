package discovery_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sjzsdu/tongstock/internal/adapter/discoveryrepo"
	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestResearcherDiscoversAcrossMultipleRealStocks(t *testing.T) {
	path := os.Getenv("TONGSTOCK_REAL_DB")
	if path == "" {
		t.Skip("set TONGSTOCK_REAL_DB to run real-data discovery integration")
	}
	source, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	rows, err := source.Query(`SELECT code FROM kline
		WHERE ktype = 9 AND length(code) = 6 AND code <> '999999'
		AND open > 0 AND high > 0 AND low > 0 AND close BETWEEN 1 AND 100 AND volume > 0
		AND length(REPLACE(date, '-', '')) = 8
		AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		GROUP BY code HAVING COUNT(*) >= 360
		ORDER BY MAX(REPLACE(date, '-', '')) DESC, code LIMIT 3`)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			t.Fatal(err)
		}
		codes = append(codes, code)
	}
	rows.Close()
	if len(codes) != 3 {
		t.Fatalf("real stock count=%d, want 3", len(codes))
	}

	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "discovery-real.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	firstDate, lastDate := "99999999", ""
	for _, code := range codes {
		bars, err := source.Query(`SELECT date, open, high, low, close, volume, amount FROM (
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
		count := 0
		for bars.Next() {
			var date string
			var open, high, low, close, volume, amount float64
			if err := bars.Scan(&date, &open, &high, &low, &close, &volume, &amount); err != nil {
				t.Fatal(err)
			}
			date = strings.ReplaceAll(date, "-", "")
			if date < firstDate {
				firstDate = date
			}
			if date > lastDate {
				lastDate = date
			}
			if _, err := store.DB().Exec(`INSERT INTO kline
				(code, ktype, date, open, high, low, close, volume, amount)
				VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`, code, date, open, high, low, close, volume, amount); err != nil {
				t.Fatal(err)
			}
			count++
		}
		bars.Close()
		if count != 360 {
			t.Fatalf("%s rows=%d, want 360", code, count)
		}
	}
	sort.Strings(codes)
	snapshot := &paradigm.DatasetSnapshot{
		ID: "discovery-real-multi", Version: "v1", Universe: codes,
		DateRange: paradigm.DateRange{Start: firstDate, End: lastDate},
		Market:    "A", PriceAdjustment: paradigm.PriceRaw,
	}
	if err := paradigm.NewDatasetSnapshotStore(store).CreateKlineSnapshot(snapshot, 9); err != nil {
		t.Fatal(err)
	}
	researcher, err := discovery.NewResearcher(discoveryrepo.New(store))
	if err != nil {
		t.Fatal(err)
	}
	request := discovery.Request{SnapshotID: snapshot.ID, StockCodes: codes}
	first, err := researcher.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := researcher.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ResultHash == "" || first.ResultHash != second.ResultHash {
		t.Fatalf("discovery is not reproducible: %s vs %s", first.ResultHash, second.ResultHash)
	}
	if first.DiscoveryTrials == 0 || len(first.Boundaries) != 3 {
		t.Fatalf("incomplete research trace: %+v", first)
	}
	for _, candidate := range first.Candidates {
		if candidate.Method == nil || !candidate.Method.IsExecutable() || candidate.Source == "" {
			t.Fatalf("candidate is not executable/traceable: %+v", candidate)
		}
	}
	t.Logf("real multi-stock discovery codes=%v trials=%d candidates=%d conclusion=%s hash=%s",
		codes, first.DiscoveryTrials, len(first.Candidates), first.Conclusion, first.ResultHash)

	// 同一真实库中上市历史较短的股票必须明确拒绝，不用补值凑样本。
	var shortCode string
	err = source.QueryRow(`SELECT code FROM kline
		WHERE ktype = 9 AND length(code) = 6 AND open > 0 AND high > 0 AND low > 0 AND close > 0 AND volume > 0
		AND length(REPLACE(date, '-', '')) = 8
		AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		GROUP BY code HAVING COUNT(*) BETWEEN 30 AND 150
		ORDER BY COUNT(*) DESC, code LIMIT 1`).Scan(&shortCode)
	if err != nil {
		t.Fatalf("select short real series: %v", err)
	}
	shortRows, err := source.Query(`SELECT date, open, high, low, close, volume, amount FROM kline
		WHERE code = ? AND ktype = 9 AND open > 0 AND high > 0 AND low > 0 AND close > 0 AND volume > 0
		AND length(REPLACE(date, '-', '')) = 8
		AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		ORDER BY REPLACE(date, '-', '')`, shortCode)
	if err != nil {
		t.Fatal(err)
	}
	shortFirst, shortLast, shortCount := "", "", 0
	for shortRows.Next() {
		var date string
		var open, high, low, close, volume, amount float64
		if err := shortRows.Scan(&date, &open, &high, &low, &close, &volume, &amount); err != nil {
			t.Fatal(err)
		}
		date = strings.ReplaceAll(date, "-", "")
		if shortFirst == "" {
			shortFirst = date
		}
		shortLast = date
		if _, err := store.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`, shortCode, date, open, high, low, close, volume, amount); err != nil {
			t.Fatal(err)
		}
		shortCount++
	}
	shortRows.Close()
	shortSnapshot := &paradigm.DatasetSnapshot{
		ID: "discovery-real-insufficient", Version: "v1", Universe: []string{shortCode},
		DateRange: paradigm.DateRange{Start: shortFirst, End: shortLast},
		Market:    "A", PriceAdjustment: paradigm.PriceRaw,
	}
	if err := paradigm.NewDatasetSnapshotStore(store).CreateKlineSnapshot(shortSnapshot, 9); err != nil {
		t.Fatal(err)
	}
	if _, err := researcher.Run(context.Background(), discovery.Request{
		SnapshotID: shortSnapshot.ID, StockCodes: []string{shortCode},
	}); err == nil {
		t.Fatalf("short real series %s with %d bars must be rejected", shortCode, shortCount)
	}
}
