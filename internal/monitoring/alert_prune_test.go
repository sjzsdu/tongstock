package monitoring

import "testing"

func TestMonitorIndustryConcentrationSkipsWhenIndustryUnknown(t *testing.T) {
	monitor := NewConcentrationMonitor(NewConcentrationConfig())
	result := monitor.MonitorIndustryConcentration([]PositionItem{
		{Code: "600000", Weight: 0.5},
		{Code: "000001", Weight: 0.5},
	})

	if result.IsConcentrated {
		t.Fatalf("unknown industries must not be reported as concentrated: %s", result.Description)
	}
	if result.Severity != "normal" {
		t.Fatalf("severity=%q, want normal", result.Severity)
	}
	if result.HHI != 0 || result.TopWeight != 0 {
		t.Fatalf("hhi=%v top=%v, want zeros when industry tags are missing", result.HHI, result.TopWeight)
	}
	if result.Description == "" {
		t.Fatal("expected a description explaining why the check was skipped")
	}
}

func TestPruneSuppressedDropsCooldownNoise(t *testing.T) {
	engine := NewAlertEngine(NewAlertConfig())
	concentration := []ConcentrationResult{{
		Type:           ConcentrationPosition,
		Severity:       "warning",
		IsConcentrated: true,
		HHI:            0.40,
		Threshold:      0.25,
	}}

	first := engine.GenerateAlerts(nil, nil, concentration, "watchlist_proxy")
	if len(first) != 1 {
		t.Fatalf("first generation produced %d alerts, want 1", len(first))
	}
	engine.GenerateAlerts(nil, nil, concentration, "watchlist_proxy")

	summary := engine.GetAlertSummary()
	if summary.TotalAlerts != 2 || summary.SuppressedCount != 1 {
		t.Fatalf("cooldown should suppress the repeat: %+v", summary)
	}

	engine.PruneSuppressed()

	summary = engine.GetAlertSummary()
	if summary.TotalAlerts != 1 || summary.ActiveCount != 1 || summary.SuppressedCount != 0 {
		t.Fatalf("prune should keep only meaningful alerts: %+v", summary)
	}
}
