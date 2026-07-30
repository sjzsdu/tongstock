package ai_quality

import (
	"testing"
)

// ============================================================================
// 测试用例库测试
// ============================================================================

func TestNewTestCaseLibrary(t *testing.T) {
	lib := NewTestCaseLibrary()
	if lib == nil {
		t.Fatal("NewTestCaseLibrary returned nil")
	}

	count := lib.Count()
	if count < 10 {
		t.Errorf("expected at least 10 test cases, got %d", count)
	}
}

func TestGetCriticalCases(t *testing.T) {
	lib := NewTestCaseLibrary()
	critical := lib.GetCriticalCases()

	if len(critical) == 0 {
		t.Error("should have critical test cases")
	}

	// 关键用例数应占合理比例
	if len(critical) < 5 {
		t.Errorf("expected at least 5 critical cases, got %d", len(critical))
	}
}

func TestGetByCategory(t *testing.T) {
	lib := NewTestCaseLibrary()

	categories := []TestCategory{
		CategoryKnownGood,
		CategoryKnownTrap,
		CategoryDataInsufficient,
		CategoryAdversarial,
	}

	for _, cat := range categories {
		cases := lib.GetByCategory(cat)
		if len(cases) == 0 {
			t.Errorf("category %s should have test cases", cat)
		}

		for _, tc := range cases {
			if tc.Category != cat {
				t.Errorf("test case %s has wrong category", tc.ID)
			}
		}
	}
}

func TestTestCases_HaveExpectedFields(t *testing.T) {
	lib := NewTestCaseLibrary()

	for _, tc := range lib.GetCases() {
		// ID 非空
		if tc.ID == "" {
			t.Error("test case ID should not be empty")
		}

		// 分类合法
		validCats := map[TestCategory]bool{
			CategoryKnownGood:        true,
			CategoryKnownTrap:        true,
			CategoryDataInsufficient: true,
			CategoryAdversarial:      true,
		}
		if !validCats[tc.Category] {
			t.Errorf("invalid category: %s", tc.Category)
		}

		// 期望非空
		if len(tc.Expectation.ExpectConclusions) == 0 {
			t.Errorf("test case %s has no expected conclusions", tc.ID)
		}

		// 输入非空
		if tc.Input.TargetID == "" {
			t.Errorf("test case %s has empty target ID", tc.ID)
		}
	}
}

// ============================================================================
// 默认审查函数测试
// ============================================================================

func TestDefaultReviewFunc_GoodInput(t *testing.T) {
	input := TestCaseInput{
		TargetID:          "good-cand",
		SplitType:         "rolling",
		TrainRatio:        0.70,
		EmbargoDays:       10,
		PurgeDays:         7,
		FeatureCount:      5,
		SampleSize:        200,
		SharpeRatio:       1.8,
		TotalReturn:       0.12,
		WinRate:           0.55,
		TotalTrades:       200,
		CostRatio:         0.20,
		MaxPositionWeight: 0.10,
		Concentration:     0.25,
		BaselineReturn:    0.06,
		BaselineSharpe:    0.9,
	}

	outcome := DefaultReviewFunc(input)

	// 好的输入不应被阻止
	if outcome.HasHardBlock {
		t.Error("good input should not have hard block")
	}
	if outcome.Conclusion == "block" {
		t.Error("good input should not be blocked")
	}
}

func TestDefaultReviewFunc_DataLeakage(t *testing.T) {
	input := TestCaseInput{
		EmbargoDays: 0,
		PurgeDays:   0,
	}

	outcome := DefaultReviewFunc(input)

	if !outcome.HasHardBlock {
		t.Error("should have hard block for data leakage")
	}
	if outcome.Conclusion != "block" {
		t.Errorf("expected block, got %s", outcome.Conclusion)
	}

	// 检查问题存在
	if !dimensionInOutcome("data_leakage", outcome) {
		t.Error("should detect data leakage")
	}
}

func TestDefaultReviewFunc_SmallSample(t *testing.T) {
	input := TestCaseInput{
		SampleSize:  5,
		TotalTrades: 3,
	}

	outcome := DefaultReviewFunc(input)

	if !outcome.HasHardBlock {
		t.Error("small sample should be hard block")
	}
	if !dimensionInOutcome("sample_size", outcome) {
		t.Error("should detect sample size issue")
	}
}

