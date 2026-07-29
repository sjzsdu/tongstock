package paradigms

import (
	"fmt"
	"time"
)

// ============================================================================
// 基准范式库
// ============================================================================

// BenchmarkCategory 基准分类
type BenchmarkCategory string

const (
	CategoryMeanReversion BenchmarkCategory = "mean_reversion" // 均值回归
	CategoryMomentum      BenchmarkCategory = "momentum"       // 趋势跟随
	CategoryBreakout      BenchmarkCategory = "breakout"       // 突破
	CategoryVolumeProfile BenchmarkCategory = "volume_profile" // 成交量特征
	CategoryEventDriven   BenchmarkCategory = "event_driven"   // 事件驱动
)

// BenchmarkDifficulty 基准难度
type BenchmarkDifficulty string

const (
	DifficultyEasy   BenchmarkDifficulty = "easy"   // 易于通过
	DifficultyMedium BenchmarkDifficulty = "medium" // 中等难度
	DifficultyHard   BenchmarkDifficulty = "hard"   // 难以通过 (应被淘汰)
)

// ExpectedResult 预期结果
type ExpectedResult string

const (
	ExpectedPass   ExpectedResult = "pass"   // 预期通过 (稳健性评分高)
	ExpectedReject ExpectedResult = "reject" // 预期淘汰 (过拟合/不稳定)
)

// BenchmarkSpec 基准规格
type BenchmarkSpec struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Category        BenchmarkCategory   `json:"category"`
	Difficulty      BenchmarkDifficulty `json:"difficulty"`
	ExpectedResult  ExpectedResult      `json:"expected_result"`
	Description     string              `json:"description"`
	EconomicLogic   string              `json:"economic_logic"`
	Schema          *ParadigmSchema     `json:"schema"`
	ExpectedMetrics *ExpectedMetrics    `json:"expected_metrics"`
	CreatedAt       time.Time           `json:"created_at"`
}

// ExpectedMetrics 预期指标
type ExpectedMetrics struct {
	MinSampleOutReturn  float64 `json:"min_sample_out_return"`
	MinSharpeRatio      float64 `json:"min_sharpe_ratio"`
	MaxDrawdown         float64 `json:"max_drawdown"`
	MinStabilityScore   float64 `json:"min_stability_score"`
	MaxParamSensitivity float64 `json:"max_param_sensitivity"`
	ExpectedObservation string  `json:"expected_observation"` // 观察说明
}

// BenchmarkLibrary 基准范式库
type BenchmarkLibrary struct {
	benchmarks []BenchmarkSpec
}

// NewBenchmarkLibrary 创建基准范式库
func NewBenchmarkLibrary() *BenchmarkLibrary {
	lib := &BenchmarkLibrary{
		benchmarks: createDefaultBenchmarks(),
	}
	return lib
}

