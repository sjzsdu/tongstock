// Package ai_present defines structured evidence-citation, timeliness, and
// uncertainty-expression conventions for AI-generated research outputs in
// TongStock. It implements tongstock-qhe.6.4.
//
// Core idea:
//   - Every claim an AI produces MUST be tagged as Fact / Calculated / Inferred / Unknown.
//   - Claims MUST cite the internal object IDs and data as-of times they are based on.
//   - Stale or missing data must automatically downgrade the conclusion's scope.
//   - LLM confidence (probability) MUST NOT be expressed as historical win rate.
//
// This package is model-agnostic: it only provides the data structures and
// validators; the integration with specific LLM run-times lives elsewhere.
package ai_present

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// ============================================================================
// Claim 分类
// ============================================================================

// ClaimKind AI 声明的来源类型
type ClaimKind string

const (
	// KindFact 事实: 系统存储的已确认事实 (如 "XX 日期收盘为 Y 元")
	KindFact ClaimKind = "fact"
	// KindCalculated 计算: 系统基于事实计算出的派生指标 (如 "MA20 = 10.5")
	KindCalculated ClaimKind = "calculated"
	// KindInferred 推断: AI / 模型基于证据作出的推断 (如 "此形态可能预示上涨")
	KindInferred ClaimKind = "inferred"
	// KindUnknown 未知: 数据缺失、无法确认的事项
	KindUnknown ClaimKind = "unknown"
)

// String 返回可读描述
func (k ClaimKind) String() string {
	switch k {
	case KindFact:
		return "事实 (Fact)"
	case KindCalculated:
		return "计算 (Calculated)"
	case KindInferred:
		return "推断 (Inferred)"
	case KindUnknown:
		return "未知 (Unknown)"
	default:
		return "未分类"
	}
}

// ClaimLevel 声明的确定程度
type ClaimLevel string

const (
	LevelCertain     ClaimLevel = "certain"     // 完全确定 (事实)
	LevelHigh        ClaimLevel = "high"        // 高置信
	LevelModerate    ClaimLevel = "moderate"    // 中等置信
	LevelLow         ClaimLevel = "low"         // 低置信
	LevelUnspecified ClaimLevel = "unspecified" // 未指定 (缺省)
)

// ============================================================================
// 证据引用
// ============================================================================

// ObjectRef 内部对象引用
type ObjectRef struct {
	Type    string `json:"type"`    // "dataset_snapshot", "feature", "experiment", "paradigm_version", ...
	ID      string `json:"id"`      // 对象 ID
	Version string `json:"version"` // 对象版本号
}

// EvidenceCitation 证据引用
type EvidenceCitation struct {
	Ref       ObjectRef `json:"ref"`
	FieldPath string    `json:"field_path"` // 可选: 字段路径, 如 "metrics.sharpe_ratio"
	AsOf      time.Time `json:"as_of"`      // 数据截止时间
	Sources   []string  `json:"sources"`    // 数据源 (如 "tdx_daily.v2")
}

// ============================================================================
// 时效性检查
// ============================================================================

// TimelinessStatus 数据时效状态
type TimelinessStatus string

const (
	TimelinessFresh   TimelinessStatus = "fresh"   // 数据新鲜: 在预期窗口内
	TimelinessStale   TimelinessStatus = "stale"   // 数据过期
	TimelinessExpired TimelinessStatus = "expired" // 数据严重过期
	TimelinessUnknown TimelinessStatus = "unknown" // 时效未知
)

// TimelinessPolicy 时效降级策略
type TimelinessPolicy struct {
	FreshWindow   time.Duration `json:"fresh_window"`   // 新鲜窗口 (如 1 天)
	StaleWindow   time.Duration `json:"stale_window"`   // 过期窗口 (如 7 天)
	StaleAction   string        `json:"stale_action"`   // "downgrade", "flag", "reject"
	ExpiredAction string        `json:"expired_action"` // "downgrade", "flag", "reject"
}

// DefaultTimelinessPolicy 默认时效策略
func DefaultTimelinessPolicy() TimelinessPolicy {
	return TimelinessPolicy{
		FreshWindow:   24 * time.Hour,
		StaleWindow:   7 * 24 * time.Hour,
		StaleAction:   "downgrade",
		ExpiredAction: "reject",
	}
}

