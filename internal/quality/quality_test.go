package quality

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func setupQualityStore(t *testing.T) *QualityStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quality_test.db")
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewQualityStore(s)
}

func makeRecords(dates []time.Time, closes []float64) []KlineRecord {
	records := make([]KlineRecord, len(dates))
	for i := range dates {
		records[i] = KlineRecord{
			Date:   dates[i],
			Open:   closes[i] - 0.5,
			High:   closes[i] + 1.0,
			Low:    closes[i] - 1.0,
			Close:  closes[i],
			Volume: 1000000,
			Amount: closes[i] * 1000000,
		}
	}
	return records
}

func makeDates(n int) []time.Time {
	dates := make([]time.Time, n)
	base := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local) // 周二
	for i := 0; i < n; i++ {
		day := base.AddDate(0, 0, i*2) // 每两天一个交易日
		dates[i] = day
	}
	return dates
}

func makeExpectedDays(n int) []time.Time {
	dates := make([]time.Time, n)
	base := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local)
	for i := 0; i < n; i++ {
		dates[i] = base.AddDate(0, 0, i*2)
	}
	return dates
}

// ==================== 基础检查测试 ====================

func TestQualityReport_EmptyReport(t *testing.T) {
	report := &QualityReport{
		ID:      "test-empty",
		Issues:  nil,
		Passed:  true,
		Blocked: false,
		Summary: QualitySummary{},
	}

	if report.HasCriticalIssues() {
		t.Error("Empty report should not have critical issues")
	}

	cfg := DefaultQualityGateConfig()
	if report.ShouldBlock(cfg) {
		t.Error("Empty report should not be blocked")
	}
}

func TestQualityReport_WithCriticalIssues(t *testing.T) {
	report := &QualityReport{
		ID: "test-critical",
		Issues: []QualityIssue{
			{Severity: SeverityCritical, Category: CategoryDuplicate},
			{Severity: SeverityWarning, Category: CategoryMissingData},
		},
	}

	if !report.HasCriticalIssues() {
		t.Error("Should have critical issues")
	}

	if report.IssueCountBySeverity(SeverityCritical) != 1 {
		t.Error("Critical count should be 1")
	}
	if report.IssueCountBySeverity(SeverityWarning) != 1 {
		t.Error("Warning count should be 1")
	}

	cfg := DefaultQualityGateConfig()
	if !report.ShouldBlock(cfg) {
		t.Error("Should be blocked when critical issues exist")
	}

	cfg.BlockOnCritical = false
	if report.ShouldBlock(cfg) {
		t.Error("Should not be blocked when BlockOnCritical is false")
	}
}

func TestNewQualityIssue(t *testing.T) {
	issue := NewQualityIssue(
		CategoryMissingData, SeverityCritical, "600000", "2024-01-15",
		"coverage", "100%", "80%", "Missing 20% of data",
	)

	if issue.ID == "" {
		t.Error("Issue ID should not be empty")
	}
	if issue.Category != CategoryMissingData {
		t.Errorf("Category = %s, want %s", issue.Category, CategoryMissingData)
	}
	if issue.Severity != SeverityCritical {
		t.Errorf("Severity = %s, want %s", issue.Severity, SeverityCritical)
	}
	if issue.StockCode != "600000" {
		t.Errorf("StockCode = %s, want 600000", issue.StockCode)
	}
}

// ==================== QualityChecker 测试 ====================

func TestQualityChecker_NoIssues(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	dates := makeDates(10)
	closes := make([]float64, 10)
	for i := range closes {
		closes[i] = 10.0 + float64(i)
	}
	records := makeRecords(dates, closes)
	expectedDays := makeExpectedDays(10)

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	if report.HasCriticalIssues() {
		t.Errorf("No critical issues expected, got: %v", report.Issues)
	}
	if !report.Passed {
		t.Error("Report should pass")
	}
	if report.Summary.TotalIssues != 0 {
		t.Errorf("TotalIssues = %d, want 0", report.Summary.TotalIssues)
	}
}

