package experiment

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// ExperimentRunner 测试
// ============================================================================

func TestExperimentRunner_RunSuccess(t *testing.T) {
	registry := NewInMemoryRegistry()
	runner := NewExperimentRunner(registry)
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	executor := &mockExecutor{
		metrics: MetricSet{
			SharpeRatio: 2.5,
			MaxDrawdown: -0.05,
			TotalReturn: 0.15,
		},
		artifacts: []Artifact{
			{Type: ArtifactMetrics, Name: "metrics"},
		},
	}

	run, err := runner.Run(context.Background(), exp, executor)
	if err != nil {
		t.Fatal(err)
	}

	// 验证运行状态
	if run.Status != RunCompleted {
		t.Errorf("Expected run status completed, got %s", run.Status)
	}
	if run.Metrics.SharpeRatio != 2.5 {
		t.Error("Should have correct metrics")
	}

	// 验证实验状态
	updatedExp, _ := registry.GetByID(exp.ID)
	if updatedExp.Status != StatusCompleted {
		t.Errorf("Expected experiment status completed, got %s", updatedExp.Status)
	}

	// 验证运行记录
	runs, _ := registry.ListRuns(exp.ID)
	if len(runs) != 1 {
		t.Errorf("Expected 1 run, got %d", len(runs))
	}
}

func TestExperimentRunner_RunFailure(t *testing.T) {
	registry := NewInMemoryRegistry()
	runner := NewExperimentRunner(registry)
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	testError := errors.New("execution failed: market data unavailable")
	executor := &mockExecutor{err: testError}

	_, err := runner.Run(context.Background(), exp, executor)
	if err == nil {
		t.Error("Expected error from failed execution")
	}

	// 验证实验状态
	updatedExp, _ := registry.GetByID(exp.ID)
	if updatedExp.Status != StatusFailed {
		t.Errorf("Expected experiment status failed, got %s", updatedExp.Status)
	}

	// 验证运行记录
	runs, _ := registry.ListRuns(exp.ID)
	if len(runs) != 1 {
		t.Errorf("Expected 1 run, got %d", len(runs))
	}
	if runs[0].Status != RunFailed {
		t.Errorf("Expected run status failed, got %s", runs[0].Status)
	}
	if runs[0].ErrorMessage != testError.Error() {
		t.Error("Error message should be preserved")
	}
}

func TestExperimentRunner_CancelledContext(t *testing.T) {
	registry := NewInMemoryRegistry()
	runner := NewExperimentRunner(registry)
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	// 创建带取消的上下文
	ctx, cancel := context.WithCancel(context.Background())

	// 取消上下文
	cancel()

	executor := &mockExecutor{
		metrics: MetricSet{SharpeRatio: 1.0},
	}

	_, err := runner.Run(ctx, exp, executor)
	if err == nil {
		t.Error("Expected error from cancelled context")
	}

	// 验证实验状态 (应标记为失败或取消)
	updatedExp, _ := registry.GetByID(exp.ID)
	t.Logf("Experiment status after cancelled context: %s", updatedExp.Status)
}

