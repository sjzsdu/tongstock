package ledger

import (
	"fmt"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/trading"
)

func newTestRun(t *testing.T, ledger *SignalLedger, version string, start time.Time) *ForwardRun {
	t.Helper()
	run, err := ledger.NewForwardRun(
		version, start, 1_000_000,
		trading.DefaultTradingConstraints(), trading.DefaultCostModel(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func completeTestSignal(entry SignalEntry) SignalEntry {
	if entry.DataSnapshot.DataHash == "" {
		entry.DataSnapshot.DataHash = "server-captured-test-hash"
	}
	if entry.DataSnapshot.CapturedAt.IsZero() || entry.DataSnapshot.CapturedAt.After(entry.SignalDate) {
		entry.DataSnapshot.CapturedAt = entry.SignalDate
	}
	if entry.Market.Date.IsZero() {
		entry.Market = ExecutionMarket{
			Date: entry.ExecutionDate, Open: entry.Price, High: entry.Price,
			Low: entry.Price, Close: entry.Price, PreClose: entry.PreClose,
			LimitUp: entry.LimitUp, LimitDown: entry.LimitDown,
			Suspended: entry.Suspended, Board: entry.Board, Volume: 1,
		}
	}
	return entry
}

// ============================================================================
// SignalLedger 测试
// ============================================================================

func TestNewSignalLedger(t *testing.T) {
	ledger := NewSignalLedger()
	if ledger == nil {
		t.Fatal("NewSignalLedger returned nil")
	}
	if len(ledger.entries) != 0 {
		t.Error("new ledger should be empty")
	}
}

func TestAppendSignal(t *testing.T) {
	ledger := NewSignalLedger()
	start := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	run := newTestRun(t, ledger, "pv-001", start)

	entry := completeTestSignal(SignalEntry{
		ID:                "sig-001",
		RunID:             run.ID,
		ParadigmVersionID: "pv-001",
		StockCode:         "600000",
		Direction:         "buy",
		SignalDate:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local),
		ExecutionDate:     time.Date(2024, 1, 16, 0, 0, 0, 0, time.Local),
		Price:             10.50,
		PreClose:          10.00,
		LimitUp:           11.00,
		LimitDown:         9.00,
		Suspended:         false,
		Board:             "main",
		Confidence:        0.85,
		DataSnapshot: DataSnapshot{
			DatasetID:    "ds-v1",
			FeatureSetID: "fs-v2",
			RuleSetID:    "rs-v3",
			DataHash:     "abc123",
			CapturedAt:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local),
		},
		Source: SignalSource{
			RuleID:      "rule-001",
			RuleDesc:    "突破前高放量",
			TriggeredBy: "close > high_20 && volume > avg_volume_5 * 1.5",
		},
	})

	if err := ledger.AppendSignal(entry); err != nil {
		t.Fatalf("AppendSignal failed: %v", err)
	}

	if len(ledger.entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(ledger.entries))
	}

	stored, err := ledger.GetSignal("sig-001")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ContentHash == "" {
		t.Error("ContentHash should be set after append")
	}
}

func TestAppendSignalDuplicate(t *testing.T) {
	ledger := NewSignalLedger()
	now := time.Now()
	run := newTestRun(t, ledger, "pv-001", now)

	entry := completeTestSignal(SignalEntry{
		ID:                "sig-001",
		RunID:             run.ID,
		ParadigmVersionID: "pv-001",
		StockCode:         "600000",
		Direction:         "buy",
		SignalDate:        time.Now(),
		ExecutionDate:     time.Now(),
		Price:             10.0,
		DataSnapshot: DataSnapshot{
			DatasetID:  "ds-v1",
			CapturedAt: time.Now(),
		},
	})

	if err := ledger.AppendSignal(entry); err != nil {
		t.Fatal(err)
	}

	// 同一个不可变信号的重试是幂等的。
	if err := ledger.AppendSignal(entry); err != nil {
		t.Fatalf("identical retry should be idempotent: %v", err)
	}
	entry.Price = 10.1
	if err := ledger.AppendSignal(entry); err == nil {
		t.Error("expected conflict for same ID with different immutable content")
	}
}

