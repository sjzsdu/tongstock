package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// NewSource 创建数据源实例
func NewSource(sourceType newsfeed.SourceType) (newsfeed.Feed, error) {
	switch sourceType {
	case newsfeed.SourceCaiLianShe:
		return NewCaiLianSheSource(), nil
	case newsfeed.SourceXueQiu:
		return NewXueQiuSource(), nil
	case newsfeed.SourceJuChao:
		return NewJuChaoSource(), nil
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// NewAllSources 创建所有数据源实例
func NewAllSources() []newsfeed.Feed {
	var feeds []newsfeed.Feed
	types := []newsfeed.SourceType{
		newsfeed.SourceCaiLianShe,
		newsfeed.SourceXueQiu,
		newsfeed.SourceJuChao,
	}
	for _, t := range types {
		if feed, err := NewSource(t); err == nil {
			feeds = append(feeds, feed)
		}
	}
	return feeds
}

// httpClient 共享的HTTP客户端
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// httpGet 通用HTTP GET请求
func httpGet(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// 添加默认User-Agent
	if headers == nil {
		headers = make(map[string]string)
	}
	if _, ok := headers["User-Agent"]; !ok {
		headers["User-Agent"] = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// parseJSON 通用JSON解析
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// extractStockCodes 从文本中提取股票代码
func extractStockCodes(text string) []string {
	var codes []string
	seen := make(map[string]bool)

	// 匹配6位数字的股票代码
	patterns := []string{
		`[0-9]{6}`, // 6位数字
	}

	for _, pattern := range patterns {
		matches := regexPattern(pattern, text)
		for _, match := range matches {
			if isStockCode(match) && !seen[match] {
				codes = append(codes, match)
				seen[match] = true
			}
		}
	}

	return codes
}

// isStockCode 判断是否为有效的A股股票代码
func isStockCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	prefix := code[:3]
	validPrefixes := []string{
		"000", "001", "002", "003", "300", "301", // 深交所
		"600", "601", "603", "605", "688", "689", // 上交所
		"800", "801", "802", "803", "804", "805", "806", "807", "808", "809",
		"810", "811", "812", "813", "814", "815", "816", "817", "818", "819",
		"820", "821", "822", "823", "824", "825", "826", "827", "828", "829",
		"830", "831", "832", "833", "834", "835", "836", "837", "838", "839",
		"840", "841", "842", "843", "844", "845", "846", "847", "848", "849",
		"850", "851", "852", "853", "854", "855", "856", "857", "858", "859",
		"860", "861", "862", "863", "864", "865", "866", "867", "868", "869",
		"870", "871", "872", "873", "874", "875", "876", "877", "878", "879",
		"880", "881", "882", "883", "884", "885", "886", "887", "888", "889",
		"890", "891", "892", "893", "894", "895", "896", "897", "898", "899", // 北交所
	}
	for _, p := range validPrefixes {
		if prefix == p {
			return true
		}
	}
	return false
}

// regexPattern 简单的正则匹配（为避免引入额外依赖，使用简单字符串匹配）
func regexPattern(pattern, text string) []string {
	var matches []string
	if pattern == `[0-9]{6}` {
		for i := 0; i <= len(text)-6; i++ {
			chunk := text[i : i+6]
			isDigits := true
			for _, c := range chunk {
				if c < '0' || c > '9' {
					isDigits = false
					break
				}
			}
			if isDigits {
				matches = append(matches, chunk)
			}
		}
	}
	return matches
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// cleanText 清理文本
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

// parseTime 解析时间字符串
func parseTime(timeStr string, formats []string) time.Time {
	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t
		}
	}
	return time.Now()
}
