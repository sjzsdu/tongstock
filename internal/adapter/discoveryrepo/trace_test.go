package discoveryrepo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestTraceRepositorySurvivesRestartAndVerifiesHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.db")
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewTraceRepository(store)
	if err != nil {
		t.Fatal(err)
	}
	result := &discovery.Result{
		ResearchID: "research-1", SnapshotID: "snapshot-1",
		GeneratorVersion: discovery.GeneratorVersion, GeneratedAt: time.Unix(100, 0).UTC(),
		HoldDays: 5, SearchBudget: 24, DiscoveryTrials: 24,
		Boundaries: []discovery.CodeBoundary{{Code: "000001", DiscoveryBars: 200, ReservedBars: 80}},
		Conclusion: "insufficient_evidence",
	}
	result.ResultHash = result.ComputeHash()
	if err := repo.Save(context.Background(), result); err != nil {
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
	repo, err = NewTraceRepository(reopened)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(context.Background(), result.ResearchID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultHash != result.ResultHash || got.DiscoveryTrials != result.DiscoveryTrials {
		t.Fatalf("reloaded trace differs: %+v", got)
	}
}
