// Package paradigmrepo provides the SQLite persistence adapter for the
// paradigms domain. It implements paradigms.Repository so the domain layer
// stays free of any storage implementation import.
package paradigmrepo

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// SQLiteRepository persists Paradigm aggregates via the shared App storage.
// It is a non-owning adapter: the caller owns the *storage.Storage lifecycle.
type SQLiteRepository struct {
	db *storage.Storage
}

// NewSQLiteRepository creates a paradigms.Repository backed by the shared
// SQLite storage. Returns an error when the storage is missing or not SQLite,
// so the composition root can fail fast instead of silently degrading.
func NewSQLiteRepository(db *storage.Storage) (*SQLiteRepository, error) {
	if db == nil {
		return nil, errors.New("paradigm repository requires non-nil storage")
	}
	if db.Dialect() != storage.SQLite {
		return nil, fmt.Errorf("paradigm repository requires sqlite storage, got %q", db.Dialect())
	}
	return &SQLiteRepository{db: db}, nil
}

// LoadAll reads every persisted paradigm. Rows that fail to decode are logged
// and skipped so a single corrupt record cannot prevent startup.
func (r *SQLiteRepository) LoadAll() ([]*paradigms.Paradigm, error) {
	rows, err := r.db.DB().Query(`SELECT data FROM paradigms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*paradigms.Paradigm
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var p paradigms.Paradigm
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			log.Printf("warn: parse paradigm from db failed: %v", err)
			continue
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// Save upserts a paradigm. The dialect-aware statement keeps the adapter
// portable across the storage backends the project already supports.
func (r *SQLiteRepository) Save(p *paradigms.Paradigm) error {
	if p == nil || p.ID == "" {
		return fmt.Errorf("paradigm id required")
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	cacheKey := p.Source.CacheKey
	updated := p.UpdatedAt.Unix()
	if updated == 0 {
		updated = time.Now().Unix()
	}
	switch r.db.Dialect() {
	case storage.Postgres:
		_, err = r.db.DB().Exec(`INSERT INTO paradigms (id, stock_code, side, cache_key, updated_at, data) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT(id) DO UPDATE SET stock_code=$2, side=$3, cache_key=$4, updated_at=$5, data=$6`, p.ID, p.StockCode, p.Side, cacheKey, updated, string(data))
	case storage.MySQL:
		_, err = r.db.DB().Exec(`INSERT INTO paradigms (id, stock_code, side, cache_key, updated_at, data) VALUES (?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE stock_code=VALUES(stock_code), side=VALUES(side), cache_key=VALUES(cache_key), updated_at=VALUES(updated_at), data=VALUES(data)`, p.ID, p.StockCode, p.Side, cacheKey, updated, string(data))
	default:
		_, err = r.db.DB().Exec(`INSERT INTO paradigms (id, stock_code, side, cache_key, updated_at, data) VALUES (?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET stock_code=excluded.stock_code, side=excluded.side, cache_key=excluded.cache_key, updated_at=excluded.updated_at, data=excluded.data`, p.ID, p.StockCode, p.Side, cacheKey, updated, string(data))
	}
	return err
}

// Delete removes a paradigm by id. A missing row is not an error at the
// adapter level; the domain Store owns the not-found semantics.
func (r *SQLiteRepository) Delete(id string) error {
	placeholder := "?"
	if r.db.Dialect() == storage.Postgres {
		placeholder = "$1"
	}
	_, err := r.db.DB().Exec(`DELETE FROM paradigms WHERE id = `+placeholder, id)
	return err
}

// Compile-time contract: SQLiteRepository satisfies paradigms.Repository.
var _ paradigms.Repository = (*SQLiteRepository)(nil)
