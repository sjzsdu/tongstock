package signal

import (
	"math"
	"time"
)

type SignalType string

const (
	SignalGoldenCross SignalType = "金叉"
	SignalDeathCross  SignalType = "死叉"
	SignalOverbought  SignalType = "超买"
	SignalOversold    SignalType = "超卖"
	SignalBreakUpper  SignalType = "突破上轨"
	SignalBreakLower  SignalType = "跌破下轨"
	SignalBullAlign   SignalType = "多头排列"
	SignalBearAlign   SignalType = "空头排列"
)

type Signal struct {
	Code      string
	Date      time.Time
	Type      SignalType
	Indicator string
	Details   string
	Strength  float64
}

type DetectOptions struct {
	EnableMACD bool
	EnableKDJ  bool
	EnableBOLL bool
	EnableMA   bool
	EnableRSI  bool
}

func DefaultDetectOptions() *DetectOptions {
	return &DetectOptions{
		EnableMACD: true,
		EnableKDJ:  true,
		EnableBOLL: true,
		EnableMA:   true,
		EnableRSI:  true,
	}
}

var _ = math.MaxInt32
