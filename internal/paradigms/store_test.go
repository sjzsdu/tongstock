package paradigms

import (
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestStoreWithStoragePersistsDB(t *testing.T) {
	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := NewStoreWithStorage("", db)
	if err != nil {
		t.Fatal(err)
	}
	p := &Paradigm{ID: "p1", Name: "created", Side: "buy", StockCode: "000001"}
	p.Source.CacheKey = "000001:day:120:stock-paradigm-miner"
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStoreWithStorage("", db)
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
	reloadedAgain, err := NewStoreWithStorage("", db)
	if err != nil {
		t.Fatal(err)
	}
	got = reloadedAgain.GetByCacheKey(p.Source.CacheKey)
	if got == nil || got.Name != "updated" {
		t.Fatalf("expected db persisted update, got %#v", got)
	}
}
