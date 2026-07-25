package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// BrowserSource 使用 ego-browser 抓取需要认证的财经新闻网站
type BrowserSource struct {
	siteType string // cailianshe | xueqiu
}

// NewBrowserSource 创建浏览器数据源
func NewBrowserSource(siteType string) *BrowserSource {
	return &BrowserSource{siteType: siteType}
}

// Name 返回数据源名称
func (s *BrowserSource) Name() newsfeed.SourceType {
	switch s.siteType {
	case "cailianshe":
		return newsfeed.SourceCaiLianShe
	case "xueqiu":
		return newsfeed.SourceXueQiu
	default:
		return newsfeed.SourceUnknown
	}
}

// RefreshInterval 返回建议的刷新间隔
func (s *BrowserSource) RefreshInterval() time.Duration {
	return 5 * time.Minute
}

// HealthCheck 检查 ego-browser 是否可用
func (s *BrowserSource) HealthCheck(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "ego-browser", "help")
	return cmd.Run() == nil
}

// Fetch 获取最新新闻列表
func (s *BrowserSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	script := s.getFetchScript()
	return s.runScript(ctx, script)
}

// FetchByStock 获取指定股票相关的新闻
func (s *BrowserSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	script := s.getFetchByStockScript(code)
	return s.runScript(ctx, script)
}

// FetchByKeyword 获取指定关键词相关的新闻
func (s *BrowserSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	script := s.getFetchByKeywordScript(keyword)
	return s.runScript(ctx, script)
}

// runScript 执行 ego-browser 抓取脚本并解析结果
func (s *BrowserSource) runScript(ctx context.Context, script string) ([]*newsfeed.NewsItem, error) {
	cmd := exec.CommandContext(ctx, "ego-browser", "nodejs")
	cmd.Stdin = strings.NewReader(script)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ego-browser 执行失败: %w", err)
	}

	// 从输出中提取 JSON 数据
	jsonStr := extractJSONFromOutput(string(output))
	if jsonStr == "" {
		return nil, fmt.Errorf("无法从 ego-browser 输出中提取数据")
	}

	var items []browserNewsItem
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		return nil, fmt.Errorf("解析新闻数据失败: %w", err)
	}

	return s.convertToNewsItems(items), nil
}

// convertToNewsItems 将浏览器抓取的数据转换为 NewsItem
func (s *BrowserSource) convertToNewsItems(items []browserNewsItem) []*newsfeed.NewsItem {
	now := time.Now()
	var news []*newsfeed.NewsItem

	for _, item := range items {
		newsItem := &newsfeed.NewsItem{
			ID:            generateID(),
			Source:        s.Name(),
			NewsType:      mapNewsType(item.NewsType),
			Title:         cleanText(item.Title),
			Summary:       cleanText(item.Summary),
			Content:       cleanText(item.Content),
			PublishTime:   parseTime(item.PublishTime, []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", time.RFC3339}),
			HotScore:      item.HotScore,
			Tags:          item.Tags,
			RelatedStocks: extractStockCodes(item.Title + " " + item.Summary),
			URL:           item.URL,
			OriginalID:    item.OriginalID,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if newsItem.OriginalID == "" {
			newsItem.OriginalID = fmt.Sprintf("browser_%s_%d", s.siteType, now.UnixNano())
		}
		news = append(news, newsItem)
	}

	return news
}

// browserNewsItem 浏览器抓取的新闻数据结构
type browserNewsItem struct {
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Content     string   `json:"content"`
	PublishTime string   `json:"publishTime"`
	HotScore    int      `json:"hotScore"`
	Tags        []string `json:"tags"`
	URL         string   `json:"url"`
	OriginalID  string   `json:"originalId"`
	NewsType    string   `json:"newsType"`
}

// mapNewsType 映射新闻类型
func mapNewsType(t string) newsfeed.NewsType {
	switch t {
	case "flash", "快讯":
		return newsfeed.NewsTypeFlash
	case "announcement", "公告":
		return newsfeed.NewsTypeAnnouncement
	case "discussion", "讨论":
		return newsfeed.NewsTypeDiscussion
	case "report", "研报":
		return newsfeed.NewsTypeReport
	default:
		return newsfeed.NewsTypeOther
	}
}

// extractJSONFromOutput 从 ego-browser 输出中提取 JSON 数组
func extractJSONFromOutput(output string) string {
	start := strings.Index(output, "[")
	if start == -1 {
		return ""
	}
	// 从最后一个 ] 开始查找
	end := strings.LastIndex(output, "]")
	if end == -1 || end <= start {
		return ""
	}
	return output[start : end+1]
}

// getFetchScript 返回抓取脚本
func (s *BrowserSource) getFetchScript() string {
	switch s.siteType {
	case "cailianshe":
		return cailiansheFetchScript
	case "xueqiu":
		return xueqiuFetchScript
	default:
		return ""
	}
}

// getFetchByStockScript 返回按股票抓取的脚本
func (s *BrowserSource) getFetchByStockScript(code string) string {
	switch s.siteType {
	case "cailianshe":
		return fmt.Sprintf(cailiansheFetchByStockScript, code)
	case "xueqiu":
		return fmt.Sprintf(xueqiuFetchByStockScript, code)
	default:
		return ""
	}
}

// getFetchByKeywordScript 返回按关键词抓取的脚本
func (s *BrowserSource) getFetchByKeywordScript(keyword string) string {
	switch s.siteType {
	case "cailianshe":
		return fmt.Sprintf(cailiansheFetchByKeywordScript, keyword)
	case "xueqiu":
		return fmt.Sprintf(xueqiuFetchByKeywordScript, keyword)
	default:
		return ""
	}
}
