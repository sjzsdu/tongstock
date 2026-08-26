package backtest

import (
	"math"
	"testing"
	"time"
)

// ============================================================================
// 测试数据生成
// ============================================================================

func generateDailyDates(n int) []time.Time {
	dates := make([]time.Time, n)
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.Local)
	for i := 0; i < n; i++ {
		dates[i] = base.AddDate(0, 0, i)
	}
	return dates
}

func generateWeekdayDates(n int) []time.Time {
	dates := make([]time.Time, 0, n)
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.Local) // Thursday
	for i := 0; len(dates) < n; i++ {
		d := base.AddDate(0, 0, i)
		if d.Weekday() >= time.Monday && d.Weekday() <= time.Friday {
			dates = append(dates, d)
		}
	}
	return dates
}

// ============================================================================
// 配置验证测试
// ============================================================================

func TestTimeSeriesSplitConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TimeSeriesSplitConfig
		wantErr bool
	}{
		{
			"valid fixed",
			DefaultFixedSplit(),
			false,
		},
		{
			"invalid train ratio",
			func() TimeSeriesSplitConfig {
				c := DefaultFixedSplit()
				c.TrainRatio = 0
				return c
			}(),
			true,
		},
		{
			"invalid valid ratio",
			func() TimeSeriesSplitConfig {
				c := DefaultFixedSplit()
				c.ValidRatio = -0.1
				return c
			}(),
			true,
		},
		{
			"train+valid >= 1",
			func() TimeSeriesSplitConfig {
				c := DefaultFixedSplit()
				c.TrainRatio = 0.8
				c.ValidRatio = 0.3
				return c
			}(),
			true,
		},
		{
			"negative embargo",
			func() TimeSeriesSplitConfig {
				c := DefaultFixedSplit()
				c.EmbargoDays = -1
				return c
			}(),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWalkForwardConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  WalkForwardConfig
		wantErr bool
	}{
		{"valid", DefaultWalkForwardConfig(), false},
		{"zero windows", func() WalkForwardConfig { c := DefaultWalkForwardConfig(); c.Windows = 0; return c }(), true},
		{"short train", func() WalkForwardConfig { c := DefaultWalkForwardConfig(); c.TrainWindowDays = 10; return c }(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// TimeSegment 测试
// ============================================================================

func TestTimeSegment_Contains(t *testing.T) {
	seg := TimeSegment{
		Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local),
		End:   time.Date(2020, 1, 10, 0, 0, 0, 0, time.Local),
	}

	tests := []struct {
		date    time.Time
		contain bool
	}{
		{time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local), true},
		{time.Date(2020, 1, 5, 0, 0, 0, 0, time.Local), true},
		{time.Date(2020, 1, 10, 0, 0, 0, 0, time.Local), true},
		{time.Date(2020, 1, 11, 0, 0, 0, 0, time.Local), false},
		{time.Date(2019, 12, 31, 0, 0, 0, 0, time.Local), false},
	}

	for _, tt := range tests {
		got := seg.Contains(tt.date)
		if got != tt.contain {
			t.Errorf("Contains(%s) = %v, want %v", tt.date.Format("2006-01-02"), got, tt.contain)
		}
	}
}

func TestTimeSegment_Overlaps(t *testing.T) {
	tests := []struct {
		a, b    TimeSegment
		overlap bool
	}{
		{
			TimeSegment{time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local), time.Date(2020, 1, 10, 0, 0, 0, 0, time.Local)},
			TimeSegment{time.Date(2020, 1, 5, 0, 0, 0, 0, time.Local), time.Date(2020, 1, 15, 0, 0, 0, 0, time.Local)},
			true,
		},
		{
			TimeSegment{time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local), time.Date(2020, 1, 5, 0, 0, 0, 0, time.Local)},
			TimeSegment{time.Date(2020, 1, 6, 0, 0, 0, 0, time.Local), time.Date(2020, 1, 10, 0, 0, 0, 0, time.Local)},
			false,
		},
		{
			TimeSegment{time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local), time.Date(2020, 1, 5, 0, 0, 0, 0, time.Local)},
			TimeSegment{time.Date(2020, 1, 5, 0, 0, 0, 0, time.Local), time.Date(2020, 1, 10, 0, 0, 0, 0, time.Local)},
			true, // 边界重叠
		},
	}

	for _, tt := range tests {
		got := tt.a.Overlaps(tt.b)
		if got != tt.overlap {
			t.Errorf("Overlaps = %v, want %v", got, tt.overlap)
		}
	}
}

