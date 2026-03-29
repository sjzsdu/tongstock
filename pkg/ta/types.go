package ta

import "time"

type KlineInput struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Amount float64
}

type IndicatorType string

const (
	IndicatorMA   IndicatorType = "MA"
	IndicatorMACD IndicatorType = "MACD"
	IndicatorKDJ  IndicatorType = "KDJ"
	IndicatorBOLL IndicatorType = "BOLL"
	IndicatorRSI  IndicatorType = "RSI"
)

type IndicatorResult struct {
	MA          map[string][]float64
	MACD        *MACDResult
	KDJ         *KDJResult
	BOLL        *BOLLResult
	RSI         map[string][]float64
	VolumeRatio *VolumeRatioResult
}

type MACDResult struct {
	DIF    []float64
	DEA    []float64
	Hist   []float64
	Fast   int
	Slow   int
	Signal int
}

type KDJResult struct {
	K  []float64
	D  []float64
	J  []float64
	N  int
	M1 int
	M2 int
}

type BOLLResult struct {
	Upper  []float64
	Middle []float64
	Lower  []float64
	N      int
	K      float64
}

type VolumeRatioResult struct {
	Current float64
	Avg5    float64
	Ratio   float64
	Signal  string
}

type IndicatorFunc func(klines []KlineInput) (interface{}, error)
