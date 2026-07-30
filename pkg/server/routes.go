package server

import "github.com/gin-gonic/gin"

// SetupRoutes is the HTTP composition adapter. Business route groups are kept
// separate so adding a capability does not grow one cross-domain registry.
func (s *Server) SetupRoutes(router *gin.Engine, apiMiddleware ...gin.HandlerFunc) {
	s.setupHealthRoutes(router)
	api := router.Group("/api", apiMiddleware...)
	s.registerMarketRoutes(api)
	s.registerAnalysisRoutes(api)
	s.registerPortfolioRoutes(api)
	s.registerSyncRoutes(api)
	s.registerSettingsRoutes(api)
	if s.newsfeedHandler != nil {
		s.newsfeedHandler.SetupRoutes(api)
	}
	s.SetupAgentRoutes(api)
	s.SetupParadigmRoutes(api)
	s.registerForwardRunRoutes(api)
	s.registerMonitoringRoutes(api)
	s.registerReviewRoutes(api)
}

func (s *Server) registerMarketRoutes(api *gin.RouterGroup) {
	api.GET("/quote", s.handleQuote)
	api.GET("/quotes", s.handleQuotes)
	api.GET("/codes", s.handleCodes)
	api.GET("/codes/list", s.handleCodesList)
	api.GET("/codes/market", s.handleCodesMarket)
	api.GET("/codes/marketcap", s.handleCodesWithMarketCap)
	api.GET("/codes/stats", s.handleCodesStats)
	api.GET("/kline", s.handleKline)
	api.GET("/index", s.handleIndex)
	api.GET("/minute", s.handleMinute)
	api.GET("/trade", s.handleTrade)
	api.GET("/auction", s.handleAuction)
	api.GET("/xdxr", s.handleXdXr)
	api.GET("/finance", s.handleFinance)
	api.GET("/finance/trends", s.handleFinanceTrends)
	api.GET("/finance/metrics", s.handleFinanceMetrics)
	api.GET("/company", s.handleCompany)
	api.GET("/company/content", s.handleCompanyContent)
	api.GET("/block", s.handleBlock)
	api.GET("/block/files", s.handleBlockFiles)
	api.GET("/block/list", s.handleBlockList)
	api.GET("/block/show", s.handleBlockShow)
	api.GET("/count", s.handleCount)
	api.GET("/stocks/search", s.handleStockSearch)
	api.GET("/stocks/search-index", s.handleStockSearchIndex)
	api.GET("/stockinfo", s.handleStockinfoList)
	api.GET("/stockinfo/:code", s.handleStockinfoGet)
	api.POST("/stockinfo/sync", s.handleStockinfoSync)
	api.GET("/stockinfo/count", s.handleStockinfoCount)
}

func (s *Server) registerAnalysisRoutes(api *gin.RouterGroup) {
	api.GET("/indicator", s.handleIndicator)
	api.GET("/screen", s.handleScreen)
	api.GET("/signal-analysis", s.handleSignalAnalysis)
	api.GET("/stock/compare", s.handleStockCompare)
	api.POST("/strategy/overnight", s.handleOvernightArbitrage)
}

func (s *Server) registerPortfolioRoutes(api *gin.RouterGroup) {
	api.GET("/history", s.handleHistoryList)
	api.POST("/history", s.handleHistoryAdd)
	api.DELETE("/history/:code", s.handleHistoryDelete)
	api.GET("/watchlist", s.handleWatchlistList)
	api.POST("/watchlist", s.handleWatchlistAdd)
	api.DELETE("/watchlist/:code", s.handleWatchlistDelete)
	api.PUT("/watchlist/:code/note", s.handleWatchlistUpdateNote)
	api.PUT("/watchlist/:code/group", s.handleWatchlistUpdateGroup)
	api.GET("/watchlist/groups", s.handleWatchlistGroups)
	api.GET("/stockpool", s.handleStockpoolList)
	api.POST("/stockpool", s.handleStockpoolUpsert)
	api.DELETE("/stockpool/:id", s.handleStockpoolDelete)
	api.POST("/trades", s.handleTradeCreate)
	api.GET("/trades", s.handleTradeList)
	api.GET("/trades/positions", s.handleTradePositions)
	api.DELETE("/trades/:id", s.handleTradeDelete)
}

func (s *Server) registerSyncRoutes(api *gin.RouterGroup) {
	api.POST("/sync/daily", s.handleSyncDaily)
	api.GET("/sync/state", s.handleSyncState)
	api.GET("/sync/freshness", s.handleSyncFreshness)
	api.POST("/kline/clean", s.handleCleanKlines)
}

func (s *Server) registerSettingsRoutes(api *gin.RouterGroup) {
	api.GET("/settings/indicator", s.handleIndicatorSettings)
	api.PUT("/settings/indicator", s.handleSaveIndicatorSettings)
}
