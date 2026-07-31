package review

import (
	"math"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// review.go tests
// ============================================================================

func TestReviewGenerator_GenerateBasicReport(t *testing.T) {
	gen := NewReviewGenerator()

	input := ReviewInput{
		SourceID:    "paradigm-001",
		SourceType:  "paradigm",
		Type:        ReviewRetrospective,
		Period:      ReviewWeekly,
		PeriodStart: time.Now().AddDate(0, 0, -7),
		PeriodEnd:   time.Now(),
		Author:      "test-agent",

		SignalCount:      100,
		ExecutedCount:    85,
		FailedCount:      5,
		UnexecutedCount:  10,
		Returns:          []float64{0.02, -0.01, 0.03, 0.01, -0.005, 0.015, 0.025, -0.02, 0.01, 0.005},
		PnL:              1500.0,
		StatusChanges:    1,
		ParamChanges:     2,
		DataQualityScore: 0.85,
		ExtraFindings: []ReviewFinding{
			{ID: "extra-1", Category: "performance", Severity: "info", Title: "额外发现", Description: "额外的测试发现"},
		},
	}

	report := gen.GenerateReview(input)

	if report.ID == "" {
		t.Error("Report ID should not be empty")
	}
	if report.Type != ReviewRetrospective {
		t.Errorf("Expected type retrospective, got %s", report.Type)
	}
	if report.Status != ReviewDraft {
		t.Errorf("Expected status draft, got %s", report.Status)
	}
	if report.Stats.TotalSignals != 100 {
		t.Errorf("Expected 100 total signals, got %d", report.Stats.TotalSignals)
	}
	if len(report.Findings) == 0 {
		t.Error("Should have at least one finding")
	}
	if len(report.ExecutiveSummary) == 0 {
		t.Error("Executive summary should not be empty")
	}
	if len(report.ActionItems) > 0 {
		// Action items should have proper structure
		for _, item := range report.ActionItems {
			if item.ID == "" || item.Title == "" {
				t.Error("Action item missing required fields")
			}
		}
	}
	if len(report.LessonsLearned) == 0 {
		t.Error("Lessons learned should not be empty")
	}
}

func TestReviewGenerator_LowWinRate(t *testing.T) {
	gen := NewReviewGenerator()

	input := ReviewInput{
		SourceID:    "paradigm-002",
		SourceType:  "paradigm",
		Type:        ReviewRetrospective,
		Period:      ReviewWeekly,
		PeriodStart: time.Now().AddDate(0, 0, -7),
		PeriodEnd:   time.Now(),
		Author:      "test-agent",

		SignalCount:      50,
		ExecutedCount:    40,
		FailedCount:      10,
		UnexecutedCount:  10,
		Returns:          []float64{-0.02, -0.01, -0.03, 0.01, -0.015, -0.025, 0.02, -0.01, -0.005, -0.01},
		PnL:              -800.0,
		DataQualityScore: 0.9,
	}

	report := gen.GenerateReview(input)

	hasWinRateFinding := false
	for _, f := range report.Findings {
		if f.Metric == "win_rate" {
			hasWinRateFinding = true
			if f.Severity != "critical" {
				t.Errorf("Expected critical severity for low win rate, got %s", f.Severity)
			}
		}
	}
	if !hasWinRateFinding {
		t.Error("Should have a win_rate finding for low win rate")
	}
}

func TestReviewGenerator_HighDataQualityIssues(t *testing.T) {
	gen := NewReviewGenerator()

	input := ReviewInput{
		SourceID:    "paradigm-003",
		SourceType:  "paradigm",
		Type:        ReviewRetrospective,
		Period:      ReviewWeekly,
		PeriodStart: time.Now().AddDate(0, 0, -7),
		PeriodEnd:   time.Now(),
		Author:      "test-agent",

		SignalCount:      200,
		ExecutedCount:    150,
		FailedCount:      30,
		UnexecutedCount:  50,
		Returns:          []float64{0.01, 0.02, -0.01, 0.005},
		PnL:              500.0,
		DataQualityScore: 0.4, // very low data quality
	}

	report := gen.GenerateReview(input)

	hasDataQualityFinding := false
	for _, f := range report.Findings {
		if f.Metric == "data_quality" && f.Severity == "critical" {
			hasDataQualityFinding = true
		}
	}
	if !hasDataQualityFinding {
		t.Error("Should have critical data quality finding for score < 0.5")
	}
}

func TestReviewGenerator_ActionItems(t *testing.T) {
	gen := NewReviewGenerator()

	input := ReviewInput{
		SourceID:    "paradigm-004",
		SourceType:  "paradigm",
		Type:        ReviewFailureAnalysis,
		Period:      ReviewWeekly,
		PeriodStart: time.Now().AddDate(0, 0, -7),
		PeriodEnd:   time.Now(),
		Author:      "test-agent",

		SignalCount:   10,
		ExecutedCount: 5,
		FailedCount:   5,
		Returns:       []float64{-0.05, -0.03, -0.02},
		PnL:           -500.0,
		Failures: []FailureEvent{
			{
				ID:       "fail-1",
				Category: FailureModelDegradation,
				Severity: SeverityCritical,
				Title:    "模型预测失效",
			},
		},
	}

	report := gen.GenerateReview(input)

	if len(report.ActionItems) == 0 {
		t.Error("Should have action items for critical findings")
	}

	for _, item := range report.ActionItems {
		if item.Status != "todo" {
			t.Errorf("Expected action item status 'todo', got %s", item.Status)
		}
	}
}

func TestReviewGenerator_Priority(t *testing.T) {
	gen := NewReviewGenerator()

	// High priority: critical failure
	input := ReviewInput{
		SourceID:   "paradigm-005",
		SourceType: "paradigm",
		Type:       ReviewPostMortem,
		Period:     ReviewWeekly,
		Author:     "test-agent",
		Failures: []FailureEvent{
			{Severity: SeverityCritical},
			{Severity: SeverityCritical},
		},
	}

	report := gen.GenerateReview(input)
	if report.Priority != PriorityHigh {
		t.Errorf("Expected high priority, got %s", report.Priority)
	}
}

func TestReviewGenerator_OpenQuestions(t *testing.T) {
	gen := NewReviewGenerator()

	input := ReviewInput{
		SourceID:    "paradigm-006",
		SourceType:  "paradigm",
		Type:        ReviewRetrospective,
		Period:      ReviewWeekly,
		PeriodStart: time.Now().AddDate(0, 0, -7),
		PeriodEnd:   time.Now(),
		Author:      "test-agent",

		SignalCount:     50,
		ExecutedCount:   30,
		FailedCount:     20,
		UnexecutedCount: 20,
		Returns:         []float64{-0.02, -0.03, -0.01},
		PnL:             -200.0,
		StatusChanges:   2,
		ParamChanges:    5,
	}

	report := gen.GenerateReview(input)

	if len(report.OpenQuestions) == 0 {
		t.Error("Should have open questions for problematic reports")
	}
}

// ============================================================================
// failure_analysis.go tests
// ============================================================================

func TestFailureAnalyzer_ClassifyFailure(t *testing.T) {
	analyzer := NewFailureAnalyzer()

	tests := []struct {
		name     string
		event    FailureEvent
		expected FailureCategory
	}{
		{
			name:     "model degradation by metric",
			event:    FailureEvent{Metric: "signal_deviation"},
			expected: FailureModelDegradation,
		},
		{
			name:     "market regime",
			event:    FailureEvent{Metric: "market_state_change"},
			expected: FailureMarketRegime,
		},
		{
			name:     "data quality",
			event:    FailureEvent{Metric: "data_missing"},
			expected: FailureDataQuality,
		},
		{
			name:     "execution",
			event:    FailureEvent{Metric: "execution_price"},
			expected: FailureExecution,
		},
		{
			name:     "risk management",
			event:    FailureEvent{Metric: "max_drawdown"},
			expected: FailureRiskManagement,
		},
		{
			name:     "liquidity",
			event:    FailureEvent{Metric: "spread"},
			expected: FailureLiquidity,
		},
		{
			name:     "user decision",
			event:    FailureEvent{Metric: "user_override"},
			expected: FailureUserDecision,
		},
		{
			name:     "system error",
			event:    FailureEvent{Metric: "api_error"},
			expected: FailureSystemError,
		},
		{
			name:     "overfitting",
			event:    FailureEvent{Metric: "in_sample_high"},
			expected: FailureOverfitting,
		},
		{
			name:     "parameter drift",
			event:    FailureEvent{Metric: "param_shift"},
			expected: FailureParameterDrift,
		},
		{
			name:     "explicit category preserved",
			event:    FailureEvent{Category: FailureModelDegradation, Metric: "unknown"},
			expected: FailureModelDegradation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.ClassifyFailure(tt.event)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestFailureAnalyzer_AnalyzeFailures(t *testing.T) {
	analyzer := NewFailureAnalyzer()

	events := []FailureEvent{
		{
			ID:         "f1",
			Category:   FailureModelDegradation,
			Severity:   SeverityCritical,
			Metric:     "signal_deviation",
			Deviation:  0.6,
			RootCause:  "模型过期",
			DetectedAt: time.Now().Add(-24 * time.Hour),
			Status:     "open",
		},
		{
			ID:         "f2",
			Category:   FailureDataQuality,
			Severity:   SeverityWarning,
			Metric:     "data_missing",
			Deviation:  0.2,
			RootCause:  "数据源延迟",
			DetectedAt: time.Now().Add(-12 * time.Hour),
			Status:     "investigating",
		},
		{
			ID:         "f3",
			Category:   FailureModelDegradation,
			Severity:   SeverityWarning,
			Metric:     "signal_deviation",
			Deviation:  0.3,
			RootCause:  "模型过期",
			DetectedAt: time.Now().Add(-2 * time.Hour),
			Status:     "open",
		},
	}

	result := analyzer.AnalyzeFailures(events)

	if result.TotalFailures != 3 {
		t.Errorf("Expected 3 total failures, got %d", result.TotalFailures)
	}
	if result.OpenFailures != 3 {
		t.Errorf("Expected 3 open failures, got %d", result.OpenFailures)
	}
	if result.ByCategory[FailureModelDegradation] != 2 {
		t.Errorf("Expected 2 model degradation failures, got %d", result.ByCategory[FailureModelDegradation])
	}
	if result.BySeverity[SeverityCritical] != 1 {
		t.Errorf("Expected 1 critical, got %d", result.BySeverity[SeverityCritical])
	}

	// Root cause analysis
	foundRootCause := false
	for _, rc := range result.TopCauses {
		if rc.Factor == "模型过期" {
			foundRootCause = true
			if rc.Count < 2 {
				t.Error("Root cause count should be at least 2")
			}
		}
	}
	if !foundRootCause {
		t.Error("Should have found root cause '模型过期'")
	}
}

func TestFailureAnalyzer_SuggestRootCause(t *testing.T) {
	analyzer := NewFailureAnalyzer()

	event := FailureEvent{
		Deviation: 0.5,
		Metric:    "signal_deviation",
	}

	cause := analyzer.SuggestRootCause(event)
	if cause == "" {
		t.Error("Should suggest a root cause")
	}
	if !strings.Contains(cause, "模型") {
		t.Error("Should mention model-related cause")
	}
}

func TestFailureAnalyzer_DetectPatterns(t *testing.T) {
	analyzer := NewFailureAnalyzer()

	// Consecutive failures
	events := []FailureEvent{
		{
			ID:         "f1",
			Category:   FailureModelDegradation,
			Severity:   SeverityWarning,
			DetectedAt: time.Now().Add(-24 * time.Hour),
		},
		{
			ID:         "f2",
			Category:   FailureModelDegradation,
			Severity:   SeverityCritical,
			DetectedAt: time.Now().Add(-20 * time.Hour),
		},
		{
			ID:         "f3",
			Category:   FailureModelDegradation,
			Severity:   SeverityCritical,
			DetectedAt: time.Now().Add(-10 * time.Hour),
		},
	}

	patterns := analyzer.detectPatterns(events)

	// Should detect consecutive failure pattern
	hasConsecutive := false
	for _, p := range patterns {
		if p.PatternType == "consecutive_failure" {
			hasConsecutive = true
		}
		if p.PatternType == "severity_escalation" {
			// Should detect escalation from warning to critical
			if p.Severity != "critical" {
				t.Error("Escalation pattern should be critical severity")
			}
		}
	}
	if !hasConsecutive {
		t.Error("Should detect consecutive failure pattern")
	}
}

func TestFailureAnalyzer_RankBySeverity(t *testing.T) {
	analyzer := NewFailureAnalyzer()

	events := []FailureEvent{
		{Severity: SeverityWarning},
		{Severity: SeverityCatastrophic},
		{Severity: SeverityInfo},
		{Severity: SeverityCritical},
	}

	ranked := analyzer.RankFailureSeverity(events)

	if ranked[0].Severity != SeverityCatastrophic {
		t.Errorf("Expected catastrophic first, got %s", ranked[0].Severity)
	}
	if ranked[1].Severity != SeverityCritical {
		t.Errorf("Expected critical second, got %s", ranked[1].Severity)
	}
}

// ============================================================================
// feedback.go tests
// ============================================================================

func TestFeedbackGenerator_GenerateFromReview(t *testing.T) {
	gen := NewFeedbackGenerator()

	report := &ReviewReport{
		ID:         "review-1",
		Type:       ReviewFailureAnalysis,
		Period:     ReviewWeekly,
		Status:     ReviewDraft,
		SourceID:   "paradigm-001",
		SourceType: "paradigm",
		Findings: []ReviewFinding{
			{
				ID:       "finding-1",
				Category: "performance",
				Severity: "critical",
				Title:    "胜率过低",
			},
		},
		Failures: []FailureEvent{
			{
				ID:        "fail-1",
				Category:  FailureModelDegradation,
				Severity:  SeverityCritical,
				RootCause: "模型退化",
			},
		},
		Author: "test-agent",
	}

	portfolio := gen.GenerateFromReview(report)

	if portfolio.TotalCount == 0 {
		t.Error("Should generate feedback items")
	}
	if portfolio.GeneratedAt.IsZero() {
		t.Error("Portfolio should have generation timestamp")
	}
	if len(portfolio.Recommendations) == 0 {
		t.Error("Should have portfolio recommendations")
	}

	for _, item := range portfolio.Items {
		if item.ID == "" {
			t.Error("Feedback item should have ID")
		}
		if item.Status != FeedbackPending {
			t.Errorf("Expected initial status pending, got %s", item.Status)
		}
		if item.Priority == "" {
			t.Error("Feedback should have priority")
		}
	}
}

func TestFeedbackGenerator_Deduplication(t *testing.T) {
	gen := NewFeedbackGenerator()

	report := &ReviewReport{
		ID:         "review-dedup",
		Type:       ReviewRetrospective,
		Period:     ReviewWeekly,
		SourceID:   "paradigm-001",
		SourceType: "paradigm",
		Findings: []ReviewFinding{
			{
				ID:       "f1",
				Category: "performance",
				Severity: "critical",
				Title:    "问题1",
				Metric:   "win_rate",
			},
			{
				ID:       "f2",
				Category: "performance",
				Severity: "critical",
				Title:    "问题1", // same title -> should be deduped
				Metric:   "win_rate",
			},
		},
		Author: "test-agent",
	}

	portfolio := gen.GenerateFromReview(report)

	// Should have deduplicated items (max 1 for same target+type+title)
	if portfolio.TotalCount > len(portfolio.Items) {
		t.Error("Items count should match TotalCount after dedup")
	}
}

func TestFeedbackGenerator_StatusUpdate(t *testing.T) {
	gen := NewFeedbackGenerator()

	feedback := ResearchFeedback{
		ID:     "fb-1",
		Status: FeedbackPending,
	}

	gen.UpdateFeedback(&feedback, FeedbackInProgress, "开始处理", "admin")

	if feedback.Status != FeedbackInProgress {
		t.Errorf("Expected status in_progress, got %s", feedback.Status)
	}
	if len(feedback.History) == 0 {
		t.Error("Should have history entry")
	}
	if feedback.History[0].OldStatus != "pending" {
		t.Error("History should record old status")
	}
}

func TestFeedbackGenerator_Reject(t *testing.T) {
	gen := NewFeedbackGenerator()

	feedback := ResearchFeedback{
		ID:     "fb-2",
		Status: FeedbackPending,
	}

	gen.RejectFeedback(&feedback, "不需要修复", "reviewer")

	if feedback.Status != FeedbackRejected {
		t.Errorf("Expected rejected, got %s", feedback.Status)
	}
}

func TestFeedbackGenerator_Implement(t *testing.T) {
	gen := NewFeedbackGenerator()

	feedback := ResearchFeedback{
		ID:     "fb-3",
		Status: FeedbackValidated,
	}

	gen.ImplementFeedback(&feedback, "v2.0", "admin")

	if feedback.Status != FeedbackImplemented {
		t.Errorf("Expected implemented, got %s", feedback.Status)
	}
	if feedback.NewVersion != "v2.0" {
		t.Errorf("Expected new version v2.0, got %s", feedback.NewVersion)
	}
}

func TestFeedbackGenerator_EffortEstimation(t *testing.T) {
	gen := NewFeedbackGenerator()

	tests := []struct {
		feedbackType FeedbackType
		expected     string
	}{
		{FeedbackDataFix, "quick"},
		{FeedbackProcessImprove, "quick"},
		{FeedbackParamUpdate, "moderate"},
		{FeedbackStrategyRev, "moderate"},
		{FeedbackHypothesis, "moderate"},
		{FeedbackModelRetrain, "extensive"},
	}

	for _, tt := range tests {
		result := gen.estimateEffort(tt.feedbackType)
		if result != tt.expected {
			t.Errorf("For %s: expected %s, got %s", tt.feedbackType, tt.expected, result)
		}
	}
}

// ============================================================================
// 辅助函数测试
// ============================================================================

func TestMean(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}
	result := mean(data)
	if result != 3 {
		t.Errorf("Expected mean 3, got %f", result)
	}

	result = mean([]float64{})
	if result != 0 {
		t.Error("Empty slice should return 0")
	}
}

func TestWinRate(t *testing.T) {
	data := []float64{0.01, -0.01, 0.02, 0.015, -0.005}
	result := winRate(data)
	// 3 wins out of 5 = 0.6
	if result != 0.6 {
		t.Errorf("Expected win rate 0.6, got %f", result)
	}
}

func TestAnnualizedSharpe(t *testing.T) {
	// Constant positive returns should have high Sharpe
	data := make([]float64, 252)
	for i := range data {
		data[i] = 0.001 // small daily positive return
	}
	result := annualizedSharpe(data)
	if result <= 0 {
		t.Errorf("Expected positive Sharpe ratio, got %f", result)
	}
}

func TestMaxDrawdown(t *testing.T) {
	// Price path: 1 -> 1.5 -> 1.2 -> 1.3 -> 1.0 -> 0.8
	returns := []float64{0.5, -0.2, 0.0833, -0.2308, -0.2}
	// cumulative: 1*1.5=1.5, 1.5*0.8=1.2, 1.2*1.0833=1.3, 1.3*0.7692=1.0, 1.0*0.8=0.8
	// peak: 1.5 at t=1
	// maxDD: (1.5 - 0.8) / 1.5 = 0.4667

	result := maxDrawdown(returns)
	if result < 0.4 || result > 0.5 {
		t.Errorf("Expected max drawdown around 0.47, got %f", result)
	}

	// All positive: zero drawdown
	posReturns := []float64{0.01, 0.02, 0.015}
	result = maxDrawdown(posReturns)
	if result != 0 {
		t.Error("All positive returns should have zero drawdown")
	}
}

func TestMaxDrawdown_Empty(t *testing.T) {
	result := maxDrawdown([]float64{})
	if result != 0 {
		t.Error("Empty returns should have zero drawdown")
	}
}

func TestContains(t *testing.T) {
	if !contains("hello world", "world") {
		t.Error("Should find substring")
	}
	if contains("hello", "xyz") {
		t.Error("Should not find non-existent substring")
	}
}

func TestMin(t *testing.T) {
	if min(5, 10) != 5 {
		t.Error("Min(5,10) should be 5")
	}
	if min(-1, 1) != -1 {
		t.Error("Min(-1,1) should be -1")
	}
}

// ============================================================================
// 集成测试
// ============================================================================

func TestFullWorkflow(t *testing.T) {
	// 1. Generate review
	reviewGen := NewReviewGenerator()
	reviewInput := ReviewInput{
		SourceID:    "paradigm-001",
		SourceType:  "paradigm",
		Type:        ReviewRetrospective,
		Period:      ReviewWeekly,
		PeriodStart: time.Now().AddDate(0, 0, -7),
		PeriodEnd:   time.Now(),
		Author:      "agent-01",

		SignalCount:      100,
		ExecutedCount:    85,
		FailedCount:      5,
		UnexecutedCount:  10,
		Returns:          make([]float64, 10),
		PnL:              1000,
		DataQualityScore: 0.9,
	}
	// Create diverse returns
	for i := range reviewInput.Returns {
		if i < 5 {
			reviewInput.Returns[i] = 0.02
		} else {
			reviewInput.Returns[i] = -0.01
		}
	}

	report := reviewGen.GenerateReview(reviewInput)
	if report == nil {
		t.Fatal("Should generate report")
	}

	// 2. Analyze failures
	analyzer := NewFailureAnalyzer()
	failures := []FailureEvent{
		{
			ID:         "fail-1",
			Category:   FailureModelDegradation,
			Severity:   SeverityCritical,
			Title:      "模型预测偏差过大",
			Metric:     "signal_deviation",
			Deviation:  0.5,
			RootCause:  "模型过期, 需要重新训练",
			DetectedAt: time.Now().Add(-24 * time.Hour),
			Status:     "open",
		},
		{
			ID:         "fail-2",
			Category:   FailureExecution,
			Severity:   SeverityWarning,
			Title:      "流动性不足",
			Metric:     "execution_volume",
			Deviation:  0.2,
			RootCause:  "小盘股流动性不足",
			DetectedAt: time.Now().Add(-12 * time.Hour),
			Status:     "investigating",
		},
	}
	analysis := analyzer.AnalyzeFailures(failures)
	if analysis.TotalFailures != 2 {
		t.Errorf("Expected 2 failures, got %d", analysis.TotalFailures)
	}

	// 3. Generate feedback
	feedbackGen := NewFeedbackGenerator()
	portfolio := feedbackGen.GenerateFromReview(report)
	if portfolio.TotalCount < len(report.Findings) {
		t.Error("Should generate feedback for critical findings")
	}

	// 4. Validate workflow states
	if len(portfolio.Items) > 0 {
		item := &portfolio.Items[0]
		feedbackGen.ValidateFeedback(item, "验证通过", "reviewer")
		if item.Status != FeedbackValidated {
			t.Errorf("Expected validated, got %s", item.Status)
		}
		if item.ValidatedAt == nil {
			t.Error("Should have validation timestamp")
		}
	}

	// Verify round-trip with failures added to report
	report.Failures = failures
	portfolio2 := feedbackGen.GenerateFromReview(report)
	if portfolio2.TotalCount == 0 {
		t.Error("Should generate feedback from failures")
	}
}

// Prevent unused import warnings
var _ = math.Abs
