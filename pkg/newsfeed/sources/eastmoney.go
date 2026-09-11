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

// EastMoneySource 东方财富资讯数据源。
//
// 东财的检索接口是全文检索，按股票搜索会命中大量「正文里恰好提到这只股票」
// 的文章（例如各种「百元股榜单」）。因此这里对每条结果计算相关度：
// 标题命中视为「关于这只股票」，仅正文命中视为「提及」，调用方可以据此过滤。
type EastMoneySource struct {
	baseURL  string
	location *time.Location
	// NameResolver 把股票代码解析为简称。为 nil 时退化为按代码检索，
	// 召回质量会下降（新闻正文极少直接写 6 位代码）。
	NameResolver func(code string) string
}

// NewEastMoneySource 创建东方财富数据源。
func NewEastMoneySource() *EastMoneySource {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return &EastMoneySource{
		baseURL:  "https://search-api-web.eastmoney.com",
		location: loc,
	}
}

// SetNameResolver 注入「代码 → 简称」解析器。
func (s *EastMoneySource) SetNameResolver(fn func(code string) string) {
	s.NameResolver = fn
}

// Name 返回数据源名称
func (s *EastMoneySource) Name() newsfeed.SourceType { return newsfeed.SourceEastMoney }

// RefreshInterval 返回建议的刷新间隔
func (s *EastMoneySource) RefreshInterval() time.Duration { return 5 * time.Minute }

// HealthCheck 检查数据源健康状态
func (s *EastMoneySource) HealthCheck(ctx context.Context) bool {
	_, err := httpGet(ctx, s.baseURL+"/search/jsonp?cb=jQuery&param=%7B%7D", map[string]string{
		"Referer": "https://so.eastmoney.com/",
	})
	return err == nil
}

// Fetch 东财检索接口必须带关键词，没有「全局最新」的概念。
// 这里明确返回空而不是伪造一个全局流。
func (s *EastMoneySource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	return nil, nil
}

// FetchByStock 获取指定股票相关的新闻与研报。
func (s *EastMoneySource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	keyword := code
	if s.NameResolver != nil {
		if name := strings.TrimSpace(s.NameResolver(code)); name != "" {
			keyword = name
		}
	}
	news, err := s.search(ctx, keyword, code, "cmsArticleWebOld")
	if err != nil {
		return nil, err
	}
	reports, err := s.search(ctx, keyword, code, "researchReport")
	if err != nil {
		// 研报失败不应拖垮新闻，降级返回已有结果。
		return news, nil
	}
	return append(news, reports...), nil
}

// FetchByKeyword 按关键词检索新闻。
func (s *EastMoneySource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	return s.search(ctx, keyword, "", "cmsArticleWebOld")
}

func (s *EastMoneySource) search(ctx context.Context, keyword, code, resultType string) ([]*newsfeed.NewsItem, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	param := fmt.Sprintf(
		`{"uid":"","keyword":%q,"type":[%q],"client":"web","clientType":"web","clientVersion":"curr","param":{%q:{"searchScope":"default","sort":"time","pageIndex":1,"pageSize":30,"preTag":"<em>","postTag":"</em>"}}}`,
		keyword, resultType, resultType,
	)
	req := s.baseURL + "/search/jsonp?cb=jQuery&param=" + url.QueryEscape(param)

	data, err := httpGet(ctx, req, map[string]string{"Referer": "https://so.eastmoney.com/"})
	if err != nil {
		return nil, err
	}
	payload, err := unwrapJSONP(string(data))
	if err != nil {
		return nil, err
	}

	var resp eastMoneyResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		return nil, fmt.Errorf("东财响应解析失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("东财返回错误: %s", resp.Msg)
	}

	switch resultType {
	case "researchReport":
		return s.parseReports(resp.Result.ResearchReport, keyword, code), nil
	default:
		return s.parseArticles(resp.Result.Articles, keyword, code), nil
	}
}

// relevance 判定文章与目标的关联强度。标题命中才算「关于」，
// 仅正文命中（榜单、统计类文章）只算「提及」。
func (s *EastMoneySource) relevance(title, body, keyword, code string) float64 {
	hay := title
	if strings.Contains(hay, keyword) {
		return 0.95
	}
	if code != "" && strings.Contains(hay, code) {
		return 0.9
	}
	if strings.Contains(body, keyword) {
		return 0.4
	}
	if code != "" && strings.Contains(body, code) {
		return 0.35
	}
	return 0.2
}

