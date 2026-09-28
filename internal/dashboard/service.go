package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/trading"
)

// MarketSnapshotReader 读取市场快照元信息。
type MarketSnapshotReader interface {
	ListMarketSnapshots(dateStart, dateEnd, status string) ([]*marketsnapshot.MarketSnapshot, error)
}

// MethodReader 读取可信方法库。
type MethodReader interface {
	Query(context.Context, methodregistry.Query) ([]*methodregistry.Method, error)
}

// SelectionReader 读取每日选股运行。
type SelectionReader interface {
	List(context.Context, string, string, int) ([]*selection.Run, error)
}

// PositionReader 读取持仓判断运行。
type PositionReader interface {
	List(context.Context, string, int) ([]*positiondecision.Run, error)
}

// HoldingReader 读取真实持仓。
type HoldingReader interface {
	GetAllPositions() ([]trading.Trade, error)
}

// DataDateReader 返回库内真实日线的最新交易日。
type DataDateReader interface {
	LatestKlineDate() (string, error)
}

// Deps 是「今日状态」读模型的装配依赖。
type Deps struct {
	Snapshots  MarketSnapshotReader
	Methods    MethodReader
	Selections SelectionReader
	Positions  PositionReader
	Holdings   HoldingReader
	DataDates  DataDateReader
}

// Service 聚合今日状态。它只读，不写入任何业务事实。
type Service struct {
	deps Deps
	now  func() time.Time
}

