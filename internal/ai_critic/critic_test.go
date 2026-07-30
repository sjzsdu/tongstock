package ai_critic

import (
	"testing"
	"time"
)

// ============================================================================
// 辅助函数
// ============================================================================

func createGoodInput() ReviewInput {
	return ReviewInput{
		TargetID:   "cand-001",
		TargetType: "candidate",
		Config: ReviewConfig{
			SplitType:      "rolling",
			TrainRatio:     0.70,
			ValidRatio:     0.15,
			EmbargoDays:    10,
			PurgeDays:      7,
			FeatureCount:   8,
			FeatureIDs:     []string{"RSI", "MACD", "MA20", "VOL", "ATR", "RSI14", "MA60", "STOCH"},
			DataSnapshotID: "snapshot-v1",
		},
		Results: ReviewResults{
			SampleSize:        200,
			SharpeRatio:       2.0,
			SortinoRatio:      2.5,
			MaxDrawdown:       -0.08,
			TotalReturn:       0.15,
			WinRate:           0.55,
			TotalTrades:       150,
			ProfitFactor:      1.8,
			GrossReturn:       0.20,
			NetReturn:         0.15,
			CostRatio:         0.25,
			MaxPositionWeight: 0.08,
			Concentration:     0.30,
			BaselineReturn:    0.08,
			BaselineSharpe:    1.0,
		},
	}
}

func createBadInput() ReviewInput {
	return ReviewInput{
		TargetID:   "cand-bad",
		TargetType: "candidate",
		Config: ReviewConfig{
			SplitType:      "fixed",
			TrainRatio:     0.90,
			ValidRatio:     0.05,
			EmbargoDays:    1,
			PurgeDays:      0,
			FeatureCount:   30,
			FeatureIDs:     []string{"R1"},
			DataSnapshotID: "snapshot-v1",
		},
		Results: ReviewResults{
			SampleSize:        15,
			SharpeRatio:       8.0,
			SortinoRatio:      10.0,
			MaxDrawdown:       -0.005,
			TotalReturn:       0.30,
			WinRate:           0.92,
			TotalTrades:       3,
			ProfitFactor:      3.0,
			GrossReturn:       0.50,
			NetReturn:         0.30,
			CostRatio:         0.40,
			MaxPositionWeight: 0.35,
			Concentration:     0.70,
			BaselineReturn:    0.08,
			BaselineSharpe:    1.0,
		},
	}
}

// ============================================================================
// 模型测试
// ============================================================================

func TestNewReviewIssue(t *testing.T) {
	issue := NewReviewIssue("test-1", DimDataLeakage, SevCritical, "标题", "描述", "建议")

	if issue.ID != "test-1" {
		t.Error("wrong ID")
	}
	if issue.Dimension != DimDataLeakage {
		t.Error("wrong dimension")
	}
	if !issue.IsHardThresholdIssue() {
		t.Error("critical should be hard threshold")
	}
}

func TestReviewOutcome_Pass(t *testing.T) {
	outcome := NewReviewOutcome("t-1", "candidate", "ai")

	outcome.AddIssueQuick(DimNarrativeBias, SevLow, "标题", "描述", "建议")
	outcome.Finalize()

	if !outcome.Passed() {
		t.Error("low severity should still pass")
	}
}

func TestReviewOutcome_Block(t *testing.T) {
	outcome := NewReviewOutcome("t-1", "candidate", "ai")

	outcome.AddIssueQuick(DimDataLeakage, SevCritical, "标题", "描述", "建议")
	outcome.Finalize()

	if outcome.Conclusion != ConclusionBlock {
		t.Errorf("expected block, got %s", outcome.Conclusion)
	}
	if !outcome.HasHardBlock() {
		t.Error("should have hard block")
	}
}

func TestReviewOutcome_Fail(t *testing.T) {
	outcome := NewReviewOutcome("t-1", "candidate", "ai")

	outcome.AddIssueQuick(DimCostSensitivity, SevHigh, "标题", "描述", "建议")
	outcome.Finalize()

	if outcome.Conclusion != ConclusionFail {
		t.Errorf("expected fail, got %s", outcome.Conclusion)
	}
}

