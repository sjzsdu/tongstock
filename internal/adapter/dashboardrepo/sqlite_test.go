package dashboardrepo

import (
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func newTestReader(t *testing.T) (*KlineDateReader, *storage.Storage) {
	t.Helper()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "dashboard.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reader, err := NewKlineDateReader(store)
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	return reader, store
}

func insertBar(t *testing.T, store *storage.Storage, code, date string, close float64) {
	t.Helper()
	_, err := store.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)
		VALUES(?,?,?,?,?,?,?,?,?)`, code, klineTypeDaily, date, 10, 11, 9, close, 1000, 10500)
	if err != nil {
		t.Fatalf("insert kline %s %s: %v", code, date, err)
	}
}

func TestLatestKlineDateEmptyDatabaseReturnsEmpty(t *testing.T) {
	reader, _ := newTestReader(t)
	got, err := reader.LatestKlineDate()
	if err != nil {
		t.Fatalf("LatestKlineDate: %v", err)
	}
	if got != "" {
		t.Fatalf("LatestKlineDate = %q, want empty", got)
	}
}

func TestLatestKlineDateReturnsNewestValidDate(t *testing.T) {
	reader, store := newTestReader(t)
	insertBar(t, store, "600000", "20240102", 10.5)
	insertBar(t, store, "600000", "20240103", 10.8)
	insertBar(t, store, "000001", "20240103", 9.5)
	// 非日线 ktype 不能影响日线水位。
	if _, err := store.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)
		VALUES('600000',1,'20240105',10,11,9,10.5,1000,10500)`); err != nil {
		t.Fatalf("insert non-daily kline: %v", err)
	}

	got, err := reader.LatestKlineDate()
	if err != nil {
		t.Fatalf("LatestKlineDate: %v", err)
	}
	if got != "2024-01-03" {
		t.Fatalf("LatestKlineDate = %q, want 2024-01-03", got)
	}
}

// 视图里最新交易日的日线若被修订成无效值，快路径会校验失败并回退到原查询，
// 返回值必须与改动前的全量查询一致：退到上一个有效交易日。
func TestLatestKlineDateFallsBackWhenNewestBarsAreInvalid(t *testing.T) {
	reader, store := newTestReader(t)
	insertBar(t, store, "600000", "20240102", 10.5)
	insertBar(t, store, "600000", "20240103", 10.8)
	// 修订行把 20240103 的 OHLC 改成无效值，视图读到的 open/close 变成 0。
	if _, err := store.DB().Exec(`INSERT INTO kline_revision(code,ktype,date,version,open,high,low,close,volume,amount,reason,created_at_ns)
		VALUES('600000',?,?,2,0,0,0,0,0,0,'quarantined',1)`, klineTypeDaily, "20240103"); err != nil {
		t.Fatalf("insert kline_revision: %v", err)
	}

	got, err := reader.LatestKlineDate()
	if err != nil {
		t.Fatalf("LatestKlineDate: %v", err)
	}
	if got != "2024-01-02" {
		t.Fatalf("LatestKlineDate = %q, want fallback 2024-01-02", got)
	}

	// 回退分支与原查询同源，直接校验原实现给出同一答案。
	viaView, err := reader.latestFromView()
	if err != nil {
		t.Fatalf("latestFromView: %v", err)
	}
	if viaView != got {
		t.Fatalf("latestFromView = %q, want %q", viaView, got)
	}
}

func TestLatestKlineDateRejectsUnexpectedFormatViaFallback(t *testing.T) {
	reader, store := newTestReader(t)
	// 历史遗留的带横线日期：基表 MAX 得到 8 位以外的字符串，快路径不适用，
	// 回退到原查询（原查询先 REPLACE 再校验长度）。写入前先摘掉校验触发器，
	// 模拟早年没有 trg_kline_validate_insert 时落库的历史行。
	if _, err := store.DB().Exec(`DROP TRIGGER trg_kline_validate_insert`); err != nil {
		t.Fatalf("drop validate trigger: %v", err)
	}
	if _, err := store.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)
		VALUES('600000',?,'2024-01-02',10,11,9,10.5,1000,10500)`, klineTypeDaily); err != nil {
		t.Fatalf("insert dashed kline: %v", err)
	}

	got, err := reader.LatestKlineDate()
	if err != nil {
		t.Fatalf("LatestKlineDate: %v", err)
	}
	if got != "2024-01-02" {
		t.Fatalf("LatestKlineDate = %q, want 2024-01-02", got)
	}
}
