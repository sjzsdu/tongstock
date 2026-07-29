package paradigms

import (
	"fmt"
	"strings"
)

// ============================================================================
// 规则编译器
// ============================================================================

// CompiledRule 编译后的规则
type CompiledRule struct {
	ID         string       `json:"id"`
	Type       RuleType     `json:"type"`
	Side       RuleSide     `json:"side"`
	Feature    string       `json:"feature"`
	Operator   RuleOperator `json:"operator"`
	Threshold  float64      `json:"threshold"`
	Threshold2 float64      `json:"threshold2,omitempty"`
	Required   bool         `json:"required"`
	Weight     float64      `json:"weight"`
	// 执行函数名 (用于调试/日志)
	FuncName string `json:"func_name"`
}

// CompiledSchema 编译后的 Schema
type CompiledSchema struct {
	ID            string          `json:"id"`
	Version       int             `json:"version"`
	EntryRules    []CompiledRule  `json:"entry_rules"`
	ExitRules     []CompiledRule  `json:"exit_rules"`
	ConfirmRules  []CompiledRule  `json:"confirm_rules"`
	InvalidRules  []CompiledRule  `json:"invalid_rules"`
	ContextRules  []ContextRule   `json:"context_rules"`
	HoldingPeriod string          `json:"holding_period"`
	MaxDrawdown   float64         `json:"max_drawdown"`
	FeatureList   []string        `json:"feature_list"` // 所有需要的特征
}

// Compiler 规则编译器
type Compiler struct {
	// 编译缓存
	cache map[string]*CompiledSchema
}

// NewCompiler 创建编译器
func NewCompiler() *Compiler {
	return &Compiler{
		cache: make(map[string]*CompiledSchema),
	}
}

// Compile 编译 Schema 为可执行规则
func (c *Compiler) Compile(schema *ParadigmSchema) (*CompiledSchema, error) {
	if err := schema.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	compiled := &CompiledSchema{
		ID:            schema.ID,
		Version:       schema.Version,
		EntryRules:    make([]CompiledRule, 0),
		ExitRules:     make([]CompiledRule, 0),
		ConfirmRules:  make([]CompiledRule, 0),
		InvalidRules:  make([]CompiledRule, 0),
		ContextRules:  schema.ContextRules,
		HoldingPeriod: schema.HoldingPeriod,
		MaxDrawdown:   schema.MaxDrawdown,
		FeatureList:   make([]string, 0),
	}

	// 收集所有特征
	featureSet := make(map[string]bool)

	// 编译规则
	for _, rule := range schema.Rules {
		compiledRule := CompiledRule{
			ID:        rule.ID,
			Type:      rule.Type,
			Side:      rule.Side,
			Feature:   rule.FeatureName,
			Operator:  rule.Operator,
			Required:  rule.Required,
			Weight:    rule.Weight,
			FuncName:  c.getFuncName(rule.Operator),
		}

		// 设置阈值
		if len(rule.Thresholds) > 0 {
			compiledRule.Threshold = rule.Thresholds[0]
		}
		if len(rule.Thresholds) > 1 {
			compiledRule.Threshold2 = rule.Thresholds[1]
		}

		// 添加到对应分组
		switch rule.Type {
		case TypeEntry:
			compiled.EntryRules = append(compiled.EntryRules, compiledRule)
		case TypeExitProfit, TypeExitLoss:
			compiled.ExitRules = append(compiled.ExitRules, compiledRule)
		case TypeConfirmation:
			compiled.ConfirmRules = append(compiled.ConfirmRules, compiledRule)
		case TypeInvalidation:
			compiled.InvalidRules = append(compiled.InvalidRules, compiledRule)
		}

		// 收集特征
		featureSet[rule.FeatureName] = true
	}

	// 填充特征列表
	for f := range featureSet {
		compiled.FeatureList = append(compiled.FeatureList, f)
	}

	// 缓存编译结果
	cacheKey := fmt.Sprintf("%s:v%d", schema.ID, schema.Version)
	c.cache[cacheKey] = compiled

	return compiled, nil
}

// getFuncName 获取运算符对应的函数名
func (c *Compiler) getFuncName(op RuleOperator) string {
	funcNames := map[RuleOperator]string{
		OpGreaterThan:  "gt",
		OpLessThan:     "lt",
		OpEqual:        "eq",
		OpBetween:      "between",
		OpCrossAbove:   "cross_above",
		OpCrossBelow:   "cross_below",
		OpAbove:        "above",
		OpBelow:        "below",
		OpMaxDrawdown:  "max_dd",
		OpMaxDuration:  "max_duration",
	}
	return funcNames[op]
}

// EvaluateRule 执行规则评估 (单次)
func (cr *CompiledRule) Evaluate(featureValue float64, prevValue float64) bool {
	switch cr.Operator {
	case OpGreaterThan:
		return featureValue > cr.Threshold
	case OpLessThan:
		return featureValue < cr.Threshold
	case OpEqual:
		return featureValue == cr.Threshold
	case OpBetween:
		return featureValue >= cr.Threshold && featureValue <= cr.Threshold2
	case OpCrossAbove:
		return prevValue < cr.Threshold && featureValue >= cr.Threshold
	case OpCrossBelow:
		return prevValue > cr.Threshold && featureValue <= cr.Threshold
	case OpAbove:
		return featureValue > cr.Threshold
	case OpBelow:
		return featureValue < cr.Threshold
	case OpMaxDrawdown:
		return true // 需要历史数据, 单独处理
	case OpMaxDuration:
		return true // 需要历史数据, 单独处理
	default:
		return false
	}
}

