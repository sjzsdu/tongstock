package backtest

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/trading"
)

func TestRunParadigmUsesNextBarOpenAndIncludesCosts(t *testing.T) {
	bars := testBars(
		[4]float64{9.8, 10.0, 9.7, 10.0},
		[4]float64{10.1, 10.5, 10.0, 10.5},
		[4]float64{10.6, 11.3, 10.5, 11.2},
		[4]float64{11.5, 11.8, 11.3, 11.6},
	)
	p := &paradigms.Paradigm{
		ID: "cost-case",
		BuyConds: []paradigms.Condition{
			{Indicator: "close", Operator: "gt", Value: "10"},
		},
		SellConds: paradigms.SellConditions{
			TakeProfit: []paradigms.Condition{
				{Indicator: "close", Operator: "gt", Value: "11"},
			},
		},
	}

	result, err := RunParadigm(context.Background(), p, bars, DefaultParadigmExecutionConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 2 {
		t.Fatalf("fills = %d, want 2: %+v", len(result.Fills), result)
	}
	buy, sell := result.Fills[0], result.Fills[1]
	if buy.ExecutionDate != bars[2].Date || buy.ExecutionDate == bars[1].Date {
		t.Fatalf("buy execution date = %s, want next market bar %s", buy.ExecutionDate, bars[2].Date)
	}
	if buy.SignalPrice != bars[1].Close {
		t.Fatalf("buy signal price = %.2f, want signal close %.2f", buy.SignalPrice, bars[1].Close)
	}
	wantBuyPrice := 10.61 // 10.60 open plus 10 bps, rounded to cents
	if buy.Price != wantBuyPrice {
		t.Fatalf("buy price = %.2f, want next open with slippage %.2f", buy.Price, wantBuyPrice)
	}
	if !sell.ExecutionDate.After(buy.ExecutionDate) {
		t.Fatalf("sell %s must be after buy %s for T+1", sell.ExecutionDate, buy.ExecutionDate)
	}
	if result.TotalCost <= 0 {
		t.Fatal("expected real commission/tax/transfer costs")
	}
	if math.Abs((result.GrossPnL-result.TotalCost)-result.NetPnL) > 0.01 {
		t.Fatalf("gross %.2f - cost %.2f != net %.2f", result.GrossPnL, result.TotalCost, result.NetPnL)
	}
	if len(result.Trades) != 1 || result.Trades[0].NetPnL >= result.Trades[0].GrossPnL {
		t.Fatalf("completed trade must separate gross and net PnL: %+v", result.Trades)
	}
}

func TestRunParadigmDoesNotReadFutureBars(t *testing.T) {
	p := &paradigms.Paradigm{
		ID: "no-future",
		BuyConds: []paradigms.Condition{
			{Indicator: "close", Operator: "cross_above", Value: "10"},
		},
	}
	prefix := testBars(
		[4]float64{9.8, 10.0, 9.5, 9.8},
		[4]float64{10.1, 10.4, 9.9, 10.2},
		[4]float64{10.3, 10.6, 10.1, 10.4},
	)
	a, err := RunParadigm(context.Background(), p, prefix, DefaultParadigmExecutionConfig())
	if err != nil {
		t.Fatal(err)
	}
	withFuture := append(append([]MarketBar(nil), prefix...), MarketBar{
		Code: "600000", Date: prefix[2].Date.AddDate(0, 0, 1),
		Open: 1000, High: 1100, Low: 900, Close: 1050, Volume: 1_000_000,
		Board: trading.BoardMain,
	})
	b, err := RunParadigm(context.Background(), p, withFuture, DefaultParadigmExecutionConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Fills) == 0 || len(b.Fills) == 0 {
		t.Fatalf("expected an existing fill, got %d and %d", len(a.Fills), len(b.Fills))
	}
	if a.Fills[0].ExecutionDate != b.Fills[0].ExecutionDate || a.Fills[0].Price != b.Fills[0].Price {
		t.Fatalf("future bar changed earlier fill: %+v vs %+v", a.Fills[0], b.Fills[0])
	}
}

func TestRunParadigmRejectsLimitUpSuspensionInsufficientAndMissingBar(t *testing.T) {
	p := &paradigms.Paradigm{
		ID: "rejects",
		BuyConds: []paradigms.Condition{
			{Indicator: "close", Operator: "gt", Value: "9"},
		},
	}
	tests := []struct {
		name string
		bars []MarketBar
		cfg  ParadigmExecutionConfig
		code trading.RejectCode
	}{
		{
			name: "limit up",
			bars: testBars(
				[4]float64{9.8, 10.2, 9.7, 10},
				[4]float64{11, 11, 11, 11},
			),
			cfg:  DefaultParadigmExecutionConfig(),
			code: trading.RejectPriceLimit,
		},
		{
			name: "suspended",
			bars: func() []MarketBar {
				b := testBars(
					[4]float64{9.8, 10.2, 9.7, 10},
					[4]float64{10, 10, 10, 10},
				)
				b[1].Suspended = true
				b[1].Volume = 0
				return b
			}(),
			cfg:  DefaultParadigmExecutionConfig(),
			code: trading.RejectSuspended,
		},
		{
			name: "insufficient for one lot",
			bars: testBars(
				[4]float64{9.8, 10.2, 9.7, 10},
				[4]float64{10, 10.2, 9.9, 10.1},
			),
			cfg: func() ParadigmExecutionConfig {
				cfg := DefaultParadigmExecutionConfig()
				cfg.InitialCash = 999
				return cfg
			}(),
			code: trading.RejectInsufficient,
		},
		{
			name: "missing next market bar",
			bars: testBars(
				[4]float64{9.8, 10.2, 9.7, 10},
			),
			cfg:  DefaultParadigmExecutionConfig(),
			code: trading.RejectMissingMarketData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RunParadigm(context.Background(), p, tt.bars, tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Fills) != 0 {
				t.Fatalf("unexpected fills: %+v", result.Fills)
			}
			if len(result.Rejections) == 0 || result.Rejections[0].Code != tt.code {
				t.Fatalf("rejections = %+v, want first code %s", result.Rejections, tt.code)
			}
			if result.Rejections[0].Order.SignalDate.IsZero() {
				t.Fatal("rejected order must retain signal_date")
			}
		})
	}
}

func TestRunParadigmRejectsUnsupportedCondition(t *testing.T) {
	p := &paradigms.Paradigm{
		BuyConds: []paradigms.Condition{
			{Indicator: "AI says buy", Operator: "describe", Value: "maybe"},
		},
	}
	_, err := RunParadigm(context.Background(), p, testBars(
		[4]float64{9.8, 10.2, 9.7, 10},
	), DefaultParadigmExecutionConfig())
	if err == nil {
		t.Fatal("unsupported non-computable condition must fail closed")
	}
}

func testBars(values ...[4]float64) []MarketBar {
	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local)
	bars := make([]MarketBar, len(values))
	for i, v := range values {
		bars[i] = MarketBar{
			Code: "600000", Date: start.AddDate(0, 0, i),
			Open: v[0], High: v[1], Low: v[2], Close: v[3],
			Volume: 100_000, Amount: v[3] * 100_000, Board: trading.BoardMain,
		}
	}
	return bars
}
