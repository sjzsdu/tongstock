package paradigm

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// FeatureRegistry 统一特征注册表: 注册/查询/版本管理。
// 任何特征定义变更会产生新版本, 确保实验可复现。
type FeatureRegistry struct {
	mu    sync.RWMutex
	specs map[string][]*FeatureSpec // ID -> versions
	byKey map[string]*FeatureSpec   // "ID@version" -> spec
}

// NewFeatureRegistry 创建空注册表。
func NewFeatureRegistry() *FeatureRegistry {
	return &FeatureRegistry{
		specs: make(map[string][]*FeatureSpec),
		byKey: make(map[string]*FeatureSpec),
	}
}

// Register 注册一个新的特征规格。
// 如果 ID+版本已存在, 返回错误。
func (r *FeatureRegistry) Register(spec *FeatureSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := spec.ComputeKey()
	if _, exists := r.byKey[key]; exists {
		return fmt.Errorf("feature %s already registered, use Update to create new version", key)
	}

	r.specs[spec.ID] = append(r.specs[spec.ID], spec)
	sort.Slice(r.specs[spec.ID], func(i, j int) bool {
		return r.specs[spec.ID][i].Version < r.specs[spec.ID][j].Version
	})
	r.byKey[key] = spec
	return nil
}

// UpdateFormula 更新特征公式, 自动产生新版本。
// 如果公式指纹未变, 返回 nil (不产生新版本)。
// 如果公式指纹变化, 版本号+1 并注册为新版本。
func (r *FeatureRegistry) UpdateFormula(id string, newFormula string, newParams map[string]interface{}) (*FeatureSpec, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.specs[id]
	if !ok || len(versions) == 0 {
		return nil, fmt.Errorf("feature %s not found", id)
	}

	latest := versions[len(versions)-1]
	newHash := HashFormula(newFormula, newParams)

	if latest.FormulaHash == newHash {
		return latest, nil // 公式未变, 返回当前版本
	}

	// 创建新版本
	newSpec := *latest
	newSpec.Version = latest.Version + 1
	newSpec.FormulaHash = newHash
	newSpec.DefaultParams = newParams
	newSpec.Description = fmt.Sprintf("%s (v%d, formula updated)", latest.Description, newSpec.Version)

	key := newSpec.ComputeKey()
	r.specs[id] = append(r.specs[id], &newSpec)
	sort.Slice(r.specs[id], func(i, j int) bool {
		return r.specs[id][i].Version < r.specs[id][j].Version
	})
	r.byKey[key] = &newSpec

	return &newSpec, nil
}

// GetByID 获取指定 ID 和版本的特征。
func (r *FeatureRegistry) GetByID(id string, version int) (*FeatureSpec, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := id
	if version > 0 {
		key = fmt.Sprintf("%s@v%d", id, version)
	}

	if spec, ok := r.byKey[key]; ok {
		return spec, nil
	}

	// 如果未指定版本, 返回最新版本
	if version == 0 {
		if versions, ok := r.specs[id]; ok && len(versions) > 0 {
			return versions[len(versions)-1], nil
		}
	}

	return nil, fmt.Errorf("feature %s not found", key)
}

// GetLatest 获取指定 ID 的最新版本。
func (r *FeatureRegistry) GetLatest(id string) (*FeatureSpec, error) {
	return r.GetByID(id, 0)
}

