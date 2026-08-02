package validation

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/sjzsdu/tongstock/internal/backtest"
)

// ============================================================================
// 统计检验层 — walk-forward 段聚合、多重检验、t 检验
// ============================================================================

// SegmentPlan 是验证工厂对日期范围的切分计划。
// 固定切分产出 train/valid/test 三段；walk-forward 产出多窗口 test 段。
type SegmentPlan struct {
	// Segments 需要回测的段；样本外段（test）用于汇总。
	Segments []SegmentSpec
	// SplitResult 原始切分结果（用于泄漏审计）
	Split *backtest.SplitResult
	// WalkForward 多窗口结果（仅 walk-forward 模式）
	WalkForward *backtest.WalkForwardResult
	// IsWalkForward 是否滚动切分
	IsWalkForward bool
}

// PlanSegments 按 ValidationJob.SplitType 切分日期列表。
// dates 必须按升序排列的交易日（YYYY-MM-DD 字符串）。
// 默认使用 walk-forward（更严格），空值或 "walk_forward" → walk-forward；
// "fixed" → 固定切分。
func PlanSegments(dates []string, splitType string) (*SegmentPlan, error) {
	if len(dates) < 30 {
		return nil, fmt.Errorf("need at least 30 trading days for validation, got %d", len(dates))
	}
	times, err := parseDates(dates)
	if err != nil {
		return nil, err
	}

	if splitType == "" || splitType == "walk_forward" {
		return planWalkForward(dates, times)
	}
	if splitType == "fixed" {
		return planFixed(dates, times)
	}
	return nil, fmt.Errorf("unknown split_type %q", splitType)
}

func planFixed(dates []string, times []time.Time) (*SegmentPlan, error) {
	cfg := backtest.DefaultFixedSplit()
	splitter, err := backtest.NewTimeSeriesSplitter(cfg)
	if err != nil {
		return nil, err
	}
	split, err := splitter.Split(times)
	if err != nil {
		return nil, err
	}
	if err := split.ValidateDataIsolation(); err != nil {
		return nil, fmt.Errorf("data isolation violated: %w", err)
	}
	plan := &SegmentPlan{
		Split: split,
		Segments: []SegmentSpec{
			{Name: "train", DateStart: formatDate(split.Train.Start), DateEnd: formatDate(split.Train.End)},
		},
	}
	if split.Valid != nil {
		plan.Segments = append(plan.Segments, SegmentSpec{
			Name: "valid", DateStart: formatDate(split.Valid.Start), DateEnd: formatDate(split.Valid.End),
		})
	}
	plan.Segments = append(plan.Segments, SegmentSpec{
		Name: "test", DateStart: formatDate(split.Test.Start), DateEnd: formatDate(split.Test.End),
	})
	return plan, nil
}

func planWalkForward(dates []string, times []time.Time) (*SegmentPlan, error) {
	cfg := backtest.DefaultWalkForwardConfig()
	// 根据数据量缩放：若数据不足以支持默认 3 窗口，则减少窗口数
	needed := cfg.TrainWindowDays + cfg.ValidWindowDays + cfg.TestWindowDays + (cfg.Windows-1)*cfg.StepDays
	if len(times) < needed {
		// 缩减为单窗口 fixed-like 滚动
		cfg.Windows = 1
		cfg.TrainWindowDays = max(60, len(times)*50/100)
		cfg.ValidWindowDays = max(20, len(times)*15/100)
		cfg.TestWindowDays = len(times) - cfg.TrainWindowDays - cfg.ValidWindowDays - cfg.EmbargoDays
		if cfg.TestWindowDays < 20 {
			return planFixed(dates, times)
		}
	}
	splitter, err := backtest.NewWalkForwardSplitter(cfg)
	if err != nil {
		return nil, err
	}
	wf, err := splitter.SplitWalkForward(times)
	if err != nil {
		return nil, err
	}
	plan := &SegmentPlan{IsWalkForward: true, WalkForward: wf}
	for _, w := range wf.Windows {
		// 仅回测每个窗口的 test 段作为样本外证据
		plan.Segments = append(plan.Segments, SegmentSpec{
			Name:      fmt.Sprintf("test_w%d", w.Index),
			DateStart: formatDate(w.Split.Test.Start),
			DateEnd:   formatDate(w.Split.Test.End),
		})
	}
	return plan, nil
}

