package validation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/backtest"
	"github.com/sjzsdu/tongstock/internal/methods"
)

// ============================================================================
// Factory — 验证工厂编排器
// 接收 ValidationJob + CompiledMethod + 数据依赖，产出 EvidenceBundle。
// 全程 fail closed：缺失真实数据或制品时返回错误，禁止降级到合成结果。
//
// 两种验证口径：
//   - 单股（job.StockCode 非空）：只在该标的上回测，基准默认为同标的买入持有。
//   - 全股票池（job.StockCode 为空，job.Universe 非空）：在每个真实代码上回测，
//     合并样本外交易；基准为等权全池买入持有（用同一批真实 K 线计算）。
// ============================================================================

// minUniverseValidCodes 是全股票池验证要求的最少有效代码数。
// 低于此值说明数据覆盖不足，结论不可信，必须 fail closed。
const minUniverseValidCodes = 5

// Factory 验证工厂。
type Factory struct {
	deps FactoryDeps
}

// NewFactory 创建验证工厂。deps 必须包含 Method 和 Bars。
func NewFactory(deps FactoryDeps) (*Factory, error) {
	if deps.Method == nil {
		return nil, fmt.Errorf("method is required")
	}
	if deps.Bars == nil {
		return nil, fmt.Errorf("bar provider is required")
	}
	return &Factory{deps: deps}, nil
}

// Run 执行完整验证流水线。
// job 描述验证范围；返回的 EvidenceBundle 包含所有证据和最终判定。
func (f *Factory) Run(ctx context.Context, job ValidationJob) (*EvidenceBundle, error) {
	if err := job.Validate(); err != nil {
		return nil, fmt.Errorf("invalid job: %w", err)
	}
	if !f.deps.Method.IsExecutable() {
		return nil, fmt.Errorf("method %q is not executable", f.deps.Method.Name)
	}
	if f.deps.Method.ContentHash != job.MethodHash {
		return nil, fmt.Errorf("method hash mismatch: job=%s method=%s",
			job.MethodHash, f.deps.Method.ContentHash)
	}
	if job.StockCode == "" {
		return f.runUniverse(ctx, job)
	}
	return f.runSingle(ctx, job, job.StockCode)
}

// runSingle 单标的验证。
func (f *Factory) runSingle(ctx context.Context, job ValidationJob, code string) (*EvidenceBundle, error) {
	dateStart, dateEnd, dates, err := f.resolveDateRange(ctx, job, code)
	if err != nil {
		return nil, err
	}
	if len(dates) < 30 {
		return nil, fmt.Errorf("insufficient real bars: %d (need >= 30) for %s in [%s,%s]",
			len(dates), code, dateStart, dateEnd)
	}

	plan, err := PlanSegments(dates, job.SplitType)
	if err != nil {
		return nil, fmt.Errorf("plan segments: %w", err)
	}

	segResults := make([]SegmentResult, 0, len(plan.Segments))
	oosObservationCount := 0
	for _, spec := range plan.Segments {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		segBars, err := f.deps.Bars.LoadBars(ctx, job.SnapshotID, code, spec.DateStart, spec.DateEnd)
		if err != nil {
			return nil, fmt.Errorf("load bars for %s %s: %w", spec.Name, code, err)
		}
		if len(segBars) == 0 {
			return nil, fmt.Errorf("segment %s has no real bars for %s: fail closed", spec.Name, code)
		}
		cfg, err := f.backtestConfig(job)
		if err != nil {
			return nil, err
		}
		result, err := RunBacktest(ctx, f.deps.Method, segBars, cfg)
		if err != nil {
			return nil, fmt.Errorf("backtest segment %s: %w", spec.Name, err)
		}
		segResults = append(segResults, SegmentResult{
			Segment: spec.Name,
			Code:    code,
			Start:   spec.DateStart,
			End:     spec.DateEnd,
			Stats:   result.Stats,
			Trades:  result.Trades,
		})
		if spec.Name != "train" && spec.Name != "valid" {
			oosObservationCount += len(segBars)
		}
	}

	oosStats := AggregateOosStats(segResults)

	// 基准超额。未提供指数时，使用同一标的的买入持有作为零参数基线。
	// 基准缺失不能静默降级，必须 fail closed。
	if f.deps.Benchmark == nil {
		return nil, fmt.Errorf("benchmark provider is required: fail closed")
	}
	benchmarkCode := job.BenchmarkCode
	if benchmarkCode == "" {
		benchmarkCode = code
	}
	benchmarkStart, benchmarkEnd := oosDateRange(segResults, dateStart, dateEnd)
	rets, err := f.deps.Benchmark.LoadDailyReturns(ctx, job.SnapshotID, benchmarkCode, benchmarkStart, benchmarkEnd)
	if err != nil {
		return nil, fmt.Errorf("load benchmark %s: %w", benchmarkCode, err)
	}
	if len(rets) == 0 {
		return nil, fmt.Errorf("benchmark %s has no returns: fail closed", benchmarkCode)
	}
	oosStats.BenchmarkReturn = computeBenchmarkReturn(rets)
	oosStats.ExcessReturn = oosStats.TotalReturn - oosStats.BenchmarkReturn

	return f.assemble(job, segResults, oosStats, oosObservationCount, 0, 0, nil, plan.Split)
}

