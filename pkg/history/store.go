package history

import (
	"database/sql"
	"time"
)

type DB struct {
	db *sql.DB
}

func Open(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath+"?cache=shared")
	if err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

type HistoryStock struct {
	Code       string    `json:"code"`
	AnalyzedAt time.Time `json:"analyzed_at"`
}

func InitTable(d *DB) error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS history_stocks (
			code TEXT PRIMARY KEY,
			analyzed_at INTEGER NOT NULL
		)
	`)
	return err
}

func GetAll(d *DB) ([]HistoryStock, error) {
	rows, err := d.db.Query(`
		SELECT code, analyzed_at
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
		if err := rows.Scan(&s.Code, &analyzedAt); err != nil {
			return nil, err
		}
		s.AnalyzedAt = time.Unix(analyzedAt, 0)
		stocks = append(stocks, s)
	}
	return stocks, nil
}

func Upsert(d *DB, stock HistoryStock) error {
	_, err := d.db.Exec(`
		INSERT INTO history_stocks (code, analyzed_at)
		VALUES (?, ?)
		ON CONFLICT(code) DO UPDATE SET
			analyzed_at = excluded.analyzed_at
	`, stock.Code, stock.AnalyzedAt.Unix())
	return err
}

func Delete(d *DB, code string) error {
	_, err := d.db.Exec(`DELETE FROM history_stocks WHERE code = ?`, code)
	return err
}
