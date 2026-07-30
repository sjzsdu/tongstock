package quality

import (
	"testing"
	"time"
)

func TestUnifiedQualityGate_NoInput(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:   "test-1",
		SourceType: "system",
		RunID:      "run-1",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
	}

	report := uqg.Evaluate(opts)

	if report.ID == "" {
		t.Error("报告 ID 为空")
	}
	if report.SourceID != "test-1" {
		t.Errorf("SourceID = %s, 期望 test-1", report.SourceID)
	}
	if report.Summary.TotalGates != 6 {
		t.Errorf("总门数 = %d, 期望 6", report.Summary.TotalGates)
	}
}

func TestUnifiedQualityGate_WithKlineData(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	records := []KlineRecord{
		{Date: time.Now().Add(-7 * 24 * time.Hour), Open: 10.0, High: 10.5, Low: 9.8, Close: 10.2, Volume: 1000},
		{Date: time.Now().Add(-6 * 24 * time.Hour), Open: 10.2, High: 10.8, Low: 10.0, Close: 10.6, Volume: 1200},
		{Date: time.Now().Add(-5 * 24 * time.Hour), Open: 10.6, High: 11.0, Low: 10.4, Close: 10.8, Volume: 1100},
	}

	opts := EvaluateOptions{
		SourceID:   "test-kline",
		SourceType: "system",
		RunID:      "run-kline",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
		KlineData:  map[string][]KlineRecord{"000001.SZ": records},
	}

	report := uqg.Evaluate(opts)

	found := false
	for _, gate := range report.Gates {
		if gate.Type == GateDataQuality {
			found = true
			if gate.Status == GateSkipped {
				t.Error("数据质量门不应被跳过")
			}
			break
		}
	}
	if !found {
		t.Error("未找到数据质量门结果")
	}
}

func TestUnifiedQualityGate_WithBacktest(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:   "test-backtest",
		SourceType: "paradigm",
		RunID:      "run-backtest",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
		BacktestResults: &BacktestGoldenResult{
			TestPassed:  true,
			TestCount:   5,
			FailCount:   0,
			TestHash:    "abc123",
			GoldenHash:  "abc123",
			Regressed:   false,
			Description: "All tests passed",
		},
	}

	report := uqg.Evaluate(opts)

	found := false
	for _, gate := range report.Gates {
		if gate.Type == GateBacktestGolden {
			found = true
			if gate.Status == GateSkipped {
				t.Error("回测黄金集门不应被跳过")
			}
			if gate.Score < 0 {
				t.Error("回测分数不应为负")
			}
			break
		}
	}
	if !found {
		t.Error("未找到回测黄金集门结果")
	}
}

func TestUnifiedQualityGate_WithCriticalFailure(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:       "test-block",
		SourceType:     "system",
		RunID:          "run-block",
		AsOfDate:       time.Now(),
		HasBackup:      false,
		CanDegrade:     false,
		ManualOverride: false,
	}

	report := uqg.Evaluate(opts)

	if !report.Blocked {
		t.Error("无备份且不能降级时应该被阻止")
	}
	if report.Status != GateBlock {
		t.Errorf("状态应该是 block, 实际是 %s", report.Status)
	}
	if len(report.RecoveryPlan.RecoverySteps) == 0 {
		t.Error("应有恢复步骤")
	}
}

func TestUnifiedQualityGate_WithParadigm(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:   "test-paradigm",
		SourceType: "paradigm",
		RunID:      "run-paradigm",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
		ParadigmScore: &ParadigmScoreInput{
			Stage:         "acceleration",
			Score:         75,
			GateThreshold: 60,
			Decision:      "promote",
			Transitions:   3,
			EvidenceCount: 8,
		},
	}

	report := uqg.Evaluate(opts)

	found := false
	for _, gate := range report.Gates {
		if gate.Type == GateParadigmStage {
			found = true
			if gate.Status == GateSkipped {
				t.Error("范式阶段门不应被跳过")
			}
			break
		}
	}
	if !found {
		t.Error("未找到范式阶段门结果")
	}
}

func TestUnifiedQualityGate_WithAI(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:   "test-ai",
		SourceType: "system",
		RunID:      "run-ai",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
		AIEvaluation: &AIEvaluationInput{
			ModelVersion:  "1.0",
			Accuracy:     0.85,
			Consistency:  0.95,
			DriftDetected: false,
			LastEvalDate: time.Now(),
			Passed:       true,
		},
	}

	report := uqg.Evaluate(opts)

	found := false
	for _, gate := range report.Gates {
		if gate.Type == GateAIEvaluation {
			found = true
			if gate.Status == GateSkipped {
				t.Error("AI 评测门不应被跳过")
			}
			break
		}
	}
	if !found {
		t.Error("未找到 AI 评测门结果")
	}
}

func TestUnifiedQualityGate_WithForward(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:   "test-forward",
		SourceType: "system",
		RunID:      "run-forward",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
		ForwardReport: &ForwardMonitorInput{
			HealthScore:    92,
			DriftDetected:  false,
			DecayDetected:  false,
			AlertCount:     0,
			CriticalAlerts: 0,
			Passed:         true,
		},
	}

	report := uqg.Evaluate(opts)

	found := false
	for _, gate := range report.Gates {
		if gate.Type == GateForwardMonitoring {
			found = true
			if gate.Status == GateSkipped {
				t.Error("前向监控门不应被跳过")
			}
			break
		}
	}
	if !found {
		t.Error("未找到前向监控门结果")
	}
}

