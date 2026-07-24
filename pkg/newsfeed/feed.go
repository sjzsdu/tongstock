package newsfeed

import (
	"context"
	"time"
)

// Feed 接口定义单个数据源的抓取能力
type Feed interface {
	// Name 返回数据源名称
	Name() SourceType

	// Fetch 获取最新新闻列表
	Fetch(ctx context.Context) ([]*NewsItem, error)

	// FetchByStock 获取指定股票相关的新闻
	FetchByStock(ctx context.Context, code string) ([]*NewsItem, error)

	// FetchByKeyword 获取指定关键词相关的新闻
	FetchByKeyword(ctx context.Context, keyword string) ([]*NewsItem, error)

	// RefreshInterval 返回建议的刷新间隔
	RefreshInterval() time.Duration

	// HealthCheck 检查数据源健康状态
	HealthCheck(ctx context.Context) bool
}

// Aggregator 统一聚合器接口
type Aggregator interface {
	// RegisterFeed 注册一个数据源
	RegisterFeed(feed Feed)

	// UnregisterFeed 取消注册数据源
	UnregisterFeed(source SourceType)

	// GetFeeds 获取所有已注册的数据源
	GetFeeds() []Feed

	// FetchAll 从所有数据源获取新闻
	FetchAll(ctx context.Context) ([]*NewsItem, error)

	// FetchByStock 从所有数据源获取指定股票相关新闻
	FetchByStock(ctx context.Context, code string) ([]*NewsItem, error)

	// FetchByKeyword 从所有数据源获取指定关键词相关新闻
	FetchByKeyword(ctx context.Context, keyword string) ([]*NewsItem, error)

	// Filter 按条件筛选新闻
	Filter(ctx context.Context, filter FeedFilter) (*FeedResult, error)

	// GetHotEvents 获取热点事件列表
	GetHotEvents(ctx context.Context, filter HotEventFilter) (*EventResult, error)

	// GetHotEventDetail 获取热点事件详情
	GetHotEventDetail(ctx context.Context, eventID string) (*HotEvent, error)

	// SaveNews 保存新闻到存储层
	SaveNews(ctx context.Context, news []*NewsItem) error

	// SaveEvent 保存热点事件到存储层
	SaveEvent(ctx context.Context, event *HotEvent) error

	// StartBackgroundFetch 启动后台定时抓取
	StartBackgroundFetch(ctx context.Context)

	// StopBackgroundFetch 停止后台定时抓取
	StopBackgroundFetch()
}

// SimpleAggregator 简单聚合器实现
type SimpleAggregator struct {
	feeds       map[SourceType]Feed
	store       Store
	fetchTicker *time.Ticker
	cancelFunc  context.CancelFunc
}

// NewSimpleAggregator 创建简单聚合器
func NewSimpleAggregator(store Store) *SimpleAggregator {
	return &SimpleAggregator{
		feeds: make(map[SourceType]Feed),
		store: store,
	}
}

// RegisterFeed 注册数据源
func (a *SimpleAggregator) RegisterFeed(feed Feed) {
	a.feeds[feed.Name()] = feed
}

// UnregisterFeed 取消注册数据源
func (a *SimpleAggregator) UnregisterFeed(source SourceType) {
	delete(a.feeds, source)
}

// GetFeeds 获取所有已注册的数据源
func (a *SimpleAggregator) GetFeeds() []Feed {
	feeds := make([]Feed, 0, len(a.feeds))
	for _, feed := range a.feeds {
		feeds = append(feeds, feed)
	}
	return feeds
}

// FetchAll 从所有数据源获取新闻
func (a *SimpleAggregator) FetchAll(ctx context.Context) ([]*NewsItem, error) {
	var allNews []*NewsItem
	for _, feed := range a.feeds {
		if !feed.HealthCheck(ctx) {
			continue
		}
		news, err := feed.Fetch(ctx)
		if err != nil {
			continue
		}
		allNews = append(allNews, news...)
	}
	return allNews, nil
}

// FetchByStock 从所有数据源获取指定股票相关新闻
func (a *SimpleAggregator) FetchByStock(ctx context.Context, code string) ([]*NewsItem, error) {
	var allNews []*NewsItem
	for _, feed := range a.feeds {
		if !feed.HealthCheck(ctx) {
			continue
		}
		news, err := feed.FetchByStock(ctx, code)
		if err != nil {
			continue
		}
		allNews = append(allNews, news...)
	}
	return allNews, nil
}

// FetchByKeyword 从所有数据源获取指定关键词相关新闻
func (a *SimpleAggregator) FetchByKeyword(ctx context.Context, keyword string) ([]*NewsItem, error) {
	var allNews []*NewsItem
	for _, feed := range a.feeds {
		if !feed.HealthCheck(ctx) {
			continue
		}
		news, err := feed.FetchByKeyword(ctx, keyword)
		if err != nil {
			continue
		}
		allNews = append(allNews, news...)
	}
	return allNews, nil
}

// Filter 按条件筛选新闻
func (a *SimpleAggregator) Filter(ctx context.Context, filter FeedFilter) (*FeedResult, error) {
	if a.store == nil {
		return nil, ErrStoreNotSet
	}
	return a.store.FilterNews(ctx, filter)
}

// GetHotEvents 获取热点事件列表
func (a *SimpleAggregator) GetHotEvents(ctx context.Context, filter HotEventFilter) (*EventResult, error) {
	if a.store == nil {
		return nil, ErrStoreNotSet
	}
	return a.store.GetHotEvents(ctx, filter)
}

// GetHotEventDetail 获取热点事件详情
func (a *SimpleAggregator) GetHotEventDetail(ctx context.Context, eventID string) (*HotEvent, error) {
	if a.store == nil {
		return nil, ErrStoreNotSet
	}
	return a.store.GetHotEventDetail(ctx, eventID)
}

// SaveNews 保存新闻到存储层
func (a *SimpleAggregator) SaveNews(ctx context.Context, news []*NewsItem) error {
	if a.store == nil {
		return ErrStoreNotSet
	}
	return a.store.SaveNews(ctx, news)
}

// SaveEvent 保存热点事件到存储层
func (a *SimpleAggregator) SaveEvent(ctx context.Context, event *HotEvent) error {
	if a.store == nil {
		return ErrStoreNotSet
	}
	return a.store.SaveHotEvent(ctx, event)
}

// StartBackgroundFetch 启动后台定时抓取
func (a *SimpleAggregator) StartBackgroundFetch(ctx context.Context) {
	ctx, a.cancelFunc = context.WithCancel(ctx)
	a.fetchTicker = time.NewTicker(5 * time.Minute)

	go func() {
		for {
			select {
			case <-a.fetchTicker.C:
				news, _ := a.FetchAll(ctx)
				if len(news) > 0 {
					_ = a.SaveNews(ctx, news)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// StopBackgroundFetch 停止后台定时抓取
func (a *SimpleAggregator) StopBackgroundFetch() {
	if a.fetchTicker != nil {
		a.fetchTicker.Stop()
	}
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
}
