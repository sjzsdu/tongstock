package selection

import (
	"fmt"
	"strings"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methods"
)

// ResolveScope evaluates a method's structured scope against one point-in-time
// member. New methods use machine-readable constraints per member; those
// constraints and snapshot membership stay fail-closed (missing facts reject
// the member, they are never inferred).
//
// The legacy Universe label is informational only: it records the pool a
// method was researched/validated on (e.g. "researched_stocks") and is
// deliberately NOT compared against the snapshot universe name, matching the
// eligibility decision in engine.go. Comparing labels made every mined method
// permanently unable to screen against today's "universe_usable" snapshot.
// The trade-off, by design: a legacy method with NO structured scope is
// fail-open — it expands to the full snapshot membership instead of staying
// confined to its research pool. Regression test:
// TestResolveScopeLegacyUniverseLabelIsInformational.
func ResolveScope(method *methods.CompiledMethod, market *marketsnapshot.MarketSnapshot, code string, values map[string]float64) (bool, string, string) {
	if method == nil || market == nil {
		return false, "scope_data_unavailable", "method or market snapshot is missing"
	}
	var member *marketsnapshot.UniverseMember
	for i := range market.UniverseMembers {
		if market.UniverseMembers[i].Code == code {
			member = &market.UniverseMembers[i]
			break
		}
	}
	s := method.Scope
	if member == nil {
		return false, "scope_excluded", "stock is not present in the current market snapshot"
	}
	if s.ExcludeST && (strings.EqualFold(member.Status, "st") || strings.Contains(strings.ToUpper(member.Name), "ST")) {
		return false, "scope_excluded", "method scope excludes ST stocks"
	}
	if len(s.BoardFilter) > 0 {
		matched := false
		for _, b := range s.BoardFilter {
			if strings.EqualFold(strings.TrimSpace(b), member.Board) {
				matched = true
				break
			}
		}
		if !matched {
			return false, "scope_excluded", fmt.Sprintf("board %q is outside method scope", member.Board)
		}
	}
	if s.MarketCapMin != nil || s.MarketCapMax != nil {
		cap, ok := marketCapValue(values)
		if !ok {
			return false, "scope_data_unavailable", "market cap is required by method scope but missing from current snapshot"
		}
		if s.MarketCapMin != nil && cap < *s.MarketCapMin {
			return false, "scope_excluded", "market cap is below method scope minimum"
		}
		if s.MarketCapMax != nil && cap > *s.MarketCapMax {
			return false, "scope_excluded", "market cap is above method scope maximum"
		}
	}
	return true, "", ""
}

func marketCapValue(values map[string]float64) (float64, bool) {
	for _, key := range []string{"market_cap", "total_market_cap", "market_cap_yi"} {
		if value, ok := values[key]; ok && value > 0 {
			return value, true
		}
	}
	return 0, false
}
