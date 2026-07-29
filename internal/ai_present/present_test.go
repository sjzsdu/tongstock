package ai_present

import (
	"testing"
	"time"
)

// ============================================================================
// Claim 分类测试
// ============================================================================

func TestClaimKind_String(t *testing.T) {
	if KindFact.String() != "事实 (Fact)" {
		t.Error("unexpected string for KindFact")
	}
	if KindCalculated.String() != "计算 (Calculated)" {
		t.Error("unexpected string for KindCalculated")
	}
	if KindInferred.String() != "推断 (Inferred)" {
		t.Error("unexpected string for KindInferred")
	}
	if KindUnknown.String() != "未知 (Unknown)" {
		t.Error("unexpected string for KindUnknown")
	}
}

// ============================================================================
// UncertaintyExpression 验证测试
// ============================================================================

func TestUncertaintyExpression_Valid(t *testing.T) {
	u := UncertaintyExpression{
		ConfidenceLevel: 0.85,
		ConfidenceScale: "model_self",
	}
	if err := u.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUncertaintyExpression_Historical(t *testing.T) {
	winRate := 0.55
	u := UncertaintyExpression{
		ConfidenceLevel:   0.55,
		ConfidenceScale:   "historical",
		HistoricalWinRate: &winRate,
		HistoricalSamples: 100,
	}
	if err := u.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUncertaintyExpression_ConfidenceOutOfRange(t *testing.T) {
	u := UncertaintyExpression{
		ConfidenceLevel: 1.5,
		ConfidenceScale: "model_self",
	}
	if err := u.Validate(); err == nil {
		t.Error("expected error for confidence out of range")
	}
}

func TestUncertaintyExpression_HistoricalMissingRate(t *testing.T) {
	u := UncertaintyExpression{
		ConfidenceLevel: 0.6,
		ConfidenceScale: "historical",
		// missing HistoricalWinRate
	}
	if err := u.Validate(); err == nil {
		t.Error("expected error for historical missing win rate")
	}
}

func TestUncertaintyExpression_ModelSelfWithHistorical(t *testing.T) {
	// 混淆检测由规则引擎(AntiConfusionRules)处理
	// Validate 本身允许这种组合
	winRate := 0.55
	u := UncertaintyExpression{
		ConfidenceLevel:   0.85,
		ConfidenceScale:   "model_self",
		HistoricalWinRate: &winRate,
	}
	if err := u.Validate(); err != nil {
		t.Errorf("unexpected error — confusion detection deferred to rule engine: %v", err)
	}
}

func TestUncertaintyExpression_IsLow(t *testing.T) {
	u := UncertaintyExpression{ConfidenceLevel: 0.3}
	if !u.IsLowConfidence() {
		t.Error("expected low confidence")
	}

	u.HistoricalWinRate = nil
	u.ConfidenceLevel = 0.8
	if u.IsLowConfidence() {
		t.Error("should not be low")
	}
}

// ============================================================================
// AIClaim 测试
// ============================================================================

func TestNewClaim(t *testing.T) {
	claim := NewClaim("c-1", KindFact, "600000 收盘于 10.50 元")
	if claim.ID != "c-1" {
		t.Error("wrong ID")
	}
	if claim.Kind != KindFact {
		t.Error("wrong kind")
	}
	if claim.Level != LevelCertain {
		t.Errorf("expected certain, got %s", claim.Level)
	}
}

func TestAIClaim_Valid_Fact(t *testing.T) {
	claim := NewClaim("c-1", KindFact, "600000 收盘于 10.50 元")
	claim.AddCitation(ObjectRef{Type: "dataset_snapshot", ID: "s-1", Version: "v1"}, time.Now())
	if err := claim.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAIClaim_Valid_Inferred(t *testing.T) {
	claim := NewClaim("c-1", KindInferred, "此形态可能预示短期上涨")
	claim.AddCitation(ObjectRef{Type: "experiment", ID: "e-1", Version: "v1"}, time.Now())
	if err := claim.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAIClaim_MissingCitation(t *testing.T) {
	claim := NewClaim("c-1", KindFact, "600000 收盘于 10.50 元")
	if err := claim.Validate(); err == nil {
		t.Error("expected error: fact must have citation")
	}
}

func TestAIClaim_UnknownNoCitation(t *testing.T) {
	// Unknown 类型不需要引用
	claim := NewClaim("c-1", KindUnknown, "该指标数据缺失")
	if err := claim.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAIClaim_EmptyStatement(t *testing.T) {
	claim := NewClaim("c-1", KindFact, "")
	claim.AddCitation(ObjectRef{Type: "t", ID: "i", Version: "v"}, time.Now())
	if err := claim.Validate(); err == nil {
		t.Error("expected error for empty statement")
	}
}

// ============================================================================
// 时效性检查测试
// ============================================================================

func TestTimelinessPolicy_Fresh(t *testing.T) {
	policy := DefaultTimelinessPolicy()
	citation := EvidenceCitation{AsOf: time.Now()}
	status := policy.CheckTimeliness(citation)
	if status != TimelinessFresh {
		t.Errorf("expected fresh, got %s", status)
	}
}

func TestTimelinessPolicy_Stale(t *testing.T) {
	policy := DefaultTimelinessPolicy()
	citation := EvidenceCitation{AsOf: time.Now().Add(-2 * 24 * time.Hour)}
	status := policy.CheckTimeliness(citation)
	if status != TimelinessStale {
		t.Errorf("expected stale, got %s", status)
	}
}

func TestTimelinessPolicy_Expired(t *testing.T) {
	policy := DefaultTimelinessPolicy()
	citation := EvidenceCitation{AsOf: time.Now().Add(-30 * 24 * time.Hour)}
	status := policy.CheckTimeliness(citation)
	if status != TimelinessExpired {
		t.Errorf("expected expired, got %s", status)
	}
}

func TestApplyTimelinessPolicy_Downgrade(t *testing.T) {
	policy := DefaultTimelinessPolicy()
	report := NewReport("r-1", "agent-1", "sess-1")

	claim := NewClaim("c-1", KindFact, "收盘价")
	claim.Level = LevelHigh
	claim.AddCitation(ObjectRef{Type: "snapshot", ID: "s-1"}, time.Now().Add(-3*24*time.Hour))
	report.AddClaim(claim)

	downgrades := report.ApplyTimelinessPolicy(policy)
	if len(downgrades) == 0 {
		t.Error("expected downgrades")
	}
	if downgrades[0].Action != "downgrade" {
		t.Errorf("expected downgrade action, got %s", downgrades[0].Action)
	}
}

func TestApplyTimelinessPolicy_Reject(t *testing.T) {
	policy := DefaultTimelinessPolicy()
	report := NewReport("r-1", "agent-1", "sess-1")

	claim := NewClaim("c-1", KindFact, "收盘价")
	claim.AddCitation(ObjectRef{Type: "snapshot", ID: "s-1"}, time.Now().Add(-30*24*time.Hour))
	report.AddClaim(claim)

	downgrades := report.ApplyTimelinessPolicy(policy)
	if len(downgrades) == 0 {
		t.Error("expected downgrades")
	}
	if downgrades[0].Action != "reject" {
		t.Errorf("expected reject action, got %s", downgrades[0].Action)
	}
	if !claim.Invalidated {
		t.Error("claim should be invalidated")
	}
}

// ============================================================================
// 反 LLM-Confusion 规则测试
// ============================================================================

func TestRule_NoLLMConfidenceAsWinrate(t *testing.T) {
	rules := DefaultAntiConfusionRules()
	report := NewReport("r-1", "agent-1", "sess-1")

	claim := NewClaim("c-1", KindInferred, "此形态可能预示上涨")
	claim.AddCitation(ObjectRef{Type: "experiment", ID: "e-1"}, time.Now())
	// 关键: model_self 与 historical_win_rate 同时存在
	claim.Uncertainty.ConfidenceLevel = 0.85
	claim.Uncertainty.ConfidenceScale = "model_self"
	winRate := 0.55
	claim.Uncertainty.HistoricalWinRate = &winRate
	claim.Uncertainty.HistoricalSamples = 100

	report.AddClaim(claim)

	violations := report.ApplyRules(rules)
	found := false
	for _, v := range violations {
		if v.RuleID == "no_llm_confidence_as_winrate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected violation for no_llm_confidence_as_winrate")
	}
}

func TestRule_InferredNotCertain(t *testing.T) {
	rules := DefaultAntiConfusionRules()
	report := NewReport("r-1", "agent-1", "sess-1")

	claim := NewClaim("c-1", KindInferred, "可能上涨")
	claim.AddCitation(ObjectRef{Type: "paradigm_version", ID: "v-1"}, time.Now())
	claim.Level = LevelCertain // 推断不应为 certain

	report.AddClaim(claim)

	violations := report.ApplyRules(rules)
	found := false
	for _, v := range violations {
		if v.RuleID == "inferred_requires_low_confidence_marker" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected violation for inferred not certain")
	}
}

func TestRule_HistoricalWithoutSamples(t *testing.T) {
	rules := DefaultAntiConfusionRules()
	report := NewReport("r-1", "agent-1", "sess-1")

	claim := NewClaim("c-1", KindCalculated, "历史胜率分析")
	claim.AddCitation(ObjectRef{Type: "experiment", ID: "e-1"}, time.Now())
	claim.Uncertainty.ConfidenceScale = "historical"
	winRate := 0.55
	claim.Uncertainty.HistoricalWinRate = &winRate
	claim.Uncertainty.HistoricalSamples = 0 // 样本量为 0

	report.AddClaim(claim)

	violations := report.ApplyRules(rules)
	found := false
	for _, v := range violations {
		if v.RuleID == "historical_winrate_requires_samples" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected violation for historical without samples")
	}
}

func TestRule_ValidReport_NoViolations(t *testing.T) {
	rules := DefaultAntiConfusionRules()
	report := NewReport("r-1", "agent-1", "sess-1")

	// 一个合规的推断声明
	claim1 := NewClaim("c-1", KindInferred, "此形态可能预示上涨")
	claim1.AddCitation(ObjectRef{Type: "experiment", ID: "e-1"}, time.Now())
	claim1.Level = LevelModerate
	claim1.Uncertainty.ConfidenceLevel = 0.7
	claim1.Uncertainty.ConfidenceScale = "model_self"
	report.AddClaim(claim1)

	// 一个合规的事实声明
	claim2 := NewClaim("c-2", KindFact, "600000 收盘于 10.50 元")
	claim2.AddCitation(ObjectRef{Type: "dataset_snapshot", ID: "s-1"}, time.Now())
	claim2.Uncertainty.ConfidenceLevel = 1.0
	claim2.Uncertainty.ConfidenceScale = "historical"
	winRate := 1.0
	claim2.Uncertainty.HistoricalWinRate = &winRate
	claim2.Uncertainty.HistoricalSamples = 1
	report.AddClaim(claim2)

	violations := report.ApplyRules(rules)
	if len(violations) > 0 {
		t.Errorf("expected no violations, got %d", len(violations))
	}
}

// ============================================================================
// AIReport 测试
// ============================================================================

func TestAIReport_Validate(t *testing.T) {
	report := NewReport("r-1", "agent-1", "sess-1")
	claim := NewClaim("c-1", KindFact, "事实声明")
	claim.AddCitation(ObjectRef{Type: "t", ID: "i", Version: "v"}, time.Now())
	report.AddClaim(claim)

	if err := report.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAIReport_ValidateFail(t *testing.T) {
	report := NewReport("r-1", "agent-1", "sess-1")
	// 没有引用的事实声明
	report.AddClaim(NewClaim("c-1", KindFact, "事实声明"))

	if err := report.Validate(); err == nil {
		t.Error("expected error")
	}
}

func TestReportSummary(t *testing.T) {
	report := NewReport("r-1", "agent-1", "sess-1")

	c1 := NewClaim("c-1", KindFact, "事实")
	c1.AddCitation(ObjectRef{Type: "t", ID: "i"}, time.Now())
	report.AddClaim(c1)

	c2 := NewClaim("c-2", KindInferred, "推断")
	c2.AddCitation(ObjectRef{Type: "t", ID: "i"}, time.Now())
	c2.Uncertainty.ConfidenceScale = "model_self"
	report.AddClaim(c2)

	c3 := NewClaim("c-3", KindCalculated, "计算")
	c3.AddCitation(ObjectRef{Type: "t", ID: "i"}, time.Now())
	c3.Uncertainty.ConfidenceScale = "historical"
	winRate := 0.8
	c3.Uncertainty.HistoricalWinRate = &winRate
	c3.Uncertainty.HistoricalSamples = 50
	report.AddClaim(c3)

	summary := report.GenerateSummary()
	if summary.ClaimsCount != 3 {
		t.Errorf("expected 3 claims, got %d", summary.ClaimsCount)
	}
	if summary.HasHistoricalWinRate != true {
		t.Error("expected historical win rate flag")
	}
	if summary.HasModelSelf != true {
		t.Error("expected model_self flag")
	}
}

// ============================================================================
// 集成测试
// ============================================================================

func TestFullPipeline_ReportTimelinessAndRules(t *testing.T) {
	// 创建报告
	report := NewReport("r-1", "agent-1", "sess-1")

	// Claim 1: 新鲜的事实
	c1 := NewClaim("c-1", KindFact, "今日收盘")
	c1.AddCitation(ObjectRef{Type: "snapshot", ID: "s-fresh"}, time.Now())
	report.AddClaim(c1)

	// Claim 2: 过期的事实 (将被降级)
	c2 := NewClaim("c-2", KindFact, "历史收盘")
	c2.AddCitation(ObjectRef{Type: "snapshot", ID: "s-stale"}, time.Now().Add(-10*24*time.Hour))
	report.AddClaim(c2)

	// Claim 3: 合规的推断 (无混淆)
	c3 := NewClaim("c-3", KindInferred, "可能上涨")
	c3.AddCitation(ObjectRef{Type: "experiment", ID: "e-1"}, time.Now())
	c3.Level = LevelModerate
	c3.Uncertainty.ConfidenceLevel = 0.85
	c3.Uncertainty.ConfidenceScale = "model_self"
	// 注意: 不添加 HistoricalWinRate — 合规!
	report.AddClaim(c3)

	// 1. 先验证
	if err := report.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// 2. 时效降级
	policy := DefaultTimelinessPolicy()
	downgrades := report.ApplyTimelinessPolicy(policy)
	if len(downgrades) != 1 {
		t.Errorf("expected 1 downgrade, got %d", len(downgrades))
	}

	// 3. 规则检查
	rules := DefaultAntiConfusionRules()
	violations := report.ApplyRules(rules)
	// 合规报告不应有违规
	for _, v := range violations {
		if v.RuleID == "no_llm_confidence_as_winrate" {
			t.Errorf("unexpected confusion violation: %s", v.Message)
		}
	}

	// 4. 验证摘要
	summary := report.GenerateSummary()
	if summary.ClaimsCount != 3 {
		t.Errorf("expected 3 claims, got %d", summary.ClaimsCount)
	}

	// 5. 重新验证
	if err := report.Validate(); err != nil {
		t.Fatalf("re-validation failed: %v", err)
	}
}

// ============================================================================
// 反例测试: 验证规则能够捕捉混淆
// ============================================================================

func TestFullPipeline_WithConfusionDetection(t *testing.T) {
	// 独立验证: 规则引擎能捕获混淆
	rules := DefaultAntiConfusionRules()

	report := NewReport("r-1", "agent-1", "sess-1")

	claim := NewClaim("c-1", KindInferred, "可能上涨")
	claim.AddCitation(ObjectRef{Type: "experiment", ID: "e-1"}, time.Now())
	claim.Level = LevelModerate
	claim.Uncertainty.ConfidenceLevel = 0.85
	claim.Uncertainty.ConfidenceScale = "model_self"
	wr := 0.55
	claim.Uncertainty.HistoricalWinRate = &wr
	claim.Uncertainty.HistoricalSamples = 100
	report.AddClaim(claim)

	// 规则检查应发现混淆
	violations := report.ApplyRules(rules)
	found := false
	for _, v := range violations {
		if v.RuleID == "no_llm_confidence_as_winrate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("rule engine should detect model_self + historical confusion")
	}
}

// ============================================================================
// 工具函数测试
// ============================================================================

func TestValidConfidenceScale(t *testing.T) {
	if !ValidConfidenceScale("model_self") {
		t.Error("model_self should be valid")
	}
	if !ValidConfidenceScale("historical") {
		t.Error("historical should be valid")
	}
	if ValidConfidenceScale("invalid") {
		t.Error("invalid should not be valid")
	}
}

func TestScaleDisplayName(t *testing.T) {
	if ScaleDisplayName("model_self") != "模型自评 (LLM self-assessment)" {
		t.Error("unexpected display name")
	}
	if ScaleDisplayName("historical") != "历史统计 (historical backtest)" {
		t.Error("unexpected display name")
	}
}

// ============================================================================
// 关键约束: 禁止 LLM confidence 与历史胜率混淆
// ============================================================================

func TestCriticalConstraint_ModelSelfNotHistorical(t *testing.T) {
	// 这是 tongstock-qhe.6.4 的核心约束:
	// LLM confidence (model_self) 不能表述为历史胜率
	// 历史胜率必须用 HistoricalWinRate + HistoricalSamples 独立表示
	// 混淆检测由规则引擎 (AntiConfusionRules) 执行,
	// UncertaintyExpression.Validate 仅做字段级校验

	// Case 1: 正确用法
	correct := UncertaintyExpression{
		ConfidenceLevel: 0.75,
		ConfidenceScale: "model_self",
		// HistoricalWinRate 为 nil — 正确!
	}
	if err := correct.Validate(); err != nil {
		t.Errorf("correct usage failed: %v", err)
	}

	// Case 2: 规则引擎检测混淆
	rules := DefaultAntiConfusionRules()
	report := NewReport("r-1", "agent-1", "sess-1")
	claim := NewClaim("c-1", KindInferred, "可能上涨")
	claim.AddCitation(ObjectRef{Type: "experiment", ID: "e-1"}, time.Now())
	claim.Uncertainty.ConfidenceLevel = 0.75
	claim.Uncertainty.ConfidenceScale = "model_self"
	claim.Uncertainty.HistoricalWinRate = floatPtr(0.75) // 典型混淆
	claim.Uncertainty.HistoricalSamples = 100
	report.AddClaim(claim)

	violations := report.ApplyRules(rules)
	found := false
	for _, v := range violations {
		if v.RuleID == "no_llm_confidence_as_winrate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("rule engine should detect model_self + historical_win_rate confusion")
	}
}

func floatPtr(v float64) *float64 { return &v }
