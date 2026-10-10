package methods

import "testing"

func TestCompilePreservesCandidateSpecificOutcomeAndScope(t *testing.T) {
	targetA, targetB := 0.08, 0.15
	minCap, maxCap := 30.0, 100.0
	entry := map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "indicator", "indicator": "ma20"}, "op": "gt"}
	a, _, err := Compile(&Candidate{Name: "A", Entry: entry, MarketCapMin: &minCap, MarketCapMax: &maxCap, ExcludeST: true, Outcome: &OutcomeRule{HorizonDays: 17, TargetReturnPct: &targetA, PriceBasis: "close", Success: "close_gte_target"}})
	if err != nil || a.Outcome.HorizonDays != 17 || a.Outcome.TargetReturnPct == nil || *a.Outcome.TargetReturnPct != targetA {
		t.Fatalf("candidate A outcome was not preserved: %#v err=%v", a, err)
	}
	if a.Scope.MarketCapMin == nil || *a.Scope.MarketCapMin != minCap || !a.Scope.ExcludeST {
		t.Fatalf("candidate A scope was not preserved: %#v", a.Scope)
	}
	b, _, err := Compile(&Candidate{Name: "B", Entry: entry, Outcome: &OutcomeRule{HorizonDays: 7, TargetReturnPct: &targetB, PriceBasis: "close", Success: "close_gte_target"}})
	if err != nil || b.Outcome.HorizonDays != 7 || b.Outcome.TargetReturnPct == nil || *b.Outcome.TargetReturnPct != targetB {
		t.Fatalf("candidate B outcome was not preserved: %#v err=%v", b, err)
	}
	if a.ContentHash == b.ContentHash {
		t.Fatal("candidate-specific outcome must affect method hash")
	}
}

func TestCompileLegacyHoldingDerivesOutcome(t *testing.T) {
	target := 0.1
	m, _, err := Compile(&Candidate{Name: "legacy", Entry: map[string]any{"type": "indicator", "indicator": "close"}, HoldingMaxDays: 20, TakeProfitPct: &target})
	if err != nil || m.Outcome.HorizonDays != 20 || m.Outcome.TargetReturnPct == nil || *m.Outcome.TargetReturnPct != target {
		t.Fatalf("legacy outcome fallback failed: %#v err=%v", m, err)
	}
}