func TestQualityChecker_DuplicateRecords(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	dates := makeDates(5)
	closes := []float64{10, 11, 12, 13, 14}
	records := makeRecords(dates, closes)
	// 复制一条记录, 制造重复
	records = append(records, records[2]) // 重复第 3 条
	expectedDays := makeExpectedDays(6)

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	if !report.HasCriticalIssues() {
		t.Error("Duplicate records should be critical")
	}

	found := false
	for _, issue := range report.Issues {
		if issue.Category == CategoryDuplicate {
			found = true
			if issue.Severity != SeverityCritical {
				t.Error("Duplicate should be critical severity")
			}
		}
	}
	if !found {
		t.Error("Should have duplicate issue")
	}
}

func TestQualityChecker_TimeReversal(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	dates := makeDates(5)
	closes := []float64{10, 11, 12, 13, 14}
	records := makeRecords(dates, closes)
	// 交换最后两条, 制造时间倒序
	records[3], records[4] = records[4], records[3]
	expectedDays := makeExpectedDays(5)

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	if !report.HasCriticalIssues() {
		t.Error("Time reversal should be critical")
	}

	found := false
	for _, issue := range report.Issues {
		if issue.Category == CategoryTimeReversal {
			found = true
		}
	}
	if !found {
		t.Error("Should have time reversal issue")
	}
}

func TestQualityChecker_AbnormalPrice(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	cfg.MaxPriceChangePct = 5.0
	checker := NewQualityChecker(cfg)

	dates := makeDates(5)
	closes := []float64{10, 10, 10, 10, 10}
	records := makeRecords(dates, closes)
	// 制造极端价格跳变 (10x)
	records[3].Close = 100.0
	records[3].High = 101.0
	records[3].Low = 99.0
	records[3].Open = 99.5
	expectedDays := makeExpectedDays(5)

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	found := false
	for _, issue := range report.Issues {
		if issue.Category == CategoryAbnormalPrice {
			found = true
		}
	}
	if !found {
		t.Error("Should have abnormal price issue")
	}
}

func TestQualityChecker_InvalidPrice(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	dates := makeDates(3)
	closes := []float64{10, 0, 12} // 中间一条 close=0
	records := makeRecords(dates, closes)
	expectedDays := makeExpectedDays(3)

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	if !report.HasCriticalIssues() {
		t.Error("Invalid price (0) should be critical")
	}
}

func TestQualityChecker_HighLowSwap(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	dates := makeDates(3)
	closes := []float64{10, 11, 12}
	records := makeRecords(dates, closes)
	// High < Low 异常
	records[1].High = 5.0
	records[1].Low = 15.0
	expectedDays := makeExpectedDays(3)

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	if !report.HasCriticalIssues() {
		t.Error("High < Low should be critical")
	}
}

func TestQualityChecker_MissingData(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	cfg.MaxMissingDays = 2
	checker := NewQualityChecker(cfg)

	dates := makeDates(3)
	closes := []float64{10, 11, 12}
	records := makeRecords(dates, closes)
	// 预期 10 天, 只有 3 天
	expectedDays := makeExpectedDays(10)

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	found := false
	for _, issue := range report.Issues {
		if issue.Category == CategoryMissingData {
			found = true
			if issue.Severity != SeverityCritical {
				t.Errorf("Missing 7 days (max 2) should be critical, got %s", issue.Severity)
			}
		}
	}
	if !found {
		t.Error("Should have missing data issue")
	}
}

func TestQualityChecker_MinorMissing(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	cfg.MaxMissingDays = 5
	checker := NewQualityChecker(cfg)

	dates := makeDates(9)
	closes := make([]float64, 9)
	for i := range closes {
		closes[i] = 10.0 + float64(i)
	}
	records := makeRecords(dates, closes)
	expectedDays := makeExpectedDays(10) // 缺 1 天 (< 5)

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	found := false
	for _, issue := range report.Issues {
		if issue.Category == CategoryMissingData {
			found = true
			if issue.Severity != SeverityWarning {
				t.Errorf("Missing 1 day (max 5) should be warning, got %s", issue.Severity)
			}
		}
	}
	if !found {
		t.Error("Should have missing data warning")
	}
}