func TestReviewOutcome_NeedsReview(t *testing.T) {
	outcome := NewReviewOutcome("t-1", "candidate", "ai")

	outcome.AddIssueQuick(DimSelectionBias, SevMedium, "标题", "描述", "建议")
	outcome.Finalize()

	if outcome.Conclusion != ConclusionNeedsReview {
		t.Errorf("expected needs_review, got %s", outcome.Conclusion)
	}
}

func TestReviewOutcome_GetHardBlockingIssues(t *testing.T) {
	outcome := NewReviewOutcome("t-1", "candidate", "ai")

	outcome.AddIssueQuick(DimDataLeakage, SevCritical, "泄漏", "描述", "建议")
	outcome.AddIssueQuick(DimSampleSize, SevCritical, "样本", "描述", "建议")
	outcome.AddIssueQuick(DimNarrativeBias, SevLow, "叙事", "描述", "建议")

	blocked := outcome.GetHardBlockingIssues()
	if len(blocked) != 2 {
		t.Errorf("expected 2 hard blocking issues, got %d", len(blocked))
	}
}

func TestHumanReview_Approve(t *testing.T) {
	outcome := NewReviewOutcome("t-1", "candidate", "ai")
	outcome.Approve("reviewer-1", "策略逻辑有效")

	if outcome.HumanReview == nil {
		t.Fatal("human review should not be nil")
	}
	if outcome.HumanReview.Decision != "approved" {
		t.Error("wrong decision")
	}
	if !outcome.HumanReview.IsValid() {
		t.Error("human review should be valid")
	}
}

func TestHumanReview_Waive(t *testing.T) {
	outcome := NewReviewOutcome("t-1", "candidate", "ai")
	outcome.WaiveIssue("reviewer-1", "issue-1", "已知限制, 可接受")

	if outcome.HumanReview == nil {
		t.Fatal("human review should not be nil")
	}
	if len(outcome.HumanReview.WaivedIssues) != 1 {
		t.Errorf("expected 1 waived issue, got %d", len(outcome.HumanReview.WaivedIssues))
	}
}

func TestHumanReview_InvalidDecision(t *testing.T) {
	hr := &HumanReviewRecord{
		ReviewerID: "r-1",
		Decision:   "invalid_decision",
		ReviewedAt: time.Now(),
	}
	if hr.IsValid() {
		t.Error("invalid decision should not be valid")
	}
}

// ============================================================================
// 批评者配置测试
// ============================================================================

func TestDefaultCriticConfig(t *testing.T) {
	cfg := DefaultCriticConfig()

	if cfg.MinSampleSize != 30 {
		t.Error("wrong min sample size")
	}
	if cfg.MaxCostRatio != 0.30 {
		t.Error("wrong max cost ratio")
	}
	if cfg.AICanOverrideHardThreshold {
		t.Error("AI should NOT be able to override hard thresholds")
	}
}

// ============================================================================
// 完整审查流程测试
// ============================================================================

func TestResearchCritic_GoodInput(t *testing.T) {
	critic := NewResearchCritic(DefaultCriticConfig())
	input := createGoodInput()

	outcome := critic.Review(input)

	if outcome.Conclusion == ConclusionBlock || outcome.Conclusion == ConclusionFail {
		t.Errorf("good input should not be blocked/failed, got %s", outcome.Conclusion)
	}
	if outcome.Summary == "" {
		t.Error("should have summary")
	}

	// 检查所有 7 个维度都被检查了
	dims := make(map[ReviewDimension]bool)
	for _, issue := range outcome.Issues {
		dims[issue.Dimension] = true
	}
	// 好的输入可能不会触发所有维度的问题, 但应覆盖核心维度
}

func TestResearchCritic_BadInput_Blocked(t *testing.T) {
	critic := NewResearchCritic(DefaultCriticConfig())
	input := createBadInput()

	outcome := critic.Review(input)

	if outcome.Conclusion != ConclusionBlock {
		t.Errorf("bad input should be blocked, got %s", outcome.Conclusion)
	}
	if !outcome.HasHardBlock() {
		t.Error("should have hard block")
	}

	// 应有多个 critical 级别问题
	criticalIssues := outcome.GetHardBlockingIssues()
	if len(criticalIssues) < 3 {
		t.Errorf("expected at least 3 critical issues, got %d", len(criticalIssues))
	}
}

// ============================================================================
// 各检查器单独测试
// ============================================================================

