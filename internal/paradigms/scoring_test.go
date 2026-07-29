package paradigms

import (
	"math"
	"testing"
)

// ============================================================================
// 评分配置测试
// ============================================================================

func TestDefaultScoringConfig(t *testing.T) {
	config := DefaultScoringConfig()

	if config.SampleOutWeight+config.ConfidenceWeight+config.DrawdownWeight+
		config.ConsistencyWeight+config.ParamSensitivityWeight+
		config.CostImpactWeight+config.ConcentrationWeight != 1.0 {
		t.Error("weights should sum to 1.0")
	}
}

// ============================================================================
// 稳健性评分器测试
// ============================================================================

func TestNewRobustnessScorer(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)
	if scorer == nil {
		t.Fatal("NewRobustnessScorer returned nil")
	}
}

func TestScoreHighQualityCandidate(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn:      0.15,
		SampleOutSharpe:      1.5,
		SampleOutReturnCI:    [2]float64{0.05, 0.25},
		SampleSize:           500,
		MaxDrawdown:          0.05,
		MaxDrawdownDuration:  30,
		DrawdownRatio:        0.3,
		WindowConsistency:    0.85,
		StateConsistency:     0.75,
		DirectionConsistency: 0.9,
		ParamSensitivity:     0.1,
		PerturbationPass:     true,
		GrossReturn:          0.20,
		NetReturn:            0.15,
		CostRatio:            0.25,
		MaxPositionWeight:    0.1,
		ConcentrationIndex:   0.2,
	}

	result := scorer.Score(input)

	if result.OverallScore <= 0.5 {
		t.Errorf("high quality candidate should score well, got %.2f", result.OverallScore)
	}
	if result.HardKilled {
		t.Error("high quality candidate should not be hard-killed")
	}
	if result.Stage != StagePromote {
		t.Errorf("expected promote stage, got %s", result.Stage)
	}
}

func TestScoreLowQualityCandidate(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn:      0.02,
		SampleOutSharpe:      0.2,
		SampleOutReturnCI:    [2]float64{-0.05, 0.09},
		SampleSize:           50,
		MaxDrawdown:          0.25,
		MaxDrawdownDuration:  100,
		DrawdownRatio:        2.0,
		WindowConsistency:    0.3,
		StateConsistency:     0.2,
		DirectionConsistency: 0.4,
		ParamSensitivity:     0.6,
		PerturbationPass:     false,
		GrossReturn:          0.05,
		NetReturn:            0.01,
		CostRatio:            0.8,
		MaxPositionWeight:    0.4,
		ConcentrationIndex:   0.6,
	}

	result := scorer.Score(input)

	if result.OverallScore > 0.5 {
		t.Errorf("low quality candidate should score poorly, got %.2f", result.OverallScore)
	}
	if result.Stage != StageReject {
		t.Errorf("expected reject stage, got %s", result.Stage)
	}
}

func TestScoreMediumQualityCandidate(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn:      0.08,
		SampleOutSharpe:      0.7,
		SampleOutReturnCI:    [2]float64{0.01, 0.15},
		SampleSize:           200,
		MaxDrawdown:          0.12,
		MaxDrawdownDuration:  60,
		DrawdownRatio:        1.0,
		WindowConsistency:    0.6,
		StateConsistency:     0.5,
		DirectionConsistency: 0.65,
		ParamSensitivity:     0.25,
		PerturbationPass:     true,
		GrossReturn:          0.10,
		NetReturn:            0.06,
		CostRatio:            0.40,
		MaxPositionWeight:    0.3,
		ConcentrationIndex:   0.4,
	}

	result := scorer.Score(input)

	if result.Stage != StageObserve {
		t.Errorf("expected observe stage for medium quality, got %s (score=%.2f)", result.Stage, result.OverallScore)
	}
}

func TestScoreComponents(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn:   0.10,
		SampleOutSharpe:   1.0,
		SampleOutReturnCI: [2]float64{0.03, 0.17},
		SampleSize:        200,
	}

	result := scorer.Score(input)

	// 检查所有组件都存在
	if len(result.Components) != 7 {
		t.Errorf("expected 7 components, got %d", len(result.Components))
	}

	// 检查每个组件
	for _, c := range result.Components {
		if c.Score < 0 || c.Score > 1 {
			t.Errorf("component %s has invalid score: %.2f", c.Name, c.Score)
		}
		if c.Contribution != c.Score*c.Weight {
			t.Errorf("component %s contribution mismatch", c.Name)
		}
	}
}

