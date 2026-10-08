package paradigms

import (
	"strings"
	"testing"

	"github.com/sjzsdu/tongstock/internal/methods"
)

func mustCompile(t *testing.T, c *methods.Candidate) *methods.CompiledMethod {
	t.Helper()
	compiled, diags, err := methods.Compile(c)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if compiled == nil || !compiled.IsExecutable() {
		t.Fatalf("compiled method not executable, diags=%v", diags)
	}
	return compiled
}

func buyParadigm(conds ...Condition) *Paradigm {
	return &Paradigm{
		ID:   "p-test",
		Name: "测试范式",
		Side: "buy",
		BuyConds: conds,
		Expectation: Expectation{
			HoldingPeriod: "3-5天",
			Confidence:    0,
		},
	}
}

func TestToCandidate_GTConstantValue(t *testing.T) {
	c, blockers, err := ToCandidate(buyParadigm(Condition{Indicator: "RSI14", Operator: "gt", Value: "70"}))
	if err != nil || len(blockers) > 0 {
		t.Fatalf("unexpected blockers %v err %v", blockers, err)
	}
	compiled := mustCompile(t, c)
	if compiled.SourceKind != "existing_paradigm" {
		t.Errorf("source kind = %q, want existing_paradigm", compiled.SourceKind)
	}
	if compiled.Scope.Universe != "universe_usable" {
		t.Errorf("universe = %q, want universe_usable", compiled.Scope.Universe)
	}
	if compiled.Holding.MinDays != 3 || compiled.Holding.MaxDays != 5 {
		t.Errorf("holding = %d-%d, want 3-5", compiled.Holding.MinDays, compiled.Holding.MaxDays)
	}
}

func TestToCandidate_GTIndicatorValue(t *testing.T) {
	c, blockers, err := ToCandidate(buyParadigm(Condition{Indicator: "close", Operator: "gt", Value: "MA20"}))
	if err != nil || len(blockers) > 0 {
		t.Fatalf("unexpected blockers %v err %v", blockers, err)
	}
	mustCompile(t, c)
	// 编译器只收集非内建指标作为 feature deps；close/ma20 均为内建。
	// 这里断言显式 deps 至少包含被引用的指标。
	joined := strings.Join(append([]string{}, c.FeatureDeps...), ",")
	if !strings.Contains(joined, "close") {
		t.Errorf("feature deps = %v, want at least close", c.FeatureDeps)
	}
}

func TestToCandidate_BetweenSplitsIntoAnd(t *testing.T) {
	c, blockers, err := ToCandidate(buyParadigm(Condition{Indicator: "rsi14", Operator: "between", Value: "30-70"}))
	if err != nil || len(blockers) > 0 {
		t.Fatalf("unexpected blockers %v err %v", blockers, err)
	}
	mustCompile(t, c)
	entryMap, ok := c.Entry.(map[string]any)
	if !ok {
		t.Fatalf("entry should be an and-node map, got %T", c.Entry)
	}
	children, ok := entryMap["children"].([]any)
	if !ok || len(children) != 2 {
		t.Fatalf("between should compile into and(gt,lt), got %v", c.Entry)
	}
}

func TestToCandidate_FailClosedOperators(t *testing.T) {
	cases := []Condition{
		{Indicator: "close", Operator: "cross_above", Value: "MA20"},
		{Indicator: "close", Operator: "cross_below", Value: "MA20"},
		{Indicator: "close", Operator: "near", Value: "MA20"},
		{Indicator: "close", Operator: "describe", Value: "放量突破"},
		{Indicator: "close", Operator: "gt", Value: "12%"},
		{Indicator: "macd.unknown_x", Operator: "gt", Value: "1"},
		{Indicator: "close", Operator: "gt", Value: "神秘值"},
	}
	for _, cond := range cases {
		p := buyParadigm(cond)
		_, blockers, err := ToCandidate(p)
		if err != nil {
			t.Fatalf("%v: unexpected err %v", cond, err)
		}
		if len(blockers) == 0 {
			t.Errorf("operator %q / value %q should be fail-closed blocked, got none", cond.Operator, cond.Value)
		}
	}
}

