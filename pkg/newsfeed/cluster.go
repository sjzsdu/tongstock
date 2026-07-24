package newsfeed

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ClusterConfig 聚类配置
type ClusterConfig struct {
	// KeywordMinFrequency 关键词最小出现频率
	KeywordMinFrequency int
	// CooccurrenceThreshold 共现阈值（两个关键词同时出现在多少条新闻中）
	CooccurrenceThreshold int
	// TimeWindow 时间窗口（分钟）
	TimeWindow int
	// SimilarityThreshold 相似度阈值（0-1）
	SimilarityThreshold float64
	// MinNewsCount 最小新闻数量
	MinNewsCount int
	// MaxEvents 最大事件数量
	MaxEvents int
}

// DefaultClusterConfig 默认聚类配置
func DefaultClusterConfig() ClusterConfig {
	return ClusterConfig{
		KeywordMinFrequency:   2,
		CooccurrenceThreshold: 1,
		TimeWindow:           60,
		SimilarityThreshold:  0.5,
		MinNewsCount:         2,
		MaxEvents:            20,
	}
}

// Clusterer 热点事件聚类器
type Clusterer struct {
	config ClusterConfig
	store  Store
}

// NewClusterer 创建聚类器
func NewClusterer(store Store, config ClusterConfig) *Clusterer {
	return &Clusterer{
		config: config,
		store:  store,
	}
}

// Cluster 对新闻进行聚类，生成热点事件
func (c *Clusterer) Cluster(ctx context.Context, news []*NewsItem) ([]*HotEvent, error) {
	if len(news) == 0 {
		return []*HotEvent{}, nil
	}

	// 1. 按时间窗口分组
	groups := c.groupByTimeWindow(news)

	var allEvents []*HotEvent
	for _, group := range groups {
		// 2. 提取关键词
		keywords := c.extractKeywords(group)
		if len(keywords) == 0 {
			continue
		}

		// 3. 构建关键词共现图
		cooccurrence := c.buildCooccurrenceGraph(group, keywords)

		// 4. 聚类关键词
		keywordClusters := c.clusterKeywords(keywords, cooccurrence)

		// 5. 生成热点事件
		events := c.generateEvents(group, keywordClusters)
		allEvents = append(allEvents, events...)
	}

	// 6. 去重事件
	allEvents = c.deduplicateEvents(allEvents)

	// 7. 排序并限制数量
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].HotIndex > allEvents[j].HotIndex
	})

	if len(allEvents) > c.config.MaxEvents {
		allEvents = allEvents[:c.config.MaxEvents]
	}

	return allEvents, nil
}

// groupByTimeWindow 按时间窗口分组
func (c *Clusterer) groupByTimeWindow(news []*NewsItem) [][]*NewsItem {
	if len(news) == 0 {
		return [][]*NewsItem{}
	}

	// 按时间排序
	sorted := make([]*NewsItem, len(news))
	copy(sorted, news)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PublishTime.Before(sorted[j].PublishTime)
	})

	var groups [][]*NewsItem
	currentGroup := []*NewsItem{sorted[0]}
	windowStart := sorted[0].PublishTime

	for i := 1; i < len(sorted); i++ {
		if sorted[i].PublishTime.Sub(windowStart).Minutes() <= float64(c.config.TimeWindow) {
			currentGroup = append(currentGroup, sorted[i])
		} else {
			groups = append(groups, currentGroup)
			currentGroup = []*NewsItem{sorted[i]}
			windowStart = sorted[i].PublishTime
		}
	}

	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}

	return groups
}

// extractKeywords 提取关键词
func (c *Clusterer) extractKeywords(news []*NewsItem) []string {
	keywordCounts := make(map[string]int)

	for _, item := range news {
		text := item.Title + " " + item.Summary
		keywords := c.tokenize(text)
		for _, kw := range keywords {
			kw = strings.ToLower(strings.TrimSpace(kw))
			if len(kw) >= 2 && !isStopWord(kw) {
				keywordCounts[kw]++
			}
		}
	}

	var keywords []string
	for kw, count := range keywordCounts {
		if count >= c.config.KeywordMinFrequency {
			keywords = append(keywords, kw)
		}
	}

	// 按频率排序
	sort.Slice(keywords, func(i, j int) bool {
		return keywordCounts[keywords[i]] > keywordCounts[keywords[j]]
	})

	return keywords
}

// tokenize 简单分词（中文按字符，英文按空格）
func (c *Clusterer) tokenize(text string) []string {
	var tokens []string
	var currentToken []rune

	for _, r := range text {
		if isChineseChar(r) {
			if len(currentToken) > 0 {
				tokens = append(tokens, string(currentToken))
				currentToken = nil
			}
			tokens = append(tokens, string(r))
		} else if isLetterOrDigit(r) {
			currentToken = append(currentToken, r)
		} else {
			if len(currentToken) > 0 {
				tokens = append(tokens, string(currentToken))
				currentToken = nil
			}
		}
	}

	if len(currentToken) > 0 {
		tokens = append(tokens, string(currentToken))
	}

	// 组合相邻中文单字形成多字词语
	return c.combineChineseTokens(tokens)
}