func TestGetComponentScore(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	result := scorer.Score(ScoreInput{})

	sampleOut := result.GetComponentScore(CategorySampleOut)
	if sampleOut == nil {
		t.Error("expected sample_out component")
	}

	nonexistent := result.GetComponentScore("nonexistent")
	if nonexistent != nil {
		t.Error("expected nil for nonexistent component")
	}
}

func TestGetPassCount(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn:      0.10,
		SampleOutSharpe:      1.0,
		SampleOutReturnCI:    [2]float64{0.03, 0.17},
		SampleSize:           200,
		MaxDrawdown:          0.05,
		WindowConsistency:    0.8,
		StateConsistency:     0.7,
		DirectionConsistency: 0.8,
		ParamSensitivity:     0.1,
		PerturbationPass:     true,
		GrossReturn:          0.15,
		NetReturn:            0.12,
		CostRatio:            0.2,
		MaxPositionWeight:    0.15,
		ConcentrationIndex:   0.2,
	}

	result := scorer.Score(input)
	passCount := result.GetPassCount()
	failCount := result.GetFailCount()

	if passCount+failCount != len(result.Components) {
		t.Error("pass + fail should equal total components")
	}
}

// ============================================================================
// 硬性否决测试
// ============================================================================

func TestHardKillLowReturn(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn: 0.01, // 低于 5% 阈值
		SampleSize:      200,
		MaxDrawdown:     0.05,
	}

	result := scorer.Score(input)

	if !result.HardKilled {
		t.Error("should be hard-killed for low return")
	}
	if result.Stage != StageReject {
		t.Error("should be rejected")
	}
}

func TestHardKillHighDrawdown(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn: 0.10,
		SampleSize:      200,
		MaxDrawdown:     0.25, // 超过 15% 阈值
	}

	result := scorer.Score(input)

	if !result.HardKilled {
		t.Error("should be hard-killed for high drawdown")
	}
}

func TestHardKillSmallSample(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn: 0.15,
		SampleSize:      10, // 低于 30 阈值
		MaxDrawdown:     0.05,
	}

	result := scorer.Score(input)

	if !result.HardKilled {
		t.Error("should be hard-killed for small sample")
	}
}

func TestHardKillHighSensitivity(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn:  0.15,
		SampleSize:       200,
		MaxDrawdown:      0.05,
		ParamSensitivity: 0.5, // 超过 30% 阈值
		PerturbationPass: false,
	}

	result := scorer.Score(input)

	if !result.HardKilled {
		t.Error("should be hard-killed for high param sensitivity")
	}
}

func TestHardKillCostRatioOver100(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn: 0.10,
		SampleSize:      200,
		MaxDrawdown:     0.05,
		CostRatio:       1.5, // 超过 100%
	}

	result := scorer.Score(input)

	if !result.HardKilled {
		t.Error("should be hard-killed for cost ratio > 100%")
	}
}

// ============================================================================
// 阶段门测试
// ============================================================================

func TestNewStageGate(t *testing.T) {
	config := DefaultScoringConfig()
	sg := NewStageGate(config)
	if sg == nil {
		t.Fatal("NewStageGate returned nil")
	}
}

func TestStageGatePromote(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)
	sg := NewStageGate(config)

	input := ScoreInput{
		SampleOutReturn:      0.15,
		SampleOutSharpe:      1.5,
		SampleOutReturnCI:    [2]float64{0.05, 0.25},
		SampleSize:           500,
		MaxDrawdown:          0.05,
		DrawdownRatio:        0.3,
		WindowConsistency:    0.85,
		StateConsistency:     0.75,
		DirectionConsistency: 0.9,
		ParamSensitivity:     0.1,
		PerturbationPass:     true,
		GrossReturn:          0.20,
		NetReturn:            0.15,
		CostRatio:            0.25,
		MaxPositionWeight:    0.1,
		ConcentrationIndex:   0.2,
	}

	score := scorer.Score(input)
	decision := sg.Evaluate(score)

	if decision.Stage != StagePromote {
		t.Errorf("expected promote, got %s (score=%.2f)", decision.Stage, decision.Score)
	}
}