func TestQualityChecker_AbnormalVolume(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	cfg.MaxVolumeRatio = 5.0
	checker := NewQualityChecker(cfg)

	dates := makeDates(10)
	closes := make([]float64, 10)
	for i := range closes {
		closes[i] = 10.0
	}
	records := makeRecords(dates, closes)
	// 最后一天成交量异常
	records[9].Volume = 100000000 // 100x 平均
	expectedDays := makeExpectedDays(10)

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	found := false
	for _, issue := range report.Issues {
		if issue.Category == CategoryAbnormalVolume {
			found = true
			if issue.Severity != SeverityWarning {
				t.Error("Abnormal volume should be warning severity")
			}
		}
	}
	if !found {
		t.Error("Should have abnormal volume issue")
	}
}

func TestQualityChecker_PoolCoverage(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	cfg.MinCoveragePercent = 95.0
	checker := NewQualityChecker(cfg)

	dates := makeDates(5)
	closes := make([]float64, 5)
	for i := range closes {
		closes[i] = 10.0
	}
	records := makeRecords(dates, closes)
	expectedDays := makeExpectedDays(20) // 只有 25% 覆盖率

	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	found := false
	for _, issue := range report.Issues {
		if issue.Category == CategoryPoolCoverage {
			found = true
			if issue.Severity != SeverityCritical {
				t.Errorf("25%% coverage (< 50%%) should be critical, got %s", issue.Severity)
			}
		}
	}
	if !found {
		t.Error("Should have pool coverage issue")
	}

	if report.Summary.CoveragePercent > 50.0 {
		t.Error("Coverage should be 25% (5/20)")
	}
}

func TestQualityChecker_SourceDegradation(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	dates := makeDates(10)
	closes := make([]float64, 10)
	for i := range closes {
		closes[i] = 10.0
	}
	records := makeRecords(dates, closes)
	expectedDays := makeExpectedDays(10)

	// asOf 距最后一天很远 (30 天)
	asOf := dates[len(dates)-1].AddDate(0, 0, 30)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	found := false
	for _, issue := range report.Issues {
		if issue.Category == CategorySourceDegradation {
			found = true
		}
	}
	if !found {
		t.Error("Should have source degradation issue")
	}
}

func TestQualityChecker_FinancialLag(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	latestReport := time.Date(2023, 6, 30, 0, 0, 0, 0, time.Local)
	asOf := time.Date(2024, 6, 30, 0, 0, 0, 0, time.Local) // 一年后

	issue := checker.CheckFinancialLag("600000", latestReport, asOf)

	if issue.Severity != SeverityCritical {
		t.Errorf("1 year financial lag should be critical, got %s", issue.Severity)
	}
	if issue.Category != CategoryFinancialLag {
		t.Errorf("Category = %s, want %s", issue.Category, CategoryFinancialLag)
	}
}

func TestQualityChecker_MildFinancialLag(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	latestReport := time.Date(2024, 4, 15, 0, 0, 0, 0, time.Local)
	asOf := time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local) // 47 天

	issue := checker.CheckFinancialLag("600000", latestReport, asOf)

	if issue.Severity != SeverityWarning {
		t.Errorf("47-day financial lag should be warning, got %s", issue.Severity)
	}
}

// ==================== QualityStore 测试 ====================

