// Package experiment 实现可复现实验注册表与执行引擎。
//
// 核心能力:
//   - 实验配置注册: 保存策略/数据/特征/切分配置
//   - 执行状态跟踪: 运行/完成/失败
//   - 结果制品管理: 指标/图表/模型
//   - 可复现性保证: 相同配置可重跑
package experiment

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"
)

// ============================================================================
// 实验状态
// ============================================================================

// ExperimentStatus 实验状态。
type ExperimentStatus string

const (
	StatusDraft     ExperimentStatus = "draft"     // 草稿
	StatusRunning   ExperimentStatus = "running"   // 运行中
	StatusCompleted ExperimentStatus = "completed" // 已完成
	StatusFailed    ExperimentStatus = "failed"    // 失败
	StatusCancelled ExperimentStatus = "cancelled" // 已取消
)

// ExperimentRunStatus 实验运行状态。
type ExperimentRunStatus string

const (
	RunPending   ExperimentRunStatus = "pending"
	RunRunning   ExperimentRunStatus = "running"
	RunCompleted ExperimentRunStatus = "completed"
	RunFailed    ExperimentRunStatus = "failed"
	RunCancelled ExperimentRunStatus = "cancelled"
)

// ============================================================================
// 环境信息
// ============================================================================

// EnvironmentInfo 运行环境信息 (用于可复现性)。
type EnvironmentInfo struct {
	// GoVersion Go 语言版本
	GoVersion string `json:"go_version"`
	// OS 操作系统
	OS string `json:"os"`
	// Arch CPU 架构
	Arch string `json:"arch"`
	// NumCPU CPU 核心数
	NumCPU int `json:"num_cpu"`
	// GitCommit Git 提交哈希
	GitCommit string `json:"git_commit"`
	// GitBranch Git 分支
	GitBranch string `json:"git_branch"`
	// Dependencies 依赖版本
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

// DetectEnvironment 检测当前运行环境。
func DetectEnvironment() EnvironmentInfo {
	info := EnvironmentInfo{
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
	}

	// 尝试获取 Git 信息 (如果在 Git 仓库中)
	gitCommit, err := getGitCommit()
	if err == nil {
		info.GitCommit = gitCommit
	}

	return info
}

// getGitCommit 获取 Git 提交哈希。
func getGitCommit() (string, error) {
	// 简化实现: 从环境变量读取
	commit := os.Getenv("GIT_COMMIT")
	if commit != "" {
		return commit, nil
	}
	return "", fmt.Errorf("git commit not available")
}

// ============================================================================
// 实验配置
// ============================================================================

// ExperimentConfig 实验配置快照 (用于可复现)。
type ExperimentConfig struct {
	// StrategyName 策略名称
	StrategyName string `json:"strategy_name"`
	// StrategyVersion 策略版本
	StrategyVersion string `json:"strategy_version"`
	// DataSnapshotID 数据快照 ID
	DataSnapshotID string `json:"data_snapshot_id"`
	// FeatureSpecs 使用的特征规格及版本
	FeatureSpecs []FeatureRef `json:"feature_specs,omitempty"`
	// SplitConfig 切分配置
	SplitConfig SplitConfigRef `json:"split_config"`
	// RandomSeed 随机种子
	RandomSeed int64 `json:"random_seed"`
	// InitialCash 初始资金
	InitialCash float64 `json:"initial_cash"`
	// CommissionRate 佣金率
	CommissionRate float64 `json:"commission_rate"`
	// MinCommission 最低佣金
	MinCommission float64 `json:"min_commission"`
	// StampDutyRate 卖出印花税率
	StampDutyRate float64 `json:"stamp_duty_rate"`
	// TransferFeeRate 双边过户费率
	TransferFeeRate float64 `json:"transfer_fee_rate"`
	// SlippageBps 滑点 (bps)
	SlippageBps float64 `json:"slippage_bps"`
	// MaxPositionSize 最大仓位比例
	MaxPositionSize float64 `json:"max_position_size"`
	// StopLossRatio 止损比例
	StopLossRatio float64 `json:"stop_loss_ratio,omitempty"`
	// TakeProfitRatio 止盈比例
	TakeProfitRatio float64 `json:"take_profit_ratio,omitempty"`
	// StrategyParams 策略参数
	StrategyParams map[string]interface{} `json:"strategy_params,omitempty"`
	// KType 冻结行情周期
	KType uint8 `json:"ktype"`
	// Board A 股板块
	Board string `json:"board"`
	// EnableT1 是否启用 T+1
	EnableT1 bool `json:"enable_t_1"`
	// EnablePriceLimit 是否启用涨跌停约束
	EnablePriceLimit bool `json:"enable_price_limit"`
}

// FeatureRef 特征引用。
type FeatureRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Name    string `json:"name"`
}

