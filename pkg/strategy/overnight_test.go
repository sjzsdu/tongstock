package strategy

import (
	"math"
	"testing"
	"time"
)

// 服务器部署在 UTC 时，time.Now() 的时区是 UTC。
// 14:30 CST = 06:30 UTC，横幅判断必须按北京时间而不是服务器本地时间。
func TestIsOvernightTimeUsesBeijingTimezone(t *testing.T) {
	utc := time.FixedZone("UTC", 0)
	cases := []struct {
		utcTime  string
		expected bool
	}{
		{"2026-10-01T06:29:00Z", false}, // 北京时间 14:29，未到 14:30
		{"2026-10-01T06:30:00Z", true},  // 北京时间 14:30，恰好在界上
		{"2026-10-01T07:00:00Z", true},  // 北京时间 15:00
		{"2026-10-01T15:59:00Z", true},  // 北京时间 23:59，仍在“14:30之后”区间
		{"2026-10-01T16:00:00Z", false}, // 北京时间次日 00:00
		{"2026-10-01T04:00:00Z", false}, // 北京时间 12:00
	}
	for _, tc := range cases {
		at, err := time.Parse(time.RFC3339, tc.utcTime)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.utcTime, err)
		}
		inUTC := at.In(utc)
		if got := IsOvernightTime(inUTC); got != tc.expected {
			t.Errorf("IsOvernightTime(%s) = %v, want %v (北京时间 %s)",
				inUTC.Format(time.RFC3339), got, tc.expected, BeijingTime(inUTC).Format("15:04"))
		}
	}
}

// 北京时间 14:30（服务器本地时间同样是 14:30，如部署在国内）也不能误判。
func TestIsOvernightTimeFromBeijingClock(t *testing.T) {
	at := time.Date(2026, 10, 1, 14, 30, 0, 0, BeijingLocation())
	if !IsOvernightTime(at) {
		t.Errorf("IsOvernightTime(14:30 CST) = false, want true")
	}
}

func TestBeijingNowConvertsToFixedPlusEight(t *testing.T) {
	now := BeijingNow()
	if got := now.Location().String(); got != "CST" {
		t.Fatalf("BeijingNow location = %q, want CST", got)
	}
	// 与 UTC 的时差固定为 8 小时（A股无夏令时）。
	_, offset := now.Zone()
	if offset != 8*3600 {
		t.Fatalf("offset = %d seconds, want %d", offset, 8*3600)
	}
	if math.Abs(float64(time.Until(now))) > float64(time.Minute) {
		t.Fatalf("BeijingNow drifted from wall clock: %v", time.Until(now))
	}
}
