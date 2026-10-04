// Package methodhealth 把前向账本（ledger.SignalLedger）里的真实 paper-trade
// 信号聚合成「已选方法前向表现」，并把健康状态交回 methodregistry 的策略状态机
// （Policy.Health）触发 observing/degraded/retired 转移。
//
// 所有指标都来自账本事实；没有执行样本的指标如实返回 nil，绝不编默认值。
package methodhealth

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
)

// LedgerReader 是前向账本的只读视图。
type LedgerReader interface {
	ListByParadigm(versionID string) []ledger.SignalEntry
}

// ForwardHealth 是一个方法的前向表现快照。
type ForwardHealth struct {
	MethodID           string    `json:"method_id"`
	Name               string    `json:"name"`
	Status             string    `json:"status"`
	Score              float64   `json:"score"`
	ForwardSamples     int       `json:"forward_samples"`
	ExecutedCount      int       `json:"executed_count"`
	RejectedCount      int       `json:"rejected_count"`
	HitRate            *float64  `json:"hit_rate,omitempty"`
	AvgReturn          *float64  `json:"avg_return,omitempty"`
	ExecutionDeviation bool      `json:"execution_deviation"`
	Decay              bool      `json:"decay"`
	Drift              bool      `json:"drift"`
	ConsecutiveSevere  int       `json:"consecutive_severe"`
	Degraded           bool      `json:"degraded"`
	Retired            bool      `json:"retired"`
	AsOf               time.Time `json:"as_of"`
}

// Evaluator 聚合前向账本信号为方法级健康度。
// applyPolicy=true 时把 HealthState 写回方法库，由 Policy.Health 决定状态转移。
type Evaluator struct {
	registry    *methodregistry.Registry
	ledger      LedgerReader
	applyPolicy bool
	now         func() time.Time
}

// New 构造评估器。now 可为 nil（使用 UTC 真实时钟）。
func New(registry *methodregistry.Registry, l LedgerReader, applyPolicy bool, now func() time.Time) (*Evaluator, error) {
	if registry == nil {
		return nil, fmt.Errorf("method health evaluator requires a method registry")
	}
	if l == nil {
		return nil, fmt.Errorf("method health evaluator requires a forward ledger")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Evaluator{registry: registry, ledger: l, applyPolicy: applyPolicy, now: now}, nil
}

// Evaluate 计算所有非终态方法的前向健康度。
func (e *Evaluator) Evaluate(ctx context.Context) ([]ForwardHealth, error) {
	cards, err := e.registry.Cards(ctx, methodregistry.Query{
		Status: []methodregistry.Status{
			methodregistry.StatusVerified, methodregistry.StatusObserving,
			methodregistry.StatusDegraded,
		},
		Limit: 5000,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ForwardHealth, 0, len(cards))
	for _, card := range cards {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m, err := e.registry.Get(ctx, card.ID)
		if err != nil {
			continue
		}
		health := e.evaluateOne(ctx, m)
		out = append(out, health)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].MethodID < out[j].MethodID
	})
	return out, nil
}

func (e *Evaluator) evaluateOne(ctx context.Context, m *methodregistry.Method) ForwardHealth {
	item := ForwardHealth{
		MethodID: m.ID, Name: m.Name, Status: string(m.Status),
		Degraded: m.Status == methodregistry.StatusDegraded,
		Retired:  m.Status == methodregistry.StatusRetired,
		AsOf:     e.now(),
	}
	version := currentVersion(m)
	if version == nil {
		return item
	}
	signals := e.ledger.ListByParadigm(version.ID)
	state := computeHealthState(signals, e.now())
	item.Score = state.Score
	item.ForwardSamples = state.ForwardSamples
	item.ExecutionDeviation = state.ExecutionDeviation
	item.Decay = state.Decay
	item.Drift = state.Drift
	item.ConsecutiveSevere = state.ConsecutiveSevere
	for _, s := range signals {
		switch {
		case s.Execution == nil:
		case s.Execution.Status == "rejected":
			item.RejectedCount++
		case s.Execution.Status == "filled" || s.Execution.Status == "partial":
			item.ExecutedCount++
		}
	}
	if executed := executedReturns(signals); len(executed) > 0 {
		wins := 0
		sum := 0.0
		for _, r := range executed {
			if r > 0 {
				wins++
			}
			sum += r
		}
		hit := float64(wins) / float64(len(executed))
		avg := sum / float64(len(executed))
		item.HitRate = &hit
		item.AvgReturn = &avg
	}
	if e.applyPolicy && state.ForwardSamples > 0 {
		if m2, err := e.registry.ApplyHealth(ctx, m.ID, state); err == nil {
			item.Status = string(m2.Status)
			item.Degraded = m2.Status == methodregistry.StatusDegraded
			item.Retired = m2.Status == methodregistry.StatusRetired
		}
	}
	return item
}

