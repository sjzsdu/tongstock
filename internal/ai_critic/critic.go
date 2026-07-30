package ai_critic

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ============================================================================
// AI 研究批评者接口
// ============================================================================

// AICritic AI 研究批评者接口
type AICritic interface {
	// Review 执行审查
	Review(input ReviewInput) *ReviewOutcome
	// ReviewDimension 获取审查维度信息
	ReviewDimension() string
}

// ============================================================================
// 审查检查器接口
// ============================================================================

// ReviewChecker 单项审查检查器
type ReviewChecker interface {
	// Name 检查器名称
	Name() string
	// Dimension 审查维度
	Dimension() ReviewDimension
	// Check 执行检查, 返回问题列表
	Check(input ReviewInput) []ReviewIssue
	// IsHardThreshold 是否为硬门槛检查
	IsHardThreshold() bool
}

// ============================================================================
// AI 研究批评者实现
// ============================================================================

// CriticConfig 批评者配置
type CriticConfig struct {
	// 样本量阈值
	MinSampleSize     int `json:"min_sample_size"`      // 最小样本量
	MinTradeCount     int `json:"min_trade_count"`      // 最小交易次数
	MinHoldPeriodDays int `json:"min_hold_period_days"` // 最小持有期天数

	// 成本阈值
	MaxCostRatio      float64 `json:"max_cost_ratio"`      // 最大成本占比
	MaxSlippageBps    float64 `json:"max_slippage_bps"`    // 最大滑点 (bps)

	// 集中度阈值
	MaxSingleWeight   float64 `json:"max_single_weight"`   // 最大单票权重
	MaxConcentration  float64 `json:"max_concentration"`   // 最大集中度指数

	// 基线阈值
	MinExcessReturn   float64 `json:"min_excess_return"`   // 最小超额收益
	MinSharpeAboveBaseline float64 `json:"min_sharpe_above_baseline"` // 最小超额夏普

	// 数据泄漏检查
	MinEmbargoDays    int `json:"min_embargo_days"`    // 最小隔离期
	MinPurgeDays      int `json:"min_purge_days"`      // 最小清洗期

	// 选择偏差检查
	MinTrainRatio     float64 `json:"min_train_ratio"`    // 最小训练集比例
	MaxOverlapRatio   float64 `json:"max_overlap_ratio"`  // 最大特征重叠率

	// 叙事偏差检查
	MinHypothesisRefs  int     `json:"min_hypothesis_refs"`  // 假设最小引用数
	MaxNarrativeScore  float64 `json:"max_narrative_score"`  // 最大叙事分数 (越低越好)

	// AI 不能自行豁免硬门槛
	AICanOverrideHardThreshold bool `json:"ai_can_override_hard_threshold"` // 默认 false
}

// DefaultCriticConfig 默认批评者配置
func DefaultCriticConfig() CriticConfig {
	return CriticConfig{
		MinSampleSize:           30,
		MinTradeCount:           10,
		MinHoldPeriodDays:       5,
		MaxCostRatio:            0.30,
		MaxSlippageBps:          10,
		MaxSingleWeight:         0.15,
		MaxConcentration:        0.50,
		MinExcessReturn:         0.02,
		MinSharpeAboveBaseline:  0.20,
		MinEmbargoDays:          5,
		MinPurgeDays:            3,
		MinTrainRatio:           0.50,
		MaxOverlapRatio:         0.80,
		MinHypothesisRefs:       3,
		MaxNarrativeScore:       0.30,
		AICanOverrideHardThreshold: false, // AI 永远不能豁免硬门槛
	}
}

// ResearchCritic AI 研究批评者实现
type ResearchCritic struct {
	config   CriticConfig
	checkers []ReviewChecker
}

// NewResearchCritic 创建研究批评者
func NewResearchCritic(config CriticConfig) *ResearchCritic {
	c := &ResearchCritic{
		config: config,
	}
	c.checkers = c.buildCheckers()
	return c
}

// buildCheckers 构建所有检查器
func (c *ResearchCritic) buildCheckers() []ReviewChecker {
	return []ReviewChecker{
		NewDataLeakageChecker(c.config),
		NewSelectionBiasChecker(c.config),
		NewSampleSizeChecker(c.config),
		NewCostSensitivityChecker(c.config),
		NewConcentrationChecker(c.config),
		NewNarrativeBiasChecker(c.config),
		NewBaselineCompareChecker(c.config),
	}
}

