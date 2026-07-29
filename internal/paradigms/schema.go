package paradigms

import (
	"fmt"
	"time"
)

// ============================================================================
// 严格范式 Schema
// ============================================================================

// RuleOperator 支持的规则运算符
type RuleOperator string

const (
	OpGreaterThan RuleOperator = "gt"           // 大于
	OpLessThan    RuleOperator = "lt"           // 小于
	OpEqual       RuleOperator = "eq"           // 等于
	OpBetween     RuleOperator = "between"      // 区间内
	OpCrossAbove  RuleOperator = "cross_above"  // 上穿 (指标上穿阈值)
	OpCrossBelow  RuleOperator = "cross_below"  // 下穿 (指标下穿阈值)
	OpAbove       RuleOperator = "above"        // 持续高于
	OpBelow       RuleOperator = "below"        // 持续低于
	OpMaxDrawdown RuleOperator = "max_dd"       // 最大回撤
	OpMaxDuration RuleOperator = "max_duration" // 最大持续天数
)

// ValidOperators 有效运算符列表
var ValidOperators = map[RuleOperator]bool{
	OpGreaterThan: true,
	OpLessThan:    true,
	OpEqual:       true,
	OpBetween:     true,
	OpCrossAbove:  true,
	OpCrossBelow:  true,
	OpAbove:       true,
	OpBelow:       true,
	OpMaxDrawdown: true,
	OpMaxDuration: true,
}

// RuleSide 规则适用方向
type RuleSide string

const (
	SideBuy  RuleSide = "buy"  // 买入
	SideSell RuleSide = "sell" // 卖出
)

// RuleType 规则类型
type RuleType string

const (
	TypeEntry        RuleType = "entry"        // 入场条件
	TypeExitProfit   RuleType = "exit_profit"  // 止盈条件
	TypeExitLoss     RuleType = "exit_loss"    // 止损条件
	TypeConfirmation RuleType = "confirmation" // 确认条件 (加分项)
	TypeInvalidation RuleType = "invalidation" // 失效条件
	TypeContext      RuleType = "context"      // 上下文条件
)

// ValidRuleTypes 有效规则类型
var ValidRuleTypes = map[RuleType]bool{
	TypeEntry:        true,
	TypeExitProfit:   true,
	TypeExitLoss:     true,
	TypeConfirmation: true,
	TypeInvalidation: true,
	TypeContext:      true,
}

// ContextKey 上下文键
type ContextKey string

const (
	ContextMarketCap           ContextKey = "market_cap"           // 市值规模
	ContextShareholderDominant ContextKey = "shareholder_dominant" // 股东主导类型
	ContextActivity            ContextKey = "activity"             // 活跃度
	ContextTrend               ContextKey = "trend"                // 趋势状态
	ContextVolatility          ContextKey = "volatility"           // 波动率
	ContextSector              ContextKey = "sector"               // 行业
	ContextMarketSentiment     ContextKey = "market_sentiment"     // 市场情绪
	ContextLiquidity           ContextKey = "liquidity"            // 流动性
)

// ValidContextKeys 有效上下文键
var ValidContextKeys = map[ContextKey]bool{
	ContextMarketCap:           true,
	ContextShareholderDominant: true,
	ContextActivity:            true,
	ContextTrend:               true,
	ContextVolatility:          true,
	ContextSector:              true,
	ContextMarketSentiment:     true,
	ContextLiquidity:           true,
}

// Rule 单条规则
type Rule struct {
	ID          string       `json:"id"`           // 规则唯一 ID
	Type        RuleType     `json:"type"`         // 规则类型
	Side        RuleSide     `json:"side"`         // 适用方向 (buy/sell)
	FeatureName string       `json:"feature_name"` // 特征/指标名
	Operator    RuleOperator `json:"operator"`     // 运算符
	Thresholds  []float64    `json:"thresholds"`   // 阈值 (支持单值或区间)
	Required    bool         `json:"required"`     // 是否必须满足
	Weight      float64      `json:"weight"`       // 权重 (0-1, 用于确认规则)
	Description string       `json:"description"`  // 规则描述
}

// IsValid 验证规则有效性
func (r *Rule) IsValid() error {
	if r.ID == "" {
		return fmt.Errorf("rule id is required")
	}
	if !ValidRuleTypes[r.Type] {
		return fmt.Errorf("invalid rule type: %s", r.Type)
	}
	if !ValidOperators[r.Operator] {
		return fmt.Errorf("invalid operator: %s", r.Operator)
	}
	if r.FeatureName == "" {
		return fmt.Errorf("feature name is required")
	}

	// 验证阈值
	switch r.Operator {
	case OpBetween:
		if len(r.Thresholds) != 2 || r.Thresholds[0] >= r.Thresholds[1] {
			return fmt.Errorf("between operator requires two ordered thresholds")
		}
	default:
		if len(r.Thresholds) != 1 {
			return fmt.Errorf("operator %s requires exactly one threshold", r.Operator)
		}
	}

	// 验证权重范围
	if r.Type == TypeConfirmation && (r.Weight < 0 || r.Weight > 1) {
		return fmt.Errorf("confirmation rule weight must be between 0 and 1")
	}

	return nil
}