// combineChineseTokens 组合相邻中文单字
func (c *Clusterer) combineChineseTokens(tokens []string) []string {
	var result []string
	i := 0
	for i < len(tokens) {
		if len(tokens[i]) == 1 && isChineseChar(rune(tokens[i][0])) {
			// 尝试组合2-3个相邻中文单字
			if i+1 < len(tokens) && len(tokens[i+1]) == 1 && isChineseChar(rune(tokens[i+1][0])) {
				combined2 := tokens[i] + tokens[i+1]
				if i+2 < len(tokens) && len(tokens[i+2]) == 1 && isChineseChar(rune(tokens[i+2][0])) {
					combined3 := combined2 + tokens[i+2]
					result = append(result, combined3)
					i += 3
				} else {
					result = append(result, combined2)
					i += 2
				}
			} else {
				result = append(result, tokens[i])
				i++
			}
		} else {
			result = append(result, tokens[i])
			i++
		}
	}
	return result
}

// isChineseChar 判断是否为中文字符
func isChineseChar(r rune) bool {
	return r >= '\u4e00' && r <= '\u9fff'
}

// isLetterOrDigit 判断是否为字母或数字
func isLetterOrDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// isStopWord 判断是否为停用词
func isStopWord(word string) bool {
	stopWords := map[string]bool{
		"的": true, "是": true, "在": true, "有": true, "和": true, "了": true, "我": true, "你": true, "他": true,
		"她": true, "它": true, "这": true, "那": true, "说": true, "看": true, "听": true, "想": true,
		"要": true, "会": true, "可以": true, "能": true, "不": true, "很": true, "也": true, "都": true,
		"就": true, "但": true, "因为": true, "所以": true, "如果": true, "虽然": true, "但是": true,
		"一个": true, "一些": true, "什么": true, "怎么": true, "为什么": true, "哪里": true, "时候": true,
		"今天": true, "明天": true, "昨天": true, "现在": true, "过去": true, "将来": true,
	}
	return stopWords[word]
}

// buildCooccurrenceGraph 构建关键词共现图
func (c *Clusterer) buildCooccurrenceGraph(news []*NewsItem, keywords []string) map[string]map[string]int {
	cooccurrence := make(map[string]map[string]int)
	for _, kw1 := range keywords {
		cooccurrence[kw1] = make(map[string]int)
		for _, kw2 := range keywords {
			if kw1 != kw2 {
				cooccurrence[kw1][kw2] = 0
			}
		}
	}

	keywordSet := make(map[string]bool)
	for _, kw := range keywords {
		keywordSet[kw] = true
	}

	for _, item := range news {
		text := strings.ToLower(item.Title + " " + item.Summary)
		found := make(map[string]bool)
		for _, kw := range keywords {
			if strings.Contains(text, kw) {
				found[kw] = true
			}
		}

		foundList := make([]string, 0, len(found))
		for kw := range found {
			foundList = append(foundList, kw)
		}

		for i := 0; i < len(foundList); i++ {
			for j := i + 1; j < len(foundList); j++ {
				kw1, kw2 := foundList[i], foundList[j]
				cooccurrence[kw1][kw2]++
				cooccurrence[kw2][kw1]++
			}
		}
	}

	return cooccurrence
}

// clusterKeywords 聚类关键词
func (c *Clusterer) clusterKeywords(keywords []string, cooccurrence map[string]map[string]int) [][]string {
	visited := make(map[string]bool)
	var clusters [][]string

	for _, kw := range keywords {
		if visited[kw] {
			continue
		}

		cluster := c.bfsCluster(kw, keywords, cooccurrence, visited)
		if len(cluster) >= 2 {
			clusters = append(clusters, cluster)
		}
	}

	return clusters
}

// bfsCluster 使用BFS进行关键词聚类
func (c *Clusterer) bfsCluster(start string, keywords []string, cooccurrence map[string]map[string]int, visited map[string]bool) []string {
	var cluster []string
	queue := []string{start}
	visited[start] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		cluster = append(cluster, current)

		for _, kw := range keywords {
			if visited[kw] || kw == current {
				continue
			}

			count := cooccurrence[current][kw]
			if count >= c.config.CooccurrenceThreshold {
				visited[kw] = true
				queue = append(queue, kw)
			}
		}
	}

	return cluster
}

// generateEvents 生成热点事件
func (c *Clusterer) generateEvents(news []*NewsItem, keywordClusters [][]string) []*HotEvent {
	var events []*HotEvent

	for _, cluster := range keywordClusters {
		// 找到相关新闻
		var relatedNews []*NewsItem
		keywordSet := make(map[string]bool)
		for _, kw := range cluster {
			keywordSet[kw] = true
		}

		for _, item := range news {
			text := strings.ToLower(item.Title + " " + item.Summary)
			matchCount := 0
			for kw := range keywordSet {
				if strings.Contains(text, kw) {
					matchCount++
				}
			}
			if matchCount >= 1 {
				relatedNews = append(relatedNews, item)
			}
		}

		if len(relatedNews) < c.config.MinNewsCount {
			continue
		}

		event := c.createEvent(relatedNews, cluster)
		events = append(events, event)
	}

	return events
}

