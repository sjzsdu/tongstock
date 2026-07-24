package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/newsfeed"
	"github.com/sjzsdu/tongstock/pkg/newsfeed/sources"
)

// NewsfeedHandler 新闻聚合API处理器
type NewsfeedHandler struct {
	aggregator   *newsfeed.SimpleAggregator
	clusterer    *newsfeed.Clusterer
	store        *newsfeed.SQLiteStore
	sentimentSvc *newsfeed.SentimentService
	alertSvc     *newsfeed.AlertService
}

// NewNewsfeedHandler 创建新闻聚合处理器
func NewNewsfeedHandler(store *newsfeed.SQLiteStore) *NewsfeedHandler {
	handler := &NewsfeedHandler{
		store: store,
	}

	// 创建聚合器
	handler.aggregator = newsfeed.NewSimpleAggregator(store)

	// 注册所有数据源
	for _, feed := range sources.NewAllSources() {
		handler.aggregator.RegisterFeed(feed)
	}

	// 创建聚类器
	handler.clusterer = newsfeed.NewClusterer(store, newsfeed.DefaultClusterConfig())

	// 创建情绪服务
	handler.sentimentSvc = newsfeed.NewSentimentService(store)

	// 创建预警服务
	handler.alertSvc = newsfeed.NewAlertService(store, handler.sentimentSvc)

	return handler
}

// SetupRoutes 设置新闻聚合相关路由
func (h *NewsfeedHandler) SetupRoutes(r *gin.RouterGroup) {
	news := r.Group("/news")
	{
		// 信息流
		news.GET("/feed", h.handleNewsFeed)
		news.GET("/feed/sources", h.handleNewsSources)
		news.GET("/item/:id", h.handleNewsItem)

		// 热点事件
		news.GET("/events", h.handleHotEvents)
		news.GET("/events/:id", h.handleHotEventDetail)
		news.POST("/events/refresh", h.handleRefreshHotEvents)

		// 个股关联资讯
		news.GET("/stock/:code", h.handleStockNews)

		// 搜索
		news.GET("/search", h.handleSearchNews)

		// 手动刷新数据源
		news.POST("/fetch", h.handleFetchNews)

		// 情绪分析
		news.GET("/sentiment/market", h.handleMarketSentiment)
		news.GET("/sentiment/trend", h.handleSentimentTrend)
		news.GET("/sentiment/heatmap", h.handleSentimentHeatmap)
		news.GET("/sentiment/stock/:code", h.handleStockSentiment)

		// 预警推送
		news.GET("/alerts", h.handleGetAlerts)
		news.GET("/alerts/unread", h.handleGetUnreadAlerts)
		news.GET("/alerts/count", h.handleGetUnreadCount)
		news.PUT("/alerts/:id/read", h.handleMarkAlertRead)
		news.PUT("/alerts/read-all", h.handleMarkAllAlertsRead)
		news.POST("/alerts/rule", h.handleAddAlertRule)
		news.GET("/alerts/rules", h.handleGetAlertRules)
		news.PUT("/alerts/rule/:id", h.handleUpdateAlertRule)
		news.DELETE("/alerts/rule/:id", h.handleDeleteAlertRule)
		news.POST("/alerts/watchlist", h.handleSetWatchlist)
	}
}