func TestToCandidate_SellSideBlocked(t *testing.T) {
	p := buyParadigm(Condition{Indicator: "close", Operator: "gt", Value: "10"})
	p.Side = "sell"
	_, blockers, err := ToCandidate(p)
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if len(blockers) == 0 {
		t.Fatal("sell side paradigm must be blocked")
	}
	if !strings.Contains(strings.Join(blockers, ";"), "buy") {
		t.Errorf("blocker should mention side, got %v", blockers)
	}
}

func TestToCandidate_EmptyBuyCondsBlocked(t *testing.T) {
	p := buyParadigm()
	_, blockers, err := ToCandidate(p)
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if len(blockers) == 0 {
		t.Fatal("empty buy_conditions must be blocked")
	}
}

func TestToCandidate_SellCondsMapping(t *testing.T) {
	p := buyParadigm(Condition{Indicator: "close", Operator: "gt", Value: "MA20"})
	p.SellConds = SellConditions{
		TakeProfit: []Condition{{Indicator: "close", Operator: "gt", Value: "10%"}},
		StopLoss:   []Condition{{Indicator: "close", Operator: "lt", Value: "-5%"}},
	}
	c, blockers, err := ToCandidate(p)
	if err != nil || len(blockers) > 0 {
		t.Fatalf("unexpected blockers %v err %v", blockers, err)
	}
	mustCompile(t, c)
	if c.TakeProfitPct == nil || *c.TakeProfitPct != 0.10 {
		t.Errorf("take profit pct = %v, want 0.10", c.TakeProfitPct)
	}
	if c.StopLossPct == nil || *c.StopLossPct != -0.05 {
		t.Errorf("stop loss pct = %v, want -0.05", c.StopLossPct)
	}
}

func TestToCandidate_SellCondsPriceLevelBecomesExitRule(t *testing.T) {
	p := buyParadigm(Condition{Indicator: "close", Operator: "gt", Value: "MA20"})
	p.SellConds = SellConditions{
		TakeProfit: []Condition{{Indicator: "close", Operator: "gt", Value: "12.30"}},
	}
	c, blockers, err := ToCandidate(p)
	if err != nil || len(blockers) > 0 {
		t.Fatalf("unexpected blockers %v err %v", blockers, err)
	}
	mustCompile(t, c)
	if c.Exit == nil {
		t.Fatal("price-level take profit should compile into an exit rule")
	}
	if c.TakeProfitPct != nil {
		t.Errorf("price-level take profit must not become pct, got %v", *c.TakeProfitPct)
	}
}

func TestToCandidate_MarketCapBoardFilter(t *testing.T) {
	p := buyParadigm(Condition{Indicator: "close", Operator: "gt", Value: "MA20"})
	p.Context.MarketCap = "mid"
	c, blockers, err := ToCandidate(p)
	if err != nil || len(blockers) > 0 {
		t.Fatalf("unexpected blockers %v err %v", blockers, err)
	}
	if len(c.BoardFilter) != 1 || c.BoardFilter[0] != "market_cap:mid" {
		t.Errorf("board filter = %v, want [market_cap:mid]", c.BoardFilter)
	}
}

func TestToCandidate_UnparseableHoldingPeriodFallsBackWithNote(t *testing.T) {
	p := buyParadigm(Condition{Indicator: "close", Operator: "gt", Value: "MA20"})
	p.Expectation.HoldingPeriod = "待验证"
	c, blockers, err := ToCandidate(p)
	if err != nil || len(blockers) > 0 {
		t.Fatalf("unexpected blockers %v err %v", blockers, err)
	}
	mustCompile(t, c)
	if c.HoldingMaxDays != defaultHoldingMaxDays || c.HoldingMinDays != defaultHoldingMinDays {
		t.Errorf("holding = %d-%d, want defaults", c.HoldingMinDays, c.HoldingMaxDays)
	}
	if !strings.Contains(c.Description, "待验证") {
		t.Errorf("fallback holding days must be noted in description, got %q", c.Description)
	}
}

func TestToCandidate_CompiledContentHashStable(t *testing.T) {
	p := buyParadigm(Condition{Indicator: "close", Operator: "gt", Value: "MA20"})
	c1, _, err := ToCandidate(p)
	if err != nil {
		t.Fatal(err)
	}
	c2, _, err := ToCandidate(p)
	if err != nil {
		t.Fatal(err)
	}
	h1 := mustCompile(t, c1).ContentHash
	h2 := mustCompile(t, c2).ContentHash
	if h1 != h2 {
		t.Errorf("content hash unstable: %s vs %s", h1, h2)
	}
}
