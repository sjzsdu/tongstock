package quality

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestEndToEndDemo 验证端到端演示流程的可复现性。
func TestEndToEndDemo(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	// 1. 准备测试 K 线数据
	now := time.Now()
	klineData := make(map[string][]KlineRecord)
	klineData["sh000001"] = generateLinearBars(20, 15.0, 0.3)
	klineData["sz399001"] = generateLinearBars(20, 8.0, 0.1)

	// 2. 运行黄金回测
	engine := NewBaselineEngineAdapter()
	gs := DefaultGoldenSet()
	runner := NewGoldenBacktestRunner(engine, gs.Specs)
	btResult, _ := runner.RunAll(context.Background())

	// 3. 组装输入
	expectedDays := make(map[string][]time.Time)
	for code := range klineData {
		days := make([]time.Time, len(klineData[code]))
		for i, r := range klineData[code] {
			days[i] = r.Date
		}
		expectedDays[code] = days
	}

	opts := EvaluateOptions{
		SourceID:        "demo-test",
		SourceType:      "demo",
		RunID:           "demo-run-test",
		KlineData:       klineData,
		ExpectedDays:    expectedDays,
		AsOfDate:        now,
		BacktestResults: btResult,
		ParadigmScore: &ParadigmScoreInput{
			Stage:         "growth",
			Score:         82.5,
			GateThreshold: 70.0,
			Decision:      "advance",
		},
		AIEvaluation: &AIEvaluationInput{
			ModelVersion:  "v2.1.0",
			Accuracy:      0.87,
			Consistency:   0.92,
			DriftDetected: false,
			LastEvalDate:  now,
			Passed:        true,
		},
		ForwardReport: &ForwardMonitorInput{
			HealthScore:   0.88,
			DriftDetected: false,
			DecayDetected: false,
			AlertCount:    0,
			Passed:        true,
		},
		HasBackup:      true,
		LastBackupTime: now.Add(-30 * time.Minute),
		CanDegrade:     true,
	}

	// 4. 运行评估
	report := uqg.Evaluate(opts)

	// 5. 验证报告完整性
	if report.ID == "" {
		t.Error("report ID should not be empty")
	}
	if len(report.Gates) < 3 {
		t.Errorf("expected at least 3 gates, got %d", len(report.Gates))
	}
	if report.Score < 0 || report.Score > 100 {
		t.Errorf("score should be between 0-100, got %.1f", report.Score)
	}

	// 6. 验证 JSON 序列化 (可复现性)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("JSON output should not be empty")
	}

	// 7. 验证每个质量门都有结果
	for _, gate := range report.Gates {
		if gate.Name == "" {
			t.Errorf("gate has empty name: %v", gate)
		}
		if gate.Status == "" {
			t.Errorf("gate %s has empty status", gate.Name)
		}
		t.Logf("  Gate: %s | Status: %s | Score: %.1f", gate.Name, gate.Status, gate.Score)
	}

	t.Logf("Final report: status=%s, score=%.1f, blocked=%v, decision=%s",
		report.Status, report.Score, report.Blocked, report.Decision)
}

// TestEndToEndDemo_Deterministic 验证端到端流程的确定性。
func TestEndToEndDemo_Deterministic(t *testing.T) {
	runDemo := func() *UnifiedQualityReport {
		config := DefaultUnifiedGateConfig()
		uqg := NewUnifiedQualityGate(config)

		now := time.Now()
		klineData := map[string][]KlineRecord{
			"sh000001": generateLinearBars(20, 15.0, 0.3),
		}

		engine := NewBaselineEngineAdapter()
		gs := DefaultGoldenSet()
		runner := NewGoldenBacktestRunner(engine, gs.Specs)
		btResult, _ := runner.RunAll(context.Background())

		return uqg.Evaluate(EvaluateOptions{
			SourceID:        "deterministic-test",
			SourceType:      "demo",
			RunID:           "deterministic-run",
			KlineData:       klineData,
			AsOfDate:        now,
			BacktestResults: btResult,
			ParadigmScore: &ParadigmScoreInput{
				Stage:         "growth",
				Score:         82.5,
				GateThreshold: 70.0,
				Decision:      "advance",
			},
			AIEvaluation: &AIEvaluationInput{
				ModelVersion:  "v2.1.0",
				Accuracy:      0.87,
				Consistency:   0.92,
				DriftDetected: false,
				LastEvalDate:  now,
				Passed:        true,
			},
			ForwardReport: &ForwardMonitorInput{
				HealthScore:   0.88,
				DriftDetected: false,
				DecayDetected: false,
				Passed:        true,
			},
			HasBackup:      true,
			LastBackupTime: now.Add(-30 * time.Minute),
			CanDegrade:     true,
		})
	}

	report1 := runDemo()
	report2 := runDemo()

	// 验证分数和状态一致
	if report1.Status != report2.Status {
		t.Errorf("status not deterministic: %s vs %s", report1.Status, report2.Status)
	}
	if report1.Score != report2.Score {
		t.Errorf("score not deterministic: %.1f vs %.1f", report1.Score, report2.Score)
	}
	if report1.Blocked != report2.Blocked {
		t.Errorf("blocked not deterministic: %v vs %v", report1.Blocked, report2.Blocked)
	}
}
