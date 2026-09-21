package newsfeed

import (
	"context"
	"testing"
	"time"
)

// seedTopicNews 写入一条指定日期、指定关联的新闻。
func seedTopicNews(t *testing.T, ctx context.Context, store *SQLiteStore, id, title, code, matchType string, conf float64, at time.Time, hot int) {
	t.Helper()
	item := newsAt(id, title, "", at)
	item.HotScore = hot
	item.Source = SourceCaiLianShe
	if code != "" {
		item.StockRefs = []StockRef{{Code: code, MatchType: matchType, Confidence: conf}}
	}
	if err := store.SaveNews(ctx, []*NewsItem{item}); err != nil {
		t.Fatal(err)
	}
}

// testNames 是榜单测试共用的股票名录。名录同时决定「哪些代码算股票」，
// 与生产语义一致：名录里没有的代码（如指数）不入榜。
var testNames = staticDirectory{names: map[string]string{
	"600519": "贵州茅台",
	"000001": "平安银行",
	"000002": "万科A",
	"000004": "国华网安",
}}

func TestParseTopicDate(t *testing.T) {
	fixed := func() time.Time { return time.Date(2026, 9, 17, 15, 0, 0, 0, time.Local) }
	day, err := parseTopicDate("2026-09-01", fixed)
	if err != nil || day.Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("parse 2006-01-02 failed: %v %v", day, err)
	}
	day, err = parseTopicDate("20260901", fixed)
	if err != nil || day.Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("parse 20060102 failed: %v %v", day, err)
	}
	day, err = parseTopicDate("", fixed)
	if err != nil || day.Format("2006-01-02") != "2026-09-17" {
		t.Fatalf("空日期应表示今天: %v %v", day, err)
	}
	if _, err := parseTopicDate("09/01", fixed); err == nil {
		t.Fatal("非法日期应报错")
	}
}

// 基础聚合：按热度分排序，附带简称与代表新闻。
func TestHotTopicsAggregatesAndSorts(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.Local)

	// 茅台：3 条强关联（含 1 条原生），热度更高。
	seedTopicNews(t, ctx, store, "m1", "贵州茅台获北向资金加仓", "600519", MatchNative, 1.0, base.Add(1*time.Hour), 80)
	seedTopicNews(t, ctx, store, "m2", "茅台批价企稳回升", "600519", MatchNameHit, 0.9, base.Add(2*time.Hour), 60)
	seedTopicNews(t, ctx, store, "m3", "白酒板块异动 茅台领涨", "600519", MatchNameHit, 0.9, base.Add(3*time.Hour), 40)
	// 平安：1 条强关联。
	seedTopicNews(t, ctx, store, "p1", "平安银行发布中期分红方案", "000001", MatchNameHit, 0.9, base.Add(4*time.Hour), 70)

	svc, err := NewService(store, []Feed{}, testNames)
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.HotStockTopics(ctx, HotTopicsRequest{Date: "2026-09-17", Mode: NewsCacheOnly})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ok" {
		t.Fatalf("status = %s, msg = %s", got.Status, got.Message)
	}
	if len(got.Items) != 2 {
		t.Fatalf("应返回 2 只热门股，实际 %d", len(got.Items))
	}
	if got.Items[0].Code != "600519" {
		t.Fatalf("茅台应排第一，实际 %s", got.Items[0].Code)
	}
	maotai := got.Items[0]
	if maotai.Mentions != 3 || maotai.NativeHits != 1 || maotai.TitleHits != 3 {
		t.Fatalf("茅台计数错误: %+v", maotai)
	}
	if maotai.Name != "贵州茅台" {
		t.Fatalf("应解析简称: %+v", maotai)
	}
	if len(maotai.Headlines) == 0 || maotai.Headlines[0].NewsID != "m1" {
		t.Fatalf("代表新闻应按热度取 m1: %+v", maotai.Headlines)
	}
	if len(maotai.Sources) != 1 || maotai.Sources[0] != string(SourceCaiLianShe) {
		t.Fatalf("来源列表错误: %+v", maotai.Sources)
	}
}

