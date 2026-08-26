package paradigms

import "time"

// 范式生命周期状态枚举。
//
// 状态:
//
//	pending    -> 候选, 尚未进入评审
//	reviewed   -> 已评审, 等待晋级决策
//	verified   -> 已验证 (样本外通过), 可进入观察
//	promoted   -> 已晋级, 面向产品决策
//	degraded   -> 降级 (收益衰减或市场状态不匹配)
//	suspended  -> 暂停 (临时失效, 可恢复)
//	rejected   -> 淘汰 (永久失效, 不再参与决策)
//
// 状态流转的校验逻辑 (ValidTransitions / ValidateTransition / ApplyTransition)
// 历史上由 Store.Transition 调用，但该路径无生产调用者，已在架构整顿中删除。
// Paradigm.Transitions 字段保留以承载历史审计记录的 JSON 序列化；新增流转
// 用例如需重新引入，可从 git 历史恢复校验逻辑。
const (
	StatePending   = "pending"
	StateReviewed  = "reviewed"
	StateVerified  = "verified"
	StatePromoted  = "promoted"
	StateDegraded  = "degraded"
	StateSuspended = "suspended"
	StateRejected  = "rejected"
)

// StateTransition 记录一次状态变更的审计信息。作为 Paradigm.Transitions
// 元素的序列化载体保留；当前无生产路径写入。
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
