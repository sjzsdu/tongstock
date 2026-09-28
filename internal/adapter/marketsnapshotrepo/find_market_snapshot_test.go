package marketsnapshotrepo_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// 回归测试：连接池只开一个连接（storage.SetMaxOpenConns(1)）。
// FindMarketSnapshot 只读一行、游标不会因耗尽而自动释放，必须先显式 Close
// 再调用 LoadMarketSnapshot；否则第二次查询会永久等待同一个连接。
// 「一键走通」复用已冻结快照时走的正是这条路径。
func TestFindMarketSnapshotReleasesCursorBeforeLoading(t *testing.T) {
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "snap.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	repo, err := marketsnapshotrepo.New(store)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := &marketsnapshot.MarketSnapshot{
		ID:              "snap-reuse-1",
		SnapshotDate:    "2026-07-16",
		Universe:        marketsnapshot.UniverseDefinition{Name: "universe_usable"},
		Market:          "CN-A",
		PriceAdjustment: "forward",
		CoveragePct:     1,
		Status:          marketsnapshot.StatusReady,
		Codes:           []marketsnapshot.CodeStatus{{Code: "000063", UniverseMember: true}},
		BuiltAt:         time.Now(),
	}
	if err := repo.SaveMarketSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := repo.FreezeMarketSnapshot(snapshot.ID); err != nil {
		t.Fatal(err)
	}

	type result struct {
		snap *marketsnapshot.MarketSnapshot
		err  error
	}
	done := make(chan result, 1)
	go func() {
		snap, err := repo.FindMarketSnapshot("2026-07-16", "universe_usable", "forward")
		done <- result{snap, err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("FindMarketSnapshot: %v", got.err)
		}
		if got.snap == nil || got.snap.ID != snapshot.ID {
			t.Fatalf("unexpected snapshot: %+v", got.snap)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("FindMarketSnapshot deadlocked: the cursor kept the only pooled connection")
	}
}
