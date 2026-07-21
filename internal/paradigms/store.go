package paradigms

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

type Store struct {
	mu        sync.RWMutex
	paradigms map[string]*Paradigm
	db        *storage.Storage
}

func NewStore(dir string) (*Store, error) {
	return &Store{paradigms: make(map[string]*Paradigm)}, nil
}

func NewStoreWithStorage(dir string, db *storage.Storage) (*Store, error) {
	s := &Store{paradigms: make(map[string]*Paradigm), db: db}
	if db != nil {
		if err := s.initDB(); err != nil {
			return nil, err
		}
		if err := s.loadAllDB(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) initDB() error {
	_, err := s.db.DB().Exec(`CREATE TABLE IF NOT EXISTS paradigms (
		id TEXT PRIMARY KEY,
		stock_code TEXT NOT NULL DEFAULT '',
		side TEXT NOT NULL DEFAULT '',
		cache_key TEXT NOT NULL DEFAULT '',
		updated_at BIGINT NOT NULL,
		data TEXT NOT NULL
	)`)
	if err != nil {
		return err
	}
	_, _ = s.db.DB().Exec(`CREATE INDEX IF NOT EXISTS idx_paradigms_stock_code ON paradigms(stock_code)`)
	_, _ = s.db.DB().Exec(`CREATE INDEX IF NOT EXISTS idx_paradigms_cache_key ON paradigms(cache_key)`)
	return nil
}

func (s *Store) loadAllDB() error {
	rows, err := s.db.DB().Query(`SELECT data FROM paradigms`)
	if err != nil {
		return err
	}
	defer rows.Close()
	loaded := map[string]*Paradigm{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var p Paradigm
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			log.Printf("warn: parse paradigm from db failed: %v", err)
			continue
		}
		loaded[p.ID] = &p
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.paradigms = loaded
	s.mu.Unlock()
	return nil
}

func (s *Store) saveDB(p *Paradigm) error {
	if s.db == nil {
		return nil
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
	switch s.db.Dialect() {
	case storage.Postgres:
		_, err = s.db.DB().Exec(`INSERT INTO paradigms (id, stock_code, side, cache_key, updated_at, data) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT(id) DO UPDATE SET stock_code=$2, side=$3, cache_key=$4, updated_at=$5, data=$6`, p.ID, p.StockCode, p.Side, cacheKey, updated, string(data))
	case storage.MySQL:
		_, err = s.db.DB().Exec(`INSERT INTO paradigms (id, stock_code, side, cache_key, updated_at, data) VALUES (?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE stock_code=VALUES(stock_code), side=VALUES(side), cache_key=VALUES(cache_key), updated_at=VALUES(updated_at), data=VALUES(data)`, p.ID, p.StockCode, p.Side, cacheKey, updated, string(data))
	default:
		_, err = s.db.DB().Exec(`INSERT INTO paradigms (id, stock_code, side, cache_key, updated_at, data) VALUES (?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET stock_code=excluded.stock_code, side=excluded.side, cache_key=excluded.cache_key, updated_at=excluded.updated_at, data=excluded.data`, p.ID, p.StockCode, p.Side, cacheKey, updated, string(data))
	}
	return err
}

func (s *Store) Save(p *Paradigm) error {
	if p == nil || p.ID == "" {
		return fmt.Errorf("paradigm id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	s.paradigms[p.ID] = p
	return s.saveDB(p)
}

func (s *Store) Get(id string) (*Paradigm, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.paradigms[id]
	if !ok {
		return nil, fmt.Errorf("paradigm %q not found", id)
	}
	return p, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.paradigms[id]; !ok {
		return fmt.Errorf("paradigm %q not found", id)
	}
	delete(s.paradigms, id)
	if s.db != nil {
		placeholder := "?"
		if s.db.Dialect() == storage.Postgres {
			placeholder = "$1"
		}
		_, _ = s.db.DB().Exec(`DELETE FROM paradigms WHERE id = `+placeholder, id)
	}
	return nil
}

func (s *Store) List() []*Paradigm {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Paradigm, 0, len(s.paradigms))
	for _, p := range s.paradigms {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Store) ListByContext(ctx Context) []*Paradigm {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Paradigm, 0)
	for _, p := range s.paradigms {
		if ctx.MarketCap != "" && p.Context.MarketCap != ctx.MarketCap {
			continue
		}
		if ctx.ShareholderDominant != "" && p.Context.ShareholderDominant != ctx.ShareholderDominant {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.paradigms)
}

func (s *Store) GetByStockCode(code string) *Paradigm {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.paradigms {
		if p.StockCode == code {
			return p
		}
	}
	return nil
}

func (s *Store) GetByCacheKey(cacheKey string) *Paradigm {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if cacheKey == "" {
		return nil
	}
	for _, p := range s.paradigms {
		if p.Source.CacheKey == cacheKey {
			return p
		}
	}
	return nil
}

func (s *Store) GetByStockCodeAndSide(code, side string) *Paradigm {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.paradigms {
		if p.StockCode == code && p.Side == side {
			return p
		}
	}
	return nil
}

func (s *Store) ListByStockCode(code string) []*Paradigm {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Paradigm
	for _, p := range s.paradigms {
		if p.StockCode == code {
			out = append(out, p)
		}
	}
	return out
}
