package sources

import (
	"bytes"
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
// 使用移动端 API 接口获取新闻数据。旧的全局列表接口已改为按
// symbol 查询：不带 symbol 会返回「股票代码为空」，因此全局流
// 改用上证指数关联的市场资讯，个股则用市场前缀 + 代码精确查询。
type TencentFinanceSource struct {
	baseURL  string
	location *time.Location
}

const (
	// qqIndexSymbol 是上证指数的 symbol，用它拉取市场/宏观新闻充当全局流。
	qqIndexSymbol = "sh000001"
	// search 接口的 type 参数：0 为公告，2 为新闻资讯。
	qqTypeNotice = 0
	qqTypeNews   = 2
)

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
	_, err := s.fetchNews(ctx, qqIndexSymbol, qqTypeNews, 1)
	return err == nil
}

// Fetch 获取最新新闻列表。腾讯不再提供全市场聚合接口，这里取上证指数
// 关联的市场/宏观新闻作为全局流。
func (s *TencentFinanceSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	items, err := s.fetchNews(ctx, qqIndexSymbol, qqTypeNews, 30)
	if err != nil {
		return nil, err
	}
	return s.parse(items, newsfeed.NewsTypeOther, ""), nil
}

// FetchByStock 获取指定股票相关的新闻与公告。
func (s *TencentFinanceSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	code = strings.TrimSpace(code)
	symbol := tencentSymbol(code)
	if symbol == "" {
		return nil, nil
	}

	// 新闻与公告分属两个 type，各拉一次；公告失败不影响新闻结果。
	news, err := s.fetchNews(ctx, symbol, qqTypeNews, 50)
	if err != nil {
		return nil, err
	}
	notices, err := s.fetchNews(ctx, symbol, qqTypeNotice, 30)
	if err != nil {
		notices = nil
	}

	out := s.parse(news, newsfeed.NewsTypeOther, code)
	out = append(out, s.parse(notices, newsfeed.NewsTypeAnnouncement, code)...)
	return out, nil
}

// FetchByKeyword 按关键词检索新闻
func (s *TencentFinanceSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}

	items, err := s.fetchNews(ctx, qqIndexSymbol, qqTypeNews, 50)
	if err != nil {
		return nil, err
	}

	var matched []*newsfeed.NewsItem
	for _, it := range s.parse(items, newsfeed.NewsTypeOther, "") {
		if strings.Contains(it.Title, keyword) || strings.Contains(it.Content, keyword) {
			matched = append(matched, it)
		}
	}
	return matched, nil
}

// fetchNews 按 symbol 拉取资讯列表。type=0 公告、type=2 新闻。
func (s *TencentFinanceSource) fetchNews(ctx context.Context, symbol string, newsType, limit int) ([]qqItem, error) {
	if limit <= 0 {
		limit = 20
	}

	req := fmt.Sprintf("%s/ifzqgtimg/appstock/news/info/search?type=%d&page=1&n=%d&symbol=%s",
		s.baseURL, newsType, limit, symbol)
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

	if len(resp.Data.List) > limit {
		resp.Data.List = resp.Data.List[:limit]
	}
	return resp.Data.List, nil
}

// parse 解析新闻数据。nativeCode 非空时表示该列表就是按这只股票拉的，
// 关联按数据源原生标注处理（置信度 1.0）。
func (s *TencentFinanceSource) parse(items []qqItem, newsType newsfeed.NewsType, nativeCode string) []*newsfeed.NewsItem {
	now := time.Now()
	out := make([]*newsfeed.NewsItem, 0, len(items))

	for _, it := range items {
		if it.ID == "" {
			continue
		}

		title := newsfeed.StripHTML(it.Title)
		summary := newsfeed.StripHTML(it.Summary)
		if r := []rune(summary); len(r) > 200 {
			summary = string(r[:200])
		}

		newsItem := &newsfeed.NewsItem{
			Source:      newsfeed.SourceTencentFinance,
			NewsType:    newsType,
			Title:       title,
			Summary:     summary,
			Content:     summary,
			PublishTime: s.parseTime(it.Time),
			// 接口已不返回评论数，热度按无数据处理。
			HotScore:   0,
			Tags:       []string{"腾讯财经"},
			URL:        it.URL,
			OriginalID: "qq_" + it.ID,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if newsItem.URL == "" {
			newsItem.URL = fmt.Sprintf("https://stockapp.finance.qq.com/mstats/detail.html?id=%s", it.ID)
		}

		if nativeCode != "" {
			newsItem.RelatedStocks = append(newsItem.RelatedStocks, nativeCode)
			newsItem.StockRefs = append(newsItem.StockRefs, newsfeed.StockRef{
				Code:       nativeCode,
				MatchType:  newsfeed.MatchNative,
				Confidence: 1.0,
			})
		}

		// 正文里出现的其它代码按命中处理。
		for _, code := range extractStockCodes(title + " " + summary) {
			if containsString(newsItem.RelatedStocks, code) {
				continue
			}
			newsItem.RelatedStocks = append(newsItem.RelatedStocks, code)
			newsItem.StockRefs = append(newsItem.StockRefs, newsfeed.StockRef{
				Code:       code,
				MatchType:  newsfeed.MatchCodeHit,
				Confidence: 0.7,
			})
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
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, v, s.location); err == nil {
			return t
		}
	}
	return time.Now()
}

// tencentSymbol 把 6 位代码转成腾讯接口的 symbol（市场前缀 + 代码），
// 无法识别时返回空串。
func tencentSymbol(code string) string {
	if len(code) != 6 {
		return ""
	}
	switch {
	case strings.HasPrefix(code, "920"), strings.HasPrefix(code, "4"), strings.HasPrefix(code, "8"):
		return "bj" + code // 北交所
	case strings.HasPrefix(code, "6"), strings.HasPrefix(code, "9"):
		return "sh" + code // 沪市 / 沪B
	case strings.HasPrefix(code, "0"), strings.HasPrefix(code, "2"), strings.HasPrefix(code, "3"):
		return "sz" + code // 深市 / 深B
	}
	return ""
}

// containsString 判断切片是否包含指定字符串。
func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// qqResponse 腾讯财经响应结构
type qqResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data qqData `json:"data"`
}

// qqData 成功时是对象（data.data 为条目数组），失败时是空数组或字符串，
// 这里统一按对象解析，其余形状视作无条目。
type qqData struct {
	List []qqItem `json:"data"`
}

func (d *qqData) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || b[0] != '{' {
		d.List = nil
		return nil
	}
	type qqDataAlias qqData
	var alias qqDataAlias
	if err := json.Unmarshal(b, &alias); err != nil {
		return err
	}
	d.List = alias.List
	return nil
}

// qqItem 腾讯财经新闻条目
type qqItem struct {
	ID      string `json:"id"`
	Symbol  string `json:"symbol"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	// Time 形如 "2006-01-02 15:04:05"。
	Time    string   `json:"time"`
	URL     string   `json:"url"`
	Src     string   `json:"src"`
	TypeStr string   `json:"typeStr"`
	Symbols []string `json:"symbols"`
}
