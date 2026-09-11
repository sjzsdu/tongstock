package newsfeed

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	owner, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "news.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Migrate(); err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreWithStorage(owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	return store
}

func newsAt(id, title, summary string, when time.Time) *NewsItem {
	return &NewsItem{
		ID:          id,
		Source:      SourceEastMoney,
		NewsType:    NewsTypeOther,
		Title:       title,
		Summary:     summary,
		PublishTime: when,
		OriginalID:  id,
	}
}

// 回归测试：FilterNews 曾经完全忽略 RelatedStocks，导致个股资讯接口
// 返回「任意股票的最新新闻」。
func TestFilterNewsHonorsRelatedStocks(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	maotai := newsAt("n1", "贵州茅台三季度营收增长", "正文提到贵州茅台", base.Add(time.Hour))
	maotai.StockRefs = []StockRef{{Code: "600519", MatchType: MatchNameHit, Confidence: 0.95}}

	pingan := newsAt("n2", "平安银行发布半年报", "正文提到平安银行", base.Add(2*time.Hour))
	pingan.StockRefs = []StockRef{{Code: "000001", MatchType: MatchNameHit, Confidence: 0.95}}

	unrelated := newsAt("n3", "某国际新闻", "与A股无关", base.Add(3*time.Hour))

	if err := store.SaveNews(ctx, []*NewsItem{maotai, pingan, unrelated}); err != nil {
		t.Fatal(err)
	}

	got, err := store.FilterNews(ctx, FeedFilter{RelatedStocks: []string{"600519"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 {
		t.Fatalf("按 600519 过滤应返回 1 条，实际 %d 条", got.Total)
	}
	if got.Items[0].ID != "n1" {
		t.Fatalf("应返回 n1，实际 %s", got.Items[0].ID)
	}
	if len(got.Items[0].StockRefs) != 1 || got.Items[0].StockRefs[0].Code != "600519" {
		t.Fatalf("关联信息未挂载: %+v", got.Items[0].StockRefs)
	}
}

func TestFilterNewsWithoutStockFilterReturnsAll(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	a := newsAt("n1", "A", "", base)
	a.StockRefs = []StockRef{{Code: "600519", MatchType: MatchNative, Confidence: 1}}
	b := newsAt("n2", "B", "", base.Add(time.Hour))
	if err := store.SaveNews(ctx, []*NewsItem{a, b}); err != nil {
		t.Fatal(err)
	}
	got, err := store.FilterNews(ctx, FeedFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 {
		t.Fatalf("无过滤应返回全部 2 条，实际 %d", got.Total)
	}
}

// 置信度下限应能剔除「正文偶然提及」这类弱关联。
func TestFilterNewsMinConfidence(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	strong := newsAt("n1", "贵州茅台涨停", "标题命中", base)
	strong.StockRefs = []StockRef{{Code: "600519", MatchType: MatchNameHit, Confidence: 0.95}}
	weak := newsAt("n2", "百元股榜单", "表格里恰好有 600519", base.Add(time.Hour))
	weak.StockRefs = []StockRef{{Code: "600519", MatchType: MatchNameHit, Confidence: 0.4}}

	if err := store.SaveNews(ctx, []*NewsItem{strong, weak}); err != nil {
		t.Fatal(err)
	}

	all, err := store.FilterNews(ctx, FeedFilter{RelatedStocks: []string{"600519"}})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 2 {
		t.Fatalf("默认应包含弱关联，实际 %d", all.Total)
	}

	strict, err := store.FilterNews(ctx, FeedFilter{RelatedStocks: []string{"600519"}, MinConfidence: 0.8})
	if err != nil {
		t.Fatal(err)
	}
	if strict.Total != 1 || strict.Items[0].ID != "n1" {
		t.Fatalf("MinConfidence=0.8 应只返回标题命中的 n1，实际 %+v", strict.Items)
	}
}

// 保存时若注入实体识别器，应自动补上数据源没给的关联。
func TestSaveNewsBackfillsRefsFromEntityMatcher(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	store.SetEntityMatcher(NewEntityMatcher([]StockEntity{
		{Code: "600519", Name: "贵州茅台"},
		{Code: "000001", Name: "平安银行"},
	}))

	item := newsAt("n1", "市场综述", "今日贵州茅台与平安银行双双上涨", time.Now())
	if err := store.SaveNews(ctx, []*NewsItem{item}); err != nil {
		t.Fatal(err)
	}

	got, err := store.FilterNews(ctx, FeedFilter{RelatedStocks: []string{"600519"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 {
		t.Fatalf("实体识别应补上关联，实际 %d 条", got.Total)
	}
}

// 不注入识别器时不得凭空猜测关联。
func TestSaveNewsWithoutMatcherKeepsSourceRefsOnly(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	withRef := newsAt("n1", "贵州茅台大涨", "", time.Now())
	withRef.StockRefs = []StockRef{{Code: "600519", MatchType: MatchNative, Confidence: 1}}
	if err := store.SaveNews(ctx, []*NewsItem{withRef}); err != nil {
		t.Fatal(err)
	}
	got, err := store.FilterNews(ctx, FeedFilter{RelatedStocks: []string{"600519"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 {
		t.Fatalf("源自带关联应保留，实际 %d", got.Total)
	}

	// 正文提到茅台但源没给关联，且未注入识别器 —— 不应产生关联
	noRef := newsAt("n2", "贵州茅台大涨", "", time.Now())
	if err := store.SaveNews(ctx, []*NewsItem{noRef}); err != nil {
		t.Fatal(err)
	}
	got2, err := store.FilterNews(ctx, FeedFilter{RelatedStocks: []string{"600519"}})
	if err != nil {
		t.Fatal(err)
	}
	if got2.Total != 1 {
		t.Fatalf("无识别器时不应新增关联，实际 %d 条", got2.Total)
	}
}