// createDefaultBenchmarks 创建默认基准
func createDefaultBenchmarks() []BenchmarkSpec {
	now := time.Now()

	return []BenchmarkSpec{
		// 1. 简单双均线交叉 (应通过)
		{
			ID:             "bm-ma-cross",
			Name:           "双均线交叉策略",
			Category:       CategoryMomentum,
			Difficulty:     DifficultyEasy,
			ExpectedResult: ExpectedPass,
			Description:    "经典双均线交叉: 5日均线上穿20日均线买入，下穿卖出",
			EconomicLogic:  "短期趋势惯性, 价格行为的趋势持续性",
			Schema:         createMaCrossSchema(),
			ExpectedMetrics: &ExpectedMetrics{
				MinSampleOutReturn:  0.05,
				MinSharpeRatio:      0.5,
				MaxDrawdown:         0.15,
				MinStabilityScore:   0.6,
				MaxParamSensitivity: 0.3,
				ExpectedObservation: "在趋势行情中表现较好, 震荡市中可能失效",
			},
			CreatedAt: now,
		},
		// 2. RS I 超卖反弹 (应通过)
		{
			ID:             "bm-rsi-reversal",
			Name:           "RSI 超卖反弹",
			Category:       CategoryMeanReversion,
			Difficulty:     DifficultyMedium,
			ExpectedResult: ExpectedPass,
			Description:    "RSI 低于 30 买入, 高于 70 卖出, 捕捉短期超卖反弹",
			EconomicLogic:  "短期超卖后的均值回归, 价格偏离后的修正",
			Schema:         createRSISchema(),
			ExpectedMetrics: &ExpectedMetrics{
				MinSampleOutReturn:  0.08,
				MinSharpeRatio:      0.8,
				MaxDrawdown:         0.12,
				MinStabilityScore:   0.5,
				MaxParamSensitivity: 0.4,
				ExpectedObservation: "在震荡市中表现较好, 趋势市中可能持续超卖",
			},
			CreatedAt: now,
		},
		// 3. 布林带突破 (应通过)
		{
			ID:             "bm-bollinger-breakout",
			Name:           "布林带突破",
			Category:       CategoryBreakout,
			Difficulty:     DifficultyMedium,
			ExpectedResult: ExpectedPass,
			Description:    "价格突破 20 日布林带上下轨时交易",
			EconomicLogic:  "波动率压缩后的扩张, 趋势确认后的延续性",
			Schema:         createBollingerSchema(),
			ExpectedMetrics: &ExpectedMetrics{
				MinSampleOutReturn:  0.06,
				MinSharpeRatio:      0.6,
				MaxDrawdown:         0.18,
				MinStabilityScore:   0.55,
				MaxParamSensitivity: 0.35,
				ExpectedObservation: "在低波动后突破时表现好, 假突破时亏损",
			},
			CreatedAt: now,
		},
		// 4. 极端参数敏感策略 (应被淘汰)
		{
			ID:             "bm-overfit-extreme",
			Name:           "极端参数敏感策略",
			Category:       CategoryMomentum,
			Difficulty:     DifficultyHard,
			ExpectedResult: ExpectedReject,
			Description:    "参数经过极致优化, 在样本内完美但样本外崩溃",
			EconomicLogic:  "无实质经济逻辑, 纯粹的曲线拟合",
			Schema:         createOverfitSchema(),
			ExpectedMetrics: &ExpectedMetrics{
				MinSampleOutReturn:  -0.05, // 预期亏损
				MinSharpeRatio:      -0.5,
				MaxDrawdown:         0.25,
				MinStabilityScore:   0.3,
				MaxParamSensitivity: 0.6,
				ExpectedObservation: "参数敏感性极高, 样本外必然失败",
			},
			CreatedAt: now,
		},
		// 5. 随机信号策略 (应被淘汰)
		{
			ID:             "bm-random-noise",
			Name:           "随机噪声策略",
			Category:       CategoryMeanReversion,
			Difficulty:     DifficultyHard,
			ExpectedResult: ExpectedReject,
			Description:    "随机生成买卖信号, 用于验证系统是否能识别无效策略",
			EconomicLogic:  "无经济逻辑, 验证系统的抗干扰能力",
			Schema:         createRandomSchema(),
			ExpectedMetrics: &ExpectedMetrics{
				MinSampleOutReturn:  0.0,
				MinSharpeRatio:      0.0,
				MaxDrawdown:         0.20,
				MinStabilityScore:   0.2,
				MaxParamSensitivity: 0.8,
				ExpectedObservation: "随机策略应被识别为无效",
			},
			CreatedAt: now,
		},
	}
}

// ============================================================================
// 基准访问方法
// ============================================================================

// GetBenchmarks 获取所有基准
func (bl *BenchmarkLibrary) GetBenchmarks() []BenchmarkSpec {
	return bl.benchmarks
}

// GetBenchmark 获取单个基准
func (bl *BenchmarkLibrary) GetBenchmark(id string) *BenchmarkSpec {
	for i := range bl.benchmarks {
		if bl.benchmarks[i].ID == id {
			return &bl.benchmarks[i]
		}
	}
	return nil
}

// GetBenchmarksByCategory 按分类获取
func (bl *BenchmarkLibrary) GetBenchmarksByCategory(category BenchmarkCategory) []BenchmarkSpec {
	var result []BenchmarkSpec
	for _, b := range bl.benchmarks {
		if b.Category == category {
			result = append(result, b)
		}
	}
	return result
}

// GetPassBenchmarks 获取应通过的基准
func (bl *BenchmarkLibrary) GetPassBenchmarks() []BenchmarkSpec {
	var result []BenchmarkSpec
	for _, b := range bl.benchmarks {
		if b.ExpectedResult == ExpectedPass {
			result = append(result, b)
		}
	}
	return result
}

