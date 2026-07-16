package watchlist

import (
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// WatchlistStock 自选股
type WatchlistStock struct {
	Code      string    `json:"code"`
	Name      string    `json:"name,omitempty"`
	Group     string    `json:"group,omitempty"`
	Note      string    `json:"note,omitempty"`
	AddedAt   time.Time `json:"added_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Store 自选股存储
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
		return `CREATE TABLE IF NOT EXISTS watchlist (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			"group" TEXT NOT NULL DEFAULT 'default',
			note TEXT NOT NULL DEFAULT '',
			added_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL DEFAULT 0
		)`
	case storage.MySQL:
		return "CREATE TABLE IF NOT EXISTS watchlist (" +
			"code VARCHAR(20) PRIMARY KEY," +
			"name VARCHAR(100) NOT NULL DEFAULT ''," +
			"`group` VARCHAR(50) NOT NULL DEFAULT 'default'," +
			"note TEXT NOT NULL," +
			"added_at BIGINT NOT NULL," +
			"updated_at BIGINT NOT NULL DEFAULT 0" +
			")"
	default: // SQLite
		return `CREATE TABLE IF NOT EXISTS watchlist (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			"group" TEXT NOT NULL DEFAULT 'default',
			note TEXT NOT NULL DEFAULT '',
			added_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`
	}
}

// ensureSchema 检查并迁移旧数据库 schema
func (s *Store) ensureSchema() error {
	columns, err := s.getColumns()
	if err != nil {
		return err
	}

	columnMap := make(map[string]bool)
	for _, col := range columns {
		columnMap[col] = true
	}

	// 添加缺失的列
	if !columnMap["group"] {
		if _, err := s.s.DB().Exec(`ALTER TABLE watchlist ADD COLUMN "group" TEXT NOT NULL DEFAULT 'default'`); err != nil {
			return err
		}
	}
	if !columnMap["note"] {
		if _, err := s.s.DB().Exec(`ALTER TABLE watchlist ADD COLUMN note TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !columnMap["updated_at"] {
		if _, err := s.s.DB().Exec(`ALTER TABLE watchlist ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) getColumns() ([]string, error) {
	var query string
	switch s.s.Dialect() {
	case storage.Postgres:
		query = `SELECT column_name FROM information_schema.columns WHERE table_name = 'watchlist'`
	case storage.MySQL:
		query = `SHOW COLUMNS FROM watchlist`
	default: // SQLite
		query = `PRAGMA table_info(watchlist)`
	}

	rows, err := s.s.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		switch s.s.Dialect() {
		case storage.Postgres:
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			columns = append(columns, name)
		case storage.MySQL:
			var field, ctype, null, key string
			var defaultVal interface{}
			var extra string
			if err := rows.Scan(&field, &ctype, &null, &key, &defaultVal, &extra); err != nil {
				return nil, err
			}
			columns = append(columns, field)
		default: // SQLite
			var cid int
			var name, ctype string
			var notNull, pk int
			var defaultVal interface{}
			if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultVal, &pk); err != nil {
				return nil, err
			}
			columns = append(columns, name)
		}
	}
	return columns, rows.Err()
}

// GetAll 获取所有自选股
func (s *Store) GetAll() ([]WatchlistStock, error) {
	groupCol := s.quotedColumn("group")
	rows, err := s.s.DB().Query(fmt.Sprintf(`SELECT code, name, %s, note, added_at, updated_at FROM watchlist ORDER BY %s, added_at DESC`, groupCol, groupCol))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stocks []WatchlistStock
	for rows.Next() {
		var stock WatchlistStock
		var addedAt, updatedAt int64
		if err := rows.Scan(&stock.Code, &stock.Name, &stock.Group, &stock.Note, &addedAt, &updatedAt); err != nil {
			return nil, err
		}
		stock.AddedAt = time.Unix(addedAt, 0)
		stock.UpdatedAt = time.Unix(updatedAt, 0)
		stocks = append(stocks, stock)
	}
	return stocks, rows.Err()
}

// ph 返回占位符 ? 或 $N
func (s *Store) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// GetByGroup 按分组获取
func (s *Store) GetByGroup(group string) ([]WatchlistStock, error) {
	groupCol := s.quotedColumn("group")
	query := fmt.Sprintf(`SELECT code, name, %s, note, added_at, updated_at FROM watchlist WHERE %s = %s ORDER BY added_at DESC`, groupCol, groupCol, s.ph(1))
	rows, err := s.s.DB().Query(query, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stocks []WatchlistStock
	for rows.Next() {
		var stock WatchlistStock
		var addedAt, updatedAt int64
		if err := rows.Scan(&stock.Code, &stock.Name, &stock.Group, &stock.Note, &addedAt, &updatedAt); err != nil {
			return nil, err
		}
		stock.AddedAt = time.Unix(addedAt, 0)
		stock.UpdatedAt = time.Unix(updatedAt, 0)
		stocks = append(stocks, stock)
	}
	return stocks, rows.Err()
}

// GetGroups 获取所有分组
func (s *Store) GetGroups() ([]string, error) {
	groupCol := s.quotedColumn("group")
	rows, err := s.s.DB().Query(fmt.Sprintf(`SELECT DISTINCT %s FROM watchlist ORDER BY %s`, groupCol, groupCol))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// Upsert 插入或更新
func (s *Store) Upsert(stock WatchlistStock) error {
	if stock.Code == "" {
		return fmt.Errorf("code is required")
	}
	if stock.Group == "" {
		stock.Group = "default"
	}
	now := time.Now().Unix()
	if stock.AddedAt.IsZero() {
		stock.AddedAt = time.Now()
	}

	groupCol := s.quotedColumn("group")

	switch s.s.Dialect() {
	case storage.Postgres:
		_, err := s.s.DB().Exec(fmt.Sprintf(`
			INSERT INTO watchlist (code, name, %s, note, added_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT(code) DO UPDATE SET
				name = CASE WHEN $2 <> '' THEN $2 ELSE watchlist.name END,
				%s = CASE WHEN $3 <> '' THEN $3 ELSE watchlist.%s END,
				note = CASE WHEN $4 <> '' THEN $4 ELSE watchlist.note END,
				updated_at = $6
		`, groupCol, groupCol, groupCol), stock.Code, stock.Name, stock.Group, stock.Note, stock.AddedAt.Unix(), now)
		return err
	case storage.MySQL:
		_, err := s.s.DB().Exec(fmt.Sprintf(`
			INSERT INTO watchlist (code, name, %s, note, added_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				name = CASE WHEN VALUES(name) <> '' THEN VALUES(name) ELSE name END,
				%s = CASE WHEN VALUES(%s) <> '' THEN VALUES(%s) ELSE %s END,
				note = CASE WHEN VALUES(note) <> '' THEN VALUES(note) ELSE note END,
				updated_at = VALUES(updated_at)
		`, groupCol, groupCol, groupCol, groupCol, groupCol), stock.Code, stock.Name, stock.Group, stock.Note, stock.AddedAt.Unix(), now)
		return err
	default: // SQLite
		_, err := s.s.DB().Exec(fmt.Sprintf(`
			INSERT INTO watchlist (code, name, %s, note, added_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE watchlist.name END,
				%s = CASE WHEN excluded.%s <> '' THEN excluded.%s ELSE watchlist.%s END,
				note = CASE WHEN excluded.note <> '' THEN excluded.note ELSE watchlist.note END,
				updated_at = ?
		`, groupCol, groupCol, groupCol, groupCol, groupCol), stock.Code, stock.Name, stock.Group, stock.Note, stock.AddedAt.Unix(), now, now)
		return err
	}
}

// UpdateNote 更新备注
func (s *Store) UpdateNote(code, note string) error {
	query := fmt.Sprintf(`UPDATE watchlist SET note = %s, updated_at = %s WHERE code = %s`, s.ph(1), s.ph(2), s.ph(3))
	_, err := s.s.DB().Exec(query, note, time.Now().Unix(), code)
	return err
}

// UpdateGroup 更新分组
func (s *Store) UpdateGroup(code, group string) error {
	if group == "" {
		group = "default"
	}
	groupCol := s.quotedColumn("group")
	query := fmt.Sprintf(`UPDATE watchlist SET %s = %s, updated_at = %s WHERE code = %s`, groupCol, s.ph(1), s.ph(2), s.ph(3))
	_, err := s.s.DB().Exec(query, group, time.Now().Unix(), code)
	return err
}

// Delete 删除
func (s *Store) Delete(code string) error {
	query := fmt.Sprintf(`DELETE FROM watchlist WHERE code = %s`, s.ph(1))
	_, err := s.s.DB().Exec(query, code)
	return err
}

// Exists 检查是否存在
func (s *Store) Exists(code string) (bool, error) {
	var count int
	query := fmt.Sprintf(`SELECT COUNT(*) FROM watchlist WHERE code = %s`, s.ph(1))
	err := s.s.DB().QueryRow(query, code).Scan(&count)
	return count > 0, err
}

// Count 数量
func (s *Store) Count() (int, error) {
	var count int
	err := s.s.DB().QueryRow(`SELECT COUNT(*) FROM watchlist`).Scan(&count)
	return count, err
}

// CountByGroup 按分组统计
func (s *Store) CountByGroup() (map[string]int, error) {
	groupCol := s.quotedColumn("group")
	rows, err := s.s.DB().Query(fmt.Sprintf(`SELECT %s, COUNT(*) FROM watchlist GROUP BY %s`, groupCol, groupCol))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var group string
		var count int
		if err := rows.Scan(&group, &count); err != nil {
			return nil, err
		}
		counts[group] = count
	}
	return counts, nil
}

func (s *Store) quotedColumn(name string) string {
	switch s.s.Dialect() {
	case storage.MySQL:
		return "`" + name + "`"
	case storage.Postgres:
		return `"` + name + `"`
	default: // SQLite
		return `"` + name + `"`
	}
}
