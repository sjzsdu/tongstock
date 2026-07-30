package paradigms

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// ============================================================================
// 研究血缘与版本追踪
//
// 目的: 让用户能回答 "某次收益变化来自数据、规则还是执行模型"
// 设计:
// 1. 任何修改都生成一个新的 ParadigmVersionRecord (旧版本不被覆盖)
// 2. 血缘链路: hypothesis → paradigm(v1) → evidence → review → paradigm(v2) → promote
// 3. 支持 diff 两个版本的规则、数据、参数差异
// ============================================================================

// ParadigmVersionRecord 范式版本快照 (不可变)
type ParadigmVersionRecord struct {
	ID            string    `json:"id"` // "{paradigm_id}#v{version}"
	ParadigmID    string    `json:"paradigm_id"`
	Version       int       `json:"version"`
	ParentVersion int       `json:"parent_version"`
	ChangeReason  string    `json:"change_reason"` // 人类可读的变更原因
	ChangeType    string    `json:"change_type"`   // "create" / "update" / "review" / "promote" / "rollback"
	ContentHash   string    `json:"content_hash"`  // 规则+参数+数据的哈希
	Author        string    `json:"author,omitempty"`
	Snapshot      *Paradigm `json:"snapshot"`      // 完整快照 (旧版本不可变)
	EvidenceHash  string    `json:"evidence_hash"` // 关联证据卡的哈希 (若有)
	CreatedAt     time.Time `json:"created_at"`
}

// LineageNode 血缘图中的节点
type LineageNode struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // "hypothesis" / "paradigm" / "evidence" / "review" / "promote" / "reject"
	Title     string    `json:"title"`
	Detail    string    `json:"detail,omitempty"`
	Version   int       `json:"version,omitempty"`
	Status    string    `json:"status"` // "open" / "in_progress" / "accepted" / "rejected" / "promoted"
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor,omitempty"`
	Payload   any       `json:"payload,omitempty"`
}

// LineageEdge 血缘图中的边
type LineageEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // "derived" / "evolved" / "validated" / "promoted"
}

// LineageGraph 完整研究血缘视图
type LineageGraph struct {
	ParadigmID   string                  `json:"paradigm_id"`
	ParadigmName string                  `json:"paradigm_name"`
	CurrentState string                  `json:"current_state"` // "draft" / "reviewed" / "promoted" / "rejected"
	Nodes        []LineageNode           `json:"nodes"`
	Edges        []LineageEdge           `json:"edges"`
	Versions     []ParadigmVersionRecord `json:"versions"`
	Summary      string                  `json:"summary"`
}

// ParadigmVersionDiff 两个版本的差异 (用于 Paradigm 的版本对比，不同于 Schema 的 VersionDiff)
type ParadigmVersionDiff struct {
	FromVersion int            `json:"from_version"`
	ToVersion   int            `json:"to_version"`
	ChangedRule bool           `json:"changed_rule"`
	ChangedData bool           `json:"changed_data"`
	ChangedMeta bool           `json:"changed_meta"`
	RuleDiff    RuleDiffDetail `json:"rule_diff"`
	DataDiff    DataDiffDetail `json:"data_diff"`
	MetaDiff    MetaDiffDetail `json:"meta_diff"`
	Summary     string         `json:"summary"`
}

// RuleDiffDetail 规则差异明细
type RuleDiffDetail struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Updated []string `json:"updated"`
}

// DataDiffDetail 数据差异明细
type DataDiffDetail struct {
	OldDataSource string `json:"old_data_source,omitempty"`
	NewDataSource string `json:"new_data_source,omitempty"`
	OldVersion    string `json:"old_version,omitempty"`
	NewVersion    string `json:"new_version,omitempty"`
	Changed       bool   `json:"changed"`
}

// MetaDiffDetail 元数据差异明细
type MetaDiffDetail struct {
	OldReviewStatus string `json:"old_review_status,omitempty"`
	NewReviewStatus string `json:"new_review_status,omitempty"`
	OldReliability  string `json:"old_reliability,omitempty"`
	NewReliability  string `json:"new_reliability,omitempty"`
	Changed         bool   `json:"changed"`
}

// ComputeParadigmHash 计算范式内容哈希 (规则 + 参数 + 数据)
func ComputeParadigmHash(p *Paradigm) string {
	if p == nil {
		return ""
	}
	// 仅对稳定字段计算哈希，忽略 updated_at 等瞬时字段
	stable := struct {
		BuyConds  []Condition    `json:"buy"`
		SellConds SellConditions `json:"sell"`
		Invalid   []string       `json:"invalid"`
		Confirm   []string       `json:"confirm"`
		Baseline  string         `json:"baseline"`
	}{
		BuyConds:  p.BuyConds,
		SellConds: p.SellConds,
		Invalid:   p.Invalid,
		Confirm:   p.Confirm,
		Baseline:  "",
	}
	data, _ := json.Marshal(stable)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}