func (s *EastMoneySource) parseArticles(items []eastMoneyArticle, keyword, code string) []*newsfeed.NewsItem {
	now := time.Now()
	out := make([]*newsfeed.NewsItem, 0, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.Code)
		if id == "" {
			continue
		}
		title := newsfeed.StripHTML(it.Title)
		body := newsfeed.StripHTML(it.Content)
		summary := body
		if len([]rune(summary)) > 200 {
			summary = string([]rune(summary)[:200])
		}
		item := &newsfeed.NewsItem{
			ID:          "",
			Source:      newsfeed.SourceEastMoney,
			NewsType:    newsfeed.NewsTypeOther,
			Title:       title,
			Summary:     summary,
			Content:     body,
			PublishTime: s.parseTime(it.Date),
			Tags:        []string{},
			URL:         it.URL,
			OriginalID:  "em_news_" + id,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if it.MediaName != "" {
			item.Tags = append(item.Tags, it.MediaName)
		}
		if code != "" {
			item.StockRefs = []newsfeed.StockRef{{
				Code:       code,
				MatchType:  newsfeed.MatchNameHit,
				Confidence: s.relevance(title, body, keyword, code),
			}}
			item.RelatedStocks = []string{code}
		}
		out = append(out, item)
	}
	return out
}

func (s *EastMoneySource) parseReports(items []eastMoneyReport, keyword, code string) []*newsfeed.NewsItem {
	now := time.Now()
	out := make([]*newsfeed.NewsItem, 0, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.Code)
		if id == "" {
			continue
		}
		title := newsfeed.StripHTML(it.Title)
		item := &newsfeed.NewsItem{
			Source:      newsfeed.SourceEastMoney,
			NewsType:    newsfeed.NewsTypeReport,
			Title:       title,
			Summary:     newsfeed.StripHTML(it.Content),
			Content:     newsfeed.StripHTML(it.Content),
			PublishTime: s.parseTime(it.Date),
			Tags:        []string{},
			URL:         "https://data.eastmoney.com/report/info/" + id + ".html",
			OriginalID:  "em_report_" + id,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if it.Source != "" {
			item.Tags = append(item.Tags, it.Source)
		}
		if code != "" {
			// 研报带原生 stockName，命中即视为确定关联。
			confidence := 0.9
			matchType := newsfeed.MatchNameHit
			if strings.Contains(it.StockName, keyword) || keyword == code {
				matchType = newsfeed.MatchNative
				confidence = 1.0
			}
			item.StockRefs = []newsfeed.StockRef{{Code: code, MatchType: matchType, Confidence: confidence}}
			item.RelatedStocks = []string{code}
		}
		out = append(out, item)
	}
	return out
}

func (s *EastMoneySource) parseTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Now()
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, s.location); err == nil {
			return t
		}
	}
	return time.Now()
}

// unwrapJSONP 剥掉 jQuery(...) 外壳。东财返回的是 JSONP 而非纯 JSON。
func unwrapJSONP(s string) (string, error) {
	s = strings.TrimSpace(s)
	open := strings.Index(s, "(")
	if open < 0 || !strings.HasSuffix(s, ")") {
		return "", fmt.Errorf("响应不是 JSONP 格式")
	}
	return s[open+1 : len(s)-1], nil
}

type eastMoneyResponse struct {
	Code   int             `json:"code"`
	Msg    string          `json:"msg"`
	Hits   int             `json:"hitsTotal"`
	Result eastMoneyResult `json:"result"`
}

type eastMoneyResult struct {
	Articles       []eastMoneyArticle `json:"cmsArticleWebOld"`
	ResearchReport []eastMoneyReport  `json:"researchReport"`
}

type eastMoneyArticle struct {
	Code      string `json:"code"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Date      string `json:"date"`
	MediaName string `json:"mediaName"`
	URL       string `json:"url"`
	Image     string `json:"image"`
}

type eastMoneyReport struct {
	Code      string `json:"code"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Date      string `json:"date"`
	Source    string `json:"source"`
	StockName string `json:"stockName"`
}
