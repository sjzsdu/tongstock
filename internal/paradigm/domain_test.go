package paradigm

import (
	"testing"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from, to State
		valid    bool
	}{
		{StateHypothesis, StateExperiment, true},
		{StateHypothesis, StateCandidate, false},
		{StateExperiment, StateCandidate, true},
		{StateExperiment, StateRetired, true},
		{StateCandidate, StateValidation, true},
		{StateCandidate, StateRetired, true},
		{StateValidation, StateObservation, true},
		{StateValidation, StateCandidate, true},
		{StateValidation, StateRetired, true},
		{StateObservation, StatePromoted, true},
		{StateObservation, StateRelegated, true},
		{StateObservation, StateRetired, true},
		{StatePromoted, StateRelegated, true},
		{StatePromoted, StateRetired, true},
		{StateRelegated, StateObservation, true},
		{StateRelegated, StateRetired, true},
		{StateRetired, StatePromoted, false},
		{StateRetired, StateCandidate, false},
		{StateHypothesis, StatePromoted, false},
		{StateCandidate, StatePromoted, false},
	}

	for _, tc := range tests {
		result := CanTransition(tc.from, tc.to)
		if result != tc.valid {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", tc.from, tc.to, result, tc.valid)
		}
	}
}

func TestNewLifecycle(t *testing.T) {
	lc := NewLifecycle("p1")
	if lc.ParadigmID != "p1" {
		t.Errorf("ParadigmID = %q, want p1", lc.ParadigmID)
	}
	if lc.Current != StateHypothesis {
		t.Errorf("Current = %v, want %v", lc.Current, StateHypothesis)
	}
	if len(lc.History) != 0 {
		t.Errorf("History should be empty, got %d entries", len(lc.History))
	}
	if lc.IsTerminal() {
		t.Error("New lifecycle should not be terminal")
	}
	if lc.IsActive() {
		t.Error("New lifecycle should not be active (hypothesis state)")
	}
}

func TestLifecycleTransitionTo(t *testing.T) {
	lc := NewLifecycle("p1")

	// Hypothesis -> Experiment
	if err := lc.TransitionTo(StateExperiment, "begin experiment", "researcher"); err != nil {
		t.Fatalf("Transition to Experiment failed: %v", err)
	}
	if lc.Current != StateExperiment {
		t.Errorf("Current = %v, want %v", lc.Current, StateExperiment)
	}
	if len(lc.History) != 1 {
		t.Errorf("History should have 1 entry, got %d", len(lc.History))
	}

	// Experiment -> Candidate
	if err := lc.TransitionTo(StateCandidate, "passed screening", "researcher"); err != nil {
		t.Fatalf("Transition to Candidate failed: %v", err)
	}

	// Candidate -> Validation
	if err := lc.TransitionTo(StateValidation, "begin validation", "researcher"); err != nil {
		t.Fatalf("Transition to Validation failed: %v", err)
	}

	// Validation -> Observation
	if err := lc.TransitionTo(StateObservation, "passed validation", "researcher"); err != nil {
		t.Fatalf("Transition to Observation failed: %v", err)
	}
	if !lc.IsActive() {
		t.Error("Observation state should be active")
	}

	// Observation -> Promoted
	if err := lc.TransitionTo(StatePromoted, "forward run passed", "reviewer"); err != nil {
		t.Fatalf("Transition to Promoted failed: %v", err)
	}
	if !lc.IsActive() {
		t.Error("Promoted state should be active")
	}

	// Promoted -> Retired
	if err := lc.TransitionTo(StateRetired, "performance degraded", "reviewer"); err != nil {
		t.Fatalf("Transition to Retired failed: %v", err)
	}
	if !lc.IsTerminal() {
		t.Error("Retired should be terminal")
	}
}

func TestLifecycleInvalidTransition(t *testing.T) {
	lc := NewLifecycle("p1")

	// Cannot skip from Hypothesis directly to Candidate
	err := lc.TransitionTo(StateCandidate, "skip", "researcher")
	if err == nil {
		t.Error("Expected error for invalid transition Hypothesis -> Candidate")
	}

	// Cannot transition from terminal state
	lc2 := NewLifecycle("p2")
	_ = lc2.TransitionTo(StateExperiment, "begin", "researcher")
	_ = lc2.TransitionTo(StateRetired, "abandoned", "researcher")
	err = lc2.TransitionTo(StateCandidate, "revive", "researcher")
	if err == nil {
		t.Error("Expected error for transition from terminal state")
	}
}

