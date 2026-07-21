package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestChatStoreWithStoragePersistsDB(t *testing.T) {
	db, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := NewChatStoreWithStorage("", db)
	if err != nil {
		t.Fatal(err)
	}
	sess := &ChatSession{ID: "chat:1", StockCode: "000001", Agent: "stock-analyst", Messages: []ChatMessage{{Role: "user", Content: "hello", Timestamp: time.Now()}}}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewChatStoreWithStorage("", db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.Get("chat:1")
	if err != nil || got.StockCode != "000001" {
		t.Fatalf("expected persisted session, got %#v err=%v", got, err)
	}

	got.StockName = "平安银行"
	if err := reloaded.Save(got); err != nil {
		t.Fatal(err)
	}

	reloadedAgain, err := NewChatStoreWithStorage("", db)
	if err != nil {
		t.Fatal(err)
	}
	got, err = reloadedAgain.Get("chat:1")
	if err != nil || got.StockName != "平安银行" {
		t.Fatalf("expected db persisted session update, got %#v err=%v", got, err)
	}
}
