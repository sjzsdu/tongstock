package stockpool

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func parseDate(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return t
}

// setupStore 创建带初始数据的测试存储
func setupStore(t *testing.T) *SecuritiesMasterStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// 准备基准数据: 4 只股票
	// - A: 长期正常上市
	// - B: 2023-01-01 上市, 2024-06-30 退市
	// - C: 2023-05-01 上市, 2024-03-01 ~ 2024-03-10 停牌
	// - D: 2022-01-01 上市, ST 股
	seed := `
INSERT INTO stockinfo (code, name, exchange, ipo_date_txt, delist_date, st_flag) VALUES
('600001', '平安银行A', 'sh', '2010-01-01', '', 0),
('600002', '退市股票B', 'sh', '2023-01-01', '2024-06-30', 0),
('600003', '停牌股票C', 'sh', '2023-05-01', '', 0),
('600004', 'ST股票D', 'sz', '2022-01-01', '', 1);
INSERT INTO security_status_history (code, effective_from, effective_to, status, reason, source, created_at) VALUES
('600003', '2024-03-01', '2024-03-10', 'suspended', '重大资产重组', 'manual', 1),
('600003', '2024-03-11', '', 'normal', '复牌', 'manual', 1),
('600004', '2023-06-01', '', 'st', '连续亏损', 'manual', 1);
`
	if _, err := s.DB().Exec(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return NewSecuritiesMasterStore(s)
}

