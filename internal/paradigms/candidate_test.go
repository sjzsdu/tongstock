package paradigms

import (
	"testing"
	"time"
)

// ============================================================================
// 候选存储测试
// ============================================================================

func TestNewCandidateStore(t *testing.T) {
	store := NewCandidateStore()
	if store == nil {
		t.Fatal("NewCandidateStore returned nil")
	}
	if store.TotalCandidates() != 0 {
		t.Errorf("expected 0 candidates, got %d", store.TotalCandidates())
	}
	if store.QuarantineSize() != 0 {
		t.Errorf("expected 0 quarantine, got %d", store.QuarantineSize())
	}
}

func TestSaveCandidate(t *testing.T) {
	store := NewCandidateStore()

	candidate := createTestCandidate("test-1", SourceManual)
	err := store.SaveCandidate(candidate)
	if err != nil {
		t.Fatalf("SaveCandidate failed: %v", err)
	}

	if store.TotalCandidates() != 1 {
		t.Errorf("expected 1 candidate, got %d", store.TotalCandidates())
	}
	if store.QuarantineSize() != 1 {
		t.Errorf("expected 1 quarantine, got %d", store.QuarantineSize())
	}
}

func TestSaveCandidateWithoutID(t *testing.T) {
	store := NewCandidateStore()

	candidate := &Candidate{
		BatchID: "batch-1",
		Source:  SourceManual,
		Status:  StatusQuarantine,
	}

	err := store.SaveCandidate(candidate)
	if err == nil {
		t.Fatal("expected error for candidate without ID")
	}
}

func TestGetCandidate(t *testing.T) {
	store := NewCandidateStore()

	candidate := createTestCandidate("test-1", SourceManual)
	store.SaveCandidate(candidate)

	got, err := store.GetCandidate("test-1")
	if err != nil {
		t.Fatalf("GetCandidate failed: %v", err)
	}
	if got.ID != "test-1" {
		t.Errorf("expected ID test-1, got %s", got.ID)
	}
}

func TestGetCandidateNotFound(t *testing.T) {
	store := NewCandidateStore()

	_, err := store.GetCandidate("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent candidate")
	}
}

func TestGetBatch(t *testing.T) {
	store := NewCandidateStore()

	for i := 0; i < 3; i++ {
		candidate := createTestCandidate("batch-1-cand-"+string(rune('a'+i)), SourceTemplate)
		candidate.BatchID = "batch-1"
		store.SaveCandidate(candidate)
	}

	batch, err := store.GetBatch("batch-1")
	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}
	if len(batch) != 3 {
		t.Errorf("expected 3 candidates in batch, got %d", len(batch))
	}
}

func TestGetBatchNotFound(t *testing.T) {
	store := NewCandidateStore()

	_, err := store.GetBatch("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent batch")
	}
}

func TestGetQuarantine(t *testing.T) {
	store := NewCandidateStore()

	for i := 0; i < 2; i++ {
		candidate := createTestCandidate("q-cand-"+string(rune('a'+i)), SourceManual)
		store.SaveCandidate(candidate)
	}

	quarantine := store.GetQuarantine()
	if len(quarantine) != 2 {
		t.Errorf("expected 2 quarantine candidates, got %d", len(quarantine))
	}
}

func TestUpdateCandidateState(t *testing.T) {
	store := NewCandidateStore()

	candidate := createTestCandidate("test-1", SourceManual)
	store.SaveCandidate(candidate)

	err := store.UpdateCandidateState("test-1", StatusTesting)
	if err != nil {
		t.Fatalf("UpdateCandidateState failed: %v", err)
	}

	// Should no longer be in quarantine
	if store.QuarantineSize() != 0 {
		t.Errorf("expected 0 quarantine after state update, got %d", store.QuarantineSize())
	}
}

func TestUpdateCandidateStateNotFound(t *testing.T) {
	store := NewCandidateStore()

	err := store.UpdateCandidateState("nonexistent", StatusTesting)
	if err == nil {
		t.Fatal("expected error for nonexistent candidate")
	}
}

func TestGetBySource(t *testing.T) {
	store := NewCandidateStore()

	store.SaveCandidate(createTestCandidate("manual-1", SourceManual))
	store.SaveCandidate(createTestCandidate("template-1", SourceTemplate))
	store.SaveCandidate(createTestCandidate("manual-2", SourceManual))

	manual := store.GetBySource(SourceManual)
	if len(manual) != 2 {
		t.Errorf("expected 2 manual candidates, got %d", len(manual))
	}

	template := store.GetBySource(SourceTemplate)
	if len(template) != 1 {
		t.Errorf("expected 1 template candidate, got %d", len(template))
	}
}

