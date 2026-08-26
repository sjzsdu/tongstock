package backtest

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sjzsdu/tongstock/internal/experiment"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// TestPersistedExperimentAgainstRealDatabase copies only selected real rows
// into an isolated temporary database, then verifies snapshot, split, execution,
// persistence and deterministic rerun without mutating the user's database.
func TestPersistedExperimentAgainstRealDatabase(t *testing.T) {
	path := os.Getenv("TONGSTOCK_REAL_DB")
	if path == "" {
		t.Skip("set TONGSTOCK_REAL_DB to run against the local real-data database")
	}
	source, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	var code string
	err = source.QueryRow(`
		SELECT code
		FROM kline
		WHERE ktype = 9 AND length(code) = 6 AND code <> '999999'
			AND open > 0 AND high > 0 AND low > 0 AND close > 0 AND close < 10000 AND volume > 0
			AND length(REPLACE(date, '-', '')) = 8
			AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		GROUP BY code
		HAVING COUNT(*) >= 180
		ORDER BY MAX(REPLACE(date, '-', '')) DESC, COUNT(*) DESC
		LIMIT 1`).Scan(&code)
	if err != nil {
		t.Fatalf("select real daily K-line series: %v", err)
	}
	rows, err := source.Query(`
		SELECT date, open, high, low, close, volume, amount
		FROM (
			SELECT date, open, high, low, close, volume, amount
			FROM kline
			WHERE code = ? AND ktype = 9
				AND open > 0 AND high > 0 AND low > 0 AND close > 0 AND close < 10000 AND volume > 0
				AND length(REPLACE(date, '-', '')) = 8
				AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
			ORDER BY REPLACE(date, '-', '') DESC
			LIMIT 180
		)
		ORDER BY REPLACE(date, '-', '')`, code)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	type realRow struct {
		date                   string
		open, high, low, close float64
		volume, amount         float64
	}
	var realRows []realRow
	for rows.Next() {
		var row realRow
		if err := rows.Scan(&row.date, &row.open, &row.high, &row.low,
			&row.close, &row.volume, &row.amount); err != nil {
			t.Fatal(err)
		}
		realRows = append(realRows, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(realRows) != 180 {
		t.Fatalf("real rows = %d, want 180", len(realRows))
	}

	tempPath := filepath.Join(t.TempDir(), "real-experiment.db")
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: tempPath})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range realRows {
		if _, err := store.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`,
			code, strings.ReplaceAll(row.date, "-", ""), row.open, row.high,
			row.low, row.close, row.volume, row.amount); err != nil {
			store.Close()
			t.Fatal(err)
		}
	}
	start, err := time.Parse("20060102", strings.ReplaceAll(realRows[0].date, "-", ""))
	if err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse("20060102", strings.ReplaceAll(realRows[len(realRows)-1].date, "-", ""))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &paradigm.DatasetSnapshot{
		ID: "snapshot-real-integration", Version: "v1", Universe: []string{code},
		DateRange: paradigm.DateRange{
			Start: start.Format("20060102"), End: end.Format("20060102"),
		},
		Market: "A", PriceAdjustment: paradigm.PriceRaw,
	}
	snapshotStore := paradigm.NewDatasetSnapshotStore(store)
	if err := snapshotStore.CreateKlineSnapshot(snapshot, 9); err != nil {
		store.Close()
		t.Fatal(err)
	}
	registry, err := experiment.NewSQLiteRegistry(store)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	config := experimentConfig(snapshot.ID)
	config.Board = string(BoardForCode(code))
	exp := experiment.NewExperiment("real-db-integration", "real rows only", config)
	exp.ID = "exp-real-integration"
	if err := registry.Create(exp); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := snapshotStore.BindExperiment(exp.ID, []string{snapshot.ID}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	executor := &ParadigmExperimentExecutor{
		SnapshotStore: snapshotStore,
		Paradigm: &paradigms.Paradigm{
			ID: "p-real-integration", StockCode: code,
			BuyConds: []paradigms.Condition{
				{Indicator: "close", Operator: "gt", Value: "0"},
			},
		},
	}
	runner := experiment.NewExperimentRunner(registry)
	first, err := runner.Run(context.Background(), exp, executor)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	second, err := runner.Run(context.Background(), exp, executor)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if first.ResultHash == "" || first.ResultHash != second.ResultHash {
		store.Close()
		t.Fatalf("real-data rerun hash mismatch: %s vs %s", first.ResultHash, second.ResultHash)
	}
	runID := second.ID
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := storage.New(storage.Config{Driver: "sqlite3", DSN: tempPath})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reloadedRegistry, err := experiment.NewSQLiteRegistry(reopened)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedRegistry.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ResultHash != second.ResultHash || len(reloaded.Artifacts) != 5 {
		t.Fatalf("reloaded real-data run incomplete: %+v", reloaded)
	}
	t.Logf("verified code=%s rows=%d range=%s..%s result_hash=%s artifacts=%d",
		code, len(realRows), start.Format("2006-01-02"), end.Format("2006-01-02"),
		reloaded.ResultHash, len(reloaded.Artifacts))
}
