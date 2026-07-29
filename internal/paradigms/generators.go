package paradigms

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// 候选生成器
// ============================================================================

// ManualGenerator 人工假设生成器
type ManualGenerator struct{}

func NewManualGenerator() *ManualGenerator {
	return &ManualGenerator{}
}

func (g *ManualGenerator) Source() CandidateSource {
	return SourceManual
}

func (g *ManualGenerator) ValidateParams(params GenerateParams) error {
	if params.Count > 10 {
		return fmt.Errorf("manual generation count should be <= 10, got %d", params.Count)
	}
	return nil
}

func (g *ManualGenerator) Generate(params GenerateParams) ([]*Candidate, error) {
	if err := g.ValidateParams(params); err != nil {
		return nil, err
	}

	if params.SeedSchema == nil {
		return nil, fmt.Errorf("manual generation requires seed schema")
	}

	candidate := g.createCandidate(params.SeedSchema, params, "manual_hypothesis")
	return []*Candidate{candidate}, nil
}

func (g *ManualGenerator) createCandidate(schema *ParadigmSchema, params GenerateParams, reason string) *Candidate {
	now := time.Now()
	return &Candidate{
		ID:          fmt.Sprintf("cand-%s-%d", params.BatchID, now.UnixNano()),
		BatchID:     params.BatchID,
		Source:      SourceManual,
		Schema:      schema.DeepCopy(),
		Status:      StatusQuarantine,
		Reason:      reason,
		SearchSpace: "manual",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// TemplateSearchGenerator 模板参数搜索生成器
type TemplateSearchGenerator struct {
	maxCombinations int
	featurePool     []string
	operatorPool    []RuleOperator
}

func NewTemplateSearchGenerator() *TemplateSearchGenerator {
	return &TemplateSearchGenerator{
		maxCombinations: 1000,
		featurePool: []string{
			"MA5", "MA10", "MA20", "MA60",
			"RSI6", "RSI12",
			"MACD", "KDJ",
			"price.close", "price.volume",
		},
		operatorPool: []RuleOperator{
			OpGreaterThan, OpLessThan, OpEqual,
			OpCrossAbove, OpCrossBelow,
			OpAbove, OpBelow,
		},
	}
}

func (g *TemplateSearchGenerator) Source() CandidateSource {
	return SourceTemplate
}

func (g *TemplateSearchGenerator) ValidateParams(params GenerateParams) error {
	if params.SearchConfig == nil {
		return fmt.Errorf("template search requires search config")
	}
	if params.SearchConfig.SearchBudget <= 0 {
		return fmt.Errorf("search budget must be positive")
	}
	if params.Count > g.maxCombinations {
		return fmt.Errorf("requested count %d exceeds max combinations %d", params.Count, g.maxCombinations)
	}
	return nil
}

func (g *TemplateSearchGenerator) Generate(params GenerateParams) ([]*Candidate, error) {
	if err := g.ValidateParams(params); err != nil {
		return nil, err
	}

	var candidates []*Candidate
	count := params.Count
	if count <= 0 || count > g.maxCombinations {
		count = minInt(g.maxCombinations, 100)
	}

	// 生成模板化的候选规则
	for i := 0; i < count && i < params.SearchConfig.SearchBudget; i++ {
		schema := g.generateSchema(i, params)
		if schema != nil {
			candidate := g.createCandidate(schema, params, fmt.Sprintf("template_variant_%d", i))
			candidates = append(candidates, candidate)
		}
	}

	return candidates, nil
}

func (g *TemplateSearchGenerator) generateSchema(index int, params GenerateParams) *ParadigmSchema {
	if params.SeedSchema == nil {
		return nil
	}

	// 基于种子 Schema 生成变体
	schema := params.SeedSchema.DeepCopy()
	schema.ID = fmt.Sprintf("%s-v%d", params.SeedSchema.ID, index+1)
	schema.Version = 1

	// 根据 index 应用不同的变异策略，确保生成的 Schema 差异足够大
	switch index % 5 {
	case 0:
		// 调整阈值 (±25%)
		for j, rule := range schema.Rules {
			if len(rule.Thresholds) > 0 {
				adjustment := 1.0 + float64((index*7+13)%100-50)/200.0 // ±25% 范围
				if adjustment <= 0 {
					adjustment = 0.2
				}
				schema.Rules[j].Thresholds[0] = rule.Thresholds[0] * adjustment
			}
		}
	case 1:
		// 调整持有期和最大回撤
		periods := []string{"intraday", "short", "medium", "long"}
		schema.HoldingPeriod = periods[index%len(periods)]
		schema.MaxDrawdown = 0.05 + float64(index%5)*0.05
		// 删除最后一条规则
		if len(schema.Rules) > 1 {
			schema.Rules = schema.Rules[:len(schema.Rules)-1]
		}
	case 2:
		// 改变规则类型和运算符
		for j, rule := range schema.Rules {
			if rule.Type == TypeExitProfit {
				schema.Rules[j].Type = TypeExitLoss
				schema.Rules[j].Operator = OpLessThan
			} else if rule.Type == TypeExitLoss {
				schema.Rules[j].Type = TypeExitProfit
				schema.Rules[j].Operator = OpGreaterThan
			}
		}
	case 3:
		// 添加新的特征和规则
		newFeature := FeatureDefinition{
			Name:        fmt.Sprintf("variant_feature_%d", index),
			Type:        "indicator",
			Description: fmt.Sprintf("变体特征 %d", index),
		}
		schema.Features = append(schema.Features, newFeature)

		newRule := Rule{
			ID:          fmt.Sprintf("variant-rule-%d", index),
			Type:        TypeConfirmation,
			Side:        SideBuy,
			FeatureName: newFeature.Name,
			Operator:    OpGreaterThan,
			Thresholds:  []float64{0},
			Weight:      0.7,
			Required:    false,
		}
		schema.Rules = append(schema.Rules, newRule)
	case 4:
		// 完全替换规则集
		if len(schema.Features) >= 2 {
			schema.Rules = []Rule{
				{ID: fmt.Sprintf("new-entry-%d", index), Type: TypeEntry, Side: SideBuy, FeatureName: schema.Features[0].Name, Operator: OpGreaterThan, Thresholds: []float64{0}, Required: true},
				{ID: fmt.Sprintf("new-exit-%d", index), Type: TypeExitProfit, Side: SideSell, FeatureName: schema.Features[1].Name, Operator: OpLessThan, Thresholds: []float64{0}, Required: true},
			}
		}
	}

	return schema
}

func (g *TemplateSearchGenerator) createCandidate(schema *ParadigmSchema, params GenerateParams, reason string) *Candidate {
	now := time.Now()
	return &Candidate{
		ID:          fmt.Sprintf("cand-%s-%d", params.BatchID, now.UnixNano()),
		BatchID:     params.BatchID,
		Source:      SourceTemplate,
		Schema:      schema,
		Status:      StatusQuarantine,
		Reason:      reason,
		SearchSpace: "template_search",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// EventStudyGenerator 事件研究生成器
type EventStudyGenerator struct {
	eventTypes []string
}

func NewEventStudyGenerator() *EventStudyGenerator {
	return &EventStudyGenerator{
		eventTypes: []string{"earnings", "merger", "product_launch", "regulatory", "analyst_upgrade"},
	}
}

func (g *EventStudyGenerator) Source() CandidateSource {
	return SourceEventStudy
}

func (g *EventStudyGenerator) ValidateParams(params GenerateParams) error {
	if params.Count > 50 {
		return fmt.Errorf("event study count should be <= 50, got %d", params.Count)
	}
	return nil
}

func (g *EventStudyGenerator) Generate(params GenerateParams) ([]*Candidate, error) {
	if err := g.ValidateParams(params); err != nil {
		return nil, err
	}

	var candidates []*Candidate
	count := params.Count
	if count <= 0 {
		count = 5
	}

	// 基于事件类型生成候选
	for i := 0; i < count && i < len(g.eventTypes); i++ {
		schema := g.generateEventSchema(g.eventTypes[i], params)
		if schema != nil {
			candidate := g.createCandidate(schema, params, fmt.Sprintf("event_study_%s", g.eventTypes[i]))
			candidates = append(candidates, candidate)
		}
	}

	return candidates, nil
}

func (g *EventStudyGenerator) generateEventSchema(eventType string, params GenerateParams) *ParadigmSchema {
	if params.SeedSchema == nil {
		return nil
	}

	schema := params.SeedSchema.DeepCopy()
	schema.ID = fmt.Sprintf("%s-%s", params.SeedSchema.ID, eventType)

	// 根据事件类型调整持有期和参数
	switch eventType {
	case "earnings":
		schema.HoldingPeriod = "short"
		schema.MaxDrawdown = 0.08
	case "merger":
		schema.HoldingPeriod = "short"
		schema.MaxDrawdown = 0.05
	case "product_launch":
		schema.HoldingPeriod = "medium"
		schema.MaxDrawdown = 0.10
	case "regulatory":
		schema.HoldingPeriod = "long"
		schema.MaxDrawdown = 0.15
	case "analyst_upgrade":
		schema.HoldingPeriod = "short"
		schema.MaxDrawdown = 0.06
	}

	// 添加上下文规则
	schema.ContextRules = append(schema.ContextRules, ContextRule{
		Key:    ContextSector,
		Values: []string{"technology", "finance", "consumer"},
	})

	return schema
}

func (g *EventStudyGenerator) createCandidate(schema *ParadigmSchema, params GenerateParams, reason string) *Candidate {
	now := time.Now()
	return &Candidate{
		ID:          fmt.Sprintf("cand-%s-%d", params.BatchID, now.UnixNano()),
		BatchID:     params.BatchID,
		Source:      SourceEventStudy,
		Schema:      schema,
		Status:      StatusQuarantine,
		Reason:      reason,
		SearchSpace: "event_study",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// AIGenerator AI 建议生成器
type AIGenerator struct{}

func NewAIGenerator() *AIGenerator {
	return &AIGenerator{}
}

func (g *AIGenerator) Source() CandidateSource {
	return SourceAI
}

func (g *AIGenerator) ValidateParams(params GenerateParams) error {
	if params.Count > 20 {
		return fmt.Errorf("AI generation count should be <= 20, got %d", params.Count)
	}
	return nil
}

func (g *AIGenerator) Generate(params GenerateParams) ([]*Candidate, error) {
	if err := g.ValidateParams(params); err != nil {
		return nil, err
	}

	var candidates []*Candidate
	count := params.Count
	if count <= 0 {
		count = 3
	}

	// AI 建议通常基于启发式规则
	for i := 0; i < count; i++ {
		schema := g.generateAISchema(i, params)
		if schema != nil {
			candidate := g.createCandidate(schema, params, fmt.Sprintf("ai_suggestion_%d", i))
			candidates = append(candidates, candidate)
		}
	}

	return candidates, nil
}

func (g *AIGenerator) generateAISchema(index int, params GenerateParams) *ParadigmSchema {
	if params.SeedSchema == nil {
		// 创建默认 Schema
		schema := NewParadigmSchema(
			fmt.Sprintf("ai-generated-%d", index),
			fmt.Sprintf("AI 生成范式 %d", index+1),
		)
		schema.HoldingPeriod = "medium"

		// 添加默认特征和规则
		schema.Features = []FeatureDefinition{
			{Name: "MA20", Type: "indicator", Description: "20日均线"},
			{Name: "RSI14", Type: "indicator", Description: "14日RSI"},
			{Name: "price.close", Type: "price", Description: "收盘价"},
		}

		// 根据 index 生成不同的规则组合
		switch index % 3 {
		case 0:
			schema.Rules = []Rule{
				{ID: "entry-1", Type: TypeEntry, Side: SideBuy, FeatureName: "RSI14", Operator: OpLessThan, Thresholds: []float64{30}, Required: true},
				{ID: "entry-2", Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: OpAbove, Thresholds: []float64{0}, Required: true},
				{ID: "exit-1", Type: TypeExitProfit, Side: SideSell, FeatureName: "price.close", Operator: OpGreaterThan, Thresholds: []float64{0.15}, Required: true},
				{ID: "exit-2", Type: TypeExitLoss, Side: SideSell, FeatureName: "RSI14", Operator: OpGreaterThan, Thresholds: []float64{70}, Required: true},
			}
		case 1:
			schema.Rules = []Rule{
				{ID: "entry-1", Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: OpCrossAbove, Thresholds: []float64{0}, Required: true},
				{ID: "entry-2", Type: TypeEntry, Side: SideBuy, FeatureName: "MA20", Operator: OpAbove, Thresholds: []float64{0}, Required: true},
				{ID: "exit-1", Type: TypeExitProfit, Side: SideSell, FeatureName: "price.close", Operator: OpGreaterThan, Thresholds: []float64{0.2}, Required: true},
				{ID: "exit-2", Type: TypeExitLoss, Side: SideSell, FeatureName: "price.close", Operator: OpLessThan, Thresholds: []float64{-0.05}, Required: true},
			}
		default:
			schema.Rules = []Rule{
				{ID: "entry-1", Type: TypeEntry, Side: SideBuy, FeatureName: "RSI14", Operator: OpBetween, Thresholds: []float64{30, 50}, Required: true},
				{ID: "entry-2", Type: TypeEntry, Side: SideBuy, FeatureName: "MA20", Operator: OpAbove, Thresholds: []float64{0}, Required: true},
				{ID: "exit-1", Type: TypeExitProfit, Side: SideSell, FeatureName: "RSI14", Operator: OpGreaterThan, Thresholds: []float64{70}, Required: true},
				{ID: "exit-2", Type: TypeExitLoss, Side: SideSell, FeatureName: "price.close", Operator: OpLessThan, Thresholds: []float64{-0.08}, Required: true},
			}
		}

		return schema
	}

	// 基于种子 Schema 生成变体
	schema := params.SeedSchema.DeepCopy()
	schema.ID = fmt.Sprintf("%s-ai-v%d", params.SeedSchema.ID, index+1)
	schema.Version = 1

	// 调整规则权重和阈值
	for j, rule := range schema.Rules {
		if rule.Type == TypeConfirmation {
			schema.Rules[j].Weight = rule.Weight * 0.8
		}
	}

	return schema
}

func (g *AIGenerator) createCandidate(schema *ParadigmSchema, params GenerateParams, reason string) *Candidate {
	now := time.Now()
	return &Candidate{
		ID:          fmt.Sprintf("cand-%s-%d", params.BatchID, now.UnixNano()),
		BatchID:     params.BatchID,
		Source:      SourceAI,
		Schema:      schema,
		Status:      StatusQuarantine,
		Reason:      reason,
		SearchSpace: "ai_suggestion",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// GeneratorRegistry 生成器注册表
type GeneratorRegistry struct {
	generators map[CandidateSource]CandidateGenerator
}

func NewGeneratorRegistry() *GeneratorRegistry {
	registry := &GeneratorRegistry{
		generators: make(map[CandidateSource]CandidateGenerator),
	}

	// 注册默认生成器
	registry.Register(NewManualGenerator())
	registry.Register(NewTemplateSearchGenerator())
	registry.Register(NewEventStudyGenerator())
	registry.Register(NewAIGenerator())

	return registry
}

// Register 注册生成器
func (r *GeneratorRegistry) Register(generator CandidateGenerator) {
	r.generators[generator.Source()] = generator
}

// Get 获取生成器
func (r *GeneratorRegistry) Get(source CandidateSource) (CandidateGenerator, error) {
	gen, ok := r.generators[source]
	if !ok {
		return nil, fmt.Errorf("generator not found for source: %s", source)
	}
	return gen, nil
}

// Generate 生成候选 (自动选择生成器)
func (r *GeneratorRegistry) Generate(params GenerateParams) ([]*Candidate, error) {
	// 自动生成 batch ID
	if params.BatchID == "" {
		params.BatchID = fmt.Sprintf("batch-%d", time.Now().UnixNano())
	}

	// 根据来源选择生成器
	source := params.Source
	if source == "" {
		source = SourceTemplate // 默认使用模板搜索
	}

	generator, err := r.Get(source)
	if err != nil {
		return nil, err
	}

	// 验证参数
	if err := generator.ValidateParams(params); err != nil {
		return nil, err
	}

	// 检查预算
	if params.SearchConfig != nil {
		if params.SearchConfig.SearchBudget > 0 && params.SearchConfig.UsedBudget >= params.SearchConfig.SearchBudget {
			return nil, fmt.Errorf("search budget exceeded: %d >= %d",
				params.SearchConfig.UsedBudget, params.SearchConfig.SearchBudget)
		}
	}

	// 生成候选
	candidates, err := generator.Generate(params)
	if err != nil {
		return nil, err
	}

	// 更新预算
	if params.SearchConfig != nil {
		params.SearchConfig.UsedBudget += len(candidates)
	}

	return candidates, nil
}

// ListSources 列出所有可用来源
func (r *GeneratorRegistry) ListSources() []CandidateSource {
	sources := make([]CandidateSource, 0, len(r.generators))
	for source := range r.generators {
		sources = append(sources, source)
	}
	return sources
}

// String 返回生成器描述
func (s CandidateSource) String() string {
	sources := map[CandidateSource]string{
		SourceManual:     "人工假设",
		SourceTemplate:   "模板参数搜索",
		SourceEventStudy: "事件研究",
		SourceAI:         "AI 建议",
		SourceMutation:   "变异",
	}
	if desc, ok := sources[s]; ok {
		return desc
	}
	return string(s)
}

// String 返回状态描述
func (s CandidateStatus) String() string {
	statuses := map[CandidateStatus]string{
		StatusQuarantine: "隔离区",
		StatusTesting:    "测试中",
		StatusValidated:  "已验证",
		StatusRejected:   "已拒绝",
		StatusPromoted:   "已晋级",
	}
	if desc, ok := statuses[s]; ok {
		return desc
	}
	return string(s)
}

// minInt 返回较小的整数
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// maxInt 返回较大的整数
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// FormatRules 格式化规则列表为字符串
func FormatRules(rules []Rule) string {
	var parts []string
	for _, r := range rules {
		threshold := fmt.Sprintf("%.2f", r.Thresholds[0])
		if len(r.Thresholds) > 1 {
			threshold = fmt.Sprintf("[%.2f, %.2f]", r.Thresholds[0], r.Thresholds[1])
		}
		parts = append(parts, fmt.Sprintf("%s:%s(%s)", r.FeatureName, r.Operator, threshold))
	}
	return strings.Join(parts, " AND ")
}
