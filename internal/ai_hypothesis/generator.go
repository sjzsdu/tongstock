package ai_hypothesis

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// 提示词模板 (版本化)
// ============================================================================

// PromptTemplate 提示词模板
type PromptTemplate struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	System  string `json:"system"` // 系统提示
	User    string `json:"user"`   // 用户提示模板
}

// DefaultPromptTemplates 默认提示词模板
var DefaultPromptTemplates = []PromptTemplate{
	{
		ID:      "hypothesis-generator-v1",
		Version: "1.0",
		System:  "你是一名量化研究专家, 专注于生成可证伪的交易假设。",
		User: `基于以下可用特征和历史证据, 生成一个结构化的交易假设。
要求:
1. 假设必须可证伪 (falsifiable) — 即可以通过回测明确证明是错误的
2. 不要给出买卖结论, 只描述条件 -> 行为关系
3. 必须包含: 行为逻辑、反例、验证项
4. 输出必须符合 ParadigmSchema 格式

可用特征:
{{.Features}}

历史证据摘要:
{{.Evidence}}

请生成假设。`,
	},
}

// ============================================================================
// 可证伪性检查器
// ============================================================================

// FalsifiabilityChecker 检查假设是否可证伪
type FalsifiabilityChecker struct {
	// 必须包含的关键词 (可证伪性指标)
	FalsifiableKeywords []string
}

// NewFalsifiabilityChecker 创建默认可证伪性检查器
func NewFalsifiabilityChecker() *FalsifiabilityChecker {
	return &FalsifiabilityChecker{
		FalsifiableKeywords: []string{
			"when", "if", "then", "只要", "当",
			"超过", "低于", "上穿", "下穿",
			"should", "would", "will",
			"break", "violate", "fail",
		},
	}
}

// Check 检查假设的可证伪性
func (fc *FalsifiabilityChecker) Check(h *AIHypothesis) FalsifiabilityResult {
	result := FalsifiabilityResult{
		IsFalsifiable: true,
		Issues:        make([]string, 0),
		Score:         0.0,
	}

	// 1. 检查陈述是否包含条件-结果结构
	statement := strings.ToLower(h.Statement)
	hasCondition := false
	for _, kw := range fc.FalsifiableKeywords {
		if strings.Contains(statement, strings.ToLower(kw)) {
			hasCondition = true
			break
		}
	}
	if !hasCondition {
		result.IsFalsifiable = false
		result.Issues = append(result.Issues,
			"假设陈述缺少条件-结果结构, 无法被证伪")
	} else {
		result.Score += 0.3
	}

	// 2. 检查是否有至少一个反例
	if len(h.CounterExamples) == 0 {
		result.IsFalsifiable = false
		result.Issues = append(result.Issues,
			"没有反例, 无法验证假设边界条件")
	} else {
		result.Score += 0.2
		// 反例越多, 可证伪性越强 (但 3 个以上 diminishing returns)
		ceScore := float64(len(h.CounterExamples))
		if ceScore > 3 {
			ceScore = 3.0
		}
		result.Score += ceScore * 0.1
	}

	// 3. 检查是否有至少一个验证项
	if len(h.Verifications) == 0 {
		result.IsFalsifiable = false
		result.Issues = append(result.Issues,
			"没有验证项, 无法通过回测检验")
	} else {
		result.Score += 0.2
		// 验证项越多越好 (但 5 个以上 diminishing returns)
		vScore := float64(len(h.Verifications))
		if vScore > 5 {
			vScore = 5.0
		}
		result.Score += vScore * 0.06
	}

	// 4. 检查陈述是否避免了循环定义
	if fc.isCircularDefinition(statement) {
		result.IsFalsifiable = false
		result.Issues = append(result.Issues,
			"假设存在循环定义, 无法独立验证")
	} else {
		result.Score += 0.1
	}

	// 5. 检查陈述是否是同义反复 (tautology)
	if fc.isTautology(statement) {
		result.IsFalsifiable = false
		result.Issues = append(result.Issues,
			"假设是同义反复 (tautology), 无法被证伪")
	} else {
		result.Score += 0.1
	}

	// 归一化分数
	if result.Score > 1.0 {
		result.Score = 1.0
	}

	return result
}

// isCircularDefinition 检查循环定义 (简化版)
func (fc *FalsifiabilityChecker) isCircularDefinition(statement string) bool {
	// 检查是否用结论作为前提
	circularPatterns := []string{
		"because it goes up",
		"will rise because",
		"涨因为涨",
		"because of the trend",
	}
	for _, pattern := range circularPatterns {
		if strings.Contains(strings.ToLower(statement), pattern) {
			return true
		}
	}
	return false
}

