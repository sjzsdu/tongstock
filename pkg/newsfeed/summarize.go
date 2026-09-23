package newsfeed

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

// 跨渠道标题聚合的可调参数。
const (
	// digestSimilarityThreshold 字符二元组 Dice 相似度阈值。
	digestSimilarityThreshold = 0.55
	// digestContainMinRunes 判定包含关系时，较短标题的最小长度（规范化后）。
	digestContainMinRunes = 6
)

// DigestGroup 聚合后的事件组：把多个渠道对同一事件的报道合并为一组。
type DigestGroup struct {
	// Title 代表标题，取组内最长的一条（信息最完整）。
	Title string `json:"title"`
	// Sources 组内出现过的渠道，按报道条数降序。
	Sources []SourceType  `json:"sources"`
	Count   int           `json:"count"`
	From    time.Time     `json:"from"`
	To      time.Time     `json:"to"`
	Items   []NewsSummary `json:"items"` // 时间倒序
}

// NewsDigest 个股资讯跨渠道综合摘要。
// 在 StockNews 结果之上做二次聚合：统计各渠道/类型的分布，
// 把跨渠道重复报道合并为事件组，并保留完整时间线，供进一步分析。
type NewsDigest struct {
	Code         string              `json:"code"`
	GeneratedAt  time.Time           `json:"generatedAt"`
	Status       string              `json:"status"`
	Message      string              `json:"message,omitempty"`
	Total        int                 `json:"total"`
	WeakCount    int                 `json:"weakCount"`
	From         time.Time           `json:"from"`
	To           time.Time           `json:"to"`
	SourceCounts map[string]int      `json:"sourceCounts"`
	TypeCounts   map[string]int      `json:"typeCounts"`
	Groups       []DigestGroup       `json:"groups"`
	Items        []NewsSummary       `json:"items"` // 时间倒序的完整时间线
	Degraded     []SourceDegradation `json:"degraded,omitempty"`
}

// BuildNewsDigest 基于 StockNews 结果构建跨渠道综合摘要。
// 纯函数，不做任何 IO。
func BuildNewsDigest(res *StockNewsResult) *NewsDigest {
	d := &NewsDigest{
		GeneratedAt:  time.Now(),
		SourceCounts: map[string]int{},
		TypeCounts:   map[string]int{},
		Groups:       []DigestGroup{},
		Items:        []NewsSummary{},
	}
	if res == nil {
		return d
	}
	d.Code = res.Code
	d.Status = res.Status
	d.Message = res.Message
	d.WeakCount = res.WeakCount
	d.Degraded = res.Degraded

	items := append([]NewsSummary(nil), res.Items...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].PublishTime.After(items[j].PublishTime)
	})
	d.Total = len(items)
	d.Items = items
	if len(items) == 0 {
		return d
	}
	d.From = items[len(items)-1].PublishTime
	d.To = items[0].PublishTime
	for _, it := range items {
		d.SourceCounts[string(it.Source)]++
		d.TypeCounts[string(it.NewsType)]++
	}
	d.Groups = groupDigestItems(items)
	return d
}

// groupDigestItems 把标题相近的条目合并为事件组（按时间倒序贪心扫描）。
// 只返回包含 2 条以上的组；单条目不构成“综合”。
func groupDigestItems(items []NewsSummary) []DigestGroup {
	type groupState struct {
		title      string // 代表标题（当前最长）
		titleRunes int
		norm       string
		items      []NewsSummary
	}
	var states []groupState
	for _, it := range items {
		norm := normalizeTitle(it.Title)
		runes := len([]rune(it.Title))
		matched := -1
		for i := range states {
			if digestSimilar(norm, states[i].norm) {
				matched = i
				break
			}
		}
		if matched < 0 {
			states = append(states, groupState{title: it.Title, titleRunes: runes, norm: norm, items: []NewsSummary{it}})
			continue
		}
		g := &states[matched]
		g.items = append(g.items, it)
		if runes > g.titleRunes {
			g.title = it.Title
			g.titleRunes = runes
		}
	}

	groups := make([]DigestGroup, 0, len(states))
	for _, g := range states {
		if len(g.items) < 2 {
			continue
		}
		sourceCount := map[SourceType]int{}
		var from, to time.Time
		for _, it := range g.items {
			sourceCount[it.Source]++
			if from.IsZero() || it.PublishTime.Before(from) {
				from = it.PublishTime
			}
			if to.IsZero() || it.PublishTime.After(to) {
				to = it.PublishTime
			}
		}
		sources := make([]SourceType, 0, len(sourceCount))
		for s := range sourceCount {
			sources = append(sources, s)
		}
		sort.Slice(sources, func(i, j int) bool {
			if sourceCount[sources[i]] != sourceCount[sources[j]] {
				return sourceCount[sources[i]] > sourceCount[sources[j]]
			}
			return sources[i] < sources[j]
		})
		groups = append(groups, DigestGroup{
			Title:   g.title,
			Sources: sources,
			Count:   len(g.items),
			From:    from,
			To:      to,
			Items:   g.items,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		return groups[i].To.After(groups[j].To)
	})
	return groups
}

// digestSimilar 判断两个规范化标题是否描述同一事件：
// 规范化后完全相同、存在包含关系（较短者达到最小长度），
// 或字符二元组 Dice 相似度达到阈值。
func digestSimilar(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	shorter, longer := a, b
	if len([]rune(a)) > len([]rune(b)) {
		shorter, longer = b, a
	}
	if len([]rune(shorter)) >= digestContainMinRunes && strings.Contains(longer, shorter) {
		return true
	}
	return diceSimilarity(a, b) >= digestSimilarityThreshold
}

// normalizeTitle 归一化标题：转小写并只保留字母/数字（含中文），
// 忽略标点与空白，便于跨渠道比对。
func normalizeTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// diceSimilarity 基于字符二元组的 Dice 相似度。
func diceSimilarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) < 2 || len(rb) < 2 {
		return 0
	}
	bigrams := func(r []rune) map[string]int {
		m := make(map[string]int, len(r)-1)
		for i := 0; i < len(r)-1; i++ {
			m[string(r[i:i+2])]++
		}
		return m
	}
	ma, mb := bigrams(ra), bigrams(rb)
	common := 0
	for k, va := range ma {
		if vb, ok := mb[k]; ok {
			if va < vb {
				common += va
			} else {
				common += vb
			}
		}
	}
	return 2 * float64(common) / float64(len(ra)-1+len(rb)-1)
}
