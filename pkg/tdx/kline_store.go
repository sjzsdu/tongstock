package tdx

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// KlineStore K线存储
type KlineStore struct {
	s   *storage.Storage
	mu  sync.RWMutex
	loc *time.Location
}

// KlineSyncState 同步状态
type KlineSyncState struct {
	Code       string    `json:"code"`
	KType      uint8     `json:"ktype"`
	FirstDate  string    `json:"first_date,omitempty"`
	LastDate   string    `json:"last_date,omitempty"`
	RowCount   int       `json:"row_count"`
	LastSyncAt time.Time `json:"last_sync_at"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
}

// 错误定义
var ErrKlineNotFound = errors.New("kline: not found")

// NewKlineStore 创建K线存储
func NewKlineStore(s *storage.Storage) (*KlineStore, error) {
	store := &KlineStore{s: s, loc: time.Local}
	if err := store.init(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *KlineStore) init() error {
	db := s.s.DB()
	
	// 创建 kline 表
	if _, err := db.Exec(s.createTableSQL()); err != nil {
		return err
	}

	// 创建索引（PostgreSQL 不支持多语句 Exec，需单独执行）
	if s.s.Dialect() == storage.Postgres {
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_code_ktype ON kline(code, ktype)`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_date ON kline(date)`)
	}

	// 创建 kline_sync_state 表
	if _, err := db.Exec(s.createSyncStateSQL()); err != nil {
		return err
	}

	return nil
}

func (s *KlineStore) createTableSQL() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `CREATE TABLE IF NOT EXISTS kline (
			code TEXT, ktype SMALLINT, date TEXT,
			open DOUBLE PRECISION, high DOUBLE PRECISION, low DOUBLE PRECISION, close DOUBLE PRECISION, 
			volume DOUBLE PRECISION, amount DOUBLE PRECISION,
			PRIMARY KEY (code, ktype, date)
		)`
	case storage.MySQL:
		return "CREATE TABLE IF NOT EXISTS kline (" +
			"code VARCHAR(20), ktype TINYINT UNSIGNED, date VARCHAR(8)," +
			"open DOUBLE, high DOUBLE, low DOUBLE, close DOUBLE, volume DOUBLE, amount DOUBLE," +
			"PRIMARY KEY (code, ktype, date), INDEX idx_code_ktype (code, ktype), INDEX idx_date (date)" +
			")"
	default: // SQLite
		return `CREATE TABLE IF NOT EXISTS kline (
			code TEXT, ktype INTEGER, date TEXT,
			open REAL, high REAL, low REAL, close REAL, volume REAL, amount REAL,
			PRIMARY KEY (code, ktype, date)
		); CREATE INDEX IF NOT EXISTS idx_code_ktype ON kline(code, ktype); CREATE INDEX IF NOT EXISTS idx_date ON kline(date);`
	}
}

func (s *KlineStore) createSyncStateSQL() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `CREATE TABLE IF NOT EXISTS kline_sync_state (
			code TEXT, ktype SMALLINT,
			first_date TEXT NOT NULL DEFAULT '', last_date TEXT NOT NULL DEFAULT '',
			row_count INTEGER NOT NULL DEFAULT 0, last_sync_at BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (code, ktype)
		)`
	case storage.MySQL:
		return "CREATE TABLE IF NOT EXISTS kline_sync_state (" +
			"code VARCHAR(20), ktype TINYINT UNSIGNED," +
			"first_date VARCHAR(8) NOT NULL DEFAULT '', last_date VARCHAR(8) NOT NULL DEFAULT ''," +
			"row_count INT NOT NULL DEFAULT 0, last_sync_at BIGINT NOT NULL DEFAULT 0," +
			"status VARCHAR(20) NOT NULL DEFAULT '', error TEXT NOT NULL," +
			"PRIMARY KEY (code, ktype))"
	default: // SQLite
		return `CREATE TABLE IF NOT EXISTS kline_sync_state (
			code TEXT, ktype INTEGER,
			first_date TEXT NOT NULL DEFAULT '', last_date TEXT NOT NULL DEFAULT '',
			row_count INTEGER NOT NULL DEFAULT 0, last_sync_at INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (code, ktype)
		)`
	}
}

// SaveKline 保存K线数据
func (s *KlineStore) SaveKline(code string, ktype uint8, klines []*protocol.Kline) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(s.saveSQL())
	if err != nil {
		return err
	}
	defer stmt.Close()

	// 日期范围检查
	now := time.Now()
	maxDate := now.AddDate(0, 0, 1)
	minDate := time.Date(1990, 1, 1, 0, 0, 0, 0, s.loc)

	skipped := 0
	for _, k := range klines {
		// 检查日期是否异常
		if k.Time.Before(minDate) || k.Time.After(maxDate) {
			skipped++
			continue
		}
		if reason := validateKline(k); reason != "" {
			skipped++
			continue
		}
		if _, err := stmt.Exec(code, ktype, k.Time.Format("20060102"), k.Open, k.High, k.Low, k.Close, k.Volume, k.Amount); err != nil {
			return err
		}
	}
	if skipped > 0 {
		log.Printf("[kline] skipped %d invalid records for %s", skipped, code)
	}
	return tx.Commit()
}

func (s *KlineStore) saveSQL() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `INSERT INTO kline (code, ktype, date, open, high, low, close, volume, amount) 
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) 
				ON CONFLICT (code, ktype, date) DO UPDATE SET 
				open=EXCLUDED.open, high=EXCLUDED.high, low=EXCLUDED.low, close=EXCLUDED.close, volume=EXCLUDED.volume, amount=EXCLUDED.amount`
	case storage.MySQL:
		return "INSERT INTO kline (code, ktype, date, open, high, low, close, volume, amount) " +
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) " +
			"ON DUPLICATE KEY UPDATE open=VALUES(open), high=VALUES(high), low=VALUES(low), close=VALUES(close), volume=VALUES(volume), amount=VALUES(amount)"
	default: // SQLite
		return `INSERT OR REPLACE INTO kline (code, ktype, date, open, high, low, close, volume, amount) 
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}
}