// isTautology 检查同义反复 (简化版)
func (fc *FalsifiabilityChecker) isTautology(statement string) bool {
	tautologyPatterns := []string{
		"will go up or down",
		"either increase or decrease",
		"可能涨也可能跌",
		"always moves in the direction",
	}
	for _, pattern := range tautologyPatterns {
		if strings.Contains(strings.ToLower(statement), pattern) {
			return true
		}
	}
	return false
}

// FalsifiabilityResult 可证伪性检查结果
type FalsifiabilityResult struct {
	IsFalsifiable bool     `json:"is_falsifiable"`
	Score         float64  `json:"score"` // 0-1, 越高越可证伪
	Issues        []string `json:"issues,omitempty"`
}

// ============================================================================
// 假设生成器接口
// ============================================================================

// HypothesisInput 假设生成输入
type HypothesisInput struct {
	// AvailableFeatures 可用的特征列表
	AvailableFeatures []FeatureInfo `json:"available_features"`
	// HistoricalEvidence 历史证据摘要
	HistoricalEvidence []EvidenceSummary `json:"historical_evidence"`
	// MarketContext 当前市场上下文
	MarketContext string `json:"market_context"`
	// AdditionalConstraints 额外约束
	AdditionalConstraints []string `json:"additional_constraints,omitempty"`
}

// FeatureInfo 可用特征信息
type FeatureInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // indicator, price, volume, market
	Description string `json:"description"`
	Available   bool   `json:"available"` // 数据是否可用
}

// EvidenceSummary 历史证据摘要
type EvidenceSummary struct {
	ID          string  `json:"id"`
	Description string  `json:"description"`
	Source      string  `json:"source"` // dataset, experiment, paradigm
	HitRate     float64 `json:"hit_rate,omitempty"`
	SampleSize  int     `json:"sample_size,omitempty"`
}

// HypothesisGenerator 假设生成器接口
type HypothesisGenerator interface {
	// Generate 生成一个假设
	Generate(input HypothesisInput) (*AIHypothesis, error)
	// GenerateBatch 批量生成假设
	GenerateBatch(input HypothesisInput, count int) ([]*AIHypothesis, error)
	// ValidateFalsifiability 验证可证伪性
	ValidateFalsifiability(h *AIHypothesis) FalsifiabilityResult
}

// ============================================================================
// 结构化假设生成器实现
// ============================================================================

// StructuredHypothesisGenerator 结构化 AI 假设生成器
type StructuredHypothesisGenerator struct {
	model          string
	modelVersion   string
	promptTemplate PromptTemplate
	falsifiability *FalsifiabilityChecker
	validators     []SchemaValidator
	// RequireVerification 必须包含验证项
	RequireVerification bool
	// RequireCounterExample 必须包含反例
	RequireCounterExample bool
}

// SchemaValidator Schema 验证器接口
type SchemaValidator interface {
	Validate(spec *HypothesisSchemaSpec) error
	Name() string
}

// NewStructuredGenerator 创建结构化假设生成器
func NewStructuredGenerator(model, modelVersion string, promptTemplateID string) *StructuredHypothesisGenerator {
	var tmpl PromptTemplate
	for _, t := range DefaultPromptTemplates {
		if t.ID == promptTemplateID {
			tmpl = t
			break
		}
	}
	if tmpl.ID == "" {
		tmpl = DefaultPromptTemplates[0]
	}

	return &StructuredHypothesisGenerator{
		model:                 model,
		modelVersion:          modelVersion,
		promptTemplate:        tmpl,
		falsifiability:        NewFalsifiabilityChecker(),
		validators:            make([]SchemaValidator, 0),
		RequireVerification:   true,
		RequireCounterExample: true,
	}
}

// AddValidator 添加 Schema 验证器
func (g *StructuredHypothesisGenerator) AddValidator(v SchemaValidator) {
	g.validators = append(g.validators, v)
}

