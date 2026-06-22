package tdx

import (
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/db"
)

func TestParseKlineStoreDateSupportsCompactAndDashed(t *testing.T) {
	for _, tc := range []struct {
		name string
		date string
	}{
		{name: "compact", date: "20260622"},
		{name: "dashed", date: "2026-06-22"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseKlineStoreDate(tc.date, time.Local)
			if err != nil {
				t.Fatalf("parseKlineStoreDate(%q) returned error: %v", tc.date, err)
			}
			if got.Year() != 2026 || got.Month() != time.June || got.Day() != 22 {
				t.Fatalf("parseKlineStoreDate(%q) = %s, want 2026-06-22", tc.date, got.Format("2006-01-02"))
			}
		})
	}
}

func TestParseKlineStoreDateRejectsInvalid(t *testing.T) {
	if _, err := parseKlineStoreDate("not-a-date", time.Local); err == nil {
		t.Fatal("parseKlineStoreDate accepted invalid date")
	}
}

func TestKlineStoreReadsLegacyDashedDates(t *testing.T) {
	database, err := db.OpenSQLite(t.TempDir() + "/kline.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer database.Close()

	store := &KlineStore{db: database, loc: time.Local}
	if err := store.init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	_, err = database.Exec(`
		INSERT INTO kline (code, ktype, date, open, high, low, close, volume, amount) VALUES
		('000001', 9, '2026-06-21', 1, 2, 1, 2, 100, 1000),
		('000001', 9, '20260622', 2, 3, 2, 3, 200, 2000)
	`)
	if err != nil {
		t.Fatalf("insert klines: %v", err)
	}

	latest, err := store.GetLatestDate("000001", 9)
	if err != nil {
		t.Fatalf("GetLatestDate: %v", err)
	}
	if latest != "20260622" {
		t.Fatalf("latest = %q, want 20260622", latest)
	}

	klines, err := store.GetKline("000001", 9, "", "")
	if err != nil {
		t.Fatalf("GetKline: %v", err)
	}
	if len(klines) != 2 {
		t.Fatalf("len(klines) = %d, want 2", len(klines))
	}
	if klines[0].Time.IsZero() || klines[1].Time.IsZero() {
		t.Fatalf("expected parsed non-zero dates, got %v and %v", klines[0].Time, klines[1].Time)
	}
}
