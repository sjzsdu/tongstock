package paradigms

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// memoryRepo is an in-memory paradigms.Repository used to exercise the Store
// without binding the domain test to any storage adapter.
type memoryRepo struct {
	mu         sync.Mutex
	byID       map[string]*Paradigm
	loadErr    error
	saveErr    error
	deleteErr  error
	deleteHook func(id string)
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{byID: make(map[string]*Paradigm)}
}

func (m *memoryRepo) LoadAll() ([]*Paradigm, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Paradigm, 0, len(m.byID))
	for _, p := range m.byID {
		clone := *p
		out = append(out, &clone)
	}
	return out, nil
}

func (m *memoryRepo) Save(p *Paradigm) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := *p
	m.byID[p.ID] = &clone
	return nil
}

func (m *memoryRepo) Delete(id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byID, id)
	if m.deleteHook != nil {
		m.deleteHook(id)
	}
	return nil
}

func TestStoreWithRepositoryPersistsAndReloads(t *testing.T) {
	repo := newMemoryRepo()
	store, err := NewStoreWithRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	p := &Paradigm{ID: "p1", Name: "created", Side: "buy", StockCode: "000001"}
	p.Source.CacheKey = "000001:day:120:stock-paradigm-miner"
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStoreWithRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.GetByCacheKey(p.Source.CacheKey)
	if got == nil || got.Name != "created" {
		t.Fatalf("expected persisted paradigm, got %#v", got)
	}

	got.Name = "updated"
	if err := reloaded.Save(got); err != nil {
		t.Fatal(err)
	}
	reloadedAgain, err := NewStoreWithRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	got = reloadedAgain.GetByCacheKey(p.Source.CacheKey)
	if got == nil || got.Name != "updated" {
		t.Fatalf("expected persisted update, got %#v", got)
	}
}

// TestStoreWithStoragePersistsDB keeps a real-SQLite integration path so the
// domain package can verify the cache+repository contract against the actual
// storage backend the adapter uses in production.
func TestStoreWithStoragePersistsDB(t *testing.T) {
	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Use a thin in-package adapter shim rather than importing the
	// paradigmrepo package, so this domain test stays self-contained.
	repo := &sqliteRepoShim{db: db}
	store, err := NewStoreWithRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	p := &Paradigm{ID: "p1", Name: "created", Side: "buy", StockCode: "000001"}
	p.Source.CacheKey = "000001:day:120:stock-paradigm-miner"
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStoreWithRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.GetByCacheKey(p.Source.CacheKey)
	if got == nil || got.Name != "created" {
		t.Fatalf("expected db persisted paradigm, got %#v", got)
	}

	got.Name = "updated"
	if err := reloaded.Save(got); err != nil {
		t.Fatal(err)
	}
	reloadedAgain, err := NewStoreWithRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	got = reloadedAgain.GetByCacheKey(p.Source.CacheKey)
	if got == nil || got.Name != "updated" {
		t.Fatalf("expected db persisted update, got %#v", got)
	}
}
