package newsfeed

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestIsCodeToken(t *testing.T) {
	cases := map[string]bool{
		"20cm3":    true,  // 规格噪声
		"7x24":     true,  // 栏目噪声
		"sz000408": true,  // 股票代码噪声
		"a50":      true,  // 型号噪声
		"openai":   false, // 纯英文术语
		"kkr":      false, // 纯英文缩写
		"mdi":      false, // 纯英文缩写
		"2026":     false, // 纯数字，由 isPureDigits 负责
	}

	for word, want := range cases {
		if got := isCodeToken(word); got != want {
			t.Errorf("isCodeToken(%q) = %v, want %v", word, got, want)
		}
	}
}

// 同名事件去重只保留一条，且保留热度更高者：
// 此前去重键含全部关键词，同名事件关键词尾集不同就全部保留。
func TestDeduplicateEventsByTitle(t *testing.T) {
	c := &Clusterer{config: DefaultClusterConfig()}

	events := []*HotEvent{
		{Title: "ai openai", Keywords: []string{"ai", "openai", "claude"}, HotIndex: 80},
		{Title: "AI OpenAI", Keywords: []string{"ai", "openai"}, HotIndex: 90},
		{Title: "ai claude", Keywords: []string{"ai", "claude"}, HotIndex: 70},
	}

	unique := c.deduplicateEvents(events)
	if len(unique) != 2 {
		t.Fatalf("want 2 unique events, got %d: %v", len(unique), titles(unique))
	}
	if unique[0].HotIndex != 90 {
		t.Errorf("want hotter duplicate (90) to survive, got %d", unique[0].HotIndex)
	}
}

// 旧事件的信息（新闻、平台计数、股票）应并入同名新事件。
func TestMergeOlder(t *testing.T) {
	newEvent := &HotEvent{
		Title:         "ai openai",
		NewsItemIDs:   []string{"n1", "n2"},
		SourceCounts:  map[string]int{"财联社": 2},
		RelatedStocks: []string{"600519"},
		HotIndex:      70,
	}
	old := &HotEvent{
		Title:         "ai openai",
		NewsItemIDs:   []string{"n2", "n3"},
		SourceCounts:  map[string]int{"财联社": 1, "东方财富": 2},
		RelatedStocks: []string{"000001", "600519"},
		HotIndex:      95,
	}

	newEvent.mergeOlder(old)

	if len(newEvent.NewsItemIDs) != 3 {
		t.Errorf("want 3 merged news ids, got %v", newEvent.NewsItemIDs)
	}
	if newEvent.SourceCounts["财联社"] != 3 || newEvent.SourceCounts["东方财富"] != 2 {
		t.Errorf("source counts not merged: %v", newEvent.SourceCounts)
	}
	if len(newEvent.RelatedStocks) != 2 {
		t.Errorf("stocks not merged: %v", newEvent.RelatedStocks)
	}
	if newEvent.HotIndex != 95 {
		t.Errorf("want hotter index preserved, got %d", newEvent.HotIndex)
	}
}

// 回归测试：同名事件曾经既不去重也不互相覆盖，越积越多。
// 刷新时应把库里的同名旧事件合并进新事件后删除，同名只保留一条。
func TestRefreshEventsReplacesSameTitleEvents(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	base := time.Now().Add(-30 * time.Minute)

	var batch []*NewsItem
	for i := 0; i < 3; i++ {
		batch = append(batch, newsAt(
			"n1"+string(rune('a'+i)),
			"OpenAI releases new ai model",
			"openai ai model update",
			base.Add(time.Duration(i)*time.Minute),
		))
	}
	if err := store.SaveNews(ctx, batch); err != nil {
		t.Fatal(err)
	}

	clusterer := &Clusterer{config: DefaultClusterConfig(), store: store}
	if _, err := clusterer.RefreshEvents(ctx); err != nil {
		t.Fatal(err)
	}

	// 模拟历史遗留的同名旧事件：不同 ID、不同关键词集合（正是线上
	// 「ai openai」x3 的形态）。
	legacy := &HotEvent{
		ID:           "event_legacy0000001",
		Title:        "AI OpenAI",
		Keywords:     []string{"ai", "openai", "claude", "anthropic"},
		HotIndex:     99,
		Status:       EventStatusActive,
		NewsItemIDs:  []string{"legacy-news-1"},
		SourceCounts: map[string]int{"华尔街见闻": 3},
	}
	if err := store.SaveHotEvent(ctx, legacy); err != nil {
		t.Fatal(err)
	}

	if _, err := clusterer.RefreshEvents(ctx); err != nil {
		t.Fatal(err)
	}

	result, err := store.GetHotEvents(ctx, HotEventFilter{Status: []EventStatus{EventStatusActive, EventStatusCooling}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range result.Items {
		if strings.EqualFold(item.Title, "ai openai") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("want exactly 1 'ai openai' event, got %d: %v", count, summaryTitles(result.Items))
	}

	// 合并进来的旧新闻应保留，事件详情能查到 legacy 新闻。
	detail, err := store.GetHotEventDetail(ctx, result.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	foundLegacy := false
	for _, id := range detail.NewsItemIDs {
		if id == "legacy-news-1" {
			foundLegacy = true
		}
	}
	if !foundLegacy {
		t.Errorf("legacy news not merged into refreshed event: %v", detail.NewsItemIDs)
	}
}

func titles(events []*HotEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Title)
	}
	return out
}

func summaryTitles(events []EventSummary) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Title)
	}
	return out
}
