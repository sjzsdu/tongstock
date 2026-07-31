package ai_tools

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// 数据快照工具
// ============================================================================

// SnapshotInfo 数据快照摘要
type SnapshotInfo struct {
	ID              string            `json:"id"`
	Version         string            `json:"version"`
	DateRange       string            `json:"date_range"`
	UniverseSize    int               `json:"universe_size"`
	Market          string            `json:"market"`
	PriceAdjustment string            `json:"price_adjustment"`
	Description     string            `json:"description"`
	CreatedAt       time.Time         `json:"created_at"`
	SourceVersions  map[string]string `json:"source_versions,omitempty"`
}

// SnapshotRepository 快照仓储接口 (由上层注入)
type SnapshotRepository interface {
	ListLatest(n int) []SnapshotInfo
	GetByID(id string) (*SnapshotInfo, error)
	SearchByDateRange(start, end string) []SnapshotInfo
}

// DataSnapshotTool 数据快照查询工具
type DataSnapshotTool struct {
	repo    SnapshotRepository
	guard   *ReadOnlyGuard
	version string
}

// NewDataSnapshotTool 创建数据快照工具
func NewDataSnapshotTool(repo SnapshotRepository) *DataSnapshotTool {
	return &DataSnapshotTool{
		repo:    repo,
		guard:   NewReadOnlyGuard(),
		version: "1.0.0",
	}
}

func (t *DataSnapshotTool) Name() string                  { return "data_snapshot" }
func (t *DataSnapshotTool) Version() string               { return t.version }
func (t *DataSnapshotTool) Permissions() []ToolPermission { return []ToolPermission{PermRead} }
func (t *DataSnapshotTool) Description() string {
	return "查询 TongStock 内部数据快照 (只读)。可列出最新快照、按 ID 查询快照详情、按日期范围搜索快照。返回: 快照 ID、版本、日期范围、股票数量、市场、价格口径、数据源版本等结构化信息。"
}

// Invoke 执行数据快照查询
//
// params:
//   - action: "list_latest" (默认), "get", "search"
//   - snapshot_id: string (action=get 时必填)
//   - start_date / end_date: string (action=search 时可选)
//   - limit: int (action=list_latest 时, 默认 10)
func (t *DataSnapshotTool) Invoke(ctx AccessContext, params map[string]any) (*ToolResult, error) {
	if err := t.guard.CheckForbidden(params); err != nil {
		return nil, err
	}

	action := getString(params, "action", "list_latest")

	switch action {
	case "list_latest":
		limit := getInt(params, "limit", 10)
		snapshots := t.repo.ListLatest(limit)
		summary := fmt.Sprintf("列出 %d 个最新数据快照", len(snapshots))
		return &ToolResult{
			Success: true,
			Data:    snapshots,
			Summary: summary,
			Version: t.version,
			Metadata: map[string]any{
				"tool":        "data_snapshot",
				"action":      action,
				"agent_id":    ctx.AgentID,
				"session_id":  ctx.SessionID,
				"returned_at": time.Now().Format(time.RFC3339),
			},
		}, nil

	case "get":
		id := getString(params, "snapshot_id", "")
		if id == "" {
			return nil, fmt.Errorf("snapshot_id is required for action=get")
		}
		snap, err := t.repo.GetByID(id)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("快照 %s 不存在", id), Version: t.version}, nil
		}
		summary := fmt.Sprintf("查询快照 %s: %s 到 %s, 共 %d 只股票", snap.ID, dateRangeStart(snap.DateRange), dateRangeEnd(snap.DateRange), snap.UniverseSize)
		return &ToolResult{
			Success: true,
			Data:    snap,
			Summary: summary,
			Version: t.version,
		}, nil

	case "search":
		start := getString(params, "start_date", "")
		end := getString(params, "end_date", "")
		snaps := t.repo.SearchByDateRange(start, end)
		summary := fmt.Sprintf("在 %s 到 %s 期间找到 %d 个快照", start, end, len(snaps))
		return &ToolResult{
			Success: true,
			Data:    snaps,
			Summary: summary,
			Version: t.version,
		}, nil

	default:
		return nil, fmt.Errorf("unknown action %q, expected: list_latest, get, search", action)
	}
}

// ============================================================================
// 特征查询工具
// ============================================================================

// FeatureInfo 特征摘要
type FeatureInfo struct {
	ID              string         `json:"id"`
	Version         string         `json:"version"`
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	Formula         string         `json:"formula"`
	Params          map[string]any `json:"params,omitempty"`
	Description     string         `json:"description"`
	Indicators      []string       `json:"indicators,omitempty"`
	Signals         []string       `json:"signals,omitempty"`
	PriceAdjustment string         `json:"price_adjustment"`
}

