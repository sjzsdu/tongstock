package paradigms

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// sqliteRepoShim is a test-only paradigms.Repository backed by pkg/storage.
// It exists so the domain package can run an integration test against the
// real SQLite backend without importing the production adapter package,
// keeping the domain's production code free of storage imports.
type sqliteRepoShim struct {
	db *storage.Storage
}

func (r *sqliteRepoShim) LoadAll() ([]*Paradigm, error) {
	rows, err := r.db.DB().Query(`SELECT data FROM paradigms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Paradigm
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var p Paradigm
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			log.Printf("warn: parse paradigm from db failed: %v", err)
			continue
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *sqliteRepoShim) Save(p *Paradigm) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	cacheKey := p.Source.CacheKey
	updated := p.UpdatedAt.Unix()
	if updated == 0 {
		updated = time.Now().Unix()
	}
	_, err = r.db.DB().Exec(`INSERT INTO paradigms (id, stock_code, side, cache_key, updated_at, data) VALUES (?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET stock_code=excluded.stock_code, side=excluded.side, cache_key=excluded.cache_key, updated_at=excluded.updated_at, data=excluded.data`,
		p.ID, p.StockCode, p.Side, cacheKey, updated, string(data))
	return err
}

func (r *sqliteRepoShim) Delete(id string) error {
	_, err := r.db.DB().Exec(fmt.Sprintf(`DELETE FROM paradigms WHERE id = %s`, "?"), id)
	return err
}
