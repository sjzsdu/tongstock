package paradigms

import (
	"fmt"
	"time"
)

// DecisionCard 是面向决策场景的范式摘要，由晋级范式派生。
// 目的: 让产品侧在发现页 / 自选 / 决策卡中只展示决策所需的信息，
// 而不是将研究侧的全部字段直接暴露给决策界面。
type DecisionCard struct {
	ParadigmID      string `json:"paradigm_id"`
	ParadigmName    string `json:"paradigm_name"`
	ParadigmVersion int    `json:"paradigm_version"`
	StockCode       string `json:"stock_code"`
	StockName       string `json:"stock_name"`
	Side            string `json:"side"` // buy / sell
	ReviewStatus    string `json:"review_status"`

	// 触发条件: 决策时关注的核心信号
	Triggers []string `json:"triggers"`
	// 失效条件: 何时该范式应被降级或淘汰
	Invalidations []string `json:"invalidations"`

	// 证据摘要
	EvidenceScore float64 `json:"evidence_score"`
	EvidenceHash  string  `json:"evidence_hash"`
	Reliability   string  `json:"reliability"` // high / medium / low

	// 决策元信息
	GeneratedAt time.Time `json:"generated_at"`
	// TTL 由范式的持有期推断: 超过该时间后需要重新评估
	TTL string `json:"ttl"`
	// 晋级或最近一次评审时间
	PromotedAt time.Time `json:"promoted_at"`

	// 可选: 当前是否可用于观察
	Active bool `json:"active"`
}

// BuildDecisionCard 将一个已晋级范式转换为决策卡
func BuildDecisionCard(p *Paradigm, version int, evidenceHash string) DecisionCard {
	card := DecisionCard{
		ParadigmID:      p.ID,
		ParadigmName:    p.Name,
		ParadigmVersion: version,
		StockCode:       p.StockCode,
		StockName:       p.StockName,
		Side:            p.Side,
		ReviewStatus:    p.ReviewStatus,
		Triggers:        condToStrings(p.BuyConds),
		Invalidations:   p.Invalid,
		EvidenceHash:    evidenceHash,
		Reliability:     p.Validation.ReliabilityLabel,
		GeneratedAt:     time.Now(),
		PromotedAt:      p.UpdatedAt,
		TTL:             inferTTL(p.Expectation.HoldingPeriod),
		Active:          CanShowOnDiscover(p.ReviewStatus),
	}
	if p.Evidence != nil {
		card.EvidenceScore = p.Evidence.Score
	}
	if len(card.Triggers) == 0 {
		card.Triggers = []string{p.Rationale}
	}
	return card
}

// condToStrings 将 Condition 列表转换为可读字符串
func condToStrings(conds []Condition) []string {
	out := make([]string, 0, len(conds))
	for _, c := range conds {
		out = append(out, fmt.Sprintf("%s %s %s", c.Indicator, c.Operator, c.Value))
	}
	return out
}

// inferTTL 根据持有期推断观察有效期 (仅作展示, 不影响真实治理)
func inferTTL(holdingPeriod string) string {
	switch holdingPeriod {
	case "1-5天", "短线", "1-5d":
		return "5 天"
	case "5-20天", "中线", "5-20d":
		return "20 天"
	case "20-60天", "60天", "长线":
		return "60 天"
	case "60天以上", "长期":
		return "90 天"
	default:
		if holdingPeriod == "" {
			return "30 天"
		}
		return holdingPeriod
	}
}

// FilterPromoted 只返回已晋级 (verified / promoted) 的范式
func FilterPromoted(list []*Paradigm) []*Paradigm {
	out := make([]*Paradigm, 0, len(list)/2)
	for _, p := range list {
		if p.ReviewStatus == StateVerified || p.ReviewStatus == StatePromoted {
			out = append(out, p)
		}
	}
	return out
}