// handleNewsFeed 获取信息流
func (h *NewsfeedHandler) handleNewsFeed(c *gin.Context) {
	filter := newsfeed.FeedFilter{}

	// 来源筛选
	if sourcesStr := c.Query("sources"); sourcesStr != "" {
		for _, s := range strings.Split(sourcesStr, ",") {
			filter.Sources = append(filter.Sources, newsfeed.SourceType(strings.TrimSpace(s)))
		}
	}

	// 新闻类型筛选
	if typesStr := c.Query("types"); typesStr != "" {
		for _, t := range strings.Split(typesStr, ",") {
			filter.NewsTypes = append(filter.NewsTypes, newsfeed.NewsType(strings.TrimSpace(t)))
		}
	}

	// 时间范围
	if startStr := c.Query("startTime"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartTime = &t
		}
	}
	if endStr := c.Query("endTime"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndTime = &t
		}
	}

	// 热度筛选
	if hotStr := c.Query("hotScoreMin"); hotStr != "" {
		if h, err := strconv.Atoi(hotStr); err == nil {
			filter.HotScoreMin = h
		}
	}

	// 分页
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil {
			filter.PageNum = p
		}
	}
	if sizeStr := c.Query("pageSize"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil {
			filter.PageSize = s
		}
	}

	// 排序
	filter.SortBy = c.DefaultQuery("sortBy", "time")

	result, err := h.store.FilterNews(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleNewsSources 获取所有数据源
func (h *NewsfeedHandler) handleNewsSources(c *gin.Context) {
	var sources []gin.H
	for _, feed := range h.aggregator.GetFeeds() {
		sources = append(sources, gin.H{
			"name":     feed.Name(),
			"interval": feed.RefreshInterval().String(),
			"healthy":  feed.HealthCheck(c.Request.Context()),
		})
	}
	c.JSON(http.StatusOK, gin.H{"sources": sources})
}

// handleNewsItem 获取单条新闻详情
func (h *NewsfeedHandler) handleNewsItem(c *gin.Context) {
	id := c.Param("id")
	item, err := h.store.GetNewsByID(c.Request.Context(), id)
	if err != nil {
		if err == newsfeed.ErrNewsNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "新闻不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, item)
}

// handleHotEvents 获取热点事件列表
func (h *NewsfeedHandler) handleHotEvents(c *gin.Context) {
	filter := newsfeed.HotEventFilter{}

	// 最低热度
	if minHotStr := c.Query("minHotIndex"); minHotStr != "" {
		if m, err := strconv.Atoi(minHotStr); err == nil {
			filter.MinHotIndex = m
		}
	}

	// 状态筛选
	if statusStr := c.Query("status"); statusStr != "" {
		for _, s := range strings.Split(statusStr, ",") {
			filter.Status = append(filter.Status, newsfeed.EventStatus(strings.TrimSpace(s)))
		}
	}

	// 限制数量
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = l
		}
	}

	result, err := h.store.GetHotEvents(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleHotEventDetail 获取热点事件详情
func (h *NewsfeedHandler) handleHotEventDetail(c *gin.Context) {
	id := c.Param("id")
	event, err := h.store.GetHotEventDetail(c.Request.Context(), id)
	if err != nil {
		if err == newsfeed.ErrEventNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "事件不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// 获取关联新闻列表
	var newsItems []*newsfeed.NewsItem
	for _, newsID := range event.NewsItemIDs {
		if item, err := h.store.GetNewsByID(c.Request.Context(), newsID); err == nil {
			newsItems = append(newsItems, item)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"event":     event,
		"newsItems": newsItems,
	})
}

// handleRefreshHotEvents 刷新热点事件
func (h *NewsfeedHandler) handleRefreshHotEvents(c *gin.Context) {
	// 获取最近的新闻
	filter := newsfeed.FeedFilter{
		PageSize: 100,
		SortBy:   "time",
	}
	result, err := h.store.FilterNews(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 转换为完整新闻对象
	var news []*newsfeed.NewsItem
	for _, summary := range result.Items {
		if item, err := h.store.GetNewsByID(c.Request.Context(), summary.ID); err == nil {
			news = append(news, item)
		}
	}

	// 聚类生成热点事件
	events, err := h.clusterer.Cluster(c.Request.Context(), news)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 保存事件
	for _, event := range events {
		if err := h.store.SaveHotEvent(c.Request.Context(), event); err != nil {
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"count":  len(events),
		"events": events,
	})
}

// handleStockNews 获取个股关联资讯
func (h *NewsfeedHandler) handleStockNews(c *gin.Context) {
	code := c.Param("code")

	// 从存储中查询
	filter := newsfeed.FeedFilter{
		RelatedStocks: []string{code},
		PageSize:      20,
		SortBy:        "time",
	}

	result, err := h.store.FilterNews(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleSearchNews 搜索新闻
func (h *NewsfeedHandler) handleSearchNews(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keyword is required"})
		return
	}

	// 从存储中查询（简单实现：匹配标题和摘要）
	// 实际应用中可以使用全文搜索引擎
	filter := newsfeed.FeedFilter{
		PageSize: 20,
		SortBy:   "time",
	}

	result, err := h.store.FilterNews(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 简单过滤
	var filtered []newsfeed.NewsSummary
	for _, item := range result.Items {
		if strings.Contains(strings.ToLower(item.Title), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(item.Summary), strings.ToLower(keyword)) {
			filtered = append(filtered, item)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total": len(filtered),
		"items": filtered,
	})
}

// handleFetchNews 手动刷新数据源
func (h *NewsfeedHandler) handleFetchNews(c *gin.Context) {
	ctx := c.Request.Context()

	// 从所有数据源获取新闻
	news, err := h.aggregator.FetchAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 去重
	news = h.clusterer.DeduplicateNews(news)

	// 保存
	if err := h.aggregator.SaveNews(ctx, news); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count": len(news),
		"msg":   "新闻获取成功",
	})
}

// handleMarketSentiment 获取市场整体情绪
func (h *NewsfeedHandler) handleMarketSentiment(c *gin.Context) {
	hours := 24
	if hoursStr := c.Query("hours"); hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil {
			hours = h
		}
	}

	sentiment, err := h.sentimentSvc.AnalyzeMarketSentiment(c.Request.Context(), hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, sentiment)
}

// handleSentimentTrend 获取情绪趋势
func (h *NewsfeedHandler) handleSentimentTrend(c *gin.Context) {
	hours := 24
	if hoursStr := c.Query("hours"); hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil {
			hours = h
		}
	}

	intervals := 12
	if intervalsStr := c.Query("intervals"); intervalsStr != "" {
		if i, err := strconv.Atoi(intervalsStr); err == nil {
			intervals = i
		}
	}

	trend, err := h.sentimentSvc.GetSentimentTrend(c.Request.Context(), hours, intervals)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, trend)
}

// handleSentimentHeatmap 获取情绪热力图
func (h *NewsfeedHandler) handleSentimentHeatmap(c *gin.Context) {
	hours := 24
	if hoursStr := c.Query("hours"); hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil {
			hours = h
		}
	}

	topN := 20
	if topNStr := c.Query("topN"); topNStr != "" {
		if t, err := strconv.Atoi(topNStr); err == nil {
			topN = t
		}
	}

	heatmap, err := h.sentimentSvc.GetSentimentHeatmap(c.Request.Context(), hours, topN)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, heatmap)
}

