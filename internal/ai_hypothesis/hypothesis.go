// Package ai_hypothesis implements structured AI hypothesis generation
// for TongStock's paradigm research system. Instead of giving buy/sell
// conclusions, the AI proposes falsifiable hypotheses based on available
// features and historical evidence.
//
// Design goals:
//   - Falsifiable: every hypothesis can be proven wrong by backtest
//   - Schema-conformant: output directly enters Candidate quarantine
//   - Explainable: behavioral logic, counter-examples, and verification items
//   - Versioned: model, prompt, and input versions are tracked
//   - Reject-by-default: missing data or unexecutable conditions → reject
package ai_hypothesis

import (
	"fmt"
	"time"
)

// ============================================================================
// 核心数据模型
// ============================================================================

// HypothesisStatus 假设状态
type HypothesisStatus string

const (
	HypothesisDraft       HypothesisStatus = "draft"       // 草稿 (刚生成)
	HypothesisValidated   HypothesisStatus = "validated"   // 已通过可证伪性检查
	HypothesisRejected    HypothesisStatus = "rejected"    // 已拒绝 (不可证伪/缺失数据)
	HypothesisSchemaOK    HypothesisStatus = "schema_ok"   // Schema 合规, 可进入隔离区
	HypothesisInQuarantine HypothesisStatus = "quarantine" // 已进入候选隔离区
)

// BehavioralLogic 行为逻辑: 为什么这个假设应该成立
type BehavioralLogic struct {
	// Mechanism 描述核心机制 (e.g., "RSI 超卖后均值回归")
	Mechanism string `json:"mechanism"`
	// Driver 驱动因素 (e.g., "短期过度抛售导致价格偏离价值中枢")
	Driver string `json:"driver"`
	// MarketContext 适用的市场环境
	MarketContext string `json:"market_context"`
	// HistoricalEvidence 历史相关证据摘要
	HistoricalEvidence []string `json:"historical_evidence,omitempty"`
	// KeyAssumptions 关键假设 (需要验证的前提)
	KeyAssumptions []string `json:"key_assumptions"`
}

// CounterExample 反例: 在哪些条件下假设会失败
type CounterExample struct {
	// Condition 反例条件描述
	Condition string `json:"condition"`
	// WhyItFails 失败原因解释
	WhyItFails string `json:"why_it_fails"`
	// Severity 严重程度: "low", "medium", "high"
	Severity string `json:"severity"`
}

// VerificationItem 验证项: 回测时需要检查的具体项
type VerificationItem struct {
	// Name 验证项名称
	Name string `json:"name"`
	// Description 验证项描述
	Description string `json:"description"`
	// Metric 使用的指标 (e.g., "sharpe_ratio", "max_drawdown", "win_rate")
	Metric string `json:"metric"`
	// Threshold 阈值 (达到此值视为通过)
	Threshold float64 `json:"threshold"`
	// Direction "above" 或 "below" (指标应高于/低于阈值)
	Direction string `json:"direction"`
	// Category 分类: "performance", "risk", "robustness", "stability"
	Category string `json:"category"`
}

// VersionTag 版本标签: 追踪 AI 生成的每个环节版本
type VersionTag struct {
	// Model LLM 模型标识
	Model string `json:"model"`
	// ModelVersion 模型版本
	ModelVersion string `json:"model_version"`
	// PromptTemplateID 使用的提示词模板 ID
	PromptTemplateID string `json:"prompt_template_id"`
	// PromptVersion 提示词模板版本
	PromptVersion string `json:"prompt_version"`
	// InputFeaturesVersion 输入特征版本 (数据快照版本)
	InputFeaturesVersion string `json:"input_features_version"`
	// InputEvidenceVersion 输入证据版本
	InputEvidenceVersion string `json:"input_evidence_version"`
	// GeneratedAt 生成时间
	GeneratedAt time.Time `json:"generated_at"`
}

// MissingDataIssue 缺失数据问题
type MissingDataIssue struct {
	// FieldName 缺失的字段名
	FieldName string `json:"field_name"`
	// FieldType 字段类型 (feature, dataset, evidence)
	FieldType string `json:"field_type"`
	// Description 缺失描述
	Description string `json:"description"`
	// Impact 影响程度: "critical", "warning"
	Impact string `json:"impact"`
}

// AIHypothesis AI 生成的结构化假设
type AIHypothesis struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Statement   string               `json:"statement"`   // 可证伪的假设陈述
	Status      HypothesisStatus     `json:"status"`
	Behavior    BehavioralLogic      `json:"behavior"`    // 行为逻辑
	CounterExamples []CounterExample `json:"counter_examples"` // 反例列表
	Verifications []VerificationItem `json:"verifications"`    // 验证项列表
	SchemaSpec   HypothesisSchemaSpec `json:"schema_spec"`   // 生成的 Schema 规范
	VersionTag   VersionTag           `json:"version_tag"`   // 版本追踪
	MissingData  []MissingDataIssue   `json:"missing_data,omitempty"` // 缺失数据
	RejectReason string               `json:"reject_reason,omitempty"` // 拒绝原因
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