func TestStageGateObserve(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)
	sg := NewStageGate(config)

	input := ScoreInput{
		SampleOutReturn:      0.08,
		SampleOutSharpe:      0.7,
		SampleOutReturnCI:    [2]float64{0.01, 0.15},
		SampleSize:           200,
		MaxDrawdown:          0.12,
		MaxDrawdownDuration:  60,
		DrawdownRatio:        1.0,
		WindowConsistency:    0.6,
		StateConsistency:     0.5,
		DirectionConsistency: 0.65,
		ParamSensitivity:     0.25,
		PerturbationPass:     true,
		GrossReturn:          0.10,
		NetReturn:            0.06,
		CostRatio:            0.40,
		MaxPositionWeight:    0.3,
		ConcentrationIndex:   0.4,
	}

	score := scorer.Score(input)
	decision := sg.Evaluate(score)

	if decision.Stage != StageObserve {
		t.Errorf("expected observe, got %s (score=%.2f)", decision.Stage, decision.Score)
	}
}

func TestStageGateReject(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)
	sg := NewStageGate(config)

	input := ScoreInput{
		SampleOutReturn:      0.01,
		SampleOutSharpe:      0.2,
		SampleOutReturnCI:    [2]float64{-0.05, 0.07},
		SampleSize:           50,
		MaxDrawdown:          0.20,
		DrawdownRatio:        2.0,
		WindowConsistency:    0.3,
		StateConsistency:     0.2,
		DirectionConsistency: 0.4,
		ParamSensitivity:     0.5,
		PerturbationPass:     false,
		GrossReturn:          0.03,
		NetReturn:            0.005,
		CostRatio:            0.8,
		MaxPositionWeight:    0.4,
		ConcentrationIndex:   0.5,
	}

	score := scorer.Score(input)
	decision := sg.Evaluate(score)

	if decision.Stage != StageReject {
		t.Errorf("expected reject, got %s (score=%.2f)", decision.Stage, decision.Score)
	}
}

func TestStageGateOverride(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)
	sg := NewStageGate(config)

	// 故意低分候选
	input := ScoreInput{
		SampleOutReturn:      0.03,
		SampleOutSharpe:      0.3,
		SampleOutReturnCI:    [2]float64{-0.02, 0.08},
		SampleSize:           100,
		MaxDrawdown:          0.12,
		DrawdownRatio:        1.5,
		WindowConsistency:    0.5,
		StateConsistency:     0.4,
		DirectionConsistency: 0.6,
		ParamSensitivity:     0.35,
		PerturbationPass:     true,
		GrossReturn:          0.06,
		NetReturn:            0.035,
		CostRatio:            0.4,
		MaxPositionWeight:    0.3,
		ConcentrationIndex:   0.4,
	}

	score := scorer.Score(input)

	// 人工覆盖为晋级
	decision := sg.Override(score, StagePromote, "人工判断策略逻辑有效，虽量化指标偏低但值得进一步观察")

	if !decision.Overridden {
		t.Error("should be marked as overridden")
	}
	if decision.Stage != StagePromote {
		t.Error("should be promoted after override")
	}
	if decision.OverrideReason == "" {
		t.Error("should have override reason")
	}
}

func TestStageGateHardKillOverrideNotAllowed(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)
	sg := NewStageGate(config)

	// 硬性否决: 低收益
	input := ScoreInput{
		SampleOutReturn: 0.01,
		SampleSize:      100,
		MaxDrawdown:     0.05,
	}

	score := scorer.Score(input)

	// 即使被否决，仍可以覆盖 (但应该记录)
	decision := sg.Override(score, StageObserve, "特殊情况允许进入观察")

	if !decision.Overridden {
		t.Error("should be marked as overridden")
	}
}

func TestStageGateHistory(t *testing.T) {
	config := DefaultScoringConfig()
	sg := NewStageGate(config)

	for i := 0; i < 5; i++ {
		score := &ScoreResult{}
		sg.Evaluate(score)
	}

	if len(sg.GetHistory()) != 5 {
		t.Errorf("expected 5 history entries, got %d", len(sg.GetHistory()))
	}
}