// handleStockSentiment 获取个股情绪
func (h *NewsfeedHandler) handleStockSentiment(c *gin.Context) {
	code := c.Param("code")

	hours := 24
	if hoursStr := c.Query("hours"); hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil {
			hours = h
		}
	}

	sentiment, err := h.sentimentSvc.GetStockSentiment(c.Request.Context(), code, hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, sentiment)
}

// handleGetAlerts 获取预警记录
func (h *NewsfeedHandler) handleGetAlerts(c *gin.Context) {
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	read := false
	if readStr := c.Query("read"); readStr != "" {
		if r, err := strconv.ParseBool(readStr); err == nil {
			read = r
		}
	}

	alerts, err := h.alertSvc.GetAlertRecords(c.Request.Context(), limit, read)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, alerts)
}

// handleGetUnreadAlerts 获取未读预警
func (h *NewsfeedHandler) handleGetUnreadAlerts(c *gin.Context) {
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	alerts, err := h.alertSvc.GetAlertRecords(c.Request.Context(), limit, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, alerts)
}

// handleGetUnreadCount 获取未读预警数量
func (h *NewsfeedHandler) handleGetUnreadCount(c *gin.Context) {
	count, err := h.alertSvc.GetUnreadCount(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

// handleMarkAlertRead 标记预警为已读
func (h *NewsfeedHandler) handleMarkAlertRead(c *gin.Context) {
	id := c.Param("id")
	if err := h.alertSvc.MarkAlertAsRead(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"msg": "标记成功"})
}

// handleMarkAllAlertsRead 标记所有预警为已读
func (h *NewsfeedHandler) handleMarkAllAlertsRead(c *gin.Context) {
	if err := h.alertSvc.MarkAllAlertsAsRead(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"msg": "全部标记成功"})
}

// handleAddAlertRule 添加预警规则
func (h *NewsfeedHandler) handleAddAlertRule(c *gin.Context) {
	var rule newsfeed.AlertRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.alertSvc.AddRule(&rule)
	c.JSON(http.StatusOK, rule)
}

// handleGetAlertRules 获取预警规则列表
func (h *NewsfeedHandler) handleGetAlertRules(c *gin.Context) {
	rules := h.alertSvc.GetRules()
	c.JSON(http.StatusOK, rules)
}

// handleUpdateAlertRule 更新预警规则
func (h *NewsfeedHandler) handleUpdateAlertRule(c *gin.Context) {
	id := c.Param("id")
	var updates newsfeed.AlertRule
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if ok := h.alertSvc.UpdateRule(id, &updates); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"msg": "更新成功"})
}

// handleDeleteAlertRule 删除预警规则
func (h *NewsfeedHandler) handleDeleteAlertRule(c *gin.Context) {
	id := c.Param("id")
	h.alertSvc.RemoveRule(id)
	c.JSON(http.StatusOK, gin.H{"msg": "删除成功"})
}

// handleSetWatchlist 设置关注列表
func (h *NewsfeedHandler) handleSetWatchlist(c *gin.Context) {
	var req struct {
		StockCodes []string `json:"stockCodes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.alertSvc.SetWatchlist(req.StockCodes)
	c.JSON(http.StatusOK, gin.H{"msg": "关注列表已更新", "count": len(req.StockCodes)})
}