func TestGetByStatus(t *testing.T) {
	store := NewCandidateStore()

	store.SaveCandidate(createTestCandidate("q-1", SourceManual))

	candidate := createTestCandidate("t-1", SourceManual)
	candidate.Status = StatusTesting
	store.SaveCandidate(candidate)

	quarantine := store.GetByStatus(StatusQuarantine)
	if len(quarantine) != 1 {
		t.Errorf("expected 1 quarantine candidate, got %d", len(quarantine))
	}

	testing := store.GetByStatus(StatusTesting)
	if len(testing) != 1 {
		t.Errorf("expected 1 testing candidate, got %d", len(testing))
	}
}

func TestRemoveCandidate(t *testing.T) {
	store := NewCandidateStore()

	candidate := createTestCandidate("test-1", SourceManual)
	store.SaveCandidate(candidate)

	err := store.RemoveCandidate("test-1")
	if err != nil {
		t.Fatalf("RemoveCandidate failed: %v", err)
	}

	if store.TotalCandidates() != 0 {
		t.Errorf("expected 0 candidates after removal, got %d", store.TotalCandidates())
	}
}

func TestRemoveCandidateNotFound(t *testing.T) {
	store := NewCandidateStore()

	err := store.RemoveCandidate("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent candidate")
	}
}

func TestSourceDistribution(t *testing.T) {
	store := NewCandidateStore()

	store.SaveCandidate(createTestCandidate("m-1", SourceManual))
	store.SaveCandidate(createTestCandidate("m-2", SourceManual))
	store.SaveCandidate(createTestCandidate("t-1", SourceTemplate))

	dist := store.SourceDistribution()
	if dist[SourceManual] != 2 {
		t.Errorf("expected 2 manual candidates, got %d", dist[SourceManual])
	}
	if dist[SourceTemplate] != 1 {
		t.Errorf("expected 1 template candidate, got %d", dist[SourceTemplate])
	}
}

func TestStatusDistribution(t *testing.T) {
	store := NewCandidateStore()

	store.SaveCandidate(createTestCandidate("q-1", SourceManual))

	candidate := createTestCandidate("v-1", SourceManual)
	candidate.Status = StatusValidated
	store.SaveCandidate(candidate)

	dist := store.StatusDistribution()
	if dist[StatusQuarantine] != 1 {
		t.Errorf("expected 1 quarantine, got %d", dist[StatusQuarantine])
	}
	if dist[StatusValidated] != 1 {
		t.Errorf("expected 1 validated, got %d", dist[StatusValidated])
	}
}

func TestClearQuarantine(t *testing.T) {
	store := NewCandidateStore()

	store.SaveCandidate(createTestCandidate("q-1", SourceManual))
	store.SaveCandidate(createTestCandidate("q-2", SourceManual))

	store.ClearQuarantine()

	if store.QuarantineSize() != 0 {
		t.Errorf("expected 0 quarantine after clear, got %d", store.QuarantineSize())
	}

	rejected := store.GetByStatus(StatusRejected)
	if len(rejected) != 2 {
		t.Errorf("expected 2 rejected after clear, got %d", len(rejected))
	}
}

// ============================================================================
// 生成器测试
// ============================================================================

func TestNewGeneratorRegistry(t *testing.T) {
	registry := NewGeneratorRegistry()
	if registry == nil {
		t.Fatal("NewGeneratorRegistry returned nil")
	}

	sources := registry.ListSources()
	if len(sources) < 3 {
		t.Errorf("expected at least 3 sources, got %d", len(sources))
	}
}

func TestManualGenerator(t *testing.T) {
	registry := NewGeneratorRegistry()

	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Source:     SourceManual,
		Count:      1,
		SeedSchema: schema,
	}

	candidates, err := registry.Generate(params)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(candidates))
	}

	if candidates[0].Source != SourceManual {
		t.Errorf("expected source manual, got %s", candidates[0].Source)
	}
}

func TestManualGeneratorWithoutSchema(t *testing.T) {
	registry := NewGeneratorRegistry()

	params := GenerateParams{
		BatchID: "batch-test",
		Source:  SourceManual,
		Count:   1,
	}

	_, err := registry.Generate(params)
	if err == nil {
		t.Fatal("expected error without seed schema")
	}
}

