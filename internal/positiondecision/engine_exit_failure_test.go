package positiondecision

import (
	"errors"
	"strings"
	"testing"
)

func TestApplyExitFailureIsExplicitAndNotHold(t *testing.T) {
	d := &Decision{Action: "watch", Priority: "medium", Executable: true}
	applyExitFailure(d, errors.New("表达式求值失败"))

	if d.Action != "insufficient_data" {
		t.Fatalf("action = %q, want insufficient_data", d.Action)
	}
	if d.Priority != "critical" {
		t.Fatalf("priority = %q, want critical", d.Priority)
	}
	if d.Executable {
		t.Fatal("executable = true, want false")
	}
	if !strings.Contains(d.Constraint, "卖出规则执行失败") || !strings.Contains(d.Constraint, "表达式求值失败") {
		t.Fatalf("constraint = %q, want failure detail", d.Constraint)
	}
	if len(d.Facts) != 1 || d.Facts[0].Kind != "exit_error" || d.Facts[0].Passed {
		t.Fatalf("facts = %+v, want a failed exit_error fact", d.Facts)
	}
	if !strings.Contains(d.Facts[0].Detail, "表达式求值失败") {
		t.Fatalf("fact detail = %q, want original error", d.Facts[0].Detail)
	}
}

func TestApplyExitFailureKeepsExistingConstraint(t *testing.T) {
	d := &Decision{Constraint: "当前停牌，无法卖出"}
	applyExitFailure(d, errors.New("boom"))

	if !strings.Contains(d.Constraint, "当前停牌") || !strings.Contains(d.Constraint, "卖出规则执行失败") {
		t.Fatalf("constraint = %q, want both existing and failure reasons", d.Constraint)
	}
}