// FeatureRepository 特征仓储接口
type FeatureRepository interface {
	ListAll() []FeatureInfo
	GetByID(id string) (*FeatureInfo, error)
	Search(keyword string) []FeatureInfo
}

// FeatureQueryTool 特征查询工具
type FeatureQueryTool struct {
	repo    FeatureRepository
	guard   *ReadOnlyGuard
	version string
}

// NewFeatureQueryTool 创建特征查询工具
func NewFeatureQueryTool(repo FeatureRepository) *FeatureQueryTool {
	return &FeatureQueryTool{
		repo:    repo,
		guard:   NewReadOnlyGuard(),
		version: "1.0.0",
	}
}

func (t *FeatureQueryTool) Name() string                  { return "feature_query" }
func (t *FeatureQueryTool) Version() string               { return t.version }
func (t *FeatureQueryTool) Permissions() []ToolPermission { return []ToolPermission{PermRead} }
func (t *FeatureQueryTool) Description() string {
	return "查询 TongStock 特征定义 (只读)。列出所有特征、按 ID 查询特征详情、按关键字搜索特征。特征包含: 技术指标 (MACD, RSI, MA 等)、信号 (金叉、超卖等)。返回: 特征 ID、版本、类型、公式、参数、描述等结构化信息。"
}

// Invoke 执行特征查询
//
// params:
//   - action: "list" (默认), "get", "search"
//   - feature_id: string (action=get 时必填)
//   - keyword: string (action=search 时必填)
func (t *FeatureQueryTool) Invoke(ctx AccessContext, params map[string]any) (*ToolResult, error) {
	if err := t.guard.CheckForbidden(params); err != nil {
		return nil, err
	}

	action := getString(params, "action", "list")

	switch action {
	case "list":
		features := t.repo.ListAll()
		summary := fmt.Sprintf("列出 %d 个特征定义", len(features))
		return &ToolResult{Success: true, Data: features, Summary: summary, Version: t.version}, nil

	case "get":
		id := getString(params, "feature_id", "")
		if id == "" {
			return nil, fmt.Errorf("feature_id is required for action=get")
		}
		f, err := t.repo.GetByID(id)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("特征 %s 不存在", id), Version: t.version}, nil
		}
		summary := fmt.Sprintf("查询特征 %s (v%s): %s", f.ID, f.Version, f.Name)
		return &ToolResult{Success: true, Data: f, Summary: summary, Version: t.version}, nil

	case "search":
		keyword := getString(params, "keyword", "")
		if keyword == "" {
			return nil, fmt.Errorf("keyword is required for action=search")
		}
		features := t.repo.Search(keyword)
		summary := fmt.Sprintf("搜索关键字 %q 找到 %d 个特征", keyword, len(features))
		return &ToolResult{Success: true, Data: features, Summary: summary, Version: t.version}, nil

	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

// ============================================================================
// 实验报告工具
// ============================================================================