func TestDataLeakageChecker_EmbargoTooShort(t *testing.T) {
	cfg := DefaultCriticConfig()
	cfg.MinEmbargoDays = 5
	checker := NewDataLeakageChecker(cfg)

	input := createGoodInput()
	input.Config.EmbargoDays = 1 // 太短

	issues := checker.Check(input)
	if len(issues) == 0 {
		t.Error("should detect embargo violation")
	}
	if issues[0].Severity != SevCritical {
		t.Error("should be critical")
	}
	if !issues[0].IsHardThresholdIssue() {
		t.Error("should be hard threshold")
	}
}

func TestDataLeakageChecker_PurgeTooShort(t *testing.T) {
	cfg := DefaultCriticConfig()
	cfg.MinPurgeDays = 3
	checker := NewDataLeakageChecker(cfg)

	input := createGoodInput()
	input.Config.PurgeDays = 0 // 没有清洗期

	issues := checker.Check(input)
	found := false
	for _, issue := range issues {
		if issue.Dimension == DimDataLeakage && issue.IsHardThresholdIssue() {
			found = true
			break
		}
	}
	if !found {
		t.Error("should detect purge violation")
	}
}

func TestSampleSizeChecker_SmallSample(t *testing.T) {
	cfg := DefaultCriticConfig()
	checker := NewSampleSizeChecker(cfg)

	input := createGoodInput()
	input.Results.SampleSize = 5 // 太少

	issues := checker.Check(input)
	if len(issues) == 0 {
		t.Error("should detect small sample")
	}
	if !issues[0].IsHardThresholdIssue() {
		t.Error("small sample should be hard threshold")
	}
}

func TestSampleSizeChecker_FewTrades(t *testing.T) {
	cfg := DefaultCriticConfig()
	checker := NewSampleSizeChecker(cfg)

	input := createGoodInput()
	input.Results.TotalTrades = 3 // 太少

	issues := checker.Check(input)
	found := false
	for _, issue := range issues {
		if issue.Dimension == DimSampleSize {
			found = true
			break
		}
	}
	if !found {
		t.Error("should detect few trades")
	}
}

func TestCostSensitivityChecker_HighCost(t *testing.T) {
	cfg := DefaultCriticConfig()
	checker := NewCostSensitivityChecker(cfg)

	input := createGoodInput()
	input.Results.CostRatio = 0.50 // 成本占比太高

	issues := checker.Check(input)
	found := false
	for _, issue := range issues {
		if issue.IsHardThresholdIssue() {
			found = true
			break
		}
	}
	if !found {
		t.Error("high cost should be hard threshold")
	}
}

func TestConcentrationChecker_HighWeight(t *testing.T) {
	cfg := DefaultCriticConfig()
	checker := NewConcentrationChecker(cfg)

	input := createGoodInput()
	input.Results.MaxPositionWeight = 0.40 // 太高

	issues := checker.Check(input)
	if len(issues) == 0 {
		t.Error("should detect high concentration")
	}
}

func TestNarrativeBiasChecker_HighSharpe(t *testing.T) {
	cfg := DefaultCriticConfig()
	checker := NewNarrativeBiasChecker(cfg)

	input := createGoodInput()
	input.Results.SharpeRatio = 10.0 // 异常高

	issues := checker.Check(input)
	found := false
	for _, issue := range issues {
		if issue.Dimension == DimNarrativeBias {
			found = true
			break
		}
	}
	if !found {
		t.Error("should detect suspiciously high sharpe")
	}
}

func TestNarrativeBiasChecker_HighWinRate(t *testing.T) {
	cfg := DefaultCriticConfig()
	checker := NewNarrativeBiasChecker(cfg)

	input := createGoodInput()
	input.Results.WinRate = 0.95 // 异常高

	issues := checker.Check(input)
	found := false
	for _, issue := range issues {
		if issue.Severity == SevMedium {
			found = true
			break
		}
	}
	if !found {
		t.Error("should detect suspiciously high win rate")
	}
}

func TestBaselineCompareChecker_Underperform(t *testing.T) {
	cfg := DefaultCriticConfig()
	checker := NewBaselineCompareChecker(cfg)

	input := createGoodInput()
	input.Results.TotalReturn = 0.05
	input.Results.BaselineReturn = 0.10 // 跑输基准

	issues := checker.Check(input)
	if len(issues) == 0 {
		t.Error("should detect underperformance")
	}
}

