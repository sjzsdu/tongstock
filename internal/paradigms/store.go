package paradigms

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	dir       string
	mu        sync.RWMutex
	paradigms map[string]*Paradigm
}

func NewStore(dir string) (*Store, error) {
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".tongstock", "paradigms")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create paradigms dir: %w", err)
	}
	s := &Store{dir: dir, paradigms: make(map[string]*Paradigm)}
	if err := s.loadAll(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) loadAll() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			log.Printf("warn: read paradigm file %s failed: %v", entry.Name(), err)
			continue
		}
		var p Paradigm
		if err := json.Unmarshal(data, &p); err != nil {
			log.Printf("warn: parse paradigm file %s failed: %v", entry.Name(), err)
			continue
		}
		s.paradigms[p.ID] = &p
	}
	return nil
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

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, p.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	s.paradigms[p.ID] = p
	return nil
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
	path := filepath.Join(s.dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	delete(s.paradigms, id)
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
