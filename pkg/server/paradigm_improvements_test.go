package server

import (
	"testing"

	"github.com/sjzsdu/tongstock/internal/paradigms"
)

func TestExtractParadigmJSON(t *testing.T) {
	text := "分析正文\n```json\n{\"id\":\"p1\",\"name\":\"突破范式\",\"side\":\"buy\",\"context\":{\"market_cap\":\"mid\",\"shareholder_dominant\":\"mixed\"},\"buy_conditions\":[{\"indicator\":\"close\",\"operator\":\"gt\",\"value\":\"MA20\"}],\"sell_conditions\":{\"stop_loss\":[{\"indicator\":\"close\",\"operator\":\"lt\",\"value\":\"MA60\"}]},\"expectation\":{\"holding_period\":\"2-6周\",\"expected_return\":\"8-15%\",\"risk_reward_ratio\":\"2:1\",\"confidence\":0.6},\"rationale\":\"test\"}\n```"
	p := extractParadigm(text, "000001", "平安银行")
	if p == nil {
		t.Fatal("expected paradigm")
	}
	if p.ID != "p1" || p.StockCode != "000001" || p.StockName != "平安银行" {
		t.Fatalf("unexpected paradigm: %+v", p)
	}
	if len(p.BuyConds) != 1 || p.BuyConds[0].Operator != "gt" {
		t.Fatalf("unexpected conditions: %+v", p.BuyConds)
	}
}

func TestValidateParadigmReliability(t *testing.T) {
	p := &paradigms.Paradigm{
		ID: "p1", Name: "n", Side: "buy",
		BuyConds: []paradigms.Condition{{Indicator: "close", Operator: "gt", Value: "MA20"}, {Indicator: "文本确认", Operator: "describe"}},
	}
	v := validateParadigm(p)
	if !v.Valid {
		t.Fatalf("expected valid: %+v", v)
	}
	if v.TotalConditions != 2 || v.AutoEvaluable != 2 {
		t.Fatalf("unexpected validation summary: %+v", v)
	}
}

func TestEvaluateStructuredCondition(t *testing.T) {
	indicator := map[string]float64{"close": 11, "ma20": 10, "rsi14": 72}
	cases := []struct {
		name string
		cond paradigms.Condition
		want string
	}{
		{"gt-indicator", paradigms.Condition{Indicator: "close", Operator: "gt", Value: "MA20"}, "met"},
		{"lt-number", paradigms.Condition{Indicator: "rsi14", Operator: "lt", Value: "70"}, "not_met"},
		{"between", paradigms.Condition{Indicator: "rsi14", Operator: "between", Value: "70-80"}, "met"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ec := EvaluatedCondition{Condition: formatConditionText(tc.cond), Type: "buy"}
			evaluateSingleCondition(&ec, tc.cond, indicator)
			if ec.Status != tc.want {
				t.Fatalf("status=%s want=%s value=%s", ec.Status, tc.want, ec.Value)
			}
		})
	}
}
