package methodregistry

import (
	"testing"
)

func evidence(passable bool, confidence string, extra map[string]string) EvidenceInput {
	e := EvidenceInput{
		ResultHash:  "rh", ComputedHash: "rh",
		SnapshotID: "snap-1", JobHash: "job-1", MethodHash: "mh",
		StockCode: "000001", Confidence: confidence, Passable: passable,
	}
	for k, v := range extra {
		switch k {
		case "result_hash":
			e.ResultHash = v
		case "computed_hash":
			e.ComputedHash = v
		case "snapshot_id":
			e.SnapshotID = v
		case "job_hash":
			e.JobHash = v
		case "stock_code":
			e.StockCode = v
		case "confidence":
			e.Confidence = v
		case "hard":
			e.HasHardBlocker = true
		}
	}
	return e
}

func TestPolicyInitial(t *testing.T) {
	p := Policy{}
	valid := evidence(true, "strong", nil)

	cases := []struct {
		name     string
		exec     bool
		universe string
		e        EvidenceInput
		want     Status
	}{
		{"not executable -> rejected", false, "multi", valid, StatusRejected},
		{"no evidence -> candidate", true, "multi", EvidenceInput{}, StatusCandidate},
		{"hash mismatch -> rejected", true, "multi", evidence(true, "strong", map[string]string{"computed_hash": "other"}), StatusRejected},
		{"missing snapshot -> rejected", true, "multi", evidence(true, "strong", map[string]string{"snapshot_id": ""}), StatusRejected},
		{"missing job -> rejected", true, "multi", evidence(true, "strong", map[string]string{"job_hash": ""}), StatusRejected},
		{"single universe stock mismatch -> rejected", true, "single:600519", evidence(true, "strong", map[string]string{"stock_code": "000001"}), StatusRejected},
		{"not passable -> rejected", true, "multi", evidence(false, "strong", nil), StatusRejected},
		{"hard blocker -> rejected", true, "multi", evidence(true, "strong", map[string]string{"hard": "1"}), StatusRejected},
		{"low confidence -> rejected", true, "multi", evidence(true, "weak", nil), StatusRejected},
		{"single-stock evidence for broad scope -> candidate", true, "multi", valid, StatusCandidate},
		{"verified pass", true, "single:000001", evidence(true, "moderate", nil), StatusVerified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := p.Initial(tc.exec, tc.universe, tc.e)
			if got != tc.want {
				t.Errorf("Initial(%v,%q) = %s, want %s", tc.exec, tc.universe, got, tc.want)
			}
		})
	}
}

func TestPolicyHealth(t *testing.T) {
	p := Policy{}
	cases := []struct {
		name    string
		current Status
		h       HealthState
		want    Status
	}{
		{"terminal retired unchanged", StatusRetired, HealthState{Score: 10}, StatusRetired},
		{"terminal rejected unchanged", StatusRejected, HealthState{Score: 10}, StatusRejected},
		{"three consecutive severe -> retired", StatusVerified, HealthState{ConsecutiveSevere: 3}, StatusRetired},
		{"critical alert -> degraded", StatusVerified, HealthState{CriticalAlerts: 1, Score: 90}, StatusDegraded},
		{"drift -> degraded", StatusVerified, HealthState{Drift: true, Score: 90}, StatusDegraded},
		{"decay -> degraded", StatusVerified, HealthState{Decay: true, Score: 90}, StatusDegraded},
		{"low score -> degraded", StatusVerified, HealthState{Score: 59}, StatusDegraded},
		{"enough samples healthy -> observing", StatusVerified, HealthState{ForwardSamples: 20, Score: 80}, StatusObserving},
		{"insufficient evidence -> unchanged", StatusVerified, HealthState{ForwardSamples: 5, Score: 80}, StatusVerified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := p.Health(tc.current, tc.h)
			if got != tc.want {
				t.Errorf("Health(%s) = %s, want %s", tc.current, got, tc.want)
			}
		})
	}
}

func TestValidateRegistration(t *testing.T) {
	if err := validateRegistration(Registration{}); err == nil {
		t.Fatal("empty registration should fail")
	}
	if err := validateRegistration(Registration{FamilyID: "f", VariantID: "v"}); err == nil {
		t.Fatal("registration without method should fail")
	}
	// 需要真实 methods.CompiledMethod；这里仅验证字段校验顺序。
}