// SplitConfigRef 切分配置引用。
type SplitConfigRef struct {
	Type            string  `json:"type"`
	TrainRatio      float64 `json:"train_ratio,omitempty"`
	ValidRatio      float64 `json:"valid_ratio,omitempty"`
	EmbargoDays     int     `json:"embargo_days"`
	PurgeDays       int     `json:"purge_days"`
	MinTrainSize    int     `json:"min_train_size,omitempty"`
	Windows         int     `json:"windows,omitempty"`
	TrainWindowDays int     `json:"train_window_days,omitempty"`
	ValidWindowDays int     `json:"valid_window_days,omitempty"`
	TestWindowDays  int     `json:"test_window_days,omitempty"`
	StepDays        int     `json:"step_days,omitempty"`
}

// ComputeHash 计算配置哈希 (用于复现检测)。
func (c ExperimentConfig) ComputeHash() string {
	data, _ := json.Marshal(c)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:8])
}

// ============================================================================
// 实验
// ============================================================================

// Experiment 实验定义。
type Experiment struct {
	// ID 实验唯一标识
	ID string `json:"id"`
	// Name 实验名称
	Name string `json:"name"`
	// Description 实验描述
	Description string `json:"description"`
	// Status 实验状态
	Status ExperimentStatus `json:"status"`
	// Config 实验配置
	Config ExperimentConfig `json:"config"`
	// ConfigHash 配置哈希
	ConfigHash string `json:"config_hash"`
	// Environment 运行环境信息
	Environment EnvironmentInfo `json:"environment"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
	// CompletedAt 完成时间
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// CreatedBy 创建者
	CreatedBy string `json:"created_by,omitempty"`
	// Tags 标签
	Tags []string `json:"tags,omitempty"`
}

// NewExperiment 创建新实验。
func NewExperiment(name, description string, config ExperimentConfig) *Experiment {
	now := time.Now()
	env := DetectEnvironment()

	return &Experiment{
		ID:          fmt.Sprintf("exp-%d", now.UnixNano()),
		Name:        name,
		Description: description,
		Status:      StatusDraft,
		Config:      config,
		ConfigHash:  config.ComputeHash(),
		Environment: env,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// Start 标记实验为运行中。
func (e *Experiment) Start() {
	e.Status = StatusRunning
	e.UpdatedAt = time.Now()
}

// Complete 标记实验为已完成。
func (e *Experiment) Complete() {
	now := time.Now()
	e.Status = StatusCompleted
	e.CompletedAt = &now
	e.UpdatedAt = now
}

// Fail 标记实验为失败。
func (e *Experiment) Fail() {
	e.Status = StatusFailed
	e.UpdatedAt = time.Now()
}

// Cancel 标记实验为已取消。
func (e *Experiment) Cancel() {
	e.Status = StatusCancelled
	e.UpdatedAt = time.Now()
}

// IsFinished 检查实验是否已结束。
func (e *Experiment) IsFinished() bool {
	return e.Status == StatusCompleted || e.Status == StatusFailed || e.Status == StatusCancelled
}

// ============================================================================
// 实验运行
// ============================================================================

// ArtifactType 制品类型。
type ArtifactType string

const (
	ArtifactMetrics     ArtifactType = "metrics"     // 指标
	ArtifactPlot        ArtifactType = "plot"        // 图表
	ArtifactModel       ArtifactType = "model"       // 模型
	ArtifactReport      ArtifactType = "report"      // 报告
	ArtifactLog         ArtifactType = "log"         // 日志
	ArtifactConfig      ArtifactType = "config"      // 配置
	ArtifactPredictions ArtifactType = "predictions" // 预测结果
	ArtifactSplit       ArtifactType = "split"       // 时间切分
	ArtifactFills       ArtifactType = "fills"       // 成交与拒单
	ArtifactEquity      ArtifactType = "equity"      // 权益曲线
	ArtifactManifest    ArtifactType = "manifest"    // 可复现运行清单
)

// Artifact 实验制品。
type Artifact struct {
	// ID 制品 ID
	ID string `json:"id"`
	// Type 制品类型
	Type ArtifactType `json:"type"`
	// Name 制品名称
	Name string `json:"name"`
	// Description 描述
	Description string `json:"description,omitempty"`
	// Content 内容 (JSON)
	Content json.RawMessage `json:"content,omitempty"`
	// ContentHash 内容哈希，不含 ID 和创建时间
	ContentHash string `json:"content_hash,omitempty"`
	// FilePath 文件路径 (如果存储在文件中)
	FilePath string `json:"file_path,omitempty"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
}

