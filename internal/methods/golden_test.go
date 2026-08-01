package methods

import (
	"fmt"
	"math"
	"testing"
)

func demoBreakoutForTest() *Candidate {
	two := 2.0
	return &Candidate{
		Name:           "20 日突破 + RSI 健康",
		Description:    "收盘价创 20 日新高，RSI14 在 (50,72] 区间，不追高。",
		SourceKind:     "structured",
		Universe:       "universe_csi800",
		MarketState:    []string{"bull", "range"},
		FeatureDeps:    nil,
		PositionMode:   "pct_equity",
		PositionPct:    &[]float64{0.08}[0],
		HoldingMaxDays: 20,
		StopLossPct:    &[]float64{-0.06}[0],
		TakeProfitPct:  &two,
		Entry: map[string]any{
			"type": "and",
			"children": []any{
				map[string]any{
					"type": "compare", "op": "gt",
					"left":  map[string]any{"type": "indicator", "indicator": "close"},
					"right": map[string]any{"type": "indicator", "indicator": "ma20"},
				},
				map[string]any{
					"type": "compare", "op": "gt",
					"left":  map[string]any{"type": "indicator", "indicator": "rsi14"},
					"right": map[string]any{"type": "constant", "value": 50},
				},
				map[string]any{
					"type": "compare", "op": "lte",
					"left":  map[string]any{"type": "indicator", "indicator": "rsi14"},
					"right": map[string]any{"type": "constant", "value": 72},
				},
			},
		},
		Exit: map[string]any{
			"type": "compare", "op": "lt",
			"left":  map[string]any{"type": "indicator", "indicator": "close"},
			"right": map[string]any{"type": "indicator", "indicator": "ma10"},
		},
	}
}

func TestCompileBreakoutGivesStableHash(t *testing.T) {
	m1, diags, err := Compile(demoBreakoutForTest())
	if err != nil {
		t.Fatal(err)
	}
	m2, _, _ := Compile(demoBreakoutForTest())
	if !m1.IsExecutable() {
		t.Fatalf("breakout 应可执行, diagnostics=%v explain=%s", diags, ExplainDiagnostics(m1))
	}
	if m1.ContentHash == "" || m2.ContentHash == "" {
		t.Fatalf("hash should be non-empty")
	}
	// 验收 #3: 同一规范输入生成相同哈希
	if m1.ContentHash != m2.ContentHash {
		t.Fatalf("hash mismatch on identical input: %s vs %s", m1.ContentHash, m2.ContentHash)
	}
	if m1.CompilerVersion != CompilerVersion {
		t.Fatalf("compiler version mismatch: %s != %s", m1.CompilerVersion, CompilerVersion)
	}
	if m1.Scope.Universe != "universe_csi800" {
		t.Fatalf("universe not preserved: %+v", m1.Scope)
	}
	// 有 3 个 AND 子节点，说明 AST 已稳定构造
	if len(m1.EntryRule.Children) != 3 {
		t.Fatalf("entry children = %d, want 3", len(m1.EntryRule.Children))
	}
}

// ===== 验收 #2: 不可执行、歧义或含未来函数的方法被明确拒绝 =====

func TestCompileRejectsFutureFunctions(t *testing.T) {
	c := demoBreakoutForTest()
	c.Entry = map[string]any{
		"type": "compare", "op": "gt",
		"left":  map[string]any{"type": "indicator", "indicator": "future_close"},
		"right": map[string]any{"type": "indicator", "indicator": "close"},
	}
	m, diags, err := Compile(c)
	if err != nil {
		t.Fatal(err)
	}
	if m.IsExecutable() {
		t.Fatalf("future_close should make method non-executable, got executable. diags=%+v", diags)
	}
	found := false
	for _, d := range diags {
		if d.Code == "FUTURE_FUNCTION" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expect FUTURE_FUNCTION diagnostic, got %+v", diags)
	}
}

func TestCompileAmbiguousRetainsAmbiguities(t *testing.T) {
	c := demoBreakoutForTest()
	// 未知名操作符:
	c.Entry = map[string]any{
		"type":  "compare",
		"op":    "cross_via", // 不在白名单
		"left":  map[string]any{"type": "indicator", "indicator": "close"},
		"right": map[string]any{"type": "indicator", "indicator": "ma20"},
	}
	m, diags, _ := Compile(c)
	if m.IsExecutable() {
		t.Fatalf("ambiguous cross_via op should produce non-executable method, diags=%+v", diags)
	}
	if len(m.Ambiguities) == 0 {
		t.Fatalf("Ambiguities 列表为空，应显式保留歧义")
	}
}

// ===== 验收 #4: 入场和退出规则均可在真实 K 线上执行 =====

func buildSyntheticBars(n int, step float64, start float64) []Bar {
	out := make([]Bar, n)
	for i := 0; i < n; i++ {
		price := start + step*float64(i) + 0.1*math.Sin(float64(i)/5)
		low := price - 0.3
		high := price + 0.5
		out[i] = Bar{
			Date:   fmt.Sprintf("2024-%02d-%02d", (i%12)+1, (i%28)+1),
			Open:   price - 0.05,
			High:   high,
			Low:    low,
			Close:  price,
			Volume: 1e6 + float64(i)*10,
			Amount: price * 1e6,
		}
	}
	return out
}

