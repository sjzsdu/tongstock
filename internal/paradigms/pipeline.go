package paradigms

import (
	"fmt"
	"time"
)

// ============================================================================
// 候选范式管线
// ============================================================================

// Pipeline 候选范式管线
type Pipeline struct {
	store       *CandidateStore
	registry    *GeneratorRegistry
	dedupSvc    *DeduplicationService
	experiments map[string]*BatchExperiment
}

// NewPipeline 创建管线
func NewPipeline(store *CandidateStore) *Pipeline {
	return &Pipeline{
		store:       store,
		registry:    NewGeneratorRegistry(),
		dedupSvc:    NewDeduplicationService(0.8),
		experiments: make(map[string]*BatchExperiment),
	}
}

// SetDeduplicationThreshold 设置去重阈值
func (p *Pipeline) SetDeduplicationThreshold(threshold float64) {
	p.dedupSvc = NewDeduplicationService(threshold)
}

// GenerateAndStore 生成并存储候选
func (p *Pipeline) GenerateAndStore(params GenerateParams) ([]*Candidate, error) {
	// 1. 生成候选
	candidates, err := p.registry.Generate(params)
	if err != nil {
		return nil, err
	}

	// 2. 去重
	candidates = p.dedupSvc.RemoveDuplicates(candidates)

	// 3. 存储候选
	for _, c := range candidates {
		if err := p.store.SaveCandidate(c); err != nil {
			return nil, fmt.Errorf("failed to save candidate %s: %w", c.ID, err)
		}
	}

	return candidates, nil
}

// ProcessQuarantine 处理隔离区的候选
func (p *Pipeline) ProcessQuarantine(processor CandidateProcessor) ([]*Candidate, error) {
	// 获取隔离区所有候选
	quarantine := p.store.GetQuarantine()
	if len(quarantine) == 0 {
		return nil, nil
	}

	// 处理每个候选
	var processed []*Candidate
	for _, c := range quarantine {
		// 标记为测试中
		c.MarkTesting()
		p.store.UpdateCandidateState(c.ID, StatusTesting)

		// 处理候选
		result, err := processor.Process(c)
		if err != nil {
			c.MarkRejected(err.Error())
			p.store.UpdateCandidateState(c.ID, StatusRejected)
			continue
		}

		// 标记为已验证
		c.MarkValidated(result)
		p.store.UpdateCandidateState(c.ID, StatusValidated)
		processed = append(processed, c)
	}

	return processed, nil
}

// RunBatchExperiment 运行批量实验
func (p *Pipeline) RunBatchExperiment(name string, candidateIDs []string, config *BatchConfig) (*BatchExperiment, error) {
	experiment := NewBatchExperiment(name, candidateIDs, config)
	p.experiments[experiment.ID] = experiment

	experiment.Start()
	return experiment, nil
}

// GetExperiment 获取实验
func (p *Pipeline) GetExperiment(id string) (*BatchExperiment, error) {
	exp, ok := p.experiments[id]
	if !ok {
		return nil, fmt.Errorf("experiment not found: %s", id)
	}
	return exp, nil
}

// PromoteCandidate 晋级候选为范式
func (p *Pipeline) PromoteCandidate(candidateID string, paradigmStore *SchemaStore) error {
	candidate, err := p.store.GetCandidate(candidateID)
	if err != nil {
		return err
	}

	if candidate.Status != StatusValidated {
		return fmt.Errorf("candidate %s is not validated (status: %s)", candidateID, candidate.Status)
	}

	// 保存为新范式 (如果提供了存储)
	if paradigmStore != nil {
		if err := paradigmStore.Save(candidate.Schema); err != nil {
			return fmt.Errorf("failed to promote candidate: %w", err)
		}
	}

	// 更新候选状态
	candidate.MarkPromoted()
	p.store.UpdateCandidateState(candidateID, StatusPromoted)

	return nil
}

// GetCandidatesBySource 按来源获取候选
func (p *Pipeline) GetCandidatesBySource(source CandidateSource) []*Candidate {
	return p.store.GetBySource(source)
}

// GetCandidatesByStatus 按状态获取候选
func (p *Pipeline) GetCandidatesByStatus(status CandidateStatus) []*Candidate {
	return p.store.GetByStatus(status)
}

// GetQuarantine 获取隔离区候选
func (p *Pipeline) GetQuarantine() []*Candidate {
	return p.store.GetQuarantine()
}

// GetStatistics 获取统计信息
func (p *Pipeline) GetStatistics() PipelineStats {
	return PipelineStats{
		TotalCandidates:    p.store.TotalCandidates(),
		QuarantineSize:     p.store.QuarantineSize(),
		SourceDistribution: p.store.SourceDistribution(),
		StatusDistribution: p.store.StatusDistribution(),
		ExperimentCount:    len(p.experiments),
	}
}

