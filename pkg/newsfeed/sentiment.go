package newsfeed

import (
	"context"
	"sort"
	"strings"
	"time"
)

// SentimentType 情绪类型
type SentimentType string

const (
	SentimentPositive SentimentType = "positive" // 正面
	SentimentNegative SentimentType = "negative" // 负面
	SentimentNeutral  SentimentType = "neutral"  // 中性
)

// SentimentResult 情绪分析结果
type SentimentResult struct {
	Type       SentimentType `json:"type"`
	Score      float64       `json:"score"`      // -1 到 1，负数表示负面，正数表示正面
	Confidence float64       `json:"confidence"` // 0 到 1，置信度
}

// SentimentAnalyzer 情绪分析器接口
type SentimentAnalyzer interface {
	Analyze(text string) SentimentResult
	AnalyzeNews(item *NewsItem) SentimentResult
}

// SimpleSentimentAnalyzer 简单情绪分析器（基于关键词）
type SimpleSentimentAnalyzer struct {
	positiveWords map[string]int
	negativeWords map[string]int
}

// NewSimpleSentimentAnalyzer 创建简单情绪分析器
func NewSimpleSentimentAnalyzer() *SimpleSentimentAnalyzer {
	return &SimpleSentimentAnalyzer{
		positiveWords: map[string]int{
			"上涨": 3, "涨停": 5, "大涨": 4, "暴涨": 5, "飙升": 4,
			"利好": 3, "利好消息": 4, "政策利好": 5, "扶持": 3, "支持": 3,
			"增长": 3, "上升": 3, "突破": 4, "创新高": 5, "走强": 3,
			"反弹": 3, "回暖": 3, "复苏": 4, "景气": 3, "繁荣": 4,
			"盈利": 3, "利润": 3, "分红": 4, "回购": 4, "增持": 3,
			"并购": 3, "重组": 4, "注入": 3, "优质": 3, "龙头": 4,
			"买入": 3, "推荐": 4, "看好": 3, "增持评级": 4,
			"超预期": 4, "亮眼": 3, "强劲": 3, "稳健": 3, "坚挺": 3,
			"降息": 3, "降准": 4, "宽松": 3, "刺激": 3, "提振": 3,
		},
		negativeWords: map[string]int{
			"下跌": 3, "跌停": 5, "大跌": 4, "暴跌": 5, "跳水": 4,
			"利空": 3, "利空消息": 4, "政策利空": 5, "打压": 3, "监管": 3,
			"下降": 3, "下滑": 3, "破位": 4, "创新低": 5, "走弱": 3,
			"低迷": 3, "萎缩": 3, "衰退": 4, "萧条": 4, "疲软": 3,
			"亏损": 3, "亏损扩大": 4, "业绩下滑": 4, "退市": 5, "ST": 4,
			"减持": 3, "卖出": 3, "规避": 4, "看空": 3, "减持评级": 4,
			"不及预期": 4, "惨淡": 3, "恶化": 3,
			"加息": 3, "收紧": 3, "紧缩": 4, "压制": 3, "冲击": 4,
			"违约": 4, "爆雷": 5, "诉讼": 3, "调查": 3, "问询": 3,
		},
	}
}

// Analyze 分析文本情绪
func (s *SimpleSentimentAnalyzer) Analyze(text string) SentimentResult {
	text = strings.ToLower(text)

	positiveScore := 0
	negativeScore := 0
	totalMatches := 0

	// 匹配正面词
	for word, weight := range s.positiveWords {
		if strings.Contains(text, strings.ToLower(word)) {
			positiveScore += weight
			totalMatches++
		}
	}

	// 匹配负面词
	for word, weight := range s.negativeWords {
		if strings.Contains(text, strings.ToLower(word)) {
			negativeScore += weight
			totalMatches++
		}
	}

	// 计算情绪分数
	var score float64
	var confidence float64

	if totalMatches > 0 {
		score = float64(positiveScore-negativeScore) / float64(positiveScore+negativeScore)
		confidence = float64(totalMatches) / 20.0 // 假设最多20个关键词匹配
		if confidence > 1.0 {
			confidence = 1.0
		}
	} else {
		score = 0
		confidence = 0.3 // 默认置信度
	}

	// 确定情绪类型
	var sentimentType SentimentType
	if score > 0.15 {
		sentimentType = SentimentPositive
	} else if score < -0.15 {
		sentimentType = SentimentNegative
	} else {
		sentimentType = SentimentNeutral
	}

	return SentimentResult{
		Type:       sentimentType,
		Score:      score,
		Confidence: confidence,
	}
}