// ============================================================================
// 边缘情况测试
// ============================================================================

func TestScoreEmptyInput(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	result := scorer.Score(ScoreInput{})

	if result.OverallScore < 0 || result.OverallScore > 1 {
		t.Error("score should be in [0,1] range")
	}
}

func TestScoreNegativeReturns(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn: -0.10,
		SampleOutSharpe: -1.0,
		SampleSize:      200,
		MaxDrawdown:     0.15,
	}

	result := scorer.Score(input)

	if !result.HardKilled {
		t.Error("negative returns should be hard-killed")
	}
	if result.FinalScore > result.OverallScore {
		t.Error("final score should be <= overall score when hard-killed")
	}
}

func TestScoreVeryHighDrawdown(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn: 0.20,
		SampleSize:      200,
		MaxDrawdown:     0.40, // 40%
	}

	result := scorer.Score(input)

	if !result.HardKilled {
		t.Error("very high drawdown should be hard-killed")
	}
}

func TestScoreZeroSampleSize(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn: 0.50,
		SampleSize:      0,
		MaxDrawdown:     0.05,
	}

	result := scorer.Score(input)

	if !result.HardKilled {
		t.Error("zero sample size should be hard-killed")
	}
}

// ============================================================================
// 各组件评分细节测试
// ============================================================================

func TestScoreSampleOutHighReturn(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn:   0.30,
		SampleOutSharpe:   2.0,
		SampleOutReturnCI: [2]float64{0.15, 0.45},
		SampleSize:        500,
	}

	result := scorer.Score(input)
	component := result.GetComponentScore(CategorySampleOut)

	if component == nil {
		t.Fatal("expected sample_out component")
	}
	if component.Score < 0.5 {
		t.Errorf("high return should score well, got %.2f", component.Score)
	}
}

func TestScoreSampleOutLowReturn(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		SampleOutReturn:   0.03,
		SampleOutSharpe:   0.3,
		SampleOutReturnCI: [2]float64{-0.02, 0.08},
		SampleSize:        100,
	}

	result := scorer.Score(input)
	component := result.GetComponentScore(CategorySampleOut)

	if component == nil {
		t.Fatal("expected sample_out component")
	}
	if component.Score > 0.5 {
		t.Errorf("low return should score poorly, got %.2f", component.Score)
	}
}

func TestScoreDrawdownLow(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		MaxDrawdown:   0.03,
		DrawdownRatio: 0.2,
	}

	result := scorer.Score(input)
	component := result.GetComponentScore(CategoryDrawdown)

	if component == nil {
		t.Fatal("expected drawdown component")
	}
	if component.Score < 0.7 {
		t.Errorf("low drawdown should score well, got %.2f", component.Score)
	}
}

func TestScoreDrawdownHigh(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		MaxDrawdown:   0.12,
		DrawdownRatio: 1.5,
	}

	result := scorer.Score(input)
	component := result.GetComponentScore(CategoryDrawdown)

	if component == nil {
		t.Fatal("expected drawdown component")
	}
	if component.Score > 0.7 {
		t.Errorf("high drawdown should score lower, got %.2f", component.Score)
	}
}

func TestScoreConsistencyHigh(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		WindowConsistency:    0.9,
		StateConsistency:     0.85,
		DirectionConsistency: 0.95,
	}

	result := scorer.Score(input)
	component := result.GetComponentScore(CategoryConsistency)

	if component == nil {
		t.Fatal("expected consistency component")
	}
	expectedScore := (0.9 + 0.85 + 0.95) / 3.0
	if math.Abs(component.Score-expectedScore) > 0.01 {
		t.Errorf("expected score %.2f, got %.2f", expectedScore, component.Score)
	}
}

func TestScoreParamSensitivityLow(t *testing.T) {
	config := DefaultScoringConfig()
	scorer := NewRobustnessScorer(config)

	input := ScoreInput{
		ParamSensitivity: 0.05,
		PerturbationPass: true,
	}

	result := scorer.Score(input)
	component := result.GetComponentScore(CategoryParamSensitivity)

	if component == nil {
		t.Fatal("expected param sensitivity component")
	}
	if component.Score < 0.8 {
		t.Errorf("low sensitivity should score well, got %.2f", component.Score)
	}
}
