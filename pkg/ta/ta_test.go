package ta

import (
	"testing"
	"time"
)

func makeTestKlines(closes []float64) []KlineInput {
	klines := make([]KlineInput, len(closes))
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	for i, c := range closes {
		klines[i] = KlineInput{
			Time: base.AddDate(0, 0, i),
			Open: c, High: c, Low: c, Close: c,
		}
	}
	return klines
}

func TestSMA(t *testing.T) {
	klines := makeTestKlines([]float64{10, 11, 12, 13, 14, 15})
	result := SMA(klines, 3)

	if len(result) != 6 {
		t.Fatalf("expected length 6, got %d", len(result))
	}
	if result[0] != 0 || result[1] != 0 {
		t.Error("first two values should be 0")
	}
	expected := (10.0 + 11.0 + 12.0) / 3.0
	if result[2] != expected {
		t.Errorf("expected %f, got %f", expected, result[2])
	}
}

func TestEMA(t *testing.T) {
	klines := makeTestKlines([]float64{10, 11, 12, 13, 14})
	result := EMA(klines, 3)

	if len(result) != 5 {
		t.Fatalf("expected length 5, got %d", len(result))
	}
	if result[0] != 0 || result[1] != 0 {
		t.Error("first two values should be 0")
	}
	expectedFirst := (10.0 + 11.0 + 12.0) / 3.0
	if result[2] != expectedFirst {
		t.Errorf("first EMA should be SMA, expected %f, got %f", expectedFirst, result[2])
	}
}

func TestCalcMACD(t *testing.T) {
	closes := make([]float64, 50)
	for i := range closes {
		closes[i] = float64(100 + i + (i%5 - 2))
	}
	klines := makeTestKlines(closes)
	result := CalcMACD(klines, 12, 26, 9)

	if len(result.DIF) != 50 {
		t.Errorf("DIF length should be 50, got %d", len(result.DIF))
	}
	if result.Fast != 12 || result.Slow != 26 || result.Signal != 9 {
		t.Error("parameters not set correctly")
	}
}

func TestCalcKDJ(t *testing.T) {
	closes := make([]float64, 30)
	for i := range closes {
		closes[i] = float64(50 + i%10)
	}
	klines := makeTestKlines(closes)
	result := CalcKDJ(klines, 9, 3, 3)

	if len(result.K) != 30 || len(result.D) != 30 || len(result.J) != 30 {
		t.Error("KDJ arrays should have correct length")
	}
	if result.N != 9 || result.M1 != 3 || result.M2 != 3 {
		t.Error("KDJ parameters not set correctly")
	}
}

func TestCalcBOLL(t *testing.T) {
	closes := make([]float64, 30)
	for i := range closes {
		closes[i] = float64(100 + i%5)
	}
	klines := makeTestKlines(closes)
	result := CalcBOLL(klines, 20, 2.0)

	if len(result.Upper) != 30 || len(result.Middle) != 30 || len(result.Lower) != 30 {
		t.Error("BOLL arrays should have correct length")
	}
	for i := 20; i < 30; i++ {
		if result.Upper[i] <= result.Middle[i] || result.Middle[i] <= result.Lower[i] {
			t.Errorf("BOLL bands out of order at index %d", i)
		}
	}
}

func TestRSI(t *testing.T) {
	closes := make([]float64, 30)
	for i := range closes {
		closes[i] = float64(100 + i%3*2 - 1)
	}
	klines := makeTestKlines(closes)
	result := CalcRSI(klines, 14)

	if len(result) != 30 {
		t.Errorf("RSI length should be 30, got %d", len(result))
	}
	for i := 14; i < 30; i++ {
		if result[i] < 0 || result[i] > 100 {
			t.Errorf("RSI out of range at index %d: %f", i, result[i])
		}
	}
}

func TestCalculate(t *testing.T) {
	closes := make([]float64, 100)
	for i := range closes {
		closes[i] = float64(100 + i%10)
	}
	klines := makeTestKlines(closes)
	cfg := DefaultConfig()
	result := Calculate(klines, cfg)

	if result.MA == nil {
		t.Error("MA should not be nil")
	}
	if result.MACD == nil {
		t.Error("MACD should not be nil")
	}
	if result.KDJ == nil {
		t.Error("KDJ should not be nil")
	}
	if result.BOLL == nil {
		t.Error("BOLL should not be nil")
	}
	if result.RSI == nil {
		t.Error("RSI should not be nil")
	}
}