// NewParadigmVersionRecord 创建一个版本记录
func NewParadigmVersionRecord(p *Paradigm, prevVersion int, changeType, changeReason, author string, evidenceHash string) *ParadigmVersionRecord {
	v := prevVersion + 1
	return &ParadigmVersionRecord{
		ID:            fmt.Sprintf("%s#v%d", p.ID, v),
		ParadigmID:    p.ID,
		Version:       v,
		ParentVersion: prevVersion,
		ChangeReason:  changeReason,
		ChangeType:    changeType,
		ContentHash:   ComputeParadigmHash(p),
		Author:        author,
		Snapshot:      p.DeepCopy(),
		EvidenceHash:  evidenceHash,
		CreatedAt:     time.Now(),
	}
}

// DeepCopy 深拷贝 Paradigm (用于版本快照)
func (p *Paradigm) DeepCopy() *Paradigm {
	if p == nil {
		return nil
	}
	cp := *p
	if p.BuyConds != nil {
		cp.BuyConds = append([]Condition(nil), p.BuyConds...)
	}
	if p.SellConds.TakeProfit != nil {
		cp.SellConds.TakeProfit = append([]Condition(nil), p.SellConds.TakeProfit...)
	}
	if p.SellConds.StopLoss != nil {
		cp.SellConds.StopLoss = append([]Condition(nil), p.SellConds.StopLoss...)
	}
	if p.Confirm != nil {
		cp.Confirm = append([]string(nil), p.Confirm...)
	}
	if p.Invalid != nil {
		cp.Invalid = append([]string(nil), p.Invalid...)
	}
	if p.Tags != nil {
		cp.Tags = append([]string(nil), p.Tags...)
	}
	if p.Transitions != nil {
		cp.Transitions = append([]StateTransition(nil), p.Transitions...)
	}
	return &cp
}

// BuildLineageGraph 从一个范式及其版本历史构建血缘图
func BuildLineageGraph(
	p *Paradigm,
	versions []ParadigmVersionRecord,
	evidenceHash string,
) *LineageGraph {
	graph := &LineageGraph{
		ParadigmID:   p.ID,
		ParadigmName: p.Name,
		CurrentState: p.ReviewStatus,
		Nodes:        []LineageNode{},
		Edges:        []LineageEdge{},
		Versions:     versions,
	}

	// 1. 根节点: 假设
	hypothesisNode := LineageNode{
		ID:        fmt.Sprintf("%s:hypothesis", p.ID),
		Type:      "hypothesis",
		Title:     fmt.Sprintf("假设: %s", p.Name),
		Detail:    p.Rationale,
		Status:    "accepted",
		Timestamp: p.CreatedAt,
		Actor:     p.Source.AgentVersion,
	}
	graph.Nodes = append(graph.Nodes, hypothesisNode)

	prevNodeID := hypothesisNode.ID

	// 2. 各版本节点
	for _, v := range versions {
		versionNode := LineageNode{
			ID:        v.ID,
			Type:      "paradigm",
			Title:     fmt.Sprintf("范式 v%d [%s]", v.Version, v.ChangeType),
			Detail:    v.ChangeReason,
			Version:   v.Version,
			Status:    "accepted",
			Timestamp: v.CreatedAt,
			Actor:     v.Author,
		}
		graph.Nodes = append(graph.Nodes, versionNode)
		graph.Edges = append(graph.Edges, LineageEdge{
			From: prevNodeID,
			To:   versionNode.ID,
			Type: "evolved",
		})
		prevNodeID = versionNode.ID
	}

	// 3. 证据节点
	if evidenceHash != "" {
		evidenceNode := LineageNode{
			ID:        fmt.Sprintf("%s:evidence", p.ID),
			Type:      "evidence",
			Title:     "证据卡",
			Detail:    fmt.Sprintf("hash=%s", evidenceHash),
			Status:    "accepted",
			Timestamp: time.Now(),
		}
		graph.Nodes = append(graph.Nodes, evidenceNode)
		graph.Edges = append(graph.Edges, LineageEdge{
			From: prevNodeID,
			To:   evidenceNode.ID,
			Type: "validated",
		})
		prevNodeID = evidenceNode.ID
	}

	// 4. 审查/晋级/淘汰节点
	switch p.ReviewStatus {
	case "reviewed":
		reviewNode := LineageNode{
			ID:        fmt.Sprintf("%s:review", p.ID),
			Type:      "review",
			Title:     "人工审查",
			Detail:    p.ReviewNote,
			Status:    "accepted",
			Timestamp: p.UpdatedAt,
		}
		graph.Nodes = append(graph.Nodes, reviewNode)
		graph.Edges = append(graph.Edges, LineageEdge{From: prevNodeID, To: reviewNode.ID, Type: "validated"})
	case "promoted":
		promoteNode := LineageNode{
			ID:        fmt.Sprintf("%s:promote", p.ID),
			Type:      "promote",
			Title:     "晋级到产品",
			Detail:    p.ReviewNote,
			Status:    "promoted",
			Timestamp: p.UpdatedAt,
		}
		graph.Nodes = append(graph.Nodes, promoteNode)
		graph.Edges = append(graph.Edges, LineageEdge{From: prevNodeID, To: promoteNode.ID, Type: "validated"})
	case "rejected":
		rejectNode := LineageNode{
			ID:        fmt.Sprintf("%s:reject", p.ID),
			Type:      "reject",
			Title:     "否决",
			Detail:    p.ReviewNote,
			Status:    "rejected",
			Timestamp: p.UpdatedAt,
		}
		graph.Nodes = append(graph.Nodes, rejectNode)
		graph.Edges = append(graph.Edges, LineageEdge{From: prevNodeID, To: rejectNode.ID, Type: "validated"})
	}

	// 5. 摘要
	graph.Summary = fmt.Sprintf(
		"该范式共演进 %d 个版本，当前状态: %s。%s",
		len(versions),
		p.ReviewStatus,
		p.ReviewNote,
	)

	return graph
}

