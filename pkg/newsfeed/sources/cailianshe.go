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

// CaiLianSheSource 财联社数据源。
//
// 旧的 www.cls.cn/api/sw 接口已下线（返回 405），nodeapi 的电报接口需要签名
// 且路径已变更（返回 404）。当前使用移动端 telegraphs 接口，实测可用。
//
// 该接口是一个滚动快讯流：只覆盖最近若干条，且没有按股票查询的入口。
// 因此个股关联来自两个途径：
//  1. 快讯自带的 stock_list（原生关联，精度最高）
//  2. 落库时由实体识别从正文补充
//
// FetchByStock 只能返回当前流中恰好命中该股票的内容，命中不到就返回空——
// 这是数据源的客观限制，不伪造结果。
type CaiLianSheSource struct {
	baseURL string
}

// NewCaiLianSheSource 创建财联社数据源
func NewCaiLianSheSource() *CaiLianSheSource {
	return &CaiLianSheSource{baseURL: "https://m.cls.cn"}
}

// Name 返回数据源名称
func (s *CaiLianSheSource) Name() newsfeed.SourceType { return newsfeed.SourceCaiLianShe }

// RefreshInterval 返回建议的刷新间隔
func (s *CaiLianSheSource) RefreshInterval() time.Duration { return 60 * time.Second }

// HealthCheck 检查数据源健康状态
func (s *CaiLianSheSource) HealthCheck(ctx context.Context) bool {
	_, err := s.fetchRaw(ctx, 1)
	return err == nil
}

// Fetch 获取最新快讯流
func (s *CaiLianSheSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	items, err := s.fetchRaw(ctx, 20)
	if err != nil {
		return nil, err
	}
	return s.parse(items), nil
}

// FetchByStock 从当前快讯流中筛选指定股票相关内容。
// 财联社没有按股票查询的接口，只能过滤现有的滚动流。
func (s *CaiLianSheSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return nil, nil
	}
	items, err := s.fetchRaw(ctx, 20)
	if err != nil {
		return nil, err
	}
	var matched []*newsfeed.NewsItem
	for _, it := range s.parse(items) {
		for _, ref := range it.StockRefs {
			if ref.Code == code {
				matched = append(matched, it)
				break
			}
		}
	}
	return matched, nil
}

// FetchByKeyword 按关键词过滤当前快讯流
func (s *CaiLianSheSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	items, err := s.fetchRaw(ctx, 50)
	if err != nil {
		return nil, err
	}
	var matched []*newsfeed.NewsItem
	for _, it := range s.parse(items) {
		if strings.Contains(it.Title, keyword) || strings.Contains(it.Summary, keyword) {
			matched = append(matched, it)
		}
	}
	return matched, nil
}

func (s *CaiLianSheSource) fetchRaw(ctx context.Context, rn int) ([]caiLianSheItem, error) {
	if rn <= 0 {
		rn = 20
	}
	req := fmt.Sprintf("%s/nodeapi/telegraphs?app=CailianpressWap&os=web&sv=1.0&rn=%d", s.baseURL, rn)
	data, err := httpGet(ctx, req, map[string]string{
		"Referer":    "https://m.cls.cn/telegraph",
		"User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148",
	})
	if err != nil {
		return nil, err
	}
	var resp caiLianSheResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("财联社响应解析失败: %w", err)
	}
	if resp.Error != nil && resp.Error.Code != 0 {
		msg := resp.Error.Message
		if msg == "" {
			msg = fmt.Sprintf("错误码 %d", resp.Error.Code)
		}
		return nil, fmt.Errorf("财联社返回错误: %s", msg)
	}
	return resp.Data.RollData, nil
}