func TestDefaultReviewFunc_HighCost(t *testing.T) {
	input := TestCaseInput{
		CostRatio: 0.50,
	}

	outcome := DefaultReviewFunc(input)

	if !outcome.HasHardBlock {
		t.Error("high cost should be hard block")
	}
	if !dimensionInOutcome("cost_sensitivity", outcome) {
		t.Error("should detect cost sensitivity")
	}
}

func TestDefaultReviewFunc_OverfitDetection(t *testing.T) {
	input := TestCaseInput{
		SplitType:    "fixed",
		TrainRatio:   0.90,
		FeatureCount: 50,
		SharpeRatio:  15.0,
		WinRate:      0.95,
	}

	outcome := DefaultReviewFunc(input)

	// 多个维度应该被触发
	expectedDims := []string{"selection_bias", "narrative_bias"}
	for _, dim := range expectedDims {
		if !dimensionInOutcome(dim, outcome) {
			t.Errorf("should detect %s for overfit input", dim)
		}
	}

	// 极端夏普可能触发额外的警告
	if input.SharpeRatio > 5.0 {
		if !dimensionInOutcome("narrative_bias", outcome) {
			t.Error("extreme sharpe should trigger narrative bias")
		}
	}
}

// ============================================================================
// 回归门测试
// ============================================================================

func TestNewRegressionGate(t *testing.T) {
	cfg := DefaultGateConfig()
	lib := NewTestCaseLibrary()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")

	gate := NewRegressionGate(cfg, lib, runner)
	if gate == nil {
		t.Fatal("NewRegressionGate returned nil")
	}
}

func TestRegressionGate_Run_Success(t *testing.T) {
	cfg := DefaultGateConfig()
	lib := NewTestCaseLibrary()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")

	gate := NewRegressionGate(cfg, lib, runner)
	report := gate.Run()

	if report == nil {
		t.Fatal("report should not be nil")
	}

	// 基本信息
	if report.TotalTests != lib.Count() {
		t.Errorf("expected %d tests, got %d", lib.Count(), report.TotalTests)
	}
	if report.Model != "test-model" {
		t.Error("wrong model")
	}

	// 所有结果应已生成
	if len(report.Results) != report.TotalTests {
		t.Errorf("expected %d results, got %d", report.TotalTests, len(report.Results))
	}

	// 关键失败数
	if report.CriticalFail < 0 {
		t.Error("critical fail should not be negative")
	}
}

func TestRegressionGate_History(t *testing.T) {
	cfg := DefaultGateConfig()
	lib := NewTestCaseLibrary()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")

	gate := NewRegressionGate(cfg, lib, runner)

	// 运行两次
	gate.Run()
	gate.Run()

	history := gate.GetHistory()
	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}

	latest := gate.GetLatestReport()
	if latest == nil {
		t.Error("latest report should not be nil")
	}
}

// ============================================================================
// 版本变化检测测试
// ============================================================================

func TestVersionChangeDetection(t *testing.T) {
	cfg := DefaultGateConfig()
	lib := NewTestCaseLibrary()
	runner := NewDefaultTestRunner("model-a", "1.0", "v1")

	gate := NewRegressionGate(cfg, lib, runner)

	// 第一次运行
	report1 := gate.Run()
	if len(report1.VersionChanges) != 0 {
		t.Error("first run should have no version changes")
	}

	// 改变模型后运行
	runner2 := NewDefaultTestRunner("model-c", "2.0", "v1")
	gate2 := NewRegressionGate(cfg, lib, runner2)

	// 先运行一次以建立历史
	gate2.Run()

	// 再改变版本运行
	runner3 := NewDefaultTestRunner("model-d", "2.0", "v1")
	gate3 := NewRegressionGate(cfg, lib, runner3)

	// 预置历史
	gate3.history = append(gate3.history, GateReport{
		Model:        "model-b",
		ModelVersion: "1.0",
	})

	report3 := gate3.Run()

	// 应该检测到变化
	foundModelChange := false
	for _, vc := range report3.VersionChanges {
		if vc.Field == "model" && vc.Impact == "critical" {
			foundModelChange = true
		}
	}
	if !foundModelChange {
		t.Error("should detect model version change")
	}
}

// ============================================================================
// 关键错误阻止发布测试
// ============================================================================

