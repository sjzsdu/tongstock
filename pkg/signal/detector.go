package signal

import (
	"sync"

	"github.com/sjzsdu/tongstock/pkg/ta"
)

func Detect(code string, klines []ta.KlineInput, result *ta.IndicatorResult, opt *DetectOptions) []Signal {
	if opt == nil {
		opt = DefaultDetectOptions()
	}

	// 首先判断当前趋势方向
	trend := TrendUnknown
	if result.MA != nil {
		trend = detectTrend(klines, result.MA)
	}

	var signals []Signal
	var mu sync.Mutex
	var wg sync.WaitGroup

	if opt.EnableMACD && result.MACD != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := detectMACDSignals(code, klines, result.MACD)
			// 应用趋势过滤
			s = filterSignalsByTrend(s, trend)
			mu.Lock()
			signals = append(signals, s...)
			mu.Unlock()
		}()
	}

	if opt.EnableKDJ && result.KDJ != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := detectKDJSignals(code, klines, result.KDJ)
			// 应用趋势过滤
			s = filterSignalsByTrend(s, trend)
			mu.Lock()
			signals = append(signals, s...)
			mu.Unlock()
		}()
	}

	if opt.EnableBOLL && result.BOLL != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := detectBOLLSignals(code, klines, result.BOLL)
			mu.Lock()
			signals = append(signals, s...)
			mu.Unlock()
		}()
	}

	if opt.EnableMA && result.MA != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := detectMASignals(code, klines, result.MA)
			// 应用趋势过滤
			s = filterSignalsByTrend(s, trend)
			mu.Lock()
			signals = append(signals, s...)
			mu.Unlock()
		}()
	}

	if opt.EnableRSI && result.RSI != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := detectRSISignals(code, klines, result.RSI)
			mu.Lock()
			signals = append(signals, s...)
			mu.Unlock()
		}()
	}

	wg.Wait()
	return signals
}