func TestTemplateSearchGenerator(t *testing.T) {
	registry := NewGeneratorRegistry()

	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Source:     SourceTemplate,
		Count:      5,
		SeedSchema: schema,
		SearchConfig: &SearchConfig{
			SearchBudget: 100,
		},
	}

	candidates, err := registry.Generate(params)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(candidates) != 5 {
		t.Errorf("expected 5 candidates, got %d", len(candidates))
	}

	if candidates[0].Source != SourceTemplate {
		t.Errorf("expected source template, got %s", candidates[0].Source)
	}
}

func TestTemplateSearchGeneratorWithoutConfig(t *testing.T) {
	registry := NewGeneratorRegistry()

	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Source:     SourceTemplate,
		Count:      5,
		SeedSchema: schema,
	}

	_, err := registry.Generate(params)
	if err == nil {
		t.Fatal("expected error without search config")
	}
}

func TestTemplateSearchGeneratorBudgetExceeded(t *testing.T) {
	registry := NewGeneratorRegistry()

	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Source:     SourceTemplate,
		Count:      5,
		SeedSchema: schema,
		SearchConfig: &SearchConfig{
			SearchBudget: 5,
			UsedBudget:   5,
		},
	}

	_, err := registry.Generate(params)
	if err == nil {
		t.Fatal("expected error for exceeded budget")
	}
}

func TestEventStudyGenerator(t *testing.T) {
	registry := NewGeneratorRegistry()

	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Source:     SourceEventStudy,
		Count:      3,
		SeedSchema: schema,
	}

	candidates, err := registry.Generate(params)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(candidates) != 3 {
		t.Errorf("expected 3 candidates, got %d", len(candidates))
	}
}

func TestAIGenerator(t *testing.T) {
	registry := NewGeneratorRegistry()

	params := GenerateParams{
		BatchID: "batch-test",
		Source:  SourceAI,
		Count:   3,
	}

	candidates, err := registry.Generate(params)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(candidates) != 3 {
		t.Errorf("expected 3 candidates, got %d", len(candidates))
	}
}

func TestDefaultSource(t *testing.T) {
	registry := NewGeneratorRegistry()

	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Count:      1,
		SeedSchema: schema,
		SearchConfig: &SearchConfig{
			SearchBudget: 100,
		},
	}

	// 不指定 source, 应该默认使用 template
	candidates, err := registry.Generate(params)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if candidates[0].Source != SourceTemplate {
		t.Errorf("expected default source template, got %s", candidates[0].Source)
	}
}

// ============================================================================
// 去重服务测试
// ============================================================================

func TestNewDeduplicationService(t *testing.T) {
	ds := NewDeduplicationService(0.8)
	if ds == nil {
		t.Fatal("NewDeduplicationService returned nil")
	}
	if ds.threshold != 0.8 {
		t.Errorf("expected threshold 0.8, got %f", ds.threshold)
	}
}

func TestDeduplicationServiceDefaultThreshold(t *testing.T) {
	ds := NewDeduplicationService(0)
	if ds.threshold != 0.8 {
		t.Errorf("expected default threshold 0.8, got %f", ds.threshold)
	}
}

func TestCalculateSimilarityIdentical(t *testing.T) {
	ds := NewDeduplicationService(0.8)

	schema := CreateMeanReversionSchema()
	similarity := ds.CalculateSimilarity(schema, schema)

	if similarity < 0.9 {
		t.Errorf("expected high similarity for identical schemas, got %f", similarity)
	}
}

func TestCalculateSimilarityDifferent(t *testing.T) {
	ds := NewDeduplicationService(0.8)

	schema1 := CreateMeanReversionSchema()
	schema2 := CreateMomentumSchema()
	similarity := ds.CalculateSimilarity(schema1, schema2)

	if similarity > 0.8 {
		t.Errorf("expected low similarity for different schemas, got %f", similarity)
	}
}

func TestIsDuplicate(t *testing.T) {
	ds := NewDeduplicationService(0.8)

	schema := CreateMeanReversionSchema()
	schemaCopy := schema.DeepCopy()

	if !ds.IsDuplicate(schema, schemaCopy) {
		t.Error("deep copy should be considered duplicate")
	}
}

func TestIsNotDuplicate(t *testing.T) {
	ds := NewDeduplicationService(0.8)

	schema1 := CreateMeanReversionSchema()
	schema2 := CreateMomentumSchema()

	if ds.IsDuplicate(schema1, schema2) {
		t.Error("different schemas should not be duplicates")
	}
}