// ph 返回占位符 ? 或 $N
func (s *KlineStore) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// GetKline 获取K线数据
func (s *KlineStore) GetKline(code string, ktype uint8, startDate, endDate string) ([]*protocol.Kline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := fmt.Sprintf(`SELECT date, open, high, low, close, volume, amount FROM kline WHERE code = %s AND ktype = %s`, s.ph(1), s.ph(2))
	args := []interface{}{code, ktype}
	idx := 3
	if startDate != "" {
		query += fmt.Sprintf(" AND date >= %s", s.ph(idx))
		args = append(args, startDate)
		idx++
	}
	if endDate != "" {
		query += fmt.Sprintf(" AND date <= %s", s.ph(idx))
		args = append(args, endDate)
	}

	rows, err := s.s.DB().Query(query+" ORDER BY date", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 日期范围检查
	now := time.Now()
	maxDate := now.AddDate(0, 0, 1) // 最多到明天
	minDate := time.Date(1990, 1, 1, 0, 0, 0, 0, s.loc) // 最早1990年

	var klines []*protocol.Kline
	var lastClose float64
	for rows.Next() {
		var k protocol.Kline
		var date string
		if err := rows.Scan(&date, &k.Open, &k.High, &k.Low, &k.Close, &k.Volume, &k.Amount); err != nil {
			return nil, err
		}
		// 尝试两种日期格式解析
		var t time.Time
		var err error
		t, err = time.Parse("2006-01-02", date)
		if err != nil {
			t, err = time.Parse("20060102", date)
		}
		if err != nil {
			continue
		}
		// 过滤日期异常的数据
		if t.Before(minDate) || t.After(maxDate) {
			continue
		}
		k.Time = t
		if validateKline(&k) != "" {
			continue
		}
		// 检查与前一条有效 K 线的价格变化
		if lastClose > 0 {
			ratio := k.Close / lastClose
			if ratio > 3.0 || ratio < 0.33 {
				continue // 跳过异常跳变
			}
		}
		lastClose = k.Close
		klines = append(klines, &k)
	}
	return klines, rows.Err()
}

// GetLatestDate 获取最新日期
func (s *KlineStore) GetLatestDate(code string, ktype uint8) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var date string
	err := s.s.DB().QueryRow(fmt.Sprintf(`SELECT date FROM kline WHERE code = %s AND ktype = %s ORDER BY date DESC LIMIT 1`, s.ph(1), s.ph(2)), code, ktype).Scan(&date)
	if err == sql.ErrNoRows {
		return "", ErrKlineNotFound
	}
	return date, err
}

// UpdateSyncState 更新同步状态
func (s *KlineStore) UpdateSyncState(code string, ktype uint8, status, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var first, last string
	var count int
	s.s.DB().QueryRow(fmt.Sprintf(`SELECT COALESCE(MIN(date), ''), COALESCE(MAX(date), ''), COUNT(*) FROM kline WHERE code = %s AND ktype = %s`, s.ph(1), s.ph(2)), code, ktype).Scan(&first, &last, &count)

	switch s.s.Dialect() {
	case storage.Postgres:
		_, err := s.s.DB().Exec(`
			INSERT INTO kline_sync_state (code, ktype, first_date, last_date, row_count, last_sync_at, status, error)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (code, ktype) DO UPDATE SET
			first_date=EXCLUDED.first_date, last_date=EXCLUDED.last_date,
			row_count=EXCLUDED.row_count, last_sync_at=EXCLUDED.last_sync_at,
			status=EXCLUDED.status, error=EXCLUDED.error
		`, code, ktype, first, last, count, time.Now().Unix(), status, errMsg)
		return err
	case storage.MySQL:
		_, err := s.s.DB().Exec(`
			INSERT INTO kline_sync_state (code, ktype, first_date, last_date, row_count, last_sync_at, status, error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE first_date=VALUES(first_date), last_date=VALUES(last_date),
			row_count=VALUES(row_count), last_sync_at=VALUES(last_sync_at),
			status=VALUES(status), error=VALUES(error)
		`, code, ktype, first, last, count, time.Now().Unix(), status, errMsg)
		return err
	default: // SQLite
		_, err := s.s.DB().Exec(`
			INSERT INTO kline_sync_state (code, ktype, first_date, last_date, row_count, last_sync_at, status, error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(code, ktype) DO UPDATE SET
			first_date=excluded.first_date, last_date=excluded.last_date,
			row_count=excluded.row_count, last_sync_at=excluded.last_sync_at,
			status=excluded.status, error=excluded.error
		`, code, ktype, first, last, count, time.Now().Unix(), status, errMsg)
		return err
	}
}