// runUniverse 全股票池验证。
// 每个真实代码独立回测，样本外交易合并后统一检验；基准为等权全池买入持有。
func (f *Factory) runUniverse(ctx context.Context, job ValidationJob) (*EvidenceBundle, error) {
	codes := dedupeSortedCodes(job.Universe)
	if len(codes) == 0 {
		return nil, fmt.Errorf("universe scope requires a non-empty universe code list: fail closed")
	}

	dateStart, dateEnd := job.DateStart, job.DateEnd
	if dateStart == "" {
		dateStart = "0001-01-01"
	}
	if dateEnd == "" {
		dateEnd = "9999-12-31"
	}

	// 第一遍：加载每个代码的真实 K 线，构建全池交易日并集。
	type codeBars struct {
		code string
		bars []BacktestBar
	}
	loaded := make([]codeBars, 0, len(codes))
	skipped := make([]SkippedCode, 0)
	dateSet := map[string]struct{}{}
	for _, code := range codes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bars, err := f.deps.Bars.LoadBars(ctx, job.SnapshotID, code, dateStart, dateEnd)
		if err != nil {
			return nil, fmt.Errorf("load bars for %s: %w", code, err)
		}
		if len(bars) < 30 {
			skipped = append(skipped, SkippedCode{Code: code, Reason: fmt.Sprintf("real bars %d < 30", len(bars))})
			continue
		}
		for _, b := range bars {
			dateSet[b.Date] = struct{}{}
		}
		loaded = append(loaded, codeBars{code: code, bars: bars})
	}
	if len(loaded) < minUniverseValidCodes {
		return nil, fmt.Errorf("universe coverage too low: %d valid codes < %d: fail closed",
			len(loaded), minUniverseValidCodes)
	}

	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	if len(dates) < 30 {
		return nil, fmt.Errorf("universe has only %d trading days (need >= 30): fail closed", len(dates))
	}

	plan, err := PlanSegments(dates, job.SplitType)
	if err != nil {
		return nil, fmt.Errorf("plan segments: %w", err)
	}
	cfg, err := f.backtestConfig(job)
	if err != nil {
		return nil, err
	}

	// 等权全池基准：按窗口累计每日横截面平均收益，避免用单标的冒充市场。
	windowReturns := make([]map[string]float64, len(plan.Segments))
	windowSums := make([]map[string]float64, len(plan.Segments))
	windowCounts := make([]map[string]int, len(plan.Segments))
	for i := range plan.Segments {
		windowReturns[i] = map[string]float64{}
		windowSums[i] = map[string]float64{}
		windowCounts[i] = map[string]int{}
	}

	segResults := make([]SegmentResult, 0, len(loaded)*len(plan.Segments))
	validCodes := 0
	oosObservationCount := 0
	for _, cb := range loaded {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		codeHasData := false
		for i, spec := range plan.Segments {
			segBars := barsInRange(cb.bars, spec.DateStart, spec.DateEnd)
			if len(segBars) < 2 {
				continue
			}
			result, err := RunBacktest(ctx, f.deps.Method, segBars, cfg)
			if err != nil {
				return nil, fmt.Errorf("backtest %s segment %s: %w", cb.code, spec.Name, err)
			}
			codeHasData = true
			segResults = append(segResults, SegmentResult{
				Segment: spec.Name,
				Code:    cb.code,
				Start:   spec.DateStart,
				End:     spec.DateEnd,
				Stats:   result.Stats,
				Trades:  result.Trades,
			})
			if isOosSegment(spec.Name) {
				oosObservationCount += len(segBars)
				accumulateDailyReturns(windowSums[i], windowCounts[i], segBars)
			}
		}
		if codeHasData {
			validCodes++
		} else {
			skipped = append(skipped, SkippedCode{Code: cb.code, Reason: "no bars in any validation segment"})
		}
	}
	if validCodes < minUniverseValidCodes {
		return nil, fmt.Errorf("universe valid codes %d < %d after segmentation: fail closed",
			validCodes, minUniverseValidCodes)
	}

	oosStats := AggregateOosStats(segResults)
	oosStats.BenchmarkReturn = meanWindowReturn(windowSums, windowCounts, plan.Segments)
	oosStats.ExcessReturn = oosStats.TotalReturn - oosStats.BenchmarkReturn

	return f.assemble(job, segResults, oosStats, oosObservationCount, len(codes), validCodes, skipped, plan.Split)
}