func TestRemoveDuplicates(t *testing.T) {
	ds := NewDeduplicationService(0.8)

	candidates := []*Candidate{
		createTestCandidate("c1", SourceManual),
		createTestCandidate("c2", SourceManual),
		createTestCandidate("c3-different", SourceManual),
	}

	// Make c2 a duplicate of c1 (deep copy)
	candidates[1].Schema = candidates[0].Schema.DeepCopy()

	// Make c3 have a different schema
	candidates[2].Schema.ID = "different-schema"
	candidates[2].Schema.Rules = []Rule{
		{ID: "different-rule", Type: TypeEntry, Side: SideBuy, FeatureName: "RSI14", Operator: OpLessThan, Thresholds: []float64{30}},
	}

	result := ds.RemoveDuplicates(candidates)
	if len(result) != 2 {
		t.Errorf("expected 2 after dedup, got %d", len(result))
	}
}

func TestFindDuplicates(t *testing.T) {
	ds := NewDeduplicationService(0.8)

	candidates := []*Candidate{
		createTestCandidate("c1", SourceManual),
		createTestCandidate("c2", SourceManual),
		createTestCandidate("c3-different", SourceManual),
	}

	// Make c1 and c2 duplicates
	candidates[1].Schema = candidates[0].Schema.DeepCopy()

	// Make c3 have a different schema
	candidates[2].Schema.ID = "different-schema"
	candidates[2].Schema.Rules = []Rule{
		{ID: "different-rule", Type: TypeEntry, Side: SideBuy, FeatureName: "RSI14", Operator: OpLessThan, Thresholds: []float64{30}},
	}

	groups := ds.FindDuplicates(candidates)
	if len(groups) != 1 {
		t.Errorf("expected 1 duplicate group, got %d", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Errorf("expected 2 in group, got %d", len(groups[0]))
	}
}

func TestClusterBySimilarity(t *testing.T) {
	ds := NewDeduplicationService(0.8)

	candidates := []*Candidate{
		createTestCandidate("c1", SourceManual),
		createTestCandidate("c2", SourceManual),
		createTestCandidate("c3", SourceManual),
	}

	// Make c1 and c2 similar
	candidates[1].Schema = candidates[0].Schema.DeepCopy()

	clusters := ds.ClusterBySimilarity(candidates, 0.5)
	if len(clusters) < 1 {
		t.Error("expected at least 1 cluster")
	}
}

// ============================================================================
// 批量实验测试
// ============================================================================

func TestNewBatchExperiment(t *testing.T) {
	config := &BatchConfig{
		MaxConcurrent: 5,
		MinScore:      0.5,
	}

	exp := NewBatchExperiment("test-exp", []string{"c1", "c2", "c3"}, config)
	if exp == nil {
		t.Fatal("NewBatchExperiment returned nil")
	}
	if exp.Status != "pending" {
		t.Errorf("expected pending status, got %s", exp.Status)
	}
	if len(exp.CandidateIDs) != 3 {
		t.Errorf("expected 3 candidate IDs, got %d", len(exp.CandidateIDs))
	}
}

func TestNewBatchExperimentDefaultConfig(t *testing.T) {
	exp := NewBatchExperiment("test-exp", []string{"c1"}, nil)
	if exp.Config == nil {
		t.Fatal("expected default config")
	}
	if exp.Config.MaxConcurrent != 5 {
		t.Errorf("expected default MaxConcurrent 5, got %d", exp.Config.MaxConcurrent)
	}
}

func TestBatchExperimentStart(t *testing.T) {
	exp := NewBatchExperiment("test-exp", []string{"c1"}, nil)
	exp.Start()
	if exp.Status != "running" {
		t.Errorf("expected running status, got %s", exp.Status)
	}
}

func TestBatchExperimentComplete(t *testing.T) {
	exp := NewBatchExperiment("test-exp", []string{"c1"}, nil)
	exp.Start()
	exp.Complete()
	if exp.Status != "completed" {
		t.Errorf("expected completed status, got %s", exp.Status)
	}
	if exp.IsComplete() != true {
		t.Error("IsComplete should be true after completion")
	}
}

func TestBatchExperimentProgress(t *testing.T) {
	exp := NewBatchExperiment("test-exp", []string{"c1", "c2", "c3"}, nil)

	if exp.Progress() != 0.0 {
		t.Errorf("expected 0.0 progress, got %f", exp.Progress())
	}

	exp.AddResult(&TestResult{})
	if exp.Progress() != 1.0/3.0 {
		t.Errorf("expected 1/3 progress, got %f", exp.Progress())
	}

	exp.AddResult(&TestResult{})
	exp.AddResult(&TestResult{})
	if exp.Progress() != 1.0 {
		t.Errorf("expected 1.0 progress, got %f", exp.Progress())
	}
}

func TestBatchExperimentSummary(t *testing.T) {
	exp := NewBatchExperiment("test-exp", []string{"c1", "c2"}, nil)
	exp.Start()
	exp.AddResult(&TestResult{
		BacktestResult: &BacktestSummary{Confidence: 0.8},
	})
	exp.Complete()

	summary := exp.Summary()
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

// ============================================================================
// 管线测试
// ============================================================================

func TestNewPipeline(t *testing.T) {
	store := NewCandidateStore()
	pipeline := NewPipeline(store)
	if pipeline == nil {
		t.Fatal("NewPipeline returned nil")
	}
}

func TestGenerateAndStore(t *testing.T) {
	store := NewCandidateStore()
	pipeline := NewPipeline(store)

	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Source:     SourceTemplate,
		Count:      5,
		SeedSchema: schema,
		SearchConfig: &SearchConfig{
			SearchBudget: 100,
		},
	}

	candidates, err := pipeline.GenerateAndStore(params)
	if err != nil {
		t.Fatalf("GenerateAndStore failed: %v", err)
	}

	// 去重后可能少于 5 个候选
	if len(candidates) == 0 {
		t.Error("expected at least 1 candidate")
	}

	if store.TotalCandidates() == 0 {
		t.Error("expected at least 1 candidate in store")
	}
}

func TestProcessQuarantine(t *testing.T) {
	store := NewCandidateStore()
	pipeline := NewPipeline(store)

	// Generate candidates (TemplateSearchGenerator supports count > 1)
	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Source:     SourceTemplate,
		Count:      5,
		SeedSchema: schema,
		SearchConfig: &SearchConfig{
			SearchBudget: 100,
		},
	}

	pipeline.GenerateAndStore(params)

	// Process quarantine
	processor := NewSimpleProcessor(0.8)
	processed, err := pipeline.ProcessQuarantine(processor)
	if err != nil {
		t.Fatalf("ProcessQuarantine failed: %v", err)
	}

	if len(processed) == 0 {
		t.Error("expected at least 1 processed candidate")
	}

	validated := store.GetByStatus(StatusValidated)
	if len(validated) == 0 {
		t.Error("expected at least 1 validated candidate")
	}
}

