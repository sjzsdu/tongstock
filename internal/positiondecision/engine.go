package positiondecision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/pkg/trading"
)

type Engine struct {
	snapshots SnapshotRepository
	trades    TradeRepository
	methods   MethodRepository
	repo      Repository
	now       func() time.Time
}

func NewEngine(s SnapshotRepository, t TradeRepository, m MethodRepository, r Repository) (*Engine, error) {
	if s == nil || t == nil || m == nil || r == nil {
		return nil, fmt.Errorf("position decision dependencies are required")
	}
	return &Engine{snapshots: s, trades: t, methods: m, repo: r, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (e *Engine) Run(ctx context.Context, req Request) (*Run, error) {
	market, err := e.snapshots.LoadMarketSnapshot(req.MarketSnapshotID, true)
	if err != nil {
		return nil, err
	}
	if !market.IsReady() {
		return nil, fmt.Errorf("market snapshot is not frozen and ready")
	}
	feature, err := e.feature(req.FeatureSnapshotID, market.ID)
	if err != nil {
		return nil, err
	}
	if feature.Status != marketsnapshot.StatusReady || !feature.LeakChecked || feature.MarketSnapshotID != market.ID {
		return nil, fmt.Errorf("feature snapshot is not eligible")
	}
	hash, _ := marketsnapshot.ComputeFeatureContentHash(feature)
	if hash != feature.ContentHash {
		return nil, fmt.Errorf("feature snapshot hash mismatch")
	}
	positions, err := e.trades.GetAllPositions()
	if err != nil {
		return nil, err
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].Code < positions[j].Code })
	runHash := hashRun(market, feature, positions)
	if old, x := e.repo.Get(ctx, "hash:"+runHash); x == nil {
		return old, nil
	}
	r := &Run{ID: "position-" + runHash[:16], RunHash: runHash, EngineVersion: EngineVersion, SnapshotID: market.ID, FeatureSnapshotID: feature.ID, SnapshotDate: market.SnapshotDate, Decisions: []Decision{}, CreatedAt: e.now()}
	states := map[string]marketsnapshot.CodeStatus{}
	for _, s := range market.Codes {
		states[s.Code] = s
	}
	for _, p := range positions {
		d := Decision{Code: p.Code, Name: p.Name, Action: "watch", Priority: "medium", Deadline: "下一交易日复核", Inferred: true, Executable: true, Cost: p.Price, PriceTime: market.SnapshotDate, SnapshotID: market.ID, Facts: []Fact{}, CounterEvidence: []string{}}
		values, ok := feature.Values[p.Code]
		close, hasClose := values["close"]
		if !ok || !hasClose || close <= 0 {
			d.Action = "insufficient_data"
			d.Priority = "high"
			d.Executable = false
			d.Constraint = "冻结特征快照缺少有效收盘价"
			d.Facts = append(d.Facts, Fact{Kind: "data", Detail: d.Constraint})
			r.Decisions = append(r.Decisions, d)
			continue
		}
		d.CurrentPrice = close
		d.ReturnPct = (close - p.Price) / p.Price
		link, linkErr := e.repo.GetLink(ctx, p.ID)
		var method *methodregistry.Method
		if linkErr == nil {
			d.Inferred = false
			d.Quantity = link.Quantity
			d.SelectionRunID = link.SelectionRunID
			d.MethodID = link.MethodID
			d.MethodVersionID = link.MethodVersionID
			method, _ = e.methods.Get(ctx, link.MethodID)
		} else {
			d.CounterEvidence = append(d.CounterEvidence, "旧持仓没有原始方法血缘，判断仅使用成本与通用风控")
		}
		if state := states[p.Code]; state.SecurityStatus == "suspended" || state.SecurityStatus == "halted" {
			d.Executable = false
			d.Constraint = "当前停牌，无法卖出；恢复交易后优先执行"
		}
		if sameTradingDate(p.CreatedAt, market.SnapshotDate) {
			d.Executable = false
			d.Constraint = "A股 T+1：买入当日不可卖出，下一交易日执行"
		}
		if method != nil {
			applyMethod(&d, method, p, market.SnapshotDate, values)
		} else {
			applyGeneric(&d)
		}
		if !d.Executable && d.Action == "exit" {
			d.Deadline = d.Constraint
		}
		d.Explanation = fmt.Sprintf("%s 当前收益 %.2f%%，动作 %s；%s", d.Code, d.ReturnPct*100, d.Action, sourceText(d.Inferred))
		r.Decisions = append(r.Decisions, d)
	}
	if err := e.repo.Save(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}
func (e *Engine) feature(id, marketID string) (*marketsnapshot.FeatureSnapshot, error) {
	if id != "" {
		return e.snapshots.LoadFeatureSnapshot(id, true)
	}
	xs, err := e.snapshots.ListFeatureSnapshots(marketID)
	if err != nil {
		return nil, err
	}
	for _, x := range xs {
		if x.Status == marketsnapshot.StatusReady && x.LeakChecked {
			return e.snapshots.LoadFeatureSnapshot(x.ID, true)
		}
	}
	return nil, fmt.Errorf("no eligible feature snapshot")
}
func applyGeneric(d *Decision) {
	switch {
	case d.ReturnPct <= -.08:
		d.Action = "exit"
		d.Priority = "critical"
		d.Deadline = "最早可交易窗口"
		d.Facts = append(d.Facts, Fact{Kind: "generic_stop", Passed: true, Detail: fmt.Sprintf("无方法持仓回撤 %.2f%% <= -8%%", d.ReturnPct*100)})
	case d.ReturnPct >= .2:
		d.Action = "reduce"
		d.Priority = "high"
		d.Deadline = "下一可交易窗口"
		d.Facts = append(d.Facts, Fact{Kind: "generic_profit", Passed: true, Detail: fmt.Sprintf("无方法持仓收益 %.2f%% >= 20%%", d.ReturnPct*100)})
	default:
		d.Action = "watch"
		d.Facts = append(d.Facts, Fact{Kind: "inferred", Passed: true, Detail: "等待补充原始买入方法或明确风险规则"})
	}
}
func applyMethod(d *Decision, m *methodregistry.Method, p trading.Trade, date string, values map[string]float64) {
	v := version(m, d.MethodVersionID)
	if v == nil || v.Method == nil {
		d.Action = "insufficient_data"
		d.Executable = false
		d.Constraint = "关联的方法版本不存在"
		return
	}
	bar := methods.Bar{Date: date, Open: values["open"], High: values["high"], Low: values["low"], Close: values["close"], Volume: values["volume"], Amount: values["amount"], Indicators: values}
	res, _ := v.Method.Exit(bar, []methods.Bar{bar}, &methods.PositionState{EntryPrice: p.Price, EntryDate: p.CreatedAt.Format("2006-01-02")})
	if res != nil {
		for _, x := range res.Trace {
			d.Facts = append(d.Facts, Fact{Kind: x.Path, Passed: x.Passed, Detail: x.Detail})
		}
		if res.Matched {
			d.Action = "exit"
			d.Priority = "critical"
			d.Deadline = "最早可交易窗口"
		} else {
			d.Action = "hold"
			d.Priority = "low"
			d.Deadline = "下一交易日复核"
		}
	}
	if m.Status == methodregistry.StatusDegraded || m.Status == methodregistry.StatusRetired || m.Status == methodregistry.StatusRejected {
		d.Action = "exit"
		d.Priority = "critical"
		d.Deadline = "最早可交易窗口"
		d.Facts = append(d.Facts, Fact{Kind: "method_health", Passed: true, Detail: "方法状态=" + string(m.Status)})
	}
}
func version(m *methodregistry.Method, id string) *methodregistry.MethodVersion {
	for i := range m.Versions {
		if m.Versions[i].ID == id {
			return &m.Versions[i]
		}
	}
	return nil
}
func sameTradingDate(t time.Time, date string) bool { return t.Format("2006-01-02") == date }
func sourceText(inferred bool) string {
	if inferred {
		return "依据为推断，未伪造原始买入理由"
	}
	return "依据可追溯到原始方法版本"
}
func hashRun(m *marketsnapshot.MarketSnapshot, f *marketsnapshot.FeatureSnapshot, p []trading.Trade) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", EngineVersion, m.ContentHash, f.ContentHash, m.SnapshotDate)
	for _, x := range p {
		fmt.Fprintf(h, "\x00%d:%s:%.8f:%d", x.ID, x.Code, x.Price, x.CreatedAt.Unix())
	}
	return hex.EncodeToString(h.Sum(nil))
}