// assemble 汇总证据、critic 反证与置信度，产出可复现的 EvidenceBundle。
func (f *Factory) assemble(job ValidationJob, segResults []SegmentResult, oosStats PerformanceStats, oosObservationCount, universeSize, validCodes int, skipped []SkippedCode, split *backtest.SplitResult) (*EvidenceBundle, error) {
	oosTrades := collectOosTrades(segResults)
	returns := tradeReturns(oosTrades)
	pValue := TTestOnReturns(returns)
	mt := ApplyMultipleTesting(job.DiscoveryTrials, pValue)

	positionWeight := 1.0
	if f.deps.Method.Position.PctEquity != nil {
		positionWeight = *f.deps.Method.Position.PctEquity
	}
	criticIn := CriticInput{
		Job: job, Stats: oosStats, Split: split,
		FeatureCount: compiledFeatureCount(f.deps.Method), ObservationCount: oosObservationCount,
		MaxPositionWeight: positionWeight, Concentration: positionWeight * positionWeight,
	}
	issues, blockers := RunCritic(criticIn, nil)

	confidenceIn := ConfidenceInput{
		Stats:           oosStats,
		Blockers:        blockers,
		CriticIssues:    issues,
		MultipleTesting: mt,
		OosTradeCount:   len(oosTrades),
	}
	conf, passable := ComputeConfidence(confidenceIn)

	bundle := &EvidenceBundle{
		JobHash:         job.JobHash(),
		MethodHash:      job.MethodHash,
		MethodName:      f.deps.Method.Name,
		SnapshotID:      job.SnapshotID,
		StockCode:       job.StockCode,
		GeneratedAt:     time.Now().UTC(),
		UniverseSize:    universeSize,
		ValidCodes:      validCodes,
		Skipped:         skipped,
		Segments:        segResults,
		OosStats:        oosStats,
		DiscoveryTrials: job.DiscoveryTrials,
		BonferroniAlpha: mt.BonferroniAlpha,
		AdjustedPValue:  mt.AdjustedPValue,
		CriticIssues:     issues,
		Confidence:       conf,
		ConfidenceReason: ExplainConfidence(confidenceIn),
		Blockers:         blockers,
		Passable:         passable,
	}
	bundle.ResultHash = bundle.ComputeResultHash()
	return bundle, nil
}

// resolveDateRange 确定回测日期范围并返回升序交易日列表。
func (f *Factory) resolveDateRange(ctx context.Context, job ValidationJob, code string) (string, string, []string, error) {
	// 快照创建阶段已根据真实数据选择范围；这里只读冻结数据。
	start := job.DateStart
	end := job.DateEnd
	if start == "" {
		start = "0001-01-01"
	}
	if end == "" {
		end = "9999-12-31"
	}

	bars, err := f.deps.Bars.LoadBars(ctx, job.SnapshotID, code, start, end)
	if err != nil {
		return "", "", nil, fmt.Errorf("load bars for %s: %w", code, err)
	}
	if len(bars) == 0 {
		return "", "", nil, fmt.Errorf("no real bars for %s in [%s,%s]: fail closed", code, start, end)
	}
	dates := make([]string, len(bars))
	for i, b := range bars {
		dates[i] = b.Date
	}
	return dates[0], dates[len(dates)-1], dates, nil
}