// GetRejectBenchmarks 获取应淘汰的基准
func (bl *BenchmarkLibrary) GetRejectBenchmarks() []BenchmarkSpec {
	var result []BenchmarkSpec
	for _, b := range bl.benchmarks {
		if b.ExpectedResult == ExpectedReject {
			result = append(result, b)
		}
	}
	return result
}

// Count 基准数量
func (bl *BenchmarkLibrary) Count() int {
	return len(bl.benchmarks)
}

// CountByDifficulty 按难度计数
func (bl *BenchmarkLibrary) CountByDifficulty() map[BenchmarkDifficulty]int {
	counts := make(map[BenchmarkDifficulty]int)
	for _, b := range bl.benchmarks {
		counts[b.Difficulty]++
	}
	return counts
}

// ============================================================================
// 基准 Schema 创建函数
// ============================================================================

// createMaCrossSchema 创建双均线交叉 Schema
func createMaCrossSchema() *ParadigmSchema {
	return &ParadigmSchema{
		ID:            "ma_cross_v1",
		Name:          "双均线交叉",
		Version:       1,
		SchemaVersion: "2.0",
		Description:   "5日均线上穿20日均线买入, 下穿卖出",
		Features: []FeatureDefinition{
			{Name: "price.ma5", Type: "indicator", Params: map[string]float64{"period": 5}},
			{Name: "price.ma20", Type: "indicator", Params: map[string]float64{"period": 20}},
			{Name: "price.close", Type: "price", Params: map[string]float64{}},
		},
		Rules: []Rule{
			{
				ID:          "entry-ma-cross",
				Type:        TypeEntry,
				Side:        SideBuy,
				FeatureName: "price.ma5",
				Operator:    OpCrossAbove,
				Thresholds:  []float64{20},
				Description: "MA5 上穿 MA20",
			},
			{
				ID:          "exit-ma-cross",
				Type:        TypeExitLoss,
				Side:        SideSell,
				FeatureName: "price.ma5",
				Operator:    OpCrossBelow,
				Thresholds:  []float64{20},
				Description: "MA5 下穿 MA20",
			},
			{
				ID:          "stop-loss",
				Type:        TypeInvalidation,
				Side:        SideSell,
				FeatureName: "price.close",
				Operator:    OpLessThan,
				Thresholds:  []float64{0.95},
				Description: "止损 5%",
			},
		},
		ContextRules: []ContextRule{
			{Key: ContextTrend, Operator: OpGreaterThan, Values: []string{"20"}},
		},
		HoldingPeriod: "short",
		MaxDrawdown:   0.15,
		ChangeReason:  "初始版本",
	}
}

// createRSISchema 创建 RSI Schema
func createRSISchema() *ParadigmSchema {
	return &ParadigmSchema{
		ID:            "rsi_reversal_v1",
		Name:          "RSI 超卖反弹",
		Version:       1,
		SchemaVersion: "2.0",
		Description:   "RSI 低于 30 买入, 高于 70 卖出",
		Features: []FeatureDefinition{
			{Name: "indicator.rsi", Type: "indicator", Params: map[string]float64{"period": 14}},
			{Name: "price.close", Type: "price", Params: map[string]float64{}},
		},
		Rules: []Rule{
			{
				ID:          "entry-rsi-oversold",
				Type:        TypeEntry,
				Side:        SideBuy,
				FeatureName: "indicator.rsi",
				Operator:    OpLessThan,
				Thresholds:  []float64{30},
				Description: "RSI < 30 超卖",
			},
			{
				ID:          "exit-rsi-overbought",
				Type:        TypeExitProfit,
				Side:        SideSell,
				FeatureName: "indicator.rsi",
				Operator:    OpGreaterThan,
				Thresholds:  []float64{70},
				Description: "RSI > 70 超买",
			},
			{
				ID:          "stop-loss",
				Type:        TypeInvalidation,
				Side:        SideSell,
				FeatureName: "price.close",
				Operator:    OpLessThan,
				Thresholds:  []float64{0.96},
				Description: "止损 4%",
			},
		},
		ContextRules: []ContextRule{
			{Key: ContextVolatility, Operator: OpGreaterThan, Values: []string{"0.01"}},
		},
		HoldingPeriod: "short",
		MaxDrawdown:   0.12,
		ChangeReason:  "初始版本",
	}
}

