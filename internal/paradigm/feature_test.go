package paradigm

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func setupFeatureStore(t *testing.T) *FeatureStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "feature_test.db")
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewFeatureStore(s)
}

// ==================== FeatureSpec 测试 ====================

func TestFeatureSpec_ComputeKey(t *testing.T) {
	spec := &FeatureSpec{ID: "MACD", Version: 1}
	if spec.ComputeKey() != "MACD@v1" {
		t.Errorf("ComputeKey = %s, want MACD@v1", spec.ComputeKey())
	}

	spec2 := &FeatureSpec{ID: "RSI", Version: 0}
	if spec2.ComputeKey() != "RSI" {
		t.Errorf("ComputeKey = %s, want RSI", spec2.ComputeKey())
	}
}

func TestFeatureSpec_ParamAccessors(t *testing.T) {
	spec := &FeatureSpec{
		ID: "test",
		DefaultParams: map[string]interface{}{
			"period":  14,
			"ratio":   2.5,
			"label":   "alpha",
			"missing": nil,
		},
	}

	if p := spec.ParamInt("period", 0); p != 14 {
		t.Errorf("ParamInt = %d, want 14", p)
	}
	if p := spec.ParamInt("missing", 99); p != 99 {
		t.Errorf("ParamInt default = %d, want 99", p)
	}
	if p := spec.ParamFloat("ratio", 0); p != 2.5 {
		t.Errorf("ParamFloat = %f, want 2.5", p)
	}
	if p := spec.ParamFloat("missing", 3.14); p != 3.14 {
		t.Errorf("ParamFloat default = %f, want 3.14", p)
	}
	if p := spec.ParamString("label", ""); p != "alpha" {
		t.Errorf("ParamString = %s, want alpha", p)
	}
	if p := spec.ParamString("missing", "default"); p != "default" {
		t.Errorf("ParamString default = %s, want default", p)
	}
}

func TestFeatureSpec_Validate(t *testing.T) {
	valid := &FeatureSpec{
		ID:       "MACD",
		Category: FeatureCategoryTechnical,
		Window:   50,
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("Valid spec should pass: %v", err)
	}

	missingID := &FeatureSpec{Category: FeatureCategoryTechnical}
	if err := missingID.Validate(); err == nil {
		t.Error("Should fail with empty ID")
	}

	missingCat := &FeatureSpec{ID: "test"}
	if err := missingCat.Validate(); err == nil {
		t.Error("Should fail with empty category")
	}

	negWindow := &FeatureSpec{ID: "test", Category: FeatureCategoryTechnical, Window: -1}
	if err := negWindow.Validate(); err == nil {
		t.Error("Should fail with negative window")
	}
}

// ==================== HashFormula 测试 ====================

func TestHashFormula_Stability(t *testing.T) {
	h1 := HashFormula("MACD", map[string]interface{}{"fast": 12, "slow": 26})
	h2 := HashFormula("MACD", map[string]interface{}{"fast": 12, "slow": 26})
	if h1 != h2 {
		t.Error("Same formula+params should produce same hash")
	}

	h3 := HashFormula("MACD", map[string]interface{}{"fast": 8, "slow": 17})
	if h1 == h3 {
		t.Error("Different params should produce different hash")
	}

	h4 := HashFormula("RSI", nil)
	h5 := HashFormula("RSI", nil)
	if h4 != h5 {
		t.Error("Same formula without params should produce same hash")
	}
}

// ==================== LeakCheck 测试 ====================

func TestLeakCheck_NoLeak(t *testing.T) {
	checkDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local)
	dataAsOf := time.Date(2024, 5, 31, 0, 0, 0, 0, time.Local)

	result := NewLeakCheck("MACD", checkDate, dataAsOf, TimingEndOfDay)
	if !result.Passed {
		t.Errorf("Expected pass, got violations: %v", result.Violations)
	}
}

func TestLeakCheck_FutureData(t *testing.T) {
	checkDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local)
	// 数据远超 effective deadline (T+1), 在 T 日不可用
	dataAsOf := time.Date(2024, 6, 3, 0, 0, 0, 0, time.Local)

	result := NewLeakCheck("MACD", checkDate, dataAsOf, TimingEndOfDay)
	if result.Passed {
		t.Error("Should fail with future data leak")
	}
	if len(result.Violations) == 0 {
		t.Error("Should have violations")
	}
}

