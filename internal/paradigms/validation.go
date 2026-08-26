package paradigms

import "strings"

// ValidateParadigm checks structural integrity of a paradigm and returns a
// summary used by the admission pipeline. This is domain validation only —
// it does not touch market data or experiments. Pure, side-effect free.
func ValidateParadigm(p *Paradigm) ValidationSummary {
	s := ValidationSummary{Valid: true, DataCompleteness: 1}
	if p == nil {
		return ValidationSummary{Valid: false, Errors: []string{"empty paradigm"}}
	}
	if p.ID == "" {
		s.Errors = append(s.Errors, "id is required")
	}
	if p.Name == "" {
		s.Errors = append(s.Errors, "name is required")
	}
	if p.Side != "buy" && p.Side != "sell" {
		s.Errors = append(s.Errors, "side must be buy or sell")
	}
	if len(p.BuyConds) == 0 {
		s.Warnings = append(s.Warnings, "buy_conditions is empty")
	}
	conds := append([]Condition{}, p.BuyConds...)
	conds = append(conds, p.SellConds.TakeProfit...)
	conds = append(conds, p.SellConds.StopLoss...)
	s.TotalConditions = len(conds)
	for _, c := range conds {
		if c.Indicator == "" {
			s.Warnings = append(s.Warnings, "condition indicator is empty")
			continue
		}
		if IsAutoEvaluableCondition(c) {
			s.AutoEvaluable++
		}
	}
	if s.TotalConditions > 0 {
		s.AutoEvaluableRatio = float64(s.AutoEvaluable) / float64(s.TotalConditions)
	}
	if len(s.Errors) > 0 {
		s.Valid = false
	}
	s.ReliabilityLabel = "low"
	if s.Valid && s.AutoEvaluableRatio >= 0.7 {
		s.ReliabilityLabel = "high"
	} else if s.Valid && s.AutoEvaluableRatio >= 0.4 {
		s.ReliabilityLabel = "medium"
	}
	return s
}

// IsAutoEvaluableCondition reports whether a condition can be evaluated by the
// deterministic engine against persisted indicator data. Conditions with the
// "describe" operator are treated as auto-evaluable so human-readable rules
// still count toward the reliability signal without blocking admission.
func IsAutoEvaluableCondition(c Condition) bool {
	ind := NormalizeIndicator(c.Indicator)
	val := NormalizeIndicator(c.Value)
	supported := map[string]bool{
		"close": true, "volume": true, "ma5": true, "ma10": true,
		"ma20": true, "ma60": true, "rsi14": true, "macd_dif": true,
	}
	return supported[ind] || supported[val] || c.Operator == "describe"
}

// NormalizeIndicator maps the many textual variants of an indicator name
// (e.g. "MACD.DIF", "收盘价", "RSI6") to the canonical key used by the
// deterministic evaluation engine. It is the single source of truth shared by
// paradigm validation, the server condition evaluator, and the backtest
// executor so the three never drift.
func NormalizeIndicator(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.Trim(s, "\"'`：:")
	switch s {
	case "price", "current", "当前价", "收盘价", "closeprice":
		return "close"
	case "成交量", "vol":
		return "volume"
	case "dif", "macd.dif", "macd_dif":
		return "macd_dif"
	case "rsi", "rsi6", "rsi14":
		return "rsi14"
	}
	return s
}