// AggregateOosStats 聚合样本外（test）段的统计指标。
// 采用按交易数加权的简化聚合：合并所有 test 段的交易记录后重算。
func AggregateOosStats(segResults []SegmentResult) PerformanceStats {
	if len(segResults) == 0 {
		return PerformanceStats{}
	}
	var allTrades []TradeRecord
	var oosSegments []SegmentResult
	for _, s := range segResults {
		if s.Segment == "train" || s.Segment == "valid" {
			continue
		}
		oosSegments = append(oosSegments, s)
		allTrades = append(allTrades, s.Trades...)
	}
	if len(oosSegments) == 0 {
		// 没有显式 test 段，用最后一段
		last := segResults[len(segResults)-1]
		return last.Stats
	}
	// 用合并交易重算交易级指标；资金曲线级取各段均值。
	// 即使测试构造的段没有交易明细，也不能只返回第一段。
	stats := PerformanceStats{}
	if len(allTrades) > 0 {
		stats = computeStats(allTrades, nil, 0, 0)
	}
	// 资金曲线级指标取各 test 段均值
	var sumReturn, sumSharpe, sumSortino, sumCalmar, sumMaxDD, sumAnnual float64
	var totalCost, totalTradeValue float64
	count := 0
	for _, s := range oosSegments {
		if s.Stats.TotalTrades > 0 {
			sumReturn += s.Stats.TotalReturn
			sumSharpe += s.Stats.SharpeRatio
			sumSortino += s.Stats.SortinoRatio
			sumCalmar += s.Stats.CalmarRatio
			sumMaxDD += s.Stats.MaxDrawdown
			sumAnnual += s.Stats.AnnualReturn
			count++
		}
		totalCost += s.Stats.TotalCost
		for _, trade := range s.Trades {
			totalTradeValue += trade.BuyPrice * float64(trade.Quantity)
		}
	}
	if count > 0 {
		stats.TotalReturn = sumReturn / float64(count)
		stats.SharpeRatio = sumSharpe / float64(count)
		stats.SortinoRatio = sumSortino / float64(count)
		stats.CalmarRatio = sumCalmar / float64(count)
		stats.MaxDrawdown = sumMaxDD / float64(count)
		stats.AnnualReturn = sumAnnual / float64(count)
	}
	stats.TotalCost = totalCost
	if totalTradeValue > 0 {
		stats.CostRatio = totalCost / totalTradeValue
	}
	return stats
}

// ============================================================================
// 多重检验（Bonferroni）与 t 检验
// ============================================================================

// MultipleTestingResult 多重检验结果。
type MultipleTestingResult struct {
	Trials          int     // 发现阶段尝试次数
	NominalAlpha    float64 // 名义显著性水平（默认 0.05）
	BonferroniAlpha float64 // Bonferroni 校正后的 alpha = nominalAlpha / trials
	AdjustedPValue  float64 // 校正后 p 值（= min(p * trials, 1)）
	Significant     bool    // 校正后是否仍显著
}

// ApplyMultipleTesting 根据发现阶段尝试次数施加 Bonferroni 惩罚。
// trials <= 1 时不惩罚。pValue 是单次检验的 p 值。
func ApplyMultipleTesting(trials int, pValue float64) MultipleTestingResult {
	const nominalAlpha = 0.05
	res := MultipleTestingResult{
		Trials:       trials,
		NominalAlpha: nominalAlpha,
	}
	if trials <= 1 {
		res.BonferroniAlpha = nominalAlpha
		res.AdjustedPValue = pValue
		res.Significant = pValue >= 0 && pValue < nominalAlpha
		return res
	}
	res.BonferroniAlpha = nominalAlpha / float64(trials)
	adjusted := pValue * float64(trials)
	if adjusted > 1 {
		adjusted = 1
	}
	res.AdjustedPValue = adjusted
	res.Significant = pValue >= 0 && adjusted < nominalAlpha
	return res
}