func TestLeakCheck_IntradayTiming(t *testing.T) {
	now := time.Now()
	checkDate := now
	dataAsOf := now.Add(time.Hour) // 一小时后

	result := NewLeakCheck("VolumeRatio", checkDate, dataAsOf, TimingIntraday)
	if result.Passed {
		t.Error("Intraday timing should fail with future data")
	}
}

// ==================== FeatureRegistry 测试 ====================

func TestFeatureRegistry_RegisterAndGet(t *testing.T) {
	r := NewFeatureRegistry()

	spec := &FeatureSpec{
		ID:          "MACD",
		Name:        "MACD",
		Category:    FeatureCategoryTechnical,
		Version:     1,
		Description: "MACD indicator",
		DefaultParams: map[string]interface{}{
			"fast": 12, "slow": 26, "signal": 9,
		},
		Window:       50,
		MinSamples:   35,
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("MACD", map[string]interface{}{"fast": 12, "slow": 26, "signal": 9}),
	}

	if err := r.Register(spec); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Get by ID and version
	got, err := r.GetByID("MACD", 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}

	// Get latest (auto)
	gotLatest, err := r.GetLatest("MACD")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if gotLatest.Version != 1 {
		t.Errorf("Latest version = %d, want 1", gotLatest.Version)
	}
}

func TestFeatureRegistry_DuplicateRejected(t *testing.T) {
	r := NewFeatureRegistry()

	spec := &FeatureSpec{
		ID:       "MACD",
		Category: FeatureCategoryTechnical,
		Version:  1,
		Window:   50,
	}

	if err := r.Register(spec); err != nil {
		t.Fatalf("First register: %v", err)
	}

	// Same ID+version should fail
	if err := r.Register(spec); err == nil {
		t.Error("Duplicate registration should fail")
	}
}

func TestFeatureRegistry_UpdateFormula(t *testing.T) {
	r := NewFeatureRegistry()

	spec := &FeatureSpec{
		ID:       "MACD",
		Category: FeatureCategoryTechnical,
		Version:  1,
		Window:   50,
		DefaultParams: map[string]interface{}{
			"fast": 12, "slow": 26, "signal": 9,
		},
		FormulaHash: HashFormula("MACD", map[string]interface{}{"fast": 12, "slow": 26, "signal": 9}),
	}
	r.Register(spec)

	// 公式未变 -> 返回当前版本
	got, err := r.UpdateFormula("MACD", "MACD", map[string]interface{}{"fast": 12, "slow": 26, "signal": 9})
	if err != nil {
		t.Fatalf("UpdateFormula (same): %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Version should stay 1 when formula unchanged, got %d", got.Version)
	}

	// 公式变更 -> 新版本
	got2, err := r.UpdateFormula("MACD", "MACD", map[string]interface{}{"fast": 8, "slow": 17, "signal": 9})
	if err != nil {
		t.Fatalf("UpdateFormula (changed): %v", err)
	}
	if got2.Version != 2 {
		t.Errorf("Version should be 2 after formula change, got %d", got2.Version)
	}

	// 验证两个版本都存在
	all, err := r.ListVersions("MACD")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("Should have 2 versions, got %d", len(all))
	}
}

func TestFeatureRegistry_List(t *testing.T) {
	r := NewFeatureRegistry()

	specs := []*FeatureSpec{
		{ID: "A", Category: FeatureCategoryTechnical, Window: 10, Version: 1},
		{ID: "B", Category: FeatureCategoryVolumePrice, Window: 20, Version: 1},
		{ID: "C", Category: FeatureCategoryMarketState, Window: 30, Version: 1},
	}
	for _, s := range specs {
		s.Status = "active"
		r.Register(s)
	}

	list := r.List()
	if len(list) != 3 {
		t.Errorf("List len = %d, want 3", len(list))
	}

	techList := r.ListByCategory(FeatureCategoryTechnical)
	if len(techList) != 1 || techList[0].ID != "A" {
		t.Errorf("Category filter failed")
	}
}

