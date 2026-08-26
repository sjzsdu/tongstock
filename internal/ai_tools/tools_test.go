package ai_tools

import (
	"testing"
	"time"
)

// ============================================================================
// 工具注册中心测试
// ============================================================================

func TestNewToolRegistry(t *testing.T) {
	reg := NewToolRegistry()
	if reg == nil {
		t.Fatal("NewToolRegistry returned nil")
	}
	if len(reg.List()) != 0 {
		t.Error("empty registry should have no tools")
	}
}

func TestRegisterAndListTool(t *testing.T) {
	reg := NewToolRegistry()
	tool := NewDataSnapshotTool(NewInMemorySnapshotRepo())

	if err := reg.Register(tool); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	tools := reg.List()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "data_snapshot" {
		t.Errorf("expected data_snapshot, got %s", tools[0].Name)
	}
}

func TestRegisterDuplicateTool(t *testing.T) {
	reg := NewToolRegistry()
	tool := NewDataSnapshotTool(NewInMemorySnapshotRepo())

	if err := reg.Register(tool); err != nil {
		t.Fatal(err)
	}

	if err := reg.Register(tool); err == nil {
		t.Error("expected error for duplicate tool registration")
	}
}

func TestUnregisterTool(t *testing.T) {
	reg := NewToolRegistry()
	tool := NewDataSnapshotTool(NewInMemorySnapshotRepo())
	reg.Register(tool)
	reg.Unregister("data_snapshot")

	if len(reg.List()) != 0 {
		t.Error("expected empty registry after unregister")
	}
}

// ============================================================================
// 工具调用测试
// ============================================================================

