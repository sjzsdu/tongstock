package paradigms

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ============================================================================
// 稳健性评分体系
// ============================================================================

// ScoreComponent 评分组件
type ScoreComponent struct {
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	Score        float64 `json:"score"`        // 0-1
	Weight       float64 `json:"weight"`       // 权重
	Contribution float64 `json:"contribution"` // score * weight
	Reason       string  `json:"reason"`
	Threshold    float64 `json:"threshold"`
	Pass         bool    `json:"pass"`
}

// 评分类别
const (
	CategorySampleOut        = "sample_out"        // 样本外期望
	CategoryConfidence       = "confidence"        // 置信区间
	CategoryDrawdown         = "drawdown"          // 回撤
	CategoryConsistency      = "consistency"       // 跨窗口/状态一致性
	CategoryParamSensitivity = "param_sensitivity" // 参数敏感性
	CategoryCostImpact       = "cost_impact"       // 成本影响
	CategoryConcentration    = "concentration"     // 集中度
)

// ============================================================================
// 稳健性评分配置
// ============================================================================

// ScoringConfig 评分配置
type ScoringConfig struct {
	// 权重配置
	SampleOutWeight        float64 `json:"sample_out_weight"`
	ConfidenceWeight       float64 `json:"confidence_weight"`
	DrawdownWeight         float64 `json:"drawdown_weight"`
	ConsistencyWeight      float64 `json:"consistency_weight"`
	ParamSensitivityWeight float64 `json:"param_sensitivity_weight"`
	CostImpactWeight       float64 `json:"cost_impact_weight"`
	ConcentrationWeight    float64 `json:"concentration_weight"`

	// 阈值配置
	MinSampleOutReturn  float64 `json:"min_sample_out_return"` // 最低样本外收益
	MinSharpeRatio      float64 `json:"min_sharpe_ratio"`      // 最低 Sharpe
	MaxDrawdown         float64 `json:"max_drawdown"`          // 最大允许回撤
	MinConsistency      float64 `json:"min_consistency"`       // 最低一致性
	MaxParamSensitivity float64 `json:"max_param_sensitivity"` // 最大参数敏感性
	MaxCostImpact       float64 `json:"max_cost_impact"`       // 最大成本影响
	MaxConcentration    float64 `json:"max_concentration"`     // 最大集中度

	// 阶段门阈值
	GateThresholdObserve float64 `json:"gate_observe"` // 进入观察阶段
	GateThresholdPromote float64 `json:"gate_promote"` // 进入晋级阶段
}

// DefaultScoringConfig 默认评分配置
func DefaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		SampleOutWeight:        0.20,
		ConfidenceWeight:       0.15,
		DrawdownWeight:         0.20,
		ConsistencyWeight:      0.15,
		ParamSensitivityWeight: 0.10,
		CostImpactWeight:       0.10,
		ConcentrationWeight:    0.10,

		MinSampleOutReturn:  0.05, // 5% 年化
		MinSharpeRatio:      0.5,
		MaxDrawdown:         0.15, // 15%
		MinConsistency:      0.6,
		MaxParamSensitivity: 0.3, // 30% 敏感性
		MaxCostImpact:       0.5, // 成本占收益比例
		MaxConcentration:    0.5, // 单一标的权重

		GateThresholdObserve: 0.50,
		GateThresholdPromote: 0.65,
	}
}

// ============================================================================
// 稳健性评分器
// ============================================================================

// RobustnessScorer 稳健性评分器
type RobustnessScorer struct {
	config ScoringConfig
}

// NewRobustnessScorer 创建稳健性评分器
func NewRobustnessScorer(config ScoringConfig) *RobustnessScorer {
	return &RobustnessScorer{config: config}
}

// ScoreInput 评分输入
type ScoreInput struct {
	// 基础指标
	SampleOutReturn   float64    `json:"sample_out_return"`    // 样本外收益率
	SampleOutSharpe   float64    `json:"sample_out_sharpe"`    // 样本外 Sharpe
	SampleOutReturnCI [2]float64 `json:"sample_out_return_ci"` // 样本外收益置信区间
	SampleSize        int        `json:"sample_size"`          // 样本量

	// 回撤指标
	MaxDrawdown         float64 `json:"max_drawdown"`          // 最大回撤
	MaxDrawdownDuration int     `json:"max_drawdown_duration"` // 最大回撤持续期
	DrawdownRatio       float64 `json:"drawdown_ratio"`        // 回撤/收益比

	// 一致性指标
	WindowConsistency    float64 `json:"window_consistency"`    // 跨窗口一致性
	StateConsistency     float64 `json:"state_consistency"`     // 跨市场状态一致性
	DirectionConsistency float64 `json:"direction_consistency"` // 方向一致性

	// 参数敏感性
	ParamSensitivity float64 `json:"param_sensitivity"` // 参数敏感性 (0-1)
	PerturbationPass bool    `json:"perturbation_pass"` // 扰动检验是否通过

	// 成本影响
	GrossReturn float64 `json:"gross_return"` // 毛收益
	NetReturn   float64 `json:"net_return"`   // 净收益
	CostRatio   float64 `json:"cost_ratio"`   // 成本占比

	// 集中度
	MaxPositionWeight  float64 `json:"max_position_weight"` // 最大单标的权重
	ConcentrationIndex float64 `json:"concentration_index"` // 集中度指数
}

