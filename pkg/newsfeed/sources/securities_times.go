package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// SecuritiesTimesSource 证券时报数据源。
//
// 证券时报是人民日报社主管主办的全国性财经证券类日报，是中国证监会指定
// 信息披露平台。提供7×24小时财经资讯，包括快讯、股市新闻、研报等。
//
// 快讯数据走 H5 站点的网关接口 ewap.stcn.com/api/transform（旧的
// m.stcn.com/nodeapi 已下线，现在返回的是 HTML 页面）。
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
		baseURL:  "https://ewap.stcn.com",
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

	// 网关接口把具体列表名放在 path 参数里，其余入参序列化成 JSON
	// 字符串塞进 other_param。缺 post_times 会直接报参数缺失。
	other := `{"type":"1","page":1,"max_id":1,"first_id":-1,"post_times":1}`
	req := fmt.Sprintf("%s/api/transform?path=news-fast_info_list&other_param=%s", s.baseURL, url.QueryEscape(other))
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

	if resp.Status != 1 {
		return nil, fmt.Errorf("证券时报返回错误: %s", resp.Msg)
	}

	if len(resp.Data) > limit {
		resp.Data = resp.Data[:limit]
	}
	return resp.Data, nil
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
			PublishTime: time.Unix(it.Time, 0).In(s.location),
			HotScore:    it.hotScore(),
			Tags:        []string{"证券时报"},
			URL:         it.Share.ShareURL,
			OriginalID:  fmt.Sprintf("stcn_%d", it.ID),
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if newsItem.URL == "" {
			newsItem.URL = fmt.Sprintf("https://h5.stcn.com/pages/detail/detail?id=%d&jump_type=fast_info", it.ID)
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

// hotScore 接口不再回阅读量，只保留「标红快讯」这一重要性信号。
func (it stcnItem) hotScore() int {
	if it.IsRed != 0 {
		return 60
	}
	return 0
}

// stcnResponse 证券时报快讯网关响应结构
type stcnResponse struct {
	Status int        `json:"status"`
	Msg    string     `json:"msg"`
	Data   []stcnItem `json:"data"`
}

// stcnItem 证券时报快讯条目
type stcnItem struct {
	ID      int64  `json:"item_id"`
	Title   string `json:"wap_title"`
	Content string `json:"wap_content"`
	// Time 是秒级 Unix 时间戳。
	Time  int64     `json:"time"`
	IsRed int       `json:"is_red"`
	Share stcnShare `json:"share"`
}

// stcnShare 分享信息，内含快讯详情页地址。
type stcnShare struct {
	ShareURL string `json:"share_url"`
}
