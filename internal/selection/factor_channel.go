package selection

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sjzsdu/tongstock/internal/factorlab"
)

// FactorMethodID 是因子组合候选在 selection 产物里的方法标识。
// 它不是 methodregistry 里的方法：方法库的本体是「可编译形态规则 + OOS 交易
// 证据」，因子组合是「截面排序 + IC 证据」——硬注册要伪造 CompiledMethod 和
// Evidence 语义（踩证据诚实红线），所以作为引擎内建候选通道接入。
const FactorMethodID = "factor-combo"

// factorStaleLimit 与 factorlab 的 staleWarningDays 对齐：名单截面日过旧的
// 产出没有预测意义（见 factorlab 快照新鲜度），不进每日选股，只如实记 exclusion。
const factorStaleLimit = 14

// FactorPickSource 提供因子通道最近一次落库产出。生产装配为
// factorlab.SQLitePickStore（结构化满足本接口），测试可插桩。
type FactorPickSource interface {
	GetLatestPickRun(ctx context.Context) (*factorlab.PickRun, error)
}

// SetFactorPicks 注入因子通道产出源；nil = 不启用因子候选通道。
func (e *Engine) SetFactorPicks(src FactorPickSource) { e.factorPicks = src }

// factorChannelRequested 判断本次请求是否包含因子通道：显式指定方法 ID 时
// 只有点名 factor-combo 才启用；全量运行（空）默认包含。
func factorChannelRequested(requested []string) bool {
	if len(requested) == 0 {
		return true
	}
	for _, id := range requested {
		if id == FactorMethodID {
			return true
		}
	}
	return false
}

// resolveFactorPick 在计算 run hash 之前读取最近落库名单（名单身份参与幂等
// 哈希：名单更新必须产出新 run，不能用旧缓存的因子候选）。
func (e *Engine) resolveFactorPick(ctx context.Context, requested []string) (*factorlab.PickRun, error) {
	if e.factorPicks == nil || !factorChannelRequested(requested) {
		return nil, nil
	}
	return e.factorPicks.GetLatestPickRun(ctx)
}

// appendFactorCandidates 把因子通道名单转成 watch 级候选，追加进 run。
// 与形态方法严格区分的语义：
//   - 不是「某股票触发某形态」，而是「截面排序预测的头部名单」；
//   - 永远 watch：无个股退出计划，绝不生成买入指令（score 不参与 0.65 买入线）；
//   - 贡献分解随 trigger facts 透出，「为何选它」可复核；
//   - 名单过时（stale_days > 14）时如实记 exclusion，不静默丢弃也不硬塞。
func (e *Engine) appendFactorCandidates(run *Run, pick *factorlab.PickRun, pickErr error) {
	if pickErr != nil {
		run.Exclusions = append(run.Exclusions, Exclusion{MethodID: FactorMethodID, ReasonCode: "factor_pick_unavailable", Detail: "读取因子通道名单失败: " + pickErr.Error()})
		return
	}
	if pick == nil {
		return // 因子通道从未落库：无产出，不声称存在。
	}
	if pick.StaleDays > factorStaleLimit {
		run.Exclusions = append(run.Exclusions, Exclusion{
			MethodID:   FactorMethodID,
			ReasonCode: "factor_pick_stale",
			Detail:     fmt.Sprintf("因子名单截面日 %s 距今 %d 天（>%d），不进每日选股；请重跑因子研究并勾选名单入库", pick.AsOf, pick.StaleDays, factorStaleLimit),
		})
		return
	}
	for _, p := range pick.Picks {
		facts := []TriggerFact{
			{Path: "factor.composite_score", Passed: true, Detail: fmt.Sprintf("%.3f", p.Score)},
			{Path: "factor.run_id", Passed: true, Detail: pick.RunID},
			{Path: "factor.as_of", Passed: true, Detail: fmt.Sprintf("%s（距今 %d 天）", pick.AsOf, pick.StaleDays)},
		}
		facts = append(facts, contributionFacts(p.Contributions)...)
		run.Candidates = append(run.Candidates, Candidate{
			Code: p.Code, Action: ActionWatch, Score: p.Score,
			DataDate: run.SnapshotDate, SnapshotID: run.SnapshotID, FeatureSnapshotID: run.FeatureSnapshotID,
			Triggers: []Trigger{{MethodID: FactorMethodID, MethodName: "因子组合（截面排序）", Facts: facts, Score: p.Score}},
			Risks: []string{
				"因子组合来自截面排序统计证据，不保证未来收益",
				fmt.Sprintf("名单截面日 %s（距今 %d 天），与本次选股快照 %s 可能不同源", pick.AsOf, pick.StaleDays, run.SnapshotDate),
				"观察级：无个股退出计划，不生成买入指令",
			},
			Explanation: factorExplanation(p, pick),
		})
	}
}

// contributionFacts 把逐因子贡献转成 trigger facts，按 |贡献| 降序（最重要的在前）。
func contributionFacts(contrib map[string]float64) []TriggerFact {
	keys := make([]string, 0, len(contrib))
	for k := range contrib {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if abs64(contrib[keys[i]]) == abs64(contrib[keys[j]]) {
			return keys[i] < keys[j]
		}
		return abs64(contrib[keys[i]]) > abs64(contrib[keys[j]])
	})
	facts := make([]TriggerFact, 0, len(keys))
	for _, k := range keys {
		facts = append(facts, TriggerFact{Path: "factor.contrib." + k, Passed: true, Detail: fmt.Sprintf("%+.3f", contrib[k])})
	}
	return facts
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// factorExplanation 是因子候选的「为何选它」：组合分 + 截面日期 + 主要贡献。
func factorExplanation(p factorlab.TopPick, pick *factorlab.PickRun) string {
	top := contributionFacts(p.Contributions)
	if len(top) > 3 {
		top = top[:3]
	}
	parts := make([]string, 0, len(top))
	for _, f := range top {
		parts = append(parts, strings.TrimPrefix(f.Path, "factor.contrib.")+" "+f.Detail)
	}
	return fmt.Sprintf("%s 进入因子组合观察名单：组合分 %.3f（截面日 %s，距今 %d 天），主要贡献 %s。这是截面排序预测结果，不是形态触发；无个股退出计划，不生成买入指令。",
		p.Code, p.Score, pick.AsOf, pick.StaleDays, strings.Join(parts, "、"))
}