// Score 执行评分
func (rs *RobustnessScorer) Score(input ScoreInput) *ScoreResult {
	result := &ScoreResult{
		Timestamp:  time.Now(),
		Components: make([]ScoreComponent, 0),
		HardKills:  make([]HardKillResult, 0),
	}

	// 1. 样本外期望评分
	sampleOutScore := rs.scoreSampleOut(input)
	result.Components = append(result.Components, sampleOutScore)

	// 2. 置信区间评分
	confidenceScore := rs.scoreConfidence(input)
	result.Components = append(result.Components, confidenceScore)

	// 3. 回撤评分
	drawdownScore := rs.scoreDrawdown(input)
	result.Components = append(result.Components, drawdownScore)

	// 4. 一致性评分
	consistencyScore := rs.scoreConsistency(input)
	result.Components = append(result.Components, consistencyScore)

	// 5. 参数敏感性评分
	paramScore := rs.scoreParamSensitivity(input)
	result.Components = append(result.Components, paramScore)

	// 6. 成本影响评分
	costScore := rs.scoreCostImpact(input)
	result.Components = append(result.Components, costScore)

	// 7. 集中度评分
	concentrationScore := rs.scoreConcentration(input)
	result.Components = append(result.Components, concentrationScore)

	// 计算综合分数
	result.OverallScore = rs.calculateOverallScore(result.Components)

	// 执行硬性否决
	result.HardKills = rs.checkHardKills(input)
	result.FinalScore = result.OverallScore
	result.HardKilled = len(result.HardKills) > 0

	if result.HardKilled {
		result.FinalScore = math.Min(result.OverallScore, 0.3) // 硬性否决时降低评分
	}

	// 确定阶段
	result.Stage = rs.determineStage(result)

	return result
}

// scoreSampleOut 样本外评分
func (rs *RobustnessScorer) scoreSampleOut(input ScoreInput) ScoreComponent {
	score := ScoreComponent{
		Name:      "样本外期望",
		Category:  CategorySampleOut,
		Weight:    rs.config.SampleOutWeight,
		Threshold: rs.config.MinSampleOutReturn,
	}

	// 基于收益率和 Sharpe 的综合评分
	returnScore := 0.0
	if input.SampleOutReturn > 0 {
		// 归一化: 以 50% 为满分
		returnScore = math.Min(1.0, input.SampleOutReturn/0.5)
	}

	sharpeScore := 0.0
	if input.SampleOutSharpe > 0 {
		// 归一化: 以 Sharpe=3 为满分
		sharpeScore = math.Min(1.0, input.SampleOutSharpe/3.0)
	}

	score.Score = returnScore*0.4 + sharpeScore*0.6
	score.Contribution = score.Score * score.Weight
	score.Pass = input.SampleOutReturn >= rs.config.MinSampleOutReturn
	score.Reason = fmt.Sprintf("Return=%.2f%%, Sharpe=%.2f", input.SampleOutReturn*100, input.SampleOutSharpe)

	return score
}

// scoreConfidence 置信区间评分
func (rs *RobustnessScorer) scoreConfidence(input ScoreInput) ScoreComponent {
	score := ScoreComponent{
		Name:     "置信区间",
		Category: CategoryConfidence,
		Weight:   rs.config.ConfidenceWeight,
	}

	// 基于置信区间宽度
	ciWidth := input.SampleOutReturnCI[1] - input.SampleOutReturnCI[0]
	meanReturn := (input.SampleOutReturnCI[0] + input.SampleOutReturnCI[1]) / 2

	if meanReturn != 0 {
		// 相对宽度
		relativeWidth := ciWidth / math.Abs(meanReturn)
		// 更合理的评分: 相对宽度 <= 1 得高分, > 1 逐渐降低
		if relativeWidth <= 0.5 {
			score.Score = 0.9
		} else if relativeWidth <= 1.0 {
			score.Score = 0.7
		} else if relativeWidth <= 2.0 {
			score.Score = 0.5
		} else {
			score.Score = 0.3
		}
	} else {
		score.Score = 0.4
	}

	// 基于样本量调整
	sampleFactor := 1.0
	if input.SampleSize < 50 {
		sampleFactor = 0.6
	} else if input.SampleSize < 200 {
		sampleFactor = 0.85
	}
	score.Score *= sampleFactor

	score.Contribution = score.Score * score.Weight
	score.Pass = score.Score >= 0.5
	score.Reason = fmt.Sprintf("CI width ratio=%.2f, N=%d", ciWidth/math.Abs(meanReturn+1e-9), input.SampleSize)

	return score
}

