package paradigms

import (
	"testing"
)

// ============================================================================
// Compiler 测试
// ============================================================================

func TestNewCompiler(t *testing.T) {
	c := NewCompiler()
	if c == nil {
		t.Fatal("NewCompiler should return a non-nil compiler")
	}
}

func TestCompile_ValidSchema(t *testing.T) {
	c := NewCompiler()
	schema := createValidSchema()

	compiled, err := c.Compile(schema)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if compiled.ID != schema.ID {
		t.Errorf("expected ID %s, got %s", schema.ID, compiled.ID)
	}
	if compiled.Version != schema.Version {
		t.Errorf("expected version %d, got %d", schema.Version, compiled.Version)
	}
	if len(compiled.EntryRules) != 2 {
		t.Errorf("expected 2 entry rules, got %d", len(compiled.EntryRules))
	}
	if len(compiled.ExitRules) != 2 {
		t.Errorf("expected 2 exit rules, got %d", len(compiled.ExitRules))
	}
	if len(compiled.FeatureList) == 0 {
		t.Error("FeatureList should not be empty")
	}
	if !containsFeature(compiled.FeatureList, "MA5") || !containsFeature(compiled.FeatureList, "price.close") {
		t.Error("FeatureList should include all required features")
	}
}

func TestCompile_InvalidSchema(t *testing.T) {
	c := NewCompiler()
	schema := createValidSchema()
	schema.Rules[0].FeatureName = "nonexistent.feature"

	_, err := c.Compile(schema)
	if err == nil {
		t.Error("expected error for invalid schema")
	}
}

