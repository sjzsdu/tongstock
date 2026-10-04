package validation

import (
	"strings"

	"github.com/sjzsdu/tongstock/internal/ai_critic"
	"github.com/sjzsdu/tongstock/internal/backtest"
)

// ============================================================================
// Critic 集成 — 把 ai_critic.ReviewOutcome 映射为 validation 的 CriticIssue / PromotionBlocker
// ============================================================================

// CriticInput 是构造 ai_critic 审查输入所需的聚合数据。
type CriticInput struct {
	Job               ValidationJob
	Stats             PerformanceStats
	Split             *backtest.SplitResult
	FeatureCount      int
	ObservationCount  int
	MaxPositionWeight float64
	Concentration     float64
}

// RunCritic 调用 ai_critic 执行反证，返回 (issues, blockers)。
// config 为 nil 时使用默认严格配置。
func RunCritic(in CriticInput, config *ai_critic.CriticConfig) ([]CriticIssue, []PromotionBlocker) {
	cfg := ai_critic.DefaultCriticConfig()
	if config != nil {
		cfg = *config
	}
	critic := ai_critic.NewResearchCritic(cfg)

	reviewIn := buildReviewInput(in)
	outcome := critic.Review(reviewIn)

	issues := make([]CriticIssue, 0, len(outcome.Issues))
	var blockers []PromotionBlocker
	for _, r := range outcome.Issues {
		issues = append(issues, CriticIssue{
			Dimension: string(r.Dimension),
			Severity:  string(r.Severity),
			Code:      r.ID,
			Message:   r.Title + ": " + r.Description,
		})
		// 硬门槛或 critical/high 严重度 → PromotionBlocker
		if r.IsHardThresholdIssue() || r.Severity == ai_critic.SevCritical || r.Severity == ai_critic.SevHigh {
			severity := "soft"
			if r.IsHardThresholdIssue() || r.Severity == ai_critic.SevCritical {
				severity = "hard"
			}
			blockers = append(blockers, PromotionBlocker{
				Code:        r.ID,
				Dimension:   string(r.Dimension),
				Severity:    severity,
				Description: r.Title + ": " + r.Description,
			})
		}
	}
	return issues, blockers
}

func buildReviewInput(in CriticInput) ai_critic.ReviewInput {
	trainRatio := 0.7
	validRatio := 0.15
	embargoDays := 5
	purgeDays := 5
	splitType := "fixed"
	if in.Split != nil {
		splitType = "walk_forward"
		embargoDays = backtest.DefaultWalkForwardConfig().EmbargoDays
		purgeDays = backtest.DefaultWalkForwardConfig().PurgeDays
	}
	if in.Job.SplitType != "" {
		splitType = in.Job.SplitType
	}

	return ai_critic.ReviewInput{
		TargetID:   in.Job.MethodHash,
		TargetType: "method_validation",
		Config: ai_critic.ReviewConfig{
			SplitType:      splitType,
			TrainRatio:     trainRatio,
			ValidRatio:     validRatio,
			EmbargoDays:    embargoDays,
			PurgeDays:      purgeDays,
			FeatureCount:   in.FeatureCount,
			DataSnapshotID: in.Job.SnapshotID,
		},
		Results: ai_critic.ReviewResults{
			SampleSize:        in.ObservationCount,
			SharpeRatio:       in.Stats.SharpeRatio,
			SortinoRatio:      in.Stats.SortinoRatio,
			MaxDrawdown:       in.Stats.MaxDrawdown,
			TotalReturn:       in.Stats.TotalReturn,
			WinRate:           in.Stats.WinRate,
			TotalTrades:       in.Stats.TotalTrades,
			ProfitFactor:      in.Stats.ProfitFactor,
			GrossReturn:       in.Stats.TotalReturn + in.Stats.TotalCost/1_000_000,
			NetReturn:         in.Stats.TotalReturn,
			CostRatio:         in.Stats.CostRatio,
			BaselineReturn:    in.Stats.BenchmarkReturn,
			BaselineSharpe:    0, // 基准夏普由外部填充时再补
			MaxPositionWeight: in.MaxPositionWeight,
			Concentration:     in.Concentration,
		},
	}
}

// ============================================================================
// 置信度计算
// ============================================================================

// ConfidenceInput 计算置信度所需的全部输入。
type ConfidenceInput struct {
	Stats           PerformanceStats
	Blockers        []PromotionBlocker
	CriticIssues    []CriticIssue
	MultipleTesting MultipleTestingResult
	OosTradeCount   int
}

// ExplainConfidence 返回置信度决策分支的机器可读原因码。
// 与 ComputeConfidence 共用同一套分支判断，保证原因与结论永不漂移。
// 返回值形如 "hard_blocker:xxx" / "insufficient_oos_trades" / "strong_evidence"。
func ExplainConfidence(in ConfidenceInput) string {
	// 1. 硬门槛
	for _, b := range in.Blockers {
		if b.Severity == "hard" {
			return "hard_blocker:" + b.Code
		}
	}

	// 2. 数据不足
	if in.OosTradeCount < 8 {
		return "insufficient_oos_trades"
	}
	if in.Stats.TotalTrades == 0 {
		return "no_trades"
	}

	// 3. 多重检验
	if in.MultipleTesting.Trials > 1 && !in.MultipleTesting.Significant {
		return "multiple_testing_not_significant"
	}

	// 4. 样本外表现下限
	if in.Stats.SharpeRatio < 0.5 {
		return "oos_sharpe_below_threshold"
	}
	if in.Stats.MaxDrawdown > 0.40 {
		return "oos_max_drawdown_above_threshold"
	}

	// 5. soft blocker
	for _, b := range in.Blockers {
		if b.Severity == "soft" {
			return "soft_blocker:" + b.Code
		}
	}

	// 6. 强置信
	return "strong_evidence"
}

// ComputeConfidence 根据硬门槛、样本外表现、多重检验和 critic 反证计算最终置信等级。
// 决策顺序：
//  1. 存在 hard blocker → rejected
//  2. 样本外交易 < 8 或数据不足 → insufficient
//  3. 多重检验后不显著 → rejected
//  4. 样本外夏普 < 0.5 或最大回撤 > 40% → weak
//  5. 存在 soft blocker → moderate
//  6. 否则 → strong
//
// 具体命中哪个分支由 ExplainConfidence 给出，两者共享判断避免漂移。
func ComputeConfidence(in ConfidenceInput) (ConfidenceLevel, bool) {
	switch reason := ExplainConfidence(in); {
	case reason == "strong_evidence":
		return ConfidenceStrong, true
	case reason == "insufficient_oos_trades" || reason == "no_trades":
		return ConfidenceInsufficient, false
	case reason == "multiple_testing_not_significant":
		return ConfidenceRejected, false
	case reason == "oos_sharpe_below_threshold" || reason == "oos_max_drawdown_above_threshold":
		return ConfidenceWeak, false
	case strings.HasPrefix(reason, "hard_blocker"):
		return ConfidenceRejected, false
	default: // soft_blocker:*
		return ConfidenceModerate, true
	}
}

// HasHardBlocker 是否存在硬门槛阻断。
func HasHardBlocker(blockers []PromotionBlocker) bool {
	for _, b := range blockers {
		if b.Severity == "hard" {
			return true
		}
	}
	return false
}