func (f *Factory) backtestConfig(job ValidationJob) (BacktestConfig, error) {
	cfg := DefaultBacktestConfig()
	if job.InitialCash > 0 {
		cfg.InitialCash = job.InitialCash
	}
	switch f.deps.Method.Position.Mode {
	case "pct_equity":
		if f.deps.Method.Position.PctEquity == nil || *f.deps.Method.Position.PctEquity <= 0 || *f.deps.Method.Position.PctEquity > 1 {
			return BacktestConfig{}, fmt.Errorf("method has invalid pct_equity position rule")
		}
		cfg.PositionPct = *f.deps.Method.Position.PctEquity
	case "":
		// 旧版编译制品未显式保存仓位时，使用审计可见的默认满仓。
	default:
		return BacktestConfig{}, fmt.Errorf("unsupported position mode %q: fail closed", f.deps.Method.Position.Mode)
	}
	return cfg, nil
}

func oosDateRange(segments []SegmentResult, fallbackStart, fallbackEnd string) (string, string) {
	start, end := "", ""
	for _, segment := range segments {
		if !isOosSegment(segment.Segment) {
			continue
		}
		if start == "" || segment.Start < start {
			start = segment.Start
		}
		if end == "" || segment.End > end {
			end = segment.End
		}
	}
	if start == "" {
		start = fallbackStart
	}
	if end == "" {
		end = fallbackEnd
	}
	return start, end
}

func isOosSegment(name string) bool {
	return name != "train" && name != "valid"
}

func dedupeSortedCodes(codes []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// barsInRange 返回 [start,end] 闭区间内的 K 线（bars 已按日期升序）。
func barsInRange(bars []BacktestBar, start, end string) []BacktestBar {
	lo := sort.Search(len(bars), func(i int) bool { return bars[i].Date >= start })
	hi := sort.Search(len(bars), func(i int) bool { return bars[i].Date > end })
	if lo >= hi {
		return nil
	}
	return bars[lo:hi]
}

// accumulateDailyReturns 把一批 K 线的日收益累加到横截面聚合器（等权口径）。
func accumulateDailyReturns(sums map[string]float64, counts map[string]int, bars []BacktestBar) {
	for i := 1; i < len(bars); i++ {
		previous := bars[i-1].Close
		if previous <= 0 {
			continue
		}
		sums[bars[i].Date] += (bars[i].Close - previous) / previous
		counts[bars[i].Date]++
	}
}

// meanWindowReturn 对每个样本外窗口计算等权全池买入持有收益，再取窗口均值。
// 与 AggregateOosStats 的「按窗口/标的求均值」口径对齐，避免不同期限直接比较。
func meanWindowReturn(sums []map[string]float64, counts []map[string]int, specs []SegmentSpec) float64 {
	total, windows := 0.0, 0
	for i, spec := range specs {
		if !isOosSegment(spec.Name) {
			continue
		}
		avg := map[string]float64{}
		for date, count := range counts[i] {
			if count > 0 {
				avg[date] = sums[i][date] / float64(count)
			}
		}
		if len(avg) == 0 {
			continue
		}
		total += computeBenchmarkReturn(avg)
		windows++
	}
	if windows == 0 {
		return 0
	}
	return total / float64(windows)
}

func compiledFeatureCount(method *methods.CompiledMethod) int {
	features := map[string]struct{}{}
	var visit func(*methods.Expr)
	visit = func(expr *methods.Expr) {
		if expr == nil {
			return
		}
		if expr.Indicator != "" {
			features[expr.Indicator] = struct{}{}
		}
		visit(expr.Left)
		visit(expr.Right)
		for _, child := range expr.Children {
			visit(child)
		}
	}
	visit(method.EntryRule)
	visit(method.ExitRule)
	visit(method.InvalidRule)
	return len(features)
}

// collectOosTrades 收集所有样本外段的交易。
func collectOosTrades(segResults []SegmentResult) []TradeRecord {
	var out []TradeRecord
	for _, s := range segResults {
		if !isOosSegment(s.Segment) {
			continue
		}
		out = append(out, s.Trades...)
	}
	if len(out) == 0 {
		// 无显式 test 段时取最后一段
		if len(segResults) > 0 {
			out = segResults[len(segResults)-1].Trades
		}
	}
	return out
}

func tradeReturns(trades []TradeRecord) []float64 {
	out := make([]float64, len(trades))
	for i, t := range trades {
		out[i] = t.ReturnPct
	}
	return out
}

func computeBenchmarkReturn(rets map[string]float64) float64 {
	if len(rets) == 0 {
		return 0
	}
	dates := make([]string, 0, len(rets))
	for d := range rets {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	cum := 1.0
	for _, d := range dates {
		cum *= (1 + rets[d])
	}
	return cum - 1
}