func TestUnifiedQualityGate_DegradationModes(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	tests := []struct {
		name       string
		hasBackup  bool
		canDegrade bool
		wantStatus string
	}{
		{"full operation", true, true, "ready"},
		{"no backup", false, true, "degraded"},
		{"no backup no degrade", false, false, "not_ready"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := EvaluateOptions{
				SourceID:   tt.name,
				SourceType: "system",
				RunID:      "run-" + tt.name,
				AsOfDate:   time.Now(),
				HasBackup:  tt.hasBackup,
				CanDegrade: tt.canDegrade,
			}

			report := uqg.Evaluate(opts)

			if report.RecoveryPlan.Status != tt.wantStatus {
				t.Errorf("恢复状态 = %s, 期望 %s", report.RecoveryPlan.Status, tt.wantStatus)
			}
		})
	}
}

func TestUnifiedQualityGate_ManualOverride(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:       "test-override",
		SourceType:     "system",
		RunID:          "run-override",
		AsOfDate:       time.Now(),
		HasBackup:      false,
		CanDegrade:     false,
		ManualOverride: true,
	}

	report := uqg.Evaluate(opts)

	if report.Blocked {
		t.Error("有手动覆盖时不应被阻止")
	}
	if !report.RecoveryPlan.ManualOverrideAllowed {
		t.Error("恢复计划应允许手动覆盖")
	}
}

func TestUnifiedQualityGate_OverallScore(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	records := []KlineRecord{
		{Date: time.Now().Add(-7 * 24 * time.Hour), Open: 10.0, High: 10.5, Low: 9.8, Close: 10.2, Volume: 1000},
		{Date: time.Now().Add(-6 * 24 * time.Hour), Open: 10.2, High: 10.8, Low: 10.0, Close: 10.6, Volume: 1200},
	}

	opts := EvaluateOptions{
		SourceID:   "test-score",
		SourceType: "system",
		RunID:      "run-score",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
		KlineData:  map[string][]KlineRecord{"000001.SZ": records},
		BacktestResults: &BacktestGoldenResult{
			TestPassed: true, TestCount: 5, FailCount: 0,
			TestHash: "abc", GoldenHash: "abc",
		},
		ParadigmScore: &ParadigmScoreInput{
			Stage: "acceleration", Score: 75, GateThreshold: 60, Decision: "promote",
		},
		AIEvaluation: &AIEvaluationInput{
			ModelVersion: "1.0", Accuracy: 0.85, Consistency: 0.95, Passed: true,
		},
		ForwardReport: &ForwardMonitorInput{
			HealthScore: 92, Passed: true,
		},
	}

	report := uqg.Evaluate(opts)

	if report.Score < 0 || report.Score > 100 {
		t.Errorf("综合分 %.1f 应在 0-100 范围内", report.Score)
	}

	nonSkipped := 0
	for _, gate := range report.Gates {
		if gate.Status != GateSkipped {
			nonSkipped++
		}
	}
	if nonSkipped < 5 {
		t.Errorf("应有至少 5 个非跳过门, 实际 %d", nonSkipped)
	}
}

func TestUnifiedQualityGate_DecisionMessages(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:   "test-decision",
		SourceType: "system",
		RunID:      "run-decision",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
	}
	report := uqg.Evaluate(opts)

	if report.Decision == "" {
		t.Error("应有决策描述")
	}
	t.Logf("决策: %s", report.Decision)
}

func TestUnifiedQualityGate_DisableGates(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	config.EnableDataQuality = false
	config.EnableBacktestGolden = false
	config.EnableParadigmStage = false
	config.EnableAIEvaluation = false
	config.EnableForwardMonitoring = false
	config.EnableRecoveryReadiness = true
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:   "test-disabled",
		SourceType: "system",
		RunID:      "run-disabled",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
	}

	report := uqg.Evaluate(opts)

	if report.Summary.TotalGates != 1 {
		t.Errorf("禁用后总门数 = %d, 期望 1", report.Summary.TotalGates)
	}
}

func TestUnifiedQualityGate_New(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	if uqg == nil {
		t.Fatal("NewUnifiedQualityGate 返回 nil")
	}
	if uqg.config != config {
		t.Error("配置未正确设置")
	}
}

func TestUnifiedQualityGate_SummaryString(t *testing.T) {
	config := DefaultUnifiedGateConfig()
	uqg := NewUnifiedQualityGate(config)

	opts := EvaluateOptions{
		SourceID:   "test-summary",
		SourceType: "system",
		RunID:      "run-summary",
		AsOfDate:   time.Now(),
		HasBackup:  true,
		CanDegrade: true,
	}

	report := uqg.Evaluate(opts)
	summary := report.SummaryString()

	if summary == "" {
		t.Error("SummaryString 不应为空")
	}
	t.Logf("SummaryString: %s", summary)
}