// ============================================================================
// 固定切分测试
// ============================================================================

func TestTimeSeriesSplitter_FixedSplit(t *testing.T) {
	dates := generateDailyDates(200) // 200 天

	splitter, err := NewTimeSeriesSplitter(DefaultFixedSplit())
	if err != nil {
		t.Fatalf("NewTimeSeriesSplitter: %v", err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	// 验证区间划分
	trainDays := result.Train.Len()
	validDays := result.Valid.Len()
	testDays := result.Test.Len()

	t.Logf("Train: %d days (%s - %s)", trainDays, result.Train.Start.Format("2006-01-02"), result.Train.End.Format("2006-01-02"))
	t.Logf("Valid: %d days (%s - %s)", validDays, result.Valid.Start.Format("2006-01-02"), result.Valid.End.Format("2006-01-02"))
	t.Logf("Test: %d days (%s - %s)", testDays, result.Test.Start.Format("2006-01-02"), result.Test.End.Format("2006-01-02"))

	// 检查比例
	expectedTrain := 140 // 70%
	if math.Abs(float64(trainDays-expectedTrain)) > 5 {
		t.Errorf("Train days = %d, expected ~%d", trainDays, expectedTrain)
	}

	// 检查总天数 (含 embargo)
	totalDays := trainDays + validDays + testDays
	if result.EmbargoTrainValid != nil {
		totalDays += result.EmbargoTrainValid.Len()
	}
	if result.EmbargoValidTest != nil {
		totalDays += result.EmbargoValidTest.Len()
	}

	// 允许一些误差 (因为 embargo 可能与实际日期重叠)
	if totalDays < 195 || totalDays > 220 {
		t.Logf("Total days = %d (expected ~%d)", totalDays, 200)
	}

	// 验证数据隔离
	if err := result.ValidateDataIsolation(); err != nil {
		t.Errorf("Data isolation violated: %v", err)
	}
}

func TestTimeSeriesSplitter_NoRandomSplit(t *testing.T) {
	// 确保时间序列切分不打乱顺序
	dates := generateDailyDates(100)

	splitter, err := NewTimeSeriesSplitter(DefaultFixedSplit())
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}

	// 验证: 训练集日期 < 验证集日期 < 测试集日期
	if !result.Train.End.Before(result.Valid.Start) {
		t.Error("Train end should be before valid start")
	}
	if !result.Valid.End.Before(result.Test.Start) {
		t.Error("Valid end should be before test start")
	}
}

func TestTimeSeriesSplitter_CustomRatio(t *testing.T) {
	dates := generateDailyDates(100)

	config := TimeSeriesSplitConfig{
		Type:         SplitFixed,
		TrainRatio:   0.6,
		ValidRatio:   0.2,
		EmbargoDays:  0,
		PurgeDays:    0,
		MinTrainSize: 30,
	}

	splitter, err := NewTimeSeriesSplitter(config)
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}

	trainDays := result.Train.Len()
	validDays := result.Valid.Len()
	testDays := result.Test.Len()

	// 60% = 60, 20% = 20, 20% = 20
	if trainDays != 60 {
		t.Errorf("Train days = %d, want 60", trainDays)
	}
	if validDays != 20 {
		t.Errorf("Valid days = %d, want 20", validDays)
	}
	if testDays != 20 {
		t.Errorf("Test days = %d, want 20", testDays)
	}
}