func TestFeatureRegistry_Dependencies(t *testing.T) {
	r := NewFeatureRegistry()

	ma := &FeatureSpec{
		ID: "MA", Category: FeatureCategoryTechnical, Window: 20, Version: 1, Status: "active",
	}
	boll := &FeatureSpec{
		ID: "BOLL", Category: FeatureCategoryTechnical, Window: 20, Version: 1,
		Dependencies: []string{"MA"}, Status: "active",
	}
	r.Register(ma)
	r.Register(boll)

	// 解析依赖
	order, err := r.ResolveDependencies("BOLL", 0)
	if err != nil {
		t.Fatalf("ResolveDependencies: %v", err)
	}

	// 依赖应先于自身
	if len(order) < 2 {
		t.Fatalf("Should have at least 2 specs, got %d", len(order))
	}
	if order[0].ID != "MA" {
		t.Errorf("First dep should be MA, got %s", order[0].ID)
	}
	if order[1].ID != "BOLL" {
		t.Errorf("Second should be BOLL, got %s", order[1].ID)
	}
}

func TestFeatureRegistry_CircularDependency(t *testing.T) {
	r := NewFeatureRegistry()

	a := &FeatureSpec{
		ID: "A", Category: FeatureCategoryTechnical, Window: 10, Version: 1,
		Dependencies: []string{"B"}, Status: "active",
	}
	b := &FeatureSpec{
		ID: "B", Category: FeatureCategoryTechnical, Window: 10, Version: 1,
		Dependencies: []string{"A"}, Status: "active",
	}
	r.Register(a)
	r.Register(b)

	_, err := r.ResolveDependencies("A", 0)
	if err == nil {
		t.Error("Circular dependency should be detected")
	}
}

func TestFeatureRegistry_Count(t *testing.T) {
	r := NewFeatureRegistry()
	if r.Count() != 0 {
		t.Error("Empty registry should have count 0")
	}

	r.Register(&FeatureSpec{ID: "A", Category: FeatureCategoryTechnical, Window: 10, Version: 1})
	r.Register(&FeatureSpec{ID: "B", Category: FeatureCategoryTechnical, Window: 10, Version: 1})
	r.Register(&FeatureSpec{ID: "A", Category: FeatureCategoryTechnical, Window: 10, Version: 2}) // 新版本, 同一 ID

	if r.Count() != 2 {
		t.Errorf("Count = %d, want 2 (unique IDs)", r.Count())
	}
}

// ==================== FeaturePipeline 测试 ====================

func TestFeaturePipeline_Compute(t *testing.T) {
	reg := BuildDefaultRegistry()
	pipeline := NewFeaturePipeline(reg)

	asOf := time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local)
	req := ComputeRequest{
		StockCode: "600000",
		Features:  []string{"MACD", "RSI"},
		AsOf:      asOf,
		PriceReq:  PriceForward,
	}

	resp, err := pipeline.Compute(req)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if resp.StockCode != "600000" {
		t.Errorf("StockCode = %s, want 600000", resp.StockCode)
	}

	// 应包含 MACD 和 RSI 的元数据
	if _, ok := resp.FeatureMeta["MACD"]; !ok {
		t.Error("Should have MACD meta")
	}
	if _, ok := resp.FeatureMeta["RSI"]; !ok {
		t.Error("Should have RSI meta")
	}

	// MACD 应依赖 EMA, 所以管线也包含 EMA
	if _, ok := resp.FeatureMeta["EMA"]; !ok {
		t.Error("MACD dependency EMA should be in pipeline meta")
	}
}

func TestFeaturePipeline_LeakCheck(t *testing.T) {
	reg := BuildDefaultRegistry()
	pipeline := NewFeaturePipeline(reg)

	// 使用未来日期 (潜在泄漏)
	futureDate := time.Now().AddDate(0, 0, 30)
	req := ComputeRequest{
		StockCode: "600000",
		Features:  []string{"MACD"},
		AsOf:      futureDate,
	}

	resp, err := pipeline.Compute(req)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if resp.LeakCheck == nil {
		t.Fatal("Should have leak check result")
	}

	// 未来日期应该触发泄漏检测
	if resp.LeakCheck.Passed {
		t.Error("Future date should fail leak check")
	}
	if len(resp.LeakCheck.Violations) == 0 {
		t.Error("Should have violations for future data")
	}
}

