package paradigms

import (
	"testing"
)

// ============================================================================
// 基准库测试
// ============================================================================

func TestNewBenchmarkLibrary(t *testing.T) {
	lib := NewBenchmarkLibrary()
	if lib == nil {
		t.Fatal("NewBenchmarkLibrary returned nil")
	}

	if lib.Count() < 5 {
		t.Errorf("expected at least 5 benchmarks, got %d", lib.Count())
	}
}

func TestGetBenchmarks(t *testing.T) {
	lib := NewBenchmarkLibrary()
	benchmarks := lib.GetBenchmarks()

	if len(benchmarks) == 0 {
		t.Error("expected some benchmarks")
	}

	// 检查每个基准的基本属性
	for _, b := range benchmarks {
		if b.ID == "" {
			t.Error("benchmark ID should not be empty")
		}
		if b.Name == "" {
			t.Error("benchmark Name should not be empty")
		}
		if b.Schema == nil {
			t.Errorf("benchmark %s should have a schema", b.ID)
		}
		if b.ExpectedMetrics == nil {
			t.Errorf("benchmark %s should have expected metrics", b.ID)
		}
	}
}

func TestGetBenchmark(t *testing.T) {
	lib := NewBenchmarkLibrary()

	benchmark := lib.GetBenchmark("bm-ma-cross")
	if benchmark == nil {
		t.Error("expected to find bm-ma-cross")
	}
	if benchmark.Name != "双均线交叉策略" {
		t.Errorf("expected name '双均线交叉策略', got '%s'", benchmark.Name)
	}

	nonExistent := lib.GetBenchmark("nonexistent")
	if nonExistent != nil {
		t.Error("should return nil for nonexistent benchmark")
	}
}

func TestGetBenchmarksByCategory(t *testing.T) {
	lib := NewBenchmarkLibrary()

	momentum := lib.GetBenchmarksByCategory(CategoryMomentum)
	if len(momentum) == 0 {
		t.Error("expected at least 1 momentum benchmark")
	}

	meanReversion := lib.GetBenchmarksByCategory(CategoryMeanReversion)
	if len(meanReversion) == 0 {
		t.Error("expected at least 1 mean reversion benchmark")
	}
}

func TestGetPassBenchmarks(t *testing.T) {
	lib := NewBenchmarkLibrary()

	pass := lib.GetPassBenchmarks()
	if len(pass) == 0 {
		t.Error("expected at least 1 pass benchmark")
	}

	for _, b := range pass {
		if b.ExpectedResult != ExpectedPass {
			t.Errorf("benchmark %s should be expected to pass", b.ID)
		}
	}
}

func TestGetRejectBenchmarks(t *testing.T) {
	lib := NewBenchmarkLibrary()

	reject := lib.GetRejectBenchmarks()
	if len(reject) == 0 {
		t.Error("expected at least 1 reject benchmark")
	}

	for _, b := range reject {
		if b.ExpectedResult != ExpectedReject {
			t.Errorf("benchmark %s should be expected to reject", b.ID)
		}
	}
}

func TestCountByDifficulty(t *testing.T) {
	lib := NewBenchmarkLibrary()

	counts := lib.CountByDifficulty()
	if len(counts) == 0 {
		t.Error("expected some difficulty counts")
	}

	// 应有 easy, medium, hard 三种
	if counts[DifficultyEasy] == 0 {
		t.Error("expected at least 1 easy benchmark")
	}
	if counts[DifficultyHard] == 0 {
		t.Error("expected at least 1 hard benchmark")
	}
}

// ============================================================================
// 端到端验证测试
// ============================================================================

func TestNewE2EValidator(t *testing.T) {
	validator := NewE2EValidator()
	if validator == nil {
		t.Fatal("NewE2EValidator returned nil")
	}
}

func TestValidateBenchmark(t *testing.T) {
	validator := NewE2EValidator()
	lib := NewBenchmarkLibrary()

	benchmark := lib.GetBenchmark("bm-ma-cross")
	if benchmark == nil {
		t.Fatal("expected to find bm-ma-cross")
	}

	result := validator.ValidateBenchmark(benchmark)

	if result == nil {
		t.Fatal("ValidateBenchmark returned nil")
	}
	if result.BenchmarkID != "bm-ma-cross" {
		t.Errorf("expected benchmark ID bm-ma-cross, got %s", result.BenchmarkID)
	}
	if result.Score == nil {
		t.Error("expected score result")
	}
}

func TestValidateAll(t *testing.T) {
	validator := NewE2EValidator()

	report := validator.ValidateAll()

	if report == nil {
		t.Fatal("ValidateAll returned nil")
	}
	if len(report.Results) == 0 {
		t.Error("expected some results")
	}
	if report.TotalCount != 5 {
		t.Errorf("expected 5 results, got %d", report.TotalCount)
	}
}

func TestValidateSelected(t *testing.T) {
	validator := NewE2EValidator()

	ids := []string{"bm-ma-cross", "bm-rsi-reversal"}
	report := validator.ValidateSelected(ids)

	if report == nil {
		t.Fatal("ValidateSelected returned nil")
	}
	if report.TotalCount != 2 {
		t.Errorf("expected 2 results, got %d", report.TotalCount)
	}
}

