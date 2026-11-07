package newsfeed

import (
	"context"
	"testing"
)

// 水位写不进去时必须显式降级，而不是静默吞掉导致每次请求都重抓。
func TestSyncSurfacesWatermarkWriteFailure(t *testing.T) {
	store := newTestStore(t)
	feed := &fakeFeed{name: SourceEastMoney, items: []*NewsItem{newsFor("600519", "茅台新闻")}}
	svc, err := NewService(store, []Feed{feed}, testNames)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE news_sync_state`); err != nil {
		t.Fatal(err)
	}

	degraded, err := svc.sync(context.Background(), "600519")
	if err != nil {
		t.Fatalf("已有新闻落库时不应整体失败: %v", err)
	}
	if len(degraded) != 1 || degraded[0].Source != "sync_state" || degraded[0].Error == "" {
		t.Fatalf("degraded = %+v, want a sync_state entry with error detail", degraded)
	}
}

func TestMarkSyncReturnsErrorWhenStateTableMissing(t *testing.T) {
	store := newTestStore(t)
	svc, err := NewService(store, nil, testNames)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE news_sync_state`); err != nil {
		t.Fatal(err)
	}

	if err := svc.markSync("600519", 1, nil); err == nil {
		t.Fatal("markSync() error = nil, want watermark write failure")
	}
}