func TestFeaturePipeline_ValidateFeatureSet(t *testing.T) {
	reg := BuildDefaultRegistry()
	pipeline := NewFeaturePipeline(reg)

	// 有效特征集
	if err := pipeline.ValidateFeatureSet([]string{"MACD", "RSI"}); err != nil {
		t.Errorf("Valid set should pass: %v", err)
	}

	// 无效特征
	if err := pipeline.ValidateFeatureSet([]string{"NONEXISTENT"}); err == nil {
		t.Error("Invalid feature should fail")
	}
}

func TestFeaturePipeline_DataRequirements(t *testing.T) {
	reg := BuildDefaultRegistry()
	pipeline := NewFeaturePipeline(reg)

	reqs, err := pipeline.GetDataRequirements([]string{"MACD", "RSI"})
	if err != nil {
		t.Fatalf("GetDataRequirements: %v", err)
	}

	// MACD 和 RSI 都需要 kline
	hasKline := false
	for _, r := range reqs {
		if r == "kline" {
			hasKline = true
			break
		}
	}
	if !hasKline {
		t.Error("Should require kline data")
	}
}

func TestFeaturePipeline_DescribeComputation(t *testing.T) {
	reg := BuildDefaultRegistry()
	pipeline := NewFeaturePipeline(reg)

	desc := pipeline.DescribeComputation([]string{"MACD"}, time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local))
	if desc == "" {
		t.Error("Description should not be empty")
	}
}

// ==================== Default Registry 测试 ====================

func TestBuildDefaultRegistry_CoversCategories(t *testing.T) {
	reg := BuildDefaultRegistry()

	// 验证所有必需分类都有特征
	categories := map[FeatureCategory]bool{
		FeatureCategoryTechnical:        false,
		FeatureCategoryVolumePrice:      false,
		FeatureCategoryRelativeStrength: false,
		FeatureCategoryMarketState:      false,
		FeatureCategoryEvent:            false,
	}

	for _, spec := range reg.List() {
		categories[spec.Category] = true
	}

	for cat, found := range categories {
		if !found {
			t.Errorf("Category %s has no registered features", cat)
		}
	}
}

func TestBuildDefaultRegistry_Dependencies(t *testing.T) {
	reg := BuildDefaultRegistry()

	// 验证所有依赖都可解析
	for _, spec := range reg.List() {
		for _, depID := range spec.Dependencies {
			if _, err := reg.GetLatest(depID); err != nil {
				t.Errorf("Feature %s depends on %s which is not registered", spec.ID, depID)
			}
		}
	}
}

// ==================== FeatureStore 测试 ====================

func TestFeatureStore_SaveAndGetSpec(t *testing.T) {
	store := setupFeatureStore(t)
	now := time.Now().Truncate(time.Second)

	spec := &FeatureSpec{
		ID:          "MACD",
		Name:        "MACD",
		Category:    FeatureCategoryTechnical,
		Version:     1,
		Description: "MACD indicator",
		DefaultParams: map[string]interface{}{
			"fast": 12, "slow": 26, "signal": 9,
		},
		Window:       50,
		MinSamples:   35,
		Dependencies: []string{"EMA"},
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  "abc123",
		Status:       "active",
		CreatedAt:    now,
	}

	if err := store.SaveSpec(spec); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}

	got, err := store.GetSpec("MACD", 1)
	if err != nil {
		t.Fatalf("GetSpec: %v", err)
	}

	if got.ID != "MACD" || got.Version != 1 {
		t.Errorf("ID/Version mismatch: %s@v%d", got.ID, got.Version)
	}
	if got.Category != FeatureCategoryTechnical {
		t.Errorf("Category = %s, want %s", got.Category, FeatureCategoryTechnical)
	}
	if got.FormulaHash != "abc123" {
		t.Errorf("FormulaHash = %s, want abc123", got.FormulaHash)
	}
	if got.Window != 50 {
		t.Errorf("Window = %d, want 50", got.Window)
	}
	if len(got.Dependencies) != 1 || got.Dependencies[0] != "EMA" {
		t.Errorf("Dependencies = %v, want [EMA]", got.Dependencies)
	}
}