// NewService 构造读模型服务。
func NewService(deps Deps) (*Service, error) {
	if deps.Snapshots == nil || deps.Methods == nil || deps.Selections == nil {
		return nil, fmt.Errorf("dashboard requires snapshot, method and selection readers")
	}
	return &Service{deps: deps, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Today 返回首屏所需的完整状态。
func (s *Service) Today(ctx context.Context) (*Today, error) {
	out := &Today{AsOf: s.now()}

	data, err := s.dataStatus()
	if err != nil {
		return nil, err
	}
	out.Data = data

	methods, err := s.methodStatus(ctx)
	if err != nil {
		return nil, err
	}
	out.Methods = methods

	sel, err := s.selectionStatus(ctx, data.LatestSnapshotID)
	if err != nil {
		return nil, err
	}
	out.Selection = sel

	pos, err := s.positionStatus(ctx)
	if err != nil {
		return nil, err
	}
	out.Positions = pos

	out.WorkLog = buildWorkLog(sel, methods)
	out.Empty = decideEmptyState(data, methods, sel)
	out.Health = buildHealth(data, methods, sel, pos)
	return out, nil
}

func (s *Service) dataStatus() (DataStatus, error) {
	st := DataStatus{}
	if s.deps.DataDates != nil {
		latest, err := s.deps.DataDates.LatestKlineDate()
		if err != nil {
			return st, fmt.Errorf("read latest kline date: %w", err)
		}
		st.LatestKlineDate = latest
	}
	snaps, err := s.deps.Snapshots.ListMarketSnapshots("", "", "")
	if err != nil {
		return st, fmt.Errorf("list market snapshots: %w", err)
	}
	// 优先使用最近一个 ready 快照；否则退回最近一个快照。
	var latest *marketsnapshot.MarketSnapshot
	for _, x := range snaps {
		if x == nil {
			continue
		}
		if latest == nil {
			latest = x
		}
		if x.Status == marketsnapshot.StatusReady {
			latest = x
			break
		}
	}
	if latest == nil {
		st.Detail = "还没有任何市场快照"
		return st, nil
	}
	st.LatestSnapshotDate = latest.SnapshotDate
	st.LatestSnapshotID = latest.ID
	st.SnapshotStatus = latest.Status
	st.SnapshotFrozen = latest.Frozen
	st.CoveragePct = latest.CoveragePct
	st.ReadyCodes = latest.ReadyKlineCodes
	st.ExpectedCodes = latest.ExpectedKlineCodes
	st.Fresh = latest.Frozen && latest.Status == marketsnapshot.StatusReady &&
		(st.LatestKlineDate == "" || latest.SnapshotDate >= st.LatestKlineDate)
	switch {
	case latest.Status != marketsnapshot.StatusReady:
		st.Detail = fmt.Sprintf("最近快照状态 %s：%s", latest.Status, latest.ReadinessReason)
	case !latest.Frozen:
		st.Detail = "最近快照尚未冻结，选股不会使用它"
	case !st.Fresh:
		st.Detail = fmt.Sprintf("库内最新交易日 %s 尚未构建快照（最近快照 %s）", st.LatestKlineDate, latest.SnapshotDate)
	default:
		st.Detail = fmt.Sprintf("快照覆盖 %.1f%%，数据已就绪", latest.CoveragePct*100)
	}
	return st, nil
}

func (s *Service) methodStatus(ctx context.Context) (MethodStatus, error) {
	items, err := s.deps.Methods.Query(ctx, methodregistry.Query{Limit: 10000})
	if err != nil {
		return MethodStatus{}, fmt.Errorf("query methods: %w", err)
	}
	st := MethodStatus{Total: len(items), ByStatus: map[string]int{}}
	for _, m := range items {
		if m == nil {
			continue
		}
		st.ByStatus[string(m.Status)]++
		switch m.Status {
		case methodregistry.StatusVerified:
			st.Verified++
		case methodregistry.StatusObserving:
			st.Observing++
		case methodregistry.StatusCandidate:
			st.Candidate++
		case methodregistry.StatusRejected:
			st.Rejected++
		case methodregistry.StatusDegraded:
			st.Degraded++
		case methodregistry.StatusRetired:
			st.Retired++
		}
	}
	return st, nil
}

func (s *Service) selectionStatus(ctx context.Context, latestSnapshotID string) (*SelectionStatus, error) {
	runs, err := s.deps.Selections.List(ctx, "", "", 30)
	if err != nil {
		return nil, fmt.Errorf("list selection runs: %w", err)
	}
	if len(runs) == 0 {
		return nil, nil
	}
	// 优先选与最新快照匹配的运行；否则退回最近一次运行。
	var chosen *selection.Run
	for _, r := range runs {
		if r == nil {
			continue
		}
		if latestSnapshotID != "" && r.SnapshotID == latestSnapshotID {
			chosen = r
			break
		}
		if chosen == nil {
			chosen = r
		}
	}
	if chosen == nil {
		return nil, nil
	}
	st := &SelectionStatus{
		RunID:             chosen.ID,
		SnapshotID:        chosen.SnapshotID,
		FeatureSnapshotID: chosen.FeatureSnapshotID,
		SnapshotDate:      chosen.SnapshotDate,
		Status:            chosen.Status,
		CreatedAt:         chosen.CreatedAt,
		ScannedStocks:     chosen.ScannedStocks,
		EligibleMethods:   chosen.EligibleMethods,
		CandidateCount:    chosen.CandidateCount,
		BuyCount:          chosen.BuyCount,
		ActionCounts:      chosen.ActionCounts,
		ExclusionCounts:   map[string]int{},
	}
	for _, e := range chosen.Exclusions {
		st.ExclusionCounts[e.ReasonCode]++
	}
	if len(chosen.Exclusions) > 8 {
		st.SampleExclusions = append([]selection.Exclusion{}, chosen.Exclusions[:8]...)
	} else {
		st.SampleExclusions = append([]selection.Exclusion{}, chosen.Exclusions...)
	}
	// 候选上限 20，避免首屏响应过大。
	if len(chosen.Candidates) > 20 {
		st.Candidates = append([]selection.Candidate{}, chosen.Candidates[:20]...)
	} else {
		st.Candidates = append([]selection.Candidate{}, chosen.Candidates...)
	}
	return st, nil
}

func (s *Service) positionStatus(ctx context.Context) (PositionStatus, error) {
	st := PositionStatus{}
	if s.deps.Holdings != nil {
		holdings, err := s.deps.Holdings.GetAllPositions()
		if err != nil {
			return st, fmt.Errorf("read holdings: %w", err)
		}
		st.HoldingCount = len(holdings)
	}
	if s.deps.Positions != nil {
		runs, err := s.deps.Positions.List(ctx, "", 1)
		if err != nil {
			return st, fmt.Errorf("list position runs: %w", err)
		}
		if len(runs) > 0 && runs[0] != nil {
			st.HasDecisionRun = true
			st.LatestRunDate = runs[0].SnapshotDate
			for _, d := range runs[0].Decisions {
				if d.Action == "exit" || d.Action == "reduce" {
					st.UrgentActions++
				}
			}
		}
	}
	return st, nil
}

func buildWorkLog(sel *SelectionStatus, methods MethodStatus) WorkLog {
	if sel == nil {
		return WorkLog{Available: false}
	}
	insufficient := sel.ExclusionCounts["insufficient_data"] +
		sel.ExclusionCounts["historical_features_unavailable"] +
		sel.ExclusionCounts["market_state_unavailable"]
	methodGated := sel.ExclusionCounts["method_not_eligible"] +
		sel.ExclusionCounts["method_degraded"] +
		sel.ExclusionCounts["method_not_executable"] +
		sel.ExclusionCounts["evidence_insufficient"] +
		sel.ExclusionCounts["universe_mismatch"]
	invalidated := sel.ExclusionCounts["invalidation_matched"]
	failed := sel.ExclusionCounts["execution_failed"]

	lines := []WorkLogLine{
		{Key: "scanned", Label: "扫描股票", Value: sel.ScannedStocks, Detail: "冻结快照中纳入扫描的股票数", Tone: "neutral"},
		{Key: "eligible_methods", Label: "通过资格的方法", Value: sel.EligibleMethods,
			Detail: fmt.Sprintf("方法库共 %d 个，其中 %d 个通过资格检查", methods.Total, sel.EligibleMethods), Tone: "neutral"},
		{Key: "candidates", Label: "命中候选", Value: sel.CandidateCount, Detail: "方法证据门槛全部通过", Tone: "positive"},
		{Key: "insufficient", Label: "因数据不足排除", Value: insufficient, Detail: "缺少特征或市场状态，无法判定", Tone: "warning"},
		{Key: "invalidated", Label: "因失效条件排除", Value: invalidated, Detail: "方法自带的失效条件被当前事实命中", Tone: "warning"},
		{Key: "method_gated", Label: "因方法资格排除", Value: methodGated, Detail: "方法未通过验证或与股票池不匹配", Tone: "warning"},
	}
	if failed > 0 {
		lines = append(lines, WorkLogLine{Key: "failed", Label: "规则执行失败", Value: failed, Detail: "规则求值异常", Tone: "warning"})
	}
	return WorkLog{Available: true, SnapshotDate: sel.SnapshotDate, Lines: lines}
}

func decideEmptyState(data DataStatus, methods MethodStatus, sel *SelectionStatus) EmptyState {
	hasCandidates := sel != nil && sel.CandidateCount > 0
	if hasCandidates {
		return EmptyState{
			Reason:  ReasonHasCandidates,
			Title:   fmt.Sprintf("今日候选 %d 个", sel.CandidateCount),
			Message: fmt.Sprintf("基于 %s 的冻结数据，确定性引擎筛出 %d 个候选。", sel.SnapshotDate, sel.CandidateCount),
		}
	}
	// 1. 数据还没同步
	if !data.Fresh {
		return EmptyState{
			Reason:  ReasonDataNotSynced,
			Title:   "行情数据尚未就绪",
			Message: data.Detail,
			Actions: []Action{
				{Kind: ActionSync, Label: "现在同步行情", Primary: true},
				{Kind: ActionResearch, Label: "先了解方法机制", To: "/methods"},
			},
		}
	}
	// 2. 还没有通过验证的方法
	if methods.Verified == 0 && methods.Observing == 0 {
		msg := "方法库里还没有通过真实回测验证的方法，因此不会产生任何候选。"
		if methods.Total > 0 {
			msg = fmt.Sprintf("方法库里有 %d 个方法，但没有一个通过验证门槛（其中 %d 个被拒绝）。", methods.Total, methods.Rejected)
		}
		return EmptyState{
			Reason:  ReasonNoVerifiedMethods,
			Title:   "还没有通过验证的方法",
			Message: msg,
			Actions: []Action{
				{Kind: ActionSeedMethods, Label: "载入内置示例方法", Primary: true},
				{Kind: ActionResearch, Label: "开始一次方法研究", To: "/agent"},
			},
		}
	}
	// 3. 有方法但还没跑过选股
	if sel == nil {
		return EmptyState{
			Reason:  ReasonSelectionNotRun,
			Title:   "数据与方法都已就绪，还没跑过选股",
			Message: "行情快照已冻结、方法库已验证。运行一次选股即可得到今日候选。",
			Actions: []Action{
				{Kind: ActionRunSelection, Label: "运行今日选股", Primary: true},
			},
		}
	}
	// 4. 正常的不推荐
	return EmptyState{
		Reason: ReasonNoCandidates,
		Title:  "已扫描，但没有方法通过证据门槛",
		Message: fmt.Sprintf("本次扫描 %d 只股票、%d 个通过资格的方法，没有一只同时满足证据门槛。这是正常结果，不是故障。",
			sel.ScannedStocks, sel.EligibleMethods),
		Actions: []Action{
			{Kind: ActionViewExclusions, Label: "查看排除原因", Primary: true},
			{Kind: ActionSeedMethods, Label: "载入内置示例方法"},
		},
	}
}

func buildHealth(data DataStatus, methods MethodStatus, sel *SelectionStatus, pos PositionStatus) Health {
	signals := []Signal{}

	dataStatus := "ok"
	dataValue := data.LatestSnapshotDate
	switch {
	case data.LatestSnapshotID == "":
		dataStatus = "blocked"
		dataValue = "无快照"
	case !data.Fresh:
		dataStatus = "attention"
	}
	if dataValue == "" {
		dataValue = "无"
	}
	signals = append(signals, Signal{Key: "data", Label: "数据新鲜度", Value: dataValue, Status: dataStatus, Detail: data.Detail})

	methodStatus := "ok"
	switch {
	case methods.Verified == 0 && methods.Observing == 0:
		methodStatus = "blocked"
	case methods.Verified == 0:
		methodStatus = "attention"
	}
	signals = append(signals, Signal{Key: "methods", Label: "已验证方法", Value: fmt.Sprintf("%d", methods.Verified), Status: methodStatus,
		Detail: fmt.Sprintf("共 %d 个方法：验证 %d / 观察 %d / 候选 %d / 拒绝 %d", methods.Total, methods.Verified, methods.Observing, methods.Candidate, methods.Rejected)})

	candStatus := "attention"
	candValue := 0
	candDetail := "尚无选股运行"
	if sel != nil {
		candValue = sel.CandidateCount
		candDetail = fmt.Sprintf("运行于 %s，扫描 %d 只", sel.SnapshotDate, sel.ScannedStocks)
		if sel.CandidateCount > 0 {
			candStatus = "ok"
		}
	}
	signals = append(signals, Signal{Key: "candidates", Label: "今日候选", Value: fmt.Sprintf("%d", candValue), Status: candStatus, Detail: candDetail})

	posStatus := "ok"
	posDetail := "无持仓"
	if pos.HoldingCount > 0 {
		posDetail = fmt.Sprintf("持仓 %d 只", pos.HoldingCount)
	}
	if pos.UrgentActions > 0 {
		posStatus = "attention"
		posDetail += fmt.Sprintf("，%d 个需处理", pos.UrgentActions)
	}
	signals = append(signals, Signal{Key: "positions", Label: "持仓", Value: fmt.Sprintf("%d", pos.HoldingCount), Status: posStatus, Detail: posDetail})

	overall := "ok"
	for _, s := range signals {
		if s.Status == "blocked" {
			overall = "blocked"
			break
		}
		if s.Status == "attention" {
			overall = "attention"
		}
	}
	return Health{Overall: overall, Signals: signals}
}
