package tdx

import "time"

var chinaLoc = time.FixedZone("CST", 8*3600)

func marketNow() time.Time {
	return time.Now().In(chinaLoc)
}

func isWeekday(t time.Time) bool {
	d := t.Weekday()
	return d != time.Saturday && d != time.Sunday
}

func isDuringTradingHours(t time.Time) bool {
	if !isWeekday(t) {
		return false
	}
	mins := t.Hour()*60 + t.Minute()
	return (mins >= 570 && mins <= 690) || (mins >= 780 && mins <= 900)
}

func isAfterMarketClose(t time.Time) bool {
	if !isWeekday(t) {
		return false
	}
	return t.Hour()*60+t.Minute() >= 900
}

func prevWeekday(t time.Time) time.Time {
	t = t.AddDate(0, 0, -1)
	for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

func lastCompleteTradingDate(now time.Time) string {
	if isAfterMarketClose(now) {
		return now.Format("20060102")
	}
	return prevWeekday(now).Format("20060102")
}

func isIntradayKline(ktype uint8) bool {
	return ktype >= 0 && ktype <= 3 || ktype == 7 || ktype == 8
}

func isDailyKline(ktype uint8) bool {
	return ktype == 4 || ktype == 9
}

func isWeeklyKline(ktype uint8) bool {
	return ktype == 5
}

func isMonthlyKline(ktype uint8) bool {
	return ktype == 6
}

func isQuarterlyKline(ktype uint8) bool {
	return ktype == 10
}

func isYearlyKline(ktype uint8) bool {
	return ktype == 11
}

func weekNumber(t time.Time) int {
	_, w := t.ISOWeek()
	return w
}

func monthNumber(t time.Time) int {
	return int(t.Month())
}

func quarterNumber(t time.Time) int {
	return (int(t.Month())-1)/3 + 1
}

func yearNumber(t time.Time) int {
	return t.Year()
}

func lastCompleteWeek(now time.Time) string {
	if now.Weekday() == time.Sunday && now.Hour() >= 15 {
		return now.Format("20060102")
	}
	daysAgo := int(now.Weekday())
	if daysAgo == 0 {
		daysAgo = 7
	}
	lastFri := now.AddDate(0, 0, -daysAgo-2)
	return lastFri.Format("20060102")
}

func lastCompleteMonth(now time.Time) string {
	if now.Day() > 20 {
		return now.Format("200601")
	}
	lastMonth := now.AddDate(0, -1, 0)
	return lastMonth.Format("200601")
}

func lastCompleteQuarter(now time.Time) string {
	q := quarterNumber(now)
	y := yearNumber(now)
	if now.Month() > time.Month((q-1)*3+3) && now.Day() > 20 {
		return now.Format("2006")
	}
	prevQ := q - 1
	if prevQ == 0 {
		prevQ = 4
		y--
	}
	return time.Date(y, time.Month((prevQ-1)*3+3), 1, 0, 0, 0, 0, time.UTC).Format("2006")
}

func lastCompleteYear(now time.Time) string {
	if now.Month() == time.December && now.Day() > 25 {
		return now.Format("2006")
	}
	return time.Date(now.Year()-1, 12, 31, 0, 0, 0, 0, time.UTC).Format("2006")
}
