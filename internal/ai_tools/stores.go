package ai_tools

import (
	"fmt"
	"time"
)

// ============================================================================
// 前向观察工具
// ============================================================================

// ForwardRunSummary 前向运行摘要
type ForwardRunSummary struct {
	ID                string    `json:"id"`
	ParadigmVersionID string    `json:"paradigm_version_id"`
	StartDate         time.Time `json:"start_date"`
	EndDate           time.Time `json:"end_date"`
	SignalsCount      int       `json:"signals_count"`
	TotalReturn       float64   `json:"total_return"`
	MaxDrawdown       float64   `json:"max_drawdown"`
	SharpeRatio       float64   `json:"sharpe_ratio"`
	Passed            bool      `json:"passed"`
	CreatedAt         time.Time `json:"created_at"`
}

// SignalDetail 交易信号详情
type SignalDetail struct {
	ID                string    `json:"id"`
	ParadigmVersionID string    `json:"paradigm_version_id"`
	StockCode         string    `json:"stock_code"`
	Direction         string    `json:"direction"`
	TriggeredAt       time.Time `json:"triggered_at"`
	Price             float64   `json:"price"`
	Confidence        float64   `json:"confidence"`
	RealizedPnL       *float64  `json:"realized_pnl,omitempty"`
}

// ForwardRunRepository 前向运行仓储接口
type ForwardRunRepository interface {
	ListLatest(n int) []ForwardRunSummary
	GetByID(id string) (*ForwardRunSummary, error)
	ListByParadigm(paradigmVersionID string) []ForwardRunSummary
	GetSignals(runID string) ([]SignalDetail, error)
}

// ForwardRunTool 前向观察工具
type ForwardRunTool struct {
	repo    ForwardRunRepository
	guard   *ReadOnlyGuard
	version string
}

// NewForwardRunTool 创建前向观察工具
func NewForwardRunTool(repo ForwardRunRepository) *ForwardRunTool {
	return &ForwardRunTool{
		repo:    repo,
		guard:   NewReadOnlyGuard(),
		version: "1.0.0",
	}
}

func (t *ForwardRunTool) Name() string         { return "forward_run" }
func (t *ForwardRunTool) Version() string       { return t.version }
func (t *ForwardRunTool) Permissions() []ToolPermission { return []ToolPermission{PermRead} }
func (t *ForwardRunTool) Description() string {
	return "查询 TongStock 前向模拟和 Paper Trading 运行 (只读)。列出最新前向运行、按 ID 查询运行详情、按范式查询历史前向、查看指定运行的信号明细。用于评估范式在未见过的市场中的实际表现。"
}

// Invoke 执行前向运行查询
//
// params:
//   - action: "list_latest" (默认), "get", "list_by_paradigm", "signals"
//   - run_id: string (action=get/signals 时必填)
//   - paradigm_version_id: string (action=list_by_paradigm 时必填)
//   - limit: int (action=list_latest 时, 默认 10)
func (t *ForwardRunTool) Invoke(ctx AccessContext, params map[string]any) (*ToolResult, error) {
	if err := t.guard.CheckForbidden(params); err != nil {
		return nil, err
	}

	action := getString(params, "action", "list_latest")

	switch action {
	case "list_latest":
		limit := getInt(params, "limit", 10)
		runs := t.repo.ListLatest(limit)
		summary := fmt.Sprintf("列出 %d 个最新前向运行", len(runs))
		return &ToolResult{Success: true, Data: runs, Summary: summary, Version: t.version}, nil

	case "get":
		id := getString(params, "run_id", "")
		if id == "" {
			return nil, fmt.Errorf("run_id is required for action=get")
		}
		run, err := t.repo.GetByID(id)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("前向运行 %s 不存在", id), Version: t.version}, nil
		}
		summary := fmt.Sprintf("查询前向运行 %s: 范式 %s, 起止 %s 到 %s, 信号 %d, 收益 %.2f%%",
			run.ID, run.ParadigmVersionID, run.StartDate.Format("2006-01-02"), run.EndDate.Format("2006-01-02"),
			run.SignalsCount, run.TotalReturn*100)
		return &ToolResult{Success: true, Data: run, Summary: summary, Version: t.version}, nil

	case "list_by_paradigm":
		pvid := getString(params, "paradigm_version_id", "")
		if pvid == "" {
			return nil, fmt.Errorf("paradigm_version_id is required for action=list_by_paradigm")
		}
		runs := t.repo.ListByParadigm(pvid)
		summary := fmt.Sprintf("范式版本 %s 有 %d 次前向运行", pvid, len(runs))
		return &ToolResult{Success: true, Data: runs, Summary: summary, Version: t.version}, nil

	case "signals":
		id := getString(params, "run_id", "")
		if id == "" {
			return nil, fmt.Errorf("run_id is required for action=signals")
		}
		signals, err := t.repo.GetSignals(id)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("前向运行 %s 信号不存在", id), Version: t.version}, nil
		}
		summary := fmt.Sprintf("前向运行 %s 有 %d 个信号", id, len(signals))
		return &ToolResult{Success: true, Data: signals, Summary: summary, Version: t.version}, nil

	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

