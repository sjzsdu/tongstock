package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// findEgoBrowser looks for ego-browser in PATH and common install locations
func findEgoBrowser() string {
	// 1. Try PATH
	if path, err := exec.LookPath("ego-browser"); err == nil {
		return path
	}
	// 2. Try common install locations
	home, err := os.UserHomeDir()
	if err == nil {
		candidates := []string{
			filepath.Join(home, ".local", "bin", "ego-browser"),
			filepath.Join(home, "go", "bin", "ego-browser"),
			"/usr/local/bin/ego-browser",
			"/opt/homebrew/bin/ego-browser",
		}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				return c
			}
		}
	}
	return ""
}

// egoBrowserPath returns the ego-browser executable path, cached after first lookup
var egoBrowserPathCache string

func egoBrowserPath() string {
	if egoBrowserPathCache == "" {
		egoBrowserPathCache = findEgoBrowser()
	}
	return egoBrowserPathCache
}

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
	bin := egoBrowserPath()
	if bin == "" {
		return false
	}
	cmd := exec.CommandContext(ctx, bin, "help")
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
	bin := egoBrowserPath()
	if bin == "" {
		return nil, fmt.Errorf("ego-browser 未安装或不在 PATH 中")
	}
	cmd := exec.CommandContext(ctx, bin, "nodejs")
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

// extractJSONFromOutput 从 ego-browser 输出中提取 JSON 数据
func extractJSONFromOutput(output string) string {
	// 首先尝试找到第一个 [ 或 {
	startBracket := strings.Index(output, "[")
	startCurly := strings.Index(output, "{")

	start := -1
	if startBracket != -1 && startCurly != -1 {
		start = min(startBracket, startCurly)
	} else if startBracket != -1 {
		start = startBracket
	} else if startCurly != -1 {
		start = startCurly
	}

	if start == -1 {
		return ""
	}

	// 使用括号匹配来找到正确的结束位置
	nesting := 0
	isArray := output[start] == '['
	var endChar byte = ']'
	if !isArray {
		endChar = '}'
	}

	for i := start; i < len(output); i++ {
		c := output[i]
		if c == '[' || c == '{' {
			nesting++
		} else if c == ']' || c == '}' {
			nesting--
			if nesting == 0 && c == endChar {
				return output[start : i+1]
			}
		} else if c == '"' {
			// 跳过字符串内的内容
			i++
			for i < len(output) && output[i] != '"' {
				if output[i] == '\\' {
					i++
				}
				i++
			}
		}
	}

	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
