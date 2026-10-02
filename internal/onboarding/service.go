package onboarding

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/selection"
)

// DataFreshness 返回库内真实日线的最新交易日，用于判断「行情是否就绪」。
type DataFreshness interface {
	LatestKlineDate() (string, error)
}

// Syncer 触发一次真实行情同步（可选）。返回同步成功的代码数。
// 未装配时，走通只做数据新鲜度检查，不会伪造「已同步」。
type Syncer interface {
	SyncUniverse(ctx context.Context, codes []string) (int, error)
}

// SelectionRunner 运行每日选股。
type SelectionRunner interface {
	Run(ctx context.Context, req selection.Request) (*selection.Run, error)
}

// Deps 是引导编排的装配依赖。
type Deps struct {
	Builder   *marketsnapshot.Builder
	Snapshots marketsnapshot.Repository
	Features  marketsnapshot.FeatureEngine
	Selection SelectionRunner
	Freshness DataFreshness
	Syncer    Syncer // 可选
}

// Service 编排「一键走通」。
type Service struct {
	deps Deps
	now  func() time.Time
}

// NewService 构造引导服务。
func NewService(deps Deps) (*Service, error) {
	if deps.Builder == nil || deps.Snapshots == nil || deps.Features == nil || deps.Selection == nil {
		return nil, fmt.Errorf("onboarding requires builder, snapshots, features and selection")
	}
	return &Service{deps: deps, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Run 依次执行：数据新鲜度 → 市场快照 → 特征快照 → 选股。
// 任一步被真实条件阻断时立即停止，并在 BlockedReason 里说明下一步。
func (s *Service) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	res := &Result{Status: StatusCompleted, Steps: []Step{}}

	universeName := strings.TrimSpace(opts.Universe)
	if universeName == "" {
		universeName = "universe_usable"
	}
	def := resolveUniverse(universeName)

	tradeDate, err := s.resolveTradeDate(opts, def)
	if err != nil {
		return nil, err
	}
	res.TradeDate = tradeDate

	// 1. 行情数据就绪
	if !s.dataStep(ctx, opts, res, tradeDate, def) {
		return s.finish(res, StatusBlocked), nil
	}

	// 2. 市场快照
	market, blocked := s.marketStep(ctx, opts, res, tradeDate, def)
	if blocked != "" {
		res.BlockedReason = blocked
		return s.finish(res, StatusBlocked), nil
	}
	res.SnapshotID = market.ID

	// 3. 特征快照
	feature, blocked := s.featureStep(ctx, res, market)
	if blocked != "" {
		res.BlockedReason = blocked
		return s.finish(res, StatusBlocked), nil
	}
	res.FeatureSnapshotID = feature.ID

	// 4. 选股
	run, err := s.deps.Selection.Run(ctx, selection.Request{MarketSnapshotID: market.ID, FeatureSnapshotID: feature.ID})
	if err != nil {
		res.Steps = append(res.Steps, Step{Key: "selection", Label: "运行今日选股", Status: StepFailed, Detail: err.Error()})
		res.BlockedReason = "选股引擎执行失败：" + err.Error()
		return s.finish(res, StatusFailed), nil
	}
	res.SelectionRunID = run.ID
	res.CandidateCount = run.CandidateCount
	res.BuyCount = run.BuyCount
	res.ScannedStocks = run.ScannedStocks
	res.EligibleMethods = run.EligibleMethods
	detail := fmt.Sprintf("扫描 %d 只股票、%d 个通过资格的方法，得到 %d 个候选", run.ScannedStocks, run.EligibleMethods, run.CandidateCount)
	res.Steps = append(res.Steps, Step{Key: "selection", Label: "运行今日选股", Status: StepDone, Detail: detail})
	return s.finish(res, StatusCompleted), nil
}

func (s *Service) resolveTradeDate(opts RunOptions, def marketsnapshot.UniverseDefinition) (string, error) {
	if d := strings.TrimSpace(opts.Date); d != "" {
		return normalizeDate(d)
	}
	if s.deps.Freshness == nil {
		return "", fmt.Errorf("no trade date provided and no data freshness reader configured")
	}
	latest, err := s.deps.Freshness.LatestKlineDate()
	if err != nil {
		return "", fmt.Errorf("read latest kline date: %w", err)
	}
	if latest == "" {
		return "", fmt.Errorf("数据库里还没有任何真实日线，无法确定交易日")
	}
	return s.universeSupportedDate(latest, def), nil
}

// universeSupportedDate returns the latest date that at least half of the
// universe's kline watermarks have actually reached. The raw freshness
// watermark (MAX(date) across the kline table) can sit ahead of the universe
// when only a handful of codes synced ahead of schedule; using it as the trade
// date would build a snapshot with near-zero coverage and block the flow.
// Falls back to latest when the universe cannot be inspected.
func (s *Service) universeSupportedDate(latest string, def marketsnapshot.UniverseDefinition) string {
	b := s.deps.Builder
	if b == nil || b.UniverseProvider == nil || b.WatermarkProvider == nil {
		return latest
	}
	members, err := b.UniverseProvider.BuildUniverse(latest, def)
	if err != nil {
		return latest
	}
	codes := make([]string, 0, len(members))
	for _, m := range members {
		if m.Selected {
			codes = append(codes, m.Code)
		}
	}
	if len(codes) == 0 {
		return latest
	}
	watermarks, err := b.WatermarkProvider.FetchWatermarks(latest, codes)
	if err != nil {
		return latest
	}
	dates := make([]string, 0, len(codes))
	for _, c := range codes {
		wm, ok := watermarks[c]
		if !ok || wm.KlineLastDate == "" {
			continue
		}
		dates = append(dates, wm.KlineLastDate)
	}
	if len(dates) == 0 {
		return latest
	}
	sort.Strings(dates)
	half := (len(dates) + 1) / 2
	// dates 升序：dates[len-half] 起至少一半代码已到该日期。
	return dates[len(dates)-half]
}

// dataStep 检查行情就绪，必要时触发真实同步。返回 false 表示被阻断。
func (s *Service) dataStep(ctx context.Context, opts RunOptions, res *Result, tradeDate string, def marketsnapshot.UniverseDefinition) bool {
	latest := ""
	if s.deps.Freshness != nil {
		latest, _ = s.deps.Freshness.LatestKlineDate()
	}

	if opts.Sync && s.deps.Syncer != nil {
		members, err := s.deps.Builder.UniverseProvider.BuildUniverse(tradeDate, def)
		if err != nil {
			res.Steps = append(res.Steps, Step{Key: "sync", Label: "同步真实行情", Status: StepFailed, Detail: err.Error()})
			res.BlockedReason = "无法构建股票池：" + err.Error()
			return false
		}
		codes := make([]string, 0, len(members))
		for _, m := range members {
			if m.Selected {
				codes = append(codes, m.Code)
			}
		}
		n, syncErr := s.deps.Syncer.SyncUniverse(ctx, codes)
		if syncErr != nil {
			res.Steps = append(res.Steps, Step{Key: "sync", Label: "同步真实行情", Status: StepFailed, Detail: syncErr.Error()})
			res.BlockedReason = "行情同步失败：" + syncErr.Error()
			return false
		}
		res.Steps = append(res.Steps, Step{Key: "sync", Label: "同步真实行情", Status: StepDone, Detail: fmt.Sprintf("已同步 %d 只股票的真实日线", n)})
		return true
	}

	if latest == "" {
		res.Steps = append(res.Steps, Step{Key: "sync", Label: "同步真实行情", Status: StepBlocked, Detail: "数据库里还没有任何真实日线"})
		res.BlockedReason = "行情数据尚未就绪，请先同步真实行情"
		return false
	}
	res.Steps = append(res.Steps, Step{Key: "sync", Label: "同步真实行情", Status: StepSkipped,
		Detail: fmt.Sprintf("库内真实日线已到 %s，本次直接使用冻结数据", latest)})
	return true
}

// marketStep 复用或构建一个 ready+frozen 的市场快照。
func (s *Service) marketStep(ctx context.Context, opts RunOptions, res *Result, tradeDate string, def marketsnapshot.UniverseDefinition) (*marketsnapshot.MarketSnapshot, string) {
	adj := firstNonEmpty(s.deps.Builder.PriceAdjustment, "forward")
	if !opts.Force {
		if existing, err := s.deps.Snapshots.FindMarketSnapshot(tradeDate, def.Name, adj); err == nil && existing != nil && existing.IsReady() {
			res.Steps = append(res.Steps, Step{Key: "snapshot", Label: "冻结行情快照", Status: StepSkipped,
				Detail: fmt.Sprintf("复用已就绪快照 %s（覆盖 %.1f%%）", existing.ID, existing.CoveragePct*100)})
			return existing, ""
		}
	}

	b := *s.deps.Builder
	if opts.CoverageThreshold > 0 {
		b.CoverageThreshold = opts.CoverageThreshold
	}
	if opts.MaxGappedCodes > 0 {
		b.MaxGappedCodes = opts.MaxGappedCodes
	}
	b.Now = s.now()
	snapshot, err := b.Build(tradeDate, def)
	if err != nil {
		res.Steps = append(res.Steps, Step{Key: "snapshot", Label: "冻结行情快照", Status: StepFailed, Detail: err.Error()})
		return nil, "构建市场快照失败：" + err.Error()
	}
	if err := s.deps.Snapshots.SaveMarketSnapshot(snapshot); err != nil {
		res.Steps = append(res.Steps, Step{Key: "snapshot", Label: "冻结行情快照", Status: StepFailed, Detail: err.Error()})
		return nil, "保存市场快照失败：" + err.Error()
	}
	if snapshot.Status != marketsnapshot.StatusReady {
		reason := fmt.Sprintf("快照状态 %s：%s", snapshot.Status, snapshot.ReadinessReason)
		res.Steps = append(res.Steps, Step{Key: "snapshot", Label: "冻结行情快照", Status: StepBlocked, Detail: reason})
		return nil, "行情快照未达就绪门槛（" + reason + "）。请补齐真实日线后重试，或降低覆盖阈值。"
	}
	if err := s.deps.Snapshots.FreezeMarketSnapshot(snapshot.ID); err != nil {
		res.Steps = append(res.Steps, Step{Key: "snapshot", Label: "冻结行情快照", Status: StepFailed, Detail: err.Error()})
		return nil, "冻结市场快照失败：" + err.Error()
	}
	snapshot.Frozen = true
	res.Steps = append(res.Steps, Step{Key: "snapshot", Label: "冻结行情快照", Status: StepDone,
		Detail: fmt.Sprintf("构建并冻结快照 %s（覆盖 %.1f%%，%d/%d 只就绪）", snapshot.ID, snapshot.CoveragePct*100, snapshot.ReadyKlineCodes, snapshot.ExpectedKlineCodes)})
	return snapshot, ""
}

// featureStep 复用或构建一个 ready+leak-checked 的特征快照。
func (s *Service) featureStep(ctx context.Context, res *Result, market *marketsnapshot.MarketSnapshot) (*marketsnapshot.FeatureSnapshot, string) {
	if existing, err := s.deps.Snapshots.ListFeatureSnapshots(market.ID); err == nil {
		for _, item := range existing {
			if item.Status == marketsnapshot.StatusReady && item.LeakChecked {
				full, loadErr := s.deps.Snapshots.LoadFeatureSnapshot(item.ID, true)
				if loadErr == nil && full != nil {
					res.Steps = append(res.Steps, Step{Key: "features", Label: "物化特征快照", Status: StepSkipped,
						Detail: fmt.Sprintf("复用已就绪特征快照 %s（%d 个特征）", full.ID, full.FeatureTotal)})
					return full, ""
				}
			}
		}
	}
	b := *s.deps.Builder
	b.Now = s.now()
	feature, err := b.BuildFeatureSnapshot(market, marketsnapshot.DefaultDslFeatures(), s.deps.Features)
	if err != nil {
		res.Steps = append(res.Steps, Step{Key: "features", Label: "物化特征快照", Status: StepFailed, Detail: err.Error()})
		return nil, "构建特征快照失败：" + err.Error()
	}
	if err := s.deps.Snapshots.SaveFeatureSnapshot(feature); err != nil {
		res.Steps = append(res.Steps, Step{Key: "features", Label: "物化特征快照", Status: StepFailed, Detail: err.Error()})
		return nil, "保存特征快照失败：" + err.Error()
	}
	res.Steps = append(res.Steps, Step{Key: "features", Label: "物化特征快照", Status: StepDone,
		Detail: fmt.Sprintf("物化 %d 个特征、%d 行（%d 只股票）", feature.FeatureTotal, feature.RowsWritten, len(feature.Values))})
	return feature, ""
}

func (s *Service) finish(res *Result, status string) *Result {
	res.Status = status
	res.FinishedAt = s.now()
	return res
}

func resolveUniverse(name string) marketsnapshot.UniverseDefinition {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "universe_csi800":
		return marketsnapshot.DefaultUniverseCSI800()
	case "universe_all_a":
		return marketsnapshot.DefaultUniverseAllA()
	default:
		return marketsnapshot.DefaultUniverseUsable()
	}
}

func normalizeDate(v string) (string, error) {
	v = strings.TrimSpace(v)
	if len(v) == 8 {
		return v[:4] + "-" + v[4:6] + "-" + v[6:], nil
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return "", fmt.Errorf("invalid date %q: %w", v, err)
	}
	return v, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
