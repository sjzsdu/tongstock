// Package backtest 实现可信回测与实验基础设施。
//
// 核心能力:
//   - 时间序列切分: 固定切分与 Walk-Forward
//   - 防泄漏机制: Embargo / Purge
//   - 验证数据隔离: 确保样本外数据不参与参数选择
package backtest

import (
	"fmt"
	"time"
)

// ============================================================================
// 时间序列切分配置
// ============================================================================

// SplitType 切分类型。
type SplitType string

const (
	// SplitFixed 固定切分: 训练集 + 验证集 + 样本外测试集
	SplitFixed SplitType = "fixed"
	// SplitWalkForward 滚动前推切分: 多个滚动窗口
	SplitWalkForward SplitType = "walk_forward"
)

// TimeSeriesSplitConfig 时间序列切分配置。
type TimeSeriesSplitConfig struct {
	// Type 切分类型
	Type SplitType `json:"type"`
	// TrainRatio 训练集比例 (0-1), 用于 fixed split
	TrainRatio float64 `json:"train_ratio,omitempty"`
	// ValidRatio 验证集比例 (0-1), 用于 fixed split
	ValidRatio float64 `json:"valid_ratio,omitempty"`
	// EmbargoDays 封锁期 (天), 训练集与验证集之间的缓冲
	EmbargoDays int `json:"embargo_days,omitempty"`
	// PurgeDays 清洗期 (天), 去除标签重叠样本
	PurgeDays int `json:"purge_days,omitempty"`
	// MinTrainSize 最小训练集大小 (天)
	MinTrainSize int `json:"min_train_size,omitempty"`
	// RandomSeed 随机种子 (为 0 时不使用随机切分, 强制时间顺序)
	RandomSeed int64 `json:"random_seed,omitempty"`
}

// DefaultFixedSplit 默认固定切分配置: 70% 训练, 15% 验证, 15% 样本外
func DefaultFixedSplit() TimeSeriesSplitConfig {
	return TimeSeriesSplitConfig{
		Type:         SplitFixed,
		TrainRatio:   0.70,
		ValidRatio:   0.15,
		EmbargoDays:  5,
		PurgeDays:    5,
		MinTrainSize: 60,
	}
}

// DefaultWalkForwardSplit 默认 Walk-Forward 配置。
func DefaultWalkForwardSplit() TimeSeriesSplitConfig {
	return TimeSeriesSplitConfig{
		Type:         SplitWalkForward,
		TrainRatio:   0.60,
		ValidRatio:   0.20,
		EmbargoDays:  5,
		PurgeDays:    5,
		MinTrainSize: 60,
	}
}

// Validate 验证切分配置。
func (c TimeSeriesSplitConfig) Validate() error {
	if c.Type == "" {
		return fmt.Errorf("split type is required")
	}
	if c.Type == SplitFixed {
		if c.TrainRatio <= 0 || c.TrainRatio >= 1 {
			return fmt.Errorf("train ratio must be in (0, 1)")
		}
		if c.ValidRatio < 0 || c.ValidRatio >= 1 {
			return fmt.Errorf("valid ratio must be in [0, 1)")
		}
		if c.TrainRatio+c.ValidRatio >= 1 {
			return fmt.Errorf("train + valid ratio must be < 1 (for out-of-sample)")
		}
	}
	if c.EmbargoDays < 0 {
		return fmt.Errorf("embargo days must be >= 0")
	}
	if c.PurgeDays < 0 {
		return fmt.Errorf("purge days must be >= 0")
	}
	if c.MinTrainSize < 0 {
		return fmt.Errorf("min train size must be >= 0")
	}
	return nil
}

// ============================================================================
// 区间定义
// ============================================================================

// DataSegment 数据区间类型。
type DataSegment string

const (
	// SegmentTrain 训练集: 用于模型训练
	SegmentTrain DataSegment = "train"
	// SegmentValid 验证集: 用于参数调优
	SegmentValid DataSegment = "valid"
	// SegmentTest 样本外测试集: 仅用于最终评估
	SegmentTest DataSegment = "test"
	// SegmentEmbargo 封锁期: 不可使用
	SegmentEmbargo DataSegment = "embargo"
	// SegmentPurge 清洗期: 标签重叠不可使用
	SegmentPurge DataSegment = "purge"
)

