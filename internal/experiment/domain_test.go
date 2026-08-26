package experiment

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// 辅助函数
// ============================================================================

// createTestConfig 创建测试配置。
func createTestConfig() ExperimentConfig {
	return ExperimentConfig{
		StrategyName:    "dual_ma",
		StrategyVersion: "1.0.0",
		DataSnapshotID:  "snapshot-2024-01",
		FeatureSpecs: []FeatureRef{
			{ID: "f1", Name: "ma_5", Version: 1},
			{ID: "f2", Name: "ma_20", Version: 1},
		},
		SplitConfig: SplitConfigRef{
			Type:        "fixed",
			TrainRatio:  0.6,
			ValidRatio:  0.2,
			EmbargoDays: 5,
			PurgeDays:   3,
		},
		RandomSeed:      42,
		InitialCash:     1000000,
		CommissionRate:  0.00025,
		SlippageBps:     5,
		MaxPositionSize: 1.0,
	}
}

// createTestExperiment 创建测试实验。
func createTestExperiment() *Experiment {
	config := createTestConfig()
	return NewExperiment("test-experiment", "A test experiment", config)
}

// mockExecutor 模拟执行器。
type mockExecutor struct {
	metrics   MetricSet
	artifacts []Artifact
	err       error
	delay     time.Duration
}

func (e *mockExecutor) Execute(ctx context.Context, exp *Experiment) (MetricSet, []Artifact, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return MetricSet{}, nil, err
	}

	if e.delay > 0 {
		select {
		case <-time.After(e.delay):
		case <-ctx.Done():
			return MetricSet{}, nil, ctx.Err()
		}
	}

	if e.err != nil {
		return MetricSet{}, nil, e.err
	}

	return e.metrics, e.artifacts, nil
}

// ============================================================================
// 配置与环境信息测试
// ============================================================================

func TestEnvironmentInfo_Detect(t *testing.T) {
	info := DetectEnvironment()

	if info.GoVersion == "" {
		t.Error("Go version should not be empty")
	}
	if info.OS == "" {
		t.Error("OS should not be empty")
	}
	if info.Arch == "" {
		t.Error("Arch should not be empty")
	}
	if info.NumCPU <= 0 {
		t.Error("NumCPU should be positive")
	}

	t.Logf("Environment: Go=%s, OS=%s, Arch=%s, CPUs=%d",
		info.GoVersion, info.OS, info.Arch, info.NumCPU)
}

func TestExperimentConfig_Hash(t *testing.T) {
	config1 := createTestConfig()
	config2 := createTestConfig()
	config3 := createTestConfig()
	config3.RandomSeed = 123 // 不同的随机种子

	hash1 := config1.ComputeHash()
	hash2 := config2.ComputeHash()
	hash3 := config3.ComputeHash()

	// 相同配置哈希一致
	if hash1 != hash2 {
		t.Error("Identical configs should produce identical hashes")
	}

	// 不同配置哈希不同
	if hash1 == hash3 {
		t.Error("Different configs should produce different hashes")
	}

	t.Logf("Config hash (seed=42): %s", hash1)
	t.Logf("Config hash (seed=123): %s", hash3)
}

// ============================================================================
// 实验状态测试
// ============================================================================

func TestExperiment_Lifecycle(t *testing.T) {
	exp := createTestExperiment()

	// 初始状态
	if exp.Status != StatusDraft {
		t.Errorf("Expected status draft, got %s", exp.Status)
	}

	// 启动
	exp.Start()
	if exp.Status != StatusRunning {
		t.Errorf("Expected status running, got %s", exp.Status)
	}

	// 完成
	exp.Complete()
	if exp.Status != StatusCompleted {
		t.Errorf("Expected status completed, got %s", exp.Status)
	}
	if exp.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	if !exp.IsFinished() {
		t.Error("Completed experiment should be finished")
	}
}