// 弱关联（正文偶然提及）不计入榜单。
func TestHotTopicsExcludesWeakRefs(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.Local)

	seedTopicNews(t, ctx, store, "strong", "贵州茅台涨停", "600519", MatchNameHit, 0.95, base, 50)
	seedTopicNews(t, ctx, store, "weak", "百元股榜单", "600519", MatchNameHit, 0.4, base.Add(time.Hour), 50)

	svc, _ := NewService(store, []Feed{}, testNames)
	got, err := svc.HotStockTopics(ctx, HotTopicsRequest{Date: "2026-09-17", Mode: NewsCacheOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Mentions != 1 {
		t.Fatalf("弱关联不应计入: %+v", got.Items)
	}
}

// 日期窗口必须严格：只统计目标当天的新闻。
func TestHotTopicsRespectsDateWindow(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	day := time.Date(2026, 9, 17, 10, 0, 0, 0, time.Local)

	seedTopicNews(t, ctx, store, "today", "今日新闻", "600519", MatchNative, 1.0, day, 50)
	seedTopicNews(t, ctx, store, "yesterday", "昨日新闻", "600519", MatchNative, 1.0, day.Add(-24*time.Hour), 90)
	seedTopicNews(t, ctx, store, "tomorrow", "明日新闻", "600519", MatchNative, 1.0, day.Add(24*time.Hour), 90)

	svc, _ := NewService(store, []Feed{}, testNames)
	got, err := svc.HotStockTopics(ctx, HotTopicsRequest{Date: "2026-09-17", Mode: NewsCacheOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Mentions != 1 {
		t.Fatalf("只应统计当天 1 条: %+v", got.Items)
	}
}

// topic 是从媒体新闻中聚合热点，与是否交易日无关。
func TestHotTopicsMediaNewsAggregation(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// 周六 2026-09-19 有新闻，应该直接返回周六的数据。
	weekend := time.Date(2026, 9, 19, 10, 0, 0, 0, time.Local)
	seedTopicNews(t, ctx, store, "sat", "周末消息", "600519", MatchNative, 1.0, weekend, 50)

	svc, _ := NewService(store, []Feed{}, testNames)
	got, err := svc.HotStockTopics(ctx, HotTopicsRequest{Date: "2026-09-19", Mode: NewsCacheOnly})
	if err != nil {
		t.Fatal(err)
	}
	if got.Fallback {
		t.Fatalf("topic 不应回溯交易日: %+v", got)
	}
	if len(got.Items) != 1 {
		t.Fatalf("应返回周六的榜单: %+v", got.Items)
	}
}

// 空数据时明确报告 insufficient_data，绝不编造榜单。
func TestHotTopicsInsufficientData(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc, _ := NewService(store, []Feed{}, testNames)

	got, err := svc.HotStockTopics(ctx, HotTopicsRequest{Date: "2026-09-17", Mode: NewsCacheOnly})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "insufficient_data" || len(got.Items) != 0 {
		t.Fatalf("空库应报 insufficient_data: %+v", got)
	}
	if got.Items == nil {
		t.Fatal("Items 应初始化为空数组而不是 null")
	}
}

// Top 限制榜单条数。
func TestHotTopicsTopLimit(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.Local)
	for i, code := range []string{"600519", "000001", "000002", "000004"} {
		seedTopicNews(t, ctx, store, "n"+code, "新闻"+code, code, MatchNative, 1.0, base.Add(time.Duration(i)*time.Hour), 50)
	}
	svc, _ := NewService(store, []Feed{}, testNames)
	got, err := svc.HotStockTopics(ctx, HotTopicsRequest{Date: "2026-09-17", Mode: NewsCacheOnly, Top: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Top=2 应只返回 2 条，实际 %d", len(got.Items))
	}
}
