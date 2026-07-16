package history

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// HistoryStock 历史记录
type HistoryStock struct {
	Code       string    `json:"code"`
	Name       string    `json:"name,omitempty"`
	AnalyzedAt time.Time `json:"analyzed_at"`
}

// Store 历史记录存储
type Store struct {
	s *storage.Storage
}

// New 创建存储实例
func New(s *storage.Storage) (*Store, error) {
	store := &Store{s: s}
	if err := store.init(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) init() error {
	_, err := s.s.DB().Exec(s.createTableSQL())
	if err != nil {
		return err
	}
	return s.ensureSchema()
}

func (s *Store) createTableSQL() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `CREATE TABLE IF NOT EXISTS history_stocks (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			analyzed_at BIGINT NOT NULL
		)`
	case storage.MySQL:
		return "CREATE TABLE IF NOT EXISTS history_stocks (" +
			"code VARCHAR(20) PRIMARY KEY," +
			"name VARCHAR(100) NOT NULL DEFAULT ''," +
			"analyzed_at BIGINT NOT NULL" +
			")"
	default: // SQLite
		return `CREATE TABLE IF NOT EXISTS history_stocks (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			analyzed_at INTEGER NOT NULL
		)`
	}
}

// ensureSchema 检查并迁移旧数据库 schema
func (s *Store) ensureSchema() error {
	// 检查 name 列是否存在
	rows, err := s.s.DB().Query(s.schemaQuery())
	if err != nil {
		return err
	}
	defer rows.Close()

	hasName := false
	for rows.Next() {
		columnName, err := s.scanColumnName(rows)
		if err != nil {
			return err
		}
		if columnName == "name" {
			hasName = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if !hasName {
		_, err = s.s.DB().Exec(`ALTER TABLE history_stocks ADD COLUMN name TEXT NOT NULL DEFAULT ''`)
		return err
	}
	return nil
}

func (s *Store) scanColumnName(rows *sql.Rows) (string, error) {
	var columnName string
	switch s.s.Dialect() {
	case storage.Postgres:
		var ordinalPos int
		var dataType, isNullable string
		var columnDefault interface{}
		var constraintType interface{}
		if err := rows.Scan(&ordinalPos, &columnName, &dataType, &isNullable, &columnDefault, &constraintType); err != nil {
			return "", err
		}
	case storage.MySQL:
		var field, ctype, null, key, extra string
		var defaultVal interface{}
		if err := rows.Scan(&field, &ctype, &null, &key, &defaultVal, &extra); err != nil {
			return "", err
		}
		columnName = field
	default: // SQLite
		var cid int
		var name, ctype string
		var notNull, pk int
		var defaultVal interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultVal, &pk); err != nil {
			return "", err
		}
		columnName = name
	}
	return columnName, nil
}

func (s *Store) schemaQuery() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `SELECT ordinal_position, column_name, data_type, is_nullable, column_default, constraint_type
				FROM information_schema.columns WHERE table_name = 'history_stocks'`
	case storage.MySQL:
		return `SHOW COLUMNS FROM history_stocks`
	default: // SQLite
		return `PRAGMA table_info(history_stocks)`
	}
}

// GetAll 获取所有历史记录
func (s *Store) GetAll() ([]HistoryStock, error) {
	rows, err := s.s.DB().Query(`SELECT code, name, analyzed_at FROM history_stocks ORDER BY analyzed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stocks []HistoryStock
	for rows.Next() {
		var stock HistoryStock
		var ts int64
		if err := rows.Scan(&stock.Code, &stock.Name, &ts); err != nil {
			return nil, err
		}
		stock.AnalyzedAt = time.Unix(ts, 0)
		stocks = append(stocks, stock)
	}
	return stocks, rows.Err()
}

// Upsert 插入或更新记录
func (s *Store) Upsert(stock HistoryStock) error {
	if stock.Code == "" {
		return fmt.Errorf("code is required")
	}

	switch s.s.Dialect() {
	case storage.Postgres:
		_, err := s.s.DB().Exec(`
			INSERT INTO history_stocks (code, name, analyzed_at) VALUES ($1, $2, $3)
			ON CONFLICT(code) DO UPDATE SET
				name = CASE WHEN $2 <> '' THEN $2 ELSE history_stocks.name END,
				analyzed_at = $3
		`, stock.Code, stock.Name, stock.AnalyzedAt.Unix())
		return err
	case storage.MySQL:
		_, err := s.s.DB().Exec(`
			INSERT INTO history_stocks (code, name, analyzed_at) VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE
				name = CASE WHEN VALUES(name) <> '' THEN VALUES(name) ELSE name END,
				analyzed_at = VALUES(analyzed_at)
		`, stock.Code, stock.Name, stock.AnalyzedAt.Unix())
		return err
	default: // SQLite
		_, err := s.s.DB().Exec(`
			INSERT INTO history_stocks (code, name, analyzed_at) VALUES (?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE history_stocks.name END,
				analyzed_at = excluded.analyzed_at
		`, stock.Code, stock.Name, stock.AnalyzedAt.Unix())
		return err
	}
}

// ph 返回占位符 ? 或 $N
func (s *Store) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// Delete 删除指定记录
func (s *Store) Delete(code string) error {
	query := fmt.Sprintf(`DELETE FROM history_stocks WHERE code = %s`, s.ph(1))
	_, err := s.s.DB().Exec(query, code)
	return err
}
