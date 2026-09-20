package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// SecuritiesTimesSource 证券时报数据源。
//
// 证券时报是人民日报社主管主办的全国性财经证券类日报，是中国证监会指定
// 信息披露平台。提供7×24小时财经资讯，包括快讯、股市新闻、研报等。
//
// 使用移动端 API 接口获取快讯和新闻数据。
type SecuritiesTimesSource struct {
	baseURL  string
	location *time.Location
}

// NewSecuritiesTimesSource 创建证券时报数据源
func NewSecuritiesTimesSource() *SecuritiesTimesSource {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return &SecuritiesTimesSource{
		baseURL:  "https://m.stcn.com",
		location: loc,
	}
}

// Name 返回数据源名称
func (s *SecuritiesTimesSource) Name() newsfeed.SourceType {
	return newsfeed.SourceSecuritiesTime
}

// RefreshInterval 返回建议的刷新间隔
func (s *SecuritiesTimesSource) RefreshInterval() time.Duration {
	return 5 * time.Minute
}

// HealthCheck 检查数据源健康状态
func (s *SecuritiesTimesSource) HealthCheck(ctx context.Context) bool {
	_, err := s.fetchNewsflash(ctx, 1)
	return err == nil
}

// Fetch 获取最新快讯列表
func (s *SecuritiesTimesSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	items, err := s.fetchNewsflash(ctx, 30)
	if err != nil {
		return nil, err
	}
	return s.parse(items), nil
}

// FetchByStock 获取指定股票相关的新闻
func (s *SecuritiesTimesSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return nil, nil
	}

	// 证券时报没有直接按股票查询的接口，只能从快讯流中过滤
	items, err := s.fetchNewsflash(ctx, 50)
	if err != nil {
		return nil, err
	}

	var matched []*newsfeed.NewsItem
	for _, it := range s.parse(items) {
		if strings.Contains(it.Title, code) || strings.Contains(it.Content, code) {
			matched = append(matched, it)
		}
	}
	return matched, nil
}

// FetchByKeyword 按关键词检索新闻
func (s *SecuritiesTimesSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}

	items, err := s.fetchNewsflash(ctx, 50)
	if err != nil {
		return nil, err
	}

	var matched []*newsfeed.NewsItem
	for _, it := range s.parse(items) {
		if strings.Contains(it.Title, keyword) || strings.Contains(it.Content, keyword) {
			matched = append(matched, it)
		}
	}
	return matched, nil
}

// fetchNewsflash 获取快讯列表
func (s *SecuritiesTimesSource) fetchNewsflash(ctx context.Context, limit int) ([]stcnItem, error) {
	if limit <= 0 {
		limit = 20
	}

	// 使用移动端快讯接口
	req := fmt.Sprintf("%s/nodeapi/newsflash/getList?app=CailianpressWap&os=web&sv=8.4.6&rn=%d", s.baseURL, limit)
	data, err := httpGet(ctx, req, map[string]string{
		"Referer":    "https://m.stcn.com/",
		"User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148",
	})
	if err != nil {
		return nil, err
	}

	var resp stcnResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("证券时报响应解析失败: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("证券时报返回错误: %s", resp.Message)
	}

	return resp.Data.List, nil
}

// parse 解析快讯数据
func (s *SecuritiesTimesSource) parse(items []stcnItem) []*newsfeed.NewsItem {
	now := time.Now()
	out := make([]*newsfeed.NewsItem, 0, len(items))

	for _, it := range items {
		if it.ID == 0 {
			continue
		}

		title := newsfeed.StripHTML(it.Title)
		content := newsfeed.StripHTML(it.Content)
		summary := content
		if r := []rune(summary); len(r) > 200 {
			summary = string(r[:200])
		}

		newsItem := &newsfeed.NewsItem{
			Source:      newsfeed.SourceSecuritiesTime,
			NewsType:    newsfeed.NewsTypeFlash,
			Title:       title,
			Summary:     summary,
			Content:     content,
			PublishTime: s.parseTime(it.CreatedAt),
			HotScore:    it.hotScore(),
			Tags:        []string{"证券时报"},
			URL:         it.URL,
			OriginalID:  fmt.Sprintf("stcn_%d", it.ID),
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if newsItem.URL == "" {
			newsItem.URL = fmt.Sprintf("https://www.stcn.com/article/detail/%d.html", it.ID)
		}

		// 提取股票代码
		newsItem.RelatedStocks = extractStockCodes(title + " " + content)
		if len(newsItem.RelatedStocks) > 0 {
			for _, code := range newsItem.RelatedStocks {
				newsItem.StockRefs = append(newsItem.StockRefs, newsfeed.StockRef{
					Code:       code,
					MatchType:  newsfeed.MatchCodeHit,
					Confidence: 0.7,
				})
			}
		}

		out = append(out, newsItem)
	}
	return out
}

// parseTime 解析时间字符串
func (s *SecuritiesTimesSource) parseTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Now()
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, v, s.location); err == nil {
			return t
		}
	}
	return time.Now()
}

// hotScore 计算热度分数
func (it stcnItem) hotScore() int {
	score := it.ReadingNum/100 + it.CommentNum*2 + it.ShareNum/50
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score
}

// stcnResponse 证券时报响应结构
type stcnResponse struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    stcnData `json:"data"`
}

type stcnData struct {
	List []stcnItem `json:"list"`
}

// stcnItem 证券时报快讯条目
type stcnItem struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	CreatedAt   string `json:"created_at"`
	URL         string `json:"url"`
	ReadingNum  int    `json:"reading_num"`
	CommentNum  int    `json:"comment_num"`
	ShareNum    int    `json:"share_num"`
	ContentType string `json:"content_type"`
}
