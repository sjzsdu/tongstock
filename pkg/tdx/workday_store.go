package tdx

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// WorkdayStore 交易日存储
type WorkdayStore struct {
	s   *storage.Storage
	mu  sync.RWMutex
	loc *time.Location
}

// 错误定义
var ErrWorkdayNotFound = errors.New("workday: not found")

// NewWorkdayStore 创建交易日存储
func NewWorkdayStore(s *storage.Storage) (*WorkdayStore, error) {
	store := &WorkdayStore{s: s, loc: time.Local}
	if err := store.init(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *WorkdayStore) init() error {
	var sql string
	switch s.s.Dialect() {
	case storage.Postgres:
		sql = `CREATE TABLE IF NOT EXISTS workday (unix BIGINT PRIMARY KEY, date TEXT)`
	case storage.MySQL:
		sql = "CREATE TABLE IF NOT EXISTS workday (unix BIGINT PRIMARY KEY, date VARCHAR(8))"
	default: // SQLite
		sql = `CREATE TABLE IF NOT EXISTS workday (unix INTEGER PRIMARY KEY, date TEXT)`
	}
	_, err := s.s.DB().Exec(sql)
	return err
}

// Is 检查指定日期是否为交易日
func (s *WorkdayStore) Is(date time.Time) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, s.loc)
	var count int
	query := fmt.Sprintf(`SELECT COUNT(*) FROM workday WHERE unix = %s`, s.ph(1))
	err := s.s.DB().QueryRow(query, day.Unix()).Scan(&count)
	return count > 0, err
}

// TodayIs 检查今天是否为交易日
func (s *WorkdayStore) TodayIs() (bool, error) {
	return s.Is(time.Now())
}

// ph 返回占位符 ? 或 $N
func (s *WorkdayStore) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// Range 获取日期范围内的交易日
func (s *WorkdayStore) Range(start, end time.Time) ([]time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, s.loc)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, s.loc)

	query := fmt.Sprintf(`SELECT unix FROM workday WHERE unix >= %s AND unix <= %s ORDER BY unix ASC`, s.ph(1), s.ph(2))
	rows, err := s.s.DB().Query(query, startDay.Unix(), endDay.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var unix int64
		if err := rows.Scan(&unix); err != nil {
			return nil, err
		}
		dates = append(dates, time.Unix(unix, 0))
	}
	return dates, rows.Err()
}

// RangeDesc 获取日期范围内的交易日（倒序）
func (s *WorkdayStore) RangeDesc(start, end time.Time) ([]time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, s.loc)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, s.loc)

	query := fmt.Sprintf(`SELECT unix FROM workday WHERE unix >= %s AND unix <= %s ORDER BY unix DESC`, s.ph(1), s.ph(2))
	rows, err := s.s.DB().Query(query, startDay.Unix(), endDay.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var unix int64
		if err := rows.Scan(&unix); err != nil {
			return nil, err
		}
		dates = append(dates, time.Unix(unix, 0))
	}
	return dates, rows.Err()
}

// RangeYear 获取指定年份的交易日
func (s *WorkdayStore) RangeYear(year int) ([]time.Time, error) {
	start := time.Date(year, 1, 1, 0, 0, 0, 0, s.loc)
	end := time.Date(year, 12, 31, 0, 0, 0, 0, s.loc)
	return s.Range(start, end)
}

// GetLastWorkday 获取最近的交易日
func (s *WorkdayStore) GetLastWorkday() (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)

	var unix int64
	query := fmt.Sprintf(`SELECT unix FROM workday WHERE unix <= %s ORDER BY unix DESC LIMIT 1`, s.ph(1))
	err := s.s.DB().QueryRow(query, today.Unix()).Scan(&unix)
	if err == sql.ErrNoRows {
		return time.Time{}, ErrWorkdayNotFound
	}
	return time.Unix(unix, 0), err
}

// GetNextWorkday 获取下一个交易日
func (s *WorkdayStore) GetNextWorkday(date time.Time) (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, s.loc)

	var unix int64
	query := fmt.Sprintf(`SELECT unix FROM workday WHERE unix > %s ORDER BY unix ASC LIMIT 1`, s.ph(1))
	err := s.s.DB().QueryRow(query, day.Unix()).Scan(&unix)
	if err == sql.ErrNoRows {
		return time.Time{}, ErrWorkdayNotFound
	}
	return time.Unix(unix, 0), err
}

// UpdateFromKline 从K线数据更新交易日
func (s *WorkdayStore) UpdateFromKline(client *Client, indexCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	resp, err := client.GetKlineDayAll(indexCode)
	if err != nil {
		return err
	}

	tx, err := s.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(s.insertIgnoreSQL())
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, k := range resp {
		day := time.Date(k.Time.Year(), k.Time.Month(), k.Time.Day(), 0, 0, 0, 0, s.loc)
		stmt.Exec(day.Unix(), day.Format("20060102"))
	}

	return tx.Commit()
}

func (s *WorkdayStore) insertIgnoreSQL() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `INSERT INTO workday (unix, date) VALUES ($1, $2) ON CONFLICT (unix) DO NOTHING`
	case storage.MySQL:
		return `INSERT IGNORE INTO workday (unix, date) VALUES (?, ?)`
	default: // SQLite
		return `INSERT OR IGNORE INTO workday (unix, date) VALUES (?, ?)`
	}
}

// Close 关闭
func (s *WorkdayStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil // 不关闭，由 storage 统一管理
}