func TestExperiment_FailLifecycle(t *testing.T) {
	exp := createTestExperiment()

	exp.Start()
	exp.Fail()

	if exp.Status != StatusFailed {
		t.Errorf("Expected status failed, got %s", exp.Status)
	}
	if !exp.IsFinished() {
		t.Error("Failed experiment should be finished")
	}
}

func TestExperiment_CancelLifecycle(t *testing.T) {
	exp := createTestExperiment()

	exp.Start()
	exp.Cancel()

	if exp.Status != StatusCancelled {
		t.Errorf("Expected status cancelled, got %s", exp.Status)
	}
	if !exp.IsFinished() {
		t.Error("Cancelled experiment should be finished")
	}
}

// ============================================================================
// 注册表测试
// ============================================================================

func TestRegistry_CreateAndGet(t *testing.T) {
	registry := NewInMemoryRegistry()
	exp := createTestExperiment()

	// 创建
	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	// 获取
	retrieved, err := registry.GetByID(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retrieved.ID != exp.ID {
		t.Error("Retrieved experiment should have same ID")
	}
	if retrieved.Name != exp.Name {
		t.Error("Retrieved experiment should have same name")
	}
}

func TestRegistry_DuplicateCreate(t *testing.T) {
	registry := NewInMemoryRegistry()
	exp := createTestExperiment()

	// 首次创建
	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	// 重复创建
	if err := registry.Create(exp); err == nil {
		t.Error("Duplicate create should fail")
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewInMemoryRegistry()

	// 创建多个实验
	for i := 0; i < 5; i++ {
		exp := createTestExperiment()
		exp.Name = "experiment-" + string(rune('a'+i))
		if err := registry.Create(exp); err != nil {
			t.Fatal(err)
		}
	}

	// 列表
	experiments, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(experiments) != 5 {
		t.Errorf("Expected 5 experiments, got %d", len(experiments))
	}
}

func TestRegistry_Update(t *testing.T) {
	registry := NewInMemoryRegistry()
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	// 更新名称
	exp.Name = "updated-name"
	if err := registry.Update(exp); err != nil {
		t.Fatal(err)
	}

	// 验证
	retrieved, err := registry.GetByID(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retrieved.Name != "updated-name" {
		t.Error("Name should be updated")
	}
}

func TestRegistry_Delete(t *testing.T) {
	registry := NewInMemoryRegistry()
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	// 删除
	if err := registry.Delete(exp.ID); err != nil {
		t.Fatal(err)
	}

	// 尝试获取
	if _, err := registry.GetByID(exp.ID); err == nil {
		t.Error("Deleted experiment should not be found")
	}
}

func TestRegistry_NonExistentGet(t *testing.T) {
	registry := NewInMemoryRegistry()

	if _, err := registry.GetByID("nonexistent"); err == nil {
		t.Error("Getting non-existent experiment should fail")
	}
}

func TestRegistry_RunManagement(t *testing.T) {
	registry := NewInMemoryRegistry()
	exp := createTestExperiment()

	if err := registry.Create(exp); err != nil {
		t.Fatal(err)
	}

	// 创建运行
	run := NewRun(exp.ID, exp.ConfigHash)
	if err := registry.CreateRun(run); err != nil {
		t.Fatal(err)
	}

	// 获取运行
	retrieved, err := registry.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retrieved.ID != run.ID {
		t.Error("Retrieved run should have same ID")
	}

	// 列表运行
	runs, err := registry.ListRuns(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Errorf("Expected 1 run, got %d", len(runs))
	}

	// 更新运行
	run.Start()
	if err := registry.UpdateRun(run); err != nil {
		t.Fatal(err)
	}
}

// ============================================================================
// 运行状态测试
// ============================================================================

func TestExperimentRun_Lifecycle(t *testing.T) {
	exp := createTestExperiment()
	run := NewRun(exp.ID, exp.ConfigHash)

	// 初始状态
	if run.Status != RunPending {
		t.Errorf("Expected status pending, got %s", run.Status)
	}

	// 运行
	run.Start()
	if run.Status != RunRunning {
		t.Errorf("Expected status running, got %s", run.Status)
	}

	// 完成
	metrics := MetricSet{SharpeRatio: 2.5, TotalReturn: 0.15}
	run.Complete(metrics, nil)
	if run.Status != RunCompleted {
		t.Errorf("Expected status completed, got %s", run.Status)
	}
	if run.Metrics.SharpeRatio != 2.5 {
		t.Error("Metrics should be set")
	}
	if run.EndTime == nil {
		t.Error("EndTime should be set")
	}
}

func TestExperimentRun_Fail(t *testing.T) {
	exp := createTestExperiment()
	run := NewRun(exp.ID, exp.ConfigHash)

	run.Start()
	testError := fmt.Errorf("execution failed: market data unavailable")
	run.Fail(testError)

	if run.Status != RunFailed {
		t.Errorf("Expected status failed, got %s", run.Status)
	}
	if run.ErrorMessage != testError.Error() {
		t.Error("Error message should be set")
	}
}

// 测试错误 (未使用, 保留兼容性)
var _ = context.DeadlineExceeded

// ============================================================================
// 指标测试
// ============================================================================

func TestArtifact_Metrics(t *testing.T) {
	metrics := MetricSet{
		SharpeRatio:  2.0,
		MaxDrawdown:  -0.05,
		TotalReturn:  0.15,
		WinRate:      0.55,
		TotalTrades:  100,
		ProfitFactor: 1.5,
	}

	artifact := ArtifactFromMetrics(metrics)

	if artifact.Type != ArtifactMetrics {
		t.Error("Artifact type should be metrics")
	}

	retrieved, err := artifact.GetMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if retrieved.SharpeRatio != 2.0 {
		t.Error("Sharpe ratio should be 2.0")
	}
	if retrieved.TotalReturn != 0.15 {
		t.Error("Total return should be 0.15")
	}
}

func TestArtifact_WrongType(t *testing.T) {
	artifact := Artifact{Type: ArtifactLog, Name: "test_log"}
	if _, err := artifact.GetMetrics(); err == nil {
		t.Error("Getting metrics from log artifact should fail")
	}
}

// ============================================================================
// 比较测试
// ============================================================================

func TestCompareExperimentRuns_Identical(t *testing.T) {
	config := createTestConfig()
	run1 := NewRun("exp-1", config.ComputeHash())
	run2 := NewRun("exp-1", config.ComputeHash())

	metrics := MetricSet{SharpeRatio: 2.0, TotalReturn: 0.15}
	run1.Metrics = &metrics
	run2.Metrics = &metrics

	comparison := CompareExperimentRuns(run1, run2)

	if !comparison.Identical {
		t.Error("Identical runs should be detected as identical")
	}
}

func TestCompareExperimentRuns_DifferentMetrics(t *testing.T) {
	config := createTestConfig()
	run1 := NewRun("exp-1", config.ComputeHash())
	run2 := NewRun("exp-1", config.ComputeHash())

	metrics1 := MetricSet{SharpeRatio: 2.0, TotalReturn: 0.15}
	metrics2 := MetricSet{SharpeRatio: 2.5, TotalReturn: 0.20}
	run1.Metrics = &metrics1
	run2.Metrics = &metrics2

	comparison := CompareExperimentRuns(run1, run2)

	if comparison.Identical {
		t.Error("Different metrics should not be identical")
	}
	if comparison.Differences["metrics"] == nil {
		t.Error("Should have metrics differences")
	}
}

func TestCompareExperimentRuns_DifferentConfig(t *testing.T) {
	config1 := createTestConfig()
	config2 := createTestConfig()
	config2.RandomSeed = 999

	run1 := NewRun("exp-1", config1.ComputeHash())
	run2 := NewRun("exp-1", config2.ComputeHash())

	comparison := CompareExperimentRuns(run1, run2)

	if comparison.Identical {
		t.Error("Different config hashes should not be identical")
	}
}