// Review 执行完整审查
func (c *ResearchCritic) Review(input ReviewInput) *ReviewOutcome {
	outcome := NewReviewOutcome(input.TargetID, input.TargetType, "ai_critic")

	for _, checker := range c.checkers {
		issues := checker.Check(input)
		for _, issue := range issues {
			outcome.AddIssue(issue)
		}
	}

	// 完成审查 (计算结论)
	outcome.Finalize()

	return outcome
}

// ReviewDimension 返回审查维度摘要
func (c *ResearchCritic) ReviewDimension() string {
	dims := make([]string, 0, len(c.checkers))
	for _, ch := range c.checkers {
		dims = append(dims, string(ch.Dimension()))
	}
	return strings.Join(dims, ", ")
}

// AddChecker 添加自定义检查器
func (c *ResearchCritic) AddChecker(ch ReviewChecker) {
	c.checkers = append(c.checkers, ch)
}

// ============================================================================
// 1. 数据泄漏检查器
// ============================================================================

// DataLeakageChecker 数据泄漏检查器
type DataLeakageChecker struct {
	config CriticConfig
}

func NewDataLeakageChecker(cfg CriticConfig) *DataLeakageChecker {
	return &DataLeakageChecker{config: cfg}
}

func (c *DataLeakageChecker) Name() string              { return "data_leakage_checker" }
func (c *DataLeakageChecker) Dimension() ReviewDimension { return DimDataLeakage }
func (c *DataLeakageChecker) IsHardThreshold() bool     { return true }

func (c *DataLeakageChecker) Check(input ReviewInput) []ReviewIssue {
	var issues []ReviewIssue

	// 检查隔离期
	if input.Config.EmbargoDays < c.config.MinEmbargoDays {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("dl-embargo-%s", input.TargetID),
			Dimension:       DimDataLeakage,
			Severity:        SevCritical,
			Title:           "隔离期过短",
			Description:     fmt.Sprintf("隔离期仅 %d 天 (要求 >= %d 天), 可能导致前视偏差", input.Config.EmbargoDays, c.config.MinEmbargoDays),
			Evidence:        fmt.Sprintf("embargo_days=%d, threshold=%d", input.Config.EmbargoDays, c.config.MinEmbargoDays),
			Recommendation:  fmt.Sprintf("将隔离期增加到 %d 天以上", c.config.MinEmbargoDays),
			MetricName:      "embargo_days",
			MetricValue:     float64(input.Config.EmbargoDays),
			MetricThreshold: float64(c.config.MinEmbargoDays),
			IsHardThreshold: true,
			CreatedAt:       time.Now(),
		})
	}

	// 检查清洗期
	if input.Config.PurgeDays < c.config.MinPurgeDays {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("dl-purge-%s", input.TargetID),
			Dimension:       DimDataLeakage,
			Severity:        SevCritical,
			Title:           "清洗期过短",
			Description:     fmt.Sprintf("清洗期仅 %d 天 (要求 >= %d 天), 训练集可能包含验证期信息", input.Config.PurgeDays, c.config.MinPurgeDays),
			Evidence:        fmt.Sprintf("purge_days=%d, threshold=%d", input.Config.PurgeDays, c.config.MinPurgeDays),
			Recommendation:  fmt.Sprintf("将清洗期增加到 %d 天以上", c.config.MinPurgeDays),
			MetricName:      "purge_days",
			MetricValue:     float64(input.Config.PurgeDays),
			MetricThreshold: float64(c.config.MinPurgeDays),
			IsHardThreshold: true,
			CreatedAt:       time.Now(),
		})
	}

	// 检查训练/验证集是否有重叠 (简化检查)
	if input.Config.TrainRatio + input.Config.ValidRatio > 0.95 {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("dl-overlap-%s", input.TargetID),
			Dimension:       DimDataLeakage,
			Severity:        SevHigh,
			Title:           "训练/验证集比例过高",
			Description:     fmt.Sprintf("训练+验证比例 %.0f%%, 可能存在交叉泄漏", (input.Config.TrainRatio+input.Config.ValidRatio)*100),
			Evidence:        fmt.Sprintf("train=%.2f, valid=%.2f", input.Config.TrainRatio, input.Config.ValidRatio),
			Recommendation:  "减少训练或验证集比例, 留出独立测试集",
			MetricName:      "split_ratio",
			MetricValue:     input.Config.TrainRatio + input.Config.ValidRatio,
			MetricThreshold: 0.95,
			CreatedAt:       time.Now(),
		})
	}

	return issues
}

