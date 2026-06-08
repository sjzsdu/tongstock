package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type DB struct {
	db *sql.DB
}

func Open(dbPath string) (*DB, error) {
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dbPath = filepath.Join(homeDir, ".tongstock", "history.db")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath+"?cache=shared&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	// 设置连接池，避免并发问题
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

type HistoryStock struct {
	Code       string    `json:"code"`
	Name       string    `json:"name,omitempty"`
	AnalyzedAt time.Time `json:"analyzed_at"`
}

func InitTable(d *DB) error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS history_stocks (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			analyzed_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	return ensureHistorySchema(d)
}

func ensureHistorySchema(d *DB) error {
	rows, err := d.db.Query(`PRAGMA table_info(history_stocks)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasName := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == "name" {
			hasName = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasName {
		return nil
	}

	_, err = d.db.Exec(`ALTER TABLE history_stocks ADD COLUMN name TEXT NOT NULL DEFAULT ''`)
	return err
}

func GetAll(d *DB) ([]HistoryStock, error) {
	rows, err := d.db.Query(`
		SELECT code, name, analyzed_at
		FROM history_stocks
		ORDER BY analyzed_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stocks []HistoryStock
	for rows.Next() {
		var s HistoryStock
		var analyzedAt int64
		if err := rows.Scan(&s.Code, &s.Name, &analyzedAt); err != nil {
			return nil, err
		}
		s.AnalyzedAt = time.Unix(analyzedAt, 0)
		stocks = append(stocks, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stocks, nil
}

func Upsert(d *DB, stock HistoryStock) error {
	if err := ensureHistorySchema(d); err != nil {
		return err
	}
	if stock.Code == "" {
		return fmt.Errorf("code is required")
	}
	res, err := d.db.Exec(`
		INSERT INTO history_stocks (code, name, analyzed_at)
		VALUES (?, ?, ?)
		ON CONFLICT(code) DO UPDATE SET
			name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE history_stocks.name END,
			analyzed_at = excluded.analyzed_at
	`, stock.Code, stock.Name, stock.AnalyzedAt.Unix())
	if err != nil {
		return err
	}
	_, err = res.RowsAffected()
	return err
}

func Delete(d *DB, code string) error {
	_, err := d.db.Exec(`DELETE FROM history_stocks WHERE code = ?`, code)
	return err
}
