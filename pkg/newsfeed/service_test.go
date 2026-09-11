package newsfeed

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 测试用的假数据源，可控成功/失败与返回内容
type fakeFeed struct {
	name   SourceType
	items  []*NewsItem
	err    error
	byCode map[string][]*NewsItem
	calls  int
}

func (f *fakeFeed) Name() SourceType                 { return f.name }
func (f *fakeFeed) RefreshInterval() time.Duration   { return time.Minute }
func (f *fakeFeed) HealthCheck(context.Context) bool { return f.err == nil }

func (f *fakeFeed) Fetch(context.Context) ([]*NewsItem, error) {
	f.calls++
	return f.items, f.err
}

func (f *fakeFeed) FetchByStock(_ context.Context, code string) ([]*NewsItem, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.byCode != nil {
		return f.byCode[code], nil
	}
	out := make([]*NewsItem, 0, len(f.items))
	for _, it := range f.items {
		out = append(out, it)
	}
	return out, nil
}

func (f *fakeFeed) FetchByKeyword(context.Context, string) ([]*NewsItem, error) {
	return f.items, f.err
}

type staticDirectory struct {
	names map[string]string
}

func (d staticDirectory) ListEntities(context.Context) ([]StockEntity, error) {
	out := make([]StockEntity, 0, len(d.names))
	for c, n := range d.names {
		out = append(out, StockEntity{Code: c, Name: n})
	}
	return out, nil
}

func (d staticDirectory) NameOf(_ context.Context, code string) (string, bool) {
	n, ok := d.names[code]
	return n, ok
}

func newsFor(code, title string) *NewsItem {
	return &NewsItem{
		Source:      SourceEastMoney,
		NewsType:    NewsTypeOther,
		Title:       title,
		Summary:     title,
		PublishTime: time.Now(),
		OriginalID:  title,
		StockRefs:   []StockRef{{Code: code, MatchType: MatchNative, Confidence: 1}},
	}
}

