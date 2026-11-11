package dashboard

import (
	"context"
	"errors"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/selection"
	"testing"
)

type referenceSnapshotReader struct {
	feature *marketsnapshot.FeatureSnapshot
	calls   int
	err     error
}

func (r *referenceSnapshotReader) ListMarketSnapshots(string, string, string) ([]*marketsnapshot.MarketSnapshot, error) {
	return nil, nil
}
func (r *referenceSnapshotReader) LoadFeatureSnapshot(string, bool) (*marketsnapshot.FeatureSnapshot, error) {
	r.calls++
	return r.feature, r.err
}

type referenceSelectionReader struct{ run *selection.Run }

func (r referenceSelectionReader) List(context.Context, string, string, int) ([]*selection.Run, error) {
	return []*selection.Run{r.run}, nil
}

func TestLegacyCandidatePriceHydrationOnlyUsesBoundVerifiedFeature(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-market", "wrong-date", "bad-hash", "missing-close", "read-failure"} {
		t.Run(mode, func(t *testing.T) {
			feature := &marketsnapshot.FeatureSnapshot{ID: "feature", MarketSnapshotID: "market", SnapshotDate: "2026-11-10", Status: marketsnapshot.StatusReady, LeakChecked: true, PriceAdjustment: "forward", FeatureIDs: []string{"close"}, Values: map[string]map[string]float64{"600000": {"close": 10}}}
			feature.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(feature)
			reader := &referenceSnapshotReader{feature: feature}
			switch mode {
			case "wrong-market":
				feature.MarketSnapshotID = "other"
			case "wrong-date":
				feature.SnapshotDate = "2026-11-09"
			case "bad-hash":
				feature.ContentHash = "wrong"
			case "missing-close":
				delete(feature.Values["600000"], "close")
				feature.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(feature)
			case "read-failure":
				reader.err = errors.New("missing snapshot")
			}
			run := &selection.Run{ID: "run", SnapshotID: "market", FeatureSnapshotID: "feature", SnapshotDate: "2026-11-10", Candidates: []selection.Candidate{{Code: "600000"}, {Code: "600001"}}}
			svc := &Service{deps: Deps{Snapshots: reader, Selections: referenceSelectionReader{run}}}
			result, err := svc.selectionStatus(context.Background(), "market")
			if err != nil {
				t.Fatal(err)
			}
			if reader.calls != 1 {
				t.Fatalf("must load once for all candidates, got %d", reader.calls)
			}
			if run.Candidates[0].ReferencePrice != nil {
				t.Fatal("stored run mutated")
			}
			if mode == "valid" {
				price := result.Candidates[0].ReferencePrice
				if price == nil || price.Close != 10 || price.Date != "2026-11-10" || price.PriceAdjustment != "forward" || price.FeatureSnapshotID != "feature" {
					t.Fatalf("bad price %+v", price)
				}
			} else if result.Candidates[0].ReferencePrice != nil {
				t.Fatalf("unsafe reference for %s", mode)
			}
			if result.Candidates[1].ReferencePrice != nil {
				t.Fatal("must not guess price for missing code")
			}
		})
	}
}
