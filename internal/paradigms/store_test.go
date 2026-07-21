package paradigms

import (
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestNewStoreWithStorageImportsJSONAndPersistsDB(t *testing.T) {
	dir := t.TempDir()
	legacy, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &Paradigm{ID: "p1", Name: "legacy", Side: "buy", StockCode: "000001"}
	p.Source.CacheKey = "000001:day:120:stock-paradigm-miner"
	if err := legacy.Save(p); err != nil {
		t.Fatal(err)
	}

	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := NewStoreWithStorage(dir, db)
	if err != nil {
		t.Fatal(err)
	}
	got := store.GetByCacheKey(p.Source.CacheKey)
	if got == nil || got.ID != "p1" {
		t.Fatalf("expected imported paradigm, got %#v", got)
	}

	got.Name = "updated"
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStoreWithStorage(t.TempDir(), db)
	if err != nil {
		t.Fatal(err)
	}
	got = reloaded.GetByCacheKey(p.Source.CacheKey)
	if got == nil || got.Name != "updated" {
		t.Fatalf("expected db persisted update, got %#v", got)
	}
}
