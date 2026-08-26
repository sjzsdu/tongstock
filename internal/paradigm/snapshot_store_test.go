package paradigm

import (
	"crypto/md5"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func setupSnapshotStore(t *testing.T) (*DatasetSnapshotStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot_test.db")
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewDatasetSnapshotStore(s), path
}

func setupKlineSnapshotStore(t *testing.T) (*DatasetSnapshotStore, *storage.Storage) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kline_snapshot_test.db")
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewDatasetSnapshotStore(s), s
}

func makeDataSource(srcType, version string, asOf time.Time) DataSource {
	h := md5.Sum([]byte(version + srcType))
	return DataSource{
		Type:            srcType,
		Version:         version,
		AsOf:            asOf,
		SourceUpdatedAt: asOf,
		Hash:            hex.EncodeToString(h[:]),
	}
}

func TestDatasetSnapshot_CreateAndGet(t *testing.T) {
	store, _ := setupSnapshotStore(t)
	now := time.Now().Truncate(time.Second)

	snap := &DatasetSnapshot{
		ID:              "snap-001",
		Version:         "v1",
		DateRange:       DateRange{Start: "2023-01-01", End: "2023-12-31"},
		Universe:        []string{"600000", "600001", "000001"},
		Market:          "SH",
		PriceAdjustment: PriceForward,
		Description:     "2023年全量SH股票前复权快照",
		CreatedAt:       now,
		Sources: []DataSource{
			makeDataSource("kline", "20240101.1", now),
			makeDataSource("quote", "20240101.1", now),
		},
	}

	if err := store.Create(snap); err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := store.GetByID("snap-001")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if loaded.ID != snap.ID {
		t.Errorf("ID mismatch: %s vs %s", loaded.ID, snap.ID)
	}
	if !loaded.Frozen {
		t.Error("Snapshot should be frozen after creation")
	}
	if loaded.PriceAdjustment != PriceForward {
		t.Errorf("PriceAdjustment = %s, want %s", loaded.PriceAdjustment, PriceForward)
	}
	if len(loaded.Sources) != 2 {
		t.Fatalf("Sources count = %d, want 2", len(loaded.Sources))
	}
	if !loaded.HasSource("kline") {
		t.Error("Should have kline source")
	}
	if !loaded.HasSource("quote") {
		t.Error("Should have quote source")
	}
	if loaded.HasSource("finance") {
		t.Error("Should not have finance source")
	}

	versions := loaded.SourceVersions()
	if versions["kline"] != "20240101.1" {
		t.Errorf("kline version = %s, want 20240101.1", versions["kline"])
	}
}

func TestDatasetSnapshot_Immutability(t *testing.T) {
	store, _ := setupSnapshotStore(t)
	now := time.Now().Truncate(time.Second)

	snap := &DatasetSnapshot{
		ID:        "snap-immutable",
		Version:   "v1",
		DateRange: DateRange{Start: "2023-01-01", End: "2023-12-31"},
		Universe:  []string{"600000"},
		Market:    "ALL",
		CreatedAt: now,
	}

	if err := store.Create(snap); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 尝试通过 Create 重新写入 (使用 INSERT OR REPLACE 但 frozen 字段已标记)
	// 这里的关键测试是: 读取后 frozen = true
	loaded, err := store.GetByID("snap-immutable")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if !loaded.Frozen {
		t.Error("Frozen should be true")
	}

	// Version 应保持一致
	if loaded.Version != "v1" {
		t.Errorf("Version should be v1, got %s", loaded.Version)
	}

	// DateRange 应保持一致
	if loaded.DateRange.End != "2023-12-31" {
		t.Errorf("DateRange.End should be 2023-12-31, got %s", loaded.DateRange.End)
	}

	replacement := *snap
	replacement.Version = "v2"
	replacement.Description = "must not replace"
	if err := store.Create(&replacement); err == nil {
		t.Fatal("duplicate snapshot ID must be rejected")
	}
	loaded, err = store.GetByID("snap-immutable")
	if err != nil {
		t.Fatalf("GetByID after duplicate: %v", err)
	}
	if loaded.Version != "v1" || loaded.Description == "must not replace" {
		t.Fatal("immutable snapshot was changed by duplicate Create")
	}
}

