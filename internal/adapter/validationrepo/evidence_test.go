package validationrepo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestSQLiteEvidenceRepositoryPersistsAndVerifiesBundle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.db")
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewEvidenceRepository(store)
	if err != nil {
		t.Fatal(err)
	}
	bundle := &validation.EvidenceBundle{
		JobHash: "job-1", MethodHash: "method-1", MethodName: "test method",
		SnapshotID: "snapshot-1", GeneratedAt: time.Unix(100, 0).UTC(),
		OosStats:        validation.PerformanceStats{TotalTrades: 12, TotalReturn: 0.08},
		DiscoveryTrials: 3, AdjustedPValue: 0.03,
		Confidence: validation.ConfidenceModerate, Passable: true,
	}
	bundle.ResultHash = bundle.ComputeResultHash()
	if err := repo.Save(context.Background(), bundle); err != nil {
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
	repo, err = NewEvidenceRepository(reopened)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(context.Background(), bundle.ResultHash)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultHash != bundle.ResultHash || got.MethodHash != bundle.MethodHash || !got.Passable {
		t.Fatalf("reloaded evidence differs: %+v", got)
	}
	listed, err := repo.ListByMethod(context.Background(), bundle.MethodHash, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ResultHash != bundle.ResultHash {
		t.Fatalf("listed evidence=%+v", listed)
	}
}

func TestSQLiteEvidenceRepositoryRejectsHashMismatch(t *testing.T) {
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "evidence.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo, err := NewEvidenceRepository(store)
	if err != nil {
		t.Fatal(err)
	}
	bundle := &validation.EvidenceBundle{JobHash: "job", MethodHash: "method", SnapshotID: "snapshot", ResultHash: "forged"}
	if err := repo.Save(context.Background(), bundle); err == nil {
		t.Fatal("forged evidence must be rejected")
	}
}
