package tdx

import (
	"math"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

func TestKlineStoreReadsDatesAndSkipsInvalid(t *testing.T) {
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: t.TempDir() + "/kline.db"})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer s.Close()

	store, err := NewKlineStore(s)
	if err != nil {
		t.Fatalf("init store: %v", err)
	}

	_, err = s.DB().Exec(`INSERT INTO kline (code, ktype, date, open, high, low, close, volume, amount) VALUES ('000001', 9, '20260621', 1, 2, 1, 2, 100, 1000), ('000001', 9, '20260622', 2, 3, 2, 3, 200, 2000)`)
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
}

func TestSaveKlineRejectsInvalidRecords(t *testing.T) {
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: t.TempDir() + "/save-kline.db"})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer s.Close()

	store, err := NewKlineStore(s)
	if err != nil {
		t.Fatalf("init store: %v", err)
	}

	validDate := time.Date(2026, time.June, 22, 0, 0, 0, 0, time.Local)
	klines := []*protocol.Kline{
		{Time: validDate, Open: 2, High: 3, Low: 1, Close: 2.5, Volume: 100, Amount: 1000},
		{Time: validDate.AddDate(0, 0, 1), Open: 0, High: 3, Low: 1, Close: 2.5},    // Invalid: Open=0
		{Time: validDate.AddDate(0, 0, 2), Open: 2, High: 1, Low: 3, Close: 2.5},    // Invalid: High<Low
		{Time: validDate.AddDate(0, 0, 3), Open: 2, High: math.Inf(1), Low: 1, Close: 2.5}, // Invalid: Inf
	}
	if err := store.SaveKline("000001", 9, klines); err != nil {
		t.Fatalf("SaveKline: %v", err)
	}

	got, err := store.GetKline("000001", 9, "", "")
	if err != nil {
		t.Fatalf("GetKline: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
}
