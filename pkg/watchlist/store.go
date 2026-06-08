package watchlist

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WatchlistStock represents a stock in the user's watchlist
type WatchlistStock struct {
	Code      string    `json:"code"`
	Name      string    `json:"name,omitempty"`
	Group     string    `json:"group,omitempty"` // 分组: default, industry, concept, custom
	Note      string    `json:"note,omitempty"`  // 用户备注
	AddedAt   time.Time `json:"added_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// DB wraps the database connection for watchlist operations
type DB struct {
	db *sql.DB
}

var (
	watchlistDB *DB
	watchlistMu sync.RWMutex
)

// Open creates or opens the watchlist database
func Open(dbPath string) (*DB, error) {
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dbPath = filepath.Join(homeDir, ".tongstock", "watchlist.db")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath+"?cache=shared&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	wdb := &DB{db: db}
	if err := wdb.initTable(); err != nil {
		return nil, err
	}

	return wdb, nil
}

// GetWatchlistDB returns the singleton watchlist database
func GetWatchlistDB(dbPath string) (*DB, error) {
	watchlistMu.RLock()
	if watchlistDB != nil {
		watchlistMu.RUnlock()
		return watchlistDB, nil
	}
	watchlistMu.RUnlock()

	watchlistMu.Lock()
	defer watchlistMu.Unlock()

	if watchlistDB != nil {
		return watchlistDB, nil
	}

	db, err := Open(dbPath)
	if err != nil {
		return nil, err
	}
	watchlistDB = db
	return watchlistDB, nil
}

func (d *DB) initTable() error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS watchlist (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			"group" TEXT NOT NULL DEFAULT 'default',
			note TEXT NOT NULL DEFAULT '',
			added_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return err
	}
	return d.ensureSchema()
}

func (d *DB) ensureSchema() error {
	rows, err := d.db.Query(`PRAGMA table_info(watchlist)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasGroup := false
	hasNote := false
	hasUpdatedAt := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == "group" {
			hasGroup = true
		}
		if name == "note" {
			hasNote = true
		}
		if name == "updated_at" {
			hasUpdatedAt = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if !hasGroup {
		_, err = d.db.Exec(`ALTER TABLE watchlist ADD COLUMN "group" TEXT NOT NULL DEFAULT 'default'`)
		if err != nil {
			return err
		}
	}
	if !hasNote {
		_, err = d.db.Exec(`ALTER TABLE watchlist ADD COLUMN note TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return err
		}
	}
	if !hasUpdatedAt {
		_, err = d.db.Exec(`ALTER TABLE watchlist ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0`)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

// GetAll returns all stocks in the watchlist
func (d *DB) GetAll() ([]WatchlistStock, error) {
	rows, err := d.db.Query(`
		SELECT code, name, "group", note, added_at, updated_at
		FROM watchlist
		ORDER BY "group", added_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stocks []WatchlistStock
	for rows.Next() {
		var s WatchlistStock
		var addedAt, updatedAt int64
		if err := rows.Scan(&s.Code, &s.Name, &s.Group, &s.Note, &addedAt, &updatedAt); err != nil {
			return nil, err
		}
		s.AddedAt = time.Unix(addedAt, 0)
		s.UpdatedAt = time.Unix(updatedAt, 0)
		stocks = append(stocks, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stocks, nil
}

// GetByGroup returns stocks in a specific group
func (d *DB) GetByGroup(group string) ([]WatchlistStock, error) {
	rows, err := d.db.Query(`
		SELECT code, name, "group", note, added_at, updated_at
		FROM watchlist
		WHERE "group" = ?
		ORDER BY added_at DESC
	`, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stocks []WatchlistStock
	for rows.Next() {
		var s WatchlistStock
		var addedAt, updatedAt int64
		if err := rows.Scan(&s.Code, &s.Name, &s.Group, &s.Note, &addedAt, &updatedAt); err != nil {
			return nil, err
		}
		s.AddedAt = time.Unix(addedAt, 0)
		s.UpdatedAt = time.Unix(updatedAt, 0)
		stocks = append(stocks, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stocks, nil
}

// GetGroups returns all unique groups
func (d *DB) GetGroups() ([]string, error) {
	rows, err := d.db.Query(`
		SELECT DISTINCT "group" FROM watchlist ORDER BY "group"
	`)
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

// Upsert adds or updates a stock in the watchlist
func (d *DB) Upsert(stock WatchlistStock) error {
	if err := d.ensureSchema(); err != nil {
		return err
	}
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

	res, err := d.db.Exec(`
		INSERT INTO watchlist (code, name, "group", note, added_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(code) DO UPDATE SET
			name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE watchlist.name END,
			"group" = CASE WHEN excluded."group" <> '' THEN excluded."group" ELSE watchlist."group" END,
			note = CASE WHEN excluded.note <> '' THEN excluded.note ELSE watchlist.note END,
			updated_at = ?
	`, stock.Code, stock.Name, stock.Group, stock.Note, stock.AddedAt.Unix(), now, now)
	if err != nil {
		return err
	}
	_, err = res.RowsAffected()
	return err
}

// UpdateNote updates the note for a specific stock
func (d *DB) UpdateNote(code, note string) error {
	_, err := d.db.Exec(`
		UPDATE watchlist SET note = ?, updated_at = ? WHERE code = ?
	`, note, time.Now().Unix(), code)
	return err
}

// UpdateGroup updates the group for a specific stock
func (d *DB) UpdateGroup(code, group string) error {
	if group == "" {
		group = "default"
	}
	_, err := d.db.Exec(`
		UPDATE watchlist SET "group" = ?, updated_at = ? WHERE code = ?
	`, group, time.Now().Unix(), code)
	return err
}

// Delete removes a stock from the watchlist
func (d *DB) Delete(code string) error {
	_, err := d.db.Exec(`DELETE FROM watchlist WHERE code = ?`, code)
	return err
}

// Exists checks if a stock is in the watchlist
func (d *DB) Exists(code string) (bool, error) {
	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM watchlist WHERE code = ?`, code).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Count returns the total number of stocks in the watchlist
func (d *DB) Count() (int, error) {
	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM watchlist`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountByGroup returns the count of stocks in each group
func (d *DB) CountByGroup() (map[string]int, error) {
	rows, err := d.db.Query(`
		SELECT "group", COUNT(*) FROM watchlist GROUP BY "group"
	`)
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