func (s *CaiLianSheSource) parse(items []caiLianSheItem) []*newsfeed.NewsItem {
	now := time.Now()
	out := make([]*newsfeed.NewsItem, 0, len(items))
	for _, it := range items {
		if it.ID == 0 {
			continue
		}
		brief := newsfeed.StripHTML(it.Brief)
		content := newsfeed.StripHTML(it.Content)
		title := newsfeed.StripHTML(it.Title)
		if title == "" {
			title = brief
		}
		runes := []rune(title)
		if len(runes) > 60 {
			title = string(runes[:60])
		}
		summary := brief
		if summary == "" {
			summary = content
		}
		if r := []rune(summary); len(r) > 200 {
			summary = string(r[:200])
		}

		newsItem := &newsfeed.NewsItem{
			Source:      newsfeed.SourceCaiLianShe,
			NewsType:    newsfeed.NewsTypeFlash,
			Title:       title,
			Summary:     summary,
			Content:     content,
			PublishTime: it.publishTime(),
			HotScore:    it.hotScore(),
			Tags:        it.Tags,
			URL:         it.ShareURL,
			OriginalID:  fmt.Sprintf("cls_%d", it.ID),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if newsItem.URL == "" {
			newsItem.URL = fmt.Sprintf("https://www.cls.cn/detail/%d", it.ID)
		}
		newsItem.StockRefs = it.stockRefs()
		if len(newsItem.StockRefs) > 0 {
			newsItem.RelatedStocks = make([]string, 0, len(newsItem.StockRefs))
			for _, r := range newsItem.StockRefs {
				newsItem.RelatedStocks = append(newsItem.RelatedStocks, r.Code)
			}
		}
		out = append(out, newsItem)
	}
	return out
}

// stockRefs 从原生 stock_list 提取关联。StockID 形如 "sh688012"。
func (it caiLianSheItem) stockRefs() []newsfeed.StockRef {
	var refs []newsfeed.StockRef
	seen := make(map[string]bool)
	for _, st := range it.StockList {
		code := normalizeCLSStockID(st.StockID)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		refs = append(refs, newsfeed.StockRef{
			Code:       code,
			MatchType:  newsfeed.MatchNative,
			Confidence: 1.0,
		})
	}
	return refs
}

func (it caiLianSheItem) publishTime() time.Time {
	if it.CTime > 0 {
		return time.Unix(it.CTime, 0)
	}
	if it.ModifiedTime > 0 {
		return time.Unix(it.ModifiedTime, 0)
	}
	return time.Now()
}

// hotScore 把阅读/评论/分享与重要级别折算成 0-100 的热度。
func (it caiLianSheItem) hotScore() int {
	score := it.ReadingNum/50 + it.CommentNum*2 + it.ShareNum/20
	switch strings.ToUpper(strings.TrimSpace(it.Level)) {
	case "A":
		score += 30
	case "B":
		score += 10
	}
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score
}

// normalizeCLSStockID 把 "sh688012" / "688012.SH" 归一为 6 位代码。
func normalizeCLSStockID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.TrimSuffix(id, ".sh")
	id = strings.TrimSuffix(id, ".sz")
	id = strings.TrimSuffix(id, ".bj")
	for _, p := range []string{"sh", "sz", "bj"} {
		id = strings.TrimPrefix(id, p)
	}
	if len(id) != 6 {
		return ""
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return id
}

// telegraphURL 构造快讯详情页地址（供测试与降级使用）
func telegraphURL(base string, id int) string {
	return base + "/detail/" + url.QueryEscape(fmt.Sprint(id))
}

type caiLianSheResponse struct {
	Error *flexError     `json:"error"`
	Data  caiLianSheData `json:"data"`
}

// flexError 兼容财联社的两种 error 形态：成功时是数字 0，失败时是对象。
// 只按其中一种解析会在另一种形态下直接解析失败。
type flexError struct {
	Code    int
	Message string
}

func (e *flexError) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		e.Code = n
		return nil
	}
	var obj struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	e.Code = obj.Code
	e.Message = obj.Message
	return nil
}

type caiLianSheData struct {
	RollData []caiLianSheItem `json:"roll_data"`
}

type caiLianSheItem struct {
	ID           int64             `json:"id"`
	Title        string            `json:"title"`
	Brief        string            `json:"brief"`
	Content      string            `json:"content"`
	CTime        int64             `json:"ctime"`
	ModifiedTime int64             `json:"modified_time"`
	Level        string            `json:"level"`
	ShareURL     string            `json:"shareurl"`
	CommentNum   int               `json:"comment_num"`
	ReadingNum   int               `json:"reading_num"`
	ShareNum     int               `json:"share_num"`
	Tags         []string          `json:"tags"`
	StockList    []caiLianSheStock `json:"stock_list"`
}

type caiLianSheStock struct {
	Name    string `json:"name"`
	StockID string `json:"StockID"`
}