// TimeSegment 时间区间。
type TimeSegment struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Len 返回区间天数。
func (s TimeSegment) Len() int {
	if s.Start.After(s.End) {
		return 0
	}
	return int(s.End.Sub(s.Start).Hours()/24) + 1
}

// Contains 检查日期是否在区间内。
func (s TimeSegment) Contains(date time.Time) bool {
	return !date.Before(s.Start) && !date.After(s.End)
}

// Overlaps 检查两个区间是否有重叠。
func (s TimeSegment) Overlaps(other TimeSegment) bool {
	return !s.Start.After(other.End) && !other.Start.After(s.End)
}

// ============================================================================
// 切分结果
// ============================================================================

// SplitResult 切分结果。
type SplitResult struct {
	// Train 训练集区间
	Train TimeSegment `json:"train"`
	// Valid 验证集区间 (可为空)
	Valid *TimeSegment `json:"valid,omitempty"`
	// Test 样本外测试集区间
	Test TimeSegment `json:"test"`
	// EmbargoTrainValid 训练与验证之间的封锁期
	EmbargoTrainValid *TimeSegment `json:"embargo_train_valid,omitempty"`
	// EmbargoValidTest 验证与测试之间的封锁期
	EmbargoValidTest *TimeSegment `json:"embargo_valid_test,omitempty"`
	// PurgeDates 需要清洗的日期 (标签重叠)
	PurgeDates []time.Time `json:"purge_dates,omitempty"`
}

// SegmentAt 返回指定日期所在的区间。
func (r *SplitResult) SegmentAt(date time.Time) DataSegment {
	// 先检查 purge
	for _, d := range r.PurgeDates {
		if d.Equal(date) {
			return SegmentPurge
		}
	}

	// 检查 embargo
	if r.EmbargoTrainValid != nil && r.EmbargoTrainValid.Contains(date) {
		return SegmentEmbargo
	}
	if r.EmbargoValidTest != nil && r.EmbargoValidTest.Contains(date) {
		return SegmentEmbargo
	}

	// 检查数据区间
	if r.Train.Contains(date) {
		return SegmentTrain
	}
	if r.Valid != nil && r.Valid.Contains(date) {
		return SegmentValid
	}
	if r.Test.Contains(date) {
		return SegmentTest
	}

	return ""
}

// ValidateDataIsolation 验证数据隔离: 测试数据不应出现在训练或验证集中。
func (r *SplitResult) ValidateDataIsolation() error {
	// 训练集与测试集不应重叠
	if r.Train.Overlaps(r.Test) {
		return fmt.Errorf("train and test segments overlap: %s vs %s",
			r.Train, r.Test)
	}

	// 验证集与测试集不应重叠
	if r.Valid != nil && r.Valid.Overlaps(r.Test) {
		return fmt.Errorf("valid and test segments overlap: %s vs %s",
			r.Valid, r.Test)
	}

	// 训练集与验证集不应重叠
	if r.Valid != nil && r.Train.Overlaps(*r.Valid) {
		return fmt.Errorf("train and valid segments overlap: %s vs %s",
			r.Train, *r.Valid)
	}

	// Purge 日期不应在训练/验证集中
	for _, d := range r.PurgeDates {
		if r.Train.Contains(d) {
			return fmt.Errorf("purge date %s is in train segment", d.Format("2006-01-02"))
		}
		if r.Valid != nil && r.Valid.Contains(d) {
			return fmt.Errorf("purge date %s is in valid segment", d.Format("2006-01-02"))
		}
	}

	return nil
}

// HasLeakage 检查是否存在泄漏: 验证数据不应参与参数选择。
func (r *SplitResult) HasLeakage() bool {
	// 简化检查: 如果训练集包含任何测试区间的日期, 则存在泄漏
	return r.Train.Overlaps(r.Test)
}

// ============================================================================
// Walk-Forward 配置与结果
// ============================================================================

// WalkForwardConfig 滚动前推配置。
type WalkForwardConfig struct {
	// Windows 滚动窗口数量
	Windows int `json:"windows"`
	// TrainWindowDays 每个窗口的训练期 (天)
	TrainWindowDays int `json:"train_window_days"`
	// ValidWindowDays 每个窗口的验证期 (天)
	ValidWindowDays int `json:"valid_window_days"`
	// TestWindowDays 每个窗口的测试期 (天)
	TestWindowDays int `json:"test_window_days"`
	// StepDays 滚动步长 (天)
	StepDays int `json:"step_days"`
	// EmbargoDays 封锁期 (天)
	EmbargoDays int `json:"embargo_days"`
	// PurgeDays 清洗期 (天)
	PurgeDays int `json:"purge_days"`
}