// ExperimentSummary 实验摘要
type ExperimentSummary struct {
	ID              string     `json:"id"`
	HypothesisID    string     `json:"hypothesis_id"`
	DatasetSnapshot string     `json:"dataset_snapshot"`
	FeatureSetID    string     `json:"feature_set_id"`
	Status          string     `json:"status"`
	HoldingPeriod   string     `json:"holding_period"`
	CostPerTrade    float64    `json:"cost_per_trade"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

// BacktestReportSummary 回测报告摘要
type BacktestReportSummary struct {
	ExperimentID string  `json:"experiment_id"`
	TotalReturn  float64 `json:"total_return"`
	NetReturn    float64 `json:"net_return"`
	MaxDrawdown  float64 `json:"max_drawdown"`
	SharpeRatio  float64 `json:"sharpe_ratio"`
	WinRate      float64 `json:"win_rate"`
	SampleSize   int     `json:"sample_size"`
	Passed       bool    `json:"passed"`
	Level        string  `json:"level,omitempty"`
}

// ExperimentRepository 实验仓储接口
type ExperimentRepository interface {
	ListLatest(n int) []ExperimentSummary
	GetByID(id string) (*ExperimentSummary, error)
	ListByStatus(status string) []ExperimentSummary
	GetBacktestReport(experimentID string) (*BacktestReportSummary, error)
}

// ExperimentReportTool 实验/回测报告查询工具
type ExperimentReportTool struct {
	repo    ExperimentRepository
	guard   *ReadOnlyGuard
	version string
}

// NewExperimentReportTool 创建实验报告工具
func NewExperimentReportTool(repo ExperimentRepository) *ExperimentReportTool {
	return &ExperimentReportTool{
		repo:    repo,
		guard:   NewReadOnlyGuard(),
		version: "1.0.0",
	}
}

func (t *ExperimentReportTool) Name() string                  { return "experiment_report" }
func (t *ExperimentReportTool) Version() string               { return t.version }
func (t *ExperimentReportTool) Permissions() []ToolPermission { return []ToolPermission{PermRead} }
func (t *ExperimentReportTool) Description() string {
	return "查询 TongStock 实验和回测报告 (只读)。列出最新实验、按 ID 查询实验详情、按状态筛选实验、获取指定实验的回测报告 (收益、回撤、Sharpe、通过率等)。返回: 结构化的实验配置和回测指标。"
}

// Invoke 执行实验报告查询
//
// params:
//   - action: "list_latest" (默认), "get", "list_by_status", "backtest_report"
//   - experiment_id: string (action=get/backtest_report 时必填)
//   - status: string (action=list_by_status 时: draft/running/completed/failed)
//   - limit: int (action=list_latest 时, 默认 10)
func (t *ExperimentReportTool) Invoke(ctx AccessContext, params map[string]any) (*ToolResult, error) {
	if err := t.guard.CheckForbidden(params); err != nil {
		return nil, err
	}

	action := getString(params, "action", "list_latest")

	switch action {
	case "list_latest":
		limit := getInt(params, "limit", 10)
		exps := t.repo.ListLatest(limit)
		summary := fmt.Sprintf("列出 %d 个最新实验", len(exps))
		return &ToolResult{Success: true, Data: exps, Summary: summary, Version: t.version}, nil

	case "get":
		id := getString(params, "experiment_id", "")
		if id == "" {
			return nil, fmt.Errorf("experiment_id is required for action=get")
		}
		exp, err := t.repo.GetByID(id)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("实验 %s 不存在", id), Version: t.version}, nil
		}
		summary := fmt.Sprintf("查询实验 %s (状态: %s)", exp.ID, exp.Status)
		return &ToolResult{Success: true, Data: exp, Summary: summary, Version: t.version}, nil

	case "list_by_status":
		status := getString(params, "status", "completed")
		exps := t.repo.ListByStatus(status)
		summary := fmt.Sprintf("按状态 %q 找到 %d 个实验", status, len(exps))
		return &ToolResult{Success: true, Data: exps, Summary: summary, Version: t.version}, nil

	case "backtest_report":
		id := getString(params, "experiment_id", "")
		if id == "" {
			return nil, fmt.Errorf("experiment_id is required for action=backtest_report")
		}
		report, err := t.repo.GetBacktestReport(id)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("实验 %s 回测报告不存在", id), Version: t.version}, nil
		}
		passedStr := "未通过"
		if report.Passed {
			passedStr = fmt.Sprintf("通过 (等级: %s)", report.Level)
		}
		summary := fmt.Sprintf("实验 %s 回测: 净收益 %.2f%%, 最大回撤 %.2f%%, Sharpe %.2f, 样本 %d, %s",
			id, report.NetReturn*100, report.MaxDrawdown*100, report.SharpeRatio, report.SampleSize, passedStr)
		return &ToolResult{Success: true, Data: report, Summary: summary, Version: t.version}, nil

	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

// ============================================================================
// 范式版本工具
// ============================================================================

// ParadigmVersionInfo 范式版本信息
type ParadigmVersionInfo struct {
	ID         string    `json:"id"`
	ParadigmID string    `json:"paradigm_id"`
	Version    int       `json:"version"`
	State      string    `json:"state"`
	RuleSet    string    `json:"rule_set"`
	ParentID   string    `json:"parent_id,omitempty"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  string    `json:"created_by"`
}

