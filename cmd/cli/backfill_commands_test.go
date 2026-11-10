package main

import (
	"github.com/sjzsdu/tongstock/pkg/storage"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateBackfillReservesWriterBeforeRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "states.db")
	first, err := storage.New(storage.Config{DSN: stateBackfillDSN(path)})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := storage.New(storage.Config{DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	// Short timeout makes a real two-connection lock test deterministic and fast.
	if _, err = second.DB().Exec(`PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}
	tx, err := first.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = second.DB().Exec(`INSERT INTO signal_state_seed VALUES('000001',9,'{}','',0,'')`); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("writer was not reserved: %v", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = second.DB().Exec(`INSERT INTO signal_state_seed VALUES('000001',9,'{}','',0,'')`); err != nil {
		t.Fatal(err)
	}
}