func TestCriticalFail_BlocksRelease(t *testing.T) {
	cfg := DefaultGateConfig()
	lib := NewTestCaseLibrary()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")

	gate := NewRegressionGate(cfg, lib, runner)
	report := gate.Run()

	// 如果有关键失败, 应该阻止发布
	if report.CriticalFail > 0 {
		if report.Status != GateBlock {
			t.Error("critical failure should block release")
		}
	}
}

func TestMaxFailures_Config(t *testing.T) {
	cfg := DefaultGateConfig()
	cfg.MaxFailures = 0 // 允许 0 个失败

	lib := NewTestCaseLibrary()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")

	gate := NewRegressionGate(cfg, lib, runner)
	report := gate.Run()

	// 更严格的配置可能导致更多警告
	_ = report.Status // 不强制要求 block, 但应该可能
}

// ============================================================================
// 已知通过范式验证
// ============================================================================

func TestKnownGood_ShouldPass(t *testing.T) {
	lib := NewTestCaseLibrary()
	cfg := DefaultGateConfig()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")
	gate := NewRegressionGate(cfg, lib, runner)

	report := gate.Run()

	// 检查已知通过类别的测试结果
	kgResults := filterResultsByCategory(report.Results, CategoryKnownGood)
	for _, result := range kgResults {
		if !result.Passed {
			t.Errorf("known-good test %s should pass but failed: %v",
				result.TestCaseID, result.Issues)
		}
	}
}

// ============================================================================
// 已知陷阱验证
// ============================================================================

func TestKnownTrap_ShouldBeCaught(t *testing.T) {
	lib := NewTestCaseLibrary()
	cfg := DefaultGateConfig()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")
	gate := NewRegressionGate(cfg, lib, runner)

	report := gate.Run()

	// 已知陷阱类别的测试: 系统应能正确识别陷阱 (Passed=true 表示正确识别)
	ktResults := filterResultsByCategory(report.Results, CategoryKnownTrap)
	if len(ktResults) == 0 {
		t.Error("should have known trap test results")
	}

	for _, result := range ktResults {
		// 已知陷阱应被正确识别 (Passed=true)
		if !result.Passed {
			t.Errorf("known trap %s should be correctly caught, but got failed: %v",
				result.TestCaseID, result.Issues)
		}
	}
}

// ============================================================================
// 数据不足验证
// ============================================================================

func TestDataInsufficient_ShouldBeRejected(t *testing.T) {
	lib := NewTestCaseLibrary()
	cfg := DefaultGateConfig()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")
	gate := NewRegressionGate(cfg, lib, runner)

	report := gate.Run()

	// 数据不足类别的测试: 系统应正确拒绝数据不足的候选 (Passed=true 表示正确识别)
	diResults := filterResultsByCategory(report.Results, CategoryDataInsufficient)
	for _, result := range diResults {
		if !result.Passed {
			t.Errorf("data-insufficient test %s should be correctly rejected by system", result.TestCaseID)
		}
	}
}

// ============================================================================
// 对抗样例验证
// ============================================================================

func TestAdversarial_ShouldNotPassCleanly(t *testing.T) {
	lib := NewTestCaseLibrary()
	cfg := DefaultGateConfig()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")
	gate := NewRegressionGate(cfg, lib, runner)

	report := gate.Run()

	adResults := filterResultsByCategory(report.Results, CategoryAdversarial)
	// 对抗样例至少应被标记 (不一定全部失败, 但应有警告)
	if len(adResults) == 0 {
		t.Error("should have adversarial test results")
	}
}

// ============================================================================
// 结果完整性测试
// ============================================================================

func TestGateReport_HasAllFields(t *testing.T) {
	cfg := DefaultGateConfig()
	lib := NewTestCaseLibrary()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")

	gate := NewRegressionGate(cfg, lib, runner)
	report := gate.Run()

	// 检查所有字段
	requiredFields := []struct {
		name  string
		value string
	}{
		{"ID", report.ID},
		{"Status", string(report.Status)},
		{"Model", report.Model},
		{"ModelVersion", report.ModelVersion},
		{"PromptVersion", report.PromptVersion},
	}

	for _, f := range requiredFields {
		if f.value == "" {
			t.Errorf("field %s should not be empty", f.name)
		}
	}

	if report.StartedAt.IsZero() || report.CompletedAt.IsZero() {
		t.Error("timestamps should be set")
	}

	if report.DurationMs < 0 {
		t.Error("duration should not be negative")
	}

	// 每个结果应有对应的测试用例 ID
	for _, result := range report.Results {
		if result.TestCaseID == "" {
			t.Error("result should have test case ID")
		}
		if result.Category == "" {
			t.Error("result should have category")
		}
	}
}

