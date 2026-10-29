package selection

import "sort"

// BuyCapPerMethod 是每个选股方法在单次 run 中最多贡献的 buy 候选数。
// 语义来自产品口径：一个方法一次选出「最多不超过 5 只」，多个方法各自
// 独立计额（不跨方法借名额，宁缺不补）。
const BuyCapPerMethod = 5

// applyPerMethodBuyCap 在 byCode 聚合与 aggregateScore 之后做**纯输出过滤**：
// 对每个方法的方法内候选集 S_m 按聚合 Candidate.Score 降序（并列按 Code 升序）
// 取前 BuyCapPerMethod 个作为该方法的名额归属，其余触发候选记为被该方法截断。
// 达到 buy 门槛、却没被任何方法保住的候选降级为 watch 并标记 TruncatedFromBuy。
//
// 硬约束：
//   - 不在聚合之前截断。聚合前只有 methodScore，同方法内除 amount 缺失的 -0.08
//     外全部并列，提前截断会退化成「按 Code 取前 5」，丢掉跨方法共振带来的真实
//     强度差异。
//   - Score 不参与截断也不被截断修改：它始终由全量 triggers 经 aggregateScore
//     算出（分数不变式）。
//   - 名额只认 dedupeFamilies 之后仍留在 triggers 里的方法：同族方法被挤出的
//     那个不算触发，不重复占额。
//   - 因子通道（FactorMethodID，watch-only）不占名额，也不参与名额归属。
//   - watch / insufficient_data / avoid 不占名额：它们不消耗别的候选的机会。
func applyPerMethodBuyCap(run *Run) {
	if run == nil {
		return
	}
	limit := run.BuyCapPerMethod
	if limit <= 0 {
		limit = BuyCapPerMethod
		run.BuyCapPerMethod = limit
	}

	// S_m：方法 ID → 该方法（去同族后）触发的候选下标。
	byMethod := make(map[string][]int, len(run.Candidates))
	for i := range run.Candidates {
		for _, trigger := range run.Candidates[i].Triggers {
			if trigger.MethodID == "" || trigger.MethodID == FactorMethodID {
				continue
			}
			byMethod[trigger.MethodID] = append(byMethod[trigger.MethodID], i)
		}
	}

	for methodID, indexes := range byMethod {
		// 排序键：聚合分降序，并列按 Code 升序（确定性，且绝不按 Code 直接截断）。
		sort.SliceStable(indexes, func(a, b int) bool {
			left, right := run.Candidates[indexes[a]], run.Candidates[indexes[b]]
			if left.Score == right.Score {
				return left.Code < right.Code
			}
			return left.Score > right.Score
		})
		for position, index := range indexes {
			if position < limit {
				run.Candidates[index].KeptByMethodIDs = append(run.Candidates[index].KeptByMethodIDs, methodID)
				continue
			}
			run.Candidates[index].TruncatedByMethodIDs = append(run.Candidates[index].TruncatedByMethodIDs, methodID)
		}
	}

	// byMethod 是 map，跨方法的追加顺序不稳定；逐个候选排序，保证同快照重跑
	// 产出逐位一致。
	for i := range run.Candidates {
		candidate := &run.Candidates[i]
		sort.Strings(candidate.KeptByMethodIDs)
		sort.Strings(candidate.TruncatedByMethodIDs)
		if candidate.Action != ActionBuy {
			continue
		}
		for _, methodID := range candidate.KeptByMethodIDs {
			run.BuyCountByMethod[methodID]++
		}
		if len(candidate.KeptByMethodIDs) > 0 {
			continue
		}
		// 所有触发它的方法都把它截在名额外：本已达 buy 门槛，降级为观察。
		candidate.Action = ActionWatch
		candidate.TruncatedFromBuy = true
		candidate.Risks = append(candidate.Risks, "方法内买入名额已满（单方法每次最多 5 只），本票降级为观察")
		candidate.Explanation = explain(*candidate)
		run.TruncatedFromBuy++
	}
}