// ============================================================================
// 证据下钻工具
// ============================================================================

// EvidenceDrilldown 原始证据下钻视图
type EvidenceDrilldown struct {
	ParadigmID        string              `json:"paradigm_id"`
	ParadigmVersionID string              `json:"paradigm_version_id"`
	ReportID          string              `json:"report_id"`
	DataSnapshotID    string              `json:"data_snapshot_id"`
	Level             string              `json:"level"`
	Score             float64             `json:"score"`
	MustFix           []string            `json:"must_fix"`
	Warnings          []string            `json:"warnings"`
	Suggestions       []string            `json:"suggestions"`
	Metrics           map[string]float64  `json:"metrics"`
	Windows           []WindowResultBrief `json:"windows"`
	Regimes           []RegimeResultBrief `json:"regimes"`
	CreatedAt         time.Time           `json:"created_at"`
}

// WindowResultBrief 窗口结果摘要
type WindowResultBrief struct {
	WindowID    string  `json:"window_id"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
	SampleSize  int     `json:"sample_size"`
	NetReturn   float64 `json:"net_return"`
	WinRate     float64 `json:"win_rate"`
	MaxDrawdown float64 `json:"max_drawdown"`
}

// RegimeResultBrief 市场状态结果摘要
type RegimeResultBrief struct {
	Regime     string  `json:"regime"`
	SampleSize int     `json:"sample_size"`
	NetReturn  float64 `json:"net_return"`
	WinRate    float64 `json:"win_rate"`
}

// EvidenceRepository 证据仓储接口
type EvidenceRepository interface {
	GetByVersion(versionID string) (*EvidenceDrilldown, error)
	ListByParadigm(paradigmID string) []EvidenceDrilldown
	GetRawMetrics(versionID string) (map[string]float64, error)
}

// EvidenceDrilldownTool 证据下钻工具
type EvidenceDrilldownTool struct {
	repo    EvidenceRepository
	guard   *ReadOnlyGuard
	version string
}

// NewEvidenceDrilldownTool 创建证据下钻工具
func NewEvidenceDrilldownTool(repo EvidenceRepository) *EvidenceDrilldownTool {
	return &EvidenceDrilldownTool{
		repo:    repo,
		guard:   NewReadOnlyGuard(),
		version: "1.0.0",
	}
}

func (t *EvidenceDrilldownTool) Name() string         { return "evidence_drilldown" }
func (t *EvidenceDrilldownTool) Version() string       { return t.version }
func (t *EvidenceDrilldownTool) Permissions() []ToolPermission { return []ToolPermission{PermRead} }
func (t *EvidenceDrilldownTool) Description() string {
	return "深度下钻范式验证证据 (只读)。按范式版本 ID 查看完整 Admission 证据: must_fix/warnings/suggestions、分窗口/分市场状态表现、原始指标。支持对比同一范式多次验证的演进。"
}

// Invoke 执行证据下钻查询
//
// params:
//   - action: "get_by_version" (默认), "list_by_paradigm", "raw_metrics"
//   - version_id: string (action=get_by_version/raw_metrics 时必填)
//   - paradigm_id: string (action=list_by_paradigm 时必填)
func (t *EvidenceDrilldownTool) Invoke(ctx AccessContext, params map[string]any) (*ToolResult, error) {
	if err := t.guard.CheckForbidden(params); err != nil {
		return nil, err
	}

	action := getString(params, "action", "get_by_version")

	switch action {
	case "get_by_version":
		vid := getString(params, "version_id", "")
		if vid == "" {
			return nil, fmt.Errorf("version_id is required")
		}
		drilldown, err := t.repo.GetByVersion(vid)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("版本 %s 证据不存在", vid), Version: t.version}, nil
		}
		summary := fmt.Sprintf("版本 %s 证据: 等级 %s, 评分 %.2f", vid, drilldown.Level, drilldown.Score)
		if len(drilldown.MustFix) > 0 {
			summary += fmt.Sprintf("; 必修复: %d 项", len(drilldown.MustFix))
		}
		return &ToolResult{
			Success:  true,
			Data:     drilldown,
			Summary:  summary,
			Warnings: drilldown.Warnings,
			Version:  t.version,
		}, nil

	case "list_by_paradigm":
		pid := getString(params, "paradigm_id", "")
		if pid == "" {
			return nil, fmt.Errorf("paradigm_id is required")
		}
		reports := t.repo.ListByParadigm(pid)
		summary := fmt.Sprintf("范式 %s 有 %d 次验证报告", pid, len(reports))
		return &ToolResult{Success: true, Data: reports, Summary: summary, Version: t.version}, nil

	case "raw_metrics":
		vid := getString(params, "version_id", "")
		if vid == "" {
			return nil, fmt.Errorf("version_id is required")
		}
		metrics, err := t.repo.GetRawMetrics(vid)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("版本 %s 原始指标不存在", vid), Version: t.version}, nil
		}
		summary := fmt.Sprintf("版本 %s 原始指标: %d 项", vid, len(metrics))
		return &ToolResult{Success: true, Data: metrics, Summary: summary, Version: t.version}, nil

	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

// ============================================================================
// 工具套件 (聚合)
// ============================================================================

// ResearchToolkit 研究工具套件 - 一键注册所有只读工具
type ResearchToolkit struct {
	Registry *ToolRegistry
}

// NewResearchToolkit 创建默认研究工具套件
func NewResearchToolkit(
	snapshotRepo SnapshotRepository,
	featureRepo FeatureRepository,
	experimentRepo ExperimentRepository,
	paradigmRepo ParadigmRepository,
	forwardRunRepo ForwardRunRepository,
	evidenceRepo EvidenceRepository,
) (*ResearchToolkit, error) {
	reg := NewToolRegistry()

	tools := []Tool{
		NewDataSnapshotTool(snapshotRepo),
		NewFeatureQueryTool(featureRepo),
		NewExperimentReportTool(experimentRepo),
		NewParadigmVersionTool(paradigmRepo),
		NewForwardRunTool(forwardRunRepo),
		NewEvidenceDrilldownTool(evidenceRepo),
	}

	for _, t := range tools {
		if err := reg.Register(t); err != nil {
			return nil, err
		}
	}

	return &ResearchToolkit{Registry: reg}, nil
}

// ============================================================================
// 内存仓储 (用于测试和演示)
// ============================================================================

// InMemorySnapshotRepo 内存快照仓储
type InMemorySnapshotRepo struct {
	snapshots map[string]*SnapshotInfo
}

// NewInMemorySnapshotRepo 创建内存快照仓储
func NewInMemorySnapshotRepo() *InMemorySnapshotRepo {
	return &InMemorySnapshotRepo{snapshots: make(map[string]*SnapshotInfo)}
}

// Add 添加快照
func (r *InMemorySnapshotRepo) Add(s *SnapshotInfo) {
	r.snapshots[s.ID] = s
}

func (r *InMemorySnapshotRepo) ListLatest(n int) []SnapshotInfo {
	result := make([]SnapshotInfo, 0, n)
	for _, s := range r.snapshots {
		result = append(result, *s)
		if len(result) >= n {
			break
		}
	}
	return result
}

func (r *InMemorySnapshotRepo) GetByID(id string) (*SnapshotInfo, error) {
	s, ok := r.snapshots[id]
	if !ok {
		return nil, fmt.Errorf("snapshot %s not found", id)
	}
	return s, nil
}

func (r *InMemorySnapshotRepo) SearchByDateRange(start, end string) []SnapshotInfo {
	var result []SnapshotInfo
	for _, s := range r.snapshots {
		if start == "" || s.DateRange >= start {
			if end == "" || s.DateRange <= end {
				result = append(result, *s)
			}
		}
	}
	return result
}

// InMemoryFeatureRepo 内存特征仓储
type InMemoryFeatureRepo struct {
	features map[string]*FeatureInfo
}

// NewInMemoryFeatureRepo 创建内存特征仓储
func NewInMemoryFeatureRepo() *InMemoryFeatureRepo {
	return &InMemoryFeatureRepo{features: make(map[string]*FeatureInfo)}
}

// Add 添加特征
func (r *InMemoryFeatureRepo) Add(f *FeatureInfo) {
	r.features[f.ID] = f
}

func (r *InMemoryFeatureRepo) ListAll() []FeatureInfo {
	result := make([]FeatureInfo, 0, len(r.features))
	for _, f := range r.features {
		result = append(result, *f)
	}
	return result
}

func (r *InMemoryFeatureRepo) GetByID(id string) (*FeatureInfo, error) {
	f, ok := r.features[id]
	if !ok {
		return nil, fmt.Errorf("feature %s not found", id)
	}
	return f, nil
}

func (r *InMemoryFeatureRepo) Search(keyword string) []FeatureInfo {
	var result []FeatureInfo
	for _, f := range r.features {
		if containsStr(f.Name, keyword) || containsStr(f.Formula, keyword) {
			result = append(result, *f)
		}
	}
	return result
}

// InMemoryExperimentRepo 内存实验仓储
type InMemoryExperimentRepo struct {
	experiments map[string]*ExperimentSummary
	reports     map[string]*BacktestReportSummary
	byStatus    map[string][]string
}

// NewInMemoryExperimentRepo 创建内存实验仓储
func NewInMemoryExperimentRepo() *InMemoryExperimentRepo {
	return &InMemoryExperimentRepo{
		experiments: make(map[string]*ExperimentSummary),
		reports:     make(map[string]*BacktestReportSummary),
		byStatus:    make(map[string][]string),
	}
}

// Add 添加实验
func (r *InMemoryExperimentRepo) Add(e *ExperimentSummary) {
	r.experiments[e.ID] = e
	r.byStatus[e.Status] = append(r.byStatus[e.Status], e.ID)
}

// AddReport 添加回测报告
func (r *InMemoryExperimentRepo) AddReport(report *BacktestReportSummary) {
	r.reports[report.ExperimentID] = report
}

func (r *InMemoryExperimentRepo) ListLatest(n int) []ExperimentSummary {
	result := make([]ExperimentSummary, 0, n)
	for _, e := range r.experiments {
		result = append(result, *e)
		if len(result) >= n {
			break
		}
	}
	return result
}

func (r *InMemoryExperimentRepo) GetByID(id string) (*ExperimentSummary, error) {
	e, ok := r.experiments[id]
	if !ok {
		return nil, fmt.Errorf("experiment %s not found", id)
	}
	return e, nil
}

func (r *InMemoryExperimentRepo) ListByStatus(status string) []ExperimentSummary {
	var result []ExperimentSummary
	for _, id := range r.byStatus[status] {
		if e, ok := r.experiments[id]; ok {
			result = append(result, *e)
		}
	}
	return result
}

func (r *InMemoryExperimentRepo) GetBacktestReport(experimentID string) (*BacktestReportSummary, error) {
	report, ok := r.reports[experimentID]
	if !ok {
		return nil, fmt.Errorf("backtest report for %s not found", experimentID)
	}
	return report, nil
}

// InMemoryParadigmRepo 内存范式仓储
type InMemoryParadigmRepo struct {
	versions   map[string]*ParadigmVersionInfo
	byParadigm map[string][]string
	byState    map[string][]string
	evidence   map[string]*ValidationEvidenceInfo
}

// NewInMemoryParadigmRepo 创建内存范式仓储
func NewInMemoryParadigmRepo() *InMemoryParadigmRepo {
	return &InMemoryParadigmRepo{
		versions:   make(map[string]*ParadigmVersionInfo),
		byParadigm: make(map[string][]string),
		byState:    make(map[string][]string),
		evidence:   make(map[string]*ValidationEvidenceInfo),
	}
}

// AddVersion 添加范式版本
func (r *InMemoryParadigmRepo) AddVersion(v *ParadigmVersionInfo) {
	r.versions[v.ID] = v
	r.byParadigm[v.ParadigmID] = append(r.byParadigm[v.ParadigmID], v.ID)
	r.byState[v.State] = append(r.byState[v.State], v.ID)
}

// AddEvidence 添加验证证据
func (r *InMemoryParadigmRepo) AddEvidence(e *ValidationEvidenceInfo) {
	r.evidence[e.ParadigmVersionID] = e
}

func (r *InMemoryParadigmRepo) ListPromoted() []ParadigmVersionInfo {
	var result []ParadigmVersionInfo
	for _, id := range r.byState["promoted"] {
		if v, ok := r.versions[id]; ok {
			result = append(result, *v)
		}
	}
	return result
}

func (r *InMemoryParadigmRepo) GetByID(id string) (*ParadigmVersionInfo, error) {
	v, ok := r.versions[id]
	if !ok {
		return nil, fmt.Errorf("paradigm version %s not found", id)
	}
	return v, nil
}

func (r *InMemoryParadigmRepo) ListHistory(paradigmID string) []ParadigmVersionInfo {
	var result []ParadigmVersionInfo
	for _, id := range r.byParadigm[paradigmID] {
		if v, ok := r.versions[id]; ok {
			result = append(result, *v)
		}
	}
	return result
}

func (r *InMemoryParadigmRepo) GetValidationEvidence(versionID string) (*ValidationEvidenceInfo, error) {
	e, ok := r.evidence[versionID]
	if !ok {
		return nil, fmt.Errorf("evidence for %s not found", versionID)
	}
	return e, nil
}

func (r *InMemoryParadigmRepo) SearchByState(state string) []ParadigmVersionInfo {
	var result []ParadigmVersionInfo
	for _, id := range r.byState[state] {
		if v, ok := r.versions[id]; ok {
			result = append(result, *v)
		}
	}
	return result
}

// InMemoryForwardRunRepo 内存前向运行仓储
type InMemoryForwardRunRepo struct {
	runs    map[string]*ForwardRunSummary
	signals map[string][]SignalDetail
	byPara  map[string][]string
}

// NewInMemoryForwardRunRepo 创建内存前向运行仓储
func NewInMemoryForwardRunRepo() *InMemoryForwardRunRepo {
	return &InMemoryForwardRunRepo{
		runs:    make(map[string]*ForwardRunSummary),
		signals: make(map[string][]SignalDetail),
		byPara:  make(map[string][]string),
	}
}

// AddRun 添加前向运行
func (r *InMemoryForwardRunRepo) AddRun(run *ForwardRunSummary) {
	r.runs[run.ID] = run
	r.byPara[run.ParadigmVersionID] = append(r.byPara[run.ParadigmVersionID], run.ID)
}

// AddSignals 添加信号
func (r *InMemoryForwardRunRepo) AddSignals(runID string, signals []SignalDetail) {
	r.signals[runID] = signals
}

func (r *InMemoryForwardRunRepo) ListLatest(n int) []ForwardRunSummary {
	result := make([]ForwardRunSummary, 0, n)
	for _, run := range r.runs {
		result = append(result, *run)
		if len(result) >= n {
			break
		}
	}
	return result
}

func (r *InMemoryForwardRunRepo) GetByID(id string) (*ForwardRunSummary, error) {
	run, ok := r.runs[id]
	if !ok {
		return nil, fmt.Errorf("forward run %s not found", id)
	}
	return run, nil
}

func (r *InMemoryForwardRunRepo) ListByParadigm(paradigmVersionID string) []ForwardRunSummary {
	var result []ForwardRunSummary
	for _, id := range r.byPara[paradigmVersionID] {
		if run, ok := r.runs[id]; ok {
			result = append(result, *run)
		}
	}
	return result
}

func (r *InMemoryForwardRunRepo) GetSignals(runID string) ([]SignalDetail, error) {
	s, ok := r.signals[runID]
	if !ok {
		return nil, fmt.Errorf("signals for %s not found", runID)
	}
	return s, nil
}

// InMemoryEvidenceRepo 内存证据仓储
type InMemoryEvidenceRepo struct {
	reports map[string]*EvidenceDrilldown
	byPara  map[string][]string
}

// NewInMemoryEvidenceRepo 创建内存证据仓储
func NewInMemoryEvidenceRepo() *InMemoryEvidenceRepo {
	return &InMemoryEvidenceRepo{
		reports: make(map[string]*EvidenceDrilldown),
		byPara:  make(map[string][]string),
	}
}

// AddReport 添加证据报告
func (r *InMemoryEvidenceRepo) AddReport(report *EvidenceDrilldown) {
	r.reports[report.ParadigmVersionID] = report
	r.byPara[report.ParadigmID] = append(r.byPara[report.ParadigmID], report.ParadigmVersionID)
}

func (r *InMemoryEvidenceRepo) GetByVersion(versionID string) (*EvidenceDrilldown, error) {
	report, ok := r.reports[versionID]
	if !ok {
		return nil, fmt.Errorf("evidence for version %s not found", versionID)
	}
	return report, nil
}

func (r *InMemoryEvidenceRepo) ListByParadigm(paradigmID string) []EvidenceDrilldown {
	var result []EvidenceDrilldown
	for _, vid := range r.byPara[paradigmID] {
		if report, ok := r.reports[vid]; ok {
			result = append(result, *report)
		}
	}
	return result
}

func (r *InMemoryEvidenceRepo) GetRawMetrics(versionID string) (map[string]float64, error) {
	report, ok := r.reports[versionID]
	if !ok {
		return nil, fmt.Errorf("evidence for version %s not found", versionID)
	}
	return report.Metrics, nil
}

// containsStr 判断字符串是否包含子串 (大小写不敏感)
func containsStr(s, substr string) bool {
	lower := toLower(s)
	lowerSub := toLower(substr)
	for i := 0; i <= len(lower)-len(lowerSub); i++ {
		if lower[i:i+len(lowerSub)] == lowerSub {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = c
		}
	}
	return string(b)
}