// AnalyzeNews 分析新闻情绪
func (s *SimpleSentimentAnalyzer) AnalyzeNews(item *NewsItem) SentimentResult {
	text := item.Title + " " + item.Summary + " " + item.Content
	return s.Analyze(text)
}

// MarketSentiment 市场情绪数据
type MarketSentiment struct {
	Timestamp     time.Time `json:"timestamp"`
	PositiveCount int       `json:"positiveCount"`
	NegativeCount int       `json:"negativeCount"`
	NeutralCount  int       `json:"neutralCount"`
	// TotalCount 是时间窗口内的新闻总数（含分析跳过的记录）。
	// 正负面中性三项相加可能小于它，页面上「今日共 N 条」应显示这个值。
	TotalCount        int                            `json:"totalCount"`
	SentimentIndex    float64                        `json:"sentimentIndex"` // 综合情绪指数 0-100
	HotScoreAvg       float64                        `json:"hotScoreAvg"`    // 平均热度
	SentimentByType   map[NewsType]SentimentResult   `json:"sentimentByType"`
	SentimentBySource map[SourceType]SentimentResult `json:"sentimentBySource"`

	// totalHotScore 是流式累计的热度和，finalize 时除以样本数得到均分。
	totalHotScore int
}

// finalize 由累计结果算出指数、均分与各维度情绪，补齐三个 map。
func (r *MarketSentiment) finalize(
	typeStats map[NewsType]dimensionStats,
	sourceStats map[SourceType]dimensionStats,
) {
	r.SentimentByType = make(map[NewsType]SentimentResult, len(typeStats))
	r.SentimentBySource = make(map[SourceType]SentimentResult, len(sourceStats))

	total := r.PositiveCount + r.NegativeCount + r.NeutralCount
	if total > 0 {
		// 综合情绪指数：正面越多越高，负面越多越低，50 为中性。
		positiveRatio := float64(r.PositiveCount) / float64(total)
		negativeRatio := float64(r.NegativeCount) / float64(total)
		r.SentimentIndex = 50 + (positiveRatio-negativeRatio)*50
		r.HotScoreAvg = float64(r.totalHotScore) / float64(total)
	}

	for newsType, stats := range typeStats {
		if stats.total > 0 {
			score := float64(stats.pos-stats.neg) / float64(stats.total)
			r.SentimentByType[newsType] = SentimentResult{
				Type:       determineSentimentType(score),
				Score:      score,
				Confidence: float64(stats.total) / 50.0,
			}
		}
	}

	for source, stats := range sourceStats {
		if stats.total > 0 {
			score := float64(stats.pos-stats.neg) / float64(stats.total)
			r.SentimentBySource[source] = SentimentResult{
				Type:       determineSentimentType(score),
				Score:      score,
				Confidence: float64(stats.total) / 50.0,
			}
		}
	}
}

// dimensionStats 累计一个维度（新闻类型/来源）的正负面与总数。
type dimensionStats struct {
	pos, neg, neu, total int
}

// record 把一条新闻的分析结果累加到市场结果与两个维度统计里。
func (r *MarketSentiment) record(
	typeStats map[NewsType]dimensionStats,
	sourceStats map[SourceType]dimensionStats,
	item *NewsItem,
	sentiment SentimentResult,
) {
	switch sentiment.Type {
	case SentimentPositive:
		r.PositiveCount++
	case SentimentNegative:
		r.NegativeCount++
	default:
		r.NeutralCount++
	}
	r.totalHotScore += item.HotScore

	ts := typeStats[item.NewsType]
	ts.total++
	switch sentiment.Type {
	case SentimentPositive:
		ts.pos++
	case SentimentNegative:
		ts.neg++
	default:
		ts.neu++
	}
	typeStats[item.NewsType] = ts

	ss := sourceStats[item.Source]
	ss.total++
	switch sentiment.Type {
	case SentimentPositive:
		ss.pos++
	case SentimentNegative:
		ss.neg++
	default:
		ss.neu++
	}
	sourceStats[item.Source] = ss
}