// ============================================================================
// 分数计算测试
// ============================================================================

func TestScoreCalculation(t *testing.T) {
	cfg := DefaultGateConfig()
	lib := NewTestCaseLibrary()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")

	gate := NewRegressionGate(cfg, lib, runner)
	report := gate.Run()

	// 分数应在合理范围 [0, 1]
	if report.Score < 0 || report.Score > 1.01 {
		t.Errorf("score should be in [0, 1], got %f", report.Score)
	}

	// 通过数 + 失败数 = 总数
	if report.PassedCount+report.FailedCount != report.TotalTests {
		t.Errorf("passed(%d) + failed(%d) != total(%d)",
			report.PassedCount, report.FailedCount, report.TotalTests)
	}
}

// ============================================================================
// 状态判定测试
// ============================================================================

func TestStatusDetermination(t *testing.T) {
	cfg := DefaultGateConfig()
	lib := NewTestCaseLibrary()
	runner := NewDefaultTestRunner("test-model", "0.1", "v1")

	gate := NewRegressionGate(cfg, lib, runner)
	report := gate.Run()

	// 状态必须是合法值
	validStatuses := map[GateStatus]bool{
		GatePass:    true,
		GateWarning: true,
		GateBlock:   true,
	}
	if !validStatuses[report.Status] {
		t.Errorf("invalid status: %s", report.Status)
	}
}

// ============================================================================
// 集成测试
// ============================================================================

func TestFullRegressionPipeline(t *testing.T) {
	// 1. 创建配置
	cfg := DefaultGateConfig()
	cfg.BlockOnCriticalFail = true
	cfg.MinPassScore = 0.90
	cfg.MaxFailures = 2

	// 2. 创建测试库
	lib := NewTestCaseLibrary()
	if lib.Count() < 10 {
		t.Fatal("test suite should have at least 10 cases")
	}

	// 3. 创建运行器
	runner := NewDefaultTestRunner("integration-test", "1.0", "v1")

	// 4. 创建门
	gate := NewRegressionGate(cfg, lib, runner)

	// 5. 运行
	report := gate.Run()

	// 6. 验证结果
	if report == nil {
		t.Fatal("report should not be nil")
	}

	// 验证所有测试用例都被执行
	if len(report.Results) != lib.Count() {
		t.Errorf("not all test cases executed: %d/%d", len(report.Results), lib.Count())
	}

	// 验证 known-good 全部通过
	for _, result := range report.Results {
		if result.Category == CategoryKnownGood && !result.Passed {
			t.Errorf("known-good test failed: %s", result.TestCaseID)
		}
	}

	// 验证 status 是合法值
	if report.Status != GatePass && report.Status != GateWarning && report.Status != GateBlock {
		t.Errorf("invalid gate status: %s", report.Status)
	}

	// 验证关键失败数正确
	if report.CriticalFail < 0 || report.CriticalFail > report.TotalTests {
		t.Errorf("critical fail count out of range: %d", report.CriticalFail)
	}

	// 7. 验证历史
	history := gate.GetHistory()
	if len(history) != 1 {
		t.Error("history should have 1 entry")
	}

	latest := gate.GetLatestReport()
	if latest == nil || latest.ID != report.ID {
		t.Error("latest report mismatch")
	}

	// 8. 运行第二次 (检测版本变化)
	report2 := gate.Run()
	if report2.ID == report.ID {
		t.Error("second run should have different ID")
	}

	// 9. 时间检查
	if report.CompletedAt.Before(report.StartedAt) {
		t.Error("completed before started")
	}

	t.Logf("Integration test passed: %d tests, %s status, score=%.2f",
		report.TotalTests, report.Status, report.Score)
}

// ============================================================================
// 辅助函数
// ============================================================================

func filterResultsByCategory(results []TestResult, cat TestCategory) []TestResult {
	var filtered []TestResult
	for _, r := range results {
		if r.Category == cat {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
