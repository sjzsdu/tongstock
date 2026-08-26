package stockdata

import (
	"fmt"
	"math"
	"time"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

const maxKlinePrice = 1_000_000.0

// validateKlineRecord is the shared trust boundary for upstream and persisted
// K-line data. Keeping it in the unified data layer prevents alternate write
// paths from bypassing the legacy KlineStore checks.
func validateKlineRecord(item *protocol.Kline, now time.Time) error {
	if item == nil {
		return fmt.Errorf("nil record")
	}
	if now.IsZero() {
		now = time.Now()
	}
	minDate := time.Date(1990, 1, 1, 0, 0, 0, 0, time.Local)
	maxDate := now.In(time.Local).AddDate(0, 0, 1)
	if item.Time.IsZero() || item.Time.Before(minDate) || item.Time.After(maxDate) {
		return fmt.Errorf("invalid date %q", item.Time.Format("20060102"))
	}

	prices := []struct {
		name  string
		value float64
	}{
		{"open", item.Open}, {"high", item.High}, {"low", item.Low}, {"close", item.Close},
	}
	for _, price := range prices {
		if price.value <= 0 || price.value > maxKlinePrice ||
			math.IsNaN(price.value) || math.IsInf(price.value, 0) {
			return fmt.Errorf("invalid %s", price.name)
		}
	}
	if item.High < item.Low || item.High < item.Open || item.High < item.Close ||
		item.Low > item.Open || item.Low > item.Close {
		return fmt.Errorf("invalid OHLC relationship")
	}
	for _, metric := range []struct {
		name  string
		value float64
	}{{"volume", item.Volume}, {"amount", item.Amount}} {
		if metric.value < 0 || math.IsNaN(metric.value) || math.IsInf(metric.value, 0) {
			return fmt.Errorf("invalid %s", metric.name)
		}
	}
	return nil
}
