package paradigms

import "time"

// SimpleProcessor is a deterministic fixture used only by pipeline tests.
type SimpleProcessor struct {
	successRate float64
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

func (p *SimpleProcessor) Process(_ *Candidate) (*TestResult, error) {
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
		CheckedAt: time.Now(),
	}, nil
}
