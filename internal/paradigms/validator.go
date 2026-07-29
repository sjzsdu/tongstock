package paradigms

import (
	"fmt"
	"strings"
)

// ============================================================================
// Schema 验证器
// ============================================================================

// Validator 范式 Schema 验证器
type Validator struct {
	// 已知特征类型: indicator, price, volume, market
	KnownFeatures map[string]bool
	// 已知指标列表
	KnownIndicators map[string][]string // indicator -> [params]
	// 已知上下文值
	ValidContextValues map[ContextKey]map[string]bool
}

// NewValidator 创建验证器
func NewValidator() *Validator {
	v := &Validator{
		KnownFeatures: map[string]bool{
			"price.open":      true,
			"price.high":      true,
			"price.low":       true,
			"price.close":     true,
			"price.volume":    true,
			"price.amount":    true,
			"indicator.MA5":   true,
			"indicator.MA10":  true,
			"indicator.MA20":  true,
			"indicator.MA60":  true,
			"indicator.EMA12": true,
			"indicator.EMA26": true,
			"indicator.MACD":  true,
			"indicator.RSI6":  true,
			"indicator.RSI12": true,
			"indicator.KDJ":   true,
			"indicator.Boll":  true,
			"indicator.ATR":   true,
		},
		KnownIndicators: map[string][]string{
			"MA5":  {"period", "method"},
			"MA10": {"period", "method"},
			"MA20": {"period", "method"},
			"MA60": {"period", "method"},
			"EMA12": {"period"},
			"EMA26": {"period"},
			"MACD":  {"fast", "slow", "signal"},
			"RSI6":  {"period"},
			"RSI12": {"period"},
			"KDJ":   {"n", "m1", "m2"},
			"Boll":  {"period", "std"},
			"ATR":   {"period"},
		},
		ValidContextValues: map[ContextKey]map[string]bool{
			ContextMarketCap: {
				"small": true, "mid": true, "large": true, "mega": true,
			},
			ContextShareholderDominant: {
				"retail": true, "hot_money": true, "foreign": true,
				"institutional": true, "state": true, "mixed": true,
			},
			ContextActivity: {
				"active": true, "normal": true, "quiet": true,
			},
			ContextTrend: {
				"uptrend": true, "downtrend": true, "range": true, "volatile": true,
			},
			ContextVolatility: {
				"low": true, "medium": true, "high": true,
			},
			ContextLiquidity: {
				"low": true, "medium": true, "high": true,
			},
		},
	}
	return v
}

// ValidateSchema 验证完整的 Schema
func (v *Validator) ValidateSchema(schema *ParadigmSchema) []ValidationError {
	var errors []ValidationError

	// 1. 验证基本结构
	if schema.ID == "" {
		errors = append(errors, ValidationError{
			Level:   "error",
			Field:   "id",
			Message: "schema id is required",
		})
	}
	if schema.Name == "" {
		errors = append(errors, ValidationError{
			Level:   "error",
			Field:   "name",
			Message: "schema name is required",
		})
	}

	// 2. 验证特征定义
	featureNames := make(map[string]bool)
	for i, f := range schema.Features {
		if f.Name == "" {
			errors = append(errors, ValidationError{
				Level:   "error",
				Field:   "features",
				Message: fmt.Sprintf("feature[%d] has empty name", i),
			})
			continue
		}
		if featureNames[f.Name] {
			errors = append(errors, ValidationError{
				Level:   "error",
				Field:   "features",
				Message: fmt.Sprintf("duplicate feature name: %s", f.Name),
			})
		}
		featureNames[f.Name] = true

		// 验证特征类型
		validTypes := map[string]bool{
			"indicator": true,
			"price":     true,
			"volume":    true,
			"market":    true,
		}
		if !validTypes[f.Type] {
			errors = append(errors, ValidationError{
				Level:   "warning",
				Field:   "features",
				Message: fmt.Sprintf("feature %s has unknown type: %s", f.Name, f.Type),
			})
		}
	}

	// 3. 验证规则
	for i, rule := range schema.Rules {
		ruleErrors := v.validateRule(&rule, featureNames)
		for _, err := range ruleErrors {
			err.Field = "rules"
			errors = append(errors, err)
		}
		_ = i // 避免未使用警告
	}

	// 4. 验证上下文规则
	for i, cr := range schema.ContextRules {
		contextErrors := v.validateContextRule(&cr)
		for _, err := range contextErrors {
			err.Field = "context_rules"
			errors = append(errors, err)
		}
		_ = i
	}

	// 5. 检查一致性 (非错误, 仅警告)
	if len(schema.GetRulesByType(TypeEntry)) == 0 {
		errors = append(errors, ValidationError{
			Level:   "warning",
			Field:   "rules",
			Message: "no entry rules defined",
		})
	}

	return errors
}

