package newsfeed

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// refreshScanSize 是刷新热点事件时每次扫描的新闻条数。
// 聚类是 O(n²) 的关键词两两比对，一次吞下全库既有内存风险，
// 也拖慢刷新节奏；分页从最新往回扫，直到凑满一个时间窗口为止。
const refreshScanSize = 500

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
		TimeWindow:            60,
		SimilarityThreshold:   0.5,
		MinNewsCount:          2,
		MaxEvents:             20,
	}
}

// Clusterer 热点事件聚类器
type Clusterer struct {
	config ClusterConfig
	store  Store
	// ensureMu/lastEnsure 用于 EnsureHotEvents 的节流，避免并发重复聚类。
	ensureMu   sync.Mutex
	lastEnsure time.Time
}

// ensureCooldown 是两次「热点事件补建检查」之间的最小间隔。
// 聚类开销不小，列表页轮询不应每次都触发。
const ensureCooldown = 5 * time.Minute

// NewClusterer 创建聚类器
func NewClusterer(store Store, config ClusterConfig) *Clusterer {
	return &Clusterer{
		config: config,
		store:  store,
	}
}

// RefreshEvents 扫描最近的新闻并重建热点事件。
//
// 热点事件此前只能靠手动 POST /events/refresh 生成，页面上的「热点TOP10」
// 在没人调过这个接口时永远是空的。刷新流程本身与手动接口一致：
// 按时间窗口分组 → 提取关键词 → 聚类 → 生成事件 → 落库。
func (c *Clusterer) RefreshEvents(ctx context.Context) (int, error) {
	if c.store == nil {
		return 0, ErrStoreNotSet
	}

	news, err := c.scanRecentNews(ctx, c.config.TimeWindow)
	if err != nil {
		return 0, err
	}
	if len(news) == 0 {
		return 0, nil
	}

	events, err := c.Cluster(ctx, news)
	if err != nil {
		return 0, err
	}

	// 库里的同名旧事件会先合并进本轮事件再删除，确保同名事件只保留一条。
	events, err = c.mergeWithActiveEvents(ctx, events)
	if err != nil {
		return 0, err
	}

	saved := 0
	for _, event := range events {
		if err := c.store.SaveHotEvent(ctx, event); err != nil {
			return saved, err
		}
		saved++
	}
	return saved, nil
}

// mergeWithActiveEvents 把本轮事件与库里的活跃事件按标题对齐：
//   - 同名事件：把旧事件累积的新闻、平台计数、相关股票并入新事件后删除旧记录，
//     再落新事件，避免同名旧事件残留；
//   - 库里同名但本轮没再发现的事件：说明该话题最近没有新新闻，但事件窗口
//     远大于刷新扫描窗口，直接删除会造成话题闪断，保留。
func (c *Clusterer) mergeWithActiveEvents(ctx context.Context, events []*HotEvent) ([]*HotEvent, error) {
	active, err := c.store.GetHotEvents(ctx, HotEventFilter{
		Status: []EventStatus{EventStatusActive, EventStatusCooling},
		Limit:  100,
	})
	if err != nil {
		return nil, err
	}

	// 先按标题索引本轮事件，再逐条处理旧事件。
	byTitle := make(map[string]*HotEvent, len(events))
	for _, event := range events {
		byTitle[eventTitleKey(event.Title)] = event
	}

	for _, old := range active.Items {
		current, ok := byTitle[eventTitleKey(old.Title)]
		if !ok {
			continue
		}
		detail, err := c.store.GetHotEventDetail(ctx, old.ID)
		if err != nil {
			// 旧详情读不到就直接覆盖：新事件是本次窗口的准确统计。
			if err := c.store.DeleteHotEvent(ctx, old.ID); err != nil {
				return nil, err
			}
			continue
		}
		current.mergeOlder(detail)
		if err := c.store.DeleteHotEvent(ctx, old.ID); err != nil {
			return nil, err
		}
	}

	return events, nil
}

