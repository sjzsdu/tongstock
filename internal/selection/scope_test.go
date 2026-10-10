package selection

import (
	"testing"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methods"
)

func TestResolveScopeDoesNotRequireUniverseNameEquality(t *testing.T) {
	minCap, maxCap := 30.0, 100.0
	m := &methods.CompiledMethod{Scope: methods.Scope{Universe: "generated_scope", BoardFilter: []string{"main"}, MarketCapMin: &minCap, MarketCapMax: &maxCap}}
	market := &marketsnapshot.MarketSnapshot{Universe: marketsnapshot.UniverseDefinition{Name: "universe_usable"}, UniverseMembers: []marketsnapshot.UniverseMember{{Code: "000001", Board: "main", Status: "normal", Selected: true}}}
	ok, reason, _ := ResolveScope(m, market, "000001", map[string]float64{"market_cap": 50})
	if !ok || reason != "" {
		t.Fatalf("structured scope should match despite universe label: ok=%v reason=%s", ok, reason)
	}
}

func TestResolveScopeFailsClosedWhenRequiredMarketCapMissing(t *testing.T) {
	minCap := 30.0
	m := &methods.CompiledMethod{Scope: methods.Scope{MarketCapMin: &minCap}}
	market := &marketsnapshot.MarketSnapshot{UniverseMembers: []marketsnapshot.UniverseMember{{Code: "000001", Board: "main", Status: "normal", Selected: true}}}
	ok, reason, detail := ResolveScope(m, market, "000001", map[string]float64{})
	if ok || reason != "scope_data_unavailable" || detail == "" {
		t.Fatalf("missing market cap must be excluded: ok=%v reason=%s detail=%s", ok, reason, detail)
	}
}

// TestResolveScopeLegacyUniverseLabelIsInformational guards the behavior the
// mined-method blocker fix depends on: a legacy method carrying only a
// Universe label (no structured scope) must NOT be rejected for the label
// differing from the snapshot universe name — it intentionally expands to the
// full snapshot membership instead of staying confined to its research pool.
func TestResolveScopeLegacyUniverseLabelIsInformational(t *testing.T) {
	m := &methods.CompiledMethod{Scope: methods.Scope{Universe: "researched_stocks"}}
	market := &marketsnapshot.MarketSnapshot{
		Universe:        marketsnapshot.UniverseDefinition{Name: "universe_usable"},
		UniverseMembers: []marketsnapshot.UniverseMember{{Code: "000001", Board: "main", Status: "normal", Selected: true}},
	}
	ok, reason, detail := ResolveScope(m, market, "000001", map[string]float64{})
	if !ok || reason != "" {
		t.Fatalf("legacy universe label must not gate screening: ok=%v reason=%s detail=%s", ok, reason, detail)
	}
}
