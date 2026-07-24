package sources

import (
	"context"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// CaiLianSheSource 财联社数据源
type CaiLianSheSource struct {
	baseURL string
}

// NewCaiLianSheSource 创建财联社数据源
func NewCaiLianSheSource() *CaiLianSheSource {
	return &CaiLianSheSource{
		baseURL: "https://www.cls.cn",
	}
}

// Name 返回数据源名称
func (s *CaiLianSheSource) Name() newsfeed.SourceType {
	return newsfeed.SourceCaiLianShe
}

// RefreshInterval 返回建议的刷新间隔
func (s *CaiLianSheSource) RefreshInterval() time.Duration {
	return 30 * time.Second
}

// HealthCheck 检查数据源健康状态
func (s *CaiLianSheSource) HealthCheck(ctx context.Context) bool {
	_, err := httpGet(ctx, s.baseURL+"/api/sw?app=CailianpressWeb&os=web&sv=7.7.5", nil)
	return err == nil
}

// Fetch 获取最新新闻列表
func (s *CaiLianSheSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	url := s.baseURL + "/api/sw?app=CailianpressWeb&os=web&sv=7.7.5"
	data, err := httpGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}

	var result caiLianSheResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	return s.parseNewsItems(result.Data.List), nil
}

// FetchByStock 获取指定股票相关的新闻
func (s *CaiLianSheSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	url := fmt.Sprintf("%s/api/sw?app=CailianpressWeb&os=web&sv=7.7.5&keyword=%s", s.baseURL, code)
	data, err := httpGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}

	var result caiLianSheResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	return s.parseNewsItems(result.Data.List), nil
}

// FetchByKeyword 获取指定关键词相关的新闻
func (s *CaiLianSheSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	url := fmt.Sprintf("%s/api/sw?app=CailianpressWeb&os=web&sv=7.7.5&keyword=%s", s.baseURL, keyword)
	data, err := httpGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}

	var result caiLianSheResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	return s.parseNewsItems(result.Data.List), nil
}

// parseNewsItems 解析新闻列表
func (s *CaiLianSheSource) parseNewsItems(items []caiLianSheItem) []*newsfeed.NewsItem {
	var news []*newsfeed.NewsItem
	for _, item := range items {
		newsItem := &newsfeed.NewsItem{
			ID:          generateID(),
			Source:      newsfeed.SourceCaiLianShe,
			NewsType:    newsfeed.NewsTypeFlash,
			Title:       cleanText(item.Title),
			Summary:     cleanText(item.Summary),
			Content:     cleanText(item.Content),
			PublishTime: parseTime(item.PublishTime, []string{"2006-01-02 15:04:05"}),
			HotScore:    item.HotScore,
			Tags:        item.Tags,
			RelatedStocks: extractStockCodes(item.Title + item.Summary),
			URL:         s.baseURL + item.URL,
			OriginalID:  fmt.Sprintf("cls_%d", item.ID),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		news = append(news, newsItem)
	}
	return news
}

// caiLianSheResponse 财联社API响应结构
type caiLianSheResponse struct {
	Code int              `json:"code"`
	Msg  string           `json:"msg"`
	Data caiLianSheData   `json:"data"`
}

type caiLianSheData struct {
	List []caiLianSheItem `json:"list"`
}

type caiLianSheItem struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Content     string   `json:"content"`
	PublishTime string   `json:"publish_time"`
	HotScore    int      `json:"hot_score"`
	Tags        []string `json:"tags"`
	URL         string   `json:"url"`
}

// generateID 生成唯一ID
func generateID() string {
	return fmt.Sprintf("news_%d_%d", time.Now().UnixNano(), time.Now().Unix())
}
