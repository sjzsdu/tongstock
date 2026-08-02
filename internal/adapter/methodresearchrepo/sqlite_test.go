package methodresearchrepo

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/methodresearch"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestSQLiteRepositoryPersistsSourceEvidenceSeparately(t *testing.T) {
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "source-evidence.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	result := &methodresearch.ResearchResult{
		ResearchID: "real-source-case", FamilyID: "method-family-turtle", Input: methodresearch.ResearchInput{Kind: methodresearch.InputURL, Value: "https://www.turtletrader.com/rules/"},
		MethodName: "海龟交易法", Status: methodresearch.StatusComplete, CreatedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
		Sources:        []methodresearch.SourceDocument{{ID: "rules", URL: "https://www.turtletrader.com/rules/", Title: "The Original Turtle Trading Rules", Publisher: "TurtleTrader", RetrievedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Tier: methodresearch.TierSecondary, ContentHash: fmt.Sprintf("%x", sha256.Sum256([]byte("System One uses a twenty-day breakout.")))}},
		ValidationJobs: []methodresearch.ValidationHandoff{{JobID: "real-source-case:system-1", FamilyID: "method-family-turtle", VariantID: "system-1", MethodHash: "method-content-hash", Scope: "universe_all", Status: "queued"}},
	}
	result.ResultHash = result.ComputeHash()
	if err := repo.Save(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.Get(context.Background(), result.ResearchID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ResultHash != result.ResultHash || len(loaded.Sources) != 1 {
		t.Fatalf("loaded=%+v", loaded)
	}
	var count int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM method_source_evidence WHERE research_id=?`, result.ResearchID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("source evidence rows=%d", count)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM method_validation_queue WHERE research_id=? AND status='queued'`, result.ResearchID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("validation queue rows=%d", count)
	}
}