// HypothesisSchemaSpec 假设对应的范式 Schema 规范 (准备进入隔离区)
type HypothesisSchemaSpec struct {
	// SchemaID 生成的 Schema ID
	SchemaID string `json:"schema_id"`
	// SchemaName 范式名称
	SchemaName string `json:"schema_name"`
	// HoldingPeriod 推荐持有期
	HoldingPeriod string `json:"holding_period"`
	// EntryConditions 入场条件描述 (供参考, 实际 Schema 用结构化规则)
	EntryConditions []string `json:"entry_conditions"`
	// ExitConditions 出场条件描述
	ExitConditions []string `json:"exit_conditions"`
	// ContextConstraints 上下文约束
	ContextConstraints []string `json:"context_constraints"`
	// ExpectedReturn 预期收益范围
	ExpectedReturn string `json:"expected_return"`
	// RiskLevel 风险等级
	RiskLevel string `json:"risk_level"`
}

// ============================================================================
// 工厂方法
// ============================================================================

// NewAIHypothesis 创建新的 AI 假设
func NewAIHypothesis(id, title, statement string) *AIHypothesis {
	now := time.Now()
	return &AIHypothesis{
		ID:            id,
		Title:         title,
		Statement:     statement,
		Status:        HypothesisDraft,
		CounterExamples: make([]CounterExample, 0),
		Verifications: make([]VerificationItem, 0),
		MissingData:   make([]MissingDataIssue, 0),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// AddCounterExample 添加反例
func (h *AIHypothesis) AddCounterExample(cond, reason, severity string) *AIHypothesis {
	h.CounterExamples = append(h.CounterExamples, CounterExample{
		Condition:  cond,
		WhyItFails: reason,
		Severity:   severity,
	})
	h.UpdatedAt = time.Now()
	return h
}

// AddVerification 添加验证项
func (h *AIHypothesis) AddVerification(name, desc, metric string, threshold float64, direction, category string) *AIHypothesis {
	h.Verifications = append(h.Verifications, VerificationItem{
		Name:        name,
		Description: desc,
		Metric:      metric,
		Threshold:   threshold,
		Direction:   direction,
		Category:    category,
	})
	h.UpdatedAt = time.Now()
	return h
}

// AddMissingData 添加缺失数据记录
func (h *AIHypothesis) AddMissingData(fieldName, fieldType, description, impact string) *AIHypothesis {
	h.MissingData = append(h.MissingData, MissingDataIssue{
		FieldName:   fieldName,
		FieldType:   fieldType,
		Description: description,
		Impact:      impact,
	})
	h.UpdatedAt = time.Now()
	return h
}

// SetVersionTag 设置版本标签
func (h *AIHypothesis) SetVersionTag(model, modelVersion, promptID, promptVersion, inputFeaturesVersion, inputEvidenceVersion string) *AIHypothesis {
	h.VersionTag = VersionTag{
		Model:                model,
		ModelVersion:         modelVersion,
		PromptTemplateID:     promptID,
		PromptVersion:        promptVersion,
		InputFeaturesVersion: inputFeaturesVersion,
		InputEvidenceVersion: inputEvidenceVersion,
		GeneratedAt:          time.Now(),
	}
	h.UpdatedAt = time.Now()
	return h
}

// Reject 拒绝假设
func (h *AIHypothesis) Reject(reason string) *AIHypothesis {
	h.Status = HypothesisRejected
	h.RejectReason = reason
	h.UpdatedAt = time.Now()
	return h
}

// Approve 批准假设通过可证伪性检查
func (h *AIHypothesis) Approve() *AIHypothesis {
	h.Status = HypothesisValidated
	h.UpdatedAt = time.Now()
	return h
}

// MarkSchemaOK 标记 Schema 合规
func (h *AIHypothesis) MarkSchemaOK() *AIHypothesis {
	h.Status = HypothesisSchemaOK
	h.UpdatedAt = time.Now()
	return h
}

// EnterQuarantine 进入隔离区
func (h *AIHypothesis) EnterQuarantine() *AIHypothesis {
	h.Status = HypothesisInQuarantine
	h.UpdatedAt = time.Now()
	return h
}

// HasCriticalMissingData 检查是否有关键性缺失数据
func (h *AIHypothesis) HasCriticalMissingData() bool {
	for _, md := range h.MissingData {
		if md.Impact == "critical" {
			return true
		}
	}
	return false
}

// Validate 基础验证
func (h *AIHypothesis) Validate() error {
	if h.ID == "" {
		return fmt.Errorf("hypothesis ID is required")
	}
	if h.Title == "" {
		return fmt.Errorf("hypothesis title is required")
	}
	if h.Statement == "" {
		return fmt.Errorf("hypothesis statement is required")
	}
	if len(h.VersionTag.Model) == 0 {
		return fmt.Errorf("version tag model is required")
	}
	if len(h.VersionTag.PromptTemplateID) == 0 {
		return fmt.Errorf("version tag prompt_template_id is required")
	}
	if len(h.VersionTag.PromptVersion) == 0 {
		return fmt.Errorf("version tag prompt_version is required")
	}
	return nil
}