// List 返回所有已注册特征的最新版本。
func (r *FeatureRegistry) List() []*FeatureSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*FeatureSpec
	for _, versions := range r.specs {
		if len(versions) > 0 {
			result = append(result, versions[len(versions)-1])
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// ListByCategory 返回指定分类的所有特征。
func (r *FeatureRegistry) ListByCategory(category FeatureCategory) []*FeatureSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*FeatureSpec
	for _, versions := range r.specs {
		for _, v := range versions {
			if v.Category == category && v.Status == "active" {
				result = append(result, v)
			}
		}
	}
	return result
}

// ListVersions 返回指定特征的所有版本。
func (r *FeatureRegistry) ListVersions(id string) ([]*FeatureSpec, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.specs[id]
	if !ok {
		return nil, fmt.Errorf("feature %s not found", id)
	}
	return versions, nil
}

// ResolveDependencies 解析特征的所有依赖, 按拓扑排序。
// 返回依赖链 (包含自身), 从底层依赖到顶层。
func (r *FeatureRegistry) ResolveDependencies(id string, version int) ([]*FeatureSpec, error) {
	spec, err := r.GetByID(id, version)
	if err != nil {
		return nil, err
	}

	visited := make(map[string]bool)
	var result []*FeatureSpec
	if err := r.collectDeps(spec, visited, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *FeatureRegistry) collectDeps(spec *FeatureSpec, visited map[string]bool, result *[]*FeatureSpec) error {
	key := spec.ComputeKey()
	if visited[key] {
		return fmt.Errorf("circular dependency detected at %s", key)
	}
	visited[key] = true

	// 先处理依赖
	for _, depID := range spec.Dependencies {
		dep, err := r.GetLatest(depID)
		if err != nil {
			return fmt.Errorf("dependency %s for %s not found: %w", depID, spec.ID, err)
		}
		if err := r.collectDeps(dep, visited, result); err != nil {
			return err
		}
	}

	*result = append(*result, spec)
	return nil
}

// Count 返回已注册特征数量 (按 ID 去重)。
func (r *FeatureRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.specs)
}

// BuildDefaultRegistry 构建默认特征注册表, 包含所有内置 TA/量价/市场状态特征。
func BuildDefaultRegistry() *FeatureRegistry {
	r := NewFeatureRegistry()

	now := func() time.Time { return time.Now() }

	register := func(spec *FeatureSpec) {
		spec.CreatedAt = now()
		spec.Status = "active"
		if err := r.Register(spec); err != nil {
			panic(fmt.Sprintf("register %s failed: %v", spec.ID, err))
		}
	}

	// ==================== TA 技术指标 ====================
	register(&FeatureSpec{
		ID:          "MA",
		Name:        "Simple Moving Average",
		Category:    FeatureCategoryTechnical,
		Description: "简单移动平均线",
		DefaultParams: map[string]interface{}{
			"periods": []interface{}{5, 10, 20, 60},
		},
		Window:       60,
		MinSamples:   5,
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("SMA", map[string]interface{}{"periods": []int{5, 10, 20, 60}}),
	})

	register(&FeatureSpec{
		ID:          "EMA",
		Name:        "Exponential Moving Average",
		Category:    FeatureCategoryTechnical,
		Description: "指数移动平均线",
		DefaultParams: map[string]interface{}{
			"periods": []interface{}{12, 26},
		},
		Window:       60,
		MinSamples:   12,
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("EMA", map[string]interface{}{"periods": []int{12, 26}}),
	})

	register(&FeatureSpec{
		ID:          "MACD",
		Name:        "Moving Average Convergence Divergence",
		Category:    FeatureCategoryTechnical,
		Description: "MACD 指标 (DIF/DEA/柱状图)",
		DefaultParams: map[string]interface{}{
			"fast":   12,
			"slow":   26,
			"signal": 9,
		},
		Window:       50,
		MinSamples:   35,
		Dependencies: []string{"EMA"},
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("MACD", map[string]interface{}{"fast": 12, "slow": 26, "signal": 9}),
	})

	register(&FeatureSpec{
		ID:          "KDJ",
		Name:        "KDJ Stochastic Oscillator",
		Category:    FeatureCategoryTechnical,
		Description: "KDJ 随机指标 (K/D/J)",
		DefaultParams: map[string]interface{}{
			"n":  9,
			"m1": 3,
			"m2": 3,
		},
		Window:       9,
		MinSamples:   9,
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("KDJ", map[string]interface{}{"n": 9, "m1": 3, "m2": 3}),
	})

	register(&FeatureSpec{
		ID:          "BOLL",
		Name:        "Bollinger Bands",
		Category:    FeatureCategoryTechnical,
		Description: "布林带 (上/中/下轨)",
		DefaultParams: map[string]interface{}{
			"n": 20,
			"k": 2.0,
		},
		Window:       20,
		MinSamples:   20,
		Dependencies: []string{"MA"},
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("BOLL", map[string]interface{}{"n": 20, "k": 2.0}),
	})

	register(&FeatureSpec{
		ID:          "RSI",
		Name:        "Relative Strength Index",
		Category:    FeatureCategoryTechnical,
		Description: "相对强弱指数",
		DefaultParams: map[string]interface{}{
			"periods": []interface{}{6, 12, 24},
		},
		Window:       24,
		MinSamples:   6,
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("RSI", map[string]interface{}{"periods": []int{6, 12, 24}}),
	})

	// ==================== 量价关系 ====================
	register(&FeatureSpec{
		ID:          "VolumeRatio",
		Name:        "Volume Ratio",
		Category:    FeatureCategoryVolumePrice,
		Description: "成交量比率 (当日成交量/5日均量)",
		DefaultParams: map[string]interface{}{
			"window": 5,
		},
		Window:       5,
		MinSamples:   5,
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("VolumeRatio", map[string]interface{}{"window": 5}),
	})

	register(&FeatureSpec{
		ID:            "OBV",
		Name:          "On-Balance Volume",
		Category:      FeatureCategoryVolumePrice,
		Description:   "能量潮指标",
		DefaultParams: map[string]interface{}{},
		Window:        0,
		MinSamples:    2,
		Timing:        TimingEndOfDay,
		DataRequired:  []string{"kline"},
		FormulaHash:   HashFormula("OBV", nil),
	})

	// ==================== 市场状态 ====================
	register(&FeatureSpec{
		ID:          "TrendDirection",
		Name:        "Trend Direction",
		Category:    FeatureCategoryMarketState,
		Description: "趋势方向判断 (上涨/下跌/横盘)",
		DefaultParams: map[string]interface{}{
			"short_ma": 5,
			"mid_ma":   10,
			"long_ma":  20,
			"lookback": 5,
		},
		Window:       20,
		MinSamples:   20,
		Dependencies: []string{"MA"},
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline"},
		FormulaHash:  HashFormula("TrendDirection", map[string]interface{}{"short_ma": 5, "mid_ma": 10, "long_ma": 20, "lookback": 5}),
	})

	register(&FeatureSpec{
		ID:          "MarketBreadth",
		Name:        "Market Breadth",
		Category:    FeatureCategoryMarketState,
		Description: "市场广度 (上涨家数/下跌家数)",
		DefaultParams: map[string]interface{}{
			"index_code": "SH000001",
		},
		Window:       1,
		MinSamples:   1,
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline", "market"},
		FormulaHash:  HashFormula("MarketBreadth", map[string]interface{}{"index_code": "SH000001"}),
	})

	// ==================== 相对强弱 ====================
	register(&FeatureSpec{
		ID:          "RelativeStrength",
		Name:        "Relative Strength (RPS)",
		Category:    FeatureCategoryRelativeStrength,
		Description: "相对价格强度 (在全市场中排名百分位)",
		DefaultParams: map[string]interface{}{
			"period": 60,
		},
		Window:       60,
		MinSamples:   20,
		Timing:       TimingEndOfDay,
		DataRequired: []string{"kline", "market"},
		FormulaHash:  HashFormula("RelativeStrength", map[string]interface{}{"period": 60}),
	})

	// ==================== 事件特征 ====================
	register(&FeatureSpec{
		ID:            "XdXrAdjustment",
		Name:          "XdXr Adjustment Factor",
		Category:      FeatureCategoryEvent,
		Description:   "除权除息调整因子",
		DefaultParams: map[string]interface{}{},
		Window:        0,
		MinSamples:    1,
		Timing:        TimingEventDriven,
		DataRequired:  []string{"xdxr"},
		FormulaHash:   HashFormula("XdXrAdjustment", nil),
	})

	register(&FeatureSpec{
		ID:            "STStatus",
		Name:          "ST Stock Status",
		Category:      FeatureCategoryEvent,
		Description:   "ST/*ST 状态标记",
		DefaultParams: map[string]interface{}{},
		Window:        0,
		MinSamples:    1,
		Timing:        TimingEventDriven,
		DataRequired:  []string{"stockinfo"},
		FormulaHash:   HashFormula("STStatus", nil),
	})

	return r
}