// PipelineStats 管线统计
type PipelineStats struct {
	TotalCandidates    int                     `json:"total_candidates"`
	QuarantineSize     int                     `json:"quarantine_size"`
	SourceDistribution map[CandidateSource]int `json:"source_distribution"`
	StatusDistribution map[CandidateStatus]int `json:"status_distribution"`
	ExperimentCount    int                     `json:"experiment_count"`
}

// String 返回统计摘要
func (s PipelineStats) String() string {
	return fmt.Sprintf("候选总数: %d, 隔离区: %d, 实验数: %d",
		s.TotalCandidates,
		s.QuarantineSize,
		s.ExperimentCount)
}

// CandidateProcessor 候选处理器接口
type CandidateProcessor interface {
	Process(candidate *Candidate) (*TestResult, error)
}

// SimpleProcessor 简单处理器 (用于测试)
type SimpleProcessor struct {
	successRate float64 // 成功率 (0-1)
}

func NewSimpleProcessor(successRate float64) *SimpleProcessor {
	if successRate < 0 {
		successRate = 0
	}
	if successRate > 1 {
		successRate = 1
	}
	return &SimpleProcessor{successRate: successRate}
}

func (p *SimpleProcessor) Process(candidate *Candidate) (*TestResult, error) {
	now := time.Now()

	// 模拟回测结果
	return &TestResult{
		BacktestResult: &BacktestSummary{
			TotalReturn: 0.15 * p.successRate,
			SharpeRatio: 1.5 * p.successRate,
			MaxDrawdown: 0.10,
			WinRate:     0.55,
			TradesCount: 20,
			SampleSize:  252,
			Confidence:  p.successRate,
		},
		CrossValidation: &CrossValidationResult{
			MeanReturn:     0.12 * p.successRate,
			StdReturn:      0.05,
			WorstReturn:    0.05 * p.successRate,
			StabilityScore: p.successRate * 0.8,
			OverfitRisk:    1.0 - p.successRate,
			Folds:          5,
		},
		CheckedAt: now,
	}, nil
}

// PipelineBuilder 管线构建器 (链式调用)
type PipelineBuilder struct {
	pipeline *Pipeline
	params   GenerateParams
}

// NewPipelineBuilder 创建构建器
func NewPipelineBuilder(store *CandidateStore) *PipelineBuilder {
	return &PipelineBuilder{
		pipeline: NewPipeline(store),
		params: GenerateParams{
			Count: 10,
			SearchConfig: &SearchConfig{
				MaxRules:      10,
				MinConfidence: 0.5,
				SearchBudget:  100,
				UsedBudget:    0,
			},
		},
	}
}

// WithBatchID 设置批次 ID
func (b *PipelineBuilder) WithBatchID(batchID string) *PipelineBuilder {
	b.params.BatchID = batchID
	return b
}

// WithSource 设置来源
func (b *PipelineBuilder) WithSource(source CandidateSource) *PipelineBuilder {
	b.params.Source = source
	return b
}

// WithCount 设置生成数量
func (b *PipelineBuilder) WithCount(count int) *PipelineBuilder {
	b.params.Count = count
	if b.params.SearchConfig != nil {
		b.params.SearchConfig.SearchBudget = count * 10
	}
	return b
}

// WithSchema 设置种子 Schema
func (b *PipelineBuilder) WithSchema(schema *ParadigmSchema) *PipelineBuilder {
	b.params.SeedSchema = schema
	return b
}

// WithSearchConfig 设置搜索配置
func (b *PipelineBuilder) WithSearchConfig(config *SearchConfig) *PipelineBuilder {
	b.params.SearchConfig = config
	return b
}

// WithDedupThreshold 设置去重阈值
func (b *PipelineBuilder) WithDedupThreshold(threshold float64) *PipelineBuilder {
	b.pipeline.SetDeduplicationThreshold(threshold)
	return b
}

// Build 构建管线并生成候选
func (b *PipelineBuilder) Build() (*Pipeline, []*Candidate, error) {
	candidates, err := b.pipeline.GenerateAndStore(b.params)
	if err != nil {
		return nil, nil, err
	}
	return b.pipeline, candidates, nil
}

// BuildWithoutGenerate 仅构建管线 (不生成)
func (b *PipelineBuilder) BuildWithoutGenerate() *Pipeline {
	return b.pipeline
}

// ============================================================================
// 种子 Schema 工厂
// ============================================================================