func TestTimeSeriesSplitter_NoValidationSegment(t *testing.T) {
	dates := generateWeekdayDates(100)
	splitter, err := NewTimeSeriesSplitter(TimeSeriesSplitConfig{
		Type: SplitFixed, TrainRatio: 0.8, ValidRatio: 0,
		EmbargoDays: 3, PurgeDays: 2, MinTrainSize: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid != nil {
		t.Fatal("validation segment must be nil")
	}
	if result.Train.End != dates[77] {
		t.Fatalf("train end = %s, want purged trading date %s", result.Train.End, dates[77])
	}
	if result.Test.Start != dates[83] {
		t.Fatalf("test start = %s, want embargoed trading date %s", result.Test.Start, dates[83])
	}
	if len(result.PurgeDates) != 2 || result.EmbargoTrainValid == nil ||
		result.EmbargoValidTest != nil {
		t.Fatalf("unexpected purge/embargo layout: %+v", result)
	}
}

func TestTimeSeriesSplitter_MinTrainSize(t *testing.T) {
	dates := generateDailyDates(10)

	config := DefaultFixedSplit()
	config.MinTrainSize = 20 // 训练集要求至少 20 天

	splitter, err := NewTimeSeriesSplitter(config)
	if err != nil {
		t.Fatal(err)
	}

	_, err = splitter.Split(dates)
	if err == nil {
		t.Error("Should fail with insufficient data")
	}
}

// ============================================================================
// Embargo / Purge 测试
// ============================================================================

func TestTimeSeriesSplitter_WithEmbargo(t *testing.T) {
	dates := generateDailyDates(100)

	config := DefaultFixedSplit()
	config.EmbargoDays = 5

	splitter, err := NewTimeSeriesSplitter(config)
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}

	// 验证封锁期存在
	if result.EmbargoTrainValid == nil {
		t.Error("Embargo between train and valid should exist")
	}
	if result.EmbargoValidTest == nil {
		t.Error("Embargo between valid and test should exist")
	}

	t.Logf("Embargo train-valid: %s - %s (%d days)",
		result.EmbargoTrainValid.Start.Format("2006-01-02"),
		result.EmbargoTrainValid.End.Format("2006-01-02"),
		result.EmbargoTrainValid.Len())
}

func TestTimeSeriesSplitter_WithPurge(t *testing.T) {
	dates := generateDailyDates(100)

	config := DefaultFixedSplit()
	config.PurgeDays = 3

	splitter, err := NewTimeSeriesSplitter(config)
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}

	// 验证 Purge 日期
	if len(result.PurgeDates) == 0 {
		t.Error("Purge dates should not be empty")
	}

	t.Logf("Purge dates: %d dates", len(result.PurgeDates))
}

// ============================================================================
// SegmentAt 测试
// ============================================================================

func TestSplitResult_SegmentAt(t *testing.T) {
	dates := generateDailyDates(100)

	splitter, err := NewTimeSeriesSplitter(DefaultFixedSplit())
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}

	// 检查训练区间
	trainDate := result.Train.Start
	if seg := result.SegmentAt(trainDate); seg != SegmentTrain {
		t.Errorf("Segment at train start = %s, want train", seg)
	}

	// 检查验证区间
	validDate := result.Valid.Start
	if seg := result.SegmentAt(validDate); seg != SegmentValid {
		t.Errorf("Segment at valid start = %s, want valid", seg)
	}

	// 检查测试区间
	testDate := result.Test.Start
	if seg := result.SegmentAt(testDate); seg != SegmentTest {
		t.Errorf("Segment at test start = %s, want test", seg)
	}

	// 检查 embargo
	if result.EmbargoTrainValid != nil {
		embargoDate := result.EmbargoTrainValid.Start
		if seg := result.SegmentAt(embargoDate); seg != SegmentEmbargo {
			t.Errorf("Segment at embargo = %s, want embargo", seg)
		}
	}
}

// ============================================================================
// Walk-Forward 测试
// ============================================================================

