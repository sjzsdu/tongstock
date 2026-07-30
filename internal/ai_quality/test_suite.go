// Package ai_quality implements the AI Research Quality Evaluation Suite
// and Regression Gate for TongStock. It tests tool selection, numerical
// references, refutation capability, hallucination detection, and compliance
// expression across known-good, known-trap, data-insufficient, and adversarial
// test cases.
//
// Design goals:
//   - Auto-runnable: evaluation can run automatically
//   - Critical errors block releases
//   - Records model/prompt version changes
//   - Covers baseline passing and rejected paradigms
package ai_quality

import (
	"fmt"
	"time"
)

// ============================================================================
// 测试用例分类
// ============================================================================

// TestCategory 测试用例分类
type TestCategory string

const (
	// CategoryKnownGood 已知通过: 所有组件都能正确处理
	CategoryKnownGood TestCategory = "known_good"
	// CategoryKnownTrap 已知陷阱: 应被批评者捕获
	CategoryKnownTrap TestCategory = "known_trap"
	// CategoryDataInsufficient 数据不足: 应正确拒绝
	CategoryDataInsufficient TestCategory = "data_insufficient"
	// CategoryAdversarial 对抗样例: AI 容易出错的情况
	CategoryAdversarial TestCategory = "adversarial"
)

// TestExpectation 测试期望
type TestExpectation struct {
	// ExpectConclusion 期望的审查结论 (至少一个匹配)
	ExpectConclusions []string `json:"expect_conclusions"`
	// ExpectDimensionCounts 期望各维度的问题数量 (维度 → 最小数量)
	ExpectDimensionCounts map[string]int `json:"expect_dimension_counts,omitempty"`
	// MustHave 必须存在的检查项
	MustHave []string `json:"must_have,omitempty"`
	// MustNotHave 禁止存在的检查项
	MustNotHave []string `json:"must_not_have,omitempty"`
}

