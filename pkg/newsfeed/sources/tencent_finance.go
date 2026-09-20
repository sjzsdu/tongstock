package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// TencentFinanceSource 腾讯财经数据源。
//
// 腾讯财经是中国领先的财经资讯平台，提供7×24小时财经资讯、
// 股票行情、基金净值、期货外汇等数据。覆盖宏观经济、金融市场、
// 产业经济等领域。
//
// 使用移动端 API 接口获取新闻数据。
type TencentFinanceSource struct {
	baseURL  string
	location *time.Location
}

// NewTencentFinanceSource 创建腾讯财经数据源
func NewTencentFinanceSource() *TencentFinanceSource {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return &TencentFinanceSource{
		baseURL:  "https://proxy.finance.qq.com",
		location: loc,
	}
}

// Name 返回数据源名称
func (s *TencentFinanceSource) Name() newsfeed.SourceType {
	return newsfeed.SourceTencentFinance
}

// RefreshInterval 返回建议的刷新间隔
func (s *TencentFinanceSource) RefreshInterval() time.Duration {
	return 3 * time.Minute
}

// HealthCheck 检查数据源健康状态
func (s *TencentFinanceSource) HealthCheck(ctx context.Context) bool {
	_, err := s.fetchNewsList(ctx, 1)
	return err == nil
}

// Fetch 获取最新新闻列表
func (s *TencentFinanceSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	items, err := s.fetchNewsList(ctx, 30)
	if err != nil {
		return nil, err
	}
	return s.parse(items), nil
}

// FetchByStock 获取指定股票相关的新闻
func (s *TencentFinanceSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return nil, nil
	}

	// 腾讯财经没有直接按股票查询的接口，只能从新闻流中过滤
	items, err := s.fetchNewsList(ctx, 50)
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
func (s *TencentFinanceSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}

	items, err := s.fetchNewsList(ctx, 50)
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

// fetchNewsList 获取新闻列表
func (s *TencentFinanceSource) fetchNewsList(ctx context.Context, limit int) ([]qqItem, error) {
	if limit <= 0 {
		limit = 20
	}

	// 使用腾讯财经新闻列表接口
	req := fmt.Sprintf("https://proxy.finance.qq.com/ifzqgtimg/appstock/news/info/search?type=0&page=0&perpage=%d", limit)
	data, err := httpGet(ctx, req, map[string]string{
		"Referer":    "https://stockapp.finance.qq.com/",
		"User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148",
	})
	if err != nil {
		return nil, err
	}

	var resp qqResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("腾讯财经响应解析失败: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("腾讯财经返回错误: %s", resp.Msg)
	}

	return resp.Data.List, nil
}

// parse 解析新闻数据
func (s *TencentFinanceSource) parse(items []qqItem) []*newsfeed.NewsItem {
	now := time.Now()
	out := make([]*newsfeed.NewsItem, 0, len(items))

	for _, it := range items {
		if it.ID == "" {
			continue
		}

		title := newsfeed.StripHTML(it.Title)
		summary := newsfeed.StripHTML(it.Abstract)
		if r := []rune(summary); len(r) > 200 {
			summary = string(r[:200])
		}

		newsItem := &newsfeed.NewsItem{
			Source:      newsfeed.SourceTencentFinance,
			NewsType:    newsfeed.NewsTypeOther,
			Title:       title,
			Summary:     summary,
			Content:     summary,
			PublishTime: s.parseTime(it.PublishTime),
			HotScore:    it.hotScore(),
			Tags:        []string{"腾讯财经"},
			URL:         it.URL,
			OriginalID:  "qq_" + it.ID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if newsItem.URL == "" {
			newsItem.URL = fmt.Sprintf("https://stockapp.finance.qq.com/mstats/detail.html?id=%s", it.ID)
		}

		// 提取股票代码
		newsItem.RelatedStocks = extractStockCodes(title + " " + summary)
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
func (s *TencentFinanceSource) parseTime(v string) time.Time {
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
func (it qqItem) hotScore() int {
	score := it.Comments * 2
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score
}

// qqResponse 腾讯财经响应结构
type qqResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data qqData `json:"data"`
}

type qqData struct {
	List []qqItem `json:"list"`
}

// qqItem 腾讯财经新闻条目
type qqItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Abstract    string `json:"abstract"`
	PublishTime string `json:"pub_time"`
	URL         string `json:"url"`
	Category    string `json:"category"`
	Source      string `json:"source"`
	Comments    int    `json:"comments"`
}
