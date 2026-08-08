package discoveryapp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func newTestStore(t *testing.T) *storage.Storage {
	t.Helper()
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open in-memory storage: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// insertDaily 插入一只股票从 start 到 end（含）的每个自然日一条有效日 K。
func insertDaily(t *testing.T, s *storage.Storage, code, start, end string) {
	t.Helper()
	for d := start; d <= end; {
		if _, err := s.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES (?, 9, ?, 10, 11, 9, 10.5, 1000, 10000)`, code, d); err != nil {
			t.Fatalf("insert kline %s %s: %v", code, d, err)
		}
		next, err := nextDay(d)
		if err != nil {
			t.Fatalf("next day: %v", err)
		}
		d = next
	}
}

func nextDay(d string) (string, error) {
	ts, err := parseDay(d)
	if err != nil {
		return "", err
	}
	ts = ts.AddDate(0, 0, 1)
	return fmt.Sprintf("%04d%02d%02d", ts.Year(), ts.Month(), ts.Day()), nil
}

func parseDay(d string) (time.Time, error) {
	return time.Parse("20060102", d)
}

func TestResolveRealRange_BasicIntersection(t *testing.T) {
	s := newTestStore(t)
	insertDaily(t, s, "000001", "20190102", "20260801")
	insertDaily(t, s, "600519", "20190304", "20260801")

	start, end, used, err := resolveRealRange(context.Background(), s, []string{"000001", "600519"})
	if err != nil {
		t.Fatalf("resolveRealRange: %v", err)
	}
	if start != "2022-08-01" || end != "2026-08-01" {
		t.Errorf("range = %s..%s, want 2022-08-01..2026-08-01 (end-4y)", start, end)
	}
	if len(used) != 2 {
		t.Errorf("used = %v, want both codes", used)
	}
}

func TestResolveRealRange_DropsShortHistory(t *testing.T) {
	s := newTestStore(t)
	insertDaily(t, s, "000001", "20190102", "20260801")
	insertDaily(t, s, "301111", "20260601", "20260801") // 次新股，仅 61 天

	start, end, used, err := resolveRealRange(context.Background(), s, []string{"000001", "301111"})
	if err != nil {
		t.Fatalf("resolveRealRange: %v", err)
	}
	if len(used) != 1 || used[0] != "000001" {
		t.Errorf("used = %v, want only the long-history stock", used)
	}
	if start >= end {
		t.Errorf("range %s..%s should be valid after dropping short history", start, end)
	}
}

func TestResolveRealRange_AllShortHistoryFallsBackToSingle(t *testing.T) {
	s := newTestStore(t)
	insertDaily(t, s, "301111", "20260601", "20260701")
	insertDaily(t, s, "301222", "20260615", "20260720")

	// 两只次新股共享区间不足 300 天：剔除后回退到单只短历史股票研究。
	_, _, used, err := resolveRealRange(context.Background(), s, []string{"301111", "301222"})
	if err != nil {
		t.Fatalf("resolveRealRange: %v", err)
	}
	if len(used) != 1 {
		t.Errorf("used = %v, want single fallback stock", used)
	}
}

func TestResolveRealRange_NoDataAtAll(t *testing.T) {
	s := newTestStore(t)
	if _, _, _, err := resolveRealRange(context.Background(), s, []string{"301111", "301222"}); err == nil {
		t.Fatal("expected error when no stock has any valid data")
	}
}

func TestResolveRealRange_SkipsMissingCode(t *testing.T) {
	s := newTestStore(t)
	insertDaily(t, s, "000001", "20190102", "20260801")

	start, end, used, err := resolveRealRange(context.Background(), s, []string{"000001", "999999"})
	if err != nil {
		t.Fatalf("resolveRealRange: %v", err)
	}
	if len(used) != 1 || used[0] != "000001" {
		t.Errorf("used = %v, want only the stock with data", used)
	}
	if start >= end {
		t.Errorf("range %s..%s invalid", start, end)
	}
}