func TestFeatureStore_GetLatestSpec(t *testing.T) {
	store := setupFeatureStore(t)

	v1 := &FeatureSpec{ID: "RSI", Category: FeatureCategoryTechnical, Version: 1, Window: 14, FormulaHash: "v1", CreatedAt: time.Now()}
	v2 := &FeatureSpec{ID: "RSI", Category: FeatureCategoryTechnical, Version: 2, Window: 24, FormulaHash: "v2", CreatedAt: time.Now()}

	store.SaveSpec(v1)
	store.SaveSpec(v2)

	got, err := store.GetLatestSpec("RSI")
	if err != nil {
		t.Fatalf("GetLatestSpec: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Latest version = %d, want 2", got.Version)
	}
	if got.Window != 24 {
		t.Errorf("Latest window = %d, want 24", got.Window)
	}
}

func TestFeatureStore_ListSpecs(t *testing.T) {
	store := setupFeatureStore(t)

	specs := []*FeatureSpec{
		{ID: "A", Category: FeatureCategoryTechnical, Version: 1, Window: 10, Status: "active", CreatedAt: time.Now()},
		{ID: "B", Category: FeatureCategoryVolumePrice, Version: 1, Window: 20, Status: "active", CreatedAt: time.Now()},
		{ID: "C", Category: FeatureCategoryTechnical, Version: 1, Window: 30, Status: "deprecated", CreatedAt: time.Now()},
	}
	for _, s := range specs {
		store.SaveSpec(s)
	}

	// 全部
	all, err := store.ListSpecs("", false)
	if err != nil {
		t.Fatalf("ListSpecs: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("All specs = %d, want 3", len(all))
	}

	// 仅技术分类
	tech, err := store.ListSpecs(FeatureCategoryTechnical, false)
	if err != nil {
		t.Fatalf("ListSpecs (category): %v", err)
	}
	// 两个技术分类的, 但取最新版本, 所以 A 和 C 各一条, 但 C 是 deprecated
	if len(tech) != 2 {
		t.Errorf("Tech specs = %d, want 2", len(tech))
	}

	// 仅活跃的技术分类
	techActive, err := store.ListSpecs(FeatureCategoryTechnical, true)
	if err != nil {
		t.Fatalf("ListSpecs (active): %v", err)
	}
	if len(techActive) != 1 {
		t.Errorf("Active tech specs = %d, want 1", len(techActive))
	}
}

func TestFeatureStore_SaveAndGetFeatureSetSpec(t *testing.T) {
	store := setupFeatureStore(t)
	now := time.Now().Truncate(time.Second)

	fs := NewFeatureSetSpec("tech-basic", "Basic Technical Set", FeatureCategoryTechnical,
		[]string{"MA", "MACD", "RSI"}, PriceForward)
	fs.CreatedAt = now
	fs.Version = 1

	if err := store.SaveFeatureSetSpec(fs); err != nil {
		t.Fatalf("SaveFeatureSetSpec: %v", err)
	}

	got, err := store.GetFeatureSetSpec("tech-basic", 1)
	if err != nil {
		t.Fatalf("GetFeatureSetSpec: %v", err)
	}

	if got.ID != "tech-basic" {
		t.Errorf("ID = %s, want tech-basic", got.ID)
	}
	if len(got.Features) != 3 {
		t.Errorf("Features len = %d, want 3", len(got.Features))
	}
	if got.PriceReq != PriceForward {
		t.Errorf("PriceReq = %s, want %s", got.PriceReq, PriceForward)
	}
}

func TestFeatureStore_SaveAndGetFeatureValue(t *testing.T) {
	store := setupFeatureStore(t)
	now := time.Now().Truncate(time.Second)
	date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local)

	val := &FeatureValue{
		FeatureID:   "MACD",
		Version:     1,
		StockCode:   "600000",
		Date:        date,
		Value:       1.234,
		SourceData:  "kline_v1",
		ComputedAt:  now,
		AsOf:        date,
		LeakChecked: true,
	}

	if err := store.SaveFeatureValue(val); err != nil {
		t.Fatalf("SaveFeatureValue: %v", err)
	}

	results, err := store.GetFeatureValues("600000", "MACD", date, date)
	if err != nil {
		t.Fatalf("GetFeatureValues: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Results len = %d, want 1", len(results))
	}
	if results[0].Value != 1.234 {
		t.Errorf("Value = %f, want 1.234", results[0].Value)
	}
	if results[0].LeakChecked != true {
		t.Error("LeakChecked should be true")
	}
}