// DefaultWalkForwardConfig 默认 Walk-Forward 配置。
func DefaultWalkForwardConfig() WalkForwardConfig {
	return WalkForwardConfig{
		Windows:         3,
		TrainWindowDays: 252, // 约 1 年交易日
		ValidWindowDays: 63,  // 约 1 个季度
		TestWindowDays:  63,  // 约 1 个季度
		StepDays:        63,  // 每季度滚动
		EmbargoDays:     5,
		PurgeDays:       5,
	}
}

// Validate 验证 Walk-Forward 配置。
func (c WalkForwardConfig) Validate() error {
	if c.Windows < 1 {
		return fmt.Errorf("windows must be >= 1")
	}
	if c.TrainWindowDays < 30 {
		return fmt.Errorf("train window must be >= 30 days")
	}
	if c.TestWindowDays < 1 {
		return fmt.Errorf("test window must be >= 1 day")
	}
	if c.StepDays < 1 {
		return fmt.Errorf("step must be >= 1 day")
	}
	return nil
}

// WalkForwardWindow 单个 Walk-Forward 窗口。
type WalkForwardWindow struct {
	// Index 窗口索引
	Index int `json:"index"`
	// Split 切分结果
	Split SplitResult `json:"split"`
	// ParamsSelected 该窗口选择的参数 (仅基于训练/验证集)
	ParamsSelected map[string]interface{} `json:"params_selected,omitempty"`
	// TestPerformance 测试期表现 (样本外)
	TestPerformance map[string]float64 `json:"test_performance,omitempty"`
}

// WalkForwardResult Walk-Forward 完整结果。
type WalkForwardResult struct {
	// Windows 所有窗口结果
	Windows []WalkForwardWindow `json:"windows"`
	// AggregatePerformance 聚合表现 (跨窗口)
	AggregatePerformance map[string]float64 `json:"aggregate_performance,omitempty"`
	// Config 配置快照
	Config WalkForwardConfig `json:"config"`
	// DataRange 整体数据范围
	DataRange TimeSegment `json:"data_range"`
}

// ============================================================================
// 切分器
// ============================================================================

// TimeSeriesSplitter 时间序列切分器。
type TimeSeriesSplitter struct {
	config   TimeSeriesSplitConfig
	wfConfig WalkForwardConfig
}

// NewTimeSeriesSplitter 创建切分器。
func NewTimeSeriesSplitter(config TimeSeriesSplitConfig) (*TimeSeriesSplitter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &TimeSeriesSplitter{config: config}, nil
}

// NewWalkForwardSplitter 创建 Walk-Forward 切分器。
func NewWalkForwardSplitter(wfConfig WalkForwardConfig) (*TimeSeriesSplitter, error) {
	if err := wfConfig.Validate(); err != nil {
		return nil, err
	}
	return &TimeSeriesSplitter{
		config:   TimeSeriesSplitConfig{Type: SplitWalkForward},
		wfConfig: wfConfig,
	}, nil
}

// Split 执行时间序列切分。
// dates: 按升序排列的交易日列表
func (s *TimeSeriesSplitter) Split(dates []time.Time) (*SplitResult, error) {
	if len(dates) == 0 {
		return nil, fmt.Errorf("dates list is empty")
	}

	if s.config.Type != SplitFixed {
		return nil, fmt.Errorf("use SplitWalkForward() for walk-forward splitting")
	}

	return s.fixedSplit(dates)
}

