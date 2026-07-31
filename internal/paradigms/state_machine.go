package paradigms

import (
	"fmt"
	"strings"
	"time"
)

// 范式生命周期状态机
//
// 状态:
//   pending    -> 候选, 尚未进入评审
//   reviewed   -> 已评审, 等待晋级决策
//   verified   -> 已验证 (样本外通过), 可进入观察
//   promoted   -> 已晋级, 面向产品决策
//   degraded   -> 降级 (收益衰减或市场状态不匹配)
//   suspended  -> 暂停 (临时失效, 可恢复)
//   rejected   -> 淘汰 (永久失效, 不再参与决策)

const (
	StatePending   = "pending"
	StateReviewed  = "reviewed"
	StateVerified  = "verified"
	StatePromoted  = "promoted"
	StateDegraded  = "degraded"
	StateSuspended = "suspended"
	StateRejected  = "rejected"
)

// StateTransition 记录一次状态变更的审计信息
type StateTransition struct {
	ID           string    `json:"id"`
	ParadigmID   string    `json:"paradigm_id"`
	From         string    `json:"from"`
	To           string    `json:"to"`
	Action       string    `json:"action"`          // promote / downgrade / suspend / resume / reject / verify / review
	Reason       string    `json:"reason"`          // 人类可读的变更原因
	Actor        string    `json:"actor,omitempty"` // 操作人或代理
	EvidenceHash string    `json:"evidence_hash,omitempty"`
	Auto         bool      `json:"auto"` // true 表示系统自动触发, false 为人工决定
	CreatedAt    time.Time `json:"created_at"`
}

// ValidTransitions 定义允许的状态迁移
// key: "from->to" ; value: 允许的 action 列表
var ValidTransitions = map[string][]string{
	// pending 流程
	StatePending + "->" + StateReviewed: {"review"},
	StatePending + "->" + StateRejected: {"reject"},
	// reviewed 流程
	StateReviewed + "->" + StateVerified: {"verify"},
	StateReviewed + "->" + StateRejected: {"reject"},
	StateReviewed + "->" + StatePending:  {"rollback"},
	// verified 流程
	StateVerified + "->" + StatePromoted:  {"promote"},
	StateVerified + "->" + StateSuspended: {"suspend"},
	StateVerified + "->" + StateDegraded:  {"downgrade"},
	StateVerified + "->" + StateRejected:  {"reject"},
	// promoted 流程
	StatePromoted + "->" + StateDegraded:  {"downgrade"},
	StatePromoted + "->" + StateSuspended: {"suspend"},
	StatePromoted + "->" + StateRejected:  {"reject"},
	// degraded 恢复/淘汰
	StateDegraded + "->" + StatePromoted:  {"promote"},
	StateDegraded + "->" + StateSuspended: {"suspend"},
	StateDegraded + "->" + StateRejected:  {"reject"},
	// suspended 恢复/淘汰
	StateSuspended + "->" + StateVerified: {"resume"},
	StateSuspended + "->" + StateDegraded: {"resume"},
	StateSuspended + "->" + StatePromoted: {"resume", "promote"},
	StateSuspended + "->" + StateRejected: {"reject"},
	// rejected 不可恢复 (保留历史)
}

// IsValidTransition 检查某个 from->to 转换是否允许, 且 action 是否匹配
func IsValidTransition(from, to, action string) bool {
	if strings.TrimSpace(from) == "" {
		from = StatePending
	}
	if from == to {
		return false
	}
	allowed, ok := ValidTransitions[from+"->"+to]
	if !ok {
		return false
	}
	if action == "" {
		return true // 只要有任何 action 允许
	}
	for _, a := range allowed {
		if a == action {
			return true
		}
	}
	return false
}

// ValidateTransition 完整校验并返回错误信息
func ValidateTransition(from, to, action string) error {
	if from == to {
		return fmt.Errorf("状态未发生变化: %s", from)
	}
	if !IsValidTransition(from, to, action) {
		return fmt.Errorf("非法状态转换: %s (%s) -> %s; 允许的转换请参见状态机定义", from, action, to)
	}
	return nil
}

// BuildTransitionRecord 构造状态变更审计记录
func BuildTransitionRecord(p *Paradigm, to, action, reason, actor, evidenceHash string, auto bool) StateTransition {
	return StateTransition{
		ID:           fmt.Sprintf("%s:%d", p.ID, time.Now().UnixNano()),
		ParadigmID:   p.ID,
		From:         p.ReviewStatus,
		To:           to,
		Action:       action,
		Reason:       reason,
		Actor:        actor,
		EvidenceHash: evidenceHash,
		Auto:         auto,
		CreatedAt:    time.Now(),
	}
}

// ApplyTransition 应用状态变更到范式 (仅内存, 需调用方保存)
func ApplyTransition(p *Paradigm, to, reason, actor string, evidenceHash string, auto bool) error {
	action := inferAction(p.ReviewStatus, to)
	if err := ValidateTransition(p.ReviewStatus, to, action); err != nil {
		return err
	}
	p.ReviewStatus = to
	if reason != "" {
		if p.ReviewNote == "" {
			p.ReviewNote = reason
		} else {
			p.ReviewNote = p.ReviewNote + "\n\n[" + to + "] " + reason
		}
	}
	p.UpdatedAt = time.Now()
	return nil
}

func inferAction(from, to string) string {
	switch to {
	case StateReviewed:
		return "review"
	case StateVerified:
		return "verify"
	case StatePromoted:
		return "promote"
	case StateDegraded:
		return "downgrade"
	case StateSuspended:
		return "suspend"
	case StateRejected:
		return "reject"
	case StatePending:
		return "rollback"
	}
	_ = from
	return ""
}

// CanShowOnDiscover 判定状态是否应该进入产品发现流 (决策入口)
func CanShowOnDiscover(status string) bool {
	return status == StateVerified || status == StatePromoted
}

// IsDecisionActive 判定状态是否为活跃的决策对象
func IsDecisionActive(status string) bool {
	return status == StatePromoted || status == StateVerified
}

// ListDecisionStatuses 返回产品决策侧认可的状态
func ListDecisionStatuses() []string {
	return []string{StateVerified, StatePromoted}
}
