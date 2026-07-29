package ai_hypothesis

import (
	"testing"
	"time"
)

// ============================================================================
// 基础数据模型测试
// ============================================================================

func TestNewAIHypothesis(t *testing.T) {
	h := NewAIHypothesis("hyp-1", "测试假设", "当 RSI 超卖时价格将反弹")

	if h.ID != "hyp-1" {
		t.Errorf("expected ID hyp-1, got %s", h.ID)
	}
	if h.Title != "测试假设" {
		t.Errorf("expected title, got %s", h.Title)
	}
	if h.Status != HypothesisDraft {
		t.Errorf("expected draft status, got %s", h.Status)
	}
	if h.CreatedAt.IsZero() {
		t.Error("created_at should be set")
	}
}

func TestAddCounterExample(t *testing.T) {
	h := NewAIHypothesis("hyp-1", "测试", "陈述")
	h.AddCounterExample("条件1", "原因1", "high")

	if len(h.CounterExamples) != 1 {
		t.Errorf("expected 1 counter example, got %d", len(h.CounterExamples))
	}
	if h.CounterExamples[0].Severity != "high" {
		t.Error("wrong severity")
	}
}

func TestAddVerification(t *testing.T) {
	h := NewAIHypothesis("hyp-1", "测试", "陈述")
	h.AddVerification("夏普比率", "风险调整收益", "sharpe_ratio", 1.0, "above", "performance")

	if len(h.Verifications) != 1 {
		t.Errorf("expected 1 verification, got %d", len(h.Verifications))
	}
	if h.Verifications[0].Threshold != 1.0 {
		t.Error("wrong threshold")
	}
}

func TestSetVersionTag(t *testing.T) {
	h := NewAIHypothesis("hyp-1", "测试", "陈述")
	h.SetVersionTag("gpt-4", "1.0", "tmpl-1", "1.0", "v1", "v2")

	if h.VersionTag.Model != "gpt-4" {
		t.Error("wrong model")
	}
	if h.VersionTag.PromptTemplateID != "tmpl-1" {
		t.Error("wrong prompt template ID")
	}
	if h.VersionTag.InputFeaturesVersion != "v1" {
		t.Error("wrong features version")
	}
	if h.VersionTag.InputEvidenceVersion != "v2" {
		t.Error("wrong evidence version")
	}
	if h.VersionTag.GeneratedAt.IsZero() {
		t.Error("generated_at should be set")
	}
}

// ============================================================================
// 状态管理测试
// ============================================================================

func TestStatusTransitions(t *testing.T) {
	h := NewAIHypothesis("hyp-1", "测试", "陈述")

	h.Approve()
	if h.Status != HypothesisValidated {
		t.Error("expected validated")
	}

	h.MarkSchemaOK()
	if h.Status != HypothesisSchemaOK {
		t.Error("expected schema_ok")
	}

	h.EnterQuarantine()
	if h.Status != HypothesisInQuarantine {
		t.Error("expected quarantine")
	}

	h.Reject("测试拒绝")
	if h.Status != HypothesisRejected {
		t.Error("expected rejected")
	}
	if h.RejectReason != "测试拒绝" {
		t.Error("wrong reject reason")
	}
}

// ============================================================================
// 缺失数据检测测试
// ============================================================================

func TestHasCriticalMissingData(t *testing.T) {
	h := NewAIHypothesis("hyp-1", "测试", "陈述")

	// 无缺失
	if h.HasCriticalMissingData() {
		t.Error("should not have critical missing data")
	}

	// 只有 warning
	h.AddMissingData("price", "feature", "价格数据延迟", "warning")
	if h.HasCriticalMissingData() {
		t.Error("should not have critical missing data with only warnings")
	}

	// 有 critical
	h.AddMissingData("RSI", "indicator", "RSI 数据缺失", "critical")
	if !h.HasCriticalMissingData() {
		t.Error("should have critical missing data")
	}
}

// ============================================================================
// 验证测试
// ============================================================================