func TestLifecycleRelegation(t *testing.T) {
	lc := NewLifecycle("p1")
	_ = lc.TransitionTo(StateExperiment, "begin", "r")
	_ = lc.TransitionTo(StateCandidate, "passed", "r")
	_ = lc.TransitionTo(StateValidation, "validate", "r")
	_ = lc.TransitionTo(StateObservation, "observe", "r")
	_ = lc.TransitionTo(StatePromoted, "promoted", "r")

	// Promoted -> Relegated
	if err := lc.TransitionTo(StateRelegated, "performance dropped", "reviewer"); err != nil {
		t.Fatalf("Transition to Relegated failed: %v", err)
	}
	if lc.Current != StateRelegated {
		t.Errorf("Current = %v, want %v", lc.Current, StateRelegated)
	}

	// Relegated -> Observation (can retry)
	if err := lc.TransitionTo(StateObservation, "retry observation", "reviewer"); err != nil {
		t.Fatalf("Transition from Relegated to Observation failed: %v", err)
	}
}

func TestTraceabilityChainValidate(t *testing.T) {
	chain := TraceabilityChain{
		ParadigmVersionID: "pv1",
	}

	// Hypothesis state: only paradigm_version_id required
	if err := chain.Validate(StateHypothesis); err != nil {
		t.Errorf("Hypothesis validation should pass: %v", err)
	}

	// Experiment state: needs hypothesis_id
	if err := chain.Validate(StateExperiment); err == nil {
		t.Error("Experiment validation should fail without hypothesis_id")
	}
	chain.HypothesisID = "h1"
	if err := chain.Validate(StateExperiment); err != nil {
		t.Errorf("Experiment validation should pass with hypothesis_id: %v", err)
	}

	// Validation state: needs dataset_snapshot_id
	if err := chain.Validate(StateValidation); err == nil {
		t.Error("Validation should fail without dataset_snapshot_id")
	}
	chain.DatasetSnapshotID = "ds1"
	if err := chain.Validate(StateValidation); err != nil {
		t.Errorf("Validation should pass with dataset_snapshot_id: %v", err)
	}

	// Promoted state: needs validation_report_id
	if err := chain.Validate(StatePromoted); err == nil {
		t.Error("Promoted should fail without validation_report_id")
	}
	chain.ValidationReportID = "vr1"
	if err := chain.Validate(StatePromoted); err != nil {
		t.Errorf("Promoted should pass with validation_report_id: %v", err)
	}
}

func TestTraceabilityChainMissingParadigmVersion(t *testing.T) {
	chain := TraceabilityChain{}
	if err := chain.Validate(StateHypothesis); err == nil {
		t.Error("Should fail when paradigm_version_id is missing")
	}
}

func TestStateDescriptions(t *testing.T) {
	states := []State{
		StateHypothesis, StateExperiment, StateCandidate,
		StateValidation, StateObservation, StatePromoted,
		StateRelegated, StateRetired,
	}

	for _, s := range states {
		desc, ok := StateDescriptions[s]
		if !ok {
			t.Errorf("No description for state %v", s)
		}
		if desc == "" {
			t.Errorf("Empty description for state %v", s)
		}
	}
}

func TestRetiredIsTerminal(t *testing.T) {
	lc := NewLifecycle("p1")
	_ = lc.TransitionTo(StateExperiment, "begin", "r")
	_ = lc.TransitionTo(StateRetired, "abandoned", "r")
	if !lc.IsTerminal() {
		t.Error("Retired state should be terminal")
	}
}

func TestIsActive(t *testing.T) {
	tests := []struct {
		state State
		active bool
	}{
		{StateHypothesis, false},
		{StateExperiment, false},
		{StateCandidate, false},
		{StateValidation, false},
		{StateObservation, true},
		{StatePromoted, true},
		{StateRelegated, false},
		{StateRetired, false},
	}

	for _, tc := range tests {
		lc := &Lifecycle{Current: tc.state}
		if lc.IsActive() != tc.active {
			t.Errorf("IsActive(%v) = %v, want %v", tc.state, lc.IsActive(), tc.active)
		}
	}
}