func TestExperimentRunner_ReproducibleRerun(t *testing.T) {
	registry := NewInMemoryRegistry()
	runner := NewExperimentRunner(registry)
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	executor := &mockExecutor{metrics: MetricSet{SharpeRatio: 2.0}}

	// 第一次运行
	_, err := runner.Run(context.Background(), exp, executor)
	if err != nil {
		t.Fatal(err)
	}

	// 同一实验允许用同一冻结配置重跑，以验证结果哈希可复现。
	second, err := runner.Run(context.Background(), exp, executor)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := registry.ListRuns(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
	if runs[0].ResultHash != second.ResultHash {
		t.Fatalf("result hashes differ: %s vs %s", runs[0].ResultHash, second.ResultHash)
	}
}

// ============================================================================
// 可复现性验证测试
// ============================================================================

func TestReproducibilityValidator_NoRuns(t *testing.T) {
	registry := NewInMemoryRegistry()
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	validator := NewReproducibilityValidator(registry)
	result, err := validator.Validate(exp.ID)
	if err != nil {
		t.Fatal(err)
	}

	if result.Reproducible {
		t.Error("Experiment with no runs should not be reproducible")
	}
	if result.NumRuns != 0 {
		t.Errorf("Expected 0 runs, got %d", result.NumRuns)
	}
}

func TestReproducibilityValidator_SingleRun(t *testing.T) {
	registry := NewInMemoryRegistry()
	runner := NewExperimentRunner(registry)
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	executor := &mockExecutor{metrics: MetricSet{SharpeRatio: 2.0}}
	_, err := runner.Run(context.Background(), exp, executor)
	if err != nil {
		t.Fatal(err)
	}

	validator := NewReproducibilityValidator(registry)
	result, err := validator.Validate(exp.ID)
	if err != nil {
		t.Fatal(err)
	}

	if result.NumRuns != 1 {
		t.Errorf("Expected 1 run, got %d", result.NumRuns)
	}
	// 单次运行无法验证可复现性, 但标记为 true (默认)
	t.Logf("Reproducible: %v, Issues: %v", result.Reproducible, result.Issues)
}

func TestReproducibilityValidator_MultipleRuns(t *testing.T) {
	registry := NewInMemoryRegistry()
	runner := NewExperimentRunner(registry)
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	executor := &mockExecutor{metrics: MetricSet{SharpeRatio: 2.0, TotalReturn: 0.15}}

	// 由于实验状态限制, 需要创建新实验进行第二次运行
	exp1 := createTestExperiment()
	exp1.Name = "run-1"
	exp2 := createTestExperiment()
	exp2.Name = "run-2"

	// 确保配置相同
	exp2.Config = exp1.Config
	exp2.ConfigHash = exp1.ConfigHash

	if err := registry.Create(exp1); err != nil {
		t.Fatal(err)
	}
	if err := registry.Create(exp2); err != nil {
		t.Fatal(err)
	}

	// 运行两次 (不同实验 ID, 但相同配置)
	_, err1 := runner.Run(context.Background(), exp1, executor)
	_, err2 := runner.Run(context.Background(), exp2, executor)

	if err1 != nil || err2 != nil {
		t.Fatal("Both runs should succeed")
	}

	// 验证: 两个不同实验 (但相同配置) 的结果可复现吗?
	// 实际上这验证的是相同配置的不同实验是否产生相同结果
	run1, _ := registry.ListRuns(exp1.ID)
	run2, _ := registry.ListRuns(exp2.ID)

	if len(run1) == 1 && len(run2) == 1 {
		comparison := CompareExperimentRuns(run1[0], run2[0])
		t.Logf("Comparison between two runs with same config: Identical=%v", comparison.Identical)
		for k, v := range comparison.Differences {
			t.Logf("  Difference: %s = %v", k, v)
		}

		if !comparison.Identical {
			t.Error("Same config should produce identical results (deterministic execution)")
		}
	}
}

func TestReproducibilityValidator_DifferentResults(t *testing.T) {
	// 验证不同结果的检测
	run1 := NewRun("exp-1", "hash-abc")
	run2 := NewRun("exp-1", "hash-abc")

	metrics1 := MetricSet{SharpeRatio: 2.0}
	metrics2 := MetricSet{SharpeRatio: 2.5}

	run1.Metrics = &metrics1
	run2.Metrics = &metrics2

	comparison := CompareExperimentRuns(run1, run2)

	if comparison.Identical {
		t.Error("Different Sharpe ratios should not be identical")
	}

	if metricsDiff, ok := comparison.Differences["metrics"].(map[string]interface{}); !ok || metricsDiff == nil {
		t.Error("Should detect metrics differences")
	}
}

// ============================================================================
// 集成测试
// ============================================================================

func TestExperiment_IntegrationWorkflow(t *testing.T) {
	registry := NewInMemoryRegistry()
	runner := NewExperimentRunner(registry)

	// 1. 创建实验
	config := createTestConfig()
	exp := NewExperiment("integration-test", "Integration test workflow", config)

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	// 2. 验证初始状态
	if exp.Status != StatusDraft {
		t.Error("Initial status should be draft")
	}

	// 3. 执行实验
	executor := &mockExecutor{
		metrics: MetricSet{
			SharpeRatio:  2.5,
			SortinoRatio: 3.2,
			MaxDrawdown:  -0.05,
			TotalReturn:  0.15,
			AnnualReturn: 0.22,
			WinRate:      0.55,
			TotalTrades:  100,
			ProfitFactor: 1.5,
			Volatility:   0.12,
			GrossPnL:     150000,
			NetPnL:       145000,
		},
		artifacts: []Artifact{
			{
				Type:      ArtifactMetrics,
				Name:      "performance_metrics",
				Content:   mustJSON(MetricSet{SharpeRatio: 2.5}),
				CreatedAt: time.Now(),
			},
			{
				Type:      ArtifactLog,
				Name:      "execution_log",
				Content:   []byte("Experiment completed successfully"),
				CreatedAt: time.Now(),
			},
		},
	}

	run, err := runner.Run(context.Background(), exp, executor)
	if err != nil {
		t.Fatal(err)
	}

	// 4. 验证运行结果
	if run.Status != RunCompleted {
		t.Errorf("Run status should be completed, got %s", run.Status)
	}
	if len(run.Artifacts) != 2 {
		t.Errorf("Expected 2 artifacts, got %d", len(run.Artifacts))
	}
	if run.Duration == 0 {
		t.Error("Duration should be set")
	}

	// 5. 验证实验状态
	updatedExp, _ := registry.GetByID(exp.ID)
	if updatedExp.Status != StatusCompleted {
		t.Error("Experiment status should be completed")
	}

	// 6. 获取运行记录
	runs, err := registry.ListRuns(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Errorf("Expected 1 run, got %d", len(runs))
	}

	// 7. 验证可复现性 (单次运行)
	validator := NewReproducibilityValidator(registry)
	result, err := validator.Validate(exp.ID)
	if err != nil {
		t.Fatal(err)
	}

	if result.NumRuns != 1 {
		t.Errorf("Expected 1 run, got %d", result.NumRuns)
	}

	t.Logf("Integration test passed!")
	t.Logf("  Experiment: %s", exp.Name)
	t.Logf("  Status: %s", updatedExp.Status)
	t.Logf("  Runs: %d", len(runs))
	t.Logf("  Duration: %s", run.DurationString())
	t.Logf("  Sharpe Ratio: %.2f", run.Metrics.SharpeRatio)
	t.Logf("  Reproducible: %v", result.Reproducible)
}

func TestExperiment_FailedWorkflow(t *testing.T) {
	registry := NewInMemoryRegistry()
	runner := NewExperimentRunner(registry)

	exp := createTestExperiment()
	exp.Name = "failed-test"

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	// 执行失败
	failureErr := fmt.Errorf("market data unavailable for date 2024-01-15")
	executor := &mockExecutor{err: failureErr}

	_, err := runner.Run(context.Background(), exp, executor)
	if err == nil {
		t.Error("Expected error")
	}

	// 验证错误信息保留
	runs, _ := registry.ListRuns(exp.ID)
	if len(runs) != 1 {
		t.Fatal("Should have 1 run")
	}

	if runs[0].ErrorMessage != failureErr.Error() {
		t.Error("Error message should be preserved")
	}

	if runs[0].Reproducible {
		t.Error("Failed runs should not be marked as reproducible")
	}

	t.Log("Failed workflow preserves diagnostic information correctly")
}

// ============================================================================
// 工具函数测试
// ============================================================================

func TestWaitForCompletion_Timeout(t *testing.T) {
	registry := NewInMemoryRegistry()
	runner := NewExperimentRunner(registry)
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	// 模拟正在运行
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 不实际运行, 但测试超时机制
	err := WaitForCompletion(ctx, runner, "nonexistent", 100*time.Millisecond)
	if err == nil {
		t.Log("Wait returned nil for non-running experiment")
	}
}

func TestConfigHash_Deterministic(t *testing.T) {
	// 相同配置应产生相同哈希 (可复现性基础)
	configs := []ExperimentConfig{
		createTestConfig(),
		createTestConfig(),
		createTestConfig(),
	}

	hashes := make([]string, 3)
	for i, c := range configs {
		hashes[i] = c.ComputeHash()
	}

	// 所有哈希应相同
	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Errorf("Hash mismatch at index %d: %s vs %s", i, hashes[i], hashes[0])
		}
	}

	t.Logf("All hashes identical (deterministic): %s", hashes[0])
}
