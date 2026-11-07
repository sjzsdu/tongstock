// Package reconcile compares the frozen-snapshot path against the as-of read
// path for one trade date. It is an opt-in diagnostic: nothing here writes to
// daily_selection_run or position_decision_run, and no production code calls
// it. Reachable only through `tongstock reconcile asof`.
package reconcile

import (
	"fmt"
	"math"
	"sort"

	"github.com/sjzsdu/tongstock/internal/asof"
	"github.com/sjzsdu/tongstock/internal/selection"
)

// ScoreTolerance is the accepted absolute difference between two candidate
// scores before the pair is reported as a mismatch.
const ScoreTolerance = 1e-9

// ActionDiff is one code whose action changed between the two paths.
type ActionDiff struct {
	Code     string `json:"code"`
	Snapshot string `json:"snapshot_action"`
	AsOf     string `json:"asof_action"`
}

// ScoreDiff is one code whose score moved by more than ScoreTolerance.
type ScoreDiff struct {
	Code     string  `json:"code"`
	Snapshot float64 `json:"snapshot_score"`
	AsOf     float64 `json:"asof_score"`
	Delta    float64 `json:"delta"`
}

// Report is the outcome of one reconciliation run. Differences are listed in
// full instead of being reduced to a count, so a mismatch can be audited.
type Report struct {
	// NoSnapshot marks a day that has nothing to reconcile. It is not an error:
	// the command reports it and exits 0 instead of inventing a comparison.
	NoSnapshot bool   `json:"no_snapshot,omitempty"`
	Reason     string `json:"reason,omitempty"`

	TradeDate         string `json:"trade_date"`
	SnapshotID        string `json:"snapshot_id"`
	FeatureSnapshotID string `json:"feature_snapshot_id"`
	SnapshotPath      string `json:"snapshot_path"`
	AsOfPath          string `json:"asof_path"`

	InputsDigest    string `json:"inputs_digest"`
	AsOfDataVersion int64  `json:"as_of_data_version"`

	SnapshotContentHash string `json:"snapshot_content_hash"`
	AsOfContentHash     string `json:"asof_content_hash"`
	ContentHashMatch    bool   `json:"content_hash_match"`

	SnapshotCandidateCodes []string     `json:"snapshot_candidate_codes"`
	AsOfCandidateCodes     []string     `json:"asof_candidate_codes"`
	MissingInAsOf          []string     `json:"missing_in_asof,omitempty"`
	ExtraInAsOf            []string     `json:"extra_in_asof,omitempty"`
	ActionDiffs            []ActionDiff `json:"action_diffs,omitempty"`
	ScoreDiffs             []ScoreDiff  `json:"score_diffs,omitempty"`

	AsOfCoverage asof.Coverage `json:"as_of_coverage"`
	DroppedCodes []string      `json:"dropped_codes,omitempty"`

	Consistent bool     `json:"consistent"`
	Notes      []string `json:"notes,omitempty"`
}

// Compare diffs two selection runs for the same trade date. The snapshot run
// is the "old" path (frozen artifact), the as-of run is the "new" path
// (recomputed from the bounded K-line stream).
func Compare(snapshotRun, asOfRun *selection.Run, snapshotHash, asOfHash string) Report {
	report := Report{
		SnapshotContentHash: snapshotHash,
		AsOfContentHash:     asOfHash,
		ContentHashMatch:    snapshotHash == asOfHash,
		Notes:               []string{},
	}
	if snapshotRun != nil {
		report.TradeDate = snapshotRun.SnapshotDate
		report.SnapshotID = snapshotRun.SnapshotID
		report.FeatureSnapshotID = snapshotRun.FeatureSnapshotID
		report.SnapshotCandidateCodes = candidateCodes(snapshotRun)
	}
	if asOfRun != nil {
		report.TradeDate = asOfRun.SnapshotDate
		report.AsOfCandidateCodes = candidateCodes(asOfRun)
	}

	oldByCode := actionsByCode(snapshotRun)
	newByCode := actionsByCode(asOfRun)

	report.MissingInAsOf = missing(oldByCode, newByCode)
	report.ExtraInAsOf = missing(newByCode, oldByCode)
	for _, code := range commonCodes(oldByCode, newByCode) {
		if oldByCode[code].Action != newByCode[code].Action {
			report.ActionDiffs = append(report.ActionDiffs, ActionDiff{
				Code:     code,
				Snapshot: oldByCode[code].Action,
				AsOf:     newByCode[code].Action,
			})
		}
		delta := newByCode[code].Score - oldByCode[code].Score
		if math.Abs(delta) > ScoreTolerance {
			report.ScoreDiffs = append(report.ScoreDiffs, ScoreDiff{
				Code:     code,
				Snapshot: oldByCode[code].Score,
				AsOf:     newByCode[code].Score,
				Delta:    delta,
			})
		}
	}

	report.Consistent = len(report.MissingInAsOf) == 0 &&
		len(report.ExtraInAsOf) == 0 &&
		len(report.ActionDiffs) == 0 &&
		len(report.ScoreDiffs) == 0 &&
		report.ContentHashMatch
	return report
}

// DiffSummary renders the mismatches as short human sentences.
func (r Report) DiffSummary() []string {
	var out []string
	for _, code := range r.MissingInAsOf {
		out = append(out, fmt.Sprintf("%s: 快照路径有候选，as-of 路径没有", code))
	}
	for _, code := range r.ExtraInAsOf {
		out = append(out, fmt.Sprintf("%s: as-of 路径有候选，快照路径没有", code))
	}
	for _, d := range r.ActionDiffs {
		out = append(out, fmt.Sprintf("%s: action %s -> %s", d.Code, d.Snapshot, d.AsOf))
	}
	for _, d := range r.ScoreDiffs {
		out = append(out, fmt.Sprintf("%s: score %.10f -> %.10f (delta %.3g)", d.Code, d.Snapshot, d.AsOf, d.Delta))
	}
	if !r.ContentHashMatch {
		out = append(out, fmt.Sprintf("content hash 不同: 快照 %s, as-of %s", shortHash(r.SnapshotContentHash), shortHash(r.AsOfContentHash)))
	}
	return out
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

func candidateCodes(run *selection.Run) []string {
	out := []string{}
	if run == nil {
		return out
	}
	for _, c := range run.Candidates {
		out = append(out, c.Code)
	}
	sort.Strings(out)
	return out
}

func actionsByCode(run *selection.Run) map[string]selection.Candidate {
	out := map[string]selection.Candidate{}
	if run == nil {
		return out
	}
	for _, c := range run.Candidates {
		out[c.Code] = c
	}
	return out
}

func missing(in, other map[string]selection.Candidate) []string {
	out := []string{}
	for code := range in {
		if _, ok := other[code]; !ok {
			out = append(out, code)
		}
	}
	sort.Strings(out)
	return out
}

func commonCodes(a, b map[string]selection.Candidate) []string {
	out := []string{}
	for code := range a {
		if _, ok := b[code]; ok {
			out = append(out, code)
		}
	}
	sort.Strings(out)
	return out
}
