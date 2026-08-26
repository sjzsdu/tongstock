package validation

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/sjzsdu/tongstock/internal/methods"
)

// ============================================================================
// Factory — 验证工厂编排器
// 接收 ValidationJob + CompiledMethod + 数据依赖，产出 EvidenceBundle。
// 全程 fail closed：缺失真实数据或制品时返回错误，禁止降级到合成结果。
// ============================================================================

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

	code := job.StockCode
	if code == "" {
		// 单股验证工厂：本 Bead 聚焦单股；全 universe 由 ai.6+ 扩展
		return nil, fmt.Errorf("stock_code is required for single-stock validation (universe scope: future bead)")
	}

	dateStart, dateEnd, dates, err := f.resolveDateRange(ctx, job, code)
	if err != nil {
		return nil, err
	}
	if len(dates) < 30 {
		return nil, fmt.Errorf("insufficient real bars: %d (need >= 30) for %s in [%s,%s]",
			len(dates), code, dateStart, dateEnd)
	}

	// 1. 切分
	plan, err := PlanSegments(dates, job.SplitType)
	if err != nil {
		return nil, fmt.Errorf("plan segments: %w", err)
	}

	// 2. 逐段回测
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
			Start:   spec.DateStart,
			End:     spec.DateEnd,
			Stats:   result.Stats,
			Trades:  result.Trades,
		})
		if spec.Name != "train" && spec.Name != "valid" {
			oosObservationCount += len(segBars)
		}
	}

	// 3. 聚合样本外
	oosStats := AggregateOosStats(segResults)

	// 4. 基准超额。未提供指数时，使用同一标的的买入持有作为零参数基线。
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

	// 5. t 检验 + 多重检验
	oosTrades := collectOosTrades(segResults)
	returns := tradeReturns(oosTrades)
	pValue := TTestOnReturns(returns)
	mt := ApplyMultipleTesting(job.DiscoveryTrials, pValue)

	// 6. Critic 反证
	positionWeight := 1.0
	if f.deps.Method.Position.PctEquity != nil {
		positionWeight = *f.deps.Method.Position.PctEquity
	}
	criticIn := CriticInput{
		Job: job, Stats: oosStats, Split: plan.Split,
		FeatureCount: compiledFeatureCount(f.deps.Method), ObservationCount: oosObservationCount,
		MaxPositionWeight: positionWeight, Concentration: positionWeight * positionWeight,
	}
	issues, blockers := RunCritic(criticIn, nil)

	// 7. 置信度
	conf, passable := ComputeConfidence(ConfidenceInput{
		Stats:           oosStats,
		Blockers:        blockers,
		CriticIssues:    issues,
		MultipleTesting: mt,
		OosTradeCount:   len(oosTrades),
	})

	// 8. 组装 EvidenceBundle
	bundle := &EvidenceBundle{
		JobHash:         job.JobHash(),
		MethodHash:      job.MethodHash,
		MethodName:      f.deps.Method.Name,
		SnapshotID:      job.SnapshotID,
		StockCode:       job.StockCode,
		GeneratedAt:     time.Now().UTC(),
		Segments:        segResults,
		OosStats:        oosStats,
		DiscoveryTrials: job.DiscoveryTrials,
		BonferroniAlpha: mt.BonferroniAlpha,
		AdjustedPValue:  mt.AdjustedPValue,
		CriticIssues:    issues,
		Confidence:      conf,
		Blockers:        blockers,
		Passable:        passable,
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
		if segment.Segment == "train" || segment.Segment == "valid" {
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
		if s.Segment == "train" || s.Segment == "valid" {
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
