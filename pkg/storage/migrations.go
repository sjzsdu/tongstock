package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type migration struct {
	version int
	name    string
	sql     string
	after   func(*sql.Tx) error
}

var sqliteMigrations = []migration{
	{
		version: 1,
		name:    "unified_base_schema",
		sql: `
CREATE TABLE IF NOT EXISTS history_stocks (
	code TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', analyzed_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS watchlist (
	code TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', "group" TEXT NOT NULL DEFAULT 'default',
	note TEXT NOT NULL DEFAULT '', added_at INTEGER NOT NULL, updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS trades (
	id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL, price REAL NOT NULL, signal TEXT NOT NULL DEFAULT '',
	ktype TEXT NOT NULL DEFAULT 'day', reason TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS stockpool (
	id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
	filters TEXT NOT NULL DEFAULT '[]', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS stockinfo (
	code TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', exchange TEXT NOT NULL DEFAULT '',
	price REAL NOT NULL DEFAULT 0, open REAL NOT NULL DEFAULT 0, high REAL NOT NULL DEFAULT 0,
	low REAL NOT NULL DEFAULT 0, last_close REAL NOT NULL DEFAULT 0, change_pct REAL NOT NULL DEFAULT 0,
	volume REAL NOT NULL DEFAULT 0, amount REAL NOT NULL DEFAULT 0, turnover_rate REAL NOT NULL DEFAULT 0,
	liu_tong_gu_ben REAL NOT NULL DEFAULT 0, zong_gu_ben REAL NOT NULL DEFAULT 0,
	market_cap REAL NOT NULL DEFAULT 0, total_market_cap REAL NOT NULL DEFAULT 0,
	jing_zi_chan REAL NOT NULL DEFAULT 0, jing_li_run REAL NOT NULL DEFAULT 0,
	mei_gu_jing_zi_chan REAL NOT NULL DEFAULT 0, province INTEGER NOT NULL DEFAULT 0,
	industry INTEGER NOT NULL DEFAULT 0, ipo_date INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS kline (
	code TEXT, ktype INTEGER, date TEXT, open REAL, high REAL, low REAL, close REAL,
	volume REAL, amount REAL, PRIMARY KEY (code, ktype, date)
);
CREATE INDEX IF NOT EXISTS idx_code_ktype ON kline(code, ktype);
CREATE INDEX IF NOT EXISTS idx_date ON kline(date);
CREATE TABLE IF NOT EXISTS kline_sync_state (
	code TEXT, ktype INTEGER, first_date TEXT NOT NULL DEFAULT '', last_date TEXT NOT NULL DEFAULT '',
	row_count INTEGER NOT NULL DEFAULT 0, last_sync_at INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '', PRIMARY KEY (code, ktype)
);
CREATE TABLE IF NOT EXISTS workday (unix INTEGER PRIMARY KEY, date TEXT);
CREATE TABLE IF NOT EXISTS cache (
	bucket TEXT NOT NULL, key TEXT NOT NULL, value BLOB, created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (bucket, key)
);
CREATE INDEX IF NOT EXISTS idx_cache_bucket ON cache(bucket);
CREATE TABLE IF NOT EXISTS chat_sessions (
	id TEXT PRIMARY KEY, stock_code TEXT NOT NULL DEFAULT '', agent TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL, data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chat_sessions_stock_code ON chat_sessions(stock_code);
CREATE TABLE IF NOT EXISTS paradigms (
	id TEXT PRIMARY KEY, stock_code TEXT NOT NULL DEFAULT '', side TEXT NOT NULL DEFAULT '',
	cache_key TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL, data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_paradigms_stock_code ON paradigms(stock_code);
CREATE INDEX IF NOT EXISTS idx_paradigms_cache_key ON paradigms(cache_key);
CREATE TABLE IF NOT EXISTS news_items (
	id TEXT PRIMARY KEY, source TEXT NOT NULL, news_type TEXT NOT NULL, title TEXT NOT NULL,
	summary TEXT, content TEXT, publish_time DATETIME NOT NULL, hot_score INTEGER DEFAULT 0,
	tags TEXT, related_stocks TEXT, url TEXT, original_id TEXT UNIQUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_news_source ON news_items(source);
CREATE INDEX IF NOT EXISTS idx_news_publish_time ON news_items(publish_time);
CREATE INDEX IF NOT EXISTS idx_news_hot_score ON news_items(hot_score);
CREATE INDEX IF NOT EXISTS idx_news_related_stocks ON news_items(related_stocks);
CREATE TABLE IF NOT EXISTS hot_events (
	id TEXT PRIMARY KEY, title TEXT NOT NULL, keywords TEXT, related_stocks TEXT,
	hot_index INTEGER DEFAULT 0, source_counts TEXT, news_item_ids TEXT,
	status TEXT DEFAULT 'active', created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_event_hot_index ON hot_events(hot_index);
CREATE INDEX IF NOT EXISTS idx_event_status ON hot_events(status);
CREATE INDEX IF NOT EXISTS idx_event_updated_at ON hot_events(updated_at);
CREATE TABLE IF NOT EXISTS alert_records (
	id TEXT PRIMARY KEY, rule_id TEXT, news_id TEXT, stock_code TEXT, level TEXT, type TEXT,
	title TEXT, summary TEXT, source TEXT, read BOOLEAN DEFAULT FALSE, trigger_time TEXT,
	created_at TEXT, FOREIGN KEY (news_id) REFERENCES news_items(id)
);
CREATE INDEX IF NOT EXISTS idx_alert_records_read ON alert_records(read);
CREATE INDEX IF NOT EXISTS idx_alert_records_trigger_time ON alert_records(trigger_time);
CREATE INDEX IF NOT EXISTS idx_alert_records_stock_code ON alert_records(stock_code);
`,
		after: func(tx *sql.Tx) error {
			if err := ensureSQLiteColumn(tx, "history_stocks", "name", `TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
			for _, column := range []struct {
				name string
				ddl  string
			}{
				{"group", `TEXT NOT NULL DEFAULT 'default'`},
				{"note", `TEXT NOT NULL DEFAULT ''`},
				{"updated_at", `INTEGER NOT NULL DEFAULT 0`},
			} {
				if err := ensureSQLiteColumn(tx, "watchlist", column.name, column.ddl); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version: 2,
		name:    "stock_data_read_model",
		sql: `
CREATE TABLE IF NOT EXISTS quote_snapshot (
	code TEXT PRIMARY KEY, payload TEXT NOT NULL, source_updated_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS finance_snapshot (
	code TEXT PRIMARY KEY, payload TEXT NOT NULL, source_updated_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS data_sync_state (
	sync_key TEXT PRIMARY KEY, data_type TEXT NOT NULL, market TEXT NOT NULL DEFAULT '',
	code TEXT NOT NULL DEFAULT '', granularity TEXT NOT NULL DEFAULT '',
	range_start TEXT NOT NULL DEFAULT '', range_end TEXT NOT NULL DEFAULT '',
	coverage_start TEXT NOT NULL DEFAULT '', coverage_end TEXT NOT NULL DEFAULT '',
	source_updated_at INTEGER NOT NULL DEFAULT 0, last_sync_at INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT '', quality TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '', rows_written INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_data_sync_lookup
	ON data_sync_state(data_type, market, code, granularity);
`,
	},
}

// Migrate upgrades the SQLite database transactionally. Store constructors
// may assume the current schema and must not own or close the connection.
func (s *Storage) Migrate() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("storage is not initialized")
	}
	if s.dialect != SQLite {
		return fmt.Errorf("migrations only support sqlite")
	}
	if _, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at INTEGER NOT NULL
)`); err != nil {
		return err
	}

	for _, m := range sqliteMigrations {
		var exists int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		tx, err := s.db.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(m.sql); err == nil && m.after != nil {
			err = m.after(tx)
		}
		if err == nil {
			_, err = tx.Exec(
				`INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`,
				m.version, m.name, time.Now().Unix(),
			)
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

func ensureSQLiteColumn(tx *sql.Tx, table, column, ddl string) error {
	rows, err := tx.Query(`PRAGMA table_info(` + quoteIdentifier(table) + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = tx.Exec(`ALTER TABLE ` + quoteIdentifier(table) + ` ADD COLUMN ` + quoteIdentifier(column) + ` ` + ddl)
	return err
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// SchemaVersion returns the latest applied migration version.
func (s *Storage) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	return version, err
}

// Ping verifies the shared database connection.
func (s *Storage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
