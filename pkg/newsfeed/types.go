package newsfeed

import (
	"time"
)

// SourceType 定义数据源类型
type SourceType string

const (
	SourceCaiLianShe SourceType = "财联社"
	SourceXueQiu     SourceType = "雪球"
	SourceJuChao     SourceType = "巨潮资讯"
	SourceEastMoney  SourceType = "东方财富"
	SourceUnknown    SourceType = "未知"
)

// NewsType 定义新闻类型
type NewsType string

const (
	NewsTypeFlash        NewsType = "快讯"
	NewsTypeAnnouncement NewsType = "公告"
	NewsTypeDiscussion   NewsType = "讨论"
	NewsTypeReport       NewsType = "研报"
	NewsTypeOther        NewsType = "其他"
)

// NewsItem 统一新闻数据结构
type NewsItem struct {
	ID          string     `json:"id"`
	Source      SourceType `json:"source"`
	NewsType    NewsType   `json:"newsType"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	Content     string     `json:"content"`
	PublishTime time.Time  `json:"publishTime"`
	HotScore    int        `json:"hotScore"`
	Tags        []string   `json:"tags"`
	// StockRefs 是本条新闻与股票的关联，带来源与置信度。
	// 保存时会写入 news_stock_ref；RelatedStocks 只是它的扁平投影，
	// 供旧代码兼容，不应作为关联判断依据。
	StockRefs     []StockRef `json:"stockRefs,omitempty"`
	RelatedStocks []string   `json:"relatedStocks"`
	URL           string     `json:"url"`
	OriginalID    string     `json:"originalId"` // 原始平台ID，用于去重
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// HotEvent 热点事件
type HotEvent struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Keywords      []string       `json:"keywords"`
	RelatedStocks []string       `json:"relatedStocks"`
	HotIndex      int            `json:"hotIndex"`     // 热度指数 1-100
	SourceCounts  map[string]int `json:"sourceCounts"` // 各平台提及次数
	NewsItemIDs   []string       `json:"newsItemIds"`  // 关联新闻ID列表
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	Status        EventStatus    `json:"status"`
}

// EventStatus 事件状态
type EventStatus string

const (
	EventStatusActive  EventStatus = "active"
	EventStatusCooling EventStatus = "cooling"
	EventStatusEnded   EventStatus = "ended"
)

// FeedFilter 信息流筛选条件
type FeedFilter struct {
	Sources       []SourceType `json:"sources"`
	NewsTypes     []NewsType   `json:"newsTypes"`
	Keywords      []string     `json:"keywords"`
	RelatedStocks []string     `json:"relatedStocks"`
	// MinConfidence 过滤关联置信度下限 [0,1]。仅对 RelatedStocks 生效：
	// 0 表示不过滤。正文偶然提及的关联置信度较低，可用此值剔除。
	MinConfidence float64    `json:"minConfidence"`
	StartTime     *time.Time `json:"startTime"`
	EndTime       *time.Time `json:"endTime"`
	HotScoreMin   int        `json:"hotScoreMin"`
	PageSize      int        `json:"pageSize"`
	PageNum       int        `json:"pageNum"`
	SortBy        string     `json:"sortBy"` // time | hot
}

// HotEventFilter 热点事件筛选条件
type HotEventFilter struct {
	Keywords      []string      `json:"keywords"`
	RelatedStocks []string      `json:"relatedStocks"`
	MinHotIndex   int           `json:"minHotIndex"`
	Status        []EventStatus `json:"status"`
	Limit         int           `json:"limit"`
}

// NewsSummary 新闻摘要（用于列表展示）
type NewsSummary struct {
	ID            string     `json:"id"`
	Source        SourceType `json:"source"`
	NewsType      NewsType   `json:"newsType"`
	Title         string     `json:"title"`
	Summary       string     `json:"summary"`
	PublishTime   time.Time  `json:"publishTime"`
	HotScore      int        `json:"hotScore"`
	Tags          []string   `json:"tags"`
	StockRefs     []StockRef `json:"stockRefs,omitempty"`
	RelatedStocks []string   `json:"relatedStocks"`
	URL           string     `json:"url"`
}

// EventSummary 事件摘要（用于列表展示）
type EventSummary struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Keywords      []string       `json:"keywords"`
	RelatedStocks []string       `json:"relatedStocks"`
	HotIndex      int            `json:"hotIndex"`
	SourceCounts  map[string]int `json:"sourceCounts"`
	NewsCount     int            `json:"newsCount"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	Status        EventStatus    `json:"status"`
}

// FeedResult 信息流查询结果
type FeedResult struct {
	Total    int           `json:"total"`
	Items    []NewsSummary `json:"items"`
	PageNum  int           `json:"pageNum"`
	PageSize int           `json:"pageSize"`
}

// EventResult 热点事件查询结果
type EventResult struct {
	Total int            `json:"total"`
	Items []EventSummary `json:"items"`
}
