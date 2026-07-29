package paradigms

import (
	"testing"
	"time"
)

// ============================================================================
// Schema 测试
// ============================================================================

func TestNewParadigmSchema(t *testing.T) {
	schema := NewParadigmSchema("test-001", "测试范式")

	if schema.ID != "test-001" {
		t.Errorf("expected ID test-001, got %s", schema.ID)
	}
	if schema.Name != "测试范式" {
		t.Errorf("expected name '测试范式', got %s", schema.Name)
	}
	if schema.Version != 1 {
		t.Errorf("expected initial version 1, got %d", schema.Version)
	}
	// CreatedAt and UpdatedAt should be very close (within 1 millisecond)
	diff := schema.UpdatedAt.Sub(schema.CreatedAt)
	if diff.Milliseconds() > 1 {
		t.Errorf("CreatedAt and UpdatedAt should be very close, got diff: %v", diff)
	}
}

func TestRuleIsValid(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{
			name:    "valid rule",
			rule:    Rule{ID: "r1", Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: OpGreaterThan, Thresholds: []float64{10.0}, Required: true},
			wantErr: false,
		},
		{
			name:    "missing ID",
			rule:    Rule{Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: OpGreaterThan, Thresholds: []float64{10.0}},
			wantErr: true,
		},
		{
			name:    "invalid type",
			rule:    Rule{ID: "r1", Type: "invalid_type", Side: SideBuy, FeatureName: "price.close", Operator: OpGreaterThan, Thresholds: []float64{10.0}},
			wantErr: true,
		},
		{
			name:    "invalid operator",
			rule:    Rule{ID: "r1", Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: "invalid_op", Thresholds: []float64{10.0}},
			wantErr: true,
		},
		{
			name:    "missing feature name",
			rule:    Rule{ID: "r1", Type: TypeEntry, Side: SideBuy, Operator: OpGreaterThan, Thresholds: []float64{10.0}},
			wantErr: true,
		},
		{
			name:    "between with two thresholds",
			rule:    Rule{ID: "r1", Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: OpBetween, Thresholds: []float64{10.0, 20.0}},
			wantErr: false,
		},
		{
			name:    "between with wrong thresholds",
			rule:    Rule{ID: "r1", Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: OpBetween, Thresholds: []float64{20.0, 10.0}},
			wantErr: true,
		},
		{
			name:    "single threshold for between",
			rule:    Rule{ID: "r1", Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: OpBetween, Thresholds: []float64{10.0}},
			wantErr: true,
		},
		{
			name:    "single threshold for gt",
			rule:    Rule{ID: "r1", Type: TypeEntry, Side: SideBuy, FeatureName: "price.close", Operator: OpGreaterThan, Thresholds: []float64{10.0}},
			wantErr: false,
		},
		{
			name:    "confirmation weight out of range",
			rule:    Rule{ID: "r1", Type: TypeConfirmation, Side: SideBuy, FeatureName: "price.close", Operator: OpGreaterThan, Thresholds: []float64{10.0}, Weight: 1.5},
			wantErr: true,
		},
		{
			name:    "confirmation weight in range",
			rule:    Rule{ID: "r1", Type: TypeConfirmation, Side: SideBuy, FeatureName: "price.close", Operator: OpGreaterThan, Thresholds: []float64{10.0}, Weight: 0.8},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.IsValid()
			if (err != nil) != tt.wantErr {
				t.Errorf("IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContextRuleIsValid(t *testing.T) {
	tests := []struct {
		name    string
		rule    ContextRule
		wantErr bool
	}{
		{
			name:    "valid context rule",
			rule:    ContextRule{Key: ContextTrend, Values: []string{"uptrend", "range"}},
			wantErr: false,
		},
		{
			name:    "invalid context key",
			rule:    ContextRule{Key: "invalid_key", Values: []string{"uptrend"}},
			wantErr: true,
		},
		{
			name:    "empty values",
			rule:    ContextRule{Key: ContextTrend, Values: []string{}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.IsValid()
			if (err != nil) != tt.wantErr {
				t.Errorf("IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParadigmSchemaIsValid(t *testing.T) {
	// 创建一个有效 Schema
	schema := createValidSchema()
	if err := schema.IsValid(); err != nil {
		t.Errorf("expected valid schema, got error: %v", err)
	}
}

func TestParadigmSchemaMissingEntry(t *testing.T) {
	schema := createValidSchema()
	// 移除入场规则
	schema.Rules = nil
	if err := schema.IsValid(); err == nil {
		t.Error("expected error for schema without entry rules")
	}
}

func TestParadigmSchemaInvalidHoldingPeriod(t *testing.T) {
	schema := createValidSchema()
	schema.HoldingPeriod = "invalid"
	if err := schema.IsValid(); err == nil {
		t.Error("expected error for invalid holding period")
	}
}

func TestParadigmSchemaUndefinedFeature(t *testing.T) {
	schema := createValidSchema()
	// 规则引用不存在的特征
	schema.Rules[0].FeatureName = "nonexistent.feature"
	if err := schema.IsValid(); err == nil {
		t.Error("expected error for undefined feature reference")
	}
}

func TestCreateVersion(t *testing.T) {
	schema := createValidSchema()
	originalVersion := schema.Version
	originalUpdatedAt := schema.UpdatedAt

	// 等待一段时间以确保时间戳不同
	time.Sleep(10 * time.Millisecond)

	newSchema := schema.CreateVersion("修改规则阈值")

	if newSchema.Version != originalVersion+1 {
		t.Errorf("expected version %d, got %d", originalVersion+1, newSchema.Version)
	}
	if newSchema.ParentVersion != originalVersion {
		t.Errorf("expected parent version %d, got %d", originalVersion, newSchema.ParentVersion)
	}
	if newSchema.ChangeReason != "修改规则阈值" {
		t.Errorf("expected change reason, got %s", newSchema.ChangeReason)
	}
	if newSchema.UpdatedAt.Equal(originalUpdatedAt) {
		t.Error("UpdatedAt should be different after CreateVersion")
	}
	// 深拷贝: 修改新 Schema 不影响原 Schema
	newSchema.Rules[0].Thresholds[0] = 20.0
	if schema.Rules[0].Thresholds[0] == 20.0 {
		t.Error("CreateVersion should create a deep copy")
	}
}

func TestGetRulesByType(t *testing.T) {
	schema := createValidSchema()

	entryRules := schema.GetRulesByType(TypeEntry)
	if len(entryRules) != 2 {
		t.Errorf("expected 2 entry rules, got %d", len(entryRules))
	}

	exitRules := schema.GetRulesByType(TypeExitProfit)
	if len(exitRules) != 1 {
		t.Errorf("expected 1 exit_profit rule, got %d", len(exitRules))
	}
}

func TestGetRulesBySide(t *testing.T) {
	schema := createValidSchema()

	buyRules := schema.GetRulesBySide(SideBuy)
	if len(buyRules) != 2 {
		t.Errorf("expected 2 buy rules, got %d", len(buyRules))
	}

	sellRules := schema.GetRulesBySide(SideSell)
	if len(sellRules) != 2 {
		t.Errorf("expected 2 sell rules, got %d", len(sellRules))
	}
}

func TestValidateExecutability(t *testing.T) {
	schema := createValidSchema()
	errors, err := schema.Validate()
	if err != nil {
		t.Fatal(err)
	}

	// 应该没有严重错误
	if schema.HasErrors(errors) {
		t.Error("should not have critical validation errors")
	}
}

func TestValidationErrorString(t *testing.T) {
	err := ValidationError{
		Level:   "error",
		Field:   "rules",
		RuleID:  "r1",
		Message: "feature not found",
	}

	str := err.String()
	if str == "" {
		t.Error("ValidationError.String() should not be empty")
	}
}

// createValidSchema 创建一个有效的测试 Schema
func createValidSchema() *ParadigmSchema {
	schema := NewParadigmSchema("test-001", "双均线金叉策略")
	schema.HoldingPeriod = "medium"
	schema.MaxDrawdown = 0.15

	// 添加特征
	schema.Features = []FeatureDefinition{
		{
			Name:        "MA5",
			Type:        "indicator",
			Calculation: "MA(close, 5)",
			Params:      map[string]float64{"period": 5},
			Description: "5日均线",
		},
		{
			Name:        "MA20",
			Type:        "indicator",
			Calculation: "MA(close, 20)",
			Params:      map[string]float64{"period": 20},
			Description: "20日均线",
		},
		{
			Name:        "price.close",
			Type:        "price",
			Description: "收盘价",
		},
	}

	// 添加上下文规则
	schema.ContextRules = []ContextRule{
		{Key: ContextTrend, Values: []string{"uptrend", "range"}},
	}

	// 添加规则
	schema.Rules = []Rule{
		{
			ID:          "entry-1",
			Type:        TypeEntry,
			Side:        SideBuy,
			FeatureName: "MA5",
			Operator:    OpCrossAbove,
			Thresholds:  []float64{10.0},
			Required:    true,
			Description: "5日均线上穿10元",
		},
		{
			ID:          "entry-2",
			Type:        TypeEntry,
			Side:        SideBuy,
			FeatureName: "price.close",
			Operator:    OpGreaterThan,
			Thresholds:  []float64{10.0},
			Required:    true,
			Description: "收盘价大于10元",
		},
		{
			ID:          "exit-1",
			Type:        TypeExitProfit,
			Side:        SideSell,
			FeatureName: "price.close",
			Operator:    OpGreaterThan,
			Thresholds:  []float64{12.0},
			Required:    true,
			Description: "收盘价大于12元止盈",
		},
		{
			ID:          "exit-2",
			Type:        TypeExitLoss,
			Side:        SideSell,
			FeatureName: "price.close",
			Operator:    OpLessThan,
			Thresholds:  []float64{9.0},
			Required:    true,
			Description: "收盘价小于9元止损",
		},
	}

	return schema
}
