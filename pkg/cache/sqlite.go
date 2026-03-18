package cache

import (
	"database/sql"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type sqliteCache struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteCache 创建基于 SQLite 的缓存实例
func NewSQLiteCache(dbPath string) (Cache, error) {
	db, err := sql.Open("sqlite3", dbPath+"?cache=shared")
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cache (
			bucket TEXT NOT NULL,
			key TEXT NOT NULL,
			value BLOB,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (bucket, key)
		);
		CREATE INDEX IF NOT EXISTS idx_cache_bucket ON cache(bucket);
	`); err != nil {
		db.Close()
		return nil, err
	}

	return &sqliteCache{db: db}, nil
}

func (c *sqliteCache) Get(bucket, key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var value []byte
	var expiresAt int64
	err := c.db.QueryRow(
		`SELECT value, expires_at FROM cache WHERE bucket = ? AND key = ?`,
		bucket, key,
	).Scan(&value, &expiresAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if expiresAt > 0 && time.Now().Unix() > expiresAt {
		return nil, ErrExpired
	}

	return value, nil
}

func (c *sqliteCache) Set(bucket, key string, value []byte, opts ...Option) error {
	o := applyOptions(opts)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()
	var expiresAt int64
	if o.TTL > 0 {
		expiresAt = now + int64(o.TTL.Seconds())
	}

	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO cache (bucket, key, value, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		bucket, key, value, now, expiresAt,
	)
	return err
}

func (c *sqliteCache) Delete(bucket, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.db.Exec(`DELETE FROM cache WHERE bucket = ? AND key = ?`, bucket, key)
	return err
}

func (c *sqliteCache) Has(bucket, key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now().Unix()
	var exists int
	err := c.db.QueryRow(
		`SELECT 1 FROM cache WHERE bucket = ? AND key = ? AND (expires_at = 0 OR expires_at > ?)`,
		bucket, key, now,
	).Scan(&exists)
	return err == nil
}

func (c *sqliteCache) List(bucket string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now().Unix()
	rows, err := c.db.Query(
		`SELECT key FROM cache WHERE bucket = ? AND (expires_at = 0 OR expires_at > ?)`,
		bucket, now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (c *sqliteCache) Clear(bucket string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.db.Exec(`DELETE FROM cache WHERE bucket = ?`, bucket)
	return err
}

func (c *sqliteCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.db != nil {
		return c.db.Close()
	}
	return nil
}