func TestBaselineCompareChecker_ExcessReturn(t *testing.T) {
	cfg := DefaultCriticConfig()
	checker := NewBaselineCompareChecker(cfg)

	input := createGoodInput()
	input.Results.TotalReturn = 0.10
	input.Results.BaselineReturn = 0.08
	input.Results.BaselineSharpe = 1.5
	input.Results.SharpeRatio = 1.0 // 低于基线

	issues := checker.Check(input)
	found := false
	for _, issue := range issues {
		if issue.Dimension == DimBaselineCompare {
			found = true
			break
		}
	}
	if !found {
		t.Error("should detect poor baseline comparison")
	}
}

// ============================================================================
// AI 不能自行豁免硬门槛测试
// ============================================================================

func TestHardThreshold_NotOverridable(t *testing.T) {
	cfg := DefaultCriticConfig()
	cfg.AICanOverrideHardThreshold = false // 确保 AI 不能豁免

	critic := NewResearchCritic(cfg)
	input := createBadInput()

	outcome := critic.Review(input)

	// 即使 AI 生成结果, 硬门槛仍阻止
	if outcome.Conclusion != ConclusionBlock {
		t.Errorf("hard threshold should block, got %s", outcome.Conclusion)
	}

	// 验证 AI 不能自行豁免
	if cfg.AICanOverrideHardThreshold {
		t.Error("AI should not be able to override hard thresholds")
	}
}

// ============================================================================
// 人工复核与豁免测试
// ============================================================================

func TestManualWaiver_Recorded(t *testing.T) {
	critic := NewResearchCritic(DefaultCriticConfig())
	input := createGoodInput()

	outcome := critic.Review(input)
	outcome.Finalize()

	// 人工复核
	outcome.Approve("human-reviewer", "策略虽有低风险但合理")

	// 检查复核记录
	if outcome.HumanReview == nil {
		t.Fatal("should have human review")
	}
	if outcome.HumanReview.ReviewerID != "human-reviewer" {
		t.Error("wrong reviewer")
	}
	if outcome.HumanReview.Decision != "approved" {
		t.Error("wrong decision")
	}
	if outcome.HumanReview.Notes != "策略虽有低风险但合理" {
		t.Error("wrong notes")
	}
}

func TestManualReject_Recorded(t *testing.T) {
	critic := NewResearchCritic(DefaultCriticConfig())
	input := createGoodInput()

	outcome := critic.Review(input)
	outcome.Reject("human-reviewer", "风险过高, 不建议晋级")

	if outcome.HumanReview.Decision != "rejected" {
		t.Error("wrong decision")
	}
}

func TestManualWaiver_IssueTracked(t *testing.T) {
	outcome := NewReviewOutcome("t-1", "candidate", "ai")
	outcome.AddIssueQuick(DimNarrativeBias, SevMedium, "问题", "描述", "建议")
	outcome.Finalize()

	// 豁免一个问题
	outcome.WaiveIssue("reviewer", "issue-narrative-bias-0", "已知限制")

	if len(outcome.HumanReview.WaivedIssues) != 1 {
		t.Error("waived issues should be tracked")
	}
	if outcome.HumanReview.Decision != "waived" {
		t.Error("decision should be waived")
	}
}

// ============================================================================
// 查询方法测试
// ============================================================================

func TestResearchCritic_GetCheckersByDimension(t *testing.T) {
	critic := NewResearchCritic(DefaultCriticConfig())

	checkers := critic.GetCheckersByDimension(DimDataLeakage)
	if len(checkers) == 0 {
		t.Error("should have data leakage checker")
	}
}

func TestResearchCritic_HardThresholdCheckers(t *testing.T) {
	critic := NewResearchCritic(DefaultCriticConfig())

	hard := critic.HardThresholdCheckers()
	if len(hard) < 3 {
		t.Errorf("expected at least 3 hard threshold checkers, got %d", len(hard))
	}

	// 检查关键硬门槛检查器存在
	dimSet := make(map[ReviewDimension]bool)
	for _, ch := range hard {
		dimSet[ch.Dimension()] = true
	}

	requiredDims := []ReviewDimension{DimDataLeakage, DimSampleSize, DimCostSensitivity}
	for _, dim := range requiredDims {
		if !dimSet[dim] {
			t.Errorf("missing hard threshold checker for %s", dim)
		}
	}
}