// ============================================================================
// 2. 选择偏差检查器
// ============================================================================

// SelectionBiasChecker 选择偏差检查器
type SelectionBiasChecker struct {
	config CriticConfig
}

func NewSelectionBiasChecker(cfg CriticConfig) *SelectionBiasChecker {
	return &SelectionBiasChecker{config: cfg}
}

func (c *SelectionBiasChecker) Name() string              { return "selection_bias_checker" }
func (c *SelectionBiasChecker) Dimension() ReviewDimension { return DimSelectionBias }
func (c *SelectionBiasChecker) IsHardThreshold() bool     { return false }

func (c *SelectionBiasChecker) Check(input ReviewInput) []ReviewIssue {
	var issues []ReviewIssue

	// 检查训练集比例
	if input.Config.TrainRatio < c.config.MinTrainRatio {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("sb-train-%s", input.TargetID),
			Dimension:       DimSelectionBias,
			Severity:        SevMedium,
			Title:           "训练集比例过低",
			Description:     fmt.Sprintf("训练集比例 %.0f%% (要求 >= %.0f%%), 可能导致模型不稳定", input.Config.TrainRatio*100, c.config.MinTrainRatio*100),
			Evidence:        fmt.Sprintf("train_ratio=%.2f", input.Config.TrainRatio),
			Recommendation:  fmt.Sprintf("提高训练集比例到 %.0f%% 以上", c.config.MinTrainRatio*100),
			MetricName:      "train_ratio",
			MetricValue:     input.Config.TrainRatio,
			MetricThreshold: c.config.MinTrainRatio,
			CreatedAt:       time.Now(),
		})
	}

	// 检查是否使用太多特征 (维度选择偏差)
	if input.Config.FeatureCount > 20 {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("sb-features-%s", input.TargetID),
			Dimension:       DimSelectionBias,
			Severity:        SevMedium,
			Title:           "特征数量过多",
			Description:     fmt.Sprintf("使用了 %d 个特征, 过多特征可能导致选择偏差", input.Config.FeatureCount),
			Evidence:        fmt.Sprintf("feature_count=%d", input.Config.FeatureCount),
			Recommendation:  "减少特征数量或使用特征选择方法",
			MetricName:      "feature_count",
			MetricValue:     float64(input.Config.FeatureCount),
			MetricThreshold: 20,
			CreatedAt:       time.Now(),
		})
	}

	// 检查切分类型: 固定切分 vs 滚动切分
	if input.Config.SplitType == "fixed" && input.Config.TrainRatio > 0.7 {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("sb-split-%s", input.TargetID),
			Dimension:       DimSelectionBias,
			Severity:        SevLow,
			Title:           "固定切分比例偏高",
			Description:     "固定时间切分且训练集比例较高, 可能对特定时段过拟合",
			Evidence:        fmt.Sprintf("split=fixed, train=%.2f", input.Config.TrainRatio),
			Recommendation:  "考虑使用滚动/扩展切分以检验时段鲁棒性",
			MetricName:      "split_type",
			MetricValue:     input.Config.TrainRatio,
			CreatedAt:       time.Now(),
		})
	}

	return issues
}

// ============================================================================
// 3. 样本不足检查器
// ============================================================================

// SampleSizeChecker 样本不足检查器
type SampleSizeChecker struct {
	config CriticConfig
}

func NewSampleSizeChecker(cfg CriticConfig) *SampleSizeChecker {
	return &SampleSizeChecker{config: cfg}
}

func (c *SampleSizeChecker) Name() string              { return "sample_size_checker" }
func (c *SampleSizeChecker) Dimension() ReviewDimension { return DimSampleSize }
func (c *SampleSizeChecker) IsHardThreshold() bool     { return true }