func TestQualityStore_SaveAndGetReport(t *testing.T) {
	store := setupQualityStore(t)
	now := time.Now().Truncate(time.Second)

	report := &QualityReport{
		ID:         "qr-001",
		SnapshotID: "snap-001",
		StockCode:  "600000",
		DateRange:  DateRangeInfo{Start: "2024-01-01", End: "2024-06-30"},
		Source:     "kline",
		AsOf:       now,
		Passed:     true,
		Blocked:    false,
		Summary: QualitySummary{
			TotalIssues:     2,
			WarningCount:    2,
			CheckedRecords:  100,
			PassedRecords:   98,
			FailedRecords:   2,
			CoveragePercent: 98.0,
		},
		Issues: []QualityIssue{
			{
				ID: "issue-1", Category: CategoryAbnormalVolume, Severity: SeverityWarning,
				StockCode: "600000", Date: "2024-03-15", Metric: "volume",
				Expected: "< 10x avg", Actual: "15x",
				Description: "Volume spike on 2024-03-15",
			},
			{
				ID: "issue-2", Category: CategoryMissingData, Severity: SeverityWarning,
				StockCode: "600000", Metric: "coverage",
				Expected: "100%", Actual: "98%",
				Description: "2 days missing",
			},
		},
		CreatedAt: now,
	}

	if err := store.SaveReport(report); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	got, err := store.GetReport("qr-001")
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}

	if got.StockCode != "600000" {
		t.Errorf("StockCode = %s, want 600000", got.StockCode)
	}
	if !got.Passed {
		t.Error("Report should be passed")
	}
	if got.Summary.TotalIssues != 2 {
		t.Errorf("TotalIssues = %d, want 2", got.Summary.TotalIssues)
	}
	if len(got.Issues) != 2 {
		t.Errorf("Issues len = %d, want 2", len(got.Issues))
	}
	if got.Issues[0].Category != CategoryAbnormalVolume {
		t.Errorf("First issue category = %s, want abnormal_volume", got.Issues[0].Category)
	}
}

func TestQualityStore_GetReportsBySnapshot(t *testing.T) {
	store := setupQualityStore(t)
	now := time.Now().Truncate(time.Second)

	for i := 0; i < 3; i++ {
		report := &QualityReport{
			ID:         fmt.Sprintf("qr-snap-%d", i),
			SnapshotID: "snap-shared",
			StockCode:  "600000",
			Source:     "kline",
			AsOf:       now,
			Passed:     true,
			CreatedAt:  now.Add(time.Duration(i) * time.Hour),
			Summary:    QualitySummary{},
		}
		store.SaveReport(report)
	}

	reports, err := store.GetReportsBySnapshot("snap-shared")
	if err != nil {
		t.Fatalf("GetReportsBySnapshot: %v", err)
	}
	if len(reports) != 3 {
		t.Errorf("Reports = %d, want 3", len(reports))
	}
}

func TestQualityStore_SaveAndLoadConfig(t *testing.T) {
	store := setupQualityStore(t)

	cfg := QualityGateConfig{
		MaxPriceChangePct:   3.0,
		MaxVolumeRatio:      8.0,
		MinCoveragePercent:  90.0,
		MaxMissingDays:      3,
		MaxFinancialLagDays: 45,
		BlockOnCritical:     false,
	}

	if err := store.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.MaxPriceChangePct != 3.0 {
		t.Errorf("MaxPriceChangePct = %f, want 3.0", loaded.MaxPriceChangePct)
	}
	if loaded.MaxMissingDays != 3 {
		t.Errorf("MaxMissingDays = %d, want 3", loaded.MaxMissingDays)
	}
	if loaded.BlockOnCritical {
		t.Error("BlockOnCritical should be false")
	}
}

func TestQualityStore_GateDecision_Block(t *testing.T) {
	store := setupQualityStore(t)
	now := time.Now().Truncate(time.Second)

	report := &QualityReport{
		ID:        "qr-block",
		StockCode: "600000",
		Source:    "kline",
		AsOf:      now,
		Passed:    false,
		Blocked:   true,
		Summary: QualitySummary{
			CriticalCount: 2,
		},
		Issues: []QualityIssue{
			{Severity: SeverityCritical, Description: "Price data corrupted"},
			{Severity: SeverityCritical, Description: "Duplicate records found"},
		},
		CreatedAt: now,
	}
	store.SaveReport(report)

	decision, err := store.GateDecision("qr-block")
	if err != nil {
		t.Fatalf("GateDecision: %v", err)
	}

	if decision.Decision != "block" {
		t.Errorf("Decision = %s, want block", decision.Decision)
	}
	if !decision.Blocked {
		t.Error("Should be blocked")
	}
	if decision.Passed {
		t.Error("Should not pass")
	}
}