// Generate 生成单个假设
func (g *StructuredHypothesisGenerator) Generate(input HypothesisInput) (*AIHypothesis, error) {
	// 1. 检查输入完整性
	if err := g.validateInput(input); err != nil {
		return nil, fmt.Errorf("input validation failed: %w", err)
	}

	// 2. 检查缺失数据
	missingData := g.detectMissingData(input)

	// 3. 生成假设 (基于模板 + 输入)
	hypothesis := g.buildHypothesis(input, missingData)

	// 4. 如果有关键缺失数据, 直接拒绝
	if hypothesis.HasCriticalMissingData() {
		hypothesis.Reject(fmt.Sprintf("缺失关键数据: %s", formatMissingData(hypothesis.MissingData)))
		return hypothesis, nil
	}

	// 5. 可证伪性检查
	falsResult := g.ValidateFalsifiability(hypothesis)
	if !falsResult.IsFalsifiable {
		hypothesis.Reject(fmt.Sprintf("不可证伪: %v", falsResult.Issues))
		return hypothesis, nil
	}

	// 6. Schema 验证
	if err := g.validateSchema(hypothesis); err != nil {
		hypothesis.Reject(fmt.Sprintf("Schema 不合规: %v", err))
		return hypothesis, nil
	}

	// 7. 全部通过, 标记为合规
	hypothesis.Approve()
	hypothesis.MarkSchemaOK()

	return hypothesis, nil
}

// GenerateBatch 批量生成假设
func (g *StructuredHypothesisGenerator) GenerateBatch(input HypothesisInput, count int) ([]*AIHypothesis, error) {
	if count <= 0 {
		count = 1
	}

	results := make([]*AIHypothesis, 0, count)
	seen := make(map[string]bool)

	for i := 0; i < count; i++ {
		// 为每个假设生成差异化输入
		iterInput := g.buildIterationInput(input, i, count)

		h, err := g.Generate(iterInput)
		if err != nil {
			continue // 跳过失败的
		}

		// 去重 (基于陈述)
		key := normalizeStatement(h.Statement)
		if seen[key] {
			continue
		}
		seen[key] = true

		results = append(results, h)
	}

	return results, nil
}

// ValidateFalsifiability 执行可证伪性检查
func (g *StructuredHypothesisGenerator) ValidateFalsifiability(h *AIHypothesis) FalsifiabilityResult {
	return g.falsifiability.Check(h)
}

// ============================================================================
// 内部方法
// ============================================================================

// validateInput 验证输入完整性
func (g *StructuredHypothesisGenerator) validateInput(input HypothesisInput) error {
	if len(input.AvailableFeatures) == 0 {
		return fmt.Errorf("no available features provided")
	}
	return nil
}

// detectMissingData 检测缺失数据
func (g *StructuredHypothesisGenerator) detectMissingData(input HypothesisInput) []MissingDataIssue {
	var missing []MissingDataIssue

	for _, f := range input.AvailableFeatures {
		if !f.Available {
			impact := "warning"
			if strings.HasPrefix(f.Type, "indicator") || f.Type == "price" {
				impact = "critical"
			}
			missing = append(missing, MissingDataIssue{
				FieldName:   f.Name,
				FieldType:   f.Type,
				Description: fmt.Sprintf("特征 %s 数据不可用", f.Name),
				Impact:      impact,
			})
		}
	}

	if len(input.HistoricalEvidence) == 0 {
		missing = append(missing, MissingDataIssue{
			FieldName:   "historical_evidence",
			FieldType:   "evidence",
			Description: "没有历史证据可供参考",
			Impact:      "warning",
		})
	}

	return missing
}

// buildHypothesis 构建假设
func (g *StructuredHypothesisGenerator) buildHypothesis(input HypothesisInput, missing []MissingDataIssue) *AIHypothesis {
	now := time.Now()

	h := NewAIHypothesis(
		fmt.Sprintf("hyp-%s-%d", g.model, now.UnixNano()),
		g.buildTitle(input),
		g.buildStatement(input),
	)

	// 设置版本标签
	inputVersion := fmt.Sprintf("features-%d", len(input.AvailableFeatures))
	evidenceVersion := fmt.Sprintf("evidence-%d", len(input.HistoricalEvidence))
	h.SetVersionTag(
		g.model,
		g.modelVersion,
		g.promptTemplate.ID,
		g.promptTemplate.Version,
		inputVersion,
		evidenceVersion,
	)

	// 设置行为逻辑
	h.Behavior = BehavioralLogic{
		Mechanism:          g.buildMechanism(input),
		Driver:             g.buildDriver(input),
		MarketContext:      input.MarketContext,
		HistoricalEvidence: g.buildEvidenceList(input),
		KeyAssumptions:     g.buildAssumptions(input),
	}

	// 添加反例
	for _, ce := range g.buildCounterExamples(input) {
		h.AddCounterExample(ce.Condition, ce.WhyItFails, ce.Severity)
	}

	// 添加验证项
	for _, vi := range g.buildVerifications(input) {
		h.AddVerification(vi.Name, vi.Description, vi.Metric, vi.Threshold, vi.Direction, vi.Category)
	}

	// 设置 Schema 规范
	h.SchemaSpec = g.buildSchemaSpec(input)

	// 添加缺失数据
	for _, md := range missing {
		h.AddMissingData(md.FieldName, md.FieldType, md.Description, md.Impact)
	}

	return h
}

