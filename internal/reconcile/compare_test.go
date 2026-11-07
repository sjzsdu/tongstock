package reconcile_test

import (
	"testing"

	"github.com/sjzsdu/tongstock/internal/reconcile"
	"github.com/sjzsdu/tongstock/internal/selection"
)

func runWith(candidates ...selection.Candidate) *selection.Run {
	return &selection.Run{SnapshotDate: "2024-01-05", Candidates: candidates}
}

// TestCompareUsesDocumentedScoreTolerance pins the 1e-9 threshold: floating
// point noise from an equivalent computation is not a difference.
func TestCompareUsesDocumentedScoreTolerance(t *testing.T) {
	snapshot := runWith(selection.Candidate{Code: "000001", Action: selection.ActionBuy, Score: 0.7})
	noise := runWith(selection.Candidate{Code: "000001", Action: selection.ActionBuy, Score: 0.7 + 1e-10})
	report := reconcile.Compare(snapshot, noise, "hash", "hash")
	if !report.Consistent {
		t.Fatalf("1e-10 score noise reported as a difference: %v", report.DiffSummary())
	}

	moved := runWith(selection.Candidate{Code: "000001", Action: selection.ActionBuy, Score: 0.7 + 1e-8})
	report = reconcile.Compare(snapshot, moved, "hash", "hash")
	if report.Consistent {
		t.Fatal("a 1e-8 score change must be reported")
	}
	if len(report.ScoreDiffs) != 1 || report.ScoreDiffs[0].Code != "000001" {
		t.Fatalf("score diffs = %+v", report.ScoreDiffs)
	}
}

func TestCompareListsCandidateAndActionDifferences(t *testing.T) {
	snapshot := runWith(
		selection.Candidate{Code: "000001", Action: selection.ActionBuy, Score: 0.7},
		selection.Candidate{Code: "000002", Action: selection.ActionWatch, Score: 0.5},
	)
	asOf := runWith(
		selection.Candidate{Code: "000001", Action: selection.ActionWatch, Score: 0.7},
		selection.Candidate{Code: "000003", Action: selection.ActionBuy, Score: 0.6},
	)
	report := reconcile.Compare(snapshot, asOf, "old", "new")
	if report.Consistent {
		t.Fatal("differing runs reported as consistent")
	}
	if len(report.MissingInAsOf) != 1 || report.MissingInAsOf[0] != "000002" {
		t.Fatalf("missing in as-of = %v", report.MissingInAsOf)
	}
	if len(report.ExtraInAsOf) != 1 || report.ExtraInAsOf[0] != "000003" {
		t.Fatalf("extra in as-of = %v", report.ExtraInAsOf)
	}
	if len(report.ActionDiffs) != 1 || report.ActionDiffs[0].Code != "000001" {
		t.Fatalf("action diffs = %+v", report.ActionDiffs)
	}
	if report.ContentHashMatch {
		t.Fatal("content hash match reported for different hashes")
	}
	if len(report.DiffSummary()) != 4 {
		t.Fatalf("summary = %v, want 4 entries", report.DiffSummary())
	}
}
