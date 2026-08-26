package selection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
)

type Engine struct {
	snapshots SnapshotRepository
	methods   MethodRepository
	runs      Repository
	now       func() time.Time
}

func NewEngine(s SnapshotRepository, m MethodRepository, r Repository) (*Engine, error) {
	if s == nil || m == nil || r == nil {
		return nil, fmt.Errorf("selection snapshots, methods and run repositories are required")
	}
	return &Engine{snapshots: s, methods: m, runs: r, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (e *Engine) Run(ctx context.Context, req Request) (*Run, error) {
	if strings.TrimSpace(req.MarketSnapshotID) == "" {
		return nil, fmt.Errorf("market_snapshot_id is required")
	}
	market, err := e.snapshots.LoadMarketSnapshot(req.MarketSnapshotID, true)
	if err != nil {
		return nil, err
	}
	if !market.IsReady() {
		return nil, fmt.Errorf("market snapshot %s is not frozen and ready", market.ID)
	}
	feature, err := e.pickFeature(req.FeatureSnapshotID, market.ID)
	if err != nil {
		return nil, err
	}
	if feature.MarketSnapshotID != market.ID || feature.Status != marketsnapshot.StatusReady || !feature.LeakChecked || feature.SnapshotDate != market.SnapshotDate {
		return nil, fmt.Errorf("feature snapshot %s is not ready, leak-checked and bound to market snapshot %s", feature.ID, market.ID)
	}
	computed, err := marketsnapshot.ComputeFeatureContentHash(feature)
	if err != nil || computed != feature.ContentHash {
		return nil, fmt.Errorf("feature snapshot %s content hash mismatch", feature.ID)
	}

	allMethods, err := e.methods.Query(ctx, methodregistry.Query{Limit: 10000})
	if err != nil {
		return nil, err
	}
	sort.Slice(allMethods, func(i, j int) bool { return allMethods[i].ID < allMethods[j].ID })
	runHash := computeRunHash(market, feature, allMethods)
	if previous, getErr := e.runs.Get(ctx, "hash:"+runHash, ""); getErr == nil {
		return previous, nil
	}

	run := &Run{ID: "selection-" + runHash[:16], RunHash: runHash, EngineVersion: EngineVersion, SnapshotID: market.ID, FeatureSnapshotID: feature.ID, SnapshotDate: market.SnapshotDate, Status: "completed", ScannedStocks: len(feature.Values), ActionCounts: map[string]int{ActionBuy: 0, ActionWatch: 0, ActionAvoid: 0, ActionInsufficientData: 0}, Candidates: []Candidate{}, Exclusions: []Exclusion{}, CreatedAt: e.now()}
	eligible := make([]*methodregistry.Method, 0)
	for _, m := range allMethods {
		reason := eligibilityReason(m, market)
		if reason != "" {
			run.Exclusions = append(run.Exclusions, Exclusion{MethodID: m.ID, ReasonCode: reason, Detail: eligibilityDetail(m, reason)})
			continue
		}
		eligible = append(eligible, m)
	}
	run.EligibleMethods = len(eligible)

	codes := selectedCodes(market, feature)
	byCode := make(map[string][]Trigger)
	methodByID := make(map[string]*methodregistry.Method)
	for _, m := range eligible {
		methodByID[m.ID] = m
		v := currentVersion(m)
		if v == nil || v.Method == nil {
			continue
		}
		if hasTemporal(v.Method.EntryRule) {
			run.Exclusions = append(run.Exclusions, Exclusion{MethodID: m.ID, ReasonCode: "historical_features_unavailable", Detail: "cross/in_window requires immutable historical feature snapshots"})
			continue
		}
		for _, code := range codes {
			values := feature.Values[code]
			missing := missingFeatures(v.Method.Scope.FeatureDeps, values)
			if len(missing) > 0 {
				run.Exclusions = append(run.Exclusions, Exclusion{MethodID: m.ID, Code: code, ReasonCode: "insufficient_data", Detail: "missing features: " + strings.Join(missing, ",")})
				continue
			}
			compiled := withResolvedRanks(v.Method, code, feature.Values)
			bar := barFromFeatures(code, market.SnapshotDate, values)
			result, execErr := compiled.Entry(bar, []methods.Bar{bar})
			if execErr != nil {
				run.Exclusions = append(run.Exclusions, Exclusion{MethodID: m.ID, Code: code, ReasonCode: "execution_failed", Detail: execErr.Error()})
				continue
			}
			if !result.Matched {
				continue
			}
			if compiled.InvalidRule != nil {
				invalid := *compiled
				invalid.EntryRule = compiled.InvalidRule
				if ir, _ := invalid.Entry(bar, []methods.Bar{bar}); ir != nil && ir.Matched {
					run.Exclusions = append(run.Exclusions, Exclusion{MethodID: m.ID, Code: code, ReasonCode: "invalidation_matched", Detail: "method invalidation rule matched current facts"})
					continue
				}
			}
			facts := make([]TriggerFact, 0, len(result.Trace))
			for _, fact := range result.Trace {
				facts = append(facts, TriggerFact{Path: fact.Path, Rule: fact.Expr, Passed: fact.Passed, Detail: fact.Detail})
			}
			byCode[code] = append(byCode[code], Trigger{MethodID: m.ID, MethodVersionID: v.ID, MethodName: m.Name, FamilyID: m.FamilyID, Evidence: v.Evidence, Facts: facts, Score: methodScore(m, values)})
		}
	}

	for code, triggers := range byCode {
		sort.Slice(triggers, func(i, j int) bool {
			if triggers[i].Score == triggers[j].Score {
				return triggers[i].MethodID < triggers[j].MethodID
			}
			return triggers[i].Score > triggers[j].Score
		})
		triggers = dedupeFamilies(triggers)
		primary := methodByID[triggers[0].MethodID]
		v := currentVersion(primary)
		exit := exitPlan(v.Method)
		score := aggregateScore(triggers)
		action := ActionWatch
		risks := []string{"方法来自历史统计证据，不保证未来收益"}
		if score >= 0.65 && exit.Complete {
			action = ActionBuy
		}
		if !exit.Complete {
			risks = append(risks, "退出计划不完整，仅可观察")
		}
		candidate := Candidate{Code: code, Action: action, Score: score, DataDate: market.SnapshotDate, SnapshotID: market.ID, FeatureSnapshotID: feature.ID, Triggers: triggers, PositionCapPct: positionCap(v.Method), BuyWindow: buyWindow(primary), Exit: exit, Invalidations: append([]string{}, primary.Invalidations...), Risks: risks}
		candidate.Explanation = explain(candidate)
		run.Candidates = append(run.Candidates, candidate)
	}
	sort.Slice(run.Candidates, func(i, j int) bool {
		if run.Candidates[i].Score == run.Candidates[j].Score {
			return run.Candidates[i].Code < run.Candidates[j].Code
		}
		return run.Candidates[i].Score > run.Candidates[j].Score
	})
	for i := range run.Candidates {
		run.Candidates[i].Rank = i + 1
		run.ActionCounts[run.Candidates[i].Action]++
		if run.Candidates[i].Action == ActionBuy {
			run.BuyCount++
		}
	}
	run.CandidateCount = len(run.Candidates)
	for _, exclusion := range run.Exclusions {
		if exclusion.ReasonCode == "insufficient_data" || exclusion.ReasonCode == "historical_features_unavailable" || exclusion.ReasonCode == "market_state_unavailable" {
			run.ActionCounts[ActionInsufficientData]++
		}
		if exclusion.ReasonCode == "invalidation_matched" {
			run.ActionCounts[ActionAvoid]++
		}
	}
	sort.SliceStable(run.Exclusions, func(i, j int) bool {
		a, b := run.Exclusions[i], run.Exclusions[j]
		if a.MethodID != b.MethodID {
			return a.MethodID < b.MethodID
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.ReasonCode < b.ReasonCode
	})
	if err := e.runs.Save(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

func (e *Engine) pickFeature(id, marketID string) (*marketsnapshot.FeatureSnapshot, error) {
	if id != "" {
		return e.snapshots.LoadFeatureSnapshot(id, true)
	}
	items, err := e.snapshots.ListFeatureSnapshots(marketID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Status == marketsnapshot.StatusReady && item.LeakChecked {
			return e.snapshots.LoadFeatureSnapshot(item.ID, true)
		}
	}
	return nil, fmt.Errorf("no ready leak-checked feature snapshot for %s", marketID)
}

func eligibilityReason(m *methodregistry.Method, market *marketsnapshot.MarketSnapshot) string {
	if m.Status != methodregistry.StatusVerified && m.Status != methodregistry.StatusObserving {
		return "method_not_eligible"
	}
	if m.Status == methodregistry.StatusDegraded {
		return "method_degraded"
	}
	v := currentVersion(m)
	if v == nil || v.Method == nil || !v.Method.IsExecutable() {
		return "method_not_executable"
	}
	if v.Evidence == nil || !v.Evidence.Passable || (v.Evidence.Confidence != "moderate" && v.Evidence.Confidence != "strong") {
		return "evidence_insufficient"
	}
	if m.Universe != "" && m.Universe != market.Universe.Name {
		return "universe_mismatch"
	}
	if len(v.Method.Scope.MarketState) > 0 {
		return "market_state_unavailable"
	}
	return ""
}
func eligibilityDetail(m *methodregistry.Method, reason string) string {
	return fmt.Sprintf("method %s status=%s excluded: %s", m.ID, m.Status, reason)
}
func currentVersion(m *methodregistry.Method) *methodregistry.MethodVersion {
	if m == nil || len(m.Versions) == 0 {
		return nil
	}
	for i := range m.Versions {
		if m.Versions[i].Version == m.CurrentVersion {
			return &m.Versions[i]
		}
	}
	return &m.Versions[len(m.Versions)-1]
}
func selectedCodes(m *marketsnapshot.MarketSnapshot, f *marketsnapshot.FeatureSnapshot) []string {
	allowed := map[string]bool{}
	for _, x := range m.UniverseMembers {
		if x.Selected {
			allowed[x.Code] = true
		}
	}
	out := []string{}
	for code := range f.Values {
		if len(allowed) == 0 || allowed[code] {
			out = append(out, code)
		}
	}
	sort.Strings(out)
	return out
}
func missingFeatures(deps []string, v map[string]float64) []string {
	var out []string
	for _, d := range deps {
		if _, ok := v[d]; !ok {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}
func barFromFeatures(_ string, date string, v map[string]float64) methods.Bar {
	return methods.Bar{Date: date, Open: v["open"], High: v["high"], Low: v["low"], Close: v["close"], Volume: v["volume"], Amount: v["amount"], Indicators: v}
}
func hasTemporal(x *methods.Expr) bool {
	if x == nil {
		return false
	}
	if x.Type == methods.NodeCross || x.Type == methods.NodeInWindow {
		return true
	}
	if hasTemporal(x.Left) || hasTemporal(x.Right) {
		return true
	}
	for _, c := range x.Children {
		if hasTemporal(c) {
			return true
		}
	}
	return false
}
func withResolvedRanks(m *methods.CompiledMethod, code string, all map[string]map[string]float64) *methods.CompiledMethod {
	raw, _ := json.Marshal(m)
	var out methods.CompiledMethod
	_ = json.Unmarshal(raw, &out)
	out.EntryRule = resolveSnapshotValues(out.EntryRule, code, all)
	out.InvalidRule = resolveSnapshotValues(out.InvalidRule, code, all)
	return &out
}
func resolveSnapshotValues(x *methods.Expr, code string, all map[string]map[string]float64) *methods.Expr {
	if x == nil {
		return nil
	}
	if x.Type == methods.NodeIndicator {
		if value, ok := all[code][x.Indicator]; ok {
			return &methods.Expr{Type: methods.NodeConstant, Value: &value}
		}
		return x
	}
	if x.Type == methods.NodeRank {
		value, ok := all[code][x.RankBy]
		vals := []float64{}
		for _, m := range all {
			if v, yes := m[x.RankBy]; yes {
				vals = append(vals, v)
			}
		}
		sort.Float64s(vals)
		passed := false
		if ok && len(vals) > 0 {
			idx := sort.SearchFloat64s(vals, value)
			p := float64(idx+1) / float64(len(vals))
			if x.RankSide == "top" {
				passed = p >= 1-x.RankPct
			} else {
				passed = p <= x.RankPct
			}
		}
		one, zero := 1.0, 0.0
		left := &methods.Expr{Type: methods.NodeConstant, Value: &zero}
		if passed {
			left.Value = &one
		}
		return &methods.Expr{Type: methods.NodeCompare, Left: left, Right: &methods.Expr{Type: methods.NodeConstant, Value: &one}, Op: methods.CmpEQ}
	}
	x.Left = resolveSnapshotValues(x.Left, code, all)
	x.Right = resolveSnapshotValues(x.Right, code, all)
	for i := range x.Children {
		x.Children[i] = resolveSnapshotValues(x.Children[i], code, all)
	}
	return x
}
func methodScore(m *methodregistry.Method, v map[string]float64) float64 {
	ev := currentVersion(m).Evidence
	base := 0.55
	if ev.Confidence == "strong" {
		base = .72
	}
	if ev.OOSTrades >= 100 {
		base += .04
	}
	if ev.OOSMaxDrawdown < -.2 {
		base -= .08
	}
	if amount, ok := v["amount"]; !ok || amount <= 0 {
		base -= .08
	}
	if m.Health != nil {
		base = .75*base + .25*clamp(m.Health.Score, 0, 1)
	}
	return round(clamp(base, 0, 1))
}
func dedupeFamilies(in []Trigger) []Trigger {
	seen := map[string]bool{}
	out := []Trigger{}
	for _, t := range in {
		if seen[t.FamilyID] {
			continue
		}
		seen[t.FamilyID] = true
		out = append(out, t)
	}
	return out
}
func aggregateScore(t []Trigger) float64 {
	if len(t) == 0 {
		return 0
	}
	score := t[0].Score
	for i := 1; i < len(t); i++ {
		score += math.Min(.03, t[i].Score*.04)
	}
	return round(clamp(score, 0, .9))
}
func exitPlan(m *methods.CompiledMethod) ExitPlan {
	x := ExitPlan{MaxHoldingDays: m.Holding.MaxDays, StopLossPct: m.Holding.StopLoss, TakeProfitPct: m.Holding.TakeProfit, HasRule: m.ExitRule != nil}
	x.Complete = x.StopLossPct != nil && x.MaxHoldingDays > 0 && (x.TakeProfitPct != nil || x.HasRule)
	return x
}
func positionCap(m *methods.CompiledMethod) float64 {
	if m.Position.PctEquity != nil {
		return round(math.Min(*m.Position.PctEquity, .1))
	}
	return .05
}
func buyWindow(m *methodregistry.Method) string {
	if strings.Contains(strings.ToLower(m.TriggerFrequency), "overnight") {
		return "下一交易日开盘，若失效条件未触发"
	}
	return "下一交易日集合竞价后至开盘30分钟，禁止追涨"
}
func explain(c Candidate) string {
	return fmt.Sprintf("%s 在 %s 的冻结数据上触发 %d 个独立方法族；确定性评分 %.2f，结论为 %s。", c.Code, c.DataDate, len(c.Triggers), c.Score, c.Action)
}
func computeRunHash(m *marketsnapshot.MarketSnapshot, f *marketsnapshot.FeatureSnapshot, items []*methodregistry.Method) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s", EngineVersion, m.ID, m.ContentHash, f.ID, f.ContentHash)
	for _, x := range items {
		v := currentVersion(x)
		if v != nil {
			fmt.Fprintf(h, "\x00%s:%s:%s", x.ID, x.Status, v.ID)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func round(v float64) float64 { return math.Round(v*10000) / 10000 }