// fixedSplit 固定切分实现。
func (s *TimeSeriesSplitter) fixedSplit(dates []time.Time) (*SplitResult, error) {
	n := len(dates)

	// 计算切分点
	trainEndIdx := int(float64(n) * s.config.TrainRatio)
	validEndIdx := trainEndIdx + int(float64(n)*s.config.ValidRatio)

	// 确保不越界
	if trainEndIdx >= n || validEndIdx > n {
		return nil, fmt.Errorf("invalid split ratios for %d dates", n)
	}

	// 检查最小训练集
	trainDays := trainEndIdx
	if trainDays < s.config.MinTrainSize {
		return nil, fmt.Errorf("train set too small: %d days (min: %d)", trainDays, s.config.MinTrainSize)
	}

	result := &SplitResult{
		Train: TimeSegment{
			Start: dates[0],
			End:   dates[trainEndIdx-1],
		},
		Valid: &TimeSegment{
			Start: dates[trainEndIdx],
			End:   dates[validEndIdx-1],
		},
		Test: TimeSegment{
			Start: dates[validEndIdx],
			End:   dates[n-1],
		},
	}

	// 添加 Embargo
	if s.config.EmbargoDays > 0 {
		embargoStart := dates[trainEndIdx]
		embargoEnd := addDays(embargoStart, s.config.EmbargoDays-1)

		// 调整训练集结束和验证集开始
		result.Train.End = addDays(result.Train.End, 0) // 保持不变
		// 封锁期: 训练结束后, 验证开始前
		result.EmbargoTrainValid = &TimeSegment{
			Start: embargoStart,
			End:   embargoEnd,
		}
		result.Valid.Start = addDays(embargoEnd, 1)

		// 验证与测试之间的封锁期
		if validEndIdx < n {
			embargoStart2 := result.Valid.End
			embargoEnd2 := addDays(embargoStart2, s.config.EmbargoDays)
			result.EmbargoValidTest = &TimeSegment{
				Start: embargoStart2,
				End:   embargoEnd2,
			}
			result.Test.Start = addDays(embargoEnd2, 1)
		}
	}

	// 计算 Purge 日期 (标签重叠)
	if s.config.PurgeDays > 0 {
		purgeDates := s.computePurgeDates(dates, result)
		result.PurgeDates = purgeDates
	}

	// 验证数据隔离
	if err := result.ValidateDataIsolation(); err != nil {
		return nil, err
	}

	return result, nil
}

// computePurgeDates 计算需要清洗的日期 (标签重叠).
func (s *TimeSeriesSplitter) computePurgeDates(dates []time.Time, result *SplitResult) []time.Time {
	var purgeDates []time.Time

	// 检查训练集末尾是否与测试集标签重叠
	// Purge: 从训练集末尾去掉 PurgeDays 天的数据
	if s.config.PurgeDays > 0 {
		trainEnd := result.Train.End
		for i := 0; i < s.config.PurgeDays; i++ {
			purgeDate := addDays(trainEnd, -i)
			purgeDates = append(purgeDates, purgeDate)
		}

		// 调整训练集结束
		result.Train.End = addDays(trainEnd, -s.config.PurgeDays)
	}

	return purgeDates
}

// SplitWalkForward 执行 Walk-Forward 切分。
func (s *TimeSeriesSplitter) SplitWalkForward(dates []time.Time) (*WalkForwardResult, error) {
	if len(dates) == 0 {
		return nil, fmt.Errorf("dates list is empty")
	}

	if s.config.Type != SplitWalkForward {
		return nil, fmt.Errorf("not configured for walk-forward")
	}

	cfg := s.wfConfig
	result := &WalkForwardResult{
		Windows:   make([]WalkForwardWindow, 0, cfg.Windows),
		Config:    cfg,
		DataRange: TimeSegment{Start: dates[0], End: dates[len(dates)-1]},
	}

	// 计算每个窗口
	step := cfg.StepDays
	for i := 0; i < cfg.Windows; i++ {
		windowStart := addDays(dates[0], i*step)
		trainEnd := addDays(windowStart, cfg.TrainWindowDays)
		validEnd := addDays(trainEnd, cfg.ValidWindowDays)
		testEnd := addDays(validEnd, cfg.TestWindowDays)

		// 如果超过数据范围, 则截断
		if testEnd.After(dates[len(dates)-1]) {
			testEnd = dates[len(dates)-1]
		}
		if trainEnd.After(dates[len(dates)-1]) {
			break // 数据不足, 停止生成窗口
		}

		split := &SplitResult{
			Train: TimeSegment{
				Start: windowStart,
				End:   trainEnd,
			},
			Valid: &TimeSegment{
				Start: trainEnd,
				End:   validEnd,
			},
			Test: TimeSegment{
				Start: validEnd,
				End:   testEnd,
			},
		}

		// 添加 Embargo
		if cfg.EmbargoDays > 0 {
			embargoEV := &TimeSegment{
				Start: addDays(trainEnd, 0),
				End:   addDays(trainEnd, cfg.EmbargoDays-1),
			}
			split.EmbargoTrainValid = embargoEV
			split.Valid.Start = addDays(embargoEV.End, 1)

			embargoVT := &TimeSegment{
				Start: addDays(validEnd, 0),
				End:   addDays(validEnd, cfg.EmbargoDays-1),
			}
			split.EmbargoValidTest = embargoVT
			split.Test.Start = addDays(embargoVT.End, 1)
		}

		// 添加 Purge
		if cfg.PurgeDays > 0 {
			purgeDates := make([]time.Time, cfg.PurgeDays)
			for j := 0; j < cfg.PurgeDays; j++ {
				purgeDates[j] = addDays(trainEnd, -j)
			}
			split.PurgeDates = purgeDates
			split.Train.End = addDays(trainEnd, -cfg.PurgeDays)
		}

		// 验证数据隔离
		if err := split.ValidateDataIsolation(); err != nil {
			continue // 跳过有问题的窗口
		}

		result.Windows = append(result.Windows, WalkForwardWindow{
			Index: i,
			Split: *split,
		})
	}

	if len(result.Windows) == 0 {
		return nil, fmt.Errorf("no valid walk-forward windows generated")
	}

	return result, nil
}

