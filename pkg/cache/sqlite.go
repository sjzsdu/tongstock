package cache

import (
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

type sqliteCache struct {
	db    *sql.DB
	mu    sync.RWMutex
	owner *storage.Storage
}

// NewSQLiteCache creates a standalone compatibility cache through the same
// storage factory and versioned migrations used by the application.
func NewSQLiteCache(dbPath string) (Cache, error) {
	owner, err := storage.New(storage.Config{Driver: "sqlite3", DSN: dbPath})
	if err != nil {
		return nil, err
	}
	return &sqliteCache{db: owner.DB(), owner: owner}, nil
}

// NewSQLiteCacheWithDB creates a non-owning cache adapter on the App-owned
// database. The schema is installed by storage migrations.
func NewSQLiteCacheWithDB(db *sql.DB) (Cache, error) {
	if db == nil {
		return nil, errors.New("nil database")
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

	if c.owner != nil {
		return c.owner.Close()
	}
	return nil
}
