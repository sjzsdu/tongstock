package discovery

import (
	"testing"

	"github.com/sjzsdu/tongstock/internal/methods"
)

func TestHistoricalScopeFailsClosedWithoutPointInTimeFacts(t *testing.T) {
	minCap := 30.0
	m := &methods.CompiledMethod{Scope: methods.Scope{MarketCapMin: &minCap}}
	if historicalScopeAvailable(m, methods.Bar{Indicators: map[string]float64{}}) {
		t.Fatal("missing historical market cap must not be treated as in-scope")
	}
	m.Scope.BoardFilter = []string{"main"}
	if historicalScopeAvailable(m, methods.Bar{Indicators: map[string]float64{"market_cap": 50}}) {
		t.Fatal("missing historical board label must not be inferred")
	}
}

func TestCandidateSpecMustDeclareItsOwnOutcome(t *testing.T) {
	target := 0.1
	base := Request{SnapshotID: "snap", StockCodes: []string{"000001"}, CandidateSpecs: []methods.Candidate{{Name: "incomplete"}}}
	if err := base.Normalize(); err == nil {
		t.Fatal("candidate without an outcome must be rejected")
	}
	base.CandidateSpecs[0].Outcome = &methods.OutcomeRule{HorizonDays: 20, TargetReturnPct: &target, PriceBasis: "close", Success: "close_gte_target"}
	if err := base.Normalize(); err != nil {
		t.Fatalf("explicit candidate outcome should be accepted: %v", err)
	}
}

func TestMatchedForwardOutcomesCountsAnyCloseWithinTargetWindow(t *testing.T) {
	target := 0.08
	m, _, err := methods.Compile(&methods.Candidate{
		Name: "window target", Entry: map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "constant", "value": 0}, "op": "gt"},
		Outcome: &methods.OutcomeRule{HorizonDays: 3, TargetReturnPct: &target, PriceBasis: "close", Success: "close_gte_target"},
	})
	if err != nil || !m.IsExecutable() {
		t.Fatalf("compile outcome method: err=%v diagnostics=%v", err, m.Diagnostics)
	}
	bar := func(date string, open, close float64) methods.Bar {
		return methods.Bar{Date: date, Open: open, High: close, Low: open, Close: close, Indicators: map[string]float64{"close": close}}
	}
	// Entry on day 0; day 2 reaches +8%, while the terminal day retreats.
	bars := []methods.Bar{bar("d0", 100, 100), bar("d1", 100, 105), bar("d2", 105, 109), bar("d3", 109, 103), bar("d4", 103, 102)}
	returns, hits, total, err := matchedForwardOutcomes(m, bars, 3)
	if err != nil || total != 1 || hits != 1 || len(returns) != 1 {
		t.Fatalf("expected one target hit within window: returns=%v hits=%d total=%d err=%v", returns, hits, total, err)
	}
}

func TestMatchedForwardOutcomesUsesProviderSelectedPriceBasis(t *testing.T) {
	target := 0.08
	m, _, err := methods.Compile(&methods.Candidate{
		Name: "high target", Entry: map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": "close"}, "right": map[string]any{"type": "constant", "value": 0}, "op": "gt"},
		Outcome: &methods.OutcomeRule{HorizonDays: 2, TargetReturnPct: &target, PriceBasis: "high", Success: "price_gte_target"},
	})
	if err != nil || !m.IsExecutable() {
		t.Fatalf("compile high outcome method: err=%v diagnostics=%v", err, m.Diagnostics)
	}
	bar := func(date string, open, high, close float64) methods.Bar {
		return methods.Bar{Date: date, Open: open, High: high, Low: open, Close: close, Indicators: map[string]float64{"close": close}}
	}
	// The close never reaches +8%; only the provider-selected high does.
	bars := []methods.Bar{bar("d0", 100, 100, 100), bar("d1", 100, 109, 105), bar("d2", 105, 106, 103), bar("d3", 103, 104, 102)}
	_, hits, total, err := matchedForwardOutcomes(m, bars, 2)
	if err != nil || total != 1 || hits != 1 {
		t.Fatalf("expected high basis to count target hit: hits=%d total=%d err=%v", hits, total, err)
	}
}
