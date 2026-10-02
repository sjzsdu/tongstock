package server

import (
	"math"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/signal"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

func TestSignalDirection(t *testing.T) {
	cases := map[string]string{
		"金叉":     "buy",
		"超卖":     "buy",
		"突破上轨":   "buy",
		"多头排列":   "buy",
		"死叉":     "sell",
		"超买":     "sell",
		"跌破下轨":   "buy",
		"空头排列":   "sell",
		"未知信号类型": "buy",
	}
	for sigType, want := range cases {
		if got := signalDirection(sigType); got != want {
			t.Errorf("signalDirection(%q) = %q, want %q", sigType, got, want)
		}
	}
}

func TestBuildSignalsResponsePeers(t *testing.T) {
	day := func(date string) time.Time {
		ts, err := time.Parse("2006-01-02", date)
		if err != nil {
			t.Fatalf("bad date: %v", err)
		}
		return ts
	}
	signals := []signal.Signal{
		{Code: "300418", Date: day("2024-06-18"), Type: signal.SignalOversold, Indicator: "KDJ", Strength: 1.1},
		{Code: "300418", Date: day("2024-06-18"), Type: signal.SignalGoldenCross, Indicator: "MACD", Strength: 0.9},
		{Code: "300418", Date: day("2024-06-18"), Type: signal.SignalOverbought, Indicator: "RSI", Strength: 0.5},
		{Code: "300418", Date: day("2024-06-20"), Type: signal.SignalDeathCross, Indicator: "MACD", Strength: 0.8},
	}

	rows := buildSignalsResponse(signals)
	if len(rows) != len(signals) {
		t.Fatalf("got %d rows, want %d", len(rows), len(signals))
	}

	// 2024-06-18：2 买（KDJ 超卖、MACD 金叉）1 卖（RSI 超买）
	first := rows[0]
	peers := first["Peers"].(gin.H)
	if peers["buy_count"].(int) != 2 {
		t.Errorf("buy_count = %v, want 2", peers["buy_count"])
	}
	if peers["sell_count"].(int) != 1 {
		t.Errorf("sell_count = %v, want 1", peers["sell_count"])
	}
	others := peers["others"].([]gin.H)
	if len(others) != 2 {
		t.Fatalf("others len = %d, want 2", len(others))
	}
	// 自身（KDJ 超卖）不在 others 中
	for _, o := range others {
		if o["indicator"] == "KDJ" && o["type"] == "超卖" {
			t.Errorf("others should exclude the signal itself, got %+v", others)
		}
	}

	// 2024-06-20：单信号，无同日同伴
	last := rows[3]
	lastPeers := last["Peers"].(gin.H)
	if lastPeers["buy_count"].(int) != 0 || lastPeers["sell_count"].(int) != 1 {
		t.Errorf("unexpected counts: %+v", lastPeers)
	}
	if len(lastPeers["others"].([]gin.H)) != 0 {
		t.Errorf("others should be empty for isolated signal")
	}

	// Peers 为空场景不存在：每条信号至少含自身计数
	// Date 字段保持 2006-01-02 格式
	if first["Date"] != "2024-06-18" {
		t.Errorf("Date = %v, want 2024-06-18", first["Date"])
	}
}

func TestMatchedBlocksInFileOrder(t *testing.T) {
	items := []*protocol.BlockItem{
		{BlockName: "沪深300", StockCode: "600519"},
		{BlockName: "白酒概念", StockCode: "000001"},
		{BlockName: "沪深300", StockCode: "000001"},
		{BlockName: "上证50", StockCode: "600519"},
		{BlockName: "沪股通", StockCode: "000001"},
		{BlockName: "沪深300", StockCode: "300418"},
	}

	got := matchedBlocksInFileOrder(items, "000001")
	want := []string{"白酒概念", "沪深300", "沪股通"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	if got := matchedBlocksInFileOrder(items, "999999"); got != nil {
		t.Errorf("no match should return nil, got %v", got)
	}
}

func TestValidQuoteForCompare(t *testing.T) {
	cases := []struct {
		price, lastClose float64
		want             bool
	}{
		{10, 9.5, true},
		{0, 9.5, false},           // 价格为 0（停牌等）
		{10, 0, false},            // LastClose==0 → 除零
		{10, -1, false},           // 负昨收
		{math.NaN(), 9.5, false},  // NaN
		{math.Inf(1), 9.5, false}, // +Inf
		{10, math.Inf(-1), false},
	}
	for _, c := range cases {
		if got := validQuoteForCompare(c.price, c.lastClose); got != c.want {
			t.Errorf("validQuoteForCompare(%v, %v) = %v, want %v", c.price, c.lastClose, got, c.want)
		}
	}
}