// SentimentTrend 情绪趋势
type SentimentTrend struct {
	Time      time.Time `json:"time"`
	Sentiment float64   `json:"sentiment"` // 情绪指数
	NewsCount int       `json:"newsCount"` // 新闻数量
}

// SentimentHeatmapItem 情绪热力图项
type SentimentHeatmapItem struct {
	StockCode string          `json:"stockCode"`
	StockName string          `json:"stockName"`
	Sentiment SentimentResult `json:"sentiment"`
	NewsCount int             `json:"newsCount"`
	HotScore  int             `json:"hotScore"`
}

// SentimentService 情绪服务
type SentimentService struct {
	analyzer *SimpleSentimentAnalyzer
	store    Store
}

// NewSentimentService 创建情绪服务
func NewSentimentService(store Store) *SentimentService {
	return &SentimentService{
		analyzer: NewSimpleSentimentAnalyzer(),
		store:    store,
	}
}

// AnalyzeMarketSentiment 分析市场整体情绪
//
// 覆盖整个时间窗口：窗口内新闻逐条流式分析，不设条数上限。
// 早期的实现用分页查询取一批再分析，页大小封顶后「今日共 N 条」会
// 钉死在分页上限（例如 1000），占比也随之失真。
func (s *SentimentService) AnalyzeMarketSentiment(ctx context.Context, hours int) (*MarketSentiment, error) {
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(hours) * time.Hour)

	filter := FeedFilter{
		StartTime: &startTime,
		EndTime:   &endTime,
	}

	// 总数走独立 COUNT：遍历可能因个别记录缺失提前跳过，统计口径以库为准。
	totalCount, err := s.store.CountNews(ctx, filter)
	if err != nil {
		return nil, err
	}

	result := &MarketSentiment{Timestamp: endTime, TotalCount: totalCount}
	if totalCount == 0 {
		result.SentimentByType = make(map[NewsType]SentimentResult)
		result.SentimentBySource = make(map[SourceType]SentimentResult)
		return result, nil
	}

	typeStats := make(map[NewsType]dimensionStats)
	sourceStats := make(map[SourceType]dimensionStats)

	// 逐条流式分析，避免一次性载入整个窗口。
	err = s.store.IterNews(ctx, filter, func(item *NewsItem) error {
		result.record(typeStats, sourceStats, item, s.analyzer.AnalyzeNews(item))
		return nil
	})
	if err != nil {
		return nil, err
	}

	result.finalize(typeStats, sourceStats)
	return result, nil
}

// calculateSentiment 对一小批新闻计算情绪数据，供趋势、热力图、个股使用。
// 与市场级接口共用 record/finalize，保证各处口径一致。
func (s *SentimentService) calculateSentiment(newsItems []*NewsItem) *MarketSentiment {
	result := &MarketSentiment{Timestamp: time.Now()}
	typeStats := make(map[NewsType]dimensionStats)
	sourceStats := make(map[SourceType]dimensionStats)

	for _, item := range newsItems {
		result.record(typeStats, sourceStats, item, s.analyzer.AnalyzeNews(item))
	}
	result.TotalCount = len(newsItems)
	result.finalize(typeStats, sourceStats)
	return result
}

func determineSentimentType(score float64) SentimentType {
	if score > 0.15 {
		return SentimentPositive
	} else if score < -0.15 {
		return SentimentNegative
	}
	return SentimentNeutral
}

