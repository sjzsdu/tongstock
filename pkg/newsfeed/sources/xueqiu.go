package sources

import (
	"context"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// XueQiuSource 雪球数据源
type XueQiuSource struct {
	baseURL string
}

// NewXueQiuSource 创建雪球数据源
func NewXueQiuSource() *XueQiuSource {
	return &XueQiuSource{
		baseURL: "https://xueqiu.com",
	}
}

// Name 返回数据源名称
func (s *XueQiuSource) Name() newsfeed.SourceType {
	return newsfeed.SourceXueQiu
}

// RefreshInterval 返回建议的刷新间隔
func (s *XueQiuSource) RefreshInterval() time.Duration {
	return 5 * time.Minute
}

// HealthCheck 检查数据源健康状态
func (s *XueQiuSource) HealthCheck(ctx context.Context) bool {
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
		"Cookie":     "xq_a_token=", // 雪球API需要登录Cookie，这里留空仅做健康检查
	}
	_, err := httpGet(ctx, s.baseURL, headers)
	return err == nil
}

// Fetch 获取最新新闻列表
func (s *XueQiuSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	url := "https://xueqiu.com/statuses/public_timeline_by_category.json?since_id=-1&max_id=-1&count=15&category=6"
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
		"Cookie":     "xq_a_token=", // 需要登录后获取有效的Cookie
	}

	data, err := httpGet(ctx, url, headers)
	if err != nil {
		// 如果API请求失败，返回空列表（雪球需要登录）
		return []*newsfeed.NewsItem{}, nil
	}

	var result xueQiuResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	return s.parseNewsItems(result.Statuses), nil
}

// FetchByStock 获取指定股票相关的新闻
func (s *XueQiuSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	// 雪球股票代码格式：SH600519 或 SZ000001
	var fullCode string
	if len(code) == 6 {
		if code[0] == '6' {
			fullCode = "SH" + code
		} else {
			fullCode = "SZ" + code
		}
	} else {
		fullCode = code
	}

	url := fmt.Sprintf("https://xueqiu.com/stock/search.json?code=%s", fullCode)
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
	}

	data, err := httpGet(ctx, url, headers)
	if err != nil {
		return []*newsfeed.NewsItem{}, nil
	}

	var result xueQiuSearchResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	// 获取股票ID后获取相关讨论
	if len(result.Data) > 0 {
		return s.fetchStockStatuses(ctx, result.Data[0].ID)
	}

	return []*newsfeed.NewsItem{}, nil
}

// fetchStockStatuses 获取股票相关讨论
func (s *XueQiuSource) fetchStockStatuses(ctx context.Context, stockID string) ([]*newsfeed.NewsItem, error) {
	url := fmt.Sprintf("https://xueqiu.com/statuses/stock_timeline.json?symbol=%s&count=15", stockID)
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
		"Cookie":     "xq_a_token=",
	}

	data, err := httpGet(ctx, url, headers)
	if err != nil {
		return []*newsfeed.NewsItem{}, nil
	}

	var result xueQiuResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	return s.parseNewsItems(result.Statuses), nil
}

// FetchByKeyword 获取指定关键词相关的新闻
func (s *XueQiuSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	url := fmt.Sprintf("https://xueqiu.com/statuses/search.json?q=%s&count=15", keyword)
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
		"Cookie":     "xq_a_token=",
	}

	data, err := httpGet(ctx, url, headers)
	if err != nil {
		return []*newsfeed.NewsItem{}, nil
	}

	var result xueQiuResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	return s.parseNewsItems(result.Statuses), nil
}

// parseNewsItems 解析新闻列表
func (s *XueQiuSource) parseNewsItems(items []xueQiuStatus) []*newsfeed.NewsItem {
	var news []*newsfeed.NewsItem
	for _, item := range items {
		newsItem := &newsfeed.NewsItem{
			ID:          generateID(),
			Source:      newsfeed.SourceXueQiu,
			NewsType:    newsfeed.NewsTypeDiscussion,
			Title:       cleanText(item.Title),
			Summary:     cleanText(item.Text),
			Content:     cleanText(item.Text),
			PublishTime: parseTime(item.CreateAt, []string{"2006-01-02T15:04:05.000Z"}),
			HotScore:    item.ReplyCount + item.LikeCount,
			Tags:        extractTags(item.Text),
			RelatedStocks: extractStockCodes(item.Text),
			URL:         s.baseURL + item.Target,
			OriginalID:  fmt.Sprintf("xq_%d", item.ID),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		news = append(news, newsItem)
	}
	return news
}

// extractTags 从文本中提取标签
func extractTags(text string) []string {
	var tags []string
	return tags
}

// xueQiuResponse 雪球API响应结构
type xueQiuResponse struct {
	Statuses []xueQiuStatus `json:"list"`
}

type xueQiuStatus struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	Text       string `json:"text"`
	Target     string `json:"target"`
	CreateAt   string `json:"created_at"`
	ReplyCount int    `json:"reply_count"`
	LikeCount  int    `json:"like_count"`
}

type xueQiuSearchResponse struct {
	Data []xueQiuSearchItem `json:"data"`
}

type xueQiuSearchItem struct {
	ID string `json:"id"`
}
