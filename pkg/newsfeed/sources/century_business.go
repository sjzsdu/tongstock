package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// CenturyBusinessSource 21世纪经济报道数据源。
//
// 21世纪经济报道是中国领先的财经媒体，以深度财经报道和行业分析著称。
// 提供宏观经济、金融市场、产业经济等领域的专业资讯。
//
// 使用移动端 API 接口获取新闻数据。
type CenturyBusinessSource struct {
	baseURL  string
	location *time.Location
}

// NewCenturyBusinessSource 创建21世纪经济报道数据源
func NewCenturyBusinessSource() *CenturyBusinessSource {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return &CenturyBusinessSource{
		baseURL:  "https://api-api.21jingji.com",
		location: loc,
	}
}

// Name 返回数据源名称
func (s *CenturyBusinessSource) Name() newsfeed.SourceType {
	return newsfeed.Source21Business
}

// RefreshInterval 返回建议的刷新间隔
func (s *CenturyBusinessSource) RefreshInterval() time.Duration {
	return 5 * time.Minute
}

// HealthCheck 检查数据源健康状态
func (s *CenturyBusinessSource) HealthCheck(ctx context.Context) bool {
	_, err := s.fetchArticles(ctx, 1)
	return err == nil
}

// Fetch 获取最新新闻列表
func (s *CenturyBusinessSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	items, err := s.fetchArticles(ctx, 30)
	if err != nil {
		return nil, err
	}
	return s.parse(items), nil
}

// FetchByStock 获取指定股票相关的新闻
func (s *CenturyBusinessSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return nil, nil
	}

	// 21世纪经济报道没有直接按股票查询的接口，只能从新闻流中过滤
	items, err := s.fetchArticles(ctx, 50)
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
func (s *CenturyBusinessSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}

	items, err := s.fetchArticles(ctx, 50)
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

// fetchArticles 获取文章列表
func (s *CenturyBusinessSource) fetchArticles(ctx context.Context, limit int) ([]jingjiItem, error) {
	if limit <= 0 {
		limit = 20
	}

	// 使用移动端文章接口
	req := fmt.Sprintf("%s/api/article/list?app=CailianpressWap&os=web&sv=8.4.6&rn=%d", s.baseURL, limit)
	data, err := httpGet(ctx, req, map[string]string{
		"Referer":    "https://m.21jingji.com/",
		"User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148",
	})
	if err != nil {
		return nil, err
	}

	var resp jingjiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("21世纪经济报道响应解析失败: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("21世纪经济报道返回错误: %s", resp.Message)
	}

	return resp.Data.List, nil
}

// parse 解析文章数据
func (s *CenturyBusinessSource) parse(items []jingjiItem) []*newsfeed.NewsItem {
	now := time.Now()
	out := make([]*newsfeed.NewsItem, 0, len(items))

	for _, it := range items {
		if it.ID == "" {
			continue
		}

		title := newsfeed.StripHTML(it.Title)
		content := newsfeed.StripHTML(it.Content)
		summary := content
		if r := []rune(summary); len(r) > 200 {
			summary = string(r[:200])
		}

		newsItem := &newsfeed.NewsItem{
			Source:      newsfeed.Source21Business,
			NewsType:    newsfeed.NewsTypeOther,
			Title:       title,
			Summary:     summary,
			Content:     content,
			PublishTime: s.parseTime(it.PublishTime),
			HotScore:    it.hotScore(),
			Tags:        []string{"21世纪经济报道"},
			URL:         it.URL,
			OriginalID:  "21jingji_" + it.ID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if newsItem.URL == "" {
			newsItem.URL = fmt.Sprintf("https://www.21jingji.com/article/%s.html", it.ID)
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
func (s *CenturyBusinessSource) parseTime(v string) time.Time {
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
func (it jingjiItem) hotScore() int {
	score := it.ViewCount/1000 + it.CommentCount*2
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score
}

// jingjiResponse 21世纪经济报道响应结构
type jingjiResponse struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    jingjiData `json:"data"`
}

type jingjiData struct {
	List []jingjiItem `json:"list"`
}

// jingjiItem 21世纪经济报道文章条目
type jingjiItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	PublishTime  string `json:"publish_time"`
	URL          string `json:"url"`
	ViewCount    int    `json:"view_count"`
	CommentCount int    `json:"comment_count"`
	Category     string `json:"category"`
}