func (c *SampleSizeChecker) Check(input ReviewInput) []ReviewIssue {
	var issues []ReviewIssue

	// 检查样本量
	if input.Results.SampleSize < c.config.MinSampleSize {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("ss-sample-%s", input.TargetID),
			Dimension:       DimSampleSize,
			Severity:        SevCritical,
			Title:           "样本量不足",
			Description:     fmt.Sprintf("样本量 %d (要求 >= %d), 统计推断不可靠", input.Results.SampleSize, c.config.MinSampleSize),
			Evidence:        fmt.Sprintf("sample_size=%d, threshold=%d", input.Results.SampleSize, c.config.MinSampleSize),
			Recommendation:  fmt.Sprintf("增加样本量到 %d 以上", c.config.MinSampleSize),
			MetricName:      "sample_size",
			MetricValue:     float64(input.Results.SampleSize),
			MetricThreshold: float64(c.config.MinSampleSize),
			IsHardThreshold: true,
			CreatedAt:       time.Now(),
		})
	}

	// 检查交易次数
	if input.Results.TotalTrades < c.config.MinTradeCount {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("ss-trades-%s", input.TargetID),
			Dimension:       DimSampleSize,
			Severity:        SevCritical,
			Title:           "交易次数过少",
			Description:     fmt.Sprintf("仅 %d 笔交易 (要求 >= %d), 策略可能只是巧合", input.Results.TotalTrades, c.config.MinTradeCount),
			Evidence:        fmt.Sprintf("total_trades=%d, threshold=%d", input.Results.TotalTrades, c.config.MinTradeCount),
			Recommendation:  fmt.Sprintf("增加交易次数到 %d 以上", c.config.MinTradeCount),
			MetricName:      "total_trades",
			MetricValue:     float64(input.Results.TotalTrades),
			MetricThreshold: float64(c.config.MinTradeCount),
			IsHardThreshold: true,
			CreatedAt:       time.Now(),
		})
	}

	return issues
}

// ============================================================================
// 4. 成本敏感检查器
// ============================================================================

// CostSensitivityChecker 成本敏感检查器
type CostSensitivityChecker struct {
	config CriticConfig
}

func NewCostSensitivityChecker(cfg CriticConfig) *CostSensitivityChecker {
	return &CostSensitivityChecker{config: cfg}
}

func (c *CostSensitivityChecker) Name() string              { return "cost_sensitivity_checker" }
func (c *CostSensitivityChecker) Dimension() ReviewDimension { return DimCostSensitivity }
func (c *CostSensitivityChecker) IsHardThreshold() bool     { return true }

func (c *CostSensitivityChecker) Check(input ReviewInput) []ReviewIssue {
	var issues []ReviewIssue

	// 检查成本占比
	if input.Results.CostRatio > c.config.MaxCostRatio {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("cs-ratio-%s", input.TargetID),
			Dimension:       DimCostSensitivity,
			Severity:        SevCritical,
			Title:           "成本占比过高",
			Description:     fmt.Sprintf("成本占比 %.1f%% (上限 %.1f%%), 交易成本侵蚀大部分收益", input.Results.CostRatio*100, c.config.MaxCostRatio*100),
			Evidence:        fmt.Sprintf("cost_ratio=%.4f, threshold=%.4f", input.Results.CostRatio, c.config.MaxCostRatio),
			Recommendation:  "降低交易频率或扩大单笔收益以覆盖成本",
			MetricName:      "cost_ratio",
			MetricValue:     input.Results.CostRatio,
			MetricThreshold: c.config.MaxCostRatio,
			IsHardThreshold: true,
			CreatedAt:       time.Now(),
		})
	}

	// 检查毛利率 vs 净利率
	if input.Results.GrossReturn > 0 && input.Results.NetReturn > 0 {
		costImpact := (input.Results.GrossReturn - input.Results.NetReturn) / input.Results.GrossReturn
		if costImpact > 0.5 {
			issues = append(issues, ReviewIssue{
				ID:              fmt.Sprintf("cs-impact-%s", input.TargetID),
				Dimension:       DimCostSensitivity,
				Severity:        SevHigh,
				Title:           "成本对收益影响过大",
				Description:     fmt.Sprintf("成本吞噬了 %.0f%% 的毛收益, 策略对成本高度敏感", costImpact*100),
				Evidence:        fmt.Sprintf("gross=%.4f, net=%.4f, impact=%.2f%%", input.Results.GrossReturn, input.Results.NetReturn, costImpact*100),
				Recommendation:  "优化交易执行以降低成本敏感性",
				MetricName:      "cost_impact",
				MetricValue:     costImpact,
				MetricThreshold: 0.5,
				CreatedAt:       time.Now(),
			})
		}
	}

	return issues
}

// ============================================================================
// 5. 集中度检查器
// ============================================================================

// ConcentrationChecker 集中度检查器
type ConcentrationChecker struct {
	config CriticConfig
}