func TestGetStatistics(t *testing.T) {
	store := NewCandidateStore()
	pipeline := NewPipeline(store)

	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Source:     SourceTemplate,
		Count:      5,
		SeedSchema: schema,
		SearchConfig: &SearchConfig{
			SearchBudget: 100,
		},
	}

	pipeline.GenerateAndStore(params)

	stats := pipeline.GetStatistics()
	if stats.TotalCandidates == 0 {
		t.Error("expected non-zero total candidates")
	}
	if stats.QuarantineSize == 0 {
		t.Error("expected non-zero quarantine size")
	}
}

func TestPipelineBuilder(t *testing.T) {
	store := NewCandidateStore()

	builder := NewPipelineBuilder(store).
		WithBatchID("batch-builder").
		WithSource(SourceTemplate).
		WithCount(5).
		WithSchema(CreateMeanReversionSchema())

	pipeline, candidates, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if pipeline == nil {
		t.Fatal("expected non-nil pipeline")
	}
	if len(candidates) == 0 {
		t.Error("expected at least 1 candidate")
	}
}

func TestPromoteCandidate(t *testing.T) {
	// Use nil SchemaStore to skip persistence
	store := NewCandidateStore()
	pipeline := NewPipeline(store)

	// Generate and process
	schema := CreateMeanReversionSchema()
	params := GenerateParams{
		BatchID:    "batch-test",
		Source:     SourceManual,
		Count:      1,
		SeedSchema: schema,
	}

	candidates, err := pipeline.GenerateAndStore(params)
	if err != nil {
		t.Fatalf("GenerateAndStore failed: %v", err)
	}

	// Process to validated
	processor := NewSimpleProcessor(0.9)
	pipeline.ProcessQuarantine(processor)

	// Promote with nil SchemaStore (skip persistence)
	err = pipeline.PromoteCandidate(candidates[0].ID, nil)
	if err != nil {
		t.Fatalf("PromoteCandidate failed: %v", err)
	}

	// Verify promoted
	promoted := store.GetByStatus(StatusPromoted)
	if len(promoted) != 1 {
		t.Errorf("expected 1 promoted, got %d", len(promoted))
	}
}