// validateRule 验证单条规则
func (v *Validator) validateRule(rule *Rule, featureNames map[string]bool) []ValidationError {
	var errors []ValidationError

	if rule.ID == "" {
		errors = append(errors, ValidationError{
			Level:   "error",
			Message: "rule id is required",
		})
	}

	// 验证规则类型
	if !ValidRuleTypes[rule.Type] {
		errors = append(errors, ValidationError{
			Level:   "error",
			RuleID:  rule.ID,
			Message: fmt.Sprintf("invalid rule type: %s", rule.Type),
		})
	}

	// 验证运算符
	if !ValidOperators[rule.Operator] {
		errors = append(errors, ValidationError{
			Level:   "error",
			RuleID:  rule.ID,
			Message: fmt.Sprintf("invalid operator: %s", rule.Operator),
		})
	}

	// 验证特征名
	if !featureNames[rule.FeatureName] {
		errors = append(errors, ValidationError{
			Level:   "error",
			RuleID:  rule.ID,
			Message: fmt.Sprintf("rule references undefined feature: %s", rule.FeatureName),
		})
	}

	// 验证阈值
	switch rule.Operator {
	case OpBetween:
		if len(rule.Thresholds) != 2 {
			errors = append(errors, ValidationError{
				Level:   "error",
				RuleID:  rule.ID,
				Message: "between operator requires exactly 2 thresholds",
			})
		} else if rule.Thresholds[0] >= rule.Thresholds[1] {
			errors = append(errors, ValidationError{
				Level:   "error",
				RuleID:  rule.ID,
				Message: "between thresholds must be ascending",
			})
		}
	default:
		if len(rule.Thresholds) != 1 {
			errors = append(errors, ValidationError{
				Level:   "error",
				RuleID:  rule.ID,
				Message: fmt.Sprintf("operator %s requires exactly 1 threshold", rule.Operator),
			})
		}
	}

	// 验证确认规则的权重
	if rule.Type == TypeConfirmation {
		if rule.Weight < 0 || rule.Weight > 1 {
			errors = append(errors, ValidationError{
				Level:   "error",
				RuleID:  rule.ID,
				Message: "confirmation rule weight must be between 0 and 1",
			})
		}
	}

	// 验证入场/出场规则必须是 required
	if (rule.Type == TypeEntry || rule.Type == TypeExitProfit || rule.Type == TypeExitLoss) && !rule.Required {
		errors = append(errors, ValidationError{
			Level:   "warning",
			RuleID:  rule.ID,
			Message: "entry/exit rules should be marked as required",
		})
	}

	return errors
}

// validateContextRule 验证上下文规则
func (v *Validator) validateContextRule(cr *ContextRule) []ValidationError {
	var errors []ValidationError

	if !ValidContextKeys[cr.Key] {
		errors = append(errors, ValidationError{
			Level:   "error",
			Message: fmt.Sprintf("invalid context key: %s", cr.Key),
		})
		return errors
	}

	// 验证上下文值
	validValues, ok := v.ValidContextValues[cr.Key]
	if ok {
		for _, val := range cr.Values {
			if !validValues[val] {
				errors = append(errors, ValidationError{
					Level:   "warning",
					Message: fmt.Sprintf("unknown value '%s' for context key '%s' (valid: %s)",
						val, cr.Key, strings.Join(getKeys(validValues), ", ")),
				})
			}
		}
	}

	return errors
}

// getKeys 获取 map 的所有 key
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ValidateFeatureReferences 验证特征引用一致性
func (v *Validator) ValidateFeatureReferences(schema *ParadigmSchema) []ValidationError {
	var errors []ValidationError
	definedFeatures := make(map[string]bool)

	for _, f := range schema.Features {
		definedFeatures[f.Name] = true

		// 检查依赖是否存在
		for _, dep := range f.Dependency {
			if !definedFeatures[dep] {
				errors = append(errors, ValidationError{
					Level:   "warning",
					Field:   "features",
					Message: fmt.Sprintf("feature %s depends on undefined feature: %s", f.Name, dep),
				})
			}
		}
	}

	// 检查规则引用的特征
	for _, r := range schema.Rules {
		if !definedFeatures[r.FeatureName] {
			errors = append(errors, ValidationError{
				Level:   "error",
				Field:   "rules",
				RuleID:  r.ID,
				Message: fmt.Sprintf("rule references undefined feature: %s", r.FeatureName),
			})
		}
	}

	return errors
}

// ValidateExecutability 验证可执行性 (无严重错误)
func (v *Validator) ValidateExecutability(schema *ParadigmSchema) error {
	errors := v.ValidateSchema(schema)

	var criticalErrors []string
	for _, e := range errors {
		if e.Level == "error" {
			criticalErrors = append(criticalErrors, e.String())
		}
	}

	if len(criticalErrors) > 0 {
		return fmt.Errorf("schema has %d critical errors:\n%s",
			len(criticalErrors),
			strings.Join(criticalErrors, "\n"))
	}

	return nil
}
