package signal

import (
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/ta"
)

func makeKlines(closes []float64) []ta.KlineInput {
	klines := make([]ta.KlineInput, len(closes))
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	for i, c := range closes {
		klines[i] = ta.KlineInput{
			Time: base.AddDate(0, 0, i),
			Open: c, High: c + 1, Low: c - 1, Close: c,
		}
	}
	return klines
}

func TestDetect(t *testing.T) {
	closes := make([]float64, 50)
	for i := range closes {
		closes[i] = float64(100 + i%10*5)
	}
	klines := makeKlines(closes)
	cfg := ta.DefaultConfig()
	result := ta.Calculate(klines, cfg)
	signals := Detect("000001", klines, result, nil)

	if signals == nil {
		t.Error("signals should not be nil")
	}
}

func TestDetectCross(t *testing.T) {
	if detectCross(1, -1) != 1 {
		t.Error("should detect golden cross")
	}
	if detectCross(-1, 1) != -1 {
		t.Error("should detect death cross")
	}
	if detectCross(1, 1) != 0 {
		t.Error("should not detect cross for same sign")
	}
}

func TestDetectMACDSignals(t *testing.T) {
	klines := makeKlines([]float64{100, 101, 102, 100, 99, 101, 103, 102, 104, 105, 106, 107, 108, 109, 110, 108, 107, 106, 108, 110, 112, 114, 116, 118, 120, 118, 116, 114, 116, 118})
	result := ta.Calculate(klines, ta.DefaultConfig())
	signals := detectMACDSignals("000001", klines, result.MACD)

	t.Logf("Found %d MACD signals", len(signals))
	for _, s := range signals {
		t.Logf("  %s %s %s", s.Date.Format("2006-01-02"), s.Type, s.Details)
	}
}