func TestEntryAndExitExecuteOnBars(t *testing.T) {
	m, _, err := Compile(demoBreakoutForTest())
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsExecutable() {
		t.Fatal(ExplainDiagnostics(m))
	}
	// 构造真实波动的 bars：前 20 天是震荡上行（有涨有跌），
	// 第 20 天产生一个"突破 20 日高点"的入场信号。
	// 让 RSI14 处于 (50,72] 区间的关键是：不能让所有天数全涨，必须有交替。
	bars := make([]Bar, 0, 25)
	lastClose := 10.0
	for i := 0; i < 20; i++ {
		// 涨跌交替：偶数日涨 0.15，奇数日跌 0.10 → RSI14 大约在 60-70 之间
		var c float64
		if i%2 == 0 {
			c = lastClose + 0.15
		} else {
			c = lastClose - 0.10
		}
		// 最后两天（第18-19天）小幅加速上涨，形成"突破"姿态，
		// 但因为前18天有涨有跌，RSI14 仍会保持在 72 以下。
		if i >= 18 {
			c = lastClose + 0.22
		}
		bars = append(bars, Bar{
			Date: fmt.Sprintf("2024-01-%02d", i+1), Close: c, Low: c - 0.3, High: c + 0.3, Open: c - 0.05, Volume: 1e6, Amount: c * 1e6,
		})
		lastClose = c
	}
	enterAt := 19
	res, err := m.Entry(bars[enterAt], bars[:enterAt+1])
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched {
		t.Fatalf("第 %d 天应触发入场: trace=\n%s", enterAt, ExplainTrace(res.Trace))
	}

	// 构造持仓态, 并把后续 bars 改为价格连续下跌触发 ma10 退出
	entryState := &PositionState{EntryPrice: bars[enterAt].Close, EntryDate: bars[enterAt].Date}
	for i := 1; i <= 6; i++ {
		c := lastClose - 0.55*float64(i)
		bars = append(bars, Bar{
			Date: fmt.Sprintf("2024-02-%02d", i), Close: c, Low: c - 0.2, High: c + 0.1, Volume: 9e5, Amount: c * 9e5,
		})
		idx := len(bars) - 1
		exit, err := m.Exit(bars[idx], bars, entryState)
		if err != nil {
			t.Fatal(err)
		}
		if exit.Matched {
			return
		}
	}
	t.Fatalf("连续下跌后应触发 ma10 退出，检查 trace")
}

// ===== 验收 #5: 编译诊断能被 AI 转成用户可理解说明 =====

func TestExplainDiagnosticsProducesText(t *testing.T) {
	c := demoBreakoutForTest()
	c.Entry = map[string]any{
		"type": "compare", "op": "gt",
		"left":                               map[string]any{"type": "indicator", "indicator": "future_close"},
		"right":                              map[string]any{"type": "indicator", "indicator": "close"},
		"weird_field_should_trigger_unknown": true,
	}
	m, _, _ := Compile(c)
	text := ExplainDiagnostics(m)
	if len(text) < 20 {
		t.Fatalf("explain 太短: %q", text)
	}
	suggest := SuggestFix(m)
	if len(suggest) == 0 {
		t.Fatalf("SuggestFix 应给出候选修复建议")
	}
	for _, s := range suggest {
		if s == "" {
			t.Fatalf("空建议")
		}
	}
}

// ===== 验收 #6: 单元、属性和真实快照集成测试通过 =====

func TestHashChangesWhenSemanticsChange(t *testing.T) {
	a, _, err := Compile(demoBreakoutForTest())
	if err != nil {
		t.Fatal(err)
	}
	b := demoBreakoutForTest()
	b.PositionPct = &[]float64{0.15}[0]
	bm, _, _ := Compile(b)
	if a.ContentHash == bm.ContentHash {
		t.Fatalf("仓位改变应改变 hash")
	}
}

// 属性测试: 对于所有可执行的编译结果, ContentHash 必须稳定 (同一输入 100 次一致)
func TestHashPropertyStable(t *testing.T) {
	c := demoBreakoutForTest()
	m, _, _ := Compile(c)
	h0 := m.ContentHash
	if h0 == "" {
		t.Fatal("hash empty")
	}
	for i := 0; i < 100; i++ {
		m2, _, _ := Compile(c)
		if m2.ContentHash != h0 {
			t.Fatalf("hash drift at i=%d", i)
		}
	}
}

// 属性测试: 不允许未来函数 - 随机插入 future_ 前缀 indicator 的方法必然非可执行。
func TestNoFutureProperty(t *testing.T) {
	names := []string{"future_price", "future_close", "future_volume", "next_open", "lookahead_high"}
	for _, name := range names {
		c := demoBreakoutForTest()
		c.Entry = map[string]any{
			"type": "compare", "op": "gt",
			"left":  map[string]any{"type": "indicator", "indicator": name},
			"right": map[string]any{"type": "indicator", "indicator": "close"},
		}
		m, _, _ := Compile(c)
		if m.IsExecutable() {
			t.Fatalf("indicator %q 作为 future function 应被拒绝", name)
		}
	}
}