func TestValidate_MissingFields(t *testing.T) {
	h := NewAIHypothesis("", "测试", "陈述")
	if err := h.Validate(); err == nil {
		t.Error("expected error for empty ID")
	}

	h = NewAIHypothesis("id", "", "陈述")
	if err := h.Validate(); err == nil {
		t.Error("expected error for empty title")
	}

	h = NewAIHypothesis("id", "title", "")
	if err := h.Validate(); err == nil {
		t.Error("expected error for empty statement")
	}
}

func TestValidate_MissingVersionTag(t *testing.T) {
	h := NewAIHypothesis("hyp-1", "测试", "陈述")
	// 没有设置版本标签
	if err := h.Validate(); err == nil {
		t.Error("expected error for missing version tag")
	}

	// 完整设置版本标签
	h.SetVersionTag("model", "v1", "tmpl", "v1", "fv1", "ev1")
	if err := h.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================================
// 可证伪性检查测试
// ============================================================================

func TestFalsifiabilityChecker_Pass(t *testing.T) {
	checker := NewFalsifiabilityChecker()

	h := NewAIHypothesis("hyp-1", "测试", "当 RSI 超卖时价格反弹")
	h.AddCounterExample("强趋势下跌", "趋势市中超卖无效", "high")
	h.AddVerification("夏普比率", "收益", "sharpe", 1.0, "above", "performance")

	result := checker.Check(h)
	if !result.IsFalsifiable {
		t.Errorf("should be falsifiable, got issues: %v", result.Issues)
	}
	if result.Score <= 0 {
		t.Error("score should be positive")
	}
}

func TestFalsifiabilityChecker_NoConditionStructure(t *testing.T) {
	checker := NewFalsifiabilityChecker()

	h := NewAIHypothesis("hyp-1", "测试", "市场会上涨") // 没有条件结构
	h.AddCounterExample("条件", "原因", "high")
	h.AddVerification("v", "d", "m", 0.5, "above", "p")

	result := checker.Check(h)
	if result.IsFalsifiable {
		t.Error("should not be falsifiable without condition structure")
	}
}

func TestFalsifiabilityChecker_NoCounterExample(t *testing.T) {
	checker := NewFalsifiabilityChecker()

	h := NewAIHypothesis("hyp-1", "当 RSI 超卖时价格反弹", "if RSI < 30 then price rebounds")
	// 没有反例
	h.AddVerification("v", "d", "m", 0.5, "above", "p")

	result := checker.Check(h)
	if result.IsFalsifiable {
		t.Error("should not be falsifiable without counter examples")
	}
}

func TestFalsifiabilityChecker_NoVerification(t *testing.T) {
	checker := NewFalsifiabilityChecker()

	h := NewAIHypothesis("hyp-1", "测试", "if RSI < 30 then price rebounds")
	h.AddCounterExample("条件", "原因", "high")
	// 没有验证项

	result := checker.Check(h)
	if result.IsFalsifiable {
		t.Error("should not be falsifiable without verifications")
	}
}

func TestFalsifiabilityChecker_CircularDefinition(t *testing.T) {
	checker := NewFalsifiabilityChecker()

	h := NewAIHypothesis("hyp-1", "测试", "The price will go up because it goes up")
	h.AddCounterExample("条件", "原因", "high")
	h.AddVerification("v", "d", "m", 0.5, "above", "p")

	result := checker.Check(h)
	if result.IsFalsifiable {
		t.Error("should not be falsifiable with circular definition")
	}
}

func TestFalsifiabilityChecker_Tautology(t *testing.T) {
	checker := NewFalsifiabilityChecker()

	h := NewAIHypothesis("hyp-1", "测试", "The price will either increase or decrease")
	h.AddCounterExample("条件", "原因", "high")
	h.AddVerification("v", "d", "m", 0.5, "above", "p")

	result := checker.Check(h)
	if result.IsFalsifiable {
		t.Error("should not be falsifiable with tautology")
	}
}

// ============================================================================
// 结构化生成器测试
// ============================================================================

func createTestInput() HypothesisInput {
	return HypothesisInput{
		AvailableFeatures: []FeatureInfo{
			{Name: "RSI14", Type: "indicator", Description: "14日RSI", Available: true},
			{Name: "price.close", Type: "price", Description: "收盘价", Available: true},
			{Name: "MA20", Type: "indicator", Description: "20日均线", Available: true},
			{Name: "volume", Type: "volume", Description: "成交量", Available: true},
		},
		HistoricalEvidence: []EvidenceSummary{
			{ID: "e-1", Description: "RSI超卖反弹", Source: "experiment", HitRate: 0.58, SampleSize: 50},
			{ID: "e-2", Description: "均线支撑", Source: "paradigm", HitRate: 0.52, SampleSize: 30},
		},
		MarketContext: "range",
	}
}

func TestStructuredGenerator_Generate(t *testing.T) {
	gen := NewStructuredGenerator("gpt-4", "1.0", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	input := createTestInput()
	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 应该通过所有检查
	if h.Status != HypothesisSchemaOK {
		t.Errorf("expected schema_ok, got %s (reason: %s)", h.Status, h.RejectReason)
	}

	// 检查版本标签
	if h.VersionTag.Model != "gpt-4" {
		t.Error("wrong model in version tag")
	}
	if h.VersionTag.PromptTemplateID != "hypothesis-generator-v1" {
		t.Error("wrong prompt template ID")
	}

	// 检查行为逻辑
	if h.Behavior.Mechanism == "" {
		t.Error("mechanism should not be empty")
	}

	// 检查反例
	if len(h.CounterExamples) == 0 {
		t.Error("should have counter examples")
	}

	// 检查验证项
	if len(h.Verifications) == 0 {
		t.Error("should have verifications")
	}

	// 检查 Schema 规范
	if h.SchemaSpec.SchemaID == "" {
		t.Error("schema ID should not be empty")
	}
	if len(h.SchemaSpec.EntryConditions) == 0 {
		t.Error("should have entry conditions")
	}

	// 验证
	if err := h.Validate(); err != nil {
		t.Errorf("validation failed: %v", err)
	}
}

func TestStructuredGenerator_Batch(t *testing.T) {
	gen := NewStructuredGenerator("gpt-4", "1.0", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	input := createTestInput()
	results, err := gen.GenerateBatch(input, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// 所有结果应通过检查
	for _, h := range results {
		if h.Status != HypothesisSchemaOK {
			t.Errorf("hypothesis %s not approved: status=%s, reason=%s",
				h.ID, h.Status, h.RejectReason)
		}
	}

	// 去重检查 — 不同假设应有不同陈述
	seen := make(map[string]bool)
	for _, h := range results {
		key := h.Statement
		if seen[key] {
			t.Errorf("duplicate statement: %s", key)
		}
		seen[key] = true
	}
}

// ============================================================================
// 缺失数据拒绝测试
// ============================================================================

func TestGenerator_RejectOnCriticalMissingData(t *testing.T) {
	gen := NewStructuredGenerator("gpt-4", "1.0", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	input := HypothesisInput{
		AvailableFeatures: []FeatureInfo{
			{Name: "RSI14", Type: "indicator", Description: "14日RSI", Available: false}, // 关键指标不可用!
			{Name: "price.close", Type: "price", Description: "收盘价", Available: true},
		},
		HistoricalEvidence: []EvidenceSummary{
			{ID: "e-1", Description: "test", Source: "dataset"},
		},
		MarketContext: "range",
	}

	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 应该被拒绝
	if h.Status != HypothesisRejected {
		t.Errorf("expected rejected, got %s", h.Status)
	}

	// 检查拒绝原因
	if h.RejectReason == "" {
		t.Error("should have reject reason")
	}

	// 检查缺失数据记录
	if len(h.MissingData) == 0 {
		t.Error("should have missing data records")
	}

	if !h.HasCriticalMissingData() {
		t.Error("should have critical missing data")
	}
}

func TestGenerator_RejectOnNoFeatures(t *testing.T) {
	gen := NewStructuredGenerator("gpt-4", "1.0", "hypothesis-generator-v1")

	input := HypothesisInput{
		AvailableFeatures: []FeatureInfo{}, // 无可用特征
		MarketContext:     "range",
	}

	_, err := gen.Generate(input)
	if err == nil {
		t.Error("expected error for no features")
	}
}

// ============================================================================
// 版本标签完整性测试
// ============================================================================

func TestVersionTag_Completeness(t *testing.T) {
	gen := NewStructuredGenerator("claude-3", "2.0", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	input := createTestInput()
	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tag := h.VersionTag
	if tag.Model != "claude-3" {
		t.Error("wrong model")
	}
	if tag.ModelVersion != "2.0" {
		t.Error("wrong model version")
	}
	if tag.PromptTemplateID != "hypothesis-generator-v1" {
		t.Error("wrong prompt template ID")
	}
	if tag.PromptVersion != "1.0" {
		t.Error("wrong prompt version")
	}
	if tag.InputFeaturesVersion == "" {
		t.Error("input features version should not be empty")
	}
	if tag.InputEvidenceVersion == "" {
		t.Error("input evidence version should not be empty")
	}
	if tag.GeneratedAt.IsZero() {
		t.Error("generated_at should be set")
	}

	// 时间戳应合理 (最近)
	if time.Since(tag.GeneratedAt) > time.Hour {
		t.Error("generated_at should be recent")
	}
}

// ============================================================================
// Schema 合规性测试
// ============================================================================

func TestSchemaValidator_EntryCondition(t *testing.T) {
	v := &EntryConditionValidator{}
	if v.Name() != "entry_condition_validator" {
		t.Error("wrong name")
	}

	// 有入场条件
	err := v.Validate(&HypothesisSchemaSpec{
		EntryConditions: []string{"RSI 超卖"},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 无入场条件
	err = v.Validate(&HypothesisSchemaSpec{})
	if err == nil {
		t.Error("expected error for empty entry conditions")
	}
}

func TestSchemaValidator_RiskLevel(t *testing.T) {
	v := &RiskLevelValidator{}

	// 有效风险等级
	validLevels := []string{"low", "medium", "high"}
	for _, level := range validLevels {
		err := v.Validate(&HypothesisSchemaSpec{RiskLevel: level})
		if err != nil {
			t.Errorf("unexpected error for %s: %v", level, err)
		}
	}

	// 无效风险等级
	err := v.Validate(&HypothesisSchemaSpec{RiskLevel: "extreme"})
	if err == nil {
		t.Error("expected error for invalid risk level")
	}
}

// ============================================================================
// 集成测试: 完整假设生成流程
// ============================================================================

func TestFullGenerationPipeline(t *testing.T) {
	// 1. 创建生成器
	gen := NewStructuredGenerator("test-model", "0.1", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	// 2. 准备输入
	input := HypothesisInput{
		AvailableFeatures: []FeatureInfo{
			{Name: "RSI14", Type: "indicator", Description: "14日RSI", Available: true},
			{Name: "price.close", Type: "price", Description: "收盘价", Available: true},
			{Name: "MA20", Type: "indicator", Description: "20日均线", Available: true},
		},
		HistoricalEvidence: []EvidenceSummary{
			{ID: "e-1", Description: "RSI 30以下反弹概率 58%", Source: "backtest", HitRate: 0.58, SampleSize: 120},
			{ID: "e-2", Description: "均线支撑有效率 52%", Source: "experiment", HitRate: 0.52, SampleSize: 80},
		},
		MarketContext: "range",
		AdditionalConstraints: []string{
			"仅适用于震荡市",
			"单票仓位不超过 10%",
		},
	}

	// 3. 生成假设
	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 4. 验证假设状态
	if h.Status != HypothesisSchemaOK {
		t.Fatalf("hypothesis not approved: status=%s, reason=%s", h.Status, h.RejectReason)
	}

	// 5. 验证可证伪性
	falsResult := gen.ValidateFalsifiability(h)
	if !falsResult.IsFalsifiable {
		t.Errorf("falsifiability check failed: %v", falsResult.Issues)
	}

	// 6. 验证版本标签完整性
	tag := h.VersionTag
	if tag.Model != "test-model" || tag.ModelVersion != "0.1" {
		t.Error("version tag model mismatch")
	}
	if tag.PromptTemplateID != "hypothesis-generator-v1" {
		t.Error("version tag prompt mismatch")
	}

	// 7. 验证行为逻辑完整性
	behavior := h.Behavior
	if behavior.Mechanism == "" || behavior.Driver == "" {
		t.Error("behavior logic incomplete")
	}
	if len(behavior.KeyAssumptions) == 0 {
		t.Error("should have key assumptions")
	}

	// 8. 验证反例覆盖
	if len(h.CounterExamples) < 2 {
		t.Error("should have at least 2 counter examples")
	}

	// 9. 验证验证项
	if len(h.Verifications) < 3 {
		t.Error("should have at least 3 verification items")
	}

	// 10. 验证 Schema 规范
	spec := h.SchemaSpec
	if len(spec.EntryConditions) == 0 || len(spec.ExitConditions) == 0 {
		t.Error("schema spec should have entry and exit conditions")
	}
	if spec.HoldingPeriod != "medium" {
		t.Error("expected medium holding period")
	}

	// 11. 验证整体验证
	if err := h.Validate(); err != nil {
		t.Errorf("validation failed: %v", err)
	}

	// 12. 模拟进入隔离区
	h.EnterQuarantine()
	if h.Status != HypothesisInQuarantine {
		t.Error("should be in quarantine")
	}
}

// ============================================================================
// 边界情况测试
// ============================================================================

func TestGenerator_OnlyWarningMissingData(t *testing.T) {
	// 只有 warning 级别的缺失数据, 应该仍然可以通过
	gen := NewStructuredGenerator("gpt-4", "1.0", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	input := HypothesisInput{
		AvailableFeatures: []FeatureInfo{
			{Name: "RSI14", Type: "indicator", Description: "RSI", Available: true},
			{Name: "price.close", Type: "price", Description: "收盘价", Available: true},
			{Name: "volume_ratio", Type: "volume", Description: "量比", Available: false}, // warning 级别
		},
		HistoricalEvidence: []EvidenceSummary{
			{ID: "e-1", Description: "test", Source: "dataset"},
		},
		MarketContext: "range",
	}

	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 应该仍然通过 (只有 warning 级别缺失)
	if h.Status != HypothesisSchemaOK {
		t.Errorf("expected schema_ok, got %s: %s", h.Status, h.RejectReason)
	}

	// 但应该有 warning 记录
	if len(h.MissingData) == 0 {
		t.Error("should have warning-level missing data")
	}
}

func TestGenerator_EmptyEvidence(t *testing.T) {
	// 无历史证据 — 应该仍然可以生成 (有 warning)
	gen := NewStructuredGenerator("gpt-4", "1.0", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	input := HypothesisInput{
		AvailableFeatures: []FeatureInfo{
			{Name: "RSI14", Type: "indicator", Description: "RSI", Available: true},
			{Name: "price.close", Type: "price", Description: "收盘价", Available: true},
		},
		// 无历史证据
		MarketContext: "range",
	}

	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 应该通过 (但会记录 warning)
	if h.Status != HypothesisSchemaOK {
		t.Errorf("expected schema_ok (with warning), got %s: %s", h.Status, h.RejectReason)
	}
}

// ============================================================================
// PromptTemplate 测试
// ============================================================================

func TestDefaultPromptTemplates(t *testing.T) {
	if len(DefaultPromptTemplates) == 0 {
		t.Error("should have at least one default template")
	}

	tmpl := DefaultPromptTemplates[0]
	if tmpl.ID == "" || tmpl.Version == "" {
		t.Error("template should have ID and version")
	}
	if tmpl.System == "" || tmpl.User == "" {
		t.Error("template should have system and user prompts")
	}
}

func TestGenerator_DefaultTemplateFallback(t *testing.T) {
	// 使用不存在的模板 ID, 应该回退到默认
	gen := NewStructuredGenerator("gpt-4", "1.0", "nonexistent-template")

	if gen.promptTemplate.ID != DefaultPromptTemplates[0].ID {
		t.Error("should fallback to default template")
	}
}

// ============================================================================
// 关键约束验证
// ============================================================================

func TestCriticalConstraint_VersionTagPresent(t *testing.T) {
	// AC: 所有假设必须标记模型、提示和输入版本
	gen := NewStructuredGenerator("my-model", "3.5", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	input := createTestInput()
	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 验证所有版本字段都存在
	tag := h.VersionTag
	checks := map[string]string{
		"Model":                tag.Model,
		"ModelVersion":         tag.ModelVersion,
		"PromptTemplateID":     tag.PromptTemplateID,
		"PromptVersion":        tag.PromptVersion,
		"InputFeaturesVersion": tag.InputFeaturesVersion,
		"InputEvidenceVersion": tag.InputEvidenceVersion,
	}

	for name, value := range checks {
		if value == "" {
			t.Errorf("version tag field %s is empty", name)
		}
	}
}

func TestCriticalConstraint_MissingDataRejected(t *testing.T) {
	// AC: 缺少数据或不可执行条件会被拒绝
	gen := NewStructuredGenerator("gpt-4", "1.0", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	// 关键指标不可用
	input := HypothesisInput{
		AvailableFeatures: []FeatureInfo{
			{Name: "MACD", Type: "indicator", Description: "MACD 指标", Available: false},
			{Name: "price.close", Type: "price", Description: "收盘价", Available: true},
		},
		MarketContext: "uptrend",
	}

	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.Status != HypothesisRejected {
		t.Errorf("should be rejected due to critical missing data, got %s", h.Status)
	}

	// 拒绝原因应说明缺失
	if h.RejectReason == "" {
		t.Error("should have reject reason")
	}
}

func TestCriticalConstraint_OutputConformsToSchema(t *testing.T) {
	// AC: 生成结果可直接进入候选隔离区
	gen := NewStructuredGenerator("gpt-4", "1.0", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	input := createTestInput()
	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// Schema 规范应包含必要字段
	spec := h.SchemaSpec
	if spec.SchemaID == "" || spec.SchemaName == "" {
		t.Error("schema spec missing ID or name")
	}
	if len(spec.EntryConditions) == 0 || len(spec.ExitConditions) == 0 {
		t.Error("schema spec missing entry/exit conditions")
	}
	if spec.HoldingPeriod == "" {
		t.Error("schema spec missing holding period")
	}

	// 应该可以直接进入隔离区
	h.EnterQuarantine()
	if h.Status != HypothesisInQuarantine {
		t.Error("should be able to enter quarantine")
	}
}

func TestCriticalConstraint_FalsifiableNotConclusion(t *testing.T) {
	// 核心约束: AI 生成可证伪假设, 而非买卖结论
	gen := NewStructuredGenerator("gpt-4", "1.0", "hypothesis-generator-v1")
	gen.validators = DefaultValidators()

	input := createTestInput()
	h, err := gen.Generate(input)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 检查陈述不是买卖结论
	statement := h.Statement
	conclusionKeywords := []string{"买入", "卖出", "buy now", "sell now", "should buy", "should sell"}
	for _, kw := range conclusionKeywords {
		if contains(statement, kw) {
			t.Errorf("statement should not contain conclusion keyword '%s': %s", kw, statement)
		}
	}

	// 检查是条件-结果结构
	conditionKeywords := []string{"when", "if", "then", "当"}
	hasCondition := false
	for _, kw := range conditionKeywords {
		if contains(statement, kw) {
			hasCondition = true
			break
		}
	}
	if !hasCondition {
		t.Error("statement should have condition-result structure")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