// MetricSet 指标集合。
type MetricSet struct {
	// SharpeRatio 夏普比率
	SharpeRatio float64 `json:"sharpe_ratio,omitempty"`
	// SortinoRatio 索提诺比率
	SortinoRatio float64 `json:"sortino_ratio,omitempty"`
	// MaxDrawdown 最大回撤
	MaxDrawdown float64 `json:"max_drawdown,omitempty"`
	// TotalReturn 总收益率
	TotalReturn float64 `json:"total_return,omitempty"`
	// AnnualReturn 年化收益率
	AnnualReturn float64 `json:"annual_return,omitempty"`
	// WinRate 胜率
	WinRate float64 `json:"win_rate,omitempty"`
	// TotalTrades 总交易次数
	TotalTrades int `json:"total_trades,omitempty"`
	// ProfitFactor 盈亏比
	ProfitFactor float64 `json:"profit_factor,omitempty"`
	// Volatility 波动率
	Volatility float64 `json:"volatility,omitempty"`
	// GrossPnL 毛收益
	GrossPnL float64 `json:"gross_pnl,omitempty"`
	// NetPnL 净收益
	NetPnL float64 `json:"net_pnl,omitempty"`
	// Custom 自定义指标
	Custom map[string]float64 `json:"custom,omitempty"`
}

// ArtifactFromMetrics 从指标创建制品。
func ArtifactFromMetrics(metrics MetricSet) Artifact {
	return Artifact{
		ID:        fmt.Sprintf("art-%d", time.Now().UnixNano()),
		Type:      ArtifactMetrics,
		Name:      "performance_metrics",
		Content:   mustJSON(metrics),
		CreatedAt: time.Now(),
	}
}

// GetMetrics 从制品获取指标。
func (a Artifact) GetMetrics() (MetricSet, error) {
	if a.Type != ArtifactMetrics {
		return MetricSet{}, fmt.Errorf("artifact is not metrics type")
	}

	var metrics MetricSet
	if err := json.Unmarshal(a.Content, &metrics); err != nil {
		return MetricSet{}, fmt.Errorf("unmarshal metrics: %w", err)
	}
	return metrics, nil
}

// ============================================================================
// 实验运行
// ============================================================================