// CheckTimeliness 检查引用的时效性
func (p TimelinessPolicy) CheckTimeliness(citation EvidenceCitation) TimelinessStatus {
	age := time.Since(citation.AsOf)
	switch {
	case age <= p.FreshWindow:
		return TimelinessFresh
	case age <= p.StaleWindow:
		return TimelinessStale
	default:
		return TimelinessExpired
	}
}

// ============================================================================
// 不确定性表达
// ============================================================================

// UncertaintyExpression 不确定性表达
type UncertaintyExpression struct {
	// ConfidenceLevel 模型置信度 (0-1)。这是 AI 对"自己答案"的信心, 不是历史胜率。
	ConfidenceLevel float64 `json:"confidence_level"`
	// ConfidenceScale 置信度尺度: "model_self" (模型自评) | "historical" (历史回测统计)
	// 严格区分! 历史胜率必须用 HistoricalWinRate 字段, 不能与 ConfidenceLevel 混淆。
	ConfidenceScale string `json:"confidence_scale"`
	// HistoricalWinRate 历史胜率 (来自回测, 不是模型自评)
	HistoricalWinRate *float64 `json:"historical_win_rate,omitempty"`
	// HistoricalSamples 历史胜率样本量
	HistoricalSamples int `json:"historical_samples,omitempty"`
	// StandardError 标准误差 (可选)
	StandardError *float64 `json:"standard_error,omitempty"`
	// Reasoning 不确定性来源说明
	Reasoning string `json:"reasoning,omitempty"`
	// Caveats 需要注意的限制
	Caveats []string `json:"caveats,omitempty"`
}

// Validate 验证不确定性表达 (防止把 LLM confidence 表述为历史胜率)
func (u UncertaintyExpression) Validate() error {
	if u.ConfidenceLevel < 0 || u.ConfidenceLevel > 1 {
		return fmt.Errorf("confidence_level must be in [0, 1], got %f", u.ConfidenceLevel)
	}

	// 关键规则: 模型自评置信度不能表述为历史胜率
	if u.ConfidenceScale == "historical" {
		// 如果 scale 标记为 historical, 必须提供 HistoricalWinRate
		if u.HistoricalWinRate == nil {
			return fmt.Errorf("confidence_scale='historical' requires historical_win_rate")
		}
		if *u.HistoricalWinRate < 0 || *u.HistoricalWinRate > 1 {
			return fmt.Errorf("historical_win_rate must be in [0, 1], got %f", *u.HistoricalWinRate)
		}
		if u.HistoricalSamples < 0 {
			return fmt.Errorf("historical_samples must be non-negative")
		}
	} else if u.ConfidenceScale == "model_self" {
		// model_self 本身合法 — 与 historical_win_rate 的混淆检查由规则引擎处理
	}
	// 空 scale 视为不要求额外字段 (事实/计算类声明可以有 0 置信度)

	if u.StandardError != nil && (*u.StandardError < 0) {
		return fmt.Errorf("standard_error must be non-negative")
	}

	return nil
}

// IsLowConfidence 返回是否为低置信度声明
func (u UncertaintyExpression) IsLowConfidence() bool {
	return u.ConfidenceLevel < 0.5
}

// IsHistorical 返回是否基于历史数据
func (u UncertaintyExpression) IsHistorical() bool {
	return u.HistoricalWinRate != nil && u.HistoricalSamples > 0
}

// ============================================================================
// AI 声明
// ============================================================================

// AIClaim AI 生成的单一声明
type AIClaim struct {
	ID          string                `json:"id"`
	Kind        ClaimKind             `json:"kind"`
	Level       ClaimLevel            `json:"level"`
	Statement   string                `json:"statement"`
	Citations   []EvidenceCitation    `json:"citations"`
	Uncertainty UncertaintyExpression `json:"uncertainty"`
	Staleness   []string              `json:"staleness,omitempty"`
	Warnings    []string              `json:"warnings,omitempty"`
	Invalidated bool                  `json:"invalidated,omitempty"`
}