// TestCase 单个测试用例
type TestCase struct {
	ID            string            `json:"id"`
	Category      TestCategory      `json:"category"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Input         TestCaseInput     `json:"input"`
	Expectation   TestExpectation   `json:"expectation"`
	IsCritical    bool              `json:"is_critical"`    // 关键错误会阻止发布
	SeverityWeight float64          `json:"severity_weight"` // 权重 (影响分数)
	CreatedAt     time.Time         `json:"created_at"`
}

// TestCaseInput 测试用例输入
type TestCaseInput struct {
	TargetID   string         `json:"target_id"`
	TargetType string         `json:"target_type"`
	// 候选/范式元数据
	SchemaID   string         `json:"schema_id,omitempty"`
	SchemaName string         `json:"schema_name,omitempty"`
	// 实验配置
	SplitType      string         `json:"split_type"`
	TrainRatio     float64        `json:"train_ratio"`
	EmbargoDays    int            `json:"embargo_days"`
	PurgeDays      int            `json:"purge_days"`
	FeatureCount   int            `json:"feature_count"`
	FeatureIDs     []string       `json:"feature_ids"`
	DataSnapshotID string         `json:"data_snapshot_id"`
	// 实验结果
	SampleSize        int     `json:"sample_size"`
	SharpeRatio       float64 `json:"sharpe_ratio"`
	MaxDrawdown       float64 `json:"max_drawdown"`
	TotalReturn       float64 `json:"total_return"`
	WinRate           float64 `json:"win_rate"`
	TotalTrades       int     `json:"total_trades"`
	CostRatio         float64 `json:"cost_ratio"`
	MaxPositionWeight float64 `json:"max_position_weight"`
	Concentration     float64 `json:"concentration"`
	BaselineReturn    float64 `json:"baseline_return"`
	BaselineSharpe    float64 `json:"baseline_sharpe"`
	// 额外测试参数
	AdversarialPattern string  `json:"adversarial_pattern,omitempty"`
}

// ============================================================================
// 测试用例库
// ============================================================================

// TestCaseLibrary 测试用例库
type TestCaseLibrary struct {
	cases []TestCase
}

// NewTestCaseLibrary 创建默认测试用例库
func NewTestCaseLibrary() *TestCaseLibrary {
	lib := &TestCaseLibrary{
		cases: make([]TestCase, 0),
	}
	lib.cases = lib.createDefaultCases()
	return lib
}

// GetCases 获取所有测试用例
func (lib *TestCaseLibrary) GetCases() []TestCase {
	return lib.cases
}

// GetCriticalCases 获取关键测试用例
func (lib *TestCaseLibrary) GetCriticalCases() []TestCase {
	var critical []TestCase
	for _, c := range lib.cases {
		if c.IsCritical {
			critical = append(critical, c)
		}
	}
	return critical
}

// GetByCategory 按分类获取
func (lib *TestCaseLibrary) GetByCategory(cat TestCategory) []TestCase {
	var matched []TestCase
	for _, c := range lib.cases {
		if c.Category == cat {
			matched = append(matched, c)
		}
	}
	return matched
}

// Count 测试用例数量
func (lib *TestCaseLibrary) Count() int {
	return len(lib.cases)
}

// ============================================================================
// 默认测试用例
// ============================================================================

func (lib *TestCaseLibrary) createDefaultCases() []TestCase {
	now := time.Now()

	return []TestCase{
		// ====================================================================
		// Category 1: Known Good (已知通过)
		// ====================================================================
		{
			ID:          "kg-ma-cross",
			Category:    CategoryKnownGood,
			Name:        "双均线交叉 - 基线通过",
			Description: "经典双均线交叉策略, 符合所有审查标准",
			IsCritical:  true,
			SeverityWeight: 1.0,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-ma-cross",
				TargetType: "candidate",
				SchemaID:   "ma_cross_v1",
				SchemaName: "双均线交叉",
				SplitType:      "rolling",
				TrainRatio:     0.70,
				EmbargoDays:    10,
				PurgeDays:      7,
				FeatureCount:   5,
				FeatureIDs:     []string{"ma5", "ma20", "close", "volume", "atr"},
				DataSnapshotID: "snap-v1",
				SampleSize:        200,
				SharpeRatio:       1.8,
				MaxDrawdown:       -0.10,
				TotalReturn:       0.12,
				WinRate:           0.55,
				TotalTrades:       200,
				CostRatio:         0.20,
				MaxPositionWeight: 0.10,
				Concentration:     0.25,
				BaselineReturn:    0.06,
				BaselineSharpe:    0.9,
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"pass", "pass_notes"},
			},
		},
		{
			ID:          "kg-rsi-reversal",
			Category:    CategoryKnownGood,
			Name:        "RSI 超卖反弹 - 基线通过",
			Description: "RSI 超卖反弹策略, 稳定的均值回归范式",
			IsCritical:  true,
			SeverityWeight: 1.0,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-rsi-rev",
				TargetType: "candidate",
				SchemaID:   "rsi_reversal_v1",
				SchemaName: "RSI 超卖反弹",
				SplitType:      "rolling",
				TrainRatio:     0.65,
				EmbargoDays:    8,
				PurgeDays:      5,
				FeatureCount:   6,
				FeatureIDs:     []string{"rsi14", "close", "ma20", "volume", "stoch", "cci"},
				DataSnapshotID: "snap-v1",
				SampleSize:        150,
				SharpeRatio:       2.2,
				MaxDrawdown:       -0.08,
				TotalReturn:       0.15,
				WinRate:           0.58,
				TotalTrades:       120,
				CostRatio:         0.18,
				MaxPositionWeight: 0.08,
				Concentration:     0.20,
				BaselineReturn:    0.05,
				BaselineSharpe:    0.85,
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"pass", "pass_notes"},
			},
		},
		// ====================================================================
		// Category 2: Known Trap (已知陷阱 - 应被批评者捕获)
		// ====================================================================
		{
			ID:          "kt-overfit-leakage",
			Category:    CategoryKnownTrap,
			Name:        "数据泄漏 - 隔离期不足",
			Description: "策略使用了过短的隔离期, 存在前视偏差",
			IsCritical:  true,
			SeverityWeight: 2.0,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-leakage",
				TargetType: "candidate",
				SchemaID:   "leaky-schema",
				SplitType:      "fixed",
				TrainRatio:     0.80,
				EmbargoDays:    0,  // 无隔离期!
				PurgeDays:      0,  // 无清洗期!
				FeatureCount:   15,
				FeatureIDs:     []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"},
				DataSnapshotID: "snap-v1",
				SampleSize:        500,
				SharpeRatio:       10.0, // 异常高
				MaxDrawdown:       -0.005,
				TotalReturn:       0.50,
				WinRate:           0.95, // 异常高
				TotalTrades:       500,
				CostRatio:         0.10,
				MaxPositionWeight: 0.10,
				Concentration:     0.20,
				BaselineReturn:    0.06,
				BaselineSharpe:    0.9,
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"block"},
				ExpectDimensionCounts: map[string]int{
					"data_leakage": 2, // 隔离期和清洗期都应被捕获
				},
				MustHave: []string{"data_leakage"},
			},
		},
		{
			ID:          "kt-selection-bias",
			Category:    CategoryKnownTrap,
			Name:        "选择偏差 - 过多特征",
			Description: "使用了 50 个特征, 存在严重的维度选择偏差",
			IsCritical:  true,
			SeverityWeight: 2.0,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-bias",
				TargetType: "candidate",
				SchemaID:   "bias-schema",
				SplitType:      "fixed",
				TrainRatio:     0.95,
				EmbargoDays:    5,
				PurgeDays:      3,
				FeatureCount:   50, // 过多特征!
				FeatureIDs:     manyFeatures(50),
				DataSnapshotID: "snap-v1",
				SampleSize:        100,
				SharpeRatio:       5.0,
				MaxDrawdown:       -0.05,
				TotalReturn:       0.20,
				WinRate:           0.70,
				TotalTrades:       80,
				CostRatio:         0.15,
				MaxPositionWeight: 0.10,
				Concentration:     0.30,
				BaselineReturn:    0.08,
				BaselineSharpe:    1.0,
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"fail", "needs_review"},
				MustHave: []string{"selection_bias"},
			},
		},
		{
			ID:          "kt-cost-sensitivity",
			Category:    CategoryKnownTrap,
			Name:        "成本敏感 - 高频低收益",
			Description: "交易成本占比过高, 策略在实际中无利可图",
			IsCritical:  true,
			SeverityWeight: 2.0,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-cost",
				TargetType: "candidate",
				SchemaID:   "cost-schema",
				SplitType:      "rolling",
				TrainRatio:     0.70,
				EmbargoDays:    5,
				PurgeDays:      3,
				FeatureCount:   8,
				FeatureIDs:     []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8"},
				DataSnapshotID: "snap-v1",
				SampleSize:        300,
				SharpeRatio:       0.8,
				MaxDrawdown:       -0.15,
				TotalReturn:       0.08,
				WinRate:           0.52,
				TotalTrades:       500,
				CostRatio:         0.55, // 成本占比过高!
				MaxPositionWeight: 0.05,
				Concentration:     0.40,
				BaselineReturn:    0.05,
				BaselineSharpe:    0.8,
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"block"},
				ExpectDimensionCounts: map[string]int{
					"cost_sensitivity": 1,
				},
				MustHave: []string{"cost_sensitivity"},
			},
		},
		{
			ID:          "kt-concentration",
			Category:    CategoryKnownTrap,
			Name:        "集中度 - 单票高权重",
			Description: "策略严重依赖少数股票, 集中度风险极高",
			IsCritical:  false,
			SeverityWeight: 1.5,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-conc",
				TargetType: "candidate",
				SchemaID:   "conc-schema",
				SplitType:      "rolling",
				TrainRatio:     0.70,
				EmbargoDays:    5,
				PurgeDays:      3,
				FeatureCount:   8,
				FeatureIDs:     []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8"},
				DataSnapshotID: "snap-v1",
				SampleSize:        100,
				SharpeRatio:       1.2,
				MaxDrawdown:       -0.20,
				TotalReturn:       0.10,
				WinRate:           0.60,
				TotalTrades:       50,
				CostRatio:         0.25,
				MaxPositionWeight: 0.40, // 单票权重过高!
				Concentration:     0.75, // 集中度过高!
				BaselineReturn:    0.08,
				BaselineSharpe:    1.0,
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"fail", "needs_review"},
				MustHave: []string{"concentration"},
			},
		},
		// ====================================================================
		// Category 3: Data Insufficient (数据不足 - 应正确拒绝)
		// ====================================================================
		{
			ID:          "di-small-sample",
			Category:    CategoryDataInsufficient,
			Name:        "样本不足 - 仅 5 个样本",
			Description: "回测仅有 5 个样本, 统计推断完全不可靠",
			IsCritical:  true,
			SeverityWeight: 2.0,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-small",
				TargetType: "candidate",
				SchemaID:   "small-schema",
				SplitType:      "fixed",
				TrainRatio:     0.80,
				EmbargoDays:    5,
				PurgeDays:      3,
				FeatureCount:   5,
				FeatureIDs:     []string{"f1", "f2", "f3", "f4", "f5"},
				DataSnapshotID: "snap-v1",
				SampleSize:        5, // 极少!
				SharpeRatio:       3.0,
				MaxDrawdown:       -0.05,
				TotalReturn:       0.15,
				WinRate:           0.80,
				TotalTrades:       2, // 极少交易!
				CostRatio:         0.15,
				MaxPositionWeight: 0.10,
				Concentration:     0.30,
				BaselineReturn:    0.06,
				BaselineSharpe:    0.9,
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"block"},
				MustHave: []string{"sample_size"},
			},
		},
		{
			ID:          "di-few-trades",
			Category:    CategoryDataInsufficient,
			Name:        "交易稀少 - 仅 3 笔交易",
			Description: "策略几乎不交易, 结果可能只是巧合",
			IsCritical:  true,
			SeverityWeight: 2.0,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-few",
				TargetType: "candidate",
				SchemaID:   "few-schema",
				SplitType:      "rolling",
				TrainRatio:     0.70,
				EmbargoDays:    5,
				PurgeDays:      3,
				FeatureCount:   5,
				FeatureIDs:     []string{"f1", "f2", "f3", "f4", "f5"},
				DataSnapshotID: "snap-v1",
				SampleSize:        200,
				SharpeRatio:       2.5,
				MaxDrawdown:       -0.03,
				TotalReturn:       0.20,
				WinRate:           1.0, // 完美胜率但只有 3 笔!
				TotalTrades:       3,
				CostRatio:         0.05,
				MaxPositionWeight: 0.30,
				Concentration:     0.80,
				BaselineReturn:    0.05,
				BaselineSharpe:    0.85,
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"block"},
				MustHave: []string{"sample_size"},
			},
		},
		// ====================================================================
		// Category 4: Adversarial (对抗样例 - AI 容易出错)
		// ====================================================================
		{
			ID:          "ad-hallucination",
			Category:    CategoryAdversarial,
			Name:        "对抗 - 异常夏普 (可能幻觉)",
			Description: "夏普比率高达 15, AI 可能错误地将其视为优异",
			IsCritical:  true,
			SeverityWeight: 2.0,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-halluc",
				TargetType: "candidate",
				SchemaID:   "halluc-schema",
				SplitType:      "rolling",
				TrainRatio:     0.70,
				EmbargoDays:    10,
				PurgeDays:      7,
				FeatureCount:   10,
				FeatureIDs:     manyFeatures(10),
				DataSnapshotID: "snap-v1",
				SampleSize:        300,
				SharpeRatio:       15.0, // 极端异常!
				MaxDrawdown:       -0.02,
				TotalReturn:       0.40,
				WinRate:           0.92,
				TotalTrades:       200,
				CostRatio:         0.10,
				MaxPositionWeight: 0.08,
				Concentration:     0.15,
				BaselineReturn:    0.06,
				BaselineSharpe:    0.9,
				AdversarialPattern: "extreme_sharpe",
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"fail", "needs_review"},
				MustHave: []string{"narrative_bias"},
			},
		},
		{
			ID:          "ad-baseline-underperform",
			Category:    CategoryAdversarial,
			Name:        "对抗 - 跑输基准但 AI 可能误判",
			Description: "策略收益为正但低于基准, AI 可能忽略基线比较",
			IsCritical:  false,
			SeverityWeight: 1.5,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-under",
				TargetType: "candidate",
				SchemaID:   "under-schema",
				SplitType:      "rolling",
				TrainRatio:     0.70,
				EmbargoDays:    8,
				PurgeDays:      5,
				FeatureCount:   6,
				FeatureIDs:     []string{"f1", "f2", "f3", "f4", "f5", "f6"},
				DataSnapshotID: "snap-v1",
				SampleSize:        200,
				SharpeRatio:       0.6,
				MaxDrawdown:       -0.12,
				TotalReturn:       0.03, // 低收益
				WinRate:           0.50,
				TotalTrades:       100,
				CostRatio:         0.20,
				MaxPositionWeight: 0.05,
				Concentration:     0.15,
				BaselineReturn:    0.08, // 跑输基准!
				BaselineSharpe:    1.2,  // 夏普也低于基准!
				AdversarialPattern: "baseline_mismatch",
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"fail", "block", "needs_review"},
				MustHave: []string{"baseline_compare"},
			},
		},
		{
			ID:          "ad-sneaky-overfit",
			Category:    CategoryAdversarial,
			Name:        "对抗 - 隐蔽过拟合 (参数微调)",
			Description: "隔离期足够但特征数多+样本量中等, 隐蔽的过拟合",
			IsCritical:  true,
			SeverityWeight: 2.0,
			CreatedAt:   now,
			Input: TestCaseInput{
				TargetID:   "cand-sneaky",
				TargetType: "candidate",
				SchemaID:   "sneaky-schema",
				SplitType:      "fixed",
				TrainRatio:     0.75,
				EmbargoDays:    5,
				PurgeDays:      3,
				FeatureCount:   30,
				FeatureIDs:     manyFeatures(30),
				DataSnapshotID: "snap-v1",
				SampleSize:        80,
				SharpeRatio:       4.5, // 高得可疑
				MaxDrawdown:       -0.02, // 回撤小得可疑
				TotalReturn:       0.35,
				WinRate:           0.88,
				TotalTrades:       40,
				CostRatio:         0.05,
				MaxPositionWeight: 0.05,
				Concentration:     0.40,
				BaselineReturn:    0.06,
				BaselineSharpe:    0.9,
				AdversarialPattern: "sneaky_overfit",
			},
			Expectation: TestExpectation{
				ExpectConclusions: []string{"fail", "needs_review"},
				MustHave: []string{"narrative_bias"},
			},
		},
	}
}

// manyFeatures 生成 N 个特征 ID
func manyFeatures(n int) []string {
	features := make([]string, n)
	for i := 0; i < n; i++ {
		features[i] = fmt.Sprintf("feat_%02d", i+1)
	}
	return features
}