// scanRecentNews 从最新一条新闻往回扫描，直到出现一条落在时间窗口之外的
// 新闻为止——窗口之外的新闻与窗口内的无法聚到同一个事件里，没必要继续读。
// FilterNews 按时间倒序返回，且排序键同时写进了 where，保证窗口判断可靠。
func (c *Clusterer) scanRecentNews(ctx context.Context, windowMinutes int) ([]*NewsItem, error) {
	var out []*NewsItem
	for page := 1; ; page++ {
		result, err := c.store.FilterNews(ctx, FeedFilter{
			SortBy:   "time",
			PageSize: refreshScanSize,
			PageNum:  page,
		})
		if err != nil {
			return out, err
		}
		if len(result.Items) == 0 {
			break
		}
		for i := range result.Items {
			summary := &result.Items[i]
			item, err := c.store.GetNewsByID(ctx, summary.ID)
			if err != nil {
				continue
			}
			if windowMinutes > 0 && time.Since(item.PublishTime) > time.Duration(windowMinutes)*time.Minute {
				return out, nil
			}
			out = append(out, item)
		}
		if len(result.Items) < refreshScanSize {
			break
		}
	}
	return out, nil
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
	// keywordOrder 记录关键词首次出现的顺序，同频时用于稳定排序。
	keywordOrder := make(map[string]int)
	order := 0

	for _, item := range news {
		text := item.Title + " " + item.Summary
		keywords := c.tokenize(text)
		for _, kw := range keywords {
			kw = strings.ToLower(strings.TrimSpace(kw))
			// 按「字符数」而非字节数判断：单个汉字占 3 字节，
			// 用字节长度会把「日」「国」这类单字放进关键词池，
			// 高频单字聚出来的事件标题就是无意义的字面拼凑。
			if utf8.RuneCountInString(kw) >= 2 && !isStopWord(kw) && !isPureDigits(kw) && !isCodeToken(kw) {
				if _, seen := keywordCounts[kw]; !seen {
					keywordOrder[kw] = order
					order++
				}
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

	// 频率优先，同频按在新闻中首次出现的先后排：后者让标题读起来
	// 接近原文语序（「ai openai」而不是「openai ai」），也让排序
	// 不再依赖 map 遍历顺序。
	sort.Slice(keywords, func(i, j int) bool {
		if keywordCounts[keywords[i]] != keywordCounts[keywords[j]] {
			return keywordCounts[keywords[i]] > keywordCounts[keywords[j]]
		}
		return keywordOrder[keywords[i]] < keywordOrder[keywords[j]]
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

// isPureDigits 过滤「29」「2026」这类纯数字词：日期与数字对热点命名没有意义。
func isPureDigits(word string) bool {
	for _, r := range word {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(word) > 0
}

// isCodeToken 过滤「20cm3」「7x24」「sz000408」这类代码型 token：
// 它们是标题里的规格/栏目/代码噪声，频次高、语义弱，混进事件标题
// 就成了「20cm3 20cm」这种不知所云的名字。
func isCodeToken(word string) bool {
	hasLetter, hasDigit := false, false
	for _, r := range word {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	// 字母数字混合的英文 token（如 20cm3、7x24、a50、b20）是型号/栏目
	// 噪声而非术语，拦截；纯英文（kkr、openai）与纯数字（已被
	// isPureDigits 拦截）不受影响。
	return hasLetter && hasDigit
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
		ID:            "event_" + c.eventSignature(title),
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
//
// 取簇内前两个关键词拼接。排序必须保持 extractKeywords 的稳定性
// （高频优先、同频按原文出现位置）：此前同频词的先后取决于 map 遍历
// 顺序，同一话题每次刷新都会得到不同标题，事件 ID 随之漂移。
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
//
// 同一标题只保留一条：同名事件大多来自相邻时间桶对同一话题的重复发现，
// 保留热度更高的那个即可，避免 TOP10 里出现三条「ai openai」。
// 去重键是归一化标题而非全部关键词——关键词集合差一个尾词也不该算新事件。
func (c *Clusterer) deduplicateEvents(events []*HotEvent) []*HotEvent {
	byTitle := make(map[string]*HotEvent, len(events))
	order := make([]string, 0, len(events))

	for _, event := range events {
		key := eventTitleKey(event.Title)
		if existing, ok := byTitle[key]; ok {
			if event.HotIndex > existing.HotIndex {
				*existing = *event
			}
			continue
		}
		byTitle[key] = event
		order = append(order, key)
	}

	unique := make([]*HotEvent, 0, len(order))
	for _, key := range order {
		unique = append(unique, byTitle[key])
	}
	return unique
}

// mergeOlder 把旧事件中的信息合并进新事件：补充新窗口里没扫到的新闻、
// 累计平台计数、并集相关股票，热度取两者较高值。
func (e *HotEvent) mergeOlder(older *HotEvent) {
	if older == nil {
		return
	}

	seen := make(map[string]bool, len(e.NewsItemIDs))
	for _, id := range e.NewsItemIDs {
		seen[id] = true
	}
	for _, id := range older.NewsItemIDs {
		if !seen[id] {
			e.NewsItemIDs = append(e.NewsItemIDs, id)
		}
	}

	for source, count := range older.SourceCounts {
		e.SourceCounts[source] += count
	}

	stockSet := make(map[string]bool, len(e.RelatedStocks))
	for _, code := range e.RelatedStocks {
		stockSet[code] = true
	}
	for _, code := range older.RelatedStocks {
		if !stockSet[code] {
			e.RelatedStocks = append(e.RelatedStocks, code)
		}
	}

	if older.HotIndex > e.HotIndex {
		e.HotIndex = older.HotIndex
	}
}

// eventSignature 生成事件签名（标题内容哈希，用作事件 ID）
//
// 签名只由标题决定：同一话题在不同时间桶里聚出的关键词尾集总有细微
// 差异，若把关键词拼进签名，标题相同的事件也会得到不同 ID，
// INSERT OR REPLACE 变成了纯 INSERT，同名事件就这样在库里越积越多。
func (c *Clusterer) eventSignature(title string) string {
	sig := eventTitleKey(title)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(sig)))[:16]
}

// eventTitleKey 是事件标题的归一化键，同名事件互相覆盖。
func eventTitleKey(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
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

// EnsureHotEvents 保证「热点TOP10」有数据。
//
// 事件只由 RefreshEvents 生成；若从来没有刷新过（首次启动、老库升级），
// 列表页永远是空的。这里检查是否存在活跃事件，没有且库里有新闻时
// 先补一次刷新。带访问间隔限流：避免每次列表请求都触发一次全量聚类。
func (c *Clusterer) EnsureHotEvents(ctx context.Context) (int, error) {
	if c.store == nil {
		return 0, ErrStoreNotSet
	}
	if !c.ensureReady() {
		return 0, nil
	}
	return c.RefreshEvents(ctx)
}

// ensureReady 报告是否需要补建热点事件；通过时顺带记录本次时间。
func (c *Clusterer) ensureReady() bool {
	c.ensureMu.Lock()
	defer c.ensureMu.Unlock()

	if time.Since(c.lastEnsure) < ensureCooldown {
		return false
	}
	c.lastEnsure = time.Now()

	result, err := c.store.GetHotEvents(context.Background(), HotEventFilter{
		Status: []EventStatus{EventStatusActive, EventStatusCooling},
		Limit:  1,
	})
	if err != nil {
		return false
	}
	return len(result.Items) == 0
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