// NewClaim 创建一个新声明
func NewClaim(id string, kind ClaimKind, statement string) *AIClaim {
	return &AIClaim{
		ID:        id,
		Kind:      kind,
		Level:     claimLevelFromKind(kind),
		Statement: statement,
		Citations: make([]EvidenceCitation, 0),
		Uncertainty: UncertaintyExpression{
			ConfidenceLevel: 0.0,
			ConfidenceScale: scaleFromKind(kind),
		},
		Warnings: make([]string, 0),
	}
}

func claimLevelFromKind(kind ClaimKind) ClaimLevel {
	switch kind {
	case KindFact, KindCalculated:
		return LevelCertain
	case KindInferred:
		return LevelModerate
	case KindUnknown:
		return LevelLow
	default:
		return LevelUnspecified
	}
}

func scaleFromKind(kind ClaimKind) string {
	switch kind {
	case KindFact, KindCalculated:
		return "" // 事实/计算类不需要置信度尺度
	case KindInferred:
		return "model_self"
	default:
		return ""
	}
}

// AddCitation 添加证据引用
func (c *AIClaim) AddCitation(ref ObjectRef, asOf time.Time, sources ...string) *AIClaim {
	c.Citations = append(c.Citations, EvidenceCitation{
		Ref:     ref,
		AsOf:    asOf,
		Sources: sources,
	})
	return c
}

// Validate 验证声明
func (c *AIClaim) Validate() error {
	if c.Statement == "" {
		return errors.New("statement is required")
	}

	// 推断和事实/计算都必须有引用 (至少 1 个)
	if c.Kind == KindFact || c.Kind == KindCalculated || c.Kind == KindInferred {
		if len(c.Citations) == 0 {
			return fmt.Errorf("claim of kind %s requires at least one citation", c.Kind)
		}
	}

	// 验证每个引用
	for i, cit := range c.Citations {
		if cit.Ref.Type == "" || cit.Ref.ID == "" {
			return fmt.Errorf("citation[%d]: ref.type and ref.id are required", i)
		}
	}

	// 验证不确定性表达
	if err := c.Uncertainty.Validate(); err != nil {
		return fmt.Errorf("uncertainty: %w", err)
	}

	return nil
}

// ============================================================================
// AI 报告
// ============================================================================

// AIReport AI 研究报告
type AIReport struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	SessionID string     `json:"session_id"`
	Generated time.Time  `json:"generated"`
	Claims    []*AIClaim `json:"claims"`
	Summary   string     `json:"summary"`
	Warnings  []string   `json:"warnings,omitempty"`
}

// NewReport 创建报告
func NewReport(id, agentID, sessionID string) *AIReport {
	return &AIReport{
		ID:        id,
		AgentID:   agentID,
		SessionID: sessionID,
		Generated: time.Now(),
		Claims:    make([]*AIClaim, 0),
		Warnings:  make([]string, 0),
	}
}

// AddClaim 添加声明
func (r *AIReport) AddClaim(claim *AIClaim) *AIReport {
	r.Claims = append(r.Claims, claim)
	return r
}

// Validate 验证所有声明
func (r *AIReport) Validate() error {
	for i, c := range r.Claims {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("claim[%d] %s: %w", i, c.ID, err)
		}
	}
	return nil
}

// ============================================================================
// 时效降级
// ============================================================================

// ApplyTimelinessPolicy 对所有声明应用时效策略
// 返回被降级的声明 ID 和新的声明等级
func (r *AIReport) ApplyTimelinessPolicy(policy TimelinessPolicy) []TimelinessDowngrade {
	var downgrades []TimelinessDowngrade

	for _, claim := range r.Claims {
		worstStatus := TimelinessFresh
		for _, cit := range claim.Citations {
			status := policy.CheckTimeliness(cit)
			if severity(status) > severity(worstStatus) {
				worstStatus = status
			}
		}

		if worstStatus != TimelinessFresh {
			action := policyAction(worstStatus, policy)
			switch action {
			case "reject":
				claim.Invalidated = true
				claim.Warnings = append(claim.Warnings, fmt.Sprintf("引用数据已严重过期: %s", worstStatus))
				downgrades = append(downgrades, TimelinessDowngrade{
					ClaimID:  claim.ID,
					OldLevel: claim.Level,
					NewLevel: "",
					Action:   "reject",
					Reason:   "数据严重过期",
				})
			case "downgrade":
				newLevel := downgradeLevel(claim.Level)
				downgrades = append(downgrades, TimelinessDowngrade{
					ClaimID:  claim.ID,
					OldLevel: claim.Level,
					NewLevel: newLevel,
					Action:   "downgrade",
					Reason:   fmt.Sprintf("引用数据已过期 (%s)", worstStatus),
				})
				claim.Level = newLevel
				claim.Warnings = append(claim.Warnings, fmt.Sprintf("数据时效降级: %s → %s (原因: %s)", claim.Level, newLevel, worstStatus))
			case "flag":
				claim.Warnings = append(claim.Warnings, fmt.Sprintf("引用数据时效存疑: %s", worstStatus))
			}
		}
	}

	return downgrades
}

