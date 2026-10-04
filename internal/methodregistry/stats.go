package methodregistry

import (
	"context"
	"sort"
	"strings"
)

// RejectStat 是一条「未通过证据门槛」原因的分布统计。
type RejectStat struct {
	// Category 是稳定的原因类别（机器可读，用于反哺研究）。
	Category string `json:"category"`
	// Reason 是审计事件里的原始原因（人类可读，含机器可读原因码后缀）。
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// ReasonCategory 把审计原因归一为稳定的统计类别。
// 原因字符串由 policy.Initial 写入，后缀携带 validation.ExplainConfidence 的
// 机器可读原因码；这里按语义归类，未知原因统一落到 other，绝不编造。
func ReasonCategory(reason string) string {
	switch {
	case strings.Contains(reason, "hard_blocker"):
		return "critic_hard_blocker"
	case strings.Contains(reason, "insufficient_oos_trades"), strings.Contains(reason, "no_trades"):
		return "insufficient_trades"
	case strings.Contains(reason, "multiple_testing"):
		return "multiple_testing"
	case strings.Contains(reason, "oos_sharpe_below_threshold"), strings.Contains(reason, "oos_max_drawdown_above_threshold"):
		return "weak_oos_performance"
	case strings.Contains(reason, "single-stock evidence cannot verify"):
		return "single_stock_scope"
	case strings.Contains(reason, "not executable"):
		return "not_executable"
	case strings.Contains(reason, "integrity mismatch"):
		return "evidence_integrity"
	case strings.Contains(reason, "lacks immutable snapshot"):
		return "evidence_missing"
	case strings.Contains(reason, "does not match single-stock method scope"):
		return "scope_mismatch"
	case strings.Contains(reason, "awaiting real validation evidence"):
		return "awaiting_evidence"
	case strings.Contains(reason, "user feedback"):
		return "user_feedback"
	default:
		return "other"
	}
}

// RejectStats 聚合当前所有 rejected 方法最新一次「注册被拒」的原因分布。
// 数据来源是既有审计轨迹（AuditEvent.Reason），不引入新的持久化状态。
func (r *Registry) RejectStats(ctx context.Context) ([]RejectStat, error) {
	methods, err := r.repo.Query(ctx, Query{Status: []Status{StatusRejected}, Limit: 5000})
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	reasons := map[string]string{}
	total := 0
	for _, m := range methods {
		events, err := r.repo.ListAudit(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		reason := latestRejectReason(events)
		if reason == "" {
			reason = "rejected without recorded reason"
		}
		counts[reason]++
		reasons[reason] = ReasonCategory(reason)
		total++
	}
	out := make([]RejectStat, 0, len(counts))
	for reason, count := range counts {
		out = append(out, RejectStat{Category: reasons[reason], Reason: reason, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Reason < out[j].Reason
	})
	return out, nil
}

// latestRejectReason 返回一个方法最新一条进入 rejected 的审计原因。
func latestRejectReason(events []AuditEvent) string {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].To == StatusRejected && strings.TrimSpace(events[i].Reason) != "" {
			return strings.TrimSpace(events[i].Reason)
		}
	}
	return ""
}