// TTestOnReturns 对交易收益率序列做单样本 t 检验（H0: 均值 = 0）。
// 返回 p 值（双侧）。样本不足时返回 1.0（无法拒绝 H0）。
func TTestOnReturns(returns []float64) float64 {
	n := len(returns)
	if n < 2 {
		return 1.0
	}
	mean, std := meanStd(returns)
	if std == 0 {
		// 无离散度：若均值非零则极端显著，否则无法判断
		if mean == 0 {
			return 1.0
		}
		return 0.0
	}
	se := std / math.Sqrt(float64(n))
	tStat := mean / se
	// 自由度 = n-1；用正态近似（n>=30 时精确度足够，小样本保守）
	df := float64(n - 1)
	pValue := 2.0 * (1.0 - studentTCDF(math.Abs(tStat), df))
	if pValue < 0 {
		pValue = 0
	}
	if pValue > 1 {
		pValue = 1
	}
	return pValue
}

// studentTCDF 用不完全贝塔函数计算 t 分布累积分布函数（双精度近似）。
// 这是数值稳定的标准实现，避免引入外部统计库。
func studentTCDF(t, df float64) float64 {
	x := df / (df + t*t)
	ib := regularizedIncompleteBeta(x, 0.5*df, 0.5)
	if t > 0 {
		return 1 - 0.5*ib
	}
	return 0.5 * ib
}

// regularizedIncompleteBeta 正则化不完全贝塔函数 I_x(a,b)。
// 采用连分数展开（Numerical Recipes 风格）。
func regularizedIncompleteBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	lgA, _ := math.Lgamma(a)
	lgB, _ := math.Lgamma(b)
	lgAB, _ := math.Lgamma(a + b)
	lbeta := lgA + lgB - lgAB
	front := math.Exp(math.Log(x)*a+math.Log(1-x)*b-lbeta) / a
	if x < (a+1)/(a+b+2) {
		return front * betaCF(a, b, x)
	}
	return 1 - front*betaCF(b, a, 1-x)
}

func betaCF(a, b, x float64) float64 {
	const maxIter = 200
	const eps = 3e-7
	qab := a + b
	qap := a + 1
	qam := a - 1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < 1e-30 {
		d = 1e-30
	}
	d = 1 / d
	h := d
	for m := 1; m <= maxIter; m++ {
		mf := float64(m)
		aa := mf * (b - mf) * x / ((qam + 2*mf) * (a + 2*mf))
		d = 1 + aa*d
		if math.Abs(d) < 1e-30 {
			d = 1e-30
		}
		c = 1 + aa/c
		if math.Abs(c) < 1e-30 {
			c = 1e-30
		}
		d = 1 / d
		h *= d * c
		aa = -(a + mf) * (qab + mf) * x / ((a + 2*mf) * (qap + 2*mf))
		d = 1 + aa*d
		if math.Abs(d) < 1e-30 {
			d = 1e-30
		}
		c = 1 + aa/c
		if math.Abs(c) < 1e-30 {
			c = 1e-30
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return h
}

// ============================================================================
// 辅助函数
// ============================================================================

func parseDates(dates []string) ([]time.Time, error) {
	times := make([]time.Time, len(dates))
	for i, d := range dates {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return nil, fmt.Errorf("invalid date %q: %w", d, err)
		}
		times[i] = t
	}
	// 验证升序
	for i := 1; i < len(times); i++ {
		if times[i].Before(times[i-1]) {
			return nil, fmt.Errorf("dates must be ascending; %s before %s", dates[i], dates[i-1])
		}
	}
	return times, nil
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SortTradesByBuyDate 原地按买入执行日期排序。
func SortTradesByBuyDate(trades []TradeRecord) {
	sort.Slice(trades, func(i, j int) bool {
		return trades[i].BuyExecutionDate < trades[j].BuyExecutionDate
	})
}