// TimelinessDowngrade 时效降级记录
type TimelinessDowngrade struct {
	ClaimID  string     `json:"claim_id"`
	OldLevel ClaimLevel `json:"old_level"`
	NewLevel ClaimLevel `json:"new_level"`
	Action   string     `json:"action"` // "downgrade", "reject"
	Reason   string     `json:"reason"`
}

func severity(status TimelinessStatus) int {
	switch status {
	case TimelinessExpired:
		return 3
	case TimelinessStale:
		return 2
	case TimelinessUnknown:
		return 1
	default:
		return 0
	}
}

func policyAction(status TimelinessStatus, policy TimelinessPolicy) string {
	if status == TimelinessExpired {
		return policy.ExpiredAction
	}
	if status == TimelinessStale {
		return policy.StaleAction
	}
	return "flag"
}

func downgradeLevel(level ClaimLevel) ClaimLevel {
	switch level {
	case LevelCertain:
		return LevelHigh
	case LevelHigh:
		return LevelModerate
	case LevelModerate:
		return LevelLow
	default:
		return LevelLow
	}
}

// ============================================================================
// 反 LLM-Confusion 规则引擎
// ============================================================================

// AntiConfusionRule 反混淆规则
type AntiConfusionRule struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Apply       func(claim *AIClaim) []string
}

// DefaultAntiConfusionRules 默认反混淆规则集
func DefaultAntiConfusionRules() []AntiConfusionRule {
	return []AntiConfusionRule{
		{
			ID:          "no_llm_confidence_as_winrate",
			Description: "禁止把 LLM confidence 表述为历史胜率",
			Apply: func(claim *AIClaim) []string {
				var issues []string
				u := claim.Uncertainty
				if u.ConfidenceScale == "model_self" && u.HistoricalWinRate != nil {
					issues = append(issues,
						fmt.Sprintf("声明 %s 把模型自评置信度与历史胜率混淆 (model_self + historical_win_rate 同时存在)", claim.ID))
				}
				return issues
			},
		},
		{
			ID:          "inferred_requires_low_confidence_marker",
			Description: "推断类声明必须明确标记为 Inferred 类型",
			Apply: func(claim *AIClaim) []string {
				var issues []string
				if claim.Kind == KindInferred && claim.Level == LevelCertain {
					issues = append(issues,
						fmt.Sprintf("推断类声明 %s 不应标记为 certain", claim.ID))
				}
				if claim.Kind == KindInferred && claim.Uncertainty.ConfidenceLevel > 0.95 {
					issues = append(issues,
						fmt.Sprintf("推断类声明 %s 置信度 %.2f 过高, 建议降低", claim.ID, claim.Uncertainty.ConfidenceLevel))
				}
				return issues
			},
		},
		{
			ID:          "historical_winrate_requires_samples",
			Description: "历史胜率必须伴随样本量",
			Apply: func(claim *AIClaim) []string {
				var issues []string
				u := claim.Uncertainty
				if u.HistoricalWinRate != nil && u.HistoricalSamples == 0 {
					issues = append(issues,
						fmt.Sprintf("声明 %s 提供历史胜率 %.2f 但未提供样本量", claim.ID, *u.HistoricalWinRate))
				}
				return issues
			},
		},
		{
			ID:          "claim_kind_consistency",
			Description: "声明类型必须与引用性质一致",
			Apply: func(claim *AIClaim) []string {
				var issues []string
				if claim.Kind == KindFact {
					for _, warning := range claim.Warnings {
						if strings.Contains(warning, "推断") || strings.Contains(warning, "可能") {
							issues = append(issues,
								fmt.Sprintf("事实声明 %s 包含推断性质的措辞: %s", claim.ID, warning))
						}
					}
				}
				return issues
			},
		},
		{
			ID:          "no_vague_probability",
			Description: "模糊概率表述必须有明确尺度标注",
			Apply: func(claim *AIClaim) []string {
				var issues []string
				u := claim.Uncertainty
				if u.ConfidenceLevel > 0.7 && u.ConfidenceScale == "" {
					issues = append(issues,
						fmt.Sprintf("声明 %s 置信度 %.2f 但未标注尺度 (model_self / historical)", claim.ID, u.ConfidenceLevel))
				}
				return issues
			},
		},
	}
}

