package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestChatStoreWithStorageImportsJSONAndPersistsDB(t *testing.T) {
	dir := t.TempDir()
	legacy, err := NewChatStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess := &ChatSession{ID: "chat:1", StockCode: "000001", Agent: "stock-analyst", Messages: []ChatMessage{{Role: "user", Content: "hello", Timestamp: time.Now()}}}
	if err := legacy.Save(sess); err != nil {
		t.Fatal(err)
	}

	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := NewChatStoreWithStorage(dir, db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("chat:1")
	if err != nil || got.StockCode != "000001" {
		t.Fatalf("expected imported session, got %#v err=%v", got, err)
	}

	got.StockName = "平安银行"
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewChatStoreWithStorage(t.TempDir(), db)
	if err != nil {
		t.Fatal(err)
	}
	got, err = reloaded.Get("chat:1")
	if err != nil || got.StockName != "平安银行" {
		t.Fatalf("expected db persisted session, got %#v err=%v", got, err)
	}
}