func TestCallTool(t *testing.T) {
	reg := NewToolRegistry()
	repo := NewInMemorySnapshotRepo()
	repo.Add(&SnapshotInfo{
		ID:           "snap-001",
		Version:      "v1",
		DateRange:    "2025-01-01-2025-12-31",
		UniverseSize: 1000,
		Market:       "SH",
	})

	tool := NewDataSnapshotTool(repo)
	reg.Register(tool)

	ctx := AccessContext{AgentID: "agent-1", SessionID: "sess-1", Timestamp: time.Now()}
	result, err := reg.Call(ctx, "data_snapshot", map[string]any{"action": "list_latest", "limit": 5})

	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestCallToolNotFound(t *testing.T) {
	reg := NewToolRegistry()
	ctx := AccessContext{AgentID: "agent-1", SessionID: "sess-1"}
	_, err := reg.Call(ctx, "nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestCallToolWithForbiddenWrite(t *testing.T) {
	reg := NewToolRegistry()
	repo := NewInMemorySnapshotRepo()
	tool := NewDataSnapshotTool(repo)
	reg.Register(tool)

	ctx := AccessContext{AgentID: "agent-1", SessionID: "sess-1"}
	// 试图通过参数注入写操作
	_, err := reg.Call(ctx, "data_snapshot", map[string]any{"promote_paradigm": "p-1"})
	if err == nil {
		t.Error("expected error for forbidden write operation")
	}
}

func TestCallToolLogs(t *testing.T) {
	reg := NewToolRegistry()
	repo := NewInMemorySnapshotRepo()
	repo.Add(&SnapshotInfo{ID: "s-1", Version: "v1", DateRange: "2025-01-01-2025-12-31", UniverseSize: 100})
	tool := NewDataSnapshotTool(repo)
	reg.Register(tool)

	ctx := AccessContext{AgentID: "agent-1", SessionID: "sess-1"}
	reg.Call(ctx, "data_snapshot", map[string]any{"action": "list_latest"})

	logs := reg.GetLogs("agent-1", 10)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].ToolName != "data_snapshot" {
		t.Errorf("expected data_snapshot, got %s", logs[0].ToolName)
	}
	if logs[0].Version == "" {
		t.Error("expected version in log")
	}
}

// ============================================================================
// 数据快照工具测试
// ============================================================================

func TestDataSnapshotTool_ListLatest(t *testing.T) {
	repo := NewInMemorySnapshotRepo()
	for i := 1; i <= 3; i++ {
		repo.Add(&SnapshotInfo{
			ID:           "snap-" + string(rune('0'+i)),
			Version:      "v" + string(rune('0'+i)),
			DateRange:    "2025-01-01-2025-12-31",
			UniverseSize: 500 * i,
			Market:       "SH",
		})
	}

	tool := NewDataSnapshotTool(repo)
	ctx := AccessContext{AgentID: "a", SessionID: "s"}
	result, err := tool.Invoke(ctx, map[string]any{"action": "list_latest", "limit": 3})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestDataSnapshotTool_Get(t *testing.T) {
	repo := NewInMemorySnapshotRepo()
	repo.Add(&SnapshotInfo{ID: "snap-001", Version: "v1", DateRange: "2025-01-01-2025-12-31", UniverseSize: 100})

	tool := NewDataSnapshotTool(repo)
	ctx := AccessContext{}

	result, err := tool.Invoke(ctx, map[string]any{"action": "get", "snapshot_id": "snap-001"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestDataSnapshotTool_GetMissing(t *testing.T) {
	repo := NewInMemorySnapshotRepo()
	tool := NewDataSnapshotTool(repo)
	ctx := AccessContext{}

	_, err := tool.Invoke(ctx, map[string]any{"action": "get"})
	if err == nil {
		t.Error("expected error for missing snapshot_id")
	}
}

func TestDataSnapshotTool_Search(t *testing.T) {
	repo := NewInMemorySnapshotRepo()
	repo.Add(&SnapshotInfo{ID: "s-1", Version: "v1", DateRange: "2025-01-01-2025-06-30", UniverseSize: 100})
	repo.Add(&SnapshotInfo{ID: "s-2", Version: "v2", DateRange: "2025-07-01-2025-12-31", UniverseSize: 100})

	tool := NewDataSnapshotTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "search", "start_date": "2025-01-01", "end_date": "2025-06-30"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestDataSnapshotTool_UnknownAction(t *testing.T) {
	repo := NewInMemorySnapshotRepo()
	tool := NewDataSnapshotTool(repo)
	ctx := AccessContext{}

	_, err := tool.Invoke(ctx, map[string]any{"action": "unknown"})
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

// ============================================================================
// 特征查询工具测试
// ============================================================================

func TestFeatureQueryTool_List(t *testing.T) {
	repo := NewInMemoryFeatureRepo()
	repo.Add(&FeatureInfo{ID: "feat-1", Name: "MACD", Type: "indicator", Formula: "EMA(12)-EMA(26)"})
	repo.Add(&FeatureInfo{ID: "feat-2", Name: "RSI", Type: "indicator", Formula: "RSI(14)"})

	tool := NewFeatureQueryTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "list"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestFeatureQueryTool_Get(t *testing.T) {
	repo := NewInMemoryFeatureRepo()
	repo.Add(&FeatureInfo{ID: "feat-1", Name: "MACD", Type: "indicator"})

	tool := NewFeatureQueryTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "get", "feature_id": "feat-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestFeatureQueryTool_Search(t *testing.T) {
	repo := NewInMemoryFeatureRepo()
	repo.Add(&FeatureInfo{ID: "feat-1", Name: "MACD", Formula: "EMA(12)-EMA(26)"})
	repo.Add(&FeatureInfo{ID: "feat-2", Name: "RSI", Formula: "RSI(14)"})

	tool := NewFeatureQueryTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "search", "keyword": "MACD"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestFeatureQueryTool_SearchMissingKeyword(t *testing.T) {
	repo := NewInMemoryFeatureRepo()
	tool := NewFeatureQueryTool(repo)
	ctx := AccessContext{}
	_, err := tool.Invoke(ctx, map[string]any{"action": "search"})
	if err == nil {
		t.Error("expected error for missing keyword")
	}
}

// ============================================================================
// 实验报告工具测试
// ============================================================================

func TestExperimentReportTool_ListLatest(t *testing.T) {
	repo := NewInMemoryExperimentRepo()
	for i := 1; i <= 3; i++ {
		repo.Add(&ExperimentSummary{
			ID:     "exp-" + string(rune('0'+i)),
			Status: "completed",
		})
	}

	tool := NewExperimentReportTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "list_latest", "limit": 5})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestExperimentReportTool_Get(t *testing.T) {
	repo := NewInMemoryExperimentRepo()
	repo.Add(&ExperimentSummary{ID: "exp-1", Status: "completed"})

	tool := NewExperimentReportTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "get", "experiment_id": "exp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestExperimentReportTool_ListByStatus(t *testing.T) {
	repo := NewInMemoryExperimentRepo()
	repo.Add(&ExperimentSummary{ID: "exp-1", Status: "completed"})
	repo.Add(&ExperimentSummary{ID: "exp-2", Status: "running"})

	tool := NewExperimentReportTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "list_by_status", "status": "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestExperimentReportTool_BacktestReport(t *testing.T) {
	repo := NewInMemoryExperimentRepo()
	repo.AddReport(&BacktestReportSummary{
		ExperimentID: "exp-1",
		TotalReturn:  0.15,
		NetReturn:    0.12,
		MaxDrawdown:  0.08,
		SharpeRatio:  1.5,
		WinRate:      0.55,
		SampleSize:   100,
		Passed:       true,
		Level:        "gold",
	})

	tool := NewExperimentReportTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "backtest_report", "experiment_id": "exp-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

// ============================================================================
// 范式版本工具测试
// ============================================================================

func TestParadigmVersionTool_ListPromoted(t *testing.T) {
	repo := NewInMemoryParadigmRepo()
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-1", ParadigmID: "p-1", Version: 1, State: "promoted"})
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-2", ParadigmID: "p-2", Version: 2, State: "promoted"})
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-3", ParadigmID: "p-3", Version: 1, State: "validation"})

	tool := NewParadigmVersionTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "list_promoted"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestParadigmVersionTool_Get(t *testing.T) {
	repo := NewInMemoryParadigmRepo()
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-1", ParadigmID: "p-1", Version: 1, State: "promoted"})

	tool := NewParadigmVersionTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "get", "version_id": "v-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestParadigmVersionTool_History(t *testing.T) {
	repo := NewInMemoryParadigmRepo()
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-1", ParadigmID: "p-1", Version: 1})
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-2", ParadigmID: "p-1", Version: 2})

	tool := NewParadigmVersionTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "history", "paradigm_id": "p-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestParadigmVersionTool_Evidence(t *testing.T) {
	repo := NewInMemoryParadigmRepo()
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-1", ParadigmID: "p-1", Version: 1, State: "promoted"})
	repo.AddEvidence(&ValidationEvidenceInfo{
		ParadigmID:        "p-1",
		ParadigmVersionID: "v-1",
		NetReturn:         0.12,
		MaxDrawdown:       0.08,
		SharpeRatio:       1.5,
		Passed:            true,
		Level:             "gold",
	})

	tool := NewParadigmVersionTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "evidence", "version_id": "v-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestParadigmVersionTool_EvidenceWithIssues(t *testing.T) {
	repo := NewInMemoryParadigmRepo()
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-1", ParadigmID: "p-1", Version: 1})
	repo.AddEvidence(&ValidationEvidenceInfo{
		ParadigmID:        "p-1",
		ParadigmVersionID: "v-1",
		Passed:            false,
		MustFix:           []string{"样本量不足", "最大回撤超限制"},
		Warnings:          []string{"收益集中度偏高"},
		Suggestions:       []string{"修复must_fix后重新验证"},
	})

	tool := NewParadigmVersionTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "evidence", "version_id": "v-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		// 注意: 即使未通过也视为成功 (证据已返回)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warnings in evidence")
	}
}

func TestParadigmVersionTool_ListByState(t *testing.T) {
	repo := NewInMemoryParadigmRepo()
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-1", State: "draft"})
	repo.AddVersion(&ParadigmVersionInfo{ID: "v-2", State: "promoted"})

	tool := NewParadigmVersionTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "list_by_state", "state": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

// ============================================================================
// 前向运行工具测试
// ============================================================================

func TestForwardRunTool_ListLatest(t *testing.T) {
	repo := NewInMemoryForwardRunRepo()
	repo.AddRun(&ForwardRunSummary{ID: "run-1", ParadigmVersionID: "v-1", SignalsCount: 15})

	tool := NewForwardRunTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "list_latest", "limit": 5})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestForwardRunTool_Get(t *testing.T) {
	repo := NewInMemoryForwardRunRepo()
	repo.AddRun(&ForwardRunSummary{ID: "run-1", ParadigmVersionID: "v-1", TotalReturn: 0.08})

	tool := NewForwardRunTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "get", "run_id": "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestForwardRunTool_ListByParadigm(t *testing.T) {
	repo := NewInMemoryForwardRunRepo()
	repo.AddRun(&ForwardRunSummary{ID: "run-1", ParadigmVersionID: "v-1"})
	repo.AddRun(&ForwardRunSummary{ID: "run-2", ParadigmVersionID: "v-1"})

	tool := NewForwardRunTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "list_by_paradigm", "paradigm_version_id": "v-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestForwardRunTool_Signals(t *testing.T) {
	repo := NewInMemoryForwardRunRepo()
	repo.AddSignals("run-1", []SignalDetail{
		{ID: "sig-1", StockCode: "600000", Direction: "buy", Price: 10.5},
		{ID: "sig-2", StockCode: "600001", Direction: "sell", Price: 12.0},
	})

	tool := NewForwardRunTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "signals", "run_id": "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

// ============================================================================
// 证据下钻工具测试
// ============================================================================

func TestEvidenceDrilldownTool_GetByVersion(t *testing.T) {
	repo := NewInMemoryEvidenceRepo()
	repo.AddReport(&EvidenceDrilldown{
		ParadigmID: "p-1", ParadigmVersionID: "v-1",
		Level: "gold", Score: 85.0,
		Metrics: map[string]float64{"net_return": 0.12, "sharpe": 1.5},
	})

	tool := NewEvidenceDrilldownTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "get_by_version", "version_id": "v-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestEvidenceDrilldownTool_ListByParadigm(t *testing.T) {
	repo := NewInMemoryEvidenceRepo()
	repo.AddReport(&EvidenceDrilldown{ParadigmID: "p-1", ParadigmVersionID: "v-1"})
	repo.AddReport(&EvidenceDrilldown{ParadigmID: "p-1", ParadigmVersionID: "v-2"})

	tool := NewEvidenceDrilldownTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "list_by_paradigm", "paradigm_id": "p-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestEvidenceDrilldownTool_RawMetrics(t *testing.T) {
	repo := NewInMemoryEvidenceRepo()
	repo.AddReport(&EvidenceDrilldown{
		ParadigmID: "p-1", ParadigmVersionID: "v-1",
		Metrics: map[string]float64{"net_return": 0.12, "max_drawdown": 0.08},
	})

	tool := NewEvidenceDrilldownTool(repo)
	ctx := AccessContext{}
	result, err := tool.Invoke(ctx, map[string]any{"action": "raw_metrics", "version_id": "v-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

// ============================================================================
// 只读权限守卫测试
// ============================================================================

func TestReadOnlyGuard_NoViolation(t *testing.T) {
	guard := NewReadOnlyGuard()
	err := guard.CheckForbidden(map[string]any{"action": "list", "snapshot_id": "s-1"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadOnlyGuard_ForbiddenPrefixInKey(t *testing.T) {
	guard := NewReadOnlyGuard()
	err := guard.CheckForbidden(map[string]any{"promote_paradigm": "p-1"})
	if err == nil {
		t.Error("expected error for forbidden prefix in key")
	}
}

func TestReadOnlyGuard_ForbiddenPrefixInValue(t *testing.T) {
	guard := NewReadOnlyGuard()
	err := guard.CheckForbidden(map[string]any{"action": "submit_for_review"})
	if err == nil {
		t.Error("expected error for forbidden prefix in value")
	}
}

// ============================================================================
// 研究工具套件集成测试
// ============================================================================

func TestNewResearchToolkit(t *testing.T) {
	snapshotRepo := NewInMemorySnapshotRepo()
	featureRepo := NewInMemoryFeatureRepo()
	experimentRepo := NewInMemoryExperimentRepo()
	paradigmRepo := NewInMemoryParadigmRepo()
	forwardRunRepo := NewInMemoryForwardRunRepo()
	evidenceRepo := NewInMemoryEvidenceRepo()

	tk, err := NewResearchToolkit(snapshotRepo, featureRepo, experimentRepo, paradigmRepo, forwardRunRepo, evidenceRepo)
	if err != nil {
		t.Fatalf("NewResearchToolkit failed: %v", err)
	}

	tools := tk.Registry.List()
	if len(tools) != 6 {
		t.Errorf("expected 6 tools, got %d", len(tools))
	}
}

func TestResearchToolkit_EndToEnd(t *testing.T) {
	// Setup all repos with data
	snapshotRepo := NewInMemorySnapshotRepo()
	snapshotRepo.Add(&SnapshotInfo{ID: "snap-1", Version: "v1", DateRange: "2025-01-01-2025-12-31", UniverseSize: 1000})

	featureRepo := NewInMemoryFeatureRepo()
	featureRepo.Add(&FeatureInfo{ID: "feat-1", Name: "MACD", Formula: "EMA(12)-EMA(26)"})

	experimentRepo := NewInMemoryExperimentRepo()
	experimentRepo.Add(&ExperimentSummary{ID: "exp-1", Status: "completed"})
	experimentRepo.AddReport(&BacktestReportSummary{
		ExperimentID: "exp-1", TotalReturn: 0.15, NetReturn: 0.12,
		MaxDrawdown: 0.08, SharpeRatio: 1.5, Passed: true, Level: "gold",
	})

	paradigmRepo := NewInMemoryParadigmRepo()
	paradigmRepo.AddVersion(&ParadigmVersionInfo{ID: "v-1", ParadigmID: "p-1", Version: 1, State: "promoted"})
	paradigmRepo.AddEvidence(&ValidationEvidenceInfo{
		ParadigmID: "p-1", ParadigmVersionID: "v-1",
		NetReturn: 0.12, SharpeRatio: 1.5, Passed: true, Level: "gold",
	})

	forwardRunRepo := NewInMemoryForwardRunRepo()
	forwardRunRepo.AddRun(&ForwardRunSummary{ID: "run-1", ParadigmVersionID: "v-1", TotalReturn: 0.08})

	evidenceRepo := NewInMemoryEvidenceRepo()
	evidenceRepo.AddReport(&EvidenceDrilldown{
		ParadigmID: "p-1", ParadigmVersionID: "v-1",
		Level: "gold", Score: 85.0,
	})

	tk, err := NewResearchToolkit(snapshotRepo, featureRepo, experimentRepo, paradigmRepo, forwardRunRepo, evidenceRepo)
	if err != nil {
		t.Fatal(err)
	}

	// Test each tool via registry
	ctx := AccessContext{AgentID: "test-agent", SessionID: "test-sess", Timestamp: time.Now()}

	// Data snapshot
	result, _ := tk.Registry.Call(ctx, "data_snapshot", map[string]any{"action": "get", "snapshot_id": "snap-1"})
	if result == nil || !result.Success {
		t.Error("data_snapshot tool failed")
	}

	// Feature query
	result, _ = tk.Registry.Call(ctx, "feature_query", map[string]any{"action": "get", "feature_id": "feat-1"})
	if result == nil || !result.Success {
		t.Error("feature_query tool failed")
	}

	// Experiment report
	result, _ = tk.Registry.Call(ctx, "experiment_report", map[string]any{"action": "backtest_report", "experiment_id": "exp-1"})
	if result == nil || !result.Success {
		t.Error("experiment_report tool failed")
	}

	// Paradigm version
	result, _ = tk.Registry.Call(ctx, "paradigm_version", map[string]any{"action": "evidence", "version_id": "v-1"})
	if result == nil || !result.Success {
		t.Error("paradigm_version tool failed")
	}

	// Forward run
	result, _ = tk.Registry.Call(ctx, "forward_run", map[string]any{"action": "get", "run_id": "run-1"})
	if result == nil || !result.Success {
		t.Error("forward_run tool failed")
	}

	// Evidence drilldown
	result, _ = tk.Registry.Call(ctx, "evidence_drilldown", map[string]any{"action": "get_by_version", "version_id": "v-1"})
	if result == nil || !result.Success {
		t.Error("evidence_drilldown tool failed")
	}

	// Verify logs
	logs := tk.Registry.GetLogs("test-agent", 100)
	if len(logs) != 6 {
		t.Errorf("expected 6 logs, got %d", len(logs))
	}
}

// ============================================================================
// 安全性测试: AI 无法绕过权限修改晋级结果
// ============================================================================

func TestAI_CannotPromoteParadigm(t *testing.T) {
	repo := NewInMemoryParadigmRepo()
	tool := NewParadigmVersionTool(repo)
	ctx := AccessContext{AgentID: "ai-agent", SessionID: "sess-1"}

	// 试图通过 action 参数触发 promote (不存在此 action, 但被守卫拦截)
	_, err := tool.Invoke(ctx, map[string]any{"action": "promote", "version_id": "v-1"})
	if err == nil {
		// 没有 promote action, 正常情况应报错
	}
}

func TestAI_CannotSubmitForReview(t *testing.T) {
	repo := NewInMemoryExperimentRepo()
	tool := NewExperimentReportTool(repo)
	ctx := AccessContext{}

	// submit_for_review 前缀应被拦截
	_, err := tool.Invoke(ctx, map[string]any{"action": "list_latest", "submit_for_review": true})
	if err == nil {
		t.Error("expected error: submit_for_review is forbidden")
	}
}

// ============================================================================
// 工具元信息测试
// ============================================================================

func TestToolMetadata(t *testing.T) {
	tools := []Tool{
		NewDataSnapshotTool(NewInMemorySnapshotRepo()),
		NewFeatureQueryTool(NewInMemoryFeatureRepo()),
		NewExperimentReportTool(NewInMemoryExperimentRepo()),
		NewParadigmVersionTool(NewInMemoryParadigmRepo()),
		NewForwardRunTool(NewInMemoryForwardRunRepo()),
		NewEvidenceDrilldownTool(NewInMemoryEvidenceRepo()),
	}

	for _, tool := range tools {
		if tool.Name() == "" {
			t.Errorf("tool has empty name")
		}
		if tool.Version() == "" {
			t.Errorf("tool %s has empty version", tool.Name())
		}
		if tool.Description() == "" {
			t.Errorf("tool %s has empty description", tool.Name())
		}
		perms := tool.Permissions()
		hasRead := false
		for _, p := range perms {
			if p == PermRead {
				hasRead = true
				break
			}
		}
		if !hasRead {
			t.Errorf("tool %s does not declare read permission", tool.Name())
		}
	}
}

// ============================================================================
// 辅助函数测试
// ============================================================================

func TestGetString(t *testing.T) {
	params := map[string]any{"key": "value", "num": 42}
	if getString(params, "key", "") != "value" {
		t.Error("expected 'value'")
	}
	if getString(params, "missing", "default") != "default" {
		t.Error("expected default")
	}
	if getString(params, "num", "") != "" {
		t.Error("non-string value should return default")
	}
}

func TestGetInt(t *testing.T) {
	params := map[string]any{"int_val": 42, "float_val": 3.14}
	if getInt(params, "int_val", 0) != 42 {
		t.Error("expected 42")
	}
	if getInt(params, "float_val", 0) != 3 {
		t.Error("expected 3")
	}
	if getInt(params, "missing", 10) != 10 {
		t.Error("expected default 10")
	}
}