// CreateDefaultSeedSchemas 创建默认的种子 Schema
func CreateDefaultSeedSchemas() []*ParadigmSchema {
	return []*ParadigmSchema{
		CreateMeanReversionSchema(),
		CreateMomentumSchema(),
		CreateBreakoutSchema(),
	}
}

// CreateMeanReversionSchema 创建均值回归范式
func CreateMeanReversionSchema() *ParadigmSchema {
	schema := NewParadigmSchema("mean-reversion", "均值回归范式")
	schema.HoldingPeriod = "medium"
	schema.MaxDrawdown = 0.12
	schema.Description = "基于超跌反弹的均值回归策略"

	schema.Features = []FeatureDefinition{
		{Name: "RSI14", Type: "indicator", Description: "14日RSI"},
		{Name: "BollingerBand", Type: "indicator", Description: "布林带"},
		{Name: "price.close", Type: "price", Description: "收盘价"},
	}

	schema.Rules = []Rule{
		{ID: "entry-1", Type: TypeEntry, Side: SideBuy, FeatureName: "RSI14", Operator: OpLessThan, Thresholds: []float64{30}, Required: true},
		{ID: "entry-2", Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: OpBelow, Thresholds: []float64{0}, Required: true},
		{ID: "exit-1", Type: TypeExitProfit, Side: SideSell, FeatureName: "RSI14", Operator: OpGreaterThan, Thresholds: []float64{70}, Required: true},
		{ID: "exit-2", Type: TypeExitLoss, Side: SideSell, FeatureName: "price.close", Operator: OpLessThan, Thresholds: []float64{-0.08}, Required: true},
	}

	return schema
}

// CreateMomentumSchema 创建动量范式
func CreateMomentumSchema() *ParadigmSchema {
	schema := NewParadigmSchema("momentum", "动量范式")
	schema.HoldingPeriod = "short"
	schema.MaxDrawdown = 0.15
	schema.Description = "基于趋势跟随的动量策略"

	schema.Features = []FeatureDefinition{
		{Name: "MA20", Type: "indicator", Description: "20日均线"},
		{Name: "MA60", Type: "indicator", Description: "60日均线"},
		{Name: "price.volume", Type: "volume", Description: "成交量"},
		{Name: "price.close", Type: "price", Description: "收盘价"},
	}

	schema.Rules = []Rule{
		{ID: "entry-1", Type: TypeEntry, Side: SideBuy, FeatureName: "MA20", Operator: OpCrossAbove, Thresholds: []float64{0}, Required: true},
		{ID: "entry-2", Type: TypeEntry, Side: SideBuy, FeatureName: "MA60", Operator: OpAbove, Thresholds: []float64{0}, Required: true},
		{ID: "entry-3", Type: TypeEntry, Side: SideBuy, FeatureName: "price.volume", Operator: OpGreaterThan, Thresholds: []float64{1.5}, Required: false},
		{ID: "exit-1", Type: TypeExitProfit, Side: SideSell, FeatureName: "MA20", Operator: OpCrossBelow, Thresholds: []float64{0}, Required: true},
		{ID: "exit-2", Type: TypeExitLoss, Side: SideSell, FeatureName: "price.close", Operator: OpLessThan, Thresholds: []float64{-0.1}, Required: true},
	}

	return schema
}

// CreateBreakoutSchema 创建突破范式
func CreateBreakoutSchema() *ParadigmSchema {
	schema := NewParadigmSchema("breakout", "突破范式")
	schema.HoldingPeriod = "medium"
	schema.MaxDrawdown = 0.20
	schema.Description = "基于关键价位突破的策略"

	schema.Features = []FeatureDefinition{
		{Name: "price.high", Type: "price", Description: "最高价"},
		{Name: "price.low", Type: "price", Description: "最低价"},
		{Name: "ATR", Type: "indicator", Description: "平均真实波幅"},
		{Name: "price.close", Type: "price", Description: "收盘价"},
	}

	schema.Rules = []Rule{
		{ID: "entry-1", Type: TypeEntry, Side: SideBuy, FeatureName: "price.high", Operator: OpGreaterThan, Thresholds: []float64{2.0}, Required: true},
		{ID: "entry-2", Type: TypeEntry, Side: SideBuy, FeatureName: "ATR", Operator: OpGreaterThan, Thresholds: []float64{0}, Required: false},
		{ID: "exit-1", Type: TypeExitProfit, Side: SideSell, FeatureName: "price.close", Operator: OpGreaterThan, Thresholds: []float64{0.15}, Required: true},
		{ID: "exit-2", Type: TypeExitLoss, Side: SideSell, FeatureName: "price.close", Operator: OpLessThan, Thresholds: []float64{-0.1}, Required: true},
	}

	return schema
}