func TestQualityStore_GateDecision_Warn(t *testing.T) {
	store := setupQualityStore(t)
	now := time.Now().Truncate(time.Second)

	// 只有 warning, 应通过但附带警告
	report := &QualityReport{
		ID:        "qr-warn",
		StockCode: "600000",
		Source:    "kline",
		AsOf:      now,
		Passed:    true,
		Blocked:   false,
		Summary: QualitySummary{
			WarningCount: 1,
		},
		Issues: []QualityIssue{
			{Severity: SeverityWarning, Description: "Minor volume anomaly"},
		},
		CreatedAt: now,
	}
	store.SaveReport(report)

	decision, err := store.GateDecision("qr-warn")
	if err != nil {
		t.Fatalf("GateDecision: %v", err)
	}

	if decision.Decision != "warn" {
		t.Errorf("Decision = %s, want warn", decision.Decision)
	}
	if decision.Blocked {
		t.Error("Should not be blocked with only warnings")
	}
	if !decision.Passed {
		t.Error("Should pass with warnings")
	}
}

func TestQualityStore_GateDecision_Pass(t *testing.T) {
	store := setupQualityStore(t)
	now := time.Now().Truncate(time.Second)

	report := &QualityReport{
		ID:        "qr-pass",
		StockCode: "600000",
		Source:    "kline",
		AsOf:      now,
		Passed:    true,
		Blocked:   false,
		Summary:   QualitySummary{},
		Issues:    nil,
		CreatedAt: now,
	}
	store.SaveReport(report)

	decision, err := store.GateDecision("qr-pass")
	if err != nil {
		t.Fatalf("GateDecision: %v", err)
	}

	if decision.Decision != "pass" {
		t.Errorf("Decision = %s, want pass", decision.Decision)
	}
	if decision.Blocked {
		t.Error("Should not be blocked")
	}
}

func TestQualityStore_ListIssues(t *testing.T) {
	store := setupQualityStore(t)
	now := time.Now().Truncate(time.Second)

	report := &QualityReport{
		ID:        "qr-list",
		StockCode: "600000",
		Source:    "kline",
		AsOf:      now,
		Passed:    false,
		Blocked:   true,
		Summary: QualitySummary{
			CriticalCount: 1,
			WarningCount:  1,
			InfoCount:     1,
		},
		Issues: []QualityIssue{
			{ID: "i1", Severity: SeverityCritical, Category: CategoryDuplicate, Description: "Dup"},
			{ID: "i2", Severity: SeverityWarning, Category: CategoryAbnormalVolume, Description: "Vol"},
			{ID: "i3", Severity: SeverityInfo, Category: CategoryPoolCoverage, Description: "Cov"},
		},
		CreatedAt: now,
	}
	store.SaveReport(report)

	// 全部
	all, err := store.ListIssues("qr-list", "")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("All issues = %d, want 3", len(all))
	}

	// 仅 critical
	critical, err := store.ListIssues("qr-list", SeverityCritical)
	if err != nil {
		t.Fatalf("ListIssues (critical): %v", err)
	}
	if len(critical) != 1 {
		t.Errorf("Critical issues = %d, want 1", len(critical))
	}
	if critical[0].Severity != SeverityCritical {
		t.Error("Should be critical")
	}
}

func TestQualityStore_Counts(t *testing.T) {
	store := setupQualityStore(t)

	cnt, _ := store.CountReports()
	if cnt != 0 {
		t.Errorf("Initial reports = %d, want 0", cnt)
	}

	now := time.Now().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		store.SaveReport(&QualityReport{
			ID: fmt.Sprintf("qr-%d", i), Source: "kline", AsOf: now,
			Summary: QualitySummary{}, CreatedAt: now,
		})
	}

	cnt, _ = store.CountReports()
	if cnt != 3 {
		t.Errorf("Reports = %d, want 3", cnt)
	}

	critCnt, _ := store.CountCriticalIssues()
	if critCnt != 0 {
		t.Errorf("Critical issues = %d, want 0", critCnt)
	}
}