func TestPromoteCandidateNotValidated(t *testing.T) {
	store := NewCandidateStore()
	pipeline := NewPipeline(store)

	candidate := createTestCandidate("test-1", SourceManual)
	store.SaveCandidate(candidate)

	// Promote with nil SchemaStore
	err := pipeline.PromoteCandidate("test-1", nil)
	if err == nil {
		t.Fatal("expected error for non-validated candidate")
	}
}

// ============================================================================
// 种子 Schema 工厂测试
// ============================================================================

func TestCreateDefaultSeedSchemas(t *testing.T) {
	schemas := CreateDefaultSeedSchemas()
	if len(schemas) < 3 {
		t.Errorf("expected at least 3 seed schemas, got %d", len(schemas))
	}

	for _, s := range schemas {
		if err := s.IsValid(); err != nil {
			t.Errorf("schema %s should be valid: %v", s.ID, err)
		}
	}
}

func TestCreateMeanReversionSchema(t *testing.T) {
	schema := CreateMeanReversionSchema()
	if schema.ID != "mean-reversion" {
		t.Errorf("expected ID mean-reversion, got %s", schema.ID)
	}
	if len(schema.Features) == 0 {
		t.Error("expected features")
	}
	if len(schema.Rules) == 0 {
		t.Error("expected rules")
	}
}

func TestCreateMomentumSchema(t *testing.T) {
	schema := CreateMomentumSchema()
	if schema.ID != "momentum" {
		t.Errorf("expected ID momentum, got %s", schema.ID)
	}
	if schema.HoldingPeriod != "short" {
		t.Errorf("expected holding period short, got %s", schema.HoldingPeriod)
	}
}

func TestCreateBreakoutSchema(t *testing.T) {
	schema := CreateBreakoutSchema()
	if schema.ID != "breakout" {
		t.Errorf("expected ID breakout, got %s", schema.ID)
	}
	if len(schema.Features) != 4 {
		t.Errorf("expected 4 features (including price.close), got %d", len(schema.Features))
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

func createTestCandidate(id string, source CandidateSource) *Candidate {
	schema := NewParadigmSchema("test-schema", "测试范式")
	schema.HoldingPeriod = "medium"
	schema.Features = []FeatureDefinition{
		{Name: "MA20", Type: "indicator"},
	}
	schema.Rules = []Rule{
		{ID: "entry-1", Type: TypeEntry, Side: SideBuy, FeatureName: "MA20", Operator: OpGreaterThan, Thresholds: []float64{0}},
	}

	return &Candidate{
		ID:        id,
		BatchID:   "test-batch",
		Source:    source,
		Schema:    schema,
		Status:    StatusQuarantine,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// ============================================================================
// 字符串转换测试
// ============================================================================

func TestCandidateSourceString(t *testing.T) {
	tests := []struct {
		source   CandidateSource
		expected string
	}{
		{SourceManual, "人工假设"},
		{SourceTemplate, "模板参数搜索"},
		{SourceEventStudy, "事件研究"},
		{SourceAI, "AI 建议"},
		{SourceMutation, "变异"},
	}

	for _, tt := range tests {
		if got := tt.source.String(); got != tt.expected {
			t.Errorf("CandidateSource(%s).String() = %s, want %s", tt.source, got, tt.expected)
		}
	}
}

func TestCandidateStatusString(t *testing.T) {
	tests := []struct {
		status   CandidateStatus
		expected string
	}{
		{StatusQuarantine, "隔离区"},
		{StatusTesting, "测试中"},
		{StatusValidated, "已验证"},
		{StatusRejected, "已拒绝"},
		{StatusPromoted, "已晋级"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("CandidateStatus(%s).String() = %s, want %s", tt.status, got, tt.expected)
		}
	}
}

func TestFormatRules(t *testing.T) {
	rules := []Rule{
		{FeatureName: "MA20", Operator: OpGreaterThan, Thresholds: []float64{0}},
		{FeatureName: "RSI14", Operator: OpLessThan, Thresholds: []float64{30}},
	}

	result := FormatRules(rules)
	if result == "" {
		t.Error("expected non-empty result")
	}
}
