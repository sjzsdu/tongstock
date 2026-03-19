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

func isDailyKline(ktype uint8) bool {
	return ktype == 4 || ktype == 9
}
