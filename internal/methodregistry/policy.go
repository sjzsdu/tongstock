package methodregistry

import (
	"fmt"
	"strings"

	"github.com/sjzsdu/tongstock/internal/validation"
)

type ValidationEvidence struct{ Bundle *validation.EvidenceBundle }

func (v ValidationEvidence) RegistryEvidence() EvidenceInput {
	if v.Bundle == nil {
		return EvidenceInput{}
	}
	b := v.Bundle
	hard := false
	for _, x := range b.Blockers {
		if x.Severity == "hard" {
			hard = true
		}
	}
	return EvidenceInput{ResultHash: b.ResultHash, ComputedHash: b.ComputeResultHash(), SnapshotID: b.SnapshotID, JobHash: b.JobHash, MethodHash: b.MethodHash, StockCode: b.StockCode, Confidence: string(b.Confidence), Passable: b.Passable, HasHardBlocker: hard, OOSTrades: b.OosStats.TotalTrades, OOSReturn: b.OosStats.TotalReturn, OOSWinRate: b.OosStats.WinRate, OOSMaxDrawdown: b.OosStats.MaxDrawdown}
}

type Policy struct{}

func (Policy) Initial(methodExecutable bool, universe string, e EvidenceInput) (Status, string) {
	if !methodExecutable {
		return StatusRejected, "compiled method is not executable"
	}
	if e.ResultHash == "" {
		return StatusCandidate, "awaiting real validation evidence"
	}
	if e.ResultHash != e.ComputedHash {
		return StatusRejected, "validation evidence integrity mismatch"
	}
	if e.SnapshotID == "" || e.JobHash == "" {
		return StatusRejected, "validation evidence lacks immutable snapshot or job"
	}
	if strings.HasPrefix(universe, "single:") && strings.TrimPrefix(universe, "single:") != e.StockCode {
		return StatusRejected, "validation stock does not match single-stock method scope"
	}
	if e.HasHardBlocker || !e.Passable {
		return StatusRejected, "validation promotion gate rejected method"
	}
	if e.Confidence != "moderate" && e.Confidence != "strong" {
		return StatusRejected, "validation confidence below moderate"
	}
	if !strings.HasPrefix(universe, "single:") && e.StockCode != "" {
		return StatusCandidate, "single-stock evidence cannot verify a broader method scope"
	}
	return StatusVerified, "machine validation policy passed"
}

func (Policy) Health(current Status, h HealthState) (Status, string) {
	if current == StatusRetired || current == StatusRejected {
		return current, "terminal state unchanged"
	}
	if h.ConsecutiveSevere >= 3 {
		return StatusRetired, "three consecutive severe forward-health failures"
	}
	if h.CriticalAlerts > 0 || h.Drift || h.Decay || h.ExecutionDeviation || h.Score < 60 {
		return StatusDegraded, "forward health policy degraded method"
	}
	if h.ForwardSamples >= 20 && h.Score >= 75 {
		return StatusObserving, "forward evidence healthy"
	}
	return current, "insufficient forward evidence for state change"
}

func validateRegistration(r Registration) error {
	if strings.TrimSpace(r.FamilyID) == "" || strings.TrimSpace(r.VariantID) == "" {
		return fmt.Errorf("family_id and variant_id are required")
	}
	if r.Method == nil {
		return fmt.Errorf("compiled method is required")
	}
	if r.Method.ContentHash == "" {
		return fmt.Errorf("compiled method hash is required")
	}
	return nil
}