// ContextRule 上下文约束规则
type ContextRule struct {
	Key      ContextKey   `json:"key"`      // 上下文键
	Operator RuleOperator `json:"operator"` // 运算符 (eq/in/not_in)
	Values   []string     `json:"values"`   // 允许的值
}

// IsValid 验证上下文规则
func (cr *ContextRule) IsValid() error {
	if !ValidContextKeys[cr.Key] {
		return fmt.Errorf("invalid context key: %s", cr.Key)
	}
	if len(cr.Values) == 0 {
		return fmt.Errorf("at least one value is required for context rule")
	}
	return nil
}

// FeatureDefinition 特征定义
type FeatureDefinition struct {
	Name        string             `json:"name"`        // 特征名
	Type        string             `json:"type"`        // 类型: indicator, price, volume, market
	Calculation string             `json:"calculation"` // 计算方式 (可选)
	Params      map[string]float64 `json:"params"`      // 参数
	Dependency  []string           `json:"dependency"`  // 依赖的其他特征
	Description string             `json:"description"` // 描述
}

// ParadigmSchema 严格范式 Schema
type ParadigmSchema struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Version       int                 `json:"version"`
	SchemaVersion string              `json:"schema_version"` // Schema 版本号
	Description   string              `json:"description"`
	Features      []FeatureDefinition `json:"features"`       // 使用的特征定义
	ContextRules  []ContextRule       `json:"context_rules"`  // 上下文约束
	Rules         []Rule              `json:"rules"`          // 规则列表
	HoldingPeriod string              `json:"holding_period"` // 持有期: "intraday", "short(1-5d)", "medium(5-20d)", "long(20d+)"
	MaxDrawdown   float64             `json:"max_drawdown"`   // 最大可接受回撤
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	ChangeReason  string              `json:"change_reason"`  // 版本变更原因
	ParentVersion int                 `json:"parent_version"` // 父版本号
}