// scoreDrawdown 回撤评分
func (rs *RobustnessScorer) scoreDrawdown(input ScoreInput) ScoreComponent {
	score := ScoreComponent{
		Name:      "回撤控制",
		Category:  CategoryDrawdown,
		Weight:    rs.config.DrawdownWeight,
		Threshold: rs.config.MaxDrawdown,
	}

	// 基于最大回撤
	if input.MaxDrawdown <= 0 {
		score.Score = 1.0
	} else if input.MaxDrawdown <= rs.config.MaxDrawdown*0.5 {
		score.Score = 0.9
	} else if input.MaxDrawdown <= rs.config.MaxDrawdown {
		score.Score = 0.6
	} else {
		score.Score = math.Max(0.1, 0.5-input.MaxDrawdown)
	}

	// 基于回撤/收益比
	if input.DrawdownRatio > 1.0 {
		score.Score *= 0.5
	}

	score.Contribution = score.Score * score.Weight
	score.Pass = input.MaxDrawdown <= rs.config.MaxDrawdown
	score.Reason = fmt.Sprintf("MaxDD=%.2f%%, DD/Ratio=%.2f", input.MaxDrawdown*100, input.DrawdownRatio)

	return score
}

// scoreConsistency 一致性评分
func (rs *RobustnessScorer) scoreConsistency(input ScoreInput) ScoreComponent {
	score := ScoreComponent{
		Name:      "一致性",
		Category:  CategoryConsistency,
		Weight:    rs.config.ConsistencyWeight,
		Threshold: rs.config.MinConsistency,
	}

	// 综合窗口、状态、方向一致性
	score.Score = (input.WindowConsistency + input.StateConsistency + input.DirectionConsistency) / 3.0

	score.Contribution = score.Score * score.Weight
	score.Pass = score.Score >= rs.config.MinConsistency
	score.Reason = fmt.Sprintf("Window=%.2f, State=%.2f, Direction=%.2f",
		input.WindowConsistency, input.StateConsistency, input.DirectionConsistency)

	return score
}

// scoreParamSensitivity 参数敏感性评分
func (rs *RobustnessScorer) scoreParamSensitivity(input ScoreInput) ScoreComponent {
	score := ScoreComponent{
		Name:      "参数稳定性",
		Category:  CategoryParamSensitivity,
		Weight:    rs.config.ParamSensitivityWeight,
		Threshold: rs.config.MaxParamSensitivity,
	}

	if input.PerturbationPass {
		score.Score = 1.0 - input.ParamSensitivity
	} else {
		score.Score = math.Max(0.0, 0.5-input.ParamSensitivity)
	}

	score.Contribution = score.Score * score.Weight
	score.Pass = input.ParamSensitivity <= rs.config.MaxParamSensitivity
	score.Reason = fmt.Sprintf("Sensitivity=%.2f, PerturbationPass=%v", input.ParamSensitivity, input.PerturbationPass)

	return score
}

// scoreCostImpact 成本影响评分
func (rs *RobustnessScorer) scoreCostImpact(input ScoreInput) ScoreComponent {
	score := ScoreComponent{
		Name:      "成本效率",
		Category:  CategoryCostImpact,
		Weight:    rs.config.CostImpactWeight,
		Threshold: rs.config.MaxCostImpact,
	}

	// 成本占比
	if input.CostRatio <= 0 {
		score.Score = 1.0
	} else if input.CostRatio <= rs.config.MaxCostImpact*0.5 {
		score.Score = 0.9
	} else if input.CostRatio <= rs.config.MaxCostImpact {
		score.Score = 0.6
	} else {
		score.Score = math.Max(0.1, 1.0-input.CostRatio)
	}

	// 毛净收益比
	if input.GrossReturn > 0 && input.NetReturn > 0 {
		retention := input.NetReturn / input.GrossReturn
		if retention < 0.5 {
			score.Score *= 0.7
		}
	}

	score.Contribution = score.Score * score.Weight
	score.Pass = input.CostRatio <= rs.config.MaxCostImpact
	score.Reason = fmt.Sprintf("CostRatio=%.2f, Net/Gross=%.2f", input.CostRatio, input.NetReturn/(input.GrossReturn+1e-9))

	return score
}