// ============================================================================
// 数据泄漏检测
// ============================================================================

// LeakDetector 泄漏检测器。
type LeakDetector struct {
	splitter *TimeSeriesSplitter
}

// NewLeakDetector 创建泄漏检测器。
func NewLeakDetector(splitter *TimeSeriesSplitter) *LeakDetector {
	return &LeakDetector{splitter: splitter}
}

// DetectLeakage 检测是否存在数据泄漏。
// 返回泄漏的日期列表和描述。
func (d *LeakDetector) DetectLeakage(dates []time.Time, split *SplitResult) ([]LeakInfo, error) {
	var leaks []LeakInfo

	for _, date := range dates {
		segment := split.SegmentAt(date)

		// 检查: 训练集日期不应在验证/测试集中
		if segment == SegmentTrain {
			// 检查该日期是否也出现在验证/测试集中
			if split.Valid != nil && split.Valid.Contains(date) {
				leaks = append(leaks, LeakInfo{
					Date:    date,
					Source:  "train",
					Target:  "valid",
					Message: fmt.Sprintf("Date %s is in both train and valid", date.Format("2006-01-02")),
				})
			}
			if split.Test.Contains(date) {
				leaks = append(leaks, LeakInfo{
					Date:    date,
					Source:  "train",
					Target:  "test",
					Message: fmt.Sprintf("Date %s is in both train and test", date.Format("2006-01-02")),
				})
			}
		}

		// 检查: Purge 日期不应被使用
		if segment == SegmentPurge {
			leaks = append(leaks, LeakInfo{
				Date:    date,
				Source:  "purge",
				Target:  "",
				Message: fmt.Sprintf("Date %s should be purged (label overlap)", date.Format("2006-01-02")),
			})
		}

		// 检查: Embargo 日期不应被使用
		if segment == SegmentEmbargo {
			leaks = append(leaks, LeakInfo{
				Date:    date,
				Source:  "embargo",
				Target:  "",
				Message: fmt.Sprintf("Date %s is in embargo period", date.Format("2006-01-02")),
			})
		}
	}

	return leaks, nil
}

// LeakInfo 泄漏信息。
type LeakInfo struct {
	Date    time.Time `json:"date"`
	Source  string    `json:"source"`
	Target  string    `json:"target,omitempty"`
	Message string    `json:"message"`
}

// HasCriticalLeak 检查是否存在严重泄漏 (训练与测试重叠).
func (d *LeakDetector) HasCriticalLeak(leaks []LeakInfo) bool {
	for _, leak := range leaks {
		if leak.Source == "train" && (leak.Target == "test" || leak.Target == "valid") {
			return true
		}
	}
	return false
}

// ============================================================================
// 辅助函数
// ============================================================================

// addDays 增加天数 (不考虑节假日).
func addDays(t time.Time, days int) time.Time {
	return t.AddDate(0, 0, days)
}

// FilterBySegment 按区间过滤数据。
// data: 原始数据 (按日期升序)
// Returns: 各区间的数据子集
func FilterBySegment(dates []time.Time, values map[string]interface{}, split *SplitResult) map[DataSegment][]time.Time {
	result := make(map[DataSegment][]time.Time)

	for _, date := range dates {
		segment := split.SegmentAt(date)
		if segment != "" {
			result[segment] = append(result[segment], date)
		}
	}

	return result
}
