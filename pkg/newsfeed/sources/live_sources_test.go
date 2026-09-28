package sources

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// TestLiveRepairedSources 逐个实测修好的数据源（需要外网）。
// 默认跳过：LIVE_SOURCES=1 go test ./pkg/newsfeed/sources/ -run LiveRepaired -v
func TestLiveRepairedSources(t *testing.T) {
	if os.Getenv("LIVE_SOURCES") == "" {
		t.Skip("set LIVE_SOURCES=1 to run live source checks")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	feeds := []newsfeed.Feed{
		NewSecuritiesTimesSource(),
		NewCenturyBusinessSource(),
		NewYicaiSource(),
		NewTencentFinanceSource(),
	}
	for _, f := range feeds {
		items, err := f.Fetch(ctx)
		if err != nil {
			t.Errorf("%s Fetch: %v", f.Name(), err)
			continue
		}
		if len(items) == 0 {
			t.Errorf("%s Fetch returned 0 items", f.Name())
			continue
		}
		var recent int
		for _, it := range items {
			if time.Since(it.PublishTime) < 72*time.Hour {
				recent++
			}
		}
		if recent == 0 {
			t.Errorf("%s Fetch: %d items but none within 72h, newest=%v", f.Name(), len(items), items[0].PublishTime)
			continue
		}
		t.Logf("%s: %d items (%d within 72h), newest=%v title=%q",
			f.Name(), len(items), recent, items[0].PublishTime, items[0].Title)
	}

	// 个股维度只测腾讯：它现在按 symbol 精确查询。
	items, err := NewTencentFinanceSource().FetchByStock(ctx, "600519")
	if err != nil {
		t.Errorf("tencent FetchByStock(600519): %v", err)
		return
	}
	if len(items) == 0 {
		t.Error("tencent FetchByStock(600519) returned 0 items")
		return
	}
	native := 0
	for _, it := range items {
		for _, r := range it.StockRefs {
			if r.Code == "600519" && r.MatchType == newsfeed.MatchNative {
				native++
			}
		}
	}
	if native == 0 {
		t.Errorf("tencent FetchByStock(600519): %d items, none native-linked", len(items))
	}
	t.Logf("tencent FetchByStock(600519): %d items, %d native-linked", len(items), native)
}