// createEvent 创建热点事件
func (c *Clusterer) createEvent(news []*NewsItem, keywords []string) *HotEvent {
	// 生成事件标题
	title := c.generateEventTitle(news, keywords)

	// 统计各平台提及次数
	sourceCounts := make(map[string]int)
	for _, item := range news {
		sourceCounts[string(item.Source)]++
	}

	// 提取相关股票
	relatedStocks := make(map[string]bool)
	for _, item := range news {
		for _, code := range item.RelatedStocks {
			relatedStocks[code] = true
		}
	}

	stocksList := make([]string, 0, len(relatedStocks))
	for code := range relatedStocks {
		stocksList = append(stocksList, code)
	}

	// 计算热度指数
	hotIndex := c.calculateHotIndex(news, keywords, sourceCounts)

	// 收集新闻ID
	newsItemIDs := make([]string, 0, len(news))
	for _, item := range news {
		newsItemIDs = append(newsItemIDs, item.ID)
	}

	return &HotEvent{
		ID:            fmt.Sprintf("event_%d", time.Now().UnixNano()),
		Title:         title,
		Keywords:      keywords,
		RelatedStocks: stocksList,
		HotIndex:      hotIndex,
		SourceCounts:  sourceCounts,
		NewsItemIDs:   newsItemIDs,
		Status:        EventStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// generateEventTitle 生成事件标题
func (c *Clusterer) generateEventTitle(news []*NewsItem, keywords []string) string {
	// 使用出现频率最高的关键词组合作为标题
	if len(keywords) >= 2 {
		return keywords[0] + " " + keywords[1]
	} else if len(keywords) == 1 {
		return keywords[0]
	}

	// 如果没有关键词，使用第一条新闻的标题
	if len(news) > 0 {
		return news[0].Title[:min(len(news[0].Title), 30)]
	}

	return "未知事件"
}

// calculateHotIndex 计算热度指数（1-100）
func (c *Clusterer) calculateHotIndex(news []*NewsItem, keywords []string, sourceCounts map[string]int) int {
	// 基础分：新闻数量
	baseScore := len(news) * 10

	// 平台多样性加分
	platformScore := len(sourceCounts) * 15

	// 热度加分
	hotScore := 0
	for _, item := range news {
		hotScore += item.HotScore
	}
	hotScore = min(hotScore/10, 30)

	// 关键词数量加分
	keywordScore := len(keywords) * 5

	total := baseScore + platformScore + hotScore + keywordScore
	return min(max(total, 1), 100)
}

// deduplicateEvents 去重事件
func (c *Clusterer) deduplicateEvents(events []*HotEvent) []*HotEvent {
	seen := make(map[string]bool)
	var unique []*HotEvent

	for _, event := range events {
		signature := c.eventSignature(event)
		if !seen[signature] {
			seen[signature] = true
			unique = append(unique, event)
		}
	}

	return unique
}

// eventSignature 生成事件签名（用于去重）
func (c *Clusterer) eventSignature(event *HotEvent) string {
	// 使用关键词和标题生成签名
	sig := strings.Join(event.Keywords, "-") + "-" + event.Title
	return fmt.Sprintf("%x", sha256.Sum256([]byte(sig)))[:16]
}

// DeduplicateNews 新闻去重
func (c *Clusterer) DeduplicateNews(news []*NewsItem) []*NewsItem {
	seen := make(map[string]bool)
	var unique []*NewsItem

	for _, item := range news {
		signature := c.newsSignature(item)
		if !seen[signature] {
			seen[signature] = true
			unique = append(unique, item)
		}
	}

	return unique
}

// newsSignature 生成新闻签名（用于去重）
func (c *Clusterer) newsSignature(item *NewsItem) string {
	// 使用标题和来源生成签名
	sig := string(item.Source) + "-" + item.Title
	return fmt.Sprintf("%x", sha256.Sum256([]byte(sig)))[:16]
}

// UpdateEventStatus 更新事件状态
func (c *Clusterer) UpdateEventStatus(ctx context.Context) error {
	if c.store == nil {
		return ErrStoreNotSet
	}

	now := time.Now()
	filter := HotEventFilter{
		Status: []EventStatus{EventStatusActive},
		Limit:  100,
	}

	result, err := c.store.GetHotEvents(ctx, filter)
	if err != nil {
		return err
	}

	for _, item := range result.Items {
		event, err := c.store.GetHotEventDetail(ctx, item.ID)
		if err != nil {
			continue
		}

		// 判断事件状态
		if now.Sub(event.UpdatedAt).Hours() > 24 {
			event.Status = EventStatusEnded
		} else if now.Sub(event.UpdatedAt).Hours() > 6 {
			event.Status = EventStatusCooling
		}

		if err := c.store.SaveHotEvent(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

// min 返回最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max 返回最大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
