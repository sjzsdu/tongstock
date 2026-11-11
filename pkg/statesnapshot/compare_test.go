package statesnapshot

import (
	"math"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/signalstate"
)

func TestBuildCompareFactsSeparatesInsufficientAndComputesReturns(t *testing.T) {
	close1, close2, close3, close4 := 100.0, 105.0, 110.0, 99.0
	obs := []compareObservation{
		{date: "20240101", close: &close1, states: map[signalstate.Dim]signalstate.State{signalstate.DimCross: signalstate.StateBuy}},
		{date: "20240102", close: &close2, states: map[signalstate.Dim]signalstate.State{signalstate.DimCross: signalstate.StateBuy}},
		{date: "20240103", close: &close3, states: map[signalstate.Dim]signalstate.State{signalstate.DimCross: signalstate.StateInsufficient}},
		{date: "20240104", close: &close4, states: map[signalstate.Dim]signalstate.State{signalstate.DimCross: signalstate.StateSell}},
	}
	stock := CompareStock{}
	buildCompareFacts(&stock, obs)
	var cross []CompareDuration
	for _, duration := range stock.Durations {
		if duration.Dim == signalstate.DimCross {
			cross = append(cross, duration)
		}
	}
	if len(cross) != 2 || cross[0].TradingDays != 2 || cross[0].ReturnPct == nil || math.Abs(*cross[0].ReturnPct-5) > 1e-9 {
		t.Fatalf("cross durations = %+v", cross)
	}
	if len(stock.Transitions) != 0 {
		t.Fatalf("insufficient data bridged transition: %+v", stock.Transitions)
	}
}

func TestBuildCompareFactsRecordsTransitions(t *testing.T) {
	close1, close2 := 100.0, 98.0
	obs := []compareObservation{
		{date: "20240101", close: &close1, states: map[signalstate.Dim]signalstate.State{signalstate.DimMA: signalstate.StateNeutral}},
		{date: "20240102", close: &close2, states: map[signalstate.Dim]signalstate.State{signalstate.DimMA: signalstate.StateSell}},
	}
	stock := CompareStock{}
	buildCompareFacts(&stock, obs)
	if len(stock.Transitions) != 1 || stock.Transitions[0].Date != "2024-01-02" || stock.Transitions[0].From != signalstate.StateNeutral || stock.Transitions[0].To != signalstate.StateSell {
		t.Fatalf("transitions = %+v", stock.Transitions)
	}
}