// GetSentimentTrend 获取情绪趋势
func (s *SentimentService) GetSentimentTrend(ctx context.Context, hours int, intervals int) ([]SentimentTrend, error) {
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(hours) * time.Hour)

	filter := FeedFilter{
		StartTime: &startTime,
		EndTime:   &endTime,
		PageSize:  2000,
	}

	result, err := s.store.FilterNews(ctx, filter)
	if err != nil {
		return nil, err
	}

	// 获取完整新闻内容
	var newsItems []*NewsItem
	for _, summary := range result.Items {
		if item, err := s.store.GetNewsByID(ctx, summary.ID); err == nil {
			newsItems = append(newsItems, item)
		}
	}

	// 按时间间隔分组
	intervalDuration := time.Duration(hours*60/intervals) * time.Minute
	trends := make([]SentimentTrend, 0, intervals)

	for i := 0; i < intervals; i++ {
		intervalStart := startTime.Add(time.Duration(i) * intervalDuration)
		intervalEnd := intervalStart.Add(intervalDuration)

		var intervalNews []*NewsItem
		for _, item := range newsItems {
			if item.PublishTime.After(intervalStart) && item.PublishTime.Before(intervalEnd) {
				intervalNews = append(intervalNews, item)
			}
		}

		// 计算该时间段的情绪
		sentiment := s.calculateSentiment(intervalNews)

		trends = append(trends, SentimentTrend{
			Time:      intervalStart,
			Sentiment: sentiment.SentimentIndex,
			NewsCount: len(intervalNews),
		})
	}

	return trends, nil
}

// GetSentimentHeatmap 获取情绪热力图数据
func (s *SentimentService) GetSentimentHeatmap(ctx context.Context, hours int, topN int) ([]SentimentHeatmapItem, error) {
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(hours) * time.Hour)

	filter := FeedFilter{
		StartTime: &startTime,
		EndTime:   &endTime,
		PageSize:  2000,
	}

	result, err := s.store.FilterNews(ctx, filter)
	if err != nil {
		return nil, err
	}

	// 获取完整新闻内容
	var newsItems []*NewsItem
	for _, summary := range result.Items {
		if item, err := s.store.GetNewsByID(ctx, summary.ID); err == nil {
			newsItems = append(newsItems, item)
		}
	}

	// 按股票分组统计
	stockStats := make(map[string]struct {
		stockName     string
		newsItems     []*NewsItem
		totalHotScore int
	})

	for _, item := range newsItems {
		for _, stock := range item.RelatedStocks {
			if stock == "" {
				continue
			}
			stats := stockStats[stock]
			if stats.stockName == "" && len(item.RelatedStocks) > 0 {
				stats.stockName = item.Title // 使用标题作为临时名称
			}
			stats.newsItems = append(stats.newsItems, item)
			stats.totalHotScore += item.HotScore
			stockStats[stock] = stats
		}
	}

	// 转换为热力图项
	var heatmapItems []SentimentHeatmapItem
	for stockCode, stats := range stockStats {
		if len(stats.newsItems) == 0 {
			continue
		}

		sentiment := s.calculateSentiment(stats.newsItems)

		heatmapItems = append(heatmapItems, SentimentHeatmapItem{
			StockCode: stockCode,
			StockName: stats.stockName,
			Sentiment: SentimentResult{
				Type:       determineSentimentType(sentiment.SentimentIndex/100*2 - 1),
				Score:      sentiment.SentimentIndex/100*2 - 1,
				Confidence: float64(len(stats.newsItems)) / 20.0,
			},
			NewsCount: len(stats.newsItems),
			HotScore:  stats.totalHotScore / len(stats.newsItems),
		})
	}

	// 按新闻数量排序，取前N
	sort.Slice(heatmapItems, func(i, j int) bool {
		return heatmapItems[i].NewsCount > heatmapItems[j].NewsCount
	})

	if len(heatmapItems) > topN {
		heatmapItems = heatmapItems[:topN]
	}

	return heatmapItems, nil
}

// GetStockSentiment 获取个股情绪
func (s *SentimentService) GetStockSentiment(ctx context.Context, stockCode string, hours int) (*MarketSentiment, error) {
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(hours) * time.Hour)

	filter := FeedFilter{
		RelatedStocks: []string{stockCode},
		StartTime:     &startTime,
		EndTime:       &endTime,
		PageSize:      100,
	}

	result, err := s.store.FilterNews(ctx, filter)
	if err != nil {
		return nil, err
	}

	var newsItems []*NewsItem
	for _, summary := range result.Items {
		if item, err := s.store.GetNewsByID(ctx, summary.ID); err == nil {
			newsItems = append(newsItems, item)
		}
	}

	return s.calculateSentiment(newsItems), nil
}
