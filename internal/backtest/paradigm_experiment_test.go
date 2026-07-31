package backtest

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/experiment"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestPersistedFixedParadigmExperimentIsReproducible(t *testing.T) {
	store, snapshotStore, snapshot, dates := createFrozenExperimentFixture(t, 120)
	defer store.Close()
	registry, err := experiment.NewSQLiteRegistry(store)
	if err != nil {
		t.Fatal(err)
	}
	exp := experiment.NewExperiment("fixed-real-bars", "fixed split", experimentConfig(snapshot.ID))
	exp.ID = "exp-fixed"
	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}
	if err := snapshotStore.BindExperiment(exp.ID, []string{snapshot.ID}); err != nil {
		t.Fatal(err)
	}
	executor := &ParadigmExperimentExecutor{
		SnapshotStore: snapshotStore,
		Paradigm:      testExperimentParadigm(),
	}
	runner := experiment.NewExperimentRunner(registry)
	first, err := runner.Run(context.Background(), exp, executor)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.Run(context.Background(), exp, executor)
	if err != nil {
		t.Fatal(err)
	}
	if first.ResultHash == "" || first.ResultHash != second.ResultHash {
		t.Fatalf("result hash is not reproducible: %q vs %q", first.ResultHash, second.ResultHash)
	}
	firstTransactions := artifactByName(t, first.Artifacts, "transactions")
	secondTransactions := artifactByName(t, second.Artifacts, "transactions")
	if firstTransactions.ContentHash != secondTransactions.ContentHash {
		t.Fatalf("transaction hashes differ: %s vs %s",
			firstTransactions.ContentHash, secondTransactions.ContentHash)
	}

	split := artifactByName(t, first.Artifacts, "time_split")
	var splitInfo splitArtifact
	if err := json.Unmarshal(split.Content, &splitInfo); err != nil {
		t.Fatal(err)
	}
	if splitInfo.Fixed == nil {
		t.Fatal("fixed split artifact missing")
	}
	trainBoundary := int(float64(len(dates)) * 0.6)
	validBoundary := trainBoundary + int(float64(len(dates))*0.2)
	if splitInfo.Fixed.Train.End != dates[trainBoundary-2-1] {
		t.Fatalf("purged train end = %s, want %s",
			splitInfo.Fixed.Train.End, dates[trainBoundary-3])
	}
	if splitInfo.Fixed.Valid.Start != dates[trainBoundary+2] {
		t.Fatalf("embargoed valid start = %s, want %s",
			splitInfo.Fixed.Valid.Start, dates[trainBoundary+2])
	}
	if splitInfo.Fixed.Test.Start != dates[validBoundary+2] {
		t.Fatalf("embargoed test start = %s, want %s",
			splitInfo.Fixed.Test.Start, dates[validBoundary+2])
	}
	if len(splitInfo.Fixed.PurgeDates) != 4 {
		t.Fatalf("purge dates = %d, want 4", len(splitInfo.Fixed.PurgeDates))
	}

	manifestArtifact := artifactByName(t, first.Artifacts, "reproducibility_manifest")
	var manifest executionManifest
	if err := json.Unmarshal(manifestArtifact.Content, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ExcludedSegment != SegmentTest ||
		len(manifest.SelectionSegments) != 2 ||
		manifest.SelectionSegments[0] != SegmentTrain ||
		manifest.SelectionSegments[1] != SegmentValid {
		t.Fatalf("test isolation manifest is invalid: %+v", manifest)
	}
	if manifest.SnapshotHash != snapshot.ContentHash || manifest.TransactionHash == "" {
		t.Fatalf("snapshot/transaction lineage missing: %+v", manifest)
	}

	reloadedRegistry, err := experiment.NewSQLiteRegistry(store)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reloadedRegistry.ListRuns(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || len(runs[0].Artifacts) != 5 {
		t.Fatalf("persisted runs/artifacts incomplete: runs=%d artifacts=%d", len(runs), len(runs[0].Artifacts))
	}
}

func TestWalkForwardParadigmExperimentPersistsEveryWindow(t *testing.T) {
	store, snapshotStore, snapshot, _ := createFrozenExperimentFixture(t, 110)
	defer store.Close()
	registry, err := experiment.NewSQLiteRegistry(store)
	if err != nil {
		t.Fatal(err)
	}
	config := experimentConfig(snapshot.ID)
	config.SplitConfig = experiment.SplitConfigRef{
		Type: string(SplitWalkForward), Windows: 2, TrainWindowDays: 40,
		ValidWindowDays: 15, TestWindowDays: 15, StepDays: 20,
		EmbargoDays: 2, PurgeDays: 2,
	}
	exp := experiment.NewExperiment("walk-forward-real-bars", "walk forward", config)
	exp.ID = "exp-walk-forward"
	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}
	if err := snapshotStore.BindExperiment(exp.ID, []string{snapshot.ID}); err != nil {
		t.Fatal(err)
	}
	runner := experiment.NewExperimentRunner(registry)
	run, err := runner.Run(context.Background(), exp, &ParadigmExperimentExecutor{
		SnapshotStore: snapshotStore,
		Paradigm:      testExperimentParadigm(),
	})
	if err != nil {
		t.Fatal(err)
	}
	split := artifactByName(t, run.Artifacts, "time_split")
	var info splitArtifact
	if err := json.Unmarshal(split.Content, &info); err != nil {
		t.Fatal(err)
	}
	if info.WalkForward == nil || len(info.WalkForward.Windows) != 2 {
		t.Fatalf("walk-forward windows missing: %+v", info.WalkForward)
	}
	if len(info.Segments) != 6 {
		t.Fatalf("segments = %d, want 2 windows x 3", len(info.Segments))
	}
	if run.Metrics == nil || run.Metrics.Custom["segment_count"] != 2 {
		t.Fatalf("aggregate metrics must use two OOS test segments: %+v", run.Metrics)
	}
}