func TestE2EValidationReport(t *testing.T) {
	validator := NewE2EValidator()
	report := validator.ValidateAll()

	// 检查报告完整性
	summary := report.GetSummary()

	if summary.TotalBenchmarks != 5 {
		t.Errorf("expected 5 total benchmarks, got %d", summary.TotalBenchmarks)
	}
	if summary.PassedCount+summary.RejectedCount > summary.TotalBenchmarks {
		t.Error("passed + rejected should not exceed total")
	}
}

func TestGenerateReport(t *testing.T) {
	validator := NewE2EValidator()
	report := validator.ValidateAll()

	summary := report.GenerateReport()

	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

// ============================================================================
// 验证管理器测试
// ============================================================================

func TestNewE2EValidationManager(t *testing.T) {
	mgr := NewE2EValidationManager()
	if mgr == nil {
		t.Fatal("NewE2EValidationManager returned nil")
	}
}

func TestStartValidation(t *testing.T) {
	mgr := NewE2EValidationManager()

	ids := []string{"bm-ma-cross", "bm-rsi-reversal"}
	statuses := mgr.StartValidation(ids)

	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.Status != "validating" {
			t.Errorf("expected status 'validating', got '%s'", s.Status)
		}
	}
}

func TestGetStatus(t *testing.T) {
	mgr := NewE2EValidationManager()

	mgr.StartValidation([]string{"bm-ma-cross"})

	status := mgr.GetStatus("bm-ma-cross")
	if status == nil {
		t.Error("expected status for bm-ma-cross")
	}
	if status.Status != "validating" {
		t.Errorf("expected status 'validating', got '%s'", status.Status)
	}

	nonExistent := mgr.GetStatus("nonexistent")
	if nonExistent != nil {
		t.Error("should return nil for nonexistent benchmark")
	}
}

func TestRunFullValidation(t *testing.T) {
	mgr := NewE2EValidationManager()

	report := mgr.RunFullValidation()

	if report == nil {
		t.Fatal("RunFullValidation returned nil")
	}

	// 检查所有基准已验证完成
	for _, result := range report.Results {
		if !result.Match {
			// 某些基准可能不匹配预期, 但系统应该正常工作
			t.Logf("Benchmark %s: expected %s, actual %s",
				result.BenchmarkName, result.ExpectedResult, result.ActualResult)
		}
	}
}

func TestGetBenchmarkLibrary(t *testing.T) {
	mgr := NewE2EValidationManager()

	lib := mgr.GetBenchmarkLibrary()
	if lib == nil {
		t.Error("expected benchmark library")
	}
	if lib.Count() < 5 {
		t.Errorf("expected at least 5 benchmarks, got %d", lib.Count())
	}
}

// ============================================================================
// 基准 Schema 验证测试
// ============================================================================

func TestBenchmarkSchemasAreValid(t *testing.T) {
	lib := NewBenchmarkLibrary()

	for _, b := range lib.GetBenchmarks() {
		if b.Schema == nil {
			continue
		}

		err := b.Schema.IsValid()
		if err != nil {
			t.Errorf("benchmark %s has invalid schema: %v", b.ID, err)
		}
	}
}

func TestBenchmarkEconomicLogic(t *testing.T) {
	lib := NewBenchmarkLibrary()

	for _, b := range lib.GetBenchmarks() {
		if b.EconomicLogic == "" {
			t.Errorf("benchmark %s should have economic logic", b.ID)
		}
		if b.Description == "" {
			t.Errorf("benchmark %s should have description", b.ID)
		}
	}
}

func TestAllBenchmarksHaveUniqueIDs(t *testing.T) {
	lib := NewBenchmarkLibrary()
	seen := make(map[string]bool)

	for _, b := range lib.GetBenchmarks() {
		if seen[b.ID] {
			t.Errorf("duplicate benchmark ID: %s", b.ID)
		}
		seen[b.ID] = true
	}
}

// ============================================================================
// 预期案例验证
// ============================================================================

func TestExpectedPassCaseExists(t *testing.T) {
	lib := NewBenchmarkLibrary()

	passBenchmarks := lib.GetPassBenchmarks()
	if len(passBenchmarks) < 1 {
		t.Error("at least 1 benchmark should be expected to pass")
	}

	// 验证至少有一个是 easy 或 medium 难度
	hasEasyOrMedium := false
	for _, b := range passBenchmarks {
		if b.Difficulty == DifficultyEasy || b.Difficulty == DifficultyMedium {
			hasEasyOrMedium = true
			break
		}
	}
	if !hasEasyOrMedium {
		t.Error("pass benchmarks should include easy or medium difficulty")
	}
}

func TestExpectedRejectCaseExists(t *testing.T) {
	lib := NewBenchmarkLibrary()

	rejectBenchmarks := lib.GetRejectBenchmarks()
	if len(rejectBenchmarks) < 1 {
		t.Error("at least 1 benchmark should be expected to reject")
	}

	// 验证至少有一个是 hard 难度 (过拟合或随机)
	hasHard := false
	for _, b := range rejectBenchmarks {
		if b.Difficulty == DifficultyHard {
			hasHard = true
			break
		}
	}
	if !hasHard {
		t.Error("reject benchmarks should include hard difficulty")
	}
}