// ApplyRules 应用规则到报告, 返回所有违反项
func (r *AIReport) ApplyRules(rules []AntiConfusionRule) []RuleViolation {
	var violations []RuleViolation

	for _, claim := range r.Claims {
		if claim.Invalidated {
			continue
		}
		for _, rule := range rules {
			issues := rule.Apply(claim)
			for _, issue := range issues {
				violations = append(violations, RuleViolation{
					ClaimID:      claim.ID,
					RuleID:       rule.ID,
					RuleDesc:     rule.Description,
					Message:      issue,
					SuggestedFix: "",
				})
			}
		}
	}

	return violations
}

// RuleViolation 规则违反
type RuleViolation struct {
	ClaimID      string `json:"claim_id"`
	RuleID       string `json:"rule_id"`
	RuleDesc     string `json:"rule_desc"`
	Message      string `json:"message"`
	SuggestedFix string `json:"suggested_fix"`
}

// ============================================================================
// 报告生成摘要
// ============================================================================

// ReportSummary 报告摘要
type ReportSummary struct {
	ClaimsCount           int         `json:"claims_count"`
	ValidClaimsCount      int         `json:"valid_claims_count"`
	ByKind                map[int]int `json:"by_kind"`
	StaleCitations        int         `json:"stale_citations"`
	RuleViolations        int         `json:"rule_violations"`
	PotentiallyMisleading int         `json:"potentially_misleading"`
	HasHistoricalWinRate  bool        `json:"has_historical_win_rate"`
	HasModelSelf          bool        `json:"has_model_self"`
	Warnings              []string    `json:"warnings,omitempty"`
}

// GenerateSummary 生成报告摘要
func (r *AIReport) GenerateSummary() ReportSummary {
	summary := ReportSummary{
		ByKind: make(map[int]int),
	}

	for _, c := range r.Claims {
		summary.ClaimsCount++
		if !c.Invalidated {
			summary.ValidClaimsCount++
		}

		switch c.Kind {
		case KindFact:
			summary.ByKind[1]++
		case KindCalculated:
			summary.ByKind[2]++
		case KindInferred:
			summary.ByKind[3]++
		case KindUnknown:
			summary.ByKind[4]++
		}

		if c.Uncertainty.IsHistorical() {
			summary.HasHistoricalWinRate = true
		}
		if c.Uncertainty.ConfidenceScale == "model_self" {
			summary.HasModelSelf = true
		}
	}

	// 简化的潜在误导检测
	if summary.HasHistoricalWinRate && summary.HasModelSelf {
		summary.PotentiallyMisleading++
		summary.Warnings = append(summary.Warnings,
			"报告同时包含历史胜率和模型自评, 需要 AI 明确区分")
	}

	return summary
}

// ============================================================================
// 小工具
// ============================================================================

// ValidConfidenceScale 检查置信度尺度是否合法
func ValidConfidenceScale(scale string) bool {
	return scale == "model_self" || scale == "historical" || scale == ""
}

// ScaleDisplayName 返回可读名称
func ScaleDisplayName(scale string) string {
	switch scale {
	case "model_self":
		return "模型自评 (LLM self-assessment)"
	case "historical":
		return "历史统计 (historical backtest)"
	default:
		return "未指定"
	}
}

// round 四舍五入到指定精度
func round(v float64, decimals int) float64 {
	pow := math.Pow10(decimals)
	return math.Round(v*pow) / pow
}
