package stockpool

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// StockPoolFilter represents a filter condition for a stock pool
type StockPoolFilter struct {
	Field    string          `json:"field"`
	Operator string          `json:"operator"`
	Value    json.RawMessage `json:"value"`
	Label    string          `json:"label,omitempty"`
}

// StockPool represents a custom stock pool
type StockPool struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Filters     []StockPoolFilter `json:"filters"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Store manages stock pool storage
type Store struct {
	s *storage.Storage
}

// New creates a new stock pool store
func New(s *storage.Storage) (*Store, error) {
	return &Store{s: s}, nil
}

// GetAll returns all stock pools
func (s *Store) GetAll() ([]StockPool, error) {
	rows, err := s.s.DB().Query(`SELECT id, name, description, filters, created_at, updated_at FROM stockpool ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pools []StockPool
	for rows.Next() {
		var pool StockPool
		var filtersRaw string
		var addedAt, updatedAt int64
		if err := rows.Scan(&pool.ID, &pool.Name, &pool.Description, &filtersRaw, &addedAt, &updatedAt); err != nil {
			return nil, err
		}
		if filtersRaw != "" {
			if err := json.Unmarshal([]byte(filtersRaw), &pool.Filters); err != nil {
				return nil, err
			}
		}
		pool.CreatedAt = time.Unix(addedAt, 0)
		pool.UpdatedAt = time.Unix(updatedAt, 0)
		pools = append(pools, pool)
	}
	return pools, rows.Err()
}

// GetByID returns a stock pool by ID
func (s *Store) GetByID(id string) (*StockPool, error) {
	var pool StockPool
	var filtersRaw string
	var addedAt, updatedAt int64

	query := fmt.Sprintf(`SELECT id, name, description, filters, created_at, updated_at FROM stockpool WHERE id = %s`, s.ph(1))
	err := s.s.DB().QueryRow(query, id).Scan(&pool.ID, &pool.Name, &pool.Description, &filtersRaw, &addedAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	if filtersRaw != "" {
		if err := json.Unmarshal([]byte(filtersRaw), &pool.Filters); err != nil {
			return nil, err
		}
	}
	pool.CreatedAt = time.Unix(addedAt, 0)
	pool.UpdatedAt = time.Unix(updatedAt, 0)
	return &pool, nil
}

// Upsert inserts or updates a stock pool
func (s *Store) Upsert(pool StockPool) error {
	if pool.ID == "" {
		return fmt.Errorf("id is required")
	}
	if pool.Name == "" {
		return fmt.Errorf("name is required")
	}

	filtersRaw, err := json.Marshal(pool.Filters)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	if pool.CreatedAt.IsZero() {
		pool.CreatedAt = time.Now()
	}

	switch s.s.Dialect() {
	case storage.Postgres:
		_, err := s.s.DB().Exec(`
			INSERT INTO stockpool (id, name, description, filters, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT(id) DO UPDATE SET
				name = $2,
				description = $3,
				filters = $4,
				updated_at = $6
		`, pool.ID, pool.Name, pool.Description, string(filtersRaw), pool.CreatedAt.Unix(), now)
		return err
	case storage.MySQL:
		_, err := s.s.DB().Exec(`
			INSERT INTO stockpool (id, name, description, filters, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				name = VALUES(name),
				description = VALUES(description),
				filters = VALUES(filters),
				updated_at = VALUES(updated_at)
		`, pool.ID, pool.Name, pool.Description, string(filtersRaw), pool.CreatedAt.Unix(), now)
		return err
	default: // SQLite
		_, err := s.s.DB().Exec(`
			INSERT INTO stockpool (id, name, description, filters, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				filters = excluded.filters,
				updated_at = ?
		`, pool.ID, pool.Name, pool.Description, string(filtersRaw), pool.CreatedAt.Unix(), now, now)
		return err
	}
}

// Delete removes a stock pool by ID
func (s *Store) Delete(id string) error {
	query := fmt.Sprintf(`DELETE FROM stockpool WHERE id = %s`, s.ph(1))
	_, err := s.s.DB().Exec(query, id)
	return err
}

func (s *Store) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}
