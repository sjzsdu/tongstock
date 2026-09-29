package newsfeed

import (
	"context"
	"testing"
	"time"
)

// 关键词搜索是信息流筛选器搜索框的支撑：标题命中、摘要命中都要算，
// 通配符必须按字面量处理，且要与其它筛选条件取交集。
func TestFilterNewsKeyword(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	byTitle := newsAt("n1", "央行宣布降准0.5个百分点", "", base)
	byTitle.Source = SourceCaiLianShe
	byTitle.NewsType = NewsTypeFlash

	bySummary := newsAt("n2", "市场高开高走", "盘中流传降准预期", base.Add(time.Hour))
	bySummary.Source = SourceEastMoney

	unrelated := newsAt("n3", "美股收盘涨跌不一", "", base.Add(2*time.Hour))
	unrelated.Source = SourceEastMoney

	if err := store.SaveNews(ctx, []*NewsItem{byTitle, bySummary, unrelated}); err != nil {
		t.Fatal(err)
	}

	got, err := store.FilterNews(ctx, FeedFilter{Keywords: []string{"降准"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 {
		t.Fatalf("关键词「降准」应命中标题与摘要共 2 条，实际 %d 条", got.Total)
	}

	// 关键词要与来源筛选取交集，而不是各自生效
	got, err = store.FilterNews(ctx, FeedFilter{
		Keywords: []string{"降准"},
		Sources:  []SourceType{SourceEastMoney},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 {
		t.Fatalf("关键词+来源应命中 1 条，实际 %d 条", got.Total)
	}
	if got.Items[0].ID != "n2" {
		t.Fatalf("应返回 n2，实际 %s", got.Items[0].ID)
	}

	// LIKE 通配符按字面量处理：搜 % 不应命中全部
	got, err = store.FilterNews(ctx, FeedFilter{Keywords: []string{"%"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 0 {
		t.Fatalf("搜字面量 %% 应命中 0 条，实际 %d 条", got.Total)
	}

	// 空关键词等同于不过滤
	got, err = store.FilterNews(ctx, FeedFilter{Keywords: []string{"   "}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 3 {
		t.Fatalf("空白关键词不应过滤，实际 %d 条", got.Total)
	}
}

func TestNewsFacets(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	items := []*NewsItem{
		newsAt("n1", "A", "", base),
		newsAt("n2", "B", "", base.Add(time.Hour)),
		newsAt("n3", "C", "", base.Add(2*time.Hour)),
	}
	flash := newsAt("n4", "D", "", base.Add(3*time.Hour))
	flash.Source = SourceCaiLianShe
	flash.NewsType = NewsTypeFlash
	items = append(items, flash)

	if err := store.SaveNews(ctx, items); err != nil {
		t.Fatal(err)
	}

	facets, err := store.NewsFacets(ctx)
	if err != nil {
		t.Fatal(err)
	}

	wantSources := []NewsFacet{{Name: string(SourceEastMoney), Count: 3}, {Name: string(SourceCaiLianShe), Count: 1}}
	if len(facets.Sources) != len(wantSources) {
		t.Fatalf("来源分布应为 %v，实际 %v", wantSources, facets.Sources)
	}
	for i, want := range wantSources {
		if facets.Sources[i] != want {
			t.Fatalf("来源分布第 %d 项应为 %v，实际 %v", i, want, facets.Sources[i])
		}
	}

	wantTypes := []NewsFacet{{Name: string(NewsTypeOther), Count: 3}, {Name: string(NewsTypeFlash), Count: 1}}
	if len(facets.Types) != len(wantTypes) {
		t.Fatalf("类型分布应为 %v，实际 %v", wantTypes, facets.Types)
	}
	for i, want := range wantTypes {
		if facets.Types[i] != want {
			t.Fatalf("类型分布第 %d 项应为 %v，实际 %v", i, want, facets.Types[i])
		}
	}
}

// 空库要返回空数组而不是 nil，调用方直接序列化成 JSON 数组即可。
func TestNewsFacetsEmptyStore(t *testing.T) {
	facets, err := newTestStore(t).NewsFacets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if facets.Sources == nil || facets.Types == nil {
		t.Fatalf("空库应返回空切片，实际 %+v", facets)
	}
	if len(facets.Sources) != 0 || len(facets.Types) != 0 {
		t.Fatalf("空库不应有分布项，实际 %+v", facets)
	}
}