// scoreConcentration 集中度评分
func (rs *RobustnessScorer) scoreConcentration(input ScoreInput) ScoreComponent {
	score := ScoreComponent{
		Name:      "分散程度",
		Category:  CategoryConcentration,
		Weight:    rs.config.ConcentrationWeight,
		Threshold: rs.config.MaxConcentration,
	}

	if input.MaxPositionWeight <= rs.config.MaxConcentration {
		score.Score = 1.0 - input.ConcentrationIndex
	} else {
		score.Score = math.Max(0.0, 1.0-input.MaxPositionWeight*2)
	}

	score.Contribution = score.Score * score.Weight
	score.Pass = input.MaxPositionWeight <= rs.config.MaxConcentration
	score.Reason = fmt.Sprintf("MaxWeight=%.2f%%, HHI=%.2f", input.MaxPositionWeight*100, input.ConcentrationIndex)

	return score
}

// calculateOverallScore 计算综合评分
func (rs *RobustnessScorer) calculateOverallScore(components []ScoreComponent) float64 {
	total := 0.0
	for _, c := range components {
		total += c.Contribution
	}
	// 归一化: 权重总和应该为 1
	return total
}

// ============================================================================
// 硬性否决机制
// ============================================================================

// HardKillResult 硬性否决结果
type HardKillResult struct {
	Reason    string  `json:"reason"`
	Category  string  `json:"category"`
	Severity  string  `json:"severity"` // "critical", "warning"
	Threshold float64 `json:"threshold"`
	Actual    float64 `json:"actual"`
}

// checkHardKills 执行硬性否决检查
func (rs *RobustnessScorer) checkHardKills(input ScoreInput) []HardKillResult {
	var kills []HardKillResult

	// 规则 1: 样本外收益为负或过低
	if input.SampleOutReturn < rs.config.MinSampleOutReturn {
		kills = append(kills, HardKillResult{
			Reason:    fmt.Sprintf("样本外收益 %.2f%% 低于阈值 %.2f%%", input.SampleOutReturn*100, rs.config.MinSampleOutReturn*100),
			Category:  CategorySampleOut,
			Severity:  "critical",
			Threshold: rs.config.MinSampleOutReturn,
			Actual:    input.SampleOutReturn,
		})
	}

	// 规则 2: 最大回撤超过阈值
	if input.MaxDrawdown > rs.config.MaxDrawdown {
		kills = append(kills, HardKillResult{
			Reason:    fmt.Sprintf("最大回撤 %.2f%% 超过阈值 %.2f%%", input.MaxDrawdown*100, rs.config.MaxDrawdown*100),
			Category:  CategoryDrawdown,
			Severity:  "critical",
			Threshold: rs.config.MaxDrawdown,
			Actual:    input.MaxDrawdown,
		})
	}

	// 规则 3: 样本量不足
	if input.SampleSize < 30 {
		kills = append(kills, HardKillResult{
			Reason:    fmt.Sprintf("样本量 %d 不足 (最低 30)", input.SampleSize),
			Category:  CategoryConfidence,
			Severity:  "critical",
			Threshold: 30,
			Actual:    float64(input.SampleSize),
		})
	}

	// 规则 4: 参数敏感性过高且扰动检验失败
	if input.ParamSensitivity > rs.config.MaxParamSensitivity && !input.PerturbationPass {
		kills = append(kills, HardKillResult{
			Reason:    fmt.Sprintf("参数敏感性 %.2f 且扰动检验失败", input.ParamSensitivity),
			Category:  CategoryParamSensitivity,
			Severity:  "critical",
			Threshold: rs.config.MaxParamSensitivity,
			Actual:    input.ParamSensitivity,
		})
	}

	// 规则 5: 成本占比过高
	if input.CostRatio > 1.0 {
		kills = append(kills, HardKillResult{
			Reason:    fmt.Sprintf("成本占收益比例 %.2f 超过 100%%", input.CostRatio),
			Category:  CategoryCostImpact,
			Severity:  "critical",
			Threshold: 1.0,
			Actual:    input.CostRatio,
		})
	}

	return kills
}

// ============================================================================
// 评分结果
// ============================================================================

