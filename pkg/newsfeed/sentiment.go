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
	Timestamp         time.Time                      `json:"timestamp"`
	PositiveCount     int                            `json:"positiveCount"`
	NegativeCount     int                            `json:"negativeCount"`
	NeutralCount      int                            `json:"neutralCount"`
	SentimentIndex    float64                        `json:"sentimentIndex"` // 综合情绪指数 0-100
	HotScoreAvg       float64                        `json:"hotScoreAvg"`    // 平均热度
	SentimentByType   map[NewsType]SentimentResult   `json:"sentimentByType"`
	SentimentBySource map[SourceType]SentimentResult `json:"sentimentBySource"`
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
func (s *SentimentService) AnalyzeMarketSentiment(ctx context.Context, hours int) (*MarketSentiment, error) {
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(hours) * time.Hour)

	filter := FeedFilter{
		StartTime: &startTime,
		EndTime:   &endTime,
		PageSize:  1000,
	}

	result, err := s.store.FilterNews(ctx, filter)
	if err != nil {
		return nil, err
	}

	// 获取完整新闻内容用于分析
	var newsItems []*NewsItem
	for _, summary := range result.Items {
		if item, err := s.store.GetNewsByID(ctx, summary.ID); err == nil {
			newsItems = append(newsItems, item)
		}
	}

	return s.calculateSentiment(newsItems), nil
}

// calculateSentiment 计算情绪数据
func (s *SentimentService) calculateSentiment(newsItems []*NewsItem) *MarketSentiment {
	result := &MarketSentiment{
		Timestamp:         time.Now(),
		SentimentByType:   make(map[NewsType]SentimentResult),
		SentimentBySource: make(map[SourceType]SentimentResult),
	}

	typeStats := make(map[NewsType]struct{ pos, neg, neu, total int })
	sourceStats := make(map[SourceType]struct{ pos, neg, neu, total int })

	totalHotScore := 0

	for _, item := range newsItems {
		sentiment := s.analyzer.AnalyzeNews(item)

		switch sentiment.Type {
		case SentimentPositive:
			result.PositiveCount++
		case SentimentNegative:
			result.NegativeCount++
		case SentimentNeutral:
			result.NeutralCount++
		}

		totalHotScore += item.HotScore

		// 按类型统计
		ts := typeStats[item.NewsType]
		switch sentiment.Type {
		case SentimentPositive:
			ts.pos++
		case SentimentNegative:
			ts.neg++
		case SentimentNeutral:
			ts.neu++
		}
		ts.total++
		typeStats[item.NewsType] = ts

		// 按来源统计
		ss := sourceStats[item.Source]
		switch sentiment.Type {
		case SentimentPositive:
			ss.pos++
		case SentimentNegative:
			ss.neg++
		case SentimentNeutral:
			ss.neu++
		}
		ss.total++
		sourceStats[item.Source] = ss
	}

	total := result.PositiveCount + result.NegativeCount + result.NeutralCount
	if total > 0 {
		// 计算综合情绪指数 (正面比例 * 100)
		// 考虑负面新闻的影响，负面越多，指数越低
		positiveRatio := float64(result.PositiveCount) / float64(total)
		negativeRatio := float64(result.NegativeCount) / float64(total)
		result.SentimentIndex = 50 + (positiveRatio-negativeRatio)*50

		result.HotScoreAvg = float64(totalHotScore) / float64(total)
	}

	// 计算各类型情绪
	for newsType, stats := range typeStats {
		if stats.total > 0 {
			score := float64(stats.pos-stats.neg) / float64(stats.total)
			result.SentimentByType[newsType] = SentimentResult{
				Type:       determineSentimentType(score),
				Score:      score,
				Confidence: float64(stats.total) / 50.0,
			}
		}
	}

	// 计算各来源情绪
	for source, stats := range sourceStats {
		if stats.total > 0 {
			score := float64(stats.pos-stats.neg) / float64(stats.total)
			result.SentimentBySource[source] = SentimentResult{
				Type:       determineSentimentType(score),
				Score:      score,
				Confidence: float64(stats.total) / 50.0,
			}
		}
	}

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
