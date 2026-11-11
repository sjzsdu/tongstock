package methodregistry

import (
	"encoding/json"
	"testing"

	"github.com/sjzsdu/tongstock/internal/validation"
)

// A zero-trade backtest has an unknown win rate, not a 0% one: RegistryEvidence
// must keep the pointer nil so the serialized evidence omits the field entirely.
func TestRegistryEvidenceOmitsWinRateWithoutTrades(t *testing.T) {
	empty := ValidationEvidence{Bundle: &validation.EvidenceBundle{OosStats: validation.PerformanceStats{TotalTrades: 0, WinRate: 0}}}.RegistryEvidence()
	if empty.OOSWinRate != nil {
		t.Fatalf("zero-trade evidence reported %v", *empty.OOSWinRate)
	}
	raw, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, found := fields["oos_win_rate"]; found {
		t.Fatalf("zero-trade evidence leaked oos_win_rate: %s", raw)
	}
	sampled := ValidationEvidence{Bundle: &validation.EvidenceBundle{OosStats: validation.PerformanceStats{TotalTrades: 5, WinRate: 0.4}}}.RegistryEvidence()
	if sampled.OOSWinRate == nil || *sampled.OOSWinRate != 0.4 {
		t.Fatalf("sampled evidence lost win rate: %+v", sampled.OOSWinRate)
	}
}

func TestHistoricalWinRateMissingIsDifferentFromZero(t *testing.T) {
	for _, test := range []struct {
		raw     string
		present bool
	}{{`{"oos_trades":10}`, false}, {`{"oos_trades":10,"oos_win_rate":0}`, true}} {
		var evidence EvidenceSummary
		if err := json.Unmarshal([]byte(test.raw), &evidence); err != nil {
			t.Fatal(err)
		}
		if (evidence.OOSWinRate != nil) != test.present {
			t.Fatalf("wrong presence %+v", evidence)
		}
		raw, err := json.Marshal(evidence)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err = json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		_, found := fields["oos_win_rate"]
		if found != test.present {
			t.Fatalf("lost presence %s", raw)
		}
	}
}