func NewConcentrationChecker(cfg CriticConfig) *ConcentrationChecker {
	return &ConcentrationChecker{config: cfg}
}

func (c *ConcentrationChecker) Name() string              { return "concentration_checker" }
func (c *ConcentrationChecker) Dimension() ReviewDimension { return DimConcentration }
func (c *ConcentrationChecker) IsHardThreshold() bool     { return false }

func (c *ConcentrationChecker) Check(input ReviewInput) []ReviewIssue {
	var issues []ReviewIssue

	// 检查最大单票权重
	if input.Results.MaxPositionWeight > c.config.MaxSingleWeight {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("conc-weight-%s", input.TargetID),
			Dimension:       DimConcentration,
			Severity:        SevHigh,
			Title:           "单票权重过高",
			Description:     fmt.Sprintf("最大单票权重 %.0f%% (上限 %.0f%%), 集中度风险高", input.Results.MaxPositionWeight*100, c.config.MaxSingleWeight*100),
			Evidence:        fmt.Sprintf("max_weight=%.4f, threshold=%.4f", input.Results.MaxPositionWeight, c.config.MaxSingleWeight),
			Recommendation:  "分散持仓, 降低单票集中度",
			MetricName:      "max_position_weight",
			MetricValue:     input.Results.MaxPositionWeight,
			MetricThreshold: c.config.MaxSingleWeight,
			CreatedAt:       time.Now(),
		})
	}

	// 检查集中度指数
	if input.Results.Concentration > c.config.MaxConcentration {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("conc-index-%s", input.TargetID),
			Dimension:       DimConcentration,
			Severity:        SevHigh,
			Title:           "组合集中度指数过高",
			Description:     fmt.Sprintf("集中度指数 %.2f (上限 %.2f), 策略依赖少数标的", input.Results.Concentration, c.config.MaxConcentration),
			Evidence:        fmt.Sprintf("concentration=%.4f, threshold=%.4f", input.Results.Concentration, c.config.MaxConcentration),
			Recommendation:  "扩展标的范围或使用分散化算法",
			MetricName:      "concentration",
			MetricValue:     input.Results.Concentration,
			MetricThreshold: c.config.MaxConcentration,
			CreatedAt:       time.Now(),
		})
	}

	// 检查小样本下的集中度
	if input.Results.TotalTrades > 0 {
		avgTradeSize := 1.0 / float64(input.Results.TotalTrades)
		if avgTradeSize > 0.3 {
			issues = append(issues, ReviewIssue{
				ID:              fmt.Sprintf("conc-trade-%s", input.TargetID),
				Dimension:       DimConcentration,
				Severity:        SevLow,
				Title:           "交易分布不均",
				Description:     "平均单笔交易占比过高, 少数交易贡献了大部分收益",
				Evidence:        fmt.Sprintf("total_trades=%d", input.Results.TotalTrades),
				Recommendation:  "确保策略在多个交易中表现一致",
				MetricName:      "trade_distribution",
				CreatedAt:       time.Now(),
			})
		}
	}

	return issues
}

// ============================================================================
// 6. 叙事偏差检查器
// ============================================================================

// NarrativeBiasChecker 叙事偏差检查器
type NarrativeBiasChecker struct {
	config CriticConfig
}

func NewNarrativeBiasChecker(cfg CriticConfig) *NarrativeBiasChecker {
	return &NarrativeBiasChecker{config: cfg}
}

func (c *NarrativeBiasChecker) Name() string              { return "narrative_bias_checker" }
func (c *NarrativeBiasChecker) Dimension() ReviewDimension { return DimNarrativeBias }
func (c *NarrativeBiasChecker) IsHardThreshold() bool     { return false }