// buildIterationInput 为批量生成构建差异化输入
func (g *StructuredHypothesisGenerator) buildIterationInput(input HypothesisInput, index, total int) HypothesisInput {
	// 简单的差异化: 调整特征权重顺序
	features := make([]FeatureInfo, len(input.AvailableFeatures))
	copy(features, input.AvailableFeatures)

	// 轮转特征顺序
	if len(features) > 0 {
		offset := index % len(features)
		rotated := make([]FeatureInfo, len(features))
		for i, f := range features {
			rotated[(i+offset)%len(features)] = f
		}
		features = rotated
	}

	return HypothesisInput{
		AvailableFeatures:     features,
		HistoricalEvidence:    input.HistoricalEvidence,
		MarketContext:         input.MarketContext,
		AdditionalConstraints: input.AdditionalConstraints,
	}
}

// buildTitle 构建假设标题
func (g *StructuredHypothesisGenerator) buildTitle(input HypothesisInput) string {
	if len(input.AvailableFeatures) == 0 {
		return "无可用特征假设"
	}
	primaryFeature := input.AvailableFeatures[0].Name
	return fmt.Sprintf("基于 %s 的条件性交易假设", primaryFeature)
}

// buildStatement 构建假设陈述 (可证伪的条件-结果结构)
func (g *StructuredHypothesisGenerator) buildStatement(input HypothesisInput) string {
	if len(input.AvailableFeatures) < 2 {
		return "当主要指标触发超卖条件时, 价格将在短期内出现均值回归"
	}

	f1 := input.AvailableFeatures[0]
	f2 := input.AvailableFeatures[1]
	return fmt.Sprintf(
		"当 %s 处于超卖区域且 %s 确认价格反转时, 价格在未来 5-20 个交易日内将向均线回归",
		f1.Name, f2.Name,
	)
}

// buildMechanism 构建机制描述
func (g *StructuredHypothesisGenerator) buildMechanism(input HypothesisInput) string {
	return "短期过度抛售导致价格暂时偏离其内在价值中枢, 当卖压衰竭后, 价格倾向于回归到近期价值区间"
}

// buildDriver 构建驱动因素
func (g *StructuredHypothesisGenerator) buildDriver(input HypothesisInput) string {
	if input.MarketContext != "" {
		return fmt.Sprintf("在 %s 市场环境下, 短期情绪超卖提供了逆向交易机会", input.MarketContext)
	}
	return "市场参与者的情绪过度反应创造了短期交易机会"
}

// buildEvidenceList 构建历史证据列表
func (g *StructuredHypothesisGenerator) buildEvidenceList(input HypothesisInput) []string {
	var evidence []string
	for _, e := range input.HistoricalEvidence {
		if e.SampleSize > 0 {
			evidence = append(evidence,
				fmt.Sprintf("%s: %s (命中率 %.1f%%, 样本量 %d)",
					e.Source, e.Description, e.HitRate*100, e.SampleSize))
		} else {
			evidence = append(evidence,
				fmt.Sprintf("%s: %s", e.Source, e.Description))
		}
	}
	if len(evidence) == 0 {
		evidence = append(evidence, "暂无历史直接证据, 需要回测验证")
	}
	return evidence
}

// buildAssumptions 构建关键假设
func (g *StructuredHypothesisGenerator) buildAssumptions(input HypothesisInput) []string {
	return []string{
		"市场存在均值回归特性",
		"超卖信号具有短期预测力",
		"交易成本不影响策略有效性",
	}
}

// buildCounterExamples 构建反例
func (g *StructuredHypothesisGenerator) buildCounterExamples(input HypothesisInput) []CounterExample {
	examples := []CounterExample{
		{
			Condition:  "市场处于强趋势下跌中",
			WhyItFails: "趋势市中, 超卖信号可能是继续下跌的前兆而非反转信号",
			Severity:   "high",
		},
		{
			Condition:  "个股出现重大利空消息",
			WhyItFails: "基本面恶化导致价格持续下跌, 技术指标失效",
			Severity:   "high",
		},
		{
			Condition:  "成交量持续萎缩",
			WhyItFails: "低流动性环境中, 价格可能进一步下跌至流动性枯竭",
			Severity:   "medium",
		},
	}
	return examples
}