// computeHealthState 从账本事实推导 HealthState。
//
// 口径（全部确定性、可复算）：
//   - Score：有跟踪 70 起步；执行样本 >=3 时按胜率/平均收益加减；执行偏差、衰减扣分；
//   - ExecutionDeviation：拒绝占比 >= 30%（样本 >= 5）；
//   - Decay：最近 5 笔执行全部非盈利且历史存在盈利；
//   - Drift：最近 10 笔执行均值为负而全程均值为正（分布漂移代理）；
//   - ConsecutiveSevere：执行尾部连续单笔收益 <= -5% 的次数。
func computeHealthState(signals []ledger.SignalEntry, now time.Time) methodregistry.HealthState {
	state := methodregistry.HealthState{AsOf: now, Score: 60}
	returns := executedReturns(signals)
	state.ForwardSamples = len(signals)
	if len(signals) > 0 {
		state.Score = 70
	}
	if len(returns) >= 3 {
		wins := 0
		sum := 0.0
		for _, r := range returns {
			if r > 0 {
				wins++
			}
			sum += r
		}
		hit := float64(wins) / float64(len(returns))
		avg := sum / float64(len(returns))
		state.Score += 20 * hit
		switch {
		case avg > 0:
			state.Score += 10
		case avg < 0:
			state.Score -= 15
		}
	}
	executed := executedEntries(signals)
	rejected := 0
	for _, s := range signals {
		if s.Execution != nil && s.Execution.Status == "rejected" {
			rejected++
		}
	}
	if total := len(executed) + rejected; total >= 5 && float64(rejected)/float64(total) >= 0.3 {
		state.ExecutionDeviation = true
		state.Score -= 25
	}
	if len(returns) >= 6 {
		recent := returns[len(returns)-5:]
		allNegative := true
		for _, r := range recent {
			if r > 0 {
				allNegative = false
				break
			}
		}
		earlierPositive := false
		for _, r := range returns[:len(returns)-5] {
			if r > 0 {
				earlierPositive = true
				break
			}
		}
		if allNegative && earlierPositive {
			state.Decay = true
			state.Score -= 20
		}
	}
	if len(returns) >= 10 {
		recentSum := 0.0
		totalSum := 0.0
		for _, r := range returns {
			totalSum += r
		}
		for _, r := range returns[len(returns)-10:] {
			recentSum += r
		}
		if recentSum < 0 && totalSum > 0 {
			state.Drift = true
			state.Score -= 15
		}
	}
	for i := len(returns) - 1; i >= 0 && returns[i] <= -0.05; i-- {
		state.ConsecutiveSevere++
	}
	state.Score = clampScore(state.Score)
	return state
}

// executedReturns 返回已成交信号的单笔收益率（净盈亏 / 成交金额）。
func executedReturns(signals []ledger.SignalEntry) []float64 {
	var out []float64
	for _, s := range executedEntries(signals) {
		notional := s.Execution.ExecPrice * float64(s.Execution.ExecQty)
		if notional > 0 {
			out = append(out, s.Execution.PnL/notional)
		}
	}
	return out
}

func executedEntries(signals []ledger.SignalEntry) []ledger.SignalEntry {
	var out []ledger.SignalEntry
	for _, s := range signals {
		if s.Execution == nil {
			continue
		}
		if s.Execution.Status == "filled" || s.Execution.Status == "partial" {
			out = append(out, s)
		}
	}
	return out
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

func clampScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