func (c *NarrativeBiasChecker) Check(input ReviewInput) []ReviewIssue {
	var issues []ReviewIssue

	// 检查假设引用数 (需要足够的独立证据支持)
	if len(input.Config.FeatureIDs) > 0 && len(input.Config.FeatureIDs) < c.config.MinHypothesisRefs {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("nar-refs-%s", input.TargetID),
			Dimension:       DimNarrativeBias,
			Severity:        SevMedium,
			Title:           "假设引用过少",
			Description:     fmt.Sprintf("仅 %d 个特征引用 (要求 >= %d), 可能存在叙事偏差", len(input.Config.FeatureIDs), c.config.MinHypothesisRefs),
			Evidence:        fmt.Sprintf("feature_refs=%d", len(input.Config.FeatureIDs)),
			Recommendation:  "增加独立特征引用, 提供更多交叉验证",
			MetricName:      "hypothesis_refs",
			MetricValue:     float64(len(input.Config.FeatureIDs)),
			MetricThreshold: float64(c.config.MinHypothesisRefs),
			CreatedAt:       time.Now(),
		})
	}

	// 检查夏普比率是否异常高 (可能是过拟合后的叙事)
	if input.Results.SharpeRatio > 5.0 {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("nar-sharpe-%s", input.TargetID),
			Dimension:       DimNarrativeBias,
			Severity:        SevHigh,
			Title:           "夏普比率异常高",
			Description:     fmt.Sprintf("夏普比率 %.2f 异常高, 可能是叙事后置 (data snooping)", input.Results.SharpeRatio),
			Evidence:        fmt.Sprintf("sharpe=%.4f", input.Results.SharpeRatio),
			Recommendation:  "进一步进行样本外验证, 警惕过拟合",
			MetricName:      "sharpe_ratio",
			MetricValue:     input.Results.SharpeRatio,
			MetricThreshold: 5.0,
			CreatedAt:       time.Now(),
		})
	}

	// 检查胜率是否异常高
	if input.Results.WinRate > 0.85 {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("nar-winrate-%s", input.TargetID),
			Dimension:       DimNarrativeBias,
			Severity:        SevMedium,
			Title:           "胜率异常高",
			Description:     fmt.Sprintf("胜率 %.1f%% 异常高, 可能存在选择偏差", input.Results.WinRate*100),
			Evidence:        fmt.Sprintf("win_rate=%.4f", input.Results.WinRate),
			Recommendation:  "检查是否存在幸存者偏差或过度优化",
			MetricName:      "win_rate",
			MetricValue:     input.Results.WinRate,
			MetricThreshold: 0.85,
			CreatedAt:       time.Now(),
		})
	}

	// 检查最大回撤是否过小 (可能是过拟合)
	if input.Results.MaxDrawdown > -0.01 && input.Results.TotalReturn > 0.05 {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("nar-drawdown-%s", input.TargetID),
			Dimension:       DimNarrativeBias,
			Severity:        SevLow,
			Title:           "回撤过小值得关注",
			Description:     fmt.Sprintf("最大回撤 %.1f%% 但收益 %.1f%%, 可能参数过度优化", input.Results.MaxDrawdown*100, input.Results.TotalReturn*100),
			Evidence:        fmt.Sprintf("max_dd=%.4f, total_return=%.4f", input.Results.MaxDrawdown, input.Results.TotalReturn),
			Recommendation:  "检查参数敏感性, 验证在不同市场环境下的表现",
			MetricName:      "max_drawdown",
			MetricValue:     input.Results.MaxDrawdown,
			CreatedAt:       time.Now(),
		})
	}

	return issues
}

// ============================================================================
// 7. 基线比较检查器
// ============================================================================

// BaselineCompareChecker 基线比较检查器
type BaselineCompareChecker struct {
	config CriticConfig
}

func NewBaselineCompareChecker(cfg CriticConfig) *BaselineCompareChecker {
	return &BaselineCompareChecker{config: cfg}
}

func (c *BaselineCompareChecker) Name() string              { return "baseline_compare_checker" }
func (c *BaselineCompareChecker) Dimension() ReviewDimension { return DimBaselineCompare }
func (c *BaselineCompareChecker) IsHardThreshold() bool     { return false }