// DiffParadigmVersions 比较两个版本的差异
func DiffParadigmVersions(from, to *Paradigm, fromVersion, toVersion int) *ParadigmVersionDiff {
	diff := &ParadigmVersionDiff{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
	}

	// 规则差异
	fromBuy := condKeys(from.BuyConds)
	toBuy := condKeys(to.BuyConds)
	fromSell := condKeys(flattenSellConds(from.SellConds))
	toSell := condKeys(flattenSellConds(to.SellConds))

	diff.RuleDiff.Added = diffKeys(toBuy, fromBuy)
	diff.RuleDiff.Removed = diffKeys(fromBuy, toBuy)
	diff.RuleDiff.Updated = append(diff.RuleDiff.Updated, diffKeys(toSell, fromSell)...)
	diff.RuleDiff.Updated = append(diff.RuleDiff.Updated, diffKeys(fromSell, toSell)...)
	diff.ChangedRule = len(diff.RuleDiff.Added)+len(diff.RuleDiff.Removed)+len(diff.RuleDiff.Updated) > 0

	// 数据差异
	diff.DataDiff.OldDataSource = from.Source.KlineType
	diff.DataDiff.NewDataSource = to.Source.KlineType
	diff.DataDiff.OldVersion = from.Source.DataWindow
	diff.DataDiff.NewVersion = to.Source.DataWindow
	diff.DataDiff.Changed = from.Source.KlineType != to.Source.KlineType || from.Source.DataWindow != to.Source.DataWindow

	// 元数据差异
	diff.MetaDiff.OldReviewStatus = from.ReviewStatus
	diff.MetaDiff.NewReviewStatus = to.ReviewStatus
	diff.MetaDiff.OldReliability = from.Validation.ReliabilityLabel
	diff.MetaDiff.NewReliability = to.Validation.ReliabilityLabel
	diff.MetaDiff.Changed = from.ReviewStatus != to.ReviewStatus

	// 摘要
	parts := []string{}
	if diff.ChangedRule {
		parts = append(parts, fmt.Sprintf("规则变更 (+%d/-%d)", len(diff.RuleDiff.Added), len(diff.RuleDiff.Removed)))
	}
	if diff.ChangedData {
		parts = append(parts, fmt.Sprintf("数据源切换 (%s→%s)", diff.DataDiff.OldDataSource, diff.DataDiff.NewDataSource))
	}
	if diff.ChangedMeta {
		parts = append(parts, fmt.Sprintf("状态变更 (%s→%s)", diff.MetaDiff.OldReviewStatus, diff.MetaDiff.NewReviewStatus))
	}
	if len(parts) == 0 {
		diff.Summary = "两版本内容完全一致"
	} else {
		diff.Summary = "变更摘要: " + joinStringsLocal(parts, ", ")
	}
	diff.RuleDiff.Updated = nil // reset: 复用 Added/Removed 即可

	return diff
}

func condKeys(conds []Condition) []string {
	out := make([]string, 0, len(conds))
	for _, c := range conds {
		out = append(out, fmt.Sprintf("%s %s %s", c.Indicator, c.Operator, c.Value))
	}
	return out
}

func flattenSellConds(sc SellConditions) []Condition {
	var out []Condition
	out = append(out, sc.TakeProfit...)
	out = append(out, sc.StopLoss...)
	return out
}

func diffKeys(a, b []string) (added []string) {
	set := map[string]struct{}{}
	for _, s := range b {
		set[s] = struct{}{}
	}
	for _, s := range a {
		if _, ok := set[s]; !ok {
			added = append(added, s)
		}
	}
	return
}

func joinStringsLocal(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += sep + s
	}
	return out
}