// TestReconstructPool_Normal 基础重建: 所有在池中的股票
func TestReconstructPool_Normal(t *testing.T) {
	m := setupStore(t)

	// 2024-01-15: A 存在, B 存在, C 存在 (未停牌), D 存在 (ST 排除)
	pool, err := m.ReconstructPool(parseDate("2024-01-15"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	codes := map[string]bool{}
	for _, s := range pool {
		codes[s.Code] = true
	}

	if !codes["600001"] {
		t.Error("600001 应在池中 (长期正常上市)")
	}
	if !codes["600002"] {
		t.Error("600002 应在池中 (2024-06-30 前未退市)")
	}
	if !codes["600003"] {
		t.Error("600003 应在池中 (1月15日未停牌)")
	}
	if codes["600004"] {
		t.Error("600004 不应在池中 (ST 股被排除)")
	}
	if len(pool) != 3 {
		t.Errorf("应 3 只, 实际 %d", len(pool))
	}
}

// TestReconstructPool_IPOBoundary 边界: 上市日之前不应出现
func TestReconstructPool_IPOBoundary(t *testing.T) {
	m := setupStore(t)

	// 2023-01-01 之前: B 不应出现 (2023-01-01 才上市)
	pool, err := m.ReconstructPool(parseDate("2022-12-31"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	for _, s := range pool {
		if s.Code == "600002" {
			t.Error("600002 不应在 2022-12-31 的池中 (尚未上市)")
		}
	}

	// 2023-01-01 当天: B 应出现 (ipo_date <= date)
	pool, err = m.ReconstructPool(parseDate("2023-01-01"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	found := false
	for _, s := range pool {
		if s.Code == "600002" {
			found = true
			break
		}
	}
	if !found {
		t.Error("600002 应在 2023-01-01 当天的池中 (ipo_date 包含当日)")
	}
}

// TestReconstructPool_Delisted 退市股在退市后不应出现
func TestReconstructPool_Delisted(t *testing.T) {
	m := setupStore(t)

	// 2024-07-01: B 已退市
	pool, err := m.ReconstructPool(parseDate("2024-07-01"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	for _, s := range pool {
		if s.Code == "600002" {
			t.Error("600002 不应在 2024-07-01 的池中 (已退市)")
		}
	}

	// 退市当天仍在: delist_date 为最后交易日 (含当日)
	pool, err = m.ReconstructPool(parseDate("2024-06-30"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	found := false
	for _, s := range pool {
		if s.Code == "600002" {
			found = true
			break
		}
	}
	if !found {
		t.Error("600002 应在 2024-06-30 当天的池中 (最后交易日)")
	}
}

// TestReconstructPool_Suspension 停牌股在停牌期间排除
func TestReconstructPool_Suspension(t *testing.T) {
	m := setupStore(t)

	// 2024-03-05: C 正在停牌 (3月1日~3月10日)
	pool, err := m.ReconstructPool(parseDate("2024-03-05"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	for _, s := range pool {
		if s.Code == "600003" {
			t.Error("600003 不应在 2024-03-05 的池中 (停牌)")
		}
	}

	// 2024-03-11: C 已复牌, 应在池中
	pool, err = m.ReconstructPool(parseDate("2024-03-11"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	found := false
	for _, s := range pool {
		if s.Code == "600003" {
			found = true
			break
		}
	}
	if !found {
		t.Error("600003 应在 2024-03-11 的池中 (已复牌)")
	}

	// includeSuspended=true 时应包含停牌股
	pool, err = m.ReconstructPool(parseDate("2024-03-05"), true, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	found = false
	for _, s := range pool {
		if s.Code == "600003" {
			found = true
			break
		}
	}
	if !found {
		t.Error("600003 在 includeSuspended=true 时应出现在池中")
	}
}

// TestReconstructPool_STFlag ST 股控制
func TestReconstructPool_STFlag(t *testing.T) {
	m := setupStore(t)

	// includeST=true 时 ST 股应包含且标记 st
	pool, err := m.ReconstructPool(parseDate("2024-01-15"), false, true)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	var stSec *AsOfSecurity
	for i := range pool {
		if pool[i].Code == "600004" {
			stSec = &pool[i]
			break
		}
	}
	if stSec == nil {
		t.Fatal("600004 在 includeST=true 时应在池中")
	}
	if stSec.Status != StatusST {
		t.Errorf("ST 股状态应为 %q, 实际 %q", StatusST, stSec.Status)
	}

	// includeST=false 时排除
	pool, err = m.ReconstructPool(parseDate("2024-01-15"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	for _, s := range pool {
		if s.Code == "600004" {
			t.Error("600004 不应在池中 (includeST=false)")
		}
	}
}

// TestGetSecurityAtDate 查询单只证券状态
func TestGetSecurityAtDate(t *testing.T) {
	m := setupStore(t)

	// 查询退市后的 B
	s, err := m.GetSecurityAtDate("600002", parseDate("2024-07-01"))
	if err != nil {
		t.Fatalf("GetSecurityAtDate: %v", err)
	}
	if s != nil {
		t.Error("已退市证券在退市后不应出现在池")
	}

	// 查询存在的 A
	s, err = m.GetSecurityAtDate("600001", parseDate("2024-01-15"))
	if err != nil {
		t.Fatalf("GetSecurityAtDate: %v", err)
	}
	if s == nil {
		t.Fatal("600001 应在池中")
	}
	if s.Status != StatusNormal {
		t.Errorf("Status 应为 normal, 实际 %s", s.Status)
	}
}

// TestInsertAndQuery 动态插入状态
func TestInsertAndQuery(t *testing.T) {
	m := setupStore(t)

	// 为 A 增加一段临时停牌
	if err := m.InsertStatus("600001", "2024-02-01", "2024-02-05", StatusSuspended, "临时停牌", "test"); err != nil {
		t.Fatalf("InsertStatus: %v", err)
	}

	// 停牌期间 A 不应在池中
	pool, err := m.ReconstructPool(parseDate("2024-02-03"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	for _, s := range pool {
		if s.Code == "600001" {
			t.Error("600001 在 2024-02-03 停牌期间不应在池中")
		}
	}

	// 停牌结束后 A 应在池中
	pool, err = m.ReconstructPool(parseDate("2024-02-06"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	found := false
	for _, s := range pool {
		if s.Code == "600001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("600001 复牌后应在池中")
	}
}

// TestEmptyDate 空日期边界
func TestEmptyDate(t *testing.T) {
	m := setupStore(t)

	// 空字符串 delist_date 表示未退市
	if err := m.UpdateIpoDelist("600005", "2024-01-01", "", 0); err != nil {
		t.Fatalf("UpdateIpoDelist: %v", err)
	}

	pool, err := m.ReconstructPool(parseDate("2024-12-31"), false, false)
	if err != nil {
		t.Fatalf("ReconstructPool: %v", err)
	}
	found := false
	for _, s := range pool {
		if s.Code == "600005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("600005 空 delist_date 应视为未退市")
	}
}