// createBollingerSchema 创建布林带 Schema
func createBollingerSchema() *ParadigmSchema {
	return &ParadigmSchema{
		ID:            "bollinger_breakout_v1",
		Name:          "布林带突破",
		Version:       1,
		SchemaVersion: "2.0",
		Description:   "价格突破布林带上下轨交易",
		Features: []FeatureDefinition{
			{Name: "indicator.bollinger_upper", Type: "indicator", Params: map[string]float64{"period": 20, "std": 2}},
			{Name: "indicator.bollinger_lower", Type: "indicator", Params: map[string]float64{"period": 20, "std": -2}},
			{Name: "price.close", Type: "price", Params: map[string]float64{}},
		},
		Rules: []Rule{
			{
				ID:          "entry-breakout-up",
				Type:        TypeEntry,
				Side:        SideBuy,
				FeatureName: "price.close",
				Operator:    OpGreaterThan,
				Thresholds:  []float64{1.0},
				Description: "价格突破上轨",
			},
			{
				ID:          "entry-breakout-down",
				Type:        TypeEntry,
				Side:        SideSell,
				FeatureName: "price.close",
				Operator:    OpLessThan,
				Thresholds:  []float64{1.0},
				Description: "价格跌破下轨",
			},
			{
				ID:          "exit-mean-revert",
				Type:        TypeExitProfit,
				Side:        SideSell,
				FeatureName: "price.close",
				Operator:    OpLessThan,
				Thresholds:  []float64{0.5},
				Description: "价格回归中轨",
			},
			{
				ID:          "stop-loss",
				Type:        TypeInvalidation,
				Side:        SideSell,
				FeatureName: "price.close",
				Operator:    OpLessThan,
				Thresholds:  []float64{0.94},
				Description: "止损 6%",
			},
		},
		ContextRules: []ContextRule{
			{Key: ContextVolatility, Operator: OpLessThan, Values: []string{"0.03"}},
		},
		HoldingPeriod: "medium",
		MaxDrawdown:   0.18,
		ChangeReason:  "初始版本",
	}
}

// createOverfitSchema 创建过拟合 Schema (预期淘汰)
func createOverfitSchema() *ParadigmSchema {
	return &ParadigmSchema{
		ID:            "overfit_extreme_v1",
		Name:          "极端参数敏感策略",
		Version:       1,
		SchemaVersion: "2.0",
		Description:   "参数极致优化, 仅用于验证系统抗过拟合能力",
		Features: []FeatureDefinition{
			{Name: "price.ma7", Type: "indicator", Params: map[string]float64{"period": 7}},
			{Name: "price.ma25", Type: "indicator", Params: map[string]float64{"period": 25}},
			{Name: "indicator.rsi", Type: "indicator", Params: map[string]float64{"period": 9}},
			{Name: "price.volume_ratio", Type: "indicator", Params: map[string]float64{"period": 10}},
			{Name: "price.close", Type: "price", Params: map[string]float64{}},
		},
		Rules: []Rule{
			{
				ID:          "entry-multi-conviction",
				Type:        TypeEntry,
				Side:        SideBuy,
				FeatureName: "price.ma7",
				Operator:    OpGreaterThan,
				Thresholds:  []float64{1.02},
				Description: "多指标联合 (极易过拟合)",
			},
			{
				ID:          "entry-rsi-precise",
				Type:        TypeEntry,
				Side:        SideBuy,
				FeatureName: "indicator.rsi",
				Operator:    OpGreaterThan,
				Thresholds:  []float64{42.7},
				Description: "RSI 精确阈值 (极易过拟合)",
			},
			{
				ID:          "exit-tight-stop",
				Type:        TypeExitLoss,
				Side:        SideSell,
				FeatureName: "price.close",
				Operator:    OpLessThan,
				Thresholds:  []float64{0.985},
				Description: "极紧密止损",
			},
			{
				ID:          "invalidation-hard-stop",
				Type:        TypeInvalidation,
				Side:        SideSell,
				FeatureName: "price.close",
				Operator:    OpLessThan,
				Thresholds:  []float64{0.97},
				Description: "极端硬止损",
			},
		},
		ContextRules: []ContextRule{
			{Key: ContextTrend, Operator: OpGreaterThan, Values: []string{"23.5"}},
			{Key: ContextVolatility, Operator: OpLessThan, Values: []string{"0.018"}},
			{Key: ContextMarketSentiment, Operator: OpGreaterThan, Values: []string{"0.55"}},
		},
		HoldingPeriod: "intraday",
		MaxDrawdown:   0.20,
		ChangeReason:  "测试过拟合检测",
	}
}

