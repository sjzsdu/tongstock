package newsfeed

import (
	"testing"
	"time"
)

func digestItem(source SourceType, newsType NewsType, title string, ts time.Time) NewsSummary {
	return NewsSummary{
		Source:      source,
		NewsType:    newsType,
		Title:       title,
		PublishTime: ts,
	}
}

func TestBuildNewsDigestEmpty(t *testing.T) {
	d := BuildNewsDigest(nil)
	if d.Total != 0 || len(d.Groups) != 0 {
		t.Fatalf("nil input should produce empty digest, got %+v", d)
	}

	d = BuildNewsDigest(&StockNewsResult{Code: "001216", Status: "insufficient_data"})
	if d.Total != 0 || d.Code != "001216" || d.Status != "insufficient_data" {
		t.Fatalf("empty result mismatch: %+v", d)
	}
}

func TestBuildNewsDigestCountsAndOrder(t *testing.T) {
	base := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	res := &StockNewsResult{
		Code:   "001216",
		Status: "ok",
		Items: []NewsSummary{
			digestItem(SourceEastMoney, NewsTypeOther, "磷化工景气度回升", base),
			digestItem(SourceCaiLianShe, NewsTypeFlash, "磷肥出口政策调整", base.Add(2*time.Hour)),
			digestItem(SourceEastMoney, NewsTypeReport, "磷化工行业研报", base.Add(time.Hour)),
		},
	}
	d := BuildNewsDigest(res)

	if d.Total != 3 {
		t.Fatalf("total = %d, want 3", d.Total)
	}
	// 时间倒序
	if !d.Items[0].PublishTime.After(d.Items[1].PublishTime) {
		t.Fatalf("items not sorted desc: %v", d.Items)
	}
	if d.SourceCounts["东方财富"] != 2 || d.SourceCounts["财联社"] != 1 {
		t.Fatalf("source counts mismatch: %v", d.SourceCounts)
	}
	if d.TypeCounts["研报"] != 1 || d.TypeCounts["快讯"] != 1 {
		t.Fatalf("type counts mismatch: %v", d.TypeCounts)
	}
	if !d.From.Equal(base) {
		t.Fatalf("from = %v, want %v", d.From, base)
	}
	if !d.To.Equal(base.Add(2 * time.Hour)) {
		t.Fatalf("to = %v, want %v", d.To, base.Add(2*time.Hour))
	}
}

func TestBuildNewsDigestCrossSourceGrouping(t *testing.T) {
	base := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	res := &StockNewsResult{
		Code:   "001216",
		Status: "ok",
		Items: []NewsSummary{
			digestItem(SourceCaiLianShe, NewsTypeFlash, "川发龙蟒：拟收购磷矿资产", base),
			digestItem(SourceEastMoney, NewsTypeOther, "川发龙蟒拟收购磷矿资产 加码上游布局", base.Add(30*time.Minute)),
			digestItem(SourceXueQiu, NewsTypeDiscussion, "磷化工板块大涨", base.Add(time.Hour)),
		},
	}
	d := BuildNewsDigest(res)

	if len(d.Groups) != 1 {
		t.Fatalf("groups = %d, want 1: %+v", len(d.Groups), d.Groups)
	}
	g := d.Groups[0]
	if g.Count != 2 {
		t.Fatalf("group count = %d, want 2", g.Count)
	}
	if len(g.Sources) != 2 {
		t.Fatalf("group sources = %v, want 2 sources", g.Sources)
	}
	// 代表标题应取组内最长的一条
	if g.Title != "川发龙蟒拟收购磷矿资产 加码上游布局" {
		t.Fatalf("group title = %q", g.Title)
	}
	if g.Items[0].PublishTime.Before(g.Items[1].PublishTime) {
		t.Fatalf("group items not sorted desc")
	}
}

func TestDigestSimilar(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"央行降准05个百分点", "央行降准0.5个百分点", true},
		{"宁德时代发布新电池", "宁德时代发布新电池，续航突破1000公里", true},
		{"完全不同的标题内容", "另一个毫不相关的事件报道", false},
		{"", "随便什么", false},
	}
	for _, c := range cases {
		got := digestSimilar(normalizeTitle(c.a), normalizeTitle(c.b))
		if got != c.want {
			t.Errorf("digestSimilar(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	got := normalizeTitle("重磅！央行宣布 降准0.5个百分点.")
	want := "重磅央行宣布降准05个百分点"
	if got != want {
		t.Fatalf("normalizeTitle = %q, want %q", got, want)
	}
}