func TestGetSignalHashValidation(t *testing.T) {
	ledger := NewSignalLedger()
	now := time.Now()
	run := newTestRun(t, ledger, "pv-001", now)

	entry := completeTestSignal(SignalEntry{
		ID:                "sig-001",
		RunID:             run.ID,
		ParadigmVersionID: "pv-001",
		StockCode:         "600000",
		Direction:         "buy",
		SignalDate:        time.Now(),
		ExecutionDate:     time.Now(),
		Price:             10.0,
		DataSnapshot: DataSnapshot{
			DatasetID:  "ds-v1",
			CapturedAt: time.Now(),
		},
	})

	if err := ledger.AppendSignal(entry); err != nil {
		t.Fatal(err)
	}

	// 获取信号 (验证 hash)
	got, err := ledger.GetSignal("sig-001")
	if err != nil {
		t.Fatalf("GetSignal failed: %v", err)
	}
	if got.ContentHash == "" {
		t.Error("ContentHash should be non-empty")
	}
	if got.StockCode != "600000" {
		t.Errorf("expected stock 600000, got %s", got.StockCode)
	}
}

func TestUpdateExecution(t *testing.T) {
	ledger := NewSignalLedger()
	now := time.Now()
	run := newTestRun(t, ledger, "pv-001", now)

	entry := completeTestSignal(SignalEntry{
		ID:                "sig-001",
		RunID:             run.ID,
		ParadigmVersionID: "pv-001",
		StockCode:         "600000",
		Direction:         "buy",
		SignalDate:        time.Now(),
		ExecutionDate:     time.Now(),
		Price:             10.0,
		DataSnapshot: DataSnapshot{
			DatasetID:  "ds-v1",
			CapturedAt: time.Now(),
		},
	})

	if err := ledger.AppendSignal(entry); err != nil {
		t.Fatal(err)
	}

	exec := ExecutionRecord{
		Status:     "filled",
		ExecPrice:  10.1,
		ExecQty:    1000,
		Fee:        5.0,
		ExecutedAt: time.Now(),
	}

	if err := ledger.UpdateExecution("sig-001", exec); err != nil {
		t.Fatalf("UpdateExecution failed: %v", err)
	}

	got, err := ledger.GetSignal("sig-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Execution == nil {
		t.Fatal("Execution should be set")
	}
	if got.Execution.Status != "filled" {
		t.Errorf("expected filled, got %s", got.Execution.Status)
	}
	if got.Execution.ExecPrice != 10.1 {
		t.Errorf("expected price 10.1, got %f", got.Execution.ExecPrice)
	}
}

func TestListByRun(t *testing.T) {
	ledger := NewSignalLedger()

	date1 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	date2 := time.Date(2024, 1, 16, 0, 0, 0, 0, time.Local)
	runID := newTestRun(t, ledger, "pv-001", date1).ID

	entries := []SignalEntry{
		{
			ID: "sig-001", RunID: runID, ParadigmVersionID: "pv-001",
			StockCode: "600000", Direction: "buy",
			SignalDate: date1, ExecutionDate: date1.AddDate(0, 0, 1),
			Price: 10.0, DataSnapshot: DataSnapshot{DatasetID: "ds", CapturedAt: date1},
		},
		{
			ID: "sig-002", RunID: runID, ParadigmVersionID: "pv-001",
			StockCode: "600001", Direction: "sell",
			SignalDate: date2, ExecutionDate: date2.AddDate(0, 0, 1),
			Price: 12.0, DataSnapshot: DataSnapshot{DatasetID: "ds", CapturedAt: date2},
		},
	}

	for _, e := range entries {
		e = completeTestSignal(e)
		if err := ledger.AppendSignal(e); err != nil {
			t.Fatal(err)
		}
	}

	list := ledger.ListByRun(runID)
	if len(list) != 2 {
		t.Errorf("expected 2 entries, got %d", len(list))
	}
	// 应该按日期升序
	if list[0].SignalDate.After(list[1].SignalDate) {
		t.Error("entries should be sorted by date ascending")
	}
}

func TestValidateSignals(t *testing.T) {
	ledger := NewSignalLedger()
	now := time.Now()
	run := newTestRun(t, ledger, "pv-001", now)

	entry := completeTestSignal(SignalEntry{
		ID:                "sig-001",
		RunID:             run.ID,
		ParadigmVersionID: "pv-001",
		StockCode:         "600000",
		Direction:         "buy",
		SignalDate:        time.Now(),
		ExecutionDate:     time.Now(),
		Price:             10.0,
		DataSnapshot: DataSnapshot{
			DatasetID:  "ds-v1",
			CapturedAt: time.Now(),
		},
	})

	entry = completeTestSignal(entry)
	if err := ledger.AppendSignal(entry); err != nil {
		t.Fatal(err)
	}

	invalid, issues := ledger.ValidateSignals()
	if invalid != 0 {
		t.Errorf("expected 0 invalid, got %d: %v", invalid, issues)
	}
}

