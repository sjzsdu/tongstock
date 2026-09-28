package methods

// SeedCandidate 是一个内置示例方法定义，附带它在产品里的用途说明。
// 它只是「编译器输入」，是否可信由验证工厂在真实冻结数据上回测决定，
// 本包不声明任何有效性。
type SeedCandidate struct {
	Key       string
	Rationale string
	Candidate *Candidate
}

// SeedCandidates 返回内置示例方法（冷启动种子）。
//
// 设计约束（与选股链路一致，避免「验证通过但选股永不触发」的假象）：
//   - 全部使用单日可判定的状态条件，不含 cross / in_window（选股只持有
//     单日冻结特征快照，无法回看历史）。
//   - 显式声明 FeatureDeps，使选股阶段能对缺失特征 fail closed。
//   - 股票池为 universe_usable，与市场快照宇宙名一致。
//   - 仓位 10% 等权，满足 critic 的单票权重硬上限。
func SeedCandidates() []SeedCandidate {
	return []SeedCandidate{
		{Key: "ma_alignment", Rationale: "均线多头排列：短中长期均线依次向上，代表趋势结构完好。", Candidate: seedMAAlignment()},
		{Key: "macd_bull", Rationale: "MACD 多头：DIF 位于 DEA 上方且柱状为正，代表中期动能偏多。", Candidate: seedMACDBull()},
		{Key: "volume_breakout", Rationale: "量价突破：收盘创 20 日新高且成交量高于 20 日均量，代表放量突破。", Candidate: seedVolumeBreakout()},
	}
}

func seedMAAlignment() *Candidate {
	pct := 0.10
	stopLoss := -0.08
	takeProfit := 0.30
	return &Candidate{
		Name:           "均线多头排列",
		Description:    "收盘价站上 ma5，且 ma5 > ma10 > ma20，趋势结构完好时入场；跌破 ma10 离场。",
		SourceKind:     "structured",
		Universe:       "universe_usable",
		FeatureDeps:    []string{"close", "ma5", "ma10", "ma20"},
		PositionMode:   "pct_equity",
		PositionPct:    &pct,
		HoldingMaxDays: 30,
		HoldingMinDays: 3,
		StopLossPct:    &stopLoss,
		TakeProfitPct:  &takeProfit,
		Entry: map[string]any{
			"type": "and",
			"children": []any{
				compare("gt", indicator("close"), indicator("ma5")),
				compare("gt", indicator("ma5"), indicator("ma10")),
				compare("gt", indicator("ma10"), indicator("ma20")),
			},
		},
		Exit: compare("lt", indicator("close"), indicator("ma10")),
	}
}

func seedMACDBull() *Candidate {
	pct := 0.10
	stopLoss := -0.08
	takeProfit := 0.25
	return &Candidate{
		Name:           "MACD 多头",
		Description:    "DIF 位于 DEA 上方且 MACD 柱为正时入场；DIF 跌破 DEA 离场。",
		SourceKind:     "structured",
		Universe:       "universe_usable",
		FeatureDeps:    []string{"macd_dif", "macd_dea", "macd_hist"},
		PositionMode:   "pct_equity",
		PositionPct:    &pct,
		HoldingMaxDays: 25,
		HoldingMinDays: 3,
		StopLossPct:    &stopLoss,
		TakeProfitPct:  &takeProfit,
		Entry: map[string]any{
			"type": "and",
			"children": []any{
				compare("gt", indicator("macd_dif"), indicator("macd_dea")),
				compare("gt", indicator("macd_hist"), constant(0)),
			},
		},
		Exit: compare("lt", indicator("macd_dif"), indicator("macd_dea")),
	}
}

func seedVolumeBreakout() *Candidate {
	pct := 0.10
	stopLoss := -0.07
	takeProfit := 0.20
	return &Candidate{
		Name:           "量价突破",
		Description:    "收盘价创 20 日新高、成交量高于 20 日均量且站上 ma20 时入场；跌破 ma10 离场。",
		SourceKind:     "structured",
		Universe:       "universe_usable",
		FeatureDeps:    []string{"close", "ma10", "ma20", "prevhigh20", "volume", "volma20"},
		PositionMode:   "pct_equity",
		PositionPct:    &pct,
		HoldingMaxDays: 20,
		HoldingMinDays: 3,
		StopLossPct:    &stopLoss,
		TakeProfitPct:  &takeProfit,
		Entry: map[string]any{
			"type": "and",
			"children": []any{
				compare("gt", indicator("close"), indicator("prevhigh20")),
				compare("gt", indicator("volume"), indicator("volma20")),
				compare("gt", indicator("close"), indicator("ma20")),
			},
		},
		Exit: compare("lt", indicator("close"), indicator("ma10")),
	}
}

func indicator(name string) map[string]any {
	return map[string]any{"type": "indicator", "indicator": name}
}

func constant(value float64) map[string]any {
	return map[string]any{"type": "constant", "value": value}
}

func compare(op string, left, right map[string]any) map[string]any {
	return map[string]any{"type": "compare", "op": op, "left": left, "right": right}
}
