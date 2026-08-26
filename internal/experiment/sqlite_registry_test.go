package experiment

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestSQLiteRegistrySurvivesRestartWithArtifacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experiments.db")
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewSQLiteRegistry(store)
	if err != nil {
		t.Fatal(err)
	}
	exp := createTestExperiment()
	exp.ID = "exp-persistent"
	exp.Tags = []string{"real-data", "reproducible"}
	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}
	run := NewRun(exp.ID, exp.ConfigHash)
	run.ID = "run-persistent"
	run.Start()
	run.Complete(MetricSet{NetPnL: 123.45, TotalTrades: 2}, []Artifact{
		{Type: ArtifactFills, Name: "transactions", Content: json.RawMessage(`[{"id":"fill-1"}]`)},
		{Type: ArtifactEquity, Name: "equity", Content: json.RawMessage(`[1000000,1000123.45]`)},
	})
	if err := registry.CreateRun(run); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateRun(run); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reloadedRegistry, err := NewSQLiteRegistry(reopened)
	if err != nil {
		t.Fatal(err)
	}
	gotExp, err := reloadedRegistry.GetByID(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotExp.ConfigHash != exp.ConfigHash || gotExp.Status != exp.Status {
		t.Fatalf("reloaded experiment mismatch: %+v", gotExp)
	}
	gotRun, err := reloadedRegistry.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotRun.ResultHash != run.ResultHash || gotRun.Metrics == nil || gotRun.Metrics.NetPnL != 123.45 {
		t.Fatalf("reloaded run mismatch: %+v", gotRun)
	}
	if len(gotRun.Artifacts) != 2 {
		t.Fatalf("artifacts = %d, want 2", len(gotRun.Artifacts))
	}
	for _, artifact := range gotRun.Artifacts {
		if artifact.ContentHash == "" || len(artifact.Content) == 0 {
			t.Fatalf("artifact was not fully persisted: %+v", artifact)
		}
	}
}

func TestSQLiteRegistryRejectsUnknownRunExperiment(t *testing.T) {
	store, err := storage.New(storage.Config{
		Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "experiments.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry, err := NewSQLiteRegistry(store)
	if err != nil {
		t.Fatal(err)
	}
	run := NewRun("missing-experiment", "hash")
	if err := registry.CreateRun(run); err == nil {
		t.Fatal("run without a persisted experiment must fail")
	}
}