// EvaluateEntry 评估入场条件
func (cs *CompiledSchema) EvaluateEntry(features map[string]float64, prevFeatures map[string]float64) (bool, float64) {
	if len(cs.EntryRules) == 0 {
		return false, 0
	}

	// 检查所有必须的入场规则
	for _, rule := range cs.EntryRules {
		if rule.Required {
			curVal := features[rule.Feature]
			prevVal := 0.0
			if prevFeatures != nil {
				prevVal = prevFeatures[rule.Feature]
			}

			if !rule.Evaluate(curVal, prevVal) {
				return false, 0
			}
		}
	}

	// 计算确认规则得分 (0-1)
	confirmScore := 0.0
	if len(cs.ConfirmRules) > 0 {
		totalWeight := 0.0
		matchedWeight := 0.0

		for _, rule := range cs.ConfirmRules {
			curVal := features[rule.Feature]
			prevVal := 0.0
			if prevFeatures != nil {
				prevVal = prevFeatures[rule.Feature]
			}

			weight := rule.Weight
			if weight == 0 {
				weight = 0.1
			}
			totalWeight += weight

			if rule.Evaluate(curVal, prevVal) {
				matchedWeight += weight
			}
		}

		if totalWeight > 0 {
			confirmScore = matchedWeight / totalWeight
		}
	}

	return true, confirmScore
}

// EvaluateExit 评估出场条件
func (cs *CompiledSchema) EvaluateExit(features map[string]float64, prevFeatures map[string]float64, positionPnL float64) (bool, string) {
	for _, rule := range cs.ExitRules {
		curVal := features[rule.Feature]
		prevVal := 0.0
		if prevFeatures != nil {
			prevVal = prevFeatures[rule.Feature]
		}

		triggered := rule.Evaluate(curVal, prevVal)

		// 检查止损: 基于持仓收益
		if rule.Type == TypeExitLoss && rule.Operator == OpMaxDrawdown {
			if positionPnL <= -rule.Threshold {
				return true, "stop_loss"
			}
		} else if rule.Type == TypeExitLoss && triggered {
			return true, "stop_loss"
		}

		// 检查止盈
		if rule.Type == TypeExitProfit && triggered {
			return true, "take_profit"
		}
	}

	return false, ""
}

// CheckInvalidation 检查失效条件
func (cs *CompiledSchema) CheckInvalidation(features map[string]float64) (bool, []string) {
	var reasons []string
	for _, rule := range cs.InvalidRules {
		curVal := features[rule.Feature]
		if rule.Evaluate(curVal, 0) {
			reasons = append(reasons, rule.ID)
		}
	}
	return len(reasons) > 0, reasons
}

// ClearCache 清除编译缓存
func (c *Compiler) ClearCache() {
	c.cache = make(map[string]*CompiledSchema)
}

// GetCached 获取缓存的编译结果
func (c *Compiler) GetCached(schema *ParadigmSchema) (*CompiledSchema, bool) {
	cacheKey := fmt.Sprintf("%s:v%d", schema.ID, schema.Version)
	cached, ok := c.cache[cacheKey]
	return cached, ok
}

// Describe 描述编译后的 Schema (用于日志/调试)
func (cs *CompiledSchema) Describe() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Schema: %s v%d\n", cs.ID, cs.Version))
	sb.WriteString(fmt.Sprintf("Features: %s\n", strings.Join(cs.FeatureList, ", ")))

	sb.WriteString("\nEntry Rules:\n")
	for _, r := range cs.EntryRules {
		sb.WriteString(fmt.Sprintf("  [%s] %s %s(%.2f)\n", r.ID, r.Feature, r.FuncName, r.Threshold))
	}

	sb.WriteString("\nExit Rules:\n")
	for _, r := range cs.ExitRules {
		sb.WriteString(fmt.Sprintf("  [%s] %s %s(%.2f)\n", r.ID, r.Feature, r.FuncName, r.Threshold))
	}

	if len(cs.ConfirmRules) > 0 {
		sb.WriteString("\nConfirm Rules:\n")
		for _, r := range cs.ConfirmRules {
			sb.WriteString(fmt.Sprintf("  [%s] %s %s(%.2f) w=%.2f\n", r.ID, r.Feature, r.FuncName, r.Threshold, r.Weight))
		}
	}

	if len(cs.InvalidRules) > 0 {
		sb.WriteString("\nInvalidation Rules:\n")
		for _, r := range cs.InvalidRules {
			sb.WriteString(fmt.Sprintf("  [%s] %s %s(%.2f)\n", r.ID, r.Feature, r.FuncName, r.Threshold))
		}
	}

	return sb.String()
}