func TestCompiledRule_Evaluate(t *testing.T) {
	tests := []struct {
		name      string
		rule      CompiledRule
		value     float64
		prevValue float64
		expected  bool
	}{
		{
			name:      "gt: value > threshold",
			rule:      CompiledRule{Operator: OpGreaterThan, Threshold: 10.0},
			value:     11.0,
			prevValue: 0,
			expected:  true,
		},
		{
			name:      "gt: value <= threshold",
			rule:      CompiledRule{Operator: OpGreaterThan, Threshold: 10.0},
			value:     9.0,
			prevValue: 0,
			expected:  false,
		},
		{
			name:      "lt: value < threshold",
			rule:      CompiledRule{Operator: OpLessThan, Threshold: 10.0},
			value:     9.0,
			prevValue: 0,
			expected:  true,
		},
		{
			name:      "lt: value >= threshold",
			rule:      CompiledRule{Operator: OpLessThan, Threshold: 10.0},
			value:     11.0,
			prevValue: 0,
			expected:  false,
		},
		{
			name:      "eq: value == threshold",
			rule:      CompiledRule{Operator: OpEqual, Threshold: 10.0},
			value:     10.0,
			prevValue: 0,
			expected:  true,
		},
		{
			name:      "eq: value != threshold",
			rule:      CompiledRule{Operator: OpEqual, Threshold: 10.0},
			value:     10.1,
			prevValue: 0,
			expected:  false,
		},
		{
			name:      "between: value in range",
			rule:      CompiledRule{Operator: OpBetween, Threshold: 9.0, Threshold2: 11.0},
			value:     10.0,
			prevValue: 0,
			expected:  true,
		},
		{
			name:      "between: value out of range",
			rule:      CompiledRule{Operator: OpBetween, Threshold: 9.0, Threshold2: 11.0},
			value:     12.0,
			prevValue: 0,
			expected:  false,
		},
		{
			name:      "cross_above: prev < threshold <= current",
			rule:      CompiledRule{Operator: OpCrossAbove, Threshold: 10.0},
			value:     10.5,
			prevValue: 9.5,
			expected:  true,
		},
		{
			name:      "cross_above: already above",
			rule:      CompiledRule{Operator: OpCrossAbove, Threshold: 10.0},
			value:     11.0,
			prevValue: 10.5,
			expected:  false,
		},
		{
			name:      "cross_below: prev > threshold >= current",
			rule:      CompiledRule{Operator: OpCrossBelow, Threshold: 10.0},
			value:     9.5,
			prevValue: 10.5,
			expected:  true,
		},
		{
			name:      "cross_below: already below",
			rule:      CompiledRule{Operator: OpCrossBelow, Threshold: 10.0},
			value:     9.0,
			prevValue: 9.5,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rule.Evaluate(tt.value, tt.prevValue)
			if result != tt.expected {
				t.Errorf("Evaluate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCompiledSchema_EvaluateEntry(t *testing.T) {
	schema := createValidSchema()
	c := NewCompiler()
	compiled, err := c.Compile(schema)
	if err != nil {
		t.Fatal(err)
	}

	// 场景1: 所有条件满足
	features := map[string]float64{
		"MA5":         10.5,
		"price.close": 10.5,
	}
	prevFeatures := map[string]float64{
		"MA5":         9.5,
		"price.close": 9.5,
	}

	passed, score := compiled.EvaluateEntry(features, prevFeatures)
	if !passed {
		t.Error("expected entry to pass when all conditions met")
	}
	if score < 0 {
		t.Error("score should be non-negative")
	}

	// 场景2: 条件不满足
	features["price.close"] = 9.5 // 收盘价低于10
	passed, _ = compiled.EvaluateEntry(features, prevFeatures)
	if passed {
		t.Error("expected entry to fail when condition not met")
	}
}

func TestCompiledSchema_EvaluateExit(t *testing.T) {
	schema := createValidSchema()
	c := NewCompiler()
	compiled, err := c.Compile(schema)
	if err != nil {
		t.Fatal(err)
	}

	// 场景1: 止盈触发
	features := map[string]float64{
		"price.close": 13.0,
	}
	triggered, reason := compiled.EvaluateExit(features, nil, 0.0)
	if !triggered || reason != "take_profit" {
		t.Errorf("expected take_profit trigger, got triggered=%v reason=%s", triggered, reason)
	}

	// 场景2: 止损触发
	features["price.close"] = 8.5
	triggered, reason = compiled.EvaluateExit(features, nil, -0.05)
	if !triggered || reason != "stop_loss" {
		t.Errorf("expected stop_loss trigger, got triggered=%v reason=%s", triggered, reason)
	}

	// 场景3: 未触发
	features["price.close"] = 10.5
	triggered, reason = compiled.EvaluateExit(features, nil, 0.02)
	if triggered {
		t.Error("expected no trigger")
	}
}

func TestCompiledSchema_CheckInvalidation(t *testing.T) {
	schema := createValidSchema()
	// 添加失效规则
	schema.Rules = append(schema.Rules, Rule{
		ID:          "inv-1",
		Type:        TypeInvalidation,
		Side:        SideSell,
		FeatureName: "price.close",
		Operator:    OpLessThan,
		Thresholds:  []float64{8.0},
		Required:    true,
		Description: "收盘价跌破8元时失效",
	})

	c := NewCompiler()
	compiled, err := c.Compile(schema)
	if err != nil {
		t.Fatal(err)
	}

	// 场景1: 未触发失效
	features := map[string]float64{
		"price.close": 10.0,
	}
	invalid, reasons := compiled.CheckInvalidation(features)
	if invalid {
		t.Error("expected no invalidation")
	}
	if len(reasons) > 0 {
		t.Error("expected empty reasons")
	}

	// 场景2: 触发失效
	features["price.close"] = 7.0
	invalid, reasons = compiled.CheckInvalidation(features)
	if !invalid {
		t.Error("expected invalidation trigger")
	}
	if len(reasons) == 0 {
		t.Error("expected reasons for invalidation")
	}
}

func TestCompilerCache(t *testing.T) {
	c := NewCompiler()
	schema := createValidSchema()

	// 第一次编译
	compiled1, err := c.Compile(schema)
	if err != nil {
		t.Fatal(err)
	}

	// 检查缓存
	cached, ok := c.GetCached(schema)
	if !ok || cached != compiled1 {
		t.Error("expected to find cached compiled schema")
	}

	// 清除缓存
	c.ClearCache()
	cached, ok = c.GetCached(schema)
	if ok {
		t.Error("expected no cached schema after clear")
	}
}

func TestCompiledSchema_Describe(t *testing.T) {
	c := NewCompiler()
	schema := createValidSchema()
	compiled, err := c.Compile(schema)
	if err != nil {
		t.Fatal(err)
	}

	desc := compiled.Describe()
	if len(desc) == 0 {
		t.Error("Describe() should return non-empty string")
	}
}

// containsFeature 检查特征列表是否包含指定特征
func containsFeature(features []string, target string) bool {
	for _, f := range features {
		if f == target {
			return true
		}
	}
	return false
}
