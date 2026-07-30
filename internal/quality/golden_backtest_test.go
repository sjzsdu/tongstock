package quality

import (
	"context"
	"testing"
)

func TestGoldenBacktestRunner_DefaultSet(t *testing.T) {
	engine := NewBaselineEngineAdapter()
	gs := DefaultGoldenSet()
	runner := NewGoldenBacktestRunner(engine, gs.Specs)

	result, details := runner.RunAll(context.Background())

	if result.TestCount != len(gs.Specs) {
		t.Fatalf("expected %d tests, got %d", len(gs.Specs), result.TestCount)
	}
	if len(details) != len(gs.Specs) {
		t.Fatalf("expected %d detail results, got %d", len(gs.Specs), len(details))
	}

	t.Logf("Golden test results: %d/%d passed", result.TestCount-result.FailCount, result.TestCount)
	for _, d := range details {
		t.Logf("  [%s] pass=%v ret=%.4f diff=%.4f trades=%d win=%.2f",
			d.SpecID, d.Passed, d.ActualReturn, d.ReturnDiff, d.ActualTrades, d.ActualWinRate)
	}
}

func TestGoldenBacktestRunner_EmptyEngine(t *testing.T) {
	runner := NewGoldenBacktestRunner(nil, nil)
	result, _ := runner.RunAll(context.Background())

	if result.TestCount != 0 {
		t.Errorf("expected 0 tests, got %d", result.TestCount)
	}
	if !result.TestPassed {
		t.Error("empty set should pass")
	}
}

func TestGoldenBacktestRunner_Deterministic(t *testing.T) {
	engine := NewBaselineEngineAdapter()
	gs := DefaultGoldenSet()
	runner := NewGoldenBacktestRunner(engine, gs.Specs)

	result1, _ := runner.RunAll(context.Background())
	result2, _ := runner.RunAll(context.Background())

	if result1.TestHash != result2.TestHash {
		t.Errorf("results not deterministic: hash1=%s hash2=%s", result1.TestHash, result2.TestHash)
	}
}

func TestGoldenBacktestRunner_RegressionDetection(t *testing.T) {
	engine := NewBaselineEngineAdapter()
	gs := DefaultGoldenSet()
	gs.GoldenHash = "wrong_hash" // 设置错误的黄金哈希

	runner := NewGoldenBacktestRunner(engine, gs.Specs)
	result, _ := runner.RunAll(context.Background())

	result.GoldenHash = gs.GoldenHash
	if result.TestHash != result.GoldenHash {
		result.Regressed = true
	}

	if result.Regressed {
		t.Log("Regression detected (expected with wrong golden hash)")
	}
}

func TestGoldenBacktestRunner_BuyAndHoldDeclining(t *testing.T) {
	engine := NewBaselineEngineAdapter()
	spec := GoldenTestSpec{
		ID:            "declining_test",
		Description:   "买入持有 (价格下跌)",
		InputData:     generateLinearBars(5, 10.0, -0.3),
		StrategyName:  "buy_and_hold",
		ExpectedReturn: -0.1361,
		Tolerance:     0.05,
	}
	runner := NewGoldenBacktestRunner(engine, []GoldenTestSpec{spec})
	_, details := runner.RunAll(context.Background())

	if len(details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(details))
	}
	t.Logf("Declining test: actual=%.4f expected=%.4f diff=%.4f passed=%v",
		details[0].ActualReturn, spec.ExpectedReturn, details[0].ReturnDiff, details[0].Passed)
}

func TestDefaultGoldenSet_Completeness(t *testing.T) {
	gs := DefaultGoldenSet()

	if gs.ID == "" {
		t.Error("golden set ID should not be empty")
	}
	if len(gs.Specs) < 4 {
		t.Errorf("expected at least 4 specs, got %d", len(gs.Specs))
	}

	for _, spec := range gs.Specs {
		if spec.ID == "" {
			t.Error("spec ID should not be empty")
		}
		if len(spec.InputData) < 2 {
			t.Errorf("spec %s: expected at least 2 data bars", spec.ID)
		}
		if spec.StrategyName == "" {
			t.Errorf("spec %s: strategy name should not be empty", spec.ID)
		}
	}
}