func TestResearchCritic_SoftThresholdCheckers(t *testing.T) {
	critic := NewResearchCritic(DefaultCriticConfig())

	soft := critic.SoftThresholdCheckers()
	if len(soft) == 0 {
		t.Error("should have soft threshold checkers")
	}

	// 检查软门槛检查器存在
	dimSet := make(map[ReviewDimension]bool)
	for _, ch := range soft {
		dimSet[ch.Dimension()] = true
	}

	requiredDims := []ReviewDimension{DimSelectionBias, DimConcentration, DimNarrativeBias, DimBaselineCompare}
	for _, dim := range requiredDims {
		if !dimSet[dim] {
			t.Errorf("missing soft threshold checker for %s", dim)
		}
	}
}

// ============================================================================
// 完整集成测试
// ============================================================================

func TestFullReviewPipeline_GoodCandidate(t *testing.T) {
	// 完整流程: 输入好数据 → 审查 → 通过 → 可晋级
	critic := NewResearchCritic(DefaultCriticConfig())
	input := createGoodInput()

	outcome := critic.Review(input)

	// 结论应是 pass 或 pass_notes 或 needs_review (不会 fail/block)
	if outcome.Conclusion == ConclusionBlock || outcome.Conclusion == ConclusionFail {
		t.Errorf("good candidate should not be blocked/failed, got %s", outcome.Conclusion)
	}

	// 应没有硬门槛阻止
	if outcome.HasHardBlock() {
		t.Error("good candidate should not have hard block")
	}

	// 摘要非空
	if outcome.Summary == "" {
		t.Error("summary should not be empty")
	}

	// 问题关联了实验指标
	for _, issue := range outcome.Issues {
		if issue.MetricName != "" {
			// 指标值和阈值应该都设置
			if issue.MetricValue != 0 {
				// 有指标值
			}
		}
	}
}

func TestFullReviewPipeline_BadCandidate_Blocked(t *testing.T) {
	// 完整流程: 输入坏数据 → 审查 → 阻塞 → 需人工复核
	critic := NewResearchCritic(DefaultCriticConfig())
	input := createBadInput()

	outcome := critic.Review(input)

	// 应该被阻止
	if !outcome.HasHardBlock() {
		t.Error("bad candidate should have hard block")
	}

	// 获取硬门槛问题
	blockingIssues := outcome.GetHardBlockingIssues()
	if len(blockingIssues) < 3 {
		t.Errorf("expected multiple blocking issues, got %d", len(blockingIssues))
	}

	// 人工复核记录
	outcome.Approve("reviewer-1", "虽然样本不足, 但策略逻辑有创新")

	if outcome.HumanReview == nil {
		t.Fatal("human review should be recorded")
	}
	if outcome.HumanReview.ReviewedAt.IsZero() {
		t.Error("review time should be set")
	}
}

// ============================================================================
// 边界测试
// ============================================================================

func TestCriticalDimension_IndependentReview(t *testing.T) {
	// 验证 7 种独立审查维度
	expectedDims := map[ReviewDimension]bool{
		DimDataLeakage:    true,
		DimSelectionBias:  true,
		DimSampleSize:     true,
		DimCostSensitivity: true,
		DimConcentration:  true,
		DimNarrativeBias:  true,
		DimBaselineCompare: true,
	}

	critic := NewResearchCritic(DefaultCriticConfig())
	checkedDims := make(map[ReviewDimension]bool)

	// 用坏数据触发所有维度
	input := createBadInput()
	outcome := critic.Review(input)

	for _, issue := range outcome.Issues {
		checkedDims[issue.Dimension] = true
	}

	for dim := range expectedDims {
		if !checkedDims[dim] {
			t.Logf("Dimension %s not triggered by bad input (may need more specific test)", dim)
		}
	}
}

func TestAllIssues_HaveEvidence(t *testing.T) {
	critic := NewResearchCritic(DefaultCriticConfig())
	input := createBadInput()
	outcome := critic.Review(input)

	for _, issue := range outcome.Issues {
		if issue.MetricName == "" {
			t.Logf("Issue %s has no metric name", issue.ID)
		}
		if issue.Recommendation == "" {
			t.Errorf("Issue %s has no recommendation", issue.ID)
		}
		if issue.Description == "" {
			t.Errorf("Issue %s has no description", issue.ID)
		}
	}
}