func TestFeatureStore_Counts(t *testing.T) {
	store := setupFeatureStore(t)

	cnt, _ := store.CountSpecs()
	if cnt != 0 {
		t.Errorf("Initial CountSpecs = %d, want 0", cnt)
	}

	store.SaveSpec(&FeatureSpec{ID: "A", Category: FeatureCategoryTechnical, Version: 1, Window: 10, CreatedAt: time.Now()})
	store.SaveSpec(&FeatureSpec{ID: "B", Category: FeatureCategoryTechnical, Version: 1, Window: 10, CreatedAt: time.Now()})
	store.SaveSpec(&FeatureSpec{ID: "A", Category: FeatureCategoryTechnical, Version: 2, Window: 10, CreatedAt: time.Now()})

	uniqueCnt, _ := store.CountSpecs()
	if uniqueCnt != 2 {
		t.Errorf("CountSpecs (unique IDs) = %d, want 2", uniqueCnt)
	}

	allCnt, _ := store.CountAllVersions()
	if allCnt != 3 {
		t.Errorf("CountAllVersions = %d, want 3", allCnt)
	}
}

// ==================== 版本化与无泄漏集成测试 ====================

func TestVersionedLeakFreeWorkflow(t *testing.T) {
	// 模拟完整工作流: 注册 -> 变更公式产生新版本 -> 使用指定版本计算 -> 无泄漏检查
	reg := NewFeatureRegistry()
	store := setupFeatureStore(t)
	now := time.Now().Truncate(time.Second)

	// Step 1: 注册初始版本
	specV1 := &FeatureSpec{
		ID:          "CUSTOM_MA",
		Name:        "Custom MA",
		Category:    FeatureCategoryTechnical,
		Version:     1,
		Description: "Custom moving average",
		DefaultParams: map[string]interface{}{
			"period": 20,
		},
		Window:       20,
		MinSamples:   20,
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("SMA", map[string]interface{}{"period": 20}),
		Status:       "active",
		CreatedAt:    now,
	}
	reg.Register(specV1)
	store.SaveSpec(specV1)

	// Step 2: 变更公式 (不同周期)
	updated, err := reg.UpdateFormula("CUSTOM_MA", "SMA", map[string]interface{}{"period": 60})
	if err != nil {
		t.Fatalf("UpdateFormula: %v", err)
	}
	if updated.Version != 2 {
		t.Errorf("Updated version = %d, want 2", updated.Version)
	}
	store.SaveSpec(updated)

	// Step 3: 使用 v1 版本计算 (确保可复现)
	pipeline := NewFeaturePipeline(reg)
	asOf := time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local)

	resp, err := pipeline.Compute(ComputeRequest{
		StockCode: "600000",
		Features:  []string{"CUSTOM_MA"},
		AsOf:      asOf,
	})
	if err != nil {
		t.Fatalf("Compute v1: %v", err)
	}

	// Step 4: 无泄漏验证
	if resp.LeakCheck == nil {
		t.Fatal("Leak check should be present")
	}
	// 今天日期的检查应该通过 (过去日期)
	if !resp.LeakCheck.Passed {
		t.Errorf("Leak check should pass for past date, violations: %v", resp.LeakCheck.Violations)
	}

	// Step 5: 特征数据落库
	date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local)
	val := &FeatureValue{
		FeatureID:   "CUSTOM_MA",
		Version:     2,
		StockCode:   "600000",
		Date:        date,
		Value:       15.5,
		ComputedAt:  now,
		AsOf:        date,
		LeakChecked: true,
	}
	store.SaveFeatureValue(val)

	results, err := store.GetFeatureValues("600000", "CUSTOM_MA", date, date)
	if err != nil {
		t.Fatalf("GetFeatureValues: %v", err)
	}
	if len(results) != 1 || results[0].Value != 15.5 {
		t.Error("Persisted value mismatch")
	}

	// Step 6: 验证两个版本都在 store 中
	v1, err := store.GetSpec("CUSTOM_MA", 1)
	if err != nil {
		t.Fatalf("Get v1: %v", err)
	}
	if v1.ParamInt("period", 0) != 20 {
		t.Errorf("v1 period = %d, want 20", v1.ParamInt("period", 0))
	}

	v2, err := store.GetSpec("CUSTOM_MA", 2)
	if err != nil {
		t.Fatalf("Get v2: %v", err)
	}
	if v2.ParamInt("period", 0) != 60 {
		t.Errorf("v2 period = %d, want 60", v2.ParamInt("period", 0))
	}
}
