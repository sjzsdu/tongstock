package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// SinaFinanceSource 新浪财经数据源。
//
// 新浪财经是中国最大的财经门户网站之一，提供7×24小时财经资讯、
// 股票行情、基金净值、期货外汇等数据。覆盖宏观经济、金融市场、
// 产业经济等领域。
//
// 使用移动端 API 接口获取新闻数据。
type SinaFinanceSource struct {
	baseURL  string
	location *time.Location
}

// NewSinaFinanceSource 创建新浪财经数据源
func NewSinaFinanceSource() *SinaFinanceSource {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return &SinaFinanceSource{
		baseURL:  "https://zhibo.sina.com.cn",
		location: loc,
	}
}

// Name 返回数据源名称
func (s *SinaFinanceSource) Name() newsfeed.SourceType {
	return newsfeed.SourceSinaFinance
}

// RefreshInterval 返回建议的刷新间隔
func (s *SinaFinanceSource) RefreshInterval() time.Duration {
	return 3 * time.Minute
}

// HealthCheck 检查数据源健康状态
func (s *SinaFinanceSource) HealthCheck(ctx context.Context) bool {
	_, err := s.fetchRollNews(ctx, 1)
	return err == nil
}

// Fetch 获取最新新闻列表
func (s *SinaFinanceSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	items, err := s.fetchRollNews(ctx, 30)
	if err != nil {
		return nil, err
	}
	return s.parse(items), nil
}

// FetchByStock 获取指定股票相关的新闻
func (s *SinaFinanceSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return nil, nil
	}

	// 新浪财经没有直接按股票查询的接口，只能从滚动新闻流中过滤
	items, err := s.fetchRollNews(ctx, 50)
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
func (s *SinaFinanceSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}

	items, err := s.fetchRollNews(ctx, 50)
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

// fetchRollNews 获取滚动新闻列表
func (s *SinaFinanceSource) fetchRollNews(ctx context.Context, limit int) ([]sinaItem, error) {
	if limit <= 0 {
		limit = 20
	}

	// 使用新浪财经滚动新闻接口
	req := fmt.Sprintf("https://feed.mix.sina.com.cn/api/roll/get?pageid=153&lid=2516&k=&num=%d&page=1&r=0.1", limit)
	data, err := httpGet(ctx, req, map[string]string{
		"Referer":    "https://finance.sina.com.cn/",
		"User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148",
	})
	if err != nil {
		return nil, err
	}

	var resp sinaResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("新浪财经响应解析失败: %w", err)
	}

	if resp.Result.Status.Code != 0 {
		return nil, fmt.Errorf("新浪财经返回错误: %s", resp.Result.Status.Msg)
	}

	return resp.Result.Data, nil
}

// parse 解析新闻数据
func (s *SinaFinanceSource) parse(items []sinaItem) []*newsfeed.NewsItem {
	now := time.Now()
	out := make([]*newsfeed.NewsItem, 0, len(items))

	for _, it := range items {
		if it.ID == "" {
			continue
		}

		title := newsfeed.StripHTML(it.Title)
		intro := newsfeed.StripHTML(it.Intro)
		summary := intro
		if r := []rune(summary); len(r) > 200 {
			summary = string(r[:200])
		}

		newsItem := &newsfeed.NewsItem{
			Source:      newsfeed.SourceSinaFinance,
			NewsType:    newsfeed.NewsTypeOther,
			Title:       title,
			Summary:     summary,
			Content:     intro,
			PublishTime: s.parseTime(it.CTime),
			HotScore:    it.hotScore(),
			Tags:        []string{"新浪财经"},
			URL:         it.URL,
			OriginalID:  "sina_" + it.ID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if newsItem.URL == "" {
			newsItem.URL = fmt.Sprintf("https://finance.sina.com.cn/%s/%s.shtml", it.Category, it.ID)
		}

		// 提取股票代码
		newsItem.RelatedStocks = extractStockCodes(title + " " + intro)
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
func (s *SinaFinanceSource) parseTime(v string) time.Time {
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
func (it sinaItem) hotScore() int {
	score := it.Comments * 2
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score
}

// sinaResponse 新浪财经响应结构
type sinaResponse struct {
	Result sinaResult `json:"result"`
}

type sinaResult struct {
	Status sinaStatus `json:"status"`
	Data   []sinaItem `json:"data"`
}

type sinaStatus struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// sinaItem 新浪财经滚动新闻条目
type sinaItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Intro     string `json:"intro"`
	CTime     string `json:"ctime"`
	URL       string `json:"url"`
	Category  string `json:"category"`
	MediaName string `json:"media_name"`
	Comments  int    `json:"comments"`
}