// ExperimentRun 实验运行记录。
type ExperimentRun struct {
	// ID 运行 ID
	ID string `json:"id"`
	// ExperimentID 实验 ID
	ExperimentID string `json:"experiment_id"`
	// Status 运行状态
	Status ExperimentRunStatus `json:"status"`
	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`
	// EndTime 结束时间
	EndTime *time.Time `json:"end_time,omitempty"`
	// Duration 运行时长
	Duration time.Duration `json:"duration,omitempty"`
	// Metrics 运行指标
	Metrics *MetricSet `json:"metrics,omitempty"`
	// Artifacts 制品列表
	Artifacts []Artifact `json:"artifacts,omitempty"`
	// ErrorMessage 错误信息
	ErrorMessage string `json:"error_message,omitempty"`
	// Logs 运行日志
	Logs string `json:"logs,omitempty"`
	// ConfigHash 运行时配置哈希
	ConfigHash string `json:"config_hash"`
	// ResultHash 指标和全部制品内容的稳定哈希
	ResultHash string `json:"result_hash,omitempty"`
	// Reproducible 是否可复现
	Reproducible bool `json:"reproducible"`
	// ReproducibilityNote 可复现性说明
	ReproducibilityNote string `json:"reproducibility_note,omitempty"`
}

// NewRun 创建新的运行记录。
func NewRun(experimentID string, configHash string) *ExperimentRun {
	return &ExperimentRun{
		ID:           fmt.Sprintf("run-%d", time.Now().UnixNano()),
		ExperimentID: experimentID,
		Status:       RunPending,
		StartTime:    time.Now(),
		ConfigHash:   configHash,
	}
}

// Start 标记运行为进行中。
func (r *ExperimentRun) Start() {
	r.Status = RunRunning
}

// Complete 标记运行为已完成。
func (r *ExperimentRun) Complete(metrics MetricSet, artifacts []Artifact) {
	now := time.Now()
	for i := range artifacts {
		if artifacts[i].ContentHash == "" {
			artifacts[i].ContentHash = hashBytes(artifacts[i].Content)
		}
		if artifacts[i].ID == "" {
			artifacts[i].ID = fmt.Sprintf("%s-art-%03d-%s", r.ID, i, artifacts[i].ContentHash[:12])
		}
		if artifacts[i].CreatedAt.IsZero() {
			artifacts[i].CreatedAt = now
		}
	}
	r.Status = RunCompleted
	r.EndTime = &now
	r.Duration = now.Sub(r.StartTime)
	r.Metrics = &metrics
	r.Artifacts = artifacts
	r.ResultHash = computeResultHash(metrics, artifacts)
	r.Reproducible = true
}

// Fail 标记运行为失败。
func (r *ExperimentRun) Fail(err error) {
	now := time.Now()
	r.Status = RunFailed
	r.EndTime = &now
	r.Duration = now.Sub(r.StartTime)
	r.ErrorMessage = err.Error()
	r.Reproducible = false
	r.ReproducibilityNote = "Run failed - check error message for details"
}

// DurationString 获取时长的可读字符串。
func (r *ExperimentRun) DurationString() string {
	if r.Duration == 0 {
		if r.EndTime != nil {
			return r.EndTime.Sub(r.StartTime).String()
		}
		return time.Since(r.StartTime).String()
	}
	return r.Duration.String()
}

// ============================================================================
// 实验注册表 (内存实现)
// ============================================================================

// Registry 实验注册表接口。
type Registry interface {
	// Create 创建实验
	Create(experiment *Experiment) error
	// GetByID 按 ID 获取实验
	GetByID(id string) (*Experiment, error)
	// List 列出所有实验
	List() ([]*Experiment, error)
	// Update 更新实验
	Update(experiment *Experiment) error
	// Delete 删除实验
	Delete(id string) error
	// CreateRun 创建运行记录
	CreateRun(run *ExperimentRun) error
	// GetRun 获取运行记录
	GetRun(runID string) (*ExperimentRun, error)
	// ListRuns 列出实验的所有运行
	ListRuns(experimentID string) ([]*ExperimentRun, error)
	// UpdateRun 更新运行记录
	UpdateRun(run *ExperimentRun) error
}

// InMemoryRegistry 内存注册表 (用于测试)。
type InMemoryRegistry struct {
	experiments map[string]*Experiment
	runs        map[string]*ExperimentRun
	runIndex    map[string][]string // experimentID -> []runID
}

// NewInMemoryRegistry 创建内存注册表。
func NewInMemoryRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{
		experiments: make(map[string]*Experiment),
		runs:        make(map[string]*ExperimentRun),
		runIndex:    make(map[string][]string),
	}
}

func (r *InMemoryRegistry) Create(experiment *Experiment) error {
	if _, exists := r.experiments[experiment.ID]; exists {
		return fmt.Errorf("experiment %s already exists", experiment.ID)
	}
	r.experiments[experiment.ID] = experiment
	return nil
}

func (r *InMemoryRegistry) GetByID(id string) (*Experiment, error) {
	exp, exists := r.experiments[id]
	if !exists {
		return nil, fmt.Errorf("experiment %s not found", id)
	}
	return exp, nil
}

func (r *InMemoryRegistry) List() ([]*Experiment, error) {
	experiments := make([]*Experiment, 0, len(r.experiments))
	for _, exp := range r.experiments {
		experiments = append(experiments, exp)
	}
	return experiments, nil
}

func (r *InMemoryRegistry) Update(experiment *Experiment) error {
	if _, exists := r.experiments[experiment.ID]; !exists {
		return fmt.Errorf("experiment %s not found", experiment.ID)
	}
	experiment.UpdatedAt = time.Now()
	r.experiments[experiment.ID] = experiment
	return nil
}

func (r *InMemoryRegistry) Delete(id string) error {
	if _, exists := r.experiments[id]; !exists {
		return fmt.Errorf("experiment %s not found", id)
	}
	delete(r.experiments, id)

	// 清理相关运行记录
	if runIDs, ok := r.runIndex[id]; ok {
		for _, runID := range runIDs {
			delete(r.runs, runID)
		}
		delete(r.runIndex, id)
	}
	return nil
}

func (r *InMemoryRegistry) CreateRun(run *ExperimentRun) error {
	if _, exists := r.runs[run.ID]; exists {
		return fmt.Errorf("run %s already exists", run.ID)
	}
	r.runs[run.ID] = run
	r.runIndex[run.ExperimentID] = append(r.runIndex[run.ExperimentID], run.ID)
	return nil
}

func (r *InMemoryRegistry) GetRun(runID string) (*ExperimentRun, error) {
	run, exists := r.runs[runID]
	if !exists {
		return nil, fmt.Errorf("run %s not found", runID)
	}
	return run, nil
}

func (r *InMemoryRegistry) ListRuns(experimentID string) ([]*ExperimentRun, error) {
	runIDs, exists := r.runIndex[experimentID]
	if !exists {
		return []*ExperimentRun{}, nil
	}

	runs := make([]*ExperimentRun, 0, len(runIDs))
	for _, runID := range runIDs {
		if run, ok := r.runs[runID]; ok {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

func (r *InMemoryRegistry) UpdateRun(run *ExperimentRun) error {
	if _, exists := r.runs[run.ID]; !exists {
		return fmt.Errorf("run %s not found", run.ID)
	}
	r.runs[run.ID] = run
	return nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// mustJSON 序列化到 JSON (忽略错误)。
func mustJSON(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

// CompareRuns 比较两次运行结果。
type RunComparison struct {
	Run1ID      string                 `json:"run_1_id"`
	Run2ID      string                 `json:"run_2_id"`
	Identical   bool                   `json:"identical"`
	Differences map[string]interface{} `json:"differences,omitempty"`
}

// CompareExperimentRuns 比较两次实验运行。
func CompareExperimentRuns(run1, run2 *ExperimentRun) RunComparison {
	comparison := RunComparison{
		Run1ID:      run1.ID,
		Run2ID:      run2.ID,
		Identical:   true,
		Differences: make(map[string]interface{}),
	}

	// 比较配置哈希
	if run1.ConfigHash != run2.ConfigHash {
		comparison.Identical = false
		comparison.Differences["config_hash"] = map[string]string{
			"run1": run1.ConfigHash,
			"run2": run2.ConfigHash,
		}
	}
	if run1.ResultHash != run2.ResultHash {
		comparison.Identical = false
		comparison.Differences["result_hash"] = map[string]string{
			"run1": run1.ResultHash,
			"run2": run2.ResultHash,
		}
	}

	// 比较指标
	if run1.Metrics != nil && run2.Metrics != nil {
		metricsDiff := compareMetrics(*run1.Metrics, *run2.Metrics)
		if len(metricsDiff) > 0 {
			comparison.Identical = false
			comparison.Differences["metrics"] = metricsDiff
		}
	} else if run1.Metrics == nil || run2.Metrics == nil {
		comparison.Differences["metrics"] = "one run has no metrics"
	}

	return comparison
}

func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:])
}

func computeResultHash(metrics MetricSet, artifacts []Artifact) string {
	type artifactDigest struct {
		Type        ArtifactType `json:"type"`
		Name        string       `json:"name"`
		ContentHash string       `json:"content_hash"`
	}
	digests := make([]artifactDigest, len(artifacts))
	for i, artifact := range artifacts {
		contentHash := artifact.ContentHash
		if contentHash == "" {
			contentHash = hashBytes(artifact.Content)
		}
		digests[i] = artifactDigest{Type: artifact.Type, Name: artifact.Name, ContentHash: contentHash}
	}
	sort.Slice(digests, func(i, j int) bool {
		if digests[i].Type != digests[j].Type {
			return digests[i].Type < digests[j].Type
		}
		if digests[i].Name != digests[j].Name {
			return digests[i].Name < digests[j].Name
		}
		return digests[i].ContentHash < digests[j].ContentHash
	})
	payload := struct {
		Metrics   MetricSet        `json:"metrics"`
		Artifacts []artifactDigest `json:"artifacts"`
	}{Metrics: metrics, Artifacts: digests}
	data, _ := json.Marshal(payload)
	return hashBytes(data)
}

// compareMetrics 比较两组指标。
func compareMetrics(m1, m2 MetricSet) map[string]interface{} {
	diffs := make(map[string]interface{})

	fields := map[string]float64{
		"sharpe_ratio":  m1.SharpeRatio,
		"sortino_ratio": m1.SortinoRatio,
		"max_drawdown":  m1.MaxDrawdown,
		"total_return":  m1.TotalReturn,
		"annual_return": m1.AnnualReturn,
		"win_rate":      m1.WinRate,
		"profit_factor": m1.ProfitFactor,
		"volatility":    m1.Volatility,
		"gross_pnl":     m1.GrossPnL,
		"net_pnl":       m1.NetPnL,
	}

	field2 := map[string]float64{
		"sharpe_ratio":  m2.SharpeRatio,
		"sortino_ratio": m2.SortinoRatio,
		"max_drawdown":  m2.MaxDrawdown,
		"total_return":  m2.TotalReturn,
		"annual_return": m2.AnnualReturn,
		"win_rate":      m2.WinRate,
		"profit_factor": m2.ProfitFactor,
		"volatility":    m2.Volatility,
		"gross_pnl":     m2.GrossPnL,
		"net_pnl":       m2.NetPnL,
	}

	for name, v1 := range fields {
		v2 := field2[name]
		if v1 != v2 {
			diffs[name] = map[string]float64{"run1": v1, "run2": v2}
		}
	}

	return diffs
}