// GetSyncState 获取同步状态
func (s *KlineStore) GetSyncState(code string, ktype uint8) (*KlineSyncState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var state KlineSyncState
	var ts int64
	err := s.s.DB().QueryRow(fmt.Sprintf(`SELECT code, ktype, first_date, last_date, row_count, last_sync_at, status, error FROM kline_sync_state WHERE code = %s AND ktype = %s`, s.ph(1), s.ph(2)), code, ktype).Scan(&state.Code, &state.KType, &state.FirstDate, &state.LastDate, &state.RowCount, &ts, &state.Status, &state.Error)
	if err == sql.ErrNoRows {
		return nil, ErrKlineNotFound
	}
	if ts > 0 {
		state.LastSyncAt = time.Unix(ts, 0)
	}
	return &state, err
}

// DetectAndCleanCorruptedKlines 检测并清理异常数据
func (s *KlineStore) DetectAndCleanCorruptedKlines(code string, ktype uint8) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 日期范围检查
	now := time.Now()
	maxDate := now.AddDate(0, 0, 1)
	minDate := time.Date(1990, 1, 1, 0, 0, 0, 0, s.loc)

	rows, err := s.s.DB().Query(fmt.Sprintf(`SELECT date, open, high, low, close FROM kline WHERE code = %s AND ktype = %s ORDER BY date`, s.ph(1), s.ph(2)), code, ktype)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var corrupted []string
	var lastClose float64
	var idx int

	for rows.Next() {
		var date string
		var open, high, low, close float64
		if err := rows.Scan(&date, &open, &high, &low, &close); err != nil {
			continue
		}

		// 解析日期
		var t time.Time
		t, err = time.Parse("2006-01-02", date)
		if err != nil {
			t, err = time.Parse("20060102", date)
		}

		// 检查日期是否异常
		isCorrupted := false
		if err != nil {
			isCorrupted = true
		} else if t.Before(minDate) || t.After(maxDate) {
			isCorrupted = true
		} else {
			// 检查价格是否异常
			k := &protocol.Kline{Open: open, High: high, Low: low, Close: close}
			if reason := validateKline(k); reason != "" {
				isCorrupted = true
			} else if idx > 0 && lastClose > 0 {
				r := close / lastClose
				if r > 5 || r < 0.2 {
					isCorrupted = true
				} else {
					lastClose = close
					idx++
				}
			} else {
				lastClose = close
				idx++
			}
		}

		if isCorrupted {
			corrupted = append(corrupted, date)
		}
	}

	if len(corrupted) == 0 {
		return 0, nil
	}

	tx, err := s.s.DB().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	deleteSQL := fmt.Sprintf(`DELETE FROM kline WHERE code = %s AND ktype = %s AND date = %s`, s.ph(1), s.ph(2), s.ph(3))
	for _, d := range corrupted {
		tx.Exec(deleteSQL, code, ktype, d)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	log.Printf("[kline] cleaned %d corrupted records for %s", len(corrupted), code)
	return len(corrupted), nil
}

// Close 关闭
func (s *KlineStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil // 不关闭，由 storage 统一管理
}

// validateKline 验证K线数据，返回空字符串表示有效
func validateKline(k *protocol.Kline) string {
	if k == nil {
		return "nil_record"
	}

	// 检查基本价格有效性
	for _, item := range []struct {
		name  string
		value float64
	}{
		{"open", k.Open},
		{"high", k.High},
		{"low", k.Low},
		{"close", k.Close},
	} {
		if item.value <= 0 || math.IsNaN(item.value) || math.IsInf(item.value, 0) {
			return "bad_" + item.name
		}
	}

	// 检查 OHLC 关系
	if k.High < k.Low {
		return "bad_ohlc_order"
	}
	if k.High < k.Open || k.High < k.Close {
		return "bad_high"
	}
	if k.Low > k.Open || k.Low > k.Close {
		return "bad_low"
	}

	// 检查价格合理性（A股不超过100万）
	if k.Close > 1000000 || k.Open > 1000000 {
		return "price_too_high"
	}

	return ""
}
