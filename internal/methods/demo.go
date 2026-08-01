package methods

// DemoBreakout 返回一个生产代码可用的示例候选：
// 「20 日突破 + RSI14 在 (50,72]」，仓位 8% 等权，最多持有 20 天，
// 止损 -6% / 止盈 200%，跌破 ma10 或 max_days 触发出场。
// 与测试的 demoBreakoutForTest 保持字段完全一致，确保版本稳定。
func DemoBreakout() *Candidate {
	two := 2.0
	stopLoss := -0.06
	pct := 0.08
	return &Candidate{
		Name:           "20 日突破 + RSI 健康",
		Description:    "收盘价创 20 日新高，RSI14 在 (50,72] 区间，不追高。",
		SourceKind:     "structured",
		Universe:       "universe_csi800",
		MarketState:    []string{"bull", "range"},
		FeatureDeps:    nil,
		PositionMode:   "pct_equity",
		PositionPct:    &pct,
		HoldingMaxDays: 20,
		StopLossPct:    &stopLoss,
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