func TestDatasetSnapshot_CreateKlineSnapshotFreezesRealContent(t *testing.T) {
	store, raw := setupKlineSnapshotStore(t)
	rows := []struct {
		code, date                     string
		open, high, low, close, volume float64
	}{
		{"000001", "20240102", 10, 11, 9, 10.5, 1000},
		{"000001", "20240103", 10.5, 12, 10, 11.5, 1200},
		{"600000", "20240102", 8, 8.5, 7.8, 8.2, 900},
		{"600000", "20240103", 8.2, 8.8, 8.1, 8.6, 1100},
	}
	for _, row := range rows {
		if _, err := raw.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`,
			row.code, row.date, row.open, row.high, row.low, row.close,
			row.volume, row.close*row.volume); err != nil {
			t.Fatalf("seed K line: %v", err)
		}
	}

	snapshot := &DatasetSnapshot{
		ID:        "snap-real-kline",
		Version:   "v1",
		DateRange: DateRange{Start: "2024-01-02", End: "2024-01-03"},
		Universe:  []string{"600000", "000001"},
		Market:    "ALL",
	}
	if err := store.CreateKlineSnapshot(snapshot, 9); err != nil {
		t.Fatalf("CreateKlineSnapshot: %v", err)
	}
	if snapshot.ContentHash == "" || len(snapshot.KlineManifests) != 2 {
		t.Fatalf("snapshot has no frozen content manifest: %+v", snapshot)
	}
	if err := store.VerifyContent(snapshot.ID); err != nil {
		t.Fatalf("VerifyContent: %v", err)
	}

	before, err := store.GetFrozenKlines(snapshot.ID, "000001", 9)
	if err != nil {
		t.Fatalf("GetFrozenKlines: %v", err)
	}
	if len(before) != 2 || before[1].Close != 11.5 {
		t.Fatalf("unexpected frozen bars: %+v", before)
	}

	// 修改可变的实时 K 线表，已冻结快照必须保持原值。
	if _, err := raw.DB().Exec(`UPDATE kline SET high = 100, close = 99, amount = volume * 99
		WHERE code = '000001' AND ktype = 9 AND date = '20240103'`); err != nil {
		t.Fatalf("mutate live K line: %v", err)
	}
	after, err := store.GetFrozenKlines(snapshot.ID, "000001", 9)
	if err != nil {
		t.Fatalf("GetFrozenKlines after live mutation: %v", err)
	}
	if after[1].Close != before[1].Close {
		t.Fatalf("frozen snapshot changed with live table: before %.2f after %.2f", before[1].Close, after[1].Close)
	}

	duplicate := *snapshot
	duplicate.Version = "v2"
	if err := store.CreateKlineSnapshot(&duplicate, 9); err == nil {
		t.Fatal("duplicate frozen snapshot ID must be rejected")
	}
}

func TestDatasetSnapshot_CreateKlineSnapshotRejectsMissingAndDetectsTampering(t *testing.T) {
	store, raw := setupKlineSnapshotStore(t)
	if _, err := raw.DB().Exec(`INSERT INTO kline
		(code, ktype, date, open, high, low, close, volume, amount)
		VALUES ('000001', 9, '20240102', 10, 11, 9, 10.5, 1000, 10500)`); err != nil {
		t.Fatalf("seed K line: %v", err)
	}

	missing := &DatasetSnapshot{
		ID:        "snap-missing-code",
		Version:   "v1",
		DateRange: DateRange{Start: "2024-01-02", End: "2024-01-03"},
		Universe:  []string{"000001", "600000"},
	}
	if err := store.CreateKlineSnapshot(missing, 9); err == nil {
		t.Fatal("snapshot with missing real data must fail")
	}
	if _, err := store.GetByID(missing.ID); err == nil {
		t.Fatal("failed snapshot creation must roll back metadata")
	}

	valid := &DatasetSnapshot{
		ID:        "snap-tamper",
		Version:   "v1",
		DateRange: DateRange{Start: "2024-01-02", End: "2024-01-03"},
		Universe:  []string{"000001"},
	}
	if err := store.CreateKlineSnapshot(valid, 9); err != nil {
		t.Fatalf("CreateKlineSnapshot: %v", err)
	}
	if _, err := raw.DB().Exec(`UPDATE snapshot_kline_bar SET close = 88
		WHERE snapshot_id = ? AND code = '000001' AND ktype = 9`, valid.ID); err != nil {
		t.Fatalf("tamper frozen K line: %v", err)
	}
	if _, err := store.GetFrozenKlines(valid.ID, "000001", 9); err == nil {
		t.Fatal("tampered frozen content must fail hash verification")
	}
}

func TestDatasetSnapshot_ValidateAsOf(t *testing.T) {
	_, _ = setupSnapshotStore(t)
	asOf := time.Date(2023, 6, 1, 0, 0, 0, 0, time.Local)

	// 正常: 所有数据源 as-of <= referenceDate
	snap := &DatasetSnapshot{
		ID:        "snap-asof-ok",
		Version:   "v1",
		DateRange: DateRange{Start: "2023-01-01", End: "2023-06-01"},
		Universe:  []string{"600000"},
		Market:    "SH",
		Sources: []DataSource{
			makeDataSource("kline", "v1", asOf),
			makeDataSource("quote", "v1", asOf),
		},
	}

	if err := snap.ValidateAsOf(asOf); err != nil {
		t.Errorf("ValidateAsOf should pass: %v", err)
	}

	// 异常: 某个数据源 as-of > referenceDate
	futureAsOf := asOf.AddDate(0, 0, 1) // 一天后
	badSnap := &DatasetSnapshot{
		ID:        "snap-asof-bad",
		Version:   "v1",
		DateRange: DateRange{Start: "2023-01-01", End: "2023-06-01"},
		Universe:  []string{"600000"},
		Market:    "SH",
		Sources: []DataSource{
			makeDataSource("kline", "v1", asOf),
			makeDataSource("quote", "v2", futureAsOf), // 违规!
		},
	}

	if err := badSnap.ValidateAsOf(asOf); err == nil {
		t.Error("ValidateAsOf should fail with future data leak")
	}
}

func TestExperimentSnapshotBinding(t *testing.T) {
	store, _ := setupSnapshotStore(t)
	now := time.Now().Truncate(time.Second)

	// 创建两个快照
	snap1 := &DatasetSnapshot{
		ID:        "snap-binding-1",
		Version:   "v1",
		DateRange: DateRange{Start: "2023-01-01", End: "2023-06-30"},
		Universe:  []string{"600000"},
		Market:    "SH",
		CreatedAt: now,
	}
	snap2 := &DatasetSnapshot{
		ID:        "snap-binding-2",
		Version:   "v2",
		DateRange: DateRange{Start: "2023-07-01", End: "2023-12-31"},
		Universe:  []string{"600000"},
		Market:    "SH",
		CreatedAt: now,
	}

	if err := store.Create(snap1); err != nil {
		t.Fatalf("Create snap1: %v", err)
	}
	if err := store.Create(snap2); err != nil {
		t.Fatalf("Create snap2: %v", err)
	}

	// 为实验绑定两个快照
	if err := store.BindExperiment("exp-001", []string{"snap-binding-1", "snap-binding-2"}); err != nil {
		t.Fatalf("BindExperiment: %v", err)
	}

	// 获取绑定结果
	bound, err := store.GetBoundSnapshots("exp-001")
	if err != nil {
		t.Fatalf("GetBoundSnapshots: %v", err)
	}
	if len(bound) != 2 {
		t.Fatalf("Bound snapshots count = %d, want 2", len(bound))
	}

	// 验证已绑定的快照仍可使用
	if err := store.VerifyBinding("exp-001", "snap-binding-1"); err != nil {
		t.Errorf("VerifyBinding for bound snapshot should pass: %v", err)
	}

	// 验证未绑定的快照不能使用 (防止静默替换)
	if err := store.VerifyBinding("exp-001", "snap-new-version"); err == nil {
		t.Error("VerifyBinding should fail for unbound snapshot")
	}

	// 新实验首次绑定: 允许
	if err := store.VerifyBinding("exp-new", "snap-binding-1"); err != nil {
		t.Errorf("VerifyBinding for new experiment should pass: %v", err)
	}
}

func TestDatasetSnapshot_ListAndCount(t *testing.T) {
	store, _ := setupSnapshotStore(t)
	now := time.Now().Truncate(time.Second)

	for i := 0; i < 5; i++ {
		snap := &DatasetSnapshot{
			ID:        "snap-list-" + string(rune('a'+i)),
			Version:   "v1",
			DateRange: DateRange{Start: "2023-01-01", End: "2023-12-31"},
			Universe:  []string{"600000"},
			Market:    "ALL",
			CreatedAt: now.Add(time.Duration(i) * time.Hour),
		}
		if err := store.Create(snap); err != nil {
			t.Fatalf("Create %s: %v", snap.ID, err)
		}
	}

	count, err := store.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 5 {
		t.Errorf("Count = %d, want 5", count)
	}

	list, err := store.List(3, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List len = %d, want 3", len(list))
	}

	// 倒序: 最新创建的应在前
	if list[0].CreatedAt.Before(list[1].CreatedAt) {
		t.Error("List should be ordered by created_at DESC")
	}
}

func TestDatasetSnapshot_MissingID(t *testing.T) {
	store, _ := setupSnapshotStore(t)

	_, err := store.GetByID("snap-nonexistent")
	if err == nil {
		t.Error("GetByID should return error for nonexistent ID")
	}
}

func TestDatasetSnapshot_EmptySources(t *testing.T) {
	store, _ := setupSnapshotStore(t)
	now := time.Now().Truncate(time.Second)

	snap := &DatasetSnapshot{
		ID:        "snap-empty-sources",
		Version:   "v1",
		DateRange: DateRange{Start: "2023-01-01", End: "2023-12-31"},
		Universe:  []string{"600000"},
		Market:    "SH",
		CreatedAt: now,
		// Sources 为空
	}

	if err := store.Create(snap); err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := store.GetByID("snap-empty-sources")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if len(loaded.Sources) != 0 {
		t.Errorf("Sources count = %d, want 0", len(loaded.Sources))
	}

	// 没有 source 时 ValidateAsOf 应通过
	if err := loaded.ValidateAsOf(now); err != nil {
		t.Errorf("ValidateAsOf with no sources should pass: %v", err)
	}
}

func TestDatasetSnapshot_RebindSafety(t *testing.T) {
	store, _ := setupSnapshotStore(t)
	now := time.Now().Truncate(time.Second)

	// 创建快照
	snap := &DatasetSnapshot{
		ID:        "snap-rebind-test",
		Version:   "v1",
		DateRange: DateRange{Start: "2023-01-01", End: "2023-12-31"},
		Universe:  []string{"600000"},
		Market:    "SH",
		CreatedAt: now,
	}
	if err := store.Create(snap); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 创建实验绑定
	if err := store.BindExperiment("exp-rebind", []string{"snap-rebind-test"}); err != nil {
		t.Fatalf("BindExperiment: %v", err)
	}

	// 尝试绑定不同版本的快照 (模拟数据更新后静默替换)
	snap2 := &DatasetSnapshot{
		ID:        "snap-rebind-v2",
		Version:   "v2", // 更新版本!
		DateRange: DateRange{Start: "2023-01-01", End: "2023-12-31"},
		Universe:  []string{"600000"},
		Market:    "SH",
		CreatedAt: now,
	}
	if err := store.Create(snap2); err != nil {
		t.Fatalf("Create snap2: %v", err)
	}

	// VerifyBinding 应拒绝未绑定的快照
	if err := store.VerifyBinding("exp-rebind", "snap-rebind-v2"); err == nil {
		t.Error("Should reject unbound new version snapshot for bound experiment")
	}

	// 旧版本仍可使用
	if err := store.VerifyBinding("exp-rebind", "snap-rebind-test"); err != nil {
		t.Errorf("Old version should still be valid: %v", err)
	}
}
