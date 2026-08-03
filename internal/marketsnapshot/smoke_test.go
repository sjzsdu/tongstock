package marketsnapshot_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func openTestDB(t *testing.T) *storage.Storage {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ms.db")
	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	// 填充工作日表（2024-01-02 是周二，作为工作日）
	td := time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local).Unix()
	if _, err := db.DB().Exec(`INSERT OR IGNORE INTO workday (unix, date) VALUES (?, '2024-01-02')`, td); err != nil {
		t.Fatal(err)
	}
	// 2 只股票的 stockinfo + kline_sync_state + kline
	seedStockinfo := `
	INSERT INTO stockinfo (code, name, exchange, updated_at, ipo_date_txt, delist_date, st_flag) VALUES
	('000001','平安银行','SZ', 1700000000, '2000-01-01','',0),
	('000002','万科A','SZ', 1700000000, '2000-01-02','',0),
	('300001','ST测试','SZ', 1700000000, '2023-06-01','',1);
	`
	if _, err := db.DB().Exec(seedStockinfo); err != nil {
		t.Fatal(err)
	}
	// 填充 30 天 K 线（000001 有 30 根，000002 有 29 根（最后一天缺），600000 全缺，300001 有 30 根但是 ST 股被排除）
	stmt, err := db.DB().Prepare(`INSERT OR IGNORE INTO kline
		(code, ktype, date, open, high, low, close, volume, amount) VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	insertSync := func(code, last string, rows int) {
		_, err := db.DB().Exec(`INSERT OR REPLACE INTO kline_sync_state
			(code, ktype, first_date, last_date, row_count, last_sync_at, status)
			VALUES (?, 9, '2023-12-01', ?, ?, ?, 'ok')`, code, last, rows, td)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 30; i++ {
		d := time.Date(2023, 12, 4, 0, 0, 0, 0, time.Local).AddDate(0, 0, i).Format("20060102")
		// 000001: 30 bars
		price := 10 + 0.1*float64(i)
		if _, err := stmt.Exec("000001", d, price, price+0.3, price-0.2, price+0.1, 1e6, price*1e6); err != nil {
			t.Fatal(err)
		}
		// 000002: 前 29 天有
		if i < 29 {
			price2 := 20 + 0.12*float64(i)
			if _, err := stmt.Exec("000002", d, price2, price2+0.5, price2-0.2, price2+0.1, 9e5, price2*9e5); err != nil {
				t.Fatal(err)
			}
		}
		// 300001: 30 bars
		if _, err := stmt.Exec("300001", d, 5.0, 5.2, 4.9, 5.1, 5e5, 5.0*5e5); err != nil {
			t.Fatal(err)
		}
	}
	insertSync("000001", "2024-01-02", 30)
	insertSync("000002", "2023-12-29", 29)
	insertSync("300001", "2024-01-02", 30)
	return db
}

// TestBuilderEndToEnd 验证从 universe 构建 → 水位 → save → freeze → 快照哈希
// 满足验收标准 1,2,4,5。
func TestBuilderEndToEnd(t *testing.T) {
	db := openTestDB(t)
	up := marketsnapshotrepo.NewSQLiteUniverseProvider(db)
	wp := marketsnapshotrepo.NewSQLiteWatermarkProvider(db)
	cal := marketsnapshotrepo.NewSQLiteTradingCalendar(db)
	fe := marketsnapshotrepo.NewSQLiteFeatureEngine(db)
	repo, err := marketsnapshotrepo.New(db)
	if err != nil {
		t.Fatal(err)
	}
	date := "2024-01-02"
	_ = filepath.Join
	_ = json.Marshal
	b := marketsnapshot.NewBuilder(up, wp, cal)
	// 故意让 000002 gap_days > 0，覆盖 partial 路径
	s, err := b.Build(date, marketsnapshot.DefaultUniverseUsable())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if s.SnapshotDate != date {
		t.Fatalf("snapshot date: %s", s.SnapshotDate)
	}
	// universe_usable 应当包含 000001 / 000002，排除 300001（ST）
	var has1, has2 bool
	for _, m := range s.UniverseMembers {
		switch m.Code {
		case "000001":
			has1 = true
		case "000002":
			has2 = true
		case "300001":
			t.Fatal("300001 是 ST 不应出现在 selected 中")
		}
	}
	if !has1 || !has2 {
		t.Fatalf("universe 缺 000001=%v 000002=%v", has1, has2)
	}
	if s.ExpectedKlineCodes != 2 {
		t.Fatalf("expected kline codes = %d, want 2", s.ExpectedKlineCodes)
	}
	// 000001 ready，000002 不 ready（最后一天缺），所以覆盖率 50% < 0.8 → failed
	if s.Status != marketsnapshot.StatusFailed {
		t.Fatalf("status = %s want failed (coverage=%.2f%%)", s.Status, s.CoveragePct*100)
	}
	// 降低阈值，让状态变为 partial
	b.CoverageThreshold = 0.5
	b.MaxGappedCodes = 100
	s2, err := b.Build(date, marketsnapshot.DefaultUniverseUsable())
	if err != nil {
		t.Fatal(err)
	}
	if s2.Status != marketsnapshot.StatusReady {
		t.Fatalf("降低阈值后 status=%s, reason=%s", s2.Status, s2.ReadinessReason)
	}
	// 持久化 + 冻结 + 再加载
	if err := repo.SaveMarketSnapshot(s2); err != nil {
		t.Fatal(err)
	}
	if err := repo.FreezeMarketSnapshot(s2.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.LoadMarketSnapshot(s2.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UniverseHash != s2.UniverseHash || loaded.ContentHash != s2.ContentHash || !loaded.Frozen {
		t.Fatalf("加载结果不匹配。uh=%v/%v ch=%v/%v frozen=%v",
			loaded.UniverseHash, s2.UniverseHash, loaded.ContentHash, s2.ContentHash, loaded.Frozen)
	}
	if !loaded.IsReady() {
		t.Fatalf("IsReady=false: status=%s frozen=%v", loaded.Status, loaded.Frozen)
	}
	// 验收 5: 重跑产生同一 hash
	s3, err := b.Build(date, marketsnapshot.DefaultUniverseUsable())
	if err != nil {
		t.Fatal(err)
	}
	if s3.ContentHash != s2.ContentHash {
		t.Fatalf("重跑 hash 变了: %s vs %s", s3.ContentHash, s2.ContentHash)
	}
	// 特征快照 + 加载
	fs, err := b.BuildFeatureSnapshot(s2, nil, fe)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.Values) != 2 {
		t.Fatalf("feature values codes = %d want 2", len(fs.Values))
	}
	for _, perCode := range fs.Values {
		for _, name := range []string{"close", "ma20", "rsi14"} {
			if _, ok := perCode[name]; !ok {
				t.Fatalf("缺少指标 %s", name)
			}
		}
	}
	if err := repo.SaveFeatureSnapshot(fs); err != nil {
		t.Fatal(err)
	}
	fs2, err := repo.LoadFeatureSnapshot(fs.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if fs2.ContentHash != fs.ContentHash || len(fs2.Values) != len(fs.Values) {
		t.Fatalf("feature snapshot 加载哈希不匹配 %s vs %s", fs2.ContentHash, fs.ContentHash)
	}
}
