package ta

import "sync"

type IndicatorConfig struct {
	MA   []int
	MACD *MACDConfig
	KDJ  *KDJConfig
	BOLL *BOLLConfig
	RSI  []int
}

type MACDConfig struct {
	Fast   int
	Slow   int
	Signal int
}

type KDJConfig struct {
	N  int
	M1 int
	M2 int
}

type BOLLConfig struct {
	N int
	K float64
}

func DefaultConfig() *IndicatorConfig {
	return &IndicatorConfig{
		MA:   []int{5, 10, 20, 60},
		MACD: &MACDConfig{Fast: 12, Slow: 26, Signal: 9},
		KDJ:  &KDJConfig{N: 9, M1: 3, M2: 3},
		BOLL: &BOLLConfig{N: 20, K: 2.0},
		RSI:  []int{6, 14},
	}
}

func Calculate(klines []KlineInput, cfg *IndicatorConfig) *IndicatorResult {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	result := &IndicatorResult{
		MA:  make(map[string][]float64),
		RSI: make(map[string][]float64),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, period := range cfg.MA {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			data := SMA(klines, p)
			mu.Lock()
			result.MA[itoa(p)] = data
			mu.Unlock()
		}(period)
	}

	for _, period := range cfg.RSI {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			data := CalcRSI(klines, p)
			mu.Lock()
			result.RSI[itoa(p)] = data
			mu.Unlock()
		}(period)
	}

	if cfg.MACD != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := CalcMACD(klines, cfg.MACD.Fast, cfg.MACD.Slow, cfg.MACD.Signal)
			mu.Lock()
			result.MACD = data
			mu.Unlock()
		}()
	}

	if cfg.KDJ != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := CalcKDJ(klines, cfg.KDJ.N, cfg.KDJ.M1, cfg.KDJ.M2)
			mu.Lock()
			result.KDJ = data
			mu.Unlock()
		}()
	}

	if cfg.BOLL != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := CalcBOLL(klines, cfg.BOLL.N, cfg.BOLL.K)
			mu.Lock()
			result.BOLL = data
			mu.Unlock()
		}()
	}

	wg.Wait()
	return result
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