// ============================================================================
// ForwardRun 测试
// ============================================================================

func TestNewForwardRun(t *testing.T) {
	ledger := NewSignalLedger()
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	run, err := ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err != nil {
		t.Fatalf("NewForwardRun failed: %v", err)
	}

	if run.ID != "fr-pv-001-20240115" {
		t.Errorf("unexpected run ID: %s", run.ID)
	}
	if run.Status != "active" {
		t.Errorf("expected active, got %s", run.Status)
	}
	if run.InitialCash != 1000000 {
		t.Errorf("expected cash 1000000, got %f", run.InitialCash)
	}
	if !run.ConstraintsSnapshot.EnableT1 {
		t.Error("T+1 should be enabled by default")
	}
}

func TestNewForwardRunDuplicate(t *testing.T) {
	ledger := NewSignalLedger()
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	_, err := ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err != nil {
		t.Fatal(err)
	}

	// 重复创建应该失败
	_, err = ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err == nil {
		t.Error("expected error for duplicate forward run")
	}
}

func TestFinalizeRun(t *testing.T) {
	ledger := NewSignalLedger()
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	run, err := ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err != nil {
		t.Fatal(err)
	}

	// 追加一条已执行的信号
	entry := SignalEntry{
		ID:                "sig-001",
		RunID:             run.ID,
		ParadigmVersionID: "pv-001",
		StockCode:         "600000",
		Direction:         "buy",
		SignalDate:        startDate,
		ExecutionDate:     startDate.AddDate(0, 0, 1),
		Price:             10.0,
		DataSnapshot: DataSnapshot{
			DatasetID: "ds", CapturedAt: startDate,
		},
	}
	entry = completeTestSignal(entry)
	if err := ledger.AppendSignal(entry); err != nil {
		t.Fatal(err)
	}

	// 更新执行结果
	exec := ExecutionRecord{
		Status: "filled", ExecPrice: 10.1, ExecQty: 1000, Fee: 5.0,
		ExecutedAt: startDate.AddDate(0, 0, 1),
	}
	if err := ledger.UpdateExecution("sig-001", exec); err != nil {
		t.Fatal(err)
	}

	endDate := time.Date(2024, 6, 30, 0, 0, 0, 0, time.Local)
	finalized, err := ledger.FinalizeRun(run.ID, endDate)
	if err != nil {
		t.Fatalf("FinalizeRun failed: %v", err)
	}

	if finalized.Status != "completed" {
		t.Errorf("expected completed, got %s", finalized.Status)
	}
	if finalized.EndDate != nil && !finalized.EndDate.Equal(endDate) {
		t.Error("end date should be set")
	}
}

// ============================================================================
// PaperTradeEngine 测试
// ============================================================================

func TestNewPaperTradeEngine(t *testing.T) {
	ledger := NewSignalLedger()
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	run, err := ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err != nil {
		t.Fatal(err)
	}

	engine, err := NewPaperTradeEngine(ledger, run.ID, constraints, costModel, 1000000)
	if err != nil {
		t.Fatalf("NewPaperTradeEngine failed: %v", err)
	}

	if engine == nil {
		t.Fatal("engine should not be nil")
	}
}