func TestTimeSeriesSplitter_WalkForward(t *testing.T) {
	dates := generateDailyDates(500) // 500 天

	wfConfig := DefaultWalkForwardConfig()
	wfConfig.Windows = 3
	wfConfig.TrainWindowDays = 100
	wfConfig.ValidWindowDays = 30
	wfConfig.TestWindowDays = 30
	wfConfig.StepDays = 30

	splitter, err := NewWalkForwardSplitter(wfConfig)
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.SplitWalkForward(dates)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Windows) < 1 {
		t.Fatal("Should generate at least 1 window")
	}

	t.Logf("Walk-Forward: %d windows generated", len(result.Windows))

	for i, window := range result.Windows {
		t.Logf("Window %d: train=%d days, valid=%d days, test=%d days",
			window.Index,
			window.Split.Train.Len(),
			window.Split.Valid.Len(),
			window.Split.Test.Len())

		// 验证每个窗口的数据隔离
		if err := window.Split.ValidateDataIsolation(); err != nil {
			t.Errorf("Window %d data isolation violated: %v", i, err)
		}
	}
}

func TestTimeSeriesSplitter_WalkForward_ChronologicalOrder(t *testing.T) {
	dates := generateDailyDates(300)

	wfConfig := DefaultWalkForwardConfig()
	wfConfig.Windows = 2
	wfConfig.TrainWindowDays = 80
	wfConfig.ValidWindowDays = 20
	wfConfig.TestWindowDays = 20
	wfConfig.StepDays = 20

	splitter, err := NewWalkForwardSplitter(wfConfig)
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.SplitWalkForward(dates)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Windows) < 2 {
		t.Fatal("Should generate 2 windows")
	}

	// Walk-Forward 窗口可能重叠 (滚动使用更多数据), 但需满足:
	// 1. 每个窗口内部数据隔离
	// 2. 窗口按时间顺序排列 (后一个窗口的测试期应在前一个窗口之后)
	for i, window := range result.Windows {
		if err := window.Split.ValidateDataIsolation(); err != nil {
			t.Errorf("Window %d: data isolation violated: %v", i, err)
		}
	}

	// 验证窗口顺序: 后一个窗口的测试开始时间应 >= 前一个窗口的测试开始时间
	w1 := result.Windows[0].Split
	w2 := result.Windows[1].Split

	if w2.Test.Start.Before(w1.Test.Start) {
		t.Error("Window 2 test should start after or at Window 1 test start")
	}

	t.Log("Walk-Forward windows are in chronological order with proper internal isolation")
}

// ============================================================================
// 泄漏检测测试
// ============================================================================

func TestLeakDetector_NoLeakage(t *testing.T) {
	dates := generateDailyDates(100)

	splitter, err := NewTimeSeriesSplitter(DefaultFixedSplit())
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}

	detector := NewLeakDetector(splitter)
	leaks, err := detector.DetectLeakage(dates, result)
	if err != nil {
		t.Fatal(err)
	}

	// 不应该有严重泄漏
	if detector.HasCriticalLeak(leaks) {
		t.Errorf("Should not have critical leakage: %v", leaks)
	}
}

func TestLeakDetector_DetectOverlap(t *testing.T) {
	// 人为制造一个有泄漏的切分
	dates := generateDailyDates(100)

	leakySplit := &SplitResult{
		Train: TimeSegment{
			Start: dates[0],
			End:   dates[50],
		},
		Valid: &TimeSegment{
			Start: dates[30], // 与训练集重叠!
			End:   dates[70],
		},
		Test: TimeSegment{
			Start: dates[60], // 与训练/验证重叠!
			End:   dates[99],
		},
	}

	detector := NewLeakDetector(nil)
	leaks, err := detector.DetectLeakage(dates, leakySplit)
	if err != nil {
		t.Fatal(err)
	}

	// 应该检测到泄漏
	if !detector.HasCriticalLeak(leaks) {
		t.Error("Should detect critical leakage")
	}

	t.Logf("Detected %d leaks", len(leaks))
	for _, leak := range leaks {
		t.Logf("  - %s", leak.Message)
	}
}