func (c *BaselineCompareChecker) Check(input ReviewInput) []ReviewIssue {
	var issues []ReviewIssue

	// 计算超额收益
	excessReturn := input.Results.TotalReturn - input.Results.BaselineReturn
	if math.Abs(input.Results.BaselineReturn) < 0.001 {
		// 基线收益接近 0, 使用绝对收益比较
		if input.Results.TotalReturn < c.config.MinExcessReturn {
			issues = append(issues, ReviewIssue{
				ID:              fmt.Sprintf("bl-return-%s", input.TargetID),
				Dimension:       DimBaselineCompare,
				Severity:        SevHigh,
				Title:           "绝对收益过低",
				Description:     fmt.Sprintf("总收益 %.1f%% (要求 >= %.1f%%), 无法覆盖无风险收益", input.Results.TotalReturn*100, c.config.MinExcessReturn*100),
				Evidence:        fmt.Sprintf("total_return=%.4f", input.Results.TotalReturn),
				Recommendation:  "提高策略收益目标或调整参数",
				MetricName:      "total_return",
				MetricValue:     input.Results.TotalReturn,
				MetricThreshold: c.config.MinExcessReturn,
				CreatedAt:       time.Now(),
			})
			return issues
		}
		return issues
	}

	// 计算超额夏普
	excessSharpe := input.Results.SharpeRatio - input.Results.BaselineSharpe

	if excessReturn < c.config.MinExcessReturn {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("bl-excess-%s", input.TargetID),
			Dimension:       DimBaselineCompare,
			Severity:        SevHigh,
			Title:           "超额收益不足",
			Description:     fmt.Sprintf("超额收益 %.1f%% (要求 >= %.1f%%), 未能跑赢基准", excessReturn*100, c.config.MinExcessReturn*100),
			Evidence:        fmt.Sprintf("excess_return=%.4f, total=%.4f, baseline=%.4f", excessReturn, input.Results.TotalReturn, input.Results.BaselineReturn),
			Recommendation:  "调整策略参数或选择更好的标的以获取超额收益",
			MetricName:      "excess_return",
			MetricValue:     excessReturn,
			MetricThreshold: c.config.MinExcessReturn,
			CreatedAt:       time.Now(),
		})
	}

	if excessSharpe < c.config.MinSharpeAboveBaseline {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("bl-sharpe-%s", input.TargetID),
			Dimension:       DimBaselineCompare,
			Severity:        SevMedium,
			Title:           "超额夏普不足",
			Description:     fmt.Sprintf("超额夏普 %.2f (要求 >= %.2f), 风险调整后未能跑赢基准", excessSharpe, c.config.MinSharpeAboveBaseline),
			Evidence:        fmt.Sprintf("excess_sharpe=%.4f, sharpe=%.4f, baseline_sharpe=%.4f", excessSharpe, input.Results.SharpeRatio, input.Results.BaselineSharpe),
			Recommendation:  "优化策略以在风险调整后提供更好的相对表现",
			MetricName:      "excess_sharpe",
			MetricValue:     excessSharpe,
			MetricThreshold: c.config.MinSharpeAboveBaseline,
			CreatedAt:       time.Now(),
		})
	}

	// 如果总收益为正但基线收益更高, 需要警告
	if input.Results.TotalReturn > 0 && input.Results.BaselineReturn > input.Results.TotalReturn {
		issues = append(issues, ReviewIssue{
			ID:              fmt.Sprintf("bl-underperform-%s", input.TargetID),
			Dimension:       DimBaselineCompare,
			Severity:        SevCritical,
			Title:           "策略跑输基准",
			Description:     fmt.Sprintf("策略收益 %.1f%% < 基准收益 %.1f%%, 绝对跑输", input.Results.TotalReturn*100, input.Results.BaselineReturn*100),
			Evidence:        fmt.Sprintf("total=%.4f, baseline=%.4f", input.Results.TotalReturn, input.Results.BaselineReturn),
			Recommendation:  "策略在当前时段显著跑输基准, 需要调整或暂停",
			MetricName:      "relative_performance",
			MetricValue:     input.Results.TotalReturn - input.Results.BaselineReturn,
			CreatedAt:       time.Now(),
		})
	}

	return issues
}

// ============================================================================
// 审核查询方法
// ============================================================================

// GetCheckersByDimension 按维度获取检查器
func (c *ResearchCritic) GetCheckersByDimension(dim ReviewDimension) []ReviewChecker {
	var matched []ReviewChecker
	for _, ch := range c.checkers {
		if ch.Dimension() == dim {
			matched = append(matched, ch)
		}
	}
	return matched
}

// HardThresholdCheckers 获取所有硬门槛检查器
func (c *ResearchCritic) HardThresholdCheckers() []ReviewChecker {
	var hard []ReviewChecker
	for _, ch := range c.checkers {
		if ch.IsHardThreshold() {
			hard = append(hard, ch)
		}
	}
	return hard
}

// SoftThresholdCheckers 获取所有软门槛检查器
func (c *ResearchCritic) SoftThresholdCheckers() []ReviewChecker {
	var soft []ReviewChecker
	for _, ch := range c.checkers {
		if !ch.IsHardThreshold() {
			soft = append(soft, ch)
		}
	}
	return soft
}
