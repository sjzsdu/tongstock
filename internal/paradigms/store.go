package paradigms

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Repository is the persistence port for the Paradigm aggregate. The domain
// layer depends on this interface only; concrete storage implementations
// (SQLite, etc.) live in adapter packages and are injected at the composition
// root. This keeps the domain free of any storage SDK import.
type Repository interface {
	// LoadAll returns every persisted paradigm. Decode-tolerant: individual
	// corrupt records may be skipped by the adapter.
	LoadAll() ([]*Paradigm, error)
	// Save upserts a single paradigm.
	Save(p *Paradigm) error
	// Delete removes a paradigm by id. A missing row is not an error.
	Delete(id string) error
}

// Store is the domain service over Paradigm aggregates. It maintains an
// in-memory write-through cache backed by a Repository port so reads stay
// local while writes are durably persisted by the injected adapter.
type Store struct {
	mu        sync.RWMutex
	paradigms map[string]*Paradigm
	repo      Repository
}

// NewStore creates an in-memory-only store when no repository is configured.
// Production callers should prefer NewStoreWithRepository so paradigms are
// durably persisted.
func NewStore() (*Store, error) {
	return &Store{
		paradigms: make(map[string]*Paradigm),
	}, nil
}

// NewStoreWithRepository creates a Store backed by the supplied repository.
// All persisted paradigms are loaded into the in-memory cache on construction.
// A nil repo produces an in-memory-only store.
func NewStoreWithRepository(repo Repository) (*Store, error) {
	s := &Store{
		paradigms: make(map[string]*Paradigm),
		repo:      repo,
	}
	if repo != nil {
		if err := s.loadAll(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) loadAll() error {
	loaded, err := s.repo.LoadAll()
	if err != nil {
		return err
	}
	cache := make(map[string]*Paradigm, len(loaded))
	for _, p := range loaded {
		cache[p.ID] = p
	}
	s.mu.Lock()
	s.paradigms = cache
	s.mu.Unlock()
	return nil
}

// Save persists a paradigm through the repository and updates the cache.
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

	if s.repo != nil {
		if err := s.repo.Save(p); err != nil {
			return err
		}
	}
	s.paradigms[p.ID] = p
	return nil
}

// Get returns a paradigm by id. Returns an error when not found.
func (s *Store) Get(id string) (*Paradigm, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.paradigms[id]
	if !ok {
		return nil, fmt.Errorf("paradigm %q not found", id)
	}
	return p, nil
}

// Delete removes a paradigm by id from both the cache and the repository.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.paradigms[id]; !ok {
		return fmt.Errorf("paradigm %q not found", id)
	}
	delete(s.paradigms, id)
	if s.repo != nil {
		return s.repo.Delete(id)
	}
	return nil
}

// List returns all paradigms sorted by UpdatedAt descending.
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

// GetByStockCode returns the first paradigm matching the stock code.
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

// GetByCacheKey returns the paradigm with the matching cache key.
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

// ListByStockCode returns all paradigms for the given stock code.
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