// NewParadigmSchema 创建新的范式 Schema
func NewParadigmSchema(id, name string) *ParadigmSchema {
	return &ParadigmSchema{
		ID:            id,
		Name:          name,
		Version:       1,
		SchemaVersion: "1.0",
		Features:      make([]FeatureDefinition, 0),
		ContextRules:  make([]ContextRule, 0),
		Rules:         make([]Rule, 0),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// IsValid 验证 Schema 完整性
func (s *ParadigmSchema) IsValid() error {
	if s.ID == "" {
		return fmt.Errorf("schema id is required")
	}
	if s.Name == "" {
		return fmt.Errorf("schema name is required")
	}

	// 验证所有规则
	for i, rule := range s.Rules {
		if err := rule.IsValid(); err != nil {
			return fmt.Errorf("rule[%d] %s: %w", i, rule.ID, err)
		}
	}

	// 验证上下文规则
	for i, cr := range s.ContextRules {
		if err := cr.IsValid(); err != nil {
			return fmt.Errorf("context_rule[%d]: %w", i, err)
		}
	}

	// 验证特征引用
	featureMap := make(map[string]bool)
	for _, f := range s.Features {
		featureMap[f.Name] = true
	}

	for _, rule := range s.Rules {
		if !featureMap[rule.FeatureName] {
			return fmt.Errorf("rule %s references undefined feature: %s", rule.ID, rule.FeatureName)
		}
	}

	// 必须有至少一条入场规则
	hasEntry := false
	for _, rule := range s.Rules {
		if rule.Type == TypeEntry {
			hasEntry = true
			break
		}
	}
	if !hasEntry {
		return fmt.Errorf("at least one entry rule is required")
	}

	// 持有期验证
	validPeriods := map[string]bool{
		"intraday": true,
		"short":    true,
		"medium":   true,
		"long":     true,
	}
	if !validPeriods[s.HoldingPeriod] {
		return fmt.Errorf("invalid holding period: %s", s.HoldingPeriod)
	}

	return nil
}

// CreateVersion 创建新版本 (不可变快照)
func (s *ParadigmSchema) CreateVersion(reason string) *ParadigmSchema {
	newSchema := &ParadigmSchema{
		ID:            s.ID,
		Name:          s.Name,
		Version:       s.Version + 1,
		SchemaVersion: s.SchemaVersion,
		Description:   s.Description,
		HoldingPeriod: s.HoldingPeriod,
		MaxDrawdown:   s.MaxDrawdown,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     time.Now(),
		ChangeReason:  reason,
		ParentVersion: s.Version,
	}

	// 深拷贝特征
	newSchema.Features = make([]FeatureDefinition, len(s.Features))
	copy(newSchema.Features, s.Features)

	// 深拷贝上下文规则
	newSchema.ContextRules = make([]ContextRule, len(s.ContextRules))
	copy(newSchema.ContextRules, s.ContextRules)

	// 深拷贝规则 (包括 Thresholds 切片)
	newSchema.Rules = make([]Rule, len(s.Rules))
	for i, r := range s.Rules {
		newSchema.Rules[i] = r
		if r.Thresholds != nil {
			newSchema.Rules[i].Thresholds = make([]float64, len(r.Thresholds))
			copy(newSchema.Rules[i].Thresholds, r.Thresholds)
		}
	}

	return newSchema
}

// GetRulesByType 按类型获取规则
func (s *ParadigmSchema) GetRulesByType(ruleType RuleType) []Rule {
	var rules []Rule
	for _, r := range s.Rules {
		if r.Type == ruleType {
			rules = append(rules, r)
		}
	}
	return rules
}

// GetRulesBySide 按方向获取规则
func (s *ParadigmSchema) GetRulesBySide(side RuleSide) []Rule {
	var rules []Rule
	for _, r := range s.Rules {
		if r.Side == side {
			rules = append(rules, r)
		}
	}
	return rules
}

// Validate 验证规则集的可执行性
func (s *ParadigmSchema) Validate() ([]ValidationError, error) {
	var errors []ValidationError

	// 1. 检查是否有冲突的规则 (如同时要求 gt 和 lt)
	rulesByFeature := make(map[string][]Rule)
	for _, r := range s.Rules {
		rulesByFeature[r.FeatureName] = append(rulesByFeature[r.FeatureName], r)
	}

	for feature, rules := range rulesByFeature {
		for i := 0; i < len(rules); i++ {
			for j := i + 1; j < len(rules); j++ {
				// 检查同一特征上 gt 和 lt 同时存在 (可能冲突)
				if (rules[i].Operator == OpGreaterThan && rules[j].Operator == OpLessThan) ||
					(rules[i].Operator == OpLessThan && rules[j].Operator == OpGreaterThan) {
					// 只有当阈值范围不重叠时才冲突
					if len(rules[i].Thresholds) > 0 && len(rules[j].Thresholds) > 0 {
						gtRule := rules[i]
						ltRule := rules[j]
						if rules[i].Operator == OpLessThan {
							gtRule, ltRule = ltRule, gtRule
						}
						if gtRule.Thresholds[0] >= ltRule.Thresholds[0] {
							errors = append(errors, ValidationError{
								Level:   "warning",
								Field:   "rules",
								Message: fmt.Sprintf("feature %s has conflicting gt(%.2f) and lt(%.2f) rules", feature, gtRule.Thresholds[0], ltRule.Thresholds[0]),
							})
						}
					}
				}
			}
		}
	}

	// 2. 检查入场规则和出场规则是否对称
	entryRules := s.GetRulesByType(TypeEntry)
	exitRules := append(s.GetRulesByType(TypeExitProfit), s.GetRulesByType(TypeExitLoss)...)

	if len(entryRules) > 0 && len(exitRules) == 0 {
		errors = append(errors, ValidationError{
			Level:   "warning",
			Field:   "rules",
			Message: "has entry rules but no exit rules (take profit or stop loss)",
		})
	}

	// 3. 检查止损规则的阈值合理性
	for _, rule := range s.GetRulesByType(TypeExitLoss) {
		if len(rule.Thresholds) > 0 && rule.Thresholds[0] >= 0.1 {
			errors = append(errors, ValidationError{
				Level:   "warning",
				Field:   "rules",
				RuleID:  rule.ID,
				Message: fmt.Sprintf("stop loss threshold %.1f%% seems high (typical range: 1-10%%)", rule.Thresholds[0]*100),
			})
		}
	}

	return errors, nil
}

// ValidationError 验证错误
type ValidationError struct {
	Level   string // error, warning, info
	Field   string // 字段: rules, context, features
	RuleID  string // 相关规则 ID (可选)
	Message string // 错误描述
}

// String 返回错误描述
func (ve ValidationError) String() string {
	if ve.RuleID != "" {
		return fmt.Sprintf("[%s] %s: rule %s - %s", ve.Level, ve.Field, ve.RuleID, ve.Message)
	}
	return fmt.Sprintf("[%s] %s: %s", ve.Level, ve.Field, ve.Message)
}

// HasErrors 检查是否有严重错误
func (s *ParadigmSchema) HasErrors(errors []ValidationError) bool {
	for _, e := range errors {
		if e.Level == "error" {
			return true
		}
	}
	return false
}
