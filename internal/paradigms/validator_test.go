package paradigms

import (
	"testing"
)

// ============================================================================
// Validator 测试
// ============================================================================

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	if v == nil {
		t.Fatal("NewValidator should return a non-nil validator")
	}
	if len(v.KnownFeatures) == 0 {
		t.Error("KnownFeatures should not be empty")
	}
}

func TestValidateSchema_Valid(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()

	errors := v.ValidateSchema(schema)
	for _, e := range errors {
		if e.Level == "error" {
			t.Errorf("unexpected error: %s", e.String())
		}
	}
}

func TestValidateSchema_MissingID(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()
	schema.ID = ""

	errors := v.ValidateSchema(schema)
	found := false
	for _, e := range errors {
		if e.Level == "error" && e.Field == "id" {
			found = true
		}
	}
	if !found {
		t.Error("should find error for missing ID")
	}
}

func TestValidateSchema_MissingName(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()
	schema.Name = ""

	errors := v.ValidateSchema(schema)
	found := false
	for _, e := range errors {
		if e.Level == "error" && e.Field == "name" {
			found = true
		}
	}
	if !found {
		t.Error("should find error for missing name")
	}
}

func TestValidateSchema_DuplicateFeature(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()
	schema.Features = append(schema.Features, FeatureDefinition{
		Name: schema.Features[0].Name, // 重复的特征名
		Type: "price",
	})

	errors := v.ValidateSchema(schema)
	found := false
	for _, e := range errors {
		if e.Level == "error" && e.Field == "features" {
			found = true
		}
	}
	if !found {
		t.Error("should find error for duplicate feature")
	}
}

func TestValidateSchema_InvalidOperator(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()
	schema.Rules[0].Operator = "invalid_operator"

	errors := v.ValidateSchema(schema)
	found := false
	for _, e := range errors {
		if e.Level == "error" && e.Field == "rules" {
			found = true
		}
	}
	if !found {
		t.Error("should find error for invalid operator")
	}
}

func TestValidateSchema_UndefinedFeature(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()
	schema.Rules[0].FeatureName = "nonexistent.feature"

	errors := v.ValidateSchema(schema)
	found := false
	for _, e := range errors {
		if e.Level == "error" && e.Field == "rules" {
			found = true
		}
	}
	if !found {
		t.Error("should find error for undefined feature")
	}
}

func TestValidateSchema_MissingThreshold(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()
	// OpBetween 需要 2 个阈值
	schema.Rules[0].Operator = OpBetween
	schema.Rules[0].Thresholds = []float64{10.0} // 只有一个阈值

	errors := v.ValidateSchema(schema)
	found := false
	for _, e := range errors {
		if e.Level == "error" && e.Field == "rules" {
			found = true
		}
	}
	if !found {
		t.Error("should find error for missing threshold")
	}
}

func TestValidateSchema_BadContextValue(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()
	schema.ContextRules = append(schema.ContextRules, ContextRule{
		Key:    ContextTrend,
		Values: []string{"invalid_trend_value"},
	})

	errors := v.ValidateSchema(schema)
	found := false
	for _, e := range errors {
		if e.Level == "warning" && e.Field == "context_rules" {
			found = true
		}
	}
	if !found {
		t.Error("should find warning for unknown context value")
	}
}

func TestValidateSchema_NoEntryRule(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()
	// 移除所有入场规则
	var newRules []Rule
	for _, r := range schema.Rules {
		if r.Type != TypeEntry {
			newRules = append(newRules, r)
		}
	}
	schema.Rules = newRules

	errors := v.ValidateSchema(schema)
	found := false
	for _, e := range errors {
		if e.Level == "warning" && e.Field == "rules" {
			found = true
		}
	}
	if !found {
		t.Error("should find warning for missing entry rule")
	}
}

func TestValidateFeatureReferences(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()

	// 添加一个依赖不存在特征的特征
	schema.Features = append(schema.Features, FeatureDefinition{
		Name:       "composite.feature",
		Type:       "indicator",
		Dependency: []string{"nonexistent.dep"},
	})

	errors := v.ValidateFeatureReferences(schema)
	found := false
	for _, e := range errors {
		if e.Level == "warning" {
			found = true
		}
	}
	if !found {
		t.Error("should find warning for undefined feature dependency")
	}
}

func TestValidateExecutability_Valid(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()

	err := v.ValidateExecutability(schema)
	if err != nil {
		t.Errorf("expected valid schema, got error: %v", err)
	}
}

func TestValidateExecutability_Invalid(t *testing.T) {
	v := NewValidator()
	schema := createValidSchema()
	schema.Rules[0].FeatureName = "nonexistent.feature"

	err := v.ValidateExecutability(schema)
	if err == nil {
		t.Error("expected error for invalid schema")
	}
}
