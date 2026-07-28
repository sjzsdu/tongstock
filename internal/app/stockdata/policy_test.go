package stockdata

import (
	"context"
	"testing"
	"time"
)

type calendarFunc func(time.Time) bool

func (fn calendarFunc) IsTradingDay(_ context.Context, _ string, day time.Time) (bool, error) {
	return fn(day), nil
}

func TestQuoteFreshnessAcrossMarketPhases(t *testing.T) {
	calendar := calendarFunc(func(day time.Time) bool {
		return day.Weekday() != time.Saturday && day.Weekday() != time.Sunday
	})
	policy := NewMarketFreshnessPolicy(calendar, time.Local)
	cases := []struct {
		name   string
		now    time.Time
		source time.Time
		fresh  bool
	}{
		{
			name:   "pre-market uses previous session",
			now:    time.Date(2026, 7, 28, 8, 30, 0, 0, time.Local),
			source: time.Date(2026, 7, 27, 15, 1, 0, 0, time.Local),
			fresh:  true,
		},
		{
			name:   "in-session realtime quote expires",
			now:    time.Date(2026, 7, 28, 10, 0, 30, 0, time.Local),
			source: time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local),
			fresh:  false,
		},
		{
			name:   "post-market requires current session",
			now:    time.Date(2026, 7, 28, 16, 0, 0, 0, time.Local),
			source: time.Date(2026, 7, 28, 15, 1, 0, 0, time.Local),
			fresh:  true,
		},
		{
			name:   "weekend accepts Friday snapshot",
			now:    time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local),
			source: time.Date(2026, 7, 31, 15, 1, 0, 0, time.Local),
			fresh:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := policy.Evaluate(context.Background(), tc.now,
				DataSpec{Type: DataQuote, Market: "sh", Code: "600000"},
				Coverage{Exists: true, SourceUpdatedAt: tc.source})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Fresh != tc.fresh {
				t.Fatalf("fresh = %v, reason = %s", decision.Fresh, decision.Reason)
			}
		})
	}
}

func TestKlineFutureRangeClampsToCompletedTradingDay(t *testing.T) {
	holiday := time.Date(2026, 7, 30, 0, 0, 0, 0, time.Local)
	calendar := calendarFunc(func(day time.Time) bool {
		return day.Weekday() != time.Saturday && day.Weekday() != time.Sunday &&
			day.Format("20060102") != holiday.Format("20060102")
	})
	policy := NewMarketFreshnessPolicy(calendar, time.Local)
	wednesday := time.Date(2026, 7, 29, 0, 0, 0, 0, time.Local)
	decision, err := policy.Evaluate(context.Background(),
		time.Date(2026, 7, 30, 12, 0, 0, 0, time.Local),
		DataSpec{
			Type: DataKline, Market: "sh", Code: "600000",
			Start: wednesday, End: time.Date(2026, 8, 5, 0, 0, 0, 0, time.Local),
		},
		Coverage{Exists: true, Start: wednesday, End: wednesday, Points: []time.Time{wednesday}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Fresh || len(decision.MissingRanges) != 0 {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestFreshnessPolicyPassesEachExchangeToTradingCalendar(t *testing.T) {
	var seen []string
	calendar := tradingCalendarRecorder{seen: &seen}
	policy := NewMarketFreshnessPolicy(calendar, time.Local)
	now := time.Date(2026, 7, 28, 10, 0, 10, 0, time.Local)
	for _, market := range []string{"sh", "sz", "bj"} {
		_, err := policy.Evaluate(context.Background(), now,
			DataSpec{Type: DataQuote, Market: market, Code: "000001"},
			Coverage{Exists: true, SourceUpdatedAt: now.Add(-5 * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 3 || seen[0] != "sh" || seen[1] != "sz" || seen[2] != "bj" {
		t.Fatalf("calendar markets = %v", seen)
	}
}

type tradingCalendarRecorder struct {
	seen *[]string
}

func (c tradingCalendarRecorder) IsTradingDay(_ context.Context, market string, _ time.Time) (bool, error) {
	*c.seen = append(*c.seen, market)
	return true, nil
}
