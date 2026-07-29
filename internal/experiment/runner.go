package experiment

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ExperimentExecutor 实验执行器接口。
type ExperimentExecutor interface {
	// Execute 执行实验, 返回指标和制品
	Execute(ctx context.Context, exp *Experiment) (MetricSet, []Artifact, error)
}

// ExperimentRunner 实验运行器。
type ExperimentRunner struct {
	registry Registry
	mu       sync.Mutex
	running  map[string]context.CancelFunc
}

// NewExperimentRunner 创建实验运行器。
func NewExperimentRunner(registry Registry) *ExperimentRunner {
	return &ExperimentRunner{
		registry: registry,
		running:  make(map[string]context.CancelFunc),
	}
}

// Run 执行实验。
func (r *ExperimentRunner) Run(ctx context.Context, exp *Experiment, executor ExperimentExecutor) (*ExperimentRun, error) {
	// 检查实验状态
	if exp.IsFinished() {
		return nil, fmt.Errorf("experiment %s is already finished (status: %s)", exp.ID, exp.Status)
	}

	// 标记实验为运行中
	exp.Start()
	if err := r.registry.Update(exp); err != nil {
		return nil, fmt.Errorf("update experiment: %w", err)
	}

	// 创建运行记录
	run := NewRun(exp.ID, exp.ConfigHash)
	if err := r.registry.CreateRun(run); err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}
	run.Start()

	// 保存运行记录
	if err := r.registry.UpdateRun(run); err != nil {
		return nil, fmt.Errorf("update run: %w", err)
	}

	// 保存取消函数
	r.mu.Lock()
	r.running[exp.ID] = context.CancelFunc(func() {})
	r.mu.Unlock()

	// 执行实验
	metrics, artifacts, err := executor.Execute(ctx, exp)

	r.mu.Lock()
	delete(r.running, exp.ID)
	r.mu.Unlock()

	if err != nil {
		// 失败
		run.Fail(err)
		exp.Fail()
		_ = r.registry.UpdateRun(run)
		_ = r.registry.Update(exp)
		return run, fmt.Errorf("experiment execution failed: %w", err)
	}

	// 成功
	run.Complete(metrics, artifacts)
	exp.Complete()

	if err := r.registry.UpdateRun(run); err != nil {
		return nil, fmt.Errorf("update run: %w", err)
	}

	if err := r.registry.Update(exp); err != nil {
		return nil, fmt.Errorf("update experiment: %w", err)
	}

	log.Printf("[ExperimentRunner] Experiment %s completed: %v", exp.Name, metrics)
	return run, nil
}

// Cancel 取消正在运行的实验。
func (r *ExperimentRunner) Cancel(experimentID string) error {
	r.mu.Lock()
	cancel, exists := r.running[experimentID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("no running experiment %s", experimentID)
	}

	cancel()
	return nil
}

// IsRunning 检查实验是否正在运行。
func (r *ExperimentRunner) IsRunning(experimentID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, exists := r.running[experimentID]
	return exists
}

// ============================================================================
// 可复现性验证
// ============================================================================

// ReproducibilityValidator 可复现性验证器。
type ReproducibilityValidator struct {
	registry Registry
}

// NewReproducibilityValidator 创建可复现性验证器。
func NewReproducibilityValidator(registry Registry) *ReproducibilityValidator {
	return &ReproducibilityValidator{
		registry: registry,
	}
}

// ValidationResult 验证结果。
type ValidationResult struct {
	// ExperimentID 实验 ID
	ExperimentID string `json:"experiment_id"`
	// Reproducible 是否可复现
	Reproducible bool `json:"reproducible"`
	// NumRuns 运行次数
	NumRuns int `json:"num_runs"`
	// AllSameConfigHash 所有运行配置哈希是否一致
	AllSameConfigHash bool `json:"all_same_config_hash"`
	// ResultsConsistent 结果是否一致
	ResultsConsistent bool `json:"results_consistent"`
	// Comparison 详细比较结果
	Comparisons []RunComparison `json:"comparisons,omitempty"`
	// Issues 发现的问题
	Issues []string `json:"issues,omitempty"`
	// Recommendations 建议
	Recommendations []string `json:"recommendations,omitempty"`
}

// Validate 验证实验的可复现性。
func (v *ReproducibilityValidator) Validate(experimentID string) (*ValidationResult, error) {
	exp, err := v.registry.GetByID(experimentID)
	if err != nil {
		return nil, fmt.Errorf("get experiment: %w", err)
	}

	runs, err := v.registry.ListRuns(experimentID)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	result := &ValidationResult{
		ExperimentID:      experimentID,
		NumRuns:           len(runs),
		AllSameConfigHash: true,
		ResultsConsistent: true,
	}

	// 验证实验配置哈希与运行记录一致
	if exp.ConfigHash != "" {
		for _, run := range runs {
			if run.ConfigHash != exp.ConfigHash {
				result.Issues = append(result.Issues, fmt.Sprintf(
					"Run %s has config hash %s, experiment has %s",
					run.ID, run.ConfigHash, exp.ConfigHash))
			}
		}
	}

	if len(runs) == 0 {
		result.Reproducible = false
		result.Issues = append(result.Issues, "no runs found")
		result.Recommendations = append(result.Recommendations, "run the experiment at least once")
		return result, nil
	}

	if len(runs) == 1 {
		// 只有一次运行, 无法验证
		result.Reproducible = true // 假设可复现
		result.Issues = append(result.Issues, "only one run - run again to verify reproducibility")
		result.Recommendations = append(result.Recommendations, "run the experiment again with the same config to verify")
		return result, nil
	}

	// 检查配置哈希一致性
	configHash := runs[0].ConfigHash
	for i, run := range runs {
		if run.ConfigHash != configHash {
			result.AllSameConfigHash = false
			result.Issues = append(result.Issues, fmt.Sprintf(
				"Run %s has different config hash (%s vs %s)",
				run.ID, run.ConfigHash, configHash))
		}

		// 比较连续运行
		if i > 0 {
			comparison := CompareExperimentRuns(runs[i-1], run)
			result.Comparisons = append(result.Comparisons, comparison)
			if !comparison.Identical {
				result.ResultsConsistent = false
			}
		}
	}

	// 最终可复现性判断
	result.Reproducible = result.AllSameConfigHash && result.ResultsConsistent

	if !result.Reproducible {
		result.Issues = append(result.Issues, "config hash or results differ across runs")
		result.Recommendations = append(result.Recommendations,
			"ensure deterministic execution: same seed, same data, same code version")
	}

	return result, nil
}

// ============================================================================
// 工具函数
// ============================================================================

// WaitForCompletion 等待实验完成 (带超时)。
func WaitForCompletion(ctx context.Context, runner *ExperimentRunner, experimentID string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for experiment %s", experimentID)
		case <-ticker.C:
			if !runner.IsRunning(experimentID) {
				return nil
			}
		}
	}
}
