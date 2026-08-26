package stockdata

import (
	"context"
	"fmt"
	"time"
)

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type WeekdayCalendar struct{}

func (WeekdayCalendar) IsTradingDay(_ context.Context, _ string, day time.Time) (bool, error) {
	return day.Weekday() != time.Saturday && day.Weekday() != time.Sunday, nil
}

type MarketFreshnessPolicy struct {
	Calendar TradingCalendar
	Location *time.Location
}

func NewMarketFreshnessPolicy(calendar TradingCalendar, location *time.Location) *MarketFreshnessPolicy {
	if calendar == nil {
		calendar = WeekdayCalendar{}
	}
	if location == nil {
		location = time.Local
	}
	return &MarketFreshnessPolicy{Calendar: calendar, Location: location}
}

func (p *MarketFreshnessPolicy) Evaluate(ctx context.Context, now time.Time, spec DataSpec, coverage Coverage) (FreshnessDecision, error) {
	now = now.In(p.Location)
	if !coverage.Exists {
		return FreshnessDecision{
			Fresh: false, Reason: "missing",
			MissingRanges: []TimeRange{{Start: spec.Start, End: spec.End}},
		}, nil
	}
	switch spec.Type {
	case DataQuote:
		return p.evaluateQuote(ctx, now, spec, coverage)
	case DataFinance:
		fresh := !coverage.SourceUpdatedAt.IsZero() && now.Sub(coverage.SourceUpdatedAt) <= 7*24*time.Hour
		reason := "finance_within_reporting_ttl"
		if !fresh {
			reason = "finance_reporting_ttl_expired"
		}
		return FreshnessDecision{Fresh: fresh, Reason: reason, AsOf: coverage.SourceUpdatedAt}, nil
	case DataKline:
		return p.evaluateKline(ctx, now, spec, coverage)
	default:
		return FreshnessDecision{}, fmt.Errorf("unsupported data type %q", spec.Type)
	}
}

func (p *MarketFreshnessPolicy) evaluateQuote(ctx context.Context, now time.Time, spec DataSpec, coverage Coverage) (FreshnessDecision, error) {
	trading, err := p.Calendar.IsTradingDay(ctx, spec.Market, now)
	if err != nil {
		return FreshnessDecision{}, err
	}
	minute := now.Hour()*60 + now.Minute()
	inSession := trading && ((minute >= 9*60+15 && minute <= 11*60+35) || (minute >= 12*60+55 && minute <= 15*60+5))
	if inSession {
		fresh := now.Sub(coverage.SourceUpdatedAt) <= 15*time.Second
		reason := "quote_within_realtime_ttl"
		if !fresh {
			reason = "quote_realtime_ttl_expired"
		}
		return FreshnessDecision{Fresh: fresh, Reason: reason, AsOf: coverage.SourceUpdatedAt}, nil
	}
	lastCompleted, err := p.latestCompletedTradingDay(ctx, spec.Market, now)
	if err != nil {
		return FreshnessDecision{}, err
	}
	sourceDay := dayOnly(coverage.SourceUpdatedAt, p.Location)
	fresh := !sourceDay.Before(lastCompleted)
	reason := "quote_latest_completed_session"
	if !fresh {
		reason = "quote_previous_session"
	}
	return FreshnessDecision{Fresh: fresh, Reason: reason, AsOf: coverage.SourceUpdatedAt}, nil
}

func (p *MarketFreshnessPolicy) evaluateKline(ctx context.Context, now time.Time, spec DataSpec, coverage Coverage) (FreshnessDecision, error) {
	start := dayOnly(spec.Start, p.Location)
	end := dayOnly(spec.End, p.Location)
	latest, err := p.latestCompletedTradingDay(ctx, spec.Market, now)
	if err != nil {
		return FreshnessDecision{}, err
	}
	// A range-less request asks whether the series is current, not whether the
	// stock traded on every exchange session since listing. Historical absences
	// can be legitimate suspensions and must not make a freshly synced series
	// permanently stale.
	if start.IsZero() && end.IsZero() {
		start, end = latest, latest
	} else if start.IsZero() {
		start = coverage.Start
	}
	if end.IsZero() || end.After(latest) {
		end = latest
	}
	if start.After(end) {
		return FreshnessDecision{Fresh: true, Reason: "future_range_has_no_completed_bars", AsOf: coverage.End}, nil
	}

	have := make(map[string]struct{}, len(coverage.Points))
	for _, point := range coverage.Points {
		have[point.In(p.Location).Format("20060102")] = struct{}{}
	}
	var ranges []TimeRange
	var current *TimeRange
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		trading, err := p.Calendar.IsTradingDay(ctx, spec.Market, day)
		if err != nil {
			return FreshnessDecision{}, err
		}
		if !trading {
			continue
		}
		if _, ok := have[day.Format("20060102")]; !ok {
			if current == nil {
				current = &TimeRange{Start: day, End: day}
			} else {
				current.End = day
			}
		} else if current != nil {
			ranges = append(ranges, *current)
			current = nil
		}
	}
	if current != nil {
		ranges = append(ranges, *current)
	}
	reason := "kline_coverage_complete"
	if len(ranges) > 0 {
		reason = "kline_missing_ranges"
	}
	return FreshnessDecision{
		Fresh: len(ranges) == 0, Reason: reason, MissingRanges: ranges, AsOf: coverage.End,
	}, nil
}

func (p *MarketFreshnessPolicy) latestCompletedTradingDay(ctx context.Context, market string, now time.Time) (time.Time, error) {
	day := dayOnly(now, p.Location)
	trading, err := p.Calendar.IsTradingDay(ctx, market, day)
	if err != nil {
		return time.Time{}, err
	}
	// Daily bars are considered complete shortly after the closing auction.
	if trading && (now.Hour() > 15 || (now.Hour() == 15 && now.Minute() >= 10)) {
		return day, nil
	}
	for day = day.AddDate(0, 0, -1); ; day = day.AddDate(0, 0, -1) {
		trading, err = p.Calendar.IsTradingDay(ctx, market, day)
		if err != nil {
			return time.Time{}, err
		}
		if trading {
			return day, nil
		}
	}
}

func dayOnly(value time.Time, location *time.Location) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	value = value.In(location)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}
