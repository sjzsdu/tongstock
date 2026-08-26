package validation

import (
	"math"
	"testing"
	"time"
)

// ============================================================================
// 统计检验层单元测试
// ============================================================================

func TestTTestOnReturns(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		want    float64 // 期望 p 值范围: "high" >0.3, "low" <0.05, "one" =1.0
		bucket  string
	}{
		{"empty", nil, 1.0, "one"},
		{"single", []float64{0.05}, 1.0, "one"},
		{"all_zero", []float64{0, 0, 0, 0}, 1.0, "one"},
		{"strong_positive", []float64{0.05, 0.04, 0.06, 0.05, 0.05, 0.04, 0.06, 0.05, 0.05, 0.05}, 0.0, "low"},
		{"noisy", []float64{0.05, -0.04, 0.06, -0.05, 0.02, -0.03, 0.01, -0.02, 0.04, -0.01}, 0.3, "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := TTestOnReturns(tt.returns)
			switch tt.bucket {
			case "one":
				if p != 1.0 {
					t.Errorf("p=%.4f, want 1.0", p)
				}
			case "low":
				if p > 0.05 {
					t.Errorf("p=%.4f, want < 0.05 (significant)", p)
				}
			case "high":
				if p < 0.3 {
					t.Errorf("p=%.4f, want >= 0.3 (not significant)", p)
				}
			}
		})
	}
}

func TestApplyMultipleTesting(t *testing.T) {
	// trials=1: 不惩罚
	mt := ApplyMultipleTesting(1, 0.03)
	if mt.BonferroniAlpha != 0.05 {
		t.Errorf("trials=1 alpha=%.4f, want 0.05", mt.BonferroniAlpha)
	}
	if !mt.Significant {
		t.Error("trials=1 p=0.03 should be significant")
	}

	// trials=10, p=0.03: Bonferroni 校正后 0.3, 不显著
	mt = ApplyMultipleTesting(10, 0.03)
	if mt.BonferroniAlpha != 0.005 {
		t.Errorf("trials=10 alpha=%.4f, want 0.005", mt.BonferroniAlpha)
	}
	if mt.Significant {
		t.Error("trials=10 p=0.03 adjusted=0.3 should NOT be significant")
	}
	if math.Abs(mt.AdjustedPValue-0.3) > 1e-9 {
		t.Errorf("adjusted p=%.4f, want 0.3", mt.AdjustedPValue)
	}

	// trials=10, p=0.001: 校正后 0.01, 显著
	mt = ApplyMultipleTesting(10, 0.001)
	if !mt.Significant {
		t.Error("trials=10 p=0.001 adjusted=0.01 should be significant")
	}
}

func TestPlanSegments_Fixed(t *testing.T) {
	// 生成 100 个交易日
	dates := generateDates("2023-01-01", 100)
	plan, err := PlanSegments(dates, "fixed")
	if err != nil {
		t.Fatalf("PlanSegments fixed: %v", err)
	}
	if plan.IsWalkForward {
		t.Error("fixed split should not be walk-forward")
	}
	if len(plan.Segments) < 3 {
		t.Errorf("fixed split should produce >=3 segments, got %d", len(plan.Segments))
	}
	// 验证数据隔离
	if plan.Split != nil {
		if err := plan.Split.ValidateDataIsolation(); err != nil {
			t.Errorf("data isolation: %v", err)
		}
	}
}

func TestPlanSegments_WalkForward(t *testing.T) {
	// 生成 800 个交易日 (约 3 年)，足够 walk-forward
	dates := generateDates("2021-01-01", 800)
	plan, err := PlanSegments(dates, "walk_forward")
	if err != nil {
		t.Fatalf("PlanSegments walk_forward: %v", err)
	}
	if !plan.IsWalkForward {
		t.Error("should be walk-forward")
	}
	if len(plan.Segments) < 1 {
		t.Errorf("walk-forward should produce >=1 segment, got %d", len(plan.Segments))
	}
}

func TestPlanSegments_InsufficientData(t *testing.T) {
	dates := generateDates("2023-01-01", 20)
	_, err := PlanSegments(dates, "fixed")
	if err == nil {
		t.Error("should fail with <30 dates")
	}
}