// ScoreResult 评分结果
type ScoreResult struct {
	Timestamp    time.Time        `json:"timestamp"`
	Components   []ScoreComponent `json:"components"`
	OverallScore float64          `json:"overall_score"`
	FinalScore   float64          `json:"final_score"`
	HardKills    []HardKillResult `json:"hard_kills"`
	HardKilled   bool             `json:"hard_killed"`
	Stage        string           `json:"stage"` // "reject", "observe", "promote"
}

// GetComponentScore 获取特定类别评分
func (sr *ScoreResult) GetComponentScore(category string) *ScoreComponent {
	for i := range sr.Components {
		if sr.Components[i].Category == category {
			return &sr.Components[i]
		}
	}
	return nil
}

// GetPassCount 获取通过的组件数
func (sr *ScoreResult) GetPassCount() int {
	count := 0
	for _, c := range sr.Components {
		if c.Pass {
			count++
		}
	}
	return count
}

// GetFailCount 获取未通过的组件数
func (sr *ScoreResult) GetFailCount() int {
	return len(sr.Components) - sr.GetPassCount()
}

// ============================================================================
// 阶段门机制
// ============================================================================

// StageGate 阶段门
type StageGate struct {
	config  ScoringConfig
	history []ScoreResult
}

// NewStageGate 创建阶段门
func NewStageGate(config ScoringConfig) *StageGate {
	return &StageGate{
		config:  config,
		history: make([]ScoreResult, 0),
	}
}

// 阶段定义
const (
	StageReject  = "reject"  // 拒绝
	StageObserve = "observe" // 观察
	StagePromote = "promote" // 晋级
)

// GateDecision 门决策
type GateDecision struct {
	Stage          string  `json:"stage"`
	Score          float64 `json:"score"`
	GateThreshold  float64 `json:"gate_threshold"`
	Reason         string  `json:"reason"`
	Overridden     bool    `json:"overridden"`
	OverrideReason string  `json:"override_reason,omitempty"`
}

// Evaluate 评估阶段门
func (sg *StageGate) Evaluate(score *ScoreResult) *GateDecision {
	sg.history = append(sg.history, *score)

	decision := &GateDecision{
		Stage: StageReject,
		Score: score.FinalScore,
	}

	// 硬性否决: 直接拒绝
	if score.HardKilled {
		decision.Reason = fmt.Sprintf("硬性否决: %s", joinHardKillReasons(score.HardKills))
		return decision
	}

	// 晋级: 高分通过
	if score.FinalScore >= sg.config.GateThresholdPromote {
		decision.Stage = StagePromote
		decision.GateThreshold = sg.config.GateThresholdPromote
		decision.Reason = fmt.Sprintf("综合评分 %.2f >= 晋级阈值 %.2f", score.FinalScore, sg.config.GateThresholdPromote)
		return decision
	}

	// 观察: 中等分数
	if score.FinalScore >= sg.config.GateThresholdObserve {
		decision.Stage = StageObserve
		decision.GateThreshold = sg.config.GateThresholdObserve
		decision.Reason = fmt.Sprintf("综合评分 %.2f >= 观察阈值 %.2f", score.FinalScore, sg.config.GateThresholdObserve)
		return decision
	}

	// 拒绝: 低分
	decision.Stage = StageReject
	decision.Reason = fmt.Sprintf("综合评分 %.2f 低于观察阈值 %.2f", score.FinalScore, sg.config.GateThresholdObserve)
	return decision
}

// Override 人工覆盖决策
func (sg *StageGate) Override(score *ScoreResult, newStage string, reason string) *GateDecision {
	decision := sg.Evaluate(score)
	decision.Overridden = true
	decision.OverrideReason = reason
	decision.Stage = newStage
	decision.Reason = fmt.Sprintf("人工覆盖: %s", reason)
	return decision
}

// GetHistory 获取决策历史
func (sg *StageGate) GetHistory() []ScoreResult {
	return sg.history
}

// ============================================================================
// 辅助函数
// ============================================================================

func (rs *RobustnessScorer) determineStage(score *ScoreResult) string {
	if score.HardKilled {
		return StageReject
	}
	if score.FinalScore >= rs.config.GateThresholdPromote {
		return StagePromote
	}
	if score.FinalScore >= rs.config.GateThresholdObserve {
		return StageObserve
	}
	return StageReject
}

func joinHardKillReasons(kills []HardKillResult) string {
	if len(kills) == 0 {
		return ""
	}
	reasons := make([]string, len(kills))
	for i, k := range kills {
		reasons[i] = k.Reason
	}
	return strings.Join(reasons, "; ")
}