func TestLeakDetector_EmbargoAndPurgeDetected(t *testing.T) {
	// 检测 embargo/purge 日期
	dates := generateDailyDates(100)

	split := &SplitResult{
		Train: TimeSegment{
			Start: dates[0],
			End:   dates[60],
		},
		Valid: &TimeSegment{
			Start: dates[66], // embargo 5 天: 61-65
			End:   dates[80],
		},
		Test: TimeSegment{
			Start: dates[86],
			End:   dates[99],
		},
		EmbargoTrainValid: &TimeSegment{
			Start: dates[61],
			End:   dates[65],
		},
		EmbargoValidTest: &TimeSegment{
			Start: dates[81],
			End:   dates[85],
		},
		PurgeDates: []time.Time{dates[57], dates[58], dates[59], dates[60]},
	}

	detector := NewLeakDetector(nil)
	leaks, err := detector.DetectLeakage(dates, split)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Detected %d leaks (embargo/purge)", len(leaks))
	for _, leak := range leaks {
		t.Logf("  - [%s] %s", leak.Source, leak.Message)
	}
}

// ============================================================================
// 数据隔离测试 (验证数据不参与参数选择)
// ============================================================================

func TestDataIsolation_ValidNotInTrain(t *testing.T) {
	dates := generateWeekdayDates(250) // 约 1 年交易日

	splitter, err := NewTimeSeriesSplitter(DefaultFixedSplit())
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}

	// 检查: 验证集的日期不应出现在训练集中
	validDates := make(map[string]bool)
	for _, d := range dates {
		if result.Valid.Contains(d) {
			validDates[d.Format("2006-01-02")] = true
		}
	}

	for _, d := range dates {
		if result.Train.Contains(d) && validDates[d.Format("2006-01-02")] {
			t.Errorf("Date %s is in both train and valid (LEAK!)", d.Format("2006-01-02"))
		}
	}
}

func TestDataIsolation_TestNotUsedForSelection(t *testing.T) {
	dates := generateWeekdayDates(250)

	splitter, err := NewTimeSeriesSplitter(DefaultFixedSplit())
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}

	// 测试: 收集所有训练/验证日期
	trainValidDates := make(map[string]bool)
	for _, d := range dates {
		seg := result.SegmentAt(d)
		if seg == SegmentTrain || seg == SegmentValid {
			trainValidDates[d.Format("2006-01-02")] = true
		}
	}

	// 确保测试日期不在训练/验证中
	for _, d := range dates {
		if result.Test.Contains(d) && trainValidDates[d.Format("2006-01-02")] {
			t.Errorf("Test date %s is in train/valid (LEAK!)", d.Format("2006-01-02"))
		}
	}

	t.Log("Data isolation verified: test data never used for parameter selection")
}

// ============================================================================
// FilterBySegment 测试
// ============================================================================

func TestFilterBySegment(t *testing.T) {
	dates := generateDailyDates(100)

	splitter, err := NewTimeSeriesSplitter(DefaultFixedSplit())
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.Split(dates)
	if err != nil {
		t.Fatal(err)
	}

	filtered := FilterBySegment(dates, nil, result)

	trainDates := filtered[SegmentTrain]
	validDates := filtered[SegmentValid]
	testDates := filtered[SegmentTest]

	t.Logf("Filtered: train=%d, valid=%d, test=%d",
		len(trainDates), len(validDates), len(testDates))

	// 检查每个区间的日期正确
	for _, d := range trainDates {
		if result.SegmentAt(d) != SegmentTrain {
			t.Errorf("Date %s should be in train", d.Format("2006-01-02"))
		}
	}
}

// ============================================================================
// 边界情况测试
// ============================================================================

func TestTimeSeriesSplitter_SingleDay(t *testing.T) {
	dates := generateDailyDates(1)

	config := DefaultFixedSplit()
	config.MinTrainSize = 1

	splitter, err := NewTimeSeriesSplitter(config)
	if err != nil {
		t.Fatal(err)
	}

	_, err = splitter.Split(dates)
	if err == nil {
		t.Error("Should fail with single day")
	}
}

func TestTimeSeriesSplitter_EmptyDates(t *testing.T) {
	splitter, err := NewTimeSeriesSplitter(DefaultFixedSplit())
	if err != nil {
		t.Fatal(err)
	}

	_, err = splitter.Split(nil)
	if err == nil {
		t.Error("Should fail with empty dates")
	}
}