func TestServiceStockNewsFetchesThenReads(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	feed := &fakeFeed{name: SourceEastMoney, byCode: map[string][]*NewsItem{
		"600519": {newsFor("600519", "贵州茅台三季报")},
	}}
	svc, err := NewService(store, []Feed{feed}, staticDirectory{names: map[string]string{"600519": "贵州茅台"}})
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.StockNews(ctx, StockNewsRequest{Code: "600519"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ok" {
		t.Fatalf("status = %s, msg = %s", got.Status, got.Message)
	}
	if len(got.Items) != 1 || got.Items[0].Title != "贵州茅台三季报" {
		t.Fatalf("items = %+v", got.Items)
	}
}

// 第二次查询应命中新鲜度窗口，不再重复抓取。
func TestServiceRespectsFreshnessWindow(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	feed := &fakeFeed{name: SourceEastMoney, byCode: map[string][]*NewsItem{
		"600519": {newsFor("600519", "A")},
	}}
	svc, _ := NewService(store, []Feed{feed}, staticDirectory{names: map[string]string{"600519": "贵州茅台"}})

	if _, err := svc.StockNews(ctx, StockNewsRequest{Code: "600519"}); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := feed.calls
	if _, err := svc.StockNews(ctx, StockNewsRequest{Code: "600519"}); err != nil {
		t.Fatal(err)
	}
	if feed.calls != callsAfterFirst {
		t.Fatalf("新鲜窗口内不应重复抓取: %d -> %d", callsAfterFirst, feed.calls)
	}

	// --refresh 应强制重新抓取
	if _, err := svc.StockNews(ctx, StockNewsRequest{Code: "600519", ForceRefresh: true}); err != nil {
		t.Fatal(err)
	}
	if feed.calls == callsAfterFirst {
		t.Fatal("--refresh 应触发抓取")
	}
}

// cache_only 绝不访问网络
func TestServiceCacheOnlyNeverFetches(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	feed := &fakeFeed{name: SourceEastMoney, byCode: map[string][]*NewsItem{
		"600519": {newsFor("600519", "A")},
	}}
	svc, _ := NewService(store, []Feed{feed}, staticDirectory{names: map[string]string{"600519": "贵州茅台"}})

	got, err := svc.StockNews(ctx, StockNewsRequest{Code: "600519", Mode: NewsCacheOnly})
	if err != nil {
		t.Fatal(err)
	}
	if feed.calls != 0 {
		t.Fatalf("cache_only 不应抓取, calls=%d", feed.calls)
	}
	if got.Status != "insufficient_data" {
		t.Fatalf("无数据应报 insufficient_data, got %s", got.Status)
	}
}

// require_fresh 下源失败必须报错，不能用空列表蒙混
func TestServiceRequireFreshFailsLoudly(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	feed := &fakeFeed{name: SourceEastMoney, err: errors.New("上游 502")}
	svc, _ := NewService(store, []Feed{feed}, staticDirectory{names: map[string]string{"600519": "贵州茅台"}})

	_, err := svc.StockNews(ctx, StockNewsRequest{Code: "600519", Mode: NewsRequireFresh})
	if err == nil {
		t.Fatal("require_fresh 下源失败必须返回错误")
	}
	if !errors.Is(err, ErrFetchUnavailable) {
		t.Fatalf("期望 ErrFetchUnavailable, got %v", err)
	}
}

// allow_stale 下源失败应返回库内旧数据并标记 stale
func TestServiceAllowStaleFallsBack(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	seed := newsFor("600519", "旧新闻")
	if err := store.SaveNews(ctx, []*NewsItem{seed}); err != nil {
		t.Fatal(err)
	}
	feed := &fakeFeed{name: SourceEastMoney, err: errors.New("上游 502")}
	svc, _ := NewService(store, []Feed{feed}, staticDirectory{names: map[string]string{"600519": "贵州茅台"}})

	got, err := svc.StockNews(ctx, StockNewsRequest{Code: "600519", Mode: NewsAllowStale})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stale" {
		t.Fatalf("status = %s", got.Status)
	}
	if len(got.Items) != 1 {
		t.Fatalf("应返回库内旧数据, got %d", len(got.Items))
	}
	if len(got.Degraded) != 1 {
		t.Fatalf("降级信息必须显式暴露, got %+v", got.Degraded)
	}
}

// 部分源失败属于降级，不应整体失败
func TestServicePartialFailureIsDegradation(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ok := &fakeFeed{name: SourceEastMoney, byCode: map[string][]*NewsItem{
		"600519": {newsFor("600519", "A")},
	}}
	bad := &fakeFeed{name: SourceCaiLianShe, err: errors.New("超时")}
	svc, _ := NewService(store, []Feed{ok, bad}, staticDirectory{names: map[string]string{"600519": "贵州茅台"}})

	got, err := svc.StockNews(ctx, StockNewsRequest{Code: "600519"})
	if err != nil {
		t.Fatalf("部分源失败不应整体失败: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("应返回成功源的数据, got %d", len(got.Items))
	}
	if len(got.Degraded) != 1 || got.Degraded[0].Source != SourceCaiLianShe {
		t.Fatalf("应记录财联社降级, got %+v", got.Degraded)
	}
}

// 别的股票的新闻绝不能出现在结果里
func TestServiceNeverLeaksOtherStocks(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	if err := store.SaveNews(ctx, []*NewsItem{
		newsFor("000001", "平安银行的新闻"),
	}); err != nil {
		t.Fatal(err)
	}
	feed := &fakeFeed{name: SourceEastMoney}
	svc, _ := NewService(store, []Feed{feed}, staticDirectory{names: map[string]string{"600519": "贵州茅台"}})

	got, err := svc.StockNews(ctx, StockNewsRequest{Code: "600519", Mode: NewsCacheOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 0 {
		t.Fatalf("不应返回其他股票的新闻: %+v", got.Items)
	}
	if got.Status != "insufficient_data" {
		t.Fatalf("status = %s", got.Status)
	}
}
