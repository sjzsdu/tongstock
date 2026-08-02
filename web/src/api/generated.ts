// Generated from api/openapi.json. Do not edit by hand.

export interface APIError {
  "code": string;
  "details"?: unknown;
  "message": string;
  "request_id"?: string;
}

export interface AgentStreamError {
  "code": string;
  "message": string;
  "request_id"?: string;
  "type": "error";
}

export interface ErrorEnvelope {
  "error": APIError;
}

export interface Finance {
  "GuDongRenShu"?: number;
  "JingLiRun"?: number;
  "JingZiChan"?: number;
  "LiuTongGuBen"?: number;
  "MeiGuJingZiChan"?: number;
  "ZhuYingShouRu"?: number;
  "ZongGuBen"?: number;
  "ZongZiChan"?: number;
}

export interface Kline {
  "Amount": number;
  "Close": number;
  "High": number;
  "Low": number;
  "Open": number;
  "Time": string;
  "Volume": number;
}

export interface Quote {
  "Amount"?: number;
  "Code": string;
  "High"?: number;
  "LastClose"?: number;
  "Low"?: number;
  "Name"?: string;
  "Open"?: number;
  "Price": number;
  "Volume"?: number;
}

export const operations = {
  postApiAgentChat: { method: "POST", path: "/api/agent/chat" },
  postApiAgentResearch: { method: "POST", path: "/api/agent/research" },
  getApiAgentChatSessionList: { method: "GET", path: "/api/agent/chat/session/list" },
  postApiAgentChatSessionSave: { method: "POST", path: "/api/agent/chat/session/save" },
  getApiAgentChatSessionId: { method: "GET", path: "/api/agent/chat/session/{id}" },
  streamAgentChat: { method: "POST", path: "/api/agent/chat/stream" },
  postApiAgentDebate: { method: "POST", path: "/api/agent/debate" },
  getApiAgentDiagnose: { method: "GET", path: "/api/agent/diagnose" },
  getApiAgentSessions: { method: "GET", path: "/api/agent/sessions" },
  getApiAgentState: { method: "GET", path: "/api/agent/state" },
  getApiAgentTranscript: { method: "GET", path: "/api/agent/transcript" },
  getApiAuction: { method: "GET", path: "/api/auction" },
  getApiBlock: { method: "GET", path: "/api/block" },
  getApiBlockFiles: { method: "GET", path: "/api/block/files" },
  getApiBlockList: { method: "GET", path: "/api/block/list" },
  getApiBlockShow: { method: "GET", path: "/api/block/show" },
  getApiCodes: { method: "GET", path: "/api/codes" },
  getApiCodesList: { method: "GET", path: "/api/codes/list" },
  getApiCodesMarket: { method: "GET", path: "/api/codes/market" },
  getApiCodesMarketcap: { method: "GET", path: "/api/codes/marketcap" },
  getApiCodesStats: { method: "GET", path: "/api/codes/stats" },
  getApiCompany: { method: "GET", path: "/api/company" },
  getApiCompanyContent: { method: "GET", path: "/api/company/content" },
  getApiCount: { method: "GET", path: "/api/count" },
  getFinance: { method: "GET", path: "/api/finance" },
  getApiFinanceMetrics: { method: "GET", path: "/api/finance/metrics" },
  getApiFinanceTrends: { method: "GET", path: "/api/finance/trends" },
  getApiHistory: { method: "GET", path: "/api/history" },
  postApiHistory: { method: "POST", path: "/api/history" },
  deleteApiHistoryCode: { method: "DELETE", path: "/api/history/{code}" },
  getApiIndex: { method: "GET", path: "/api/index" },
  getApiIndicator: { method: "GET", path: "/api/indicator" },
  getKline: { method: "GET", path: "/api/kline" },
  postApiKlineClean: { method: "POST", path: "/api/kline/clean" },
  getApiMinute: { method: "GET", path: "/api/minute" },
  getApiNewsAlerts: { method: "GET", path: "/api/news/alerts" },
  getApiNewsAlertsCount: { method: "GET", path: "/api/news/alerts/count" },
  putApiNewsAlertsReadAll: { method: "PUT", path: "/api/news/alerts/read-all" },
  postApiNewsAlertsRule: { method: "POST", path: "/api/news/alerts/rule" },
  deleteApiNewsAlertsRuleId: { method: "DELETE", path: "/api/news/alerts/rule/{id}" },
  putApiNewsAlertsRuleId: { method: "PUT", path: "/api/news/alerts/rule/{id}" },
  getApiNewsAlertsRules: { method: "GET", path: "/api/news/alerts/rules" },
  getApiNewsAlertsUnread: { method: "GET", path: "/api/news/alerts/unread" },
  postApiNewsAlertsWatchlist: { method: "POST", path: "/api/news/alerts/watchlist" },
  putApiNewsAlertsIdRead: { method: "PUT", path: "/api/news/alerts/{id}/read" },
  getApiNewsEvents: { method: "GET", path: "/api/news/events" },
  postApiNewsEventsRefresh: { method: "POST", path: "/api/news/events/refresh" },
  getApiNewsEventsId: { method: "GET", path: "/api/news/events/{id}" },
  getApiNewsFeed: { method: "GET", path: "/api/news/feed" },
  getApiNewsFeedSources: { method: "GET", path: "/api/news/feed/sources" },
  postApiNewsFetch: { method: "POST", path: "/api/news/fetch" },
  postApiNewsFetchBrowser: { method: "POST", path: "/api/news/fetch/browser" },
  getApiNewsItemId: { method: "GET", path: "/api/news/item/{id}" },
  getApiNewsSearch: { method: "GET", path: "/api/news/search" },
  getApiNewsSentimentHeatmap: { method: "GET", path: "/api/news/sentiment/heatmap" },
  getApiNewsSentimentMarket: { method: "GET", path: "/api/news/sentiment/market" },
  getApiNewsSentimentStockCode: { method: "GET", path: "/api/news/sentiment/stock/{code}" },
  getApiNewsSentimentTrend: { method: "GET", path: "/api/news/sentiment/trend" },
  getApiNewsStockCode: { method: "GET", path: "/api/news/stock/{code}" },
  getApiParadigmAlerts: { method: "GET", path: "/api/paradigm/alerts" },
  postApiParadigmAnalyze: { method: "POST", path: "/api/paradigm/analyze" },
  postApiParadigmBacktest: { method: "POST", path: "/api/paradigm/backtest" },
  postApiParadigmEvaluate: { method: "POST", path: "/api/paradigm/evaluate" },
  getApiParadigmList: { method: "GET", path: "/api/paradigm/list" },
  getApiParadigmStats: { method: "GET", path: "/api/paradigm/stats" },
  getApiParadigmStockCode: { method: "GET", path: "/api/paradigm/stock/{code}" },
  deleteApiParadigmId: { method: "DELETE", path: "/api/paradigm/{id}" },
  putApiParadigmIdReview: { method: "PUT", path: "/api/paradigm/{id}/review" },
  getQuote: { method: "GET", path: "/api/quote" },
  getApiQuotes: { method: "GET", path: "/api/quotes" },
  getApiScreen: { method: "GET", path: "/api/screen" },
  getApiSettingsIndicator: { method: "GET", path: "/api/settings/indicator" },
  putApiSettingsIndicator: { method: "PUT", path: "/api/settings/indicator" },
  getApiSignalAnalysis: { method: "GET", path: "/api/signal-analysis" },
  getApiStockCompare: { method: "GET", path: "/api/stock/compare" },
  getApiStockinfo: { method: "GET", path: "/api/stockinfo" },
  getApiStockinfoCount: { method: "GET", path: "/api/stockinfo/count" },
  postApiStockinfoSync: { method: "POST", path: "/api/stockinfo/sync" },
  getApiStockinfoCode: { method: "GET", path: "/api/stockinfo/{code}" },
  getApiStockpool: { method: "GET", path: "/api/stockpool" },
  postApiStockpool: { method: "POST", path: "/api/stockpool" },
  deleteApiStockpoolId: { method: "DELETE", path: "/api/stockpool/{id}" },
  getApiStocksSearch: { method: "GET", path: "/api/stocks/search" },
  getApiStocksSearchIndex: { method: "GET", path: "/api/stocks/search-index" },
  postApiStrategyOvernight: { method: "POST", path: "/api/strategy/overnight" },
  postApiSyncDaily: { method: "POST", path: "/api/sync/daily" },
  getApiSyncFreshness: { method: "GET", path: "/api/sync/freshness" },
  getApiSyncState: { method: "GET", path: "/api/sync/state" },
  getApiTrade: { method: "GET", path: "/api/trade" },
  getApiTrades: { method: "GET", path: "/api/trades" },
  postApiTrades: { method: "POST", path: "/api/trades" },
  getApiTradesPositions: { method: "GET", path: "/api/trades/positions" },
  deleteApiTradesId: { method: "DELETE", path: "/api/trades/{id}" },
  getApiWatchlist: { method: "GET", path: "/api/watchlist" },
  postApiWatchlist: { method: "POST", path: "/api/watchlist" },
  getApiWatchlistGroups: { method: "GET", path: "/api/watchlist/groups" },
  deleteApiWatchlistCode: { method: "DELETE", path: "/api/watchlist/{code}" },
  putApiWatchlistCodeGroup: { method: "PUT", path: "/api/watchlist/{code}/group" },
  putApiWatchlistCodeNote: { method: "PUT", path: "/api/watchlist/{code}/note" },
  getApiXdxr: { method: "GET", path: "/api/xdxr" },
  getHealth: { method: "GET", path: "/health" },
  getDataSyncDiagnostics: { method: "GET", path: "/health/data-sync" },
  getDiagnostics: { method: "GET", path: "/health/diagnostics" },
  getLiveness: { method: "GET", path: "/health/live" },
  getReadiness: { method: "GET", path: "/health/ready" },
  getApiForwardRuns: { method: "GET", path: "/api/forward/runs" },
  postApiForwardRuns: { method: "POST", path: "/api/forward/runs" },
  getApiForwardRunsId: { method: "GET", path: "/api/forward/runs/{id}" },
  getApiForwardRunsIdSignals: { method: "GET", path: "/api/forward/runs/{id}/signals" },
  getApiForwardRunsIdEquity: { method: "GET", path: "/api/forward/runs/{id}/equity" },
  getApiForwardRunsIdCompare: { method: "GET", path: "/api/forward/runs/{id}/compare" },
  postApiForwardRunsIdExecute: { method: "POST", path: "/api/forward/runs/{id}/execute" },
  postApiForwardRunsIdFinalize: { method: "POST", path: "/api/forward/runs/{id}/finalize" },
  getApiForwardSignals: { method: "GET", path: "/api/forward/signals" },
  getApiForwardSignalsId: { method: "GET", path: "/api/forward/signals/{id}" },
  getApiMethods: { method: "GET", path: "/api/methods" },
  getApiMethodsId: { method: "GET", path: "/api/methods/{id}" },
  getApiMethodsIdAudit: { method: "GET", path: "/api/methods/{id}/audit" },
  getApiMethodFamiliesId: { method: "GET", path: "/api/method-families/{id}" },
  getApiMonitoringReport: { method: "GET", path: "/api/monitoring/report" },
  getApiMonitoringAlerts: { method: "GET", path: "/api/monitoring/alerts" },
  getApiMonitoringConfig: { method: "GET", path: "/api/monitoring/config" },
  postApiPositionDecisionsRun: { method: "POST", path: "/api/position-decisions/run" },
  getApiPositionDecisionsToday: { method: "GET", path: "/api/position-decisions/today" },
  getApiPositionDecisionRun: { method: "GET", path: "/api/position-decisions/runs/{id}" },
  getApiSelectionsToday: { method: "GET", path: "/api/selections/today" },
  getApiSelectionRuns: { method: "GET", path: "/api/selections/runs" },
  getApiSelectionRun: { method: "GET", path: "/api/selections/runs/{id}" },
  getApiMonitoringHealth: { method: "GET", path: "/api/monitoring/health" },
  postApiMonitoringAlertsIdAck: { method: "POST", path: "/api/monitoring/alerts/{id}/ack" },
  postApiMonitoringAlertsIdResolve: { method: "POST", path: "/api/monitoring/alerts/{id}/resolve" },
} as const;