// ==================== 集成测试: 完整工作流 ====================

func TestQualityGate_IntegrationWorkflow(t *testing.T) {
	// Step 1: 创建数据
	dates := makeDates(10)
	closes := make([]float64, 10)
	for i := range closes {
		closes[i] = 10.0 + float64(i*10) // 10, 20, 30, ... 100
	}
	records := makeRecords(dates, closes)
	expectedDays := makeExpectedDays(10)
	asOf := dates[len(dates)-1].AddDate(0, 0, 1)

	// Step 2: 质量检查 (正常数据)
	checker := NewQualityChecker(DefaultQualityGateConfig())
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	if !report.Passed {
		t.Errorf("Clean data should pass: %d issues found", report.Summary.TotalIssues)
	}
	if report.HasCriticalIssues() {
		t.Error("Clean data should have no critical issues")
	}

	// Step 3: 异常数据 - 制造严重问题
	badRecords := makeRecords(dates, closes)
	badRecords[3].Close = 0 // 无效价格
	badRecords[3].High = 0
	badRecords[3].Low = 0
	badRecords[3].Open = 0

	badReport := checker.CheckKline("600000", badRecords, expectedDays, asOf)

	if !badReport.HasCriticalIssues() {
		t.Error("Should detect invalid price as critical")
	}

	// Step 4: 持久化 + 质量门
	store := setupQualityStore(t)
	badReport.ID = "qr-integration-bad"
	store.SaveReport(badReport)

	decision, err := store.GateDecision("qr-integration-bad")
	if err != nil {
		t.Fatalf("GateDecision: %v", err)
	}

	if decision.Decision != "block" {
		t.Errorf("Should block on invalid price, got %s", decision.Decision)
	}
	if !decision.Blocked {
		t.Error("Should be blocked")
	}

	// Step 5: 验证可复现
	loaded, err := store.GetReport("qr-integration-bad")
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if loaded.Summary.CriticalCount != badReport.Summary.CriticalCount {
		t.Errorf("Critical count mismatch: loaded=%d vs original=%d",
			loaded.Summary.CriticalCount, badReport.Summary.CriticalCount)
	}

	// Step 6: 验证哈希稳定性
	if loaded.ComputeHash() != badReport.ComputeHash() {
		t.Error("Hash should be consistent after round-trip")
	}
}

func TestQualityGate_BlockOnConfig(t *testing.T) {
	// 测试不同配置下的阻断行为
	report := &QualityReport{
		ID: "qr-config-test",
		Issues: []QualityIssue{
			{Severity: SeverityCritical, Description: "Critical issue"},
		},
	}

	// 默认: 有 critical -> 阻断
	cfg := DefaultQualityGateConfig()
	if !report.ShouldBlock(cfg) {
		t.Error("Default config should block on critical")
	}

	// BlockOnCritical = false: 不阻断
	cfg.BlockOnCritical = false
	if report.ShouldBlock(cfg) {
		t.Error("Should not block when BlockOnCritical is false")
	}
}

// 边界测试: NaN/Inf 处理
func TestQualityChecker_NaNPrice(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	dates := makeDates(3)
	closes := []float64{10, 11, 12}
	records := makeRecords(dates, closes)
	records[1].Close = math.NaN()
	records[1].High = math.NaN()
	records[1].Low = math.NaN()
	records[1].Open = math.NaN()

	expectedDays := makeExpectedDays(3)
	asOf := dates[len(dates)-1].AddDate(0, 0, 1)
	report := checker.CheckKline("600000", records, expectedDays, asOf)

	if !report.HasCriticalIssues() {
		t.Error("NaN price should be critical")
	}
}

func TestQualityChecker_EmptyRecords(t *testing.T) {
	cfg := DefaultQualityGateConfig()
	checker := NewQualityChecker(cfg)

	report := checker.CheckKline("600000", nil, nil, time.Now())

	if report.Summary.TotalIssues != 0 {
		t.Error("Empty records should produce no issues")
	}
	if report.Summary.CheckedRecords != 0 {
		t.Error("CheckedRecords should be 0")
	}
}
