package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sjzsdu/tongstock/internal/paradigms"
)

// EvaluatedCondition is a paradigm condition evaluated against stock data
type EvaluatedCondition struct {
	Condition string `json:"condition"`       // e.g. "MA20 > MA10"
	Type      string `json:"type"`            // buy / take_profit / stop_loss
	Status    string `json:"status"`          // met / not_met / unknown
	Value     string `json:"value,omitempty"` // current stock value, e.g. "MA20=12.34, MA10=12.28"
}

// evaluateParadigmConditions evaluates all paradigm conditions against current stock data
func (s *Server) evaluateParadigmConditions(stockCode string, p *paradigms.Paradigm) []EvaluatedCondition {
	if p == nil {
		return nil
	}

	indicator := s.fetchCurrentIndicator(stockCode)
	if indicator == nil {
		return nil
	}

	var results []EvaluatedCondition

	// Evaluate buy conditions
	for _, c := range p.BuyConds {
		ec := EvaluatedCondition{Condition: formatConditionText(c), Type: "buy"}
		evaluateSingleCondition(&ec, c, indicator)
		results = append(results, ec)
	}

	// Evaluate take profit conditions
	for _, c := range p.SellConds.TakeProfit {
		ec := EvaluatedCondition{Condition: formatConditionText(c), Type: "take_profit"}
		evaluateSingleCondition(&ec, c, indicator)
		results = append(results, ec)
	}

	// Evaluate stop loss conditions
	for _, c := range p.SellConds.StopLoss {
		ec := EvaluatedCondition{Condition: formatConditionText(c), Type: "stop_loss"}
		evaluateSingleCondition(&ec, c, indicator)
		results = append(results, ec)
	}

	return results
}

func formatConditionText(c paradigms.Condition) string {
	if c.Operator == "describe" || c.Value == "" {
		return c.Indicator
	}
	return fmt.Sprintf("%s %s %s", c.Indicator, c.Operator, c.Value)
}

func evaluateSingleCondition(ec *EvaluatedCondition, c paradigms.Condition, indicator map[string]float64) {
	if evaluateStructuredCondition(ec, c, indicator) {
		return
	}
	text := strings.ToLower(ec.Condition)
	close, hasClose := indicator["close"]

	// Try to match indicator patterns and compare
	if strings.Contains(text, "ma20") && strings.Contains(text, "ma10") {
		if ma20, ok := indicator["ma20"]; ok {
			if ma10, ok := indicator["ma10"]; ok {
				ec.Value = fmt.Sprintf("MA20=%.2f, MA10=%.2f", ma20, ma10)
				if ma20 > ma10 {
					ec.Status = "met"
				} else {
					ec.Status = "not_met"
				}
				return
			}
		}
	}

	if strings.Contains(text, "ma60") {
		if hasClose {
			if ma60, ok := indicator["ma60"]; ok {
				ec.Value = fmt.Sprintf("当前价=%.2f, MA60=%.2f", close, ma60)
				if strings.Contains(text, ">") || strings.Contains(text, "上方") {
					if close > ma60 {
						ec.Status = "met"
					} else {
						ec.Status = "not_met"
					}
				} else if strings.Contains(text, "<") || strings.Contains(text, "跌破") {
					if close < ma60 {
						ec.Status = "met"
					} else {
						ec.Status = "not_met"
					}
				}
				return
			}
		}
	}

	if strings.Contains(text, "rsi") {
		if rsi, ok := indicator["rsi14"]; ok {
			ec.Value = fmt.Sprintf("RSI14=%.1f", rsi)
			if strings.Contains(text, "> 70") || strings.Contains(text, ">70") || strings.Contains(text, "超买") {
				if rsi > 70 {
					ec.Status = "met"
				} else {
					ec.Status = "not_met"
				}
				return
			}
			if strings.Contains(text, "< 30") || strings.Contains(text, "<30") || strings.Contains(text, "超卖") {
				if rsi < 30 {
					ec.Status = "met"
				} else {
					ec.Status = "not_met"
				}
				return
			}
			ec.Status = "unknown"
			ec.Value = fmt.Sprintf("RSI14=%.1f (阈值未明确)", rsi)
			return
		}
	}

	if strings.Contains(text, "macd") {
		if dif, ok := indicator["macd_dif"]; ok {
			ec.Value = fmt.Sprintf("MACD DIF=%.4f", dif)
			if strings.Contains(text, "死叉") || strings.Contains(text, "dif < 0") {
				if dif < 0 {
					ec.Status = "met"
				} else {
					ec.Status = "not_met"
				}
				return
			}
			if strings.Contains(text, "金叉") || strings.Contains(text, "dif > 0") {
				if dif > 0 {
					ec.Status = "met"
				} else {
					ec.Status = "not_met"
				}
				return
			}
		}
	}

	if strings.Contains(text, "成交量") || strings.Contains(text, "量") {
		if vol, ok := indicator["volume"]; ok {
			if avgVol, ok := indicator["avg_volume_20"]; ok {
				ratio := vol / avgVol
				ec.Value = fmt.Sprintf("量比=%.2f (量=%.0f, 20日均量=%.0f)", ratio, vol, avgVol)
				if ratio > 1.2 {
					ec.Status = "met"
				} else {
					ec.Status = "not_met"
				}
				return
			}
		}
	}

	// Price comparison: "跌破XX" or "突破XX"
	if strings.Contains(text, "跌破") || strings.Contains(text, "突破") {
		if price := extractPrice(ec.Condition); price > 0 && hasClose {
			ec.Value = fmt.Sprintf("当前价=%.2f, 关键价=%.2f", close, price)
			if strings.Contains(text, "跌破") {
				if close < price {
					ec.Status = "met"
				} else {
					ec.Status = "not_met"
				}
			} else {
				if close > price {
					ec.Status = "met"
				} else {
					ec.Status = "not_met"
				}
			}
			return
		}
	}

	ec.Status = "unknown"
	ec.Value = "无法自动匹配"
}