func TestExecuteSignalBuy(t *testing.T) {
	ledger := NewSignalLedger()
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	run, err := ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err != nil {
		t.Fatal(err)
	}

	engine, err := NewPaperTradeEngine(ledger, run.ID, constraints, costModel, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	entry := SignalEntry{
		ID: "sig-001", RunID: run.ID, ParadigmVersionID: "pv-001",
		StockCode: "600000", Direction: "buy",
		SignalDate:    startDate,
		ExecutionDate: startDate.AddDate(0, 0, 1),
		Price:         10.0, PreClose: 9.5,
		LimitUp: 10.45, LimitDown: 8.55,
		Suspended: false, Board: "main",
		DataSnapshot: DataSnapshot{DatasetID: "ds", CapturedAt: startDate},
	}
	entry = completeTestSignal(entry)
	if err := ledger.AppendSignal(entry); err != nil {
		t.Fatal(err)
	}

	exec, err := engine.ExecuteSignal(entry, entry.Market)
	if err != nil {
		t.Fatalf("ExecuteSignal failed: %v", err)
	}

	if exec.Status != "filled" {
		t.Errorf("expected filled, got %s (reason: %s)", exec.Status, exec.RejectReason)
	}
	if exec.ExecQty <= 0 {
		t.Error("executed quantity should be positive")
	}
	if exec.ExecPrice <= 0 {
		t.Error("executed price should be positive")
	}
}

func TestExecuteSignalRejected(t *testing.T) {
	ledger := NewSignalLedger()
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	run, err := ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err != nil {
		t.Fatal(err)
	}

	engine, err := NewPaperTradeEngine(ledger, run.ID, constraints, costModel, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	// 停牌股票应该被拒绝
	entry := SignalEntry{
		ID: "sig-suspended", RunID: run.ID, ParadigmVersionID: "pv-001",
		StockCode: "600000", Direction: "buy",
		SignalDate:    startDate,
		ExecutionDate: startDate.AddDate(0, 0, 1),
		Price:         10.0, PreClose: 9.5,
		LimitUp: 10.45, LimitDown: 8.55,
		Suspended: true, Board: "main",
		DataSnapshot: DataSnapshot{DatasetID: "ds", CapturedAt: startDate},
	}
	entry = completeTestSignal(entry)
	if err := ledger.AppendSignal(entry); err != nil {
		t.Fatal(err)
	}

	exec, err := engine.ExecuteSignal(entry, entry.Market)
	if err != nil {
		t.Fatalf("ExecuteSignal failed: %v", err)
	}

	if exec.Status != "rejected" {
		t.Errorf("expected rejected for suspended stock, got %s", exec.Status)
	}
}

func TestExecuteAllPending(t *testing.T) {
	ledger := NewSignalLedger()
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	run, err := ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err != nil {
		t.Fatal(err)
	}

	engine, err := NewPaperTradeEngine(ledger, run.ID, constraints, costModel, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	// 追加多条信号
	for i := 0; i < 5; i++ {
		entry := SignalEntry{
			ID: fmt.Sprintf("sig-%03d", i), RunID: run.ID, ParadigmVersionID: "pv-001",
			StockCode: "600000", Direction: "buy",
			SignalDate:    startDate.AddDate(0, 0, i),
			ExecutionDate: startDate.AddDate(0, 0, i+1),
			Price:         10.0 + float64(i)*0.1, PreClose: 9.5,
			LimitUp: 10.45, LimitDown: 8.55,
			Suspended: false, Board: "main",
			DataSnapshot: DataSnapshot{DatasetID: "ds", CapturedAt: startDate},
		}
		entry = completeTestSignal(entry)
		if err := ledger.AppendSignal(entry); err != nil {
			t.Fatal(err)
		}
	}

	executed, rejected, err := engine.ExecuteAllPending(
		func(entry SignalEntry) (ExecutionMarket, error) { return entry.Market, nil },
	)
	if err != nil {
		t.Fatalf("ExecuteAllPending failed: %v", err)
	}

	if executed != 5 {
		t.Errorf("expected 5 executed, got %d", executed)
	}
	if rejected != 0 {
		t.Errorf("expected 0 rejected, got %d", rejected)
	}
}

// ============================================================================
// ComparisonReport 测试
// ============================================================================

func TestNewComparisonReport(t *testing.T) {
	ledger := NewSignalLedger()
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	run, err := ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err != nil {
		t.Fatal(err)
	}

	theoretical := TheoreticalMetrics{
		TotalReturn:   0.15,
		AnnualizedRet: 0.30,
		MaxDrawdown:   0.05,
		SharpeRatio:   2.0,
		WinRate:       0.65,
		SignalCount:   20,
		IdealPnL:      150000,
	}

	report := NewComparisonReport("pv-001", run.ID, theoretical, run, nil)
	if report == nil {
		t.Fatal("report should not be nil")
	}
	if report.Gap.ReturnGapPct < 0 {
		t.Error("return gap should be non-negative (actual <= theoretical)")
	}
}

func TestValidateGapThreshold(t *testing.T) {
	ledger := NewSignalLedger()
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)
	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	run, err := ledger.NewForwardRun("pv-001", startDate, 1000000, constraints, costModel)
	if err != nil {
		t.Fatal(err)
	}

	theoretical := TheoreticalMetrics{
		TotalReturn:   0.15,
		AnnualizedRet: 0.30,
		MaxDrawdown:   0.05,
		SharpeRatio:   2.0,
		WinRate:       0.65,
		SignalCount:   20,
		IdealPnL:      150000,
	}

	report := NewComparisonReport("pv-001", run.ID, theoretical, run, nil)

	// 宽松阈值应该通过
	ok, warnings := report.ValidateGapThreshold(1.0, 1.0)
	if !ok {
		t.Errorf("expected pass with loose thresholds, got warnings: %v", warnings)
	}
}