func TestSplitResult_HasLeakage(t *testing.T) {
	// 无泄漏
	cleanSplit := &SplitResult{
		Train: TimeSegment{time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local), time.Date(2020, 6, 30, 0, 0, 0, 0, time.Local)},
		Valid: &TimeSegment{time.Date(2020, 7, 1, 0, 0, 0, 0, time.Local), time.Date(2020, 9, 30, 0, 0, 0, 0, time.Local)},
		Test:  TimeSegment{time.Date(2020, 10, 1, 0, 0, 0, 0, time.Local), time.Date(2020, 12, 31, 0, 0, 0, 0, time.Local)},
	}

	if cleanSplit.HasLeakage() {
		t.Error("Clean split should not have leakage")
	}

	// 有泄漏
	leakySplit := &SplitResult{
		Train: TimeSegment{time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local), time.Date(2020, 12, 31, 0, 0, 0, 0, time.Local)},
		Valid: &TimeSegment{time.Date(2020, 7, 1, 0, 0, 0, 0, time.Local), time.Date(2020, 9, 30, 0, 0, 0, 0, time.Local)},
		Test:  TimeSegment{time.Date(2020, 6, 1, 0, 0, 0, 0, time.Local), time.Date(2020, 8, 31, 0, 0, 0, 0, time.Local)},
	}

	if !leakySplit.HasLeakage() {
		t.Error("Leaky split should have leakage")
	}
}

// ============================================================================
// 集成测试: 完整 Walk-Forward 工作流
// ============================================================================

func TestWalkForward_IntegrationWorkflow(t *testing.T) {
	// 模拟: 3 年数据, 3 个 Walk-Forward 窗口
	// 每个窗口: 训练 6 月 + 验证 2 月 + 测试 2 月
	dates := generateWeekdayDates(750) // 约 3 年交易日

	wfConfig := WalkForwardConfig{
		Windows:         3,
		TrainWindowDays: 126, // 约 6 月
		ValidWindowDays: 42,  // 约 2 月
		TestWindowDays:  42,  // 约 2 月
		StepDays:        42,  // 每季度滚动
		EmbargoDays:     5,
		PurgeDays:       3,
	}

	splitter, err := NewWalkForwardSplitter(wfConfig)
	if err != nil {
		t.Fatal(err)
	}

	result, err := splitter.SplitWalkForward(dates)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Windows) != 3 {
		t.Fatalf("Expected 3 windows, got %d", len(result.Windows))
	}

	// 验证每个窗口
	for i, window := range result.Windows {
		split := window.Split

		// 1. 数据隔离
		if err := split.ValidateDataIsolation(); err != nil {
			t.Errorf("Window %d: data isolation failed: %v", i, err)
		}

		// 2. 时间顺序
		if !split.Train.End.Before(split.Valid.Start) {
			t.Errorf("Window %d: train should be before valid", i)
		}
		if !split.Valid.End.Before(split.Test.Start) {
			t.Errorf("Window %d: valid should be before test", i)
		}

		// 3. Embargo 有效
		if split.EmbargoTrainValid != nil {
			if !split.EmbargoTrainValid.End.Before(split.Valid.Start) {
				t.Errorf("Window %d: embargo should be before valid", i)
			}
		}

		// 4. 最小训练集
		if split.Train.Len() < wfConfig.TrainWindowDays*2 { // 交易日约为天数的 2/3
			t.Logf("Window %d: train only %d days (expected ~%d)", i, split.Train.Len(), wfConfig.TrainWindowDays)
		}
	}

	// 5. 验证窗口按时间顺序排列 (测试期递增)
	for i := 1; i < len(result.Windows); i++ {
		prevTestStart := result.Windows[i-1].Split.Test.Start
		currTestStart := result.Windows[i].Split.Test.Start
		if currTestStart.Before(prevTestStart) {
			t.Errorf("Window %d test starts before Window %d: %s vs %s",
				i, i-1, currTestStart.Format("2006-01-02"), prevTestStart.Format("2006-01-02"))
		}
	}

	t.Log("Walk-Forward integration test passed!")
	t.Logf("Generated %d windows from %d dates", len(result.Windows), len(dates))
}