func evaluateStructuredCondition(ec *EvaluatedCondition, c paradigms.Condition, indicator map[string]float64) bool {
	op := strings.ToLower(strings.TrimSpace(c.Operator))
	if op == "" || op == "describe" {
		return false
	}
	leftName := paradigms.NormalizeIndicator(c.Indicator)
	left, ok := indicator[leftName]
	if !ok {
		return false
	}
	right, rightLabel, ok := resolveConditionValue(c.Value, indicator)
	if !ok && op != "between" {
		return false
	}
	ec.Value = fmt.Sprintf("%s=%.4f, %s=%.4f", leftName, left, rightLabel, right)
	switch op {
	case "gt", ">":
		ec.Status = boolStatus(left > right)
	case "lt", "<":
		ec.Status = boolStatus(left < right)
	case "near":
		tolerance := 0.03
		if right != 0 && absFloat(left-right)/absFloat(right) <= tolerance {
			ec.Status = "met"
		} else {
			ec.Status = "not_met"
		}
	case "between":
		lo, hi, ok := parseRange(c.Value)
		if !ok {
			return false
		}
		ec.Value = fmt.Sprintf("%s=%.4f, range=%.4f-%.4f", leftName, left, lo, hi)
		ec.Status = boolStatus(left >= lo && left <= hi)
	case "cross_above":
		prevLeft, prevRight, ok := resolvePreviousPair(leftName, c.Value, indicator)
		if !ok {
			return false
		}
		ec.Value = fmt.Sprintf("%s prev=%.4f now=%.4f, %s prev=%.4f now=%.4f", leftName, prevLeft, left, rightLabel, prevRight, right)
		ec.Status = boolStatus(prevLeft <= prevRight && left > right)
	case "cross_below":
		prevLeft, prevRight, ok := resolvePreviousPair(leftName, c.Value, indicator)
		if !ok {
			return false
		}
		ec.Value = fmt.Sprintf("%s prev=%.4f now=%.4f, %s prev=%.4f now=%.4f", leftName, prevLeft, left, rightLabel, prevRight, right)
		ec.Status = boolStatus(prevLeft >= prevRight && left < right)
	default:
		return false
	}
	return true
}

func resolvePreviousPair(leftName, rightRaw string, indicator map[string]float64) (float64, float64, bool) {
	prevLeft, ok := indicator["prev_"+leftName]
	if !ok {
		return 0, 0, false
	}
	rightName := paradigms.NormalizeIndicator(rightRaw)
	if prevRight, ok := indicator["prev_"+rightName]; ok {
		return prevLeft, prevRight, true
	}
	right, _, ok := resolveConditionValue(rightRaw, indicator)
	return prevLeft, right, ok
}

func resolveConditionValue(v string, indicator map[string]float64) (float64, string, bool) {
	label := paradigms.NormalizeIndicator(v)
	if val, ok := indicator[label]; ok {
		return val, label, true
	}
	v = strings.Trim(strings.TrimSpace(v), "%")
	f, err := strconv.ParseFloat(v, 64)
	if err == nil {
		return f, v, true
	}
	return 0, label, false
}

func parseRange(v string) (float64, float64, bool) {
	v = strings.ReplaceAll(v, "至", "-")
	v = strings.ReplaceAll(v, "~", "-")
	parts := strings.Split(v, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, err1 := strconv.ParseFloat(strings.TrimSpace(strings.Trim(parts[0], "%")), 64)
	b, err2 := strconv.ParseFloat(strings.TrimSpace(strings.Trim(parts[1], "%")), 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if a > b {
		a, b = b, a
	}
	return a, b, true
}

func boolStatus(v bool) string {
	if v {
		return "met"
	}
	return "not_met"
}
func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
