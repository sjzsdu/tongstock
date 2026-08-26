package paradigms

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnavailableEvidenceOmitsUnprovenNumbers(t *testing.T) {
	card := EvidenceCard{
		ParadigmID: "p-1", Available: false, PromotionEligible: false,
		UnavailableReasons: []string{"没有持久化实验"},
		PromotionBlockers:  []string{"真实交易证据不可用"},
		CounterEvidence:    []CounterExample{},
		RiskFlags:          []RiskFlag{},
	}
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"in_sample"`, `"out_of_sample"`, `"confidence_interval"`,
		`"cost_analysis"`, `"trade_samples"`,
	} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("unavailable evidence emitted unproven field %s: %s", forbidden, data)
		}
	}
}