// createRandomSchema 创建随机 Schema (预期淘汰)
func createRandomSchema() *ParadigmSchema {
	return &ParadigmSchema{
		ID:            "random_noise_v1",
		Name:          "随机噪声策略",
		Version:       1,
		SchemaVersion: "2.0",
		Description:   "随机买卖信号, 用于验证系统识别无效策略的能力",
		Features: []FeatureDefinition{
			{Name: "price.close", Type: "price", Params: map[string]float64{}},
			{Name: "indicator.random", Type: "indicator", Params: map[string]float64{"seed": 42}},
		},
		Rules: []Rule{
			{
				ID:          "entry-random",
				Type:        TypeEntry,
				Side:        SideBuy,
				FeatureName: "indicator.random",
				Operator:    OpGreaterThan,
				Thresholds:  []float64{0.5},
				Description: "随机买入",
			},
			{
				ID:          "exit-random",
				Type:        TypeExitLoss,
				Side:        SideSell,
				FeatureName: "indicator.random",
				Operator:    OpLessThan,
				Thresholds:  []float64{0.5},
				Description: "随机卖出",
			},
		},
		ContextRules:  []ContextRule{},
		HoldingPeriod: "intraday",
		MaxDrawdown:   0.20,
		ChangeReason:  "测试无效策略检测",
	}
}

// ============================================================================
// 基准验证结果
// ============================================================================

// BenchmarkValidationResult 基准验证结果
type BenchmarkValidationResult struct {
	BenchmarkID    string             `json:"benchmark_id"`
	BenchmarkName  string             `json:"benchmark_name"`
	ExpectedResult ExpectedResult     `json:"expected_result"`
	ActualResult   string             `json:"actual_result"` // "pass", "reject", "unexpected"
	Match          bool               `json:"match"`         // 是否符合预期
	Score          *ScoreResult       `json:"score"`
	OverfitCheck   *OverfitProtection `json:"overfit_check"`
	Notes          string             `json:"notes"`
	ValidatedAt    time.Time          `json:"validated_at"`
}

// BenchmarkValidationReport 基准验证报告
type BenchmarkValidationReport struct {
	ID          string                      `json:"id"`
	Timestamp   time.Time                   `json:"timestamp"`
	Results     []BenchmarkValidationResult `json:"results"`
	TotalCount  int                         `json:"total_count"`
	PassedCount int                         `json:"passed_count"`
	FailedCount int                         `json:"failed_count"`
	MatchRate   float64                     `json:"match_rate"`
	Summary     string                      `json:"summary"`
}

// GenerateReport 生成验证报告摘要
func (r *BenchmarkValidationReport) GenerateReport() string {
	summary := fmt.Sprintf("基准验证报告: %d/%d 通过 (匹配率 %.1f%%)\n",
		r.PassedCount, r.TotalCount, r.MatchRate*100)

	summary += "\n通过的基准:\n"
	for _, result := range r.Results {
		if result.Match && result.ExpectedResult == ExpectedPass {
			summary += fmt.Sprintf("  ✓ %s: %s\n", result.BenchmarkName, result.ActualResult)
		}
	}

	summary += "\n淘汰的基准:\n"
	for _, result := range r.Results {
		if result.Match && result.ExpectedResult == ExpectedReject {
			summary += fmt.Sprintf("  ✓ %s: %s\n", result.BenchmarkName, result.ActualResult)
		}
	}

	summary += "\n异常结果:\n"
	for _, result := range r.Results {
		if !result.Match {
			summary += fmt.Sprintf("  ✗ %s: 预期%s, 实际%s\n",
				result.BenchmarkName, result.ExpectedResult, result.ActualResult)
		}
	}

	return summary
}