func TestPlanSegments_UnknownSplit(t *testing.T) {
	dates := generateDates("2023-01-01", 100)
	_, err := PlanSegments(dates, "bogus")
	if err == nil {
		t.Error("unknown split type should fail")
	}
}

func TestAggregateOosStats(t *testing.T) {
	segs := []SegmentResult{
		{Segment: "train", Stats: PerformanceStats{TotalTrades: 5, TotalReturn: 0.10}},
		{Segment: "test_w0", Stats: PerformanceStats{TotalTrades: 8, TotalReturn: 0.15, SharpeRatio: 1.2}},
		{Segment: "test_w1", Stats: PerformanceStats{TotalTrades: 6, TotalReturn: 0.05, SharpeRatio: 0.8}},
	}
	stats := AggregateOosStats(segs)
	// 应只聚合 test 段
	if stats.TotalReturn < 0.099 || stats.TotalReturn > 0.101 {
		t.Errorf("oos total_return=%.4f, want ~0.10 (avg of 0.15, 0.05)", stats.TotalReturn)
	}
}

func TestComputeConfidence(t *testing.T) {
	// hard blocker → rejected
	conf, pass := ComputeConfidence(ConfidenceInput{
		Stats:         PerformanceStats{TotalTrades: 20, SharpeRatio: 1.5, MaxDrawdown: 0.1},
		Blockers:      []PromotionBlocker{{Severity: "hard"}},
		OosTradeCount: 20,
	})
	if conf != ConfidenceRejected || pass {
		t.Errorf("hard blocker: conf=%s pass=%v, want rejected/false", conf, pass)
	}

	// insufficient: OOS trades < 8
	conf, pass = ComputeConfidence(ConfidenceInput{
		Stats:         PerformanceStats{TotalTrades: 5},
		OosTradeCount: 5,
	})
	if conf != ConfidenceInsufficient || pass {
		t.Errorf("insufficient: conf=%s pass=%v", conf, pass)
	}

	// multiple testing not significant → rejected
	conf, pass = ComputeConfidence(ConfidenceInput{
		Stats:           PerformanceStats{TotalTrades: 20, SharpeRatio: 1.5, MaxDrawdown: 0.1},
		MultipleTesting: MultipleTestingResult{Trials: 10, Significant: false},
		OosTradeCount:   20,
	})
	if conf != ConfidenceRejected || pass {
		t.Errorf("mt not significant: conf=%s pass=%v", conf, pass)
	}

	// weak: sharpe < 0.5
	conf, pass = ComputeConfidence(ConfidenceInput{
		Stats:           PerformanceStats{TotalTrades: 20, SharpeRatio: 0.3, MaxDrawdown: 0.1},
		MultipleTesting: MultipleTestingResult{Trials: 1, Significant: true},
		OosTradeCount:   20,
	})
	if conf != ConfidenceWeak || pass {
		t.Errorf("weak sharpe: conf=%s pass=%v", conf, pass)
	}

	// strong
	conf, pass = ComputeConfidence(ConfidenceInput{
		Stats:           PerformanceStats{TotalTrades: 20, SharpeRatio: 1.5, MaxDrawdown: 0.1},
		MultipleTesting: MultipleTestingResult{Trials: 1, Significant: true},
		OosTradeCount:   20,
	})
	if conf != ConfidenceStrong || !pass {
		t.Errorf("strong: conf=%s pass=%v", conf, pass)
	}

	// moderate: soft blocker
	conf, pass = ComputeConfidence(ConfidenceInput{
		Stats:           PerformanceStats{TotalTrades: 20, SharpeRatio: 1.5, MaxDrawdown: 0.1},
		Blockers:        []PromotionBlocker{{Severity: "soft"}},
		MultipleTesting: MultipleTestingResult{Trials: 1, Significant: true},
		OosTradeCount:   20,
	})
	if conf != ConfidenceModerate || !pass {
		t.Errorf("moderate: conf=%s pass=%v", conf, pass)
	}
}

// generateDates 生成从 start 起的 n 个连续日历日 (YYYY-MM-DD)。
// 用于测试切分逻辑，不涉及真实交易日。
func generateDates(start string, n int) []string {
	out := make([]string, n)
	t := mustParseTime(start)
	for i := 0; i < n; i++ {
		out[i] = t.AddDate(0, 0, i).Format("2006-01-02")
	}
	return out
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