func createFrozenExperimentFixture(t *testing.T, count int) (*storage.Storage, *paradigm.DatasetSnapshotStore, *paradigm.DatasetSnapshot, []time.Time) {
	t.Helper()
	store, err := storage.New(storage.Config{
		Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "experiment.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	dates := weekdayDates(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), count)
	for i, date := range dates {
		closePrice := 10 + float64(i)*0.01
		if _, err := store.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`,
			"600001", date.Format("20060102"), closePrice-0.02, closePrice+0.05,
			closePrice-0.05, closePrice, 100000+float64(i), closePrice*100000); err != nil {
			store.Close()
			t.Fatal(err)
		}
	}
	snapshot := &paradigm.DatasetSnapshot{
		ID: "snapshot-experiment", Version: "v1", Universe: []string{"600001"},
		DateRange: paradigm.DateRange{
			Start: dates[0].Format("20060102"), End: dates[len(dates)-1].Format("20060102"),
		},
		Market: "A", PriceAdjustment: paradigm.PriceRaw,
	}
	snapshotStore := paradigm.NewDatasetSnapshotStore(store)
	if err := snapshotStore.CreateKlineSnapshot(snapshot, 9); err != nil {
		store.Close()
		t.Fatal(err)
	}
	return store, snapshotStore, snapshot, dates
}

func experimentConfig(snapshotID string) experiment.ExperimentConfig {
	return experiment.ExperimentConfig{
		StrategyName: "paradigm", StrategyVersion: "1",
		DataSnapshotID: snapshotID, KType: 9, Board: "main",
		SplitConfig: experiment.SplitConfigRef{
			Type: string(SplitFixed), TrainRatio: 0.6, ValidRatio: 0.2,
			EmbargoDays: 2, PurgeDays: 2, MinTrainSize: 30,
		},
		InitialCash: 100000, CommissionRate: 0.00025, MinCommission: 5,
		StampDutyRate: 0.0005, TransferFeeRate: 0.00001, SlippageBps: 10,
		MaxPositionSize: 0.5, EnableT1: true, EnablePriceLimit: true,
		StrategyParams: map[string]interface{}{"source": "pre_registered"},
	}
}

func testExperimentParadigm() *paradigms.Paradigm {
	return &paradigms.Paradigm{
		ID: "p-real-experiment", StockCode: "600001",
		BuyConds: []paradigms.Condition{
			{Indicator: "close", Operator: "gt", Value: "0"},
		},
	}
}

func artifactByName(t *testing.T, artifacts []experiment.Artifact, name string) experiment.Artifact {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.Name == name {
			return artifact
		}
	}
	t.Fatalf("artifact %q missing from %+v", name, artifacts)
	return experiment.Artifact{}
}

func weekdayDates(start time.Time, count int) []time.Time {
	result := make([]time.Time, 0, count)
	for date := start; len(result) < count; date = date.AddDate(0, 0, 1) {
		if date.Weekday() != time.Saturday && date.Weekday() != time.Sunday {
			result = append(result, date)
		}
	}
	return result
}