// buildVerifications 构建验证项
func (g *StructuredHypothesisGenerator) buildVerifications(input HypothesisInput) []VerificationItem {
	return []VerificationItem{
		{
			Name:        "夏普比率",
			Description: "风险调整后收益",
			Metric:      "sharpe_ratio",
			Threshold:   1.0,
			Direction:   "above",
			Category:    "performance",
		},
		{
			Name:        "最大回撤",
			Description: "策略期间最大回撤幅度",
			Metric:      "max_drawdown",
			Threshold:   0.20,
			Direction:   "below",
			Category:    "risk",
		},
		{
			Name:        "胜率",
			Description: "盈利交易占比",
			Metric:      "win_rate",
			Threshold:   0.50,
			Direction:   "above",
			Category:    "performance",
		},
		{
			Name:        "收益稳定性",
			Description: "月度收益标准差",
			Metric:      "monthly_return_std",
			Threshold:   0.05,
			Direction:   "below",
			Category:    "stability",
		},
	}
}

// buildSchemaSpec 构建 Schema 规范
func (g *StructuredHypothesisGenerator) buildSchemaSpec(input HypothesisInput) HypothesisSchemaSpec {
	var entryConds []string
	var exitConds []string

	for _, f := range input.AvailableFeatures {
		if f.Type == "indicator" {
			entryConds = append(entryConds,
				fmt.Sprintf("%s 处于超卖区域", f.Name))
		}
		if f.Type == "price" {
			exitConds = append(exitConds,
				fmt.Sprintf("价格回归到 %s 均线", f.Name))
		}
	}

	if len(entryConds) == 0 {
		entryConds = append(entryConds, "主要指标触发超卖")
	}
	if len(exitConds) == 0 {
		exitConds = append(exitConds, "价格回归均值")
	}

	return HypothesisSchemaSpec{
		SchemaID:        fmt.Sprintf("schema-%d", time.Now().UnixNano()),
		SchemaName:      g.buildTitle(input),
		HoldingPeriod:   "medium",
		EntryConditions: entryConds,
		ExitConditions:  exitConds,
		ContextConstraints: []string{
			"震荡市或短期超卖反弹",
			"非强趋势市场",
		},
		ExpectedReturn: "5-15%",
		RiskLevel:      "medium",
	}
}

// validateSchema 执行 Schema 验证
func (g *StructuredHypothesisGenerator) validateSchema(h *AIHypothesis) error {
	// 检查是否有至少一个入场条件
	if len(h.SchemaSpec.EntryConditions) == 0 {
		return fmt.Errorf("schema must have at least one entry condition")
	}

	// 检查验证项完整性
	if g.RequireVerification && len(h.Verifications) == 0 {
		return fmt.Errorf("hypothesis must have at least one verification item")
	}

	// 检查反例完整性
	if g.RequireCounterExample && len(h.CounterExamples) == 0 {
		return fmt.Errorf("hypothesis must have at least one counter-example")
	}

	// 运行自定义验证器
	for _, v := range g.validators {
		if err := v.Validate(&h.SchemaSpec); err != nil {
			return fmt.Errorf("validator %s: %w", v.Name(), err)
		}
	}

	return nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// formatMissingData 格式化缺失数据
func formatMissingData(issues []MissingDataIssue) string {
	var parts []string
	for _, md := range issues {
		parts = append(parts, fmt.Sprintf("%s(%s)", md.FieldName, md.Impact))
	}
	return strings.Join(parts, ", ")
}

// normalizeStatement 标准化陈述 (用于去重)
func normalizeStatement(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

// ============================================================================
// 内置 Schema 验证器
// ============================================================================

// EntryConditionValidator 入场条件验证器
type EntryConditionValidator struct{}

func (v *EntryConditionValidator) Name() string { return "entry_condition_validator" }

func (v *EntryConditionValidator) Validate(spec *HypothesisSchemaSpec) error {
	if len(spec.EntryConditions) == 0 {
		return fmt.Errorf("entry conditions must not be empty")
	}
	return nil
}

// RiskLevelValidator 风险等级验证器
type RiskLevelValidator struct{}

func (v *RiskLevelValidator) Name() string { return "risk_level_validator" }

func (v *RiskLevelValidator) Validate(spec *HypothesisSchemaSpec) error {
	validLevels := map[string]bool{"low": true, "medium": true, "high": true}
	if !validLevels[spec.RiskLevel] {
		return fmt.Errorf("invalid risk level: %s", spec.RiskLevel)
	}
	return nil
}

// DefaultValidators 返回默认验证器列表
func DefaultValidators() []SchemaValidator {
	return []SchemaValidator{
		&EntryConditionValidator{},
		&RiskLevelValidator{},
	}
}