// ValidationEvidenceInfo 验证证据摘要
type ValidationEvidenceInfo struct {
	ParadigmID        string   `json:"paradigm_id"`
	ParadigmVersionID string   `json:"paradigm_version_id"`
	NetReturn         float64  `json:"net_return"`
	MaxDrawdown       float64  `json:"max_drawdown"`
	SharpeRatio       float64  `json:"sharpe_ratio"`
	WinRate           float64  `json:"win_rate"`
	SampleSize        int      `json:"sample_size"`
	Passed            bool     `json:"passed"`
	Level             string   `json:"level,omitempty"`
	MustFix           []string `json:"must_fix,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	Suggestions       []string `json:"suggestions,omitempty"`
}

// ParadigmRepository 范式仓储接口
type ParadigmRepository interface {
	ListPromoted() []ParadigmVersionInfo
	GetByID(id string) (*ParadigmVersionInfo, error)
	ListHistory(paradigmID string) []ParadigmVersionInfo
	GetValidationEvidence(versionID string) (*ValidationEvidenceInfo, error)
	SearchByState(state string) []ParadigmVersionInfo
}

// ParadigmVersionTool 范式版本/证据下钻工具
type ParadigmVersionTool struct {
	repo    ParadigmRepository
	guard   *ReadOnlyGuard
	version string
}

// NewParadigmVersionTool 创建范式版本工具
func NewParadigmVersionTool(repo ParadigmRepository) *ParadigmVersionTool {
	return &ParadigmVersionTool{
		repo:    repo,
		guard:   NewReadOnlyGuard(),
		version: "1.0.0",
	}
}

func (t *ParadigmVersionTool) Name() string                  { return "paradigm_version" }
func (t *ParadigmVersionTool) Version() string               { return t.version }
func (t *ParadigmVersionTool) Permissions() []ToolPermission { return []ToolPermission{PermRead} }
func (t *ParadigmVersionTool) Description() string {
	return "查询 TongStock 范式版本、血缘和证据下钻 (只读)。列出已晋级范式、按 ID 查询范式版本、查看范式版本历史 (血缘)、获取指定版本的验证证据 (must_fix/warnings/suggestions)。AI 无法修改晋级结果。"
}

// Invoke 执行范式版本查询
//
// params:
//   - action: "list_promoted" (默认), "get", "history", "evidence", "list_by_state"
//   - version_id: string (action=get/evidence 时必填)
//   - paradigm_id: string (action=history 时必填)
//   - state: string (action=list_by_state 时: draft/experiment/validation/promoted/rejected/retired)
func (t *ParadigmVersionTool) Invoke(ctx AccessContext, params map[string]any) (*ToolResult, error) {
	if err := t.guard.CheckForbidden(params); err != nil {
		return nil, err
	}

	action := getString(params, "action", "list_promoted")

	switch action {
	case "list_promoted":
		paradigms := t.repo.ListPromoted()
		summary := fmt.Sprintf("列出 %d 个已晋级范式", len(paradigms))
		return &ToolResult{Success: true, Data: paradigms, Summary: summary, Version: t.version}, nil

	case "get":
		id := getString(params, "version_id", "")
		if id == "" {
			return nil, fmt.Errorf("version_id is required for action=get")
		}
		pv, err := t.repo.GetByID(id)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("范式版本 %s 不存在", id), Version: t.version}, nil
		}
		summary := fmt.Sprintf("查询范式版本 %s: paradigm=%s, version=%d, state=%s", pv.ID, pv.ParadigmID, pv.Version, pv.State)
		return &ToolResult{Success: true, Data: pv, Summary: summary, Version: t.version}, nil

	case "history":
		pid := getString(params, "paradigm_id", "")
		if pid == "" {
			return nil, fmt.Errorf("paradigm_id is required for action=history")
		}
		versions := t.repo.ListHistory(pid)
		summary := fmt.Sprintf("范式 %s 版本历史: %d 个版本", pid, len(versions))
		return &ToolResult{Success: true, Data: versions, Summary: summary, Version: t.version}, nil

	case "evidence":
		vid := getString(params, "version_id", "")
		if vid == "" {
			return nil, fmt.Errorf("version_id is required for action=evidence")
		}
		evidence, err := t.repo.GetValidationEvidence(vid)
		if err != nil {
			return &ToolResult{Success: false, Summary: fmt.Sprintf("范式版本 %s 证据不存在", vid), Version: t.version}, nil
		}
		passedStr := "未通过"
		if evidence.Passed {
			passedStr = fmt.Sprintf("通过 (等级: %s)", evidence.Level)
		}
		summary := fmt.Sprintf("范式版本 %s 证据: 净收益 %.2f%%, 最大回撤 %.2f%%, Sharpe %.2f, %s",
			vid, evidence.NetReturn*100, evidence.MaxDrawdown*100, evidence.SharpeRatio, passedStr)
		if len(evidence.MustFix) > 0 {
			summary += fmt.Sprintf("; 必修复: %s", strings.Join(evidence.MustFix, "; "))
		}
		if len(evidence.Warnings) > 0 {
			summary += fmt.Sprintf("; 警告: %s", strings.Join(evidence.Warnings, "; "))
		}
		return &ToolResult{
			Success:  true,
			Data:     evidence,
			Summary:  summary,
			Warnings: evidence.Warnings,
			Version:  t.version,
		}, nil

	case "list_by_state":
		state := getString(params, "state", "promoted")
		paradigms := t.repo.SearchByState(state)
		summary := fmt.Sprintf("按状态 %q 找到 %d 个范式版本", state, len(paradigms))
		return &ToolResult{Success: true, Data: paradigms, Summary: summary, Version: t.version}, nil

	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

func getString(params map[string]any, key, defaultVal string) string {
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

func getInt(params map[string]any, key string, defaultVal int) int {
	if v, ok := params[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case float64:
			return int(n)
		case int64:
			return int(n)
		}
	}
	return defaultVal
}

func dateRangeStart(dr string) string {
	parts := strings.SplitN(dr, "-", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return dr
}

func dateRangeEnd(dr string) string {
	parts := strings.SplitN(dr, "-", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}
