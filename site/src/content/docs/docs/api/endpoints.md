---
title: 端点参考
description: TongStock 全部 HTTP 路由，由 api/openapi.json 生成，请勿手工编辑。
---

本页由 `site/scripts/gen-api-docs.mjs` 从仓库里的 `api/openapi.json` 生成，**不要手工编辑**。
契约的权威定义、以及它与 Gin 实际路由的双向比对，都在仓库里完成；本页只是它的可读视图。

- 路由总数：**159**（含 health 探针）
- 生成来源：`api/openapi.json`
- 参数列：带 `?` 表示可选；`(query)` / `(path)` 表示参数位置
- 请求列：`是` 表示该端点接受 JSON 请求体

## 健康检查

共 5 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| GET | `/health` | getHealth | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/health/data-sync` | getDataSyncDiagnostics | — | — | 200 |
| GET | `/health/diagnostics` | getDiagnostics | — | — | 200 |
| GET | `/health/live` | getLiveness | — | — | 200 |
| GET | `/health/ready` | getReadiness | — | — | 200 / 503 |

## 行情与基础数据

共 33 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/auction` | getApiAuction | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/block` | getApiBlock | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/block/files` | getApiBlockFiles | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/block/list` | getApiBlockList | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/block/show` | getApiBlockShow | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/codes` | getApiCodes | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/codes/list` | getApiCodesList | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/codes/market` | getApiCodesMarket | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/codes/marketcap` | getApiCodesMarketcap | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/codes/stats` | getApiCodesStats | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/count` | getApiCount | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/history` | getApiHistory | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/history` | postApiHistory | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| DELETE | `/api/history/{code}` | deleteApiHistoryCode | `code`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/index` | getApiIndex | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/quote` | getQuote | `code`(query) · `consistency?`(query) · `refresh?`(query) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/quotes` | getApiQuotes | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/stockinfo` | getApiStockinfo | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/stockinfo/{code}` | getApiStockinfoCode | `code`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/stockinfo/count` | getApiStockinfoCount | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/stockinfo/sync` | postApiStockinfoSync | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/stockpool` | getApiStockpool | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/stockpool` | postApiStockpool | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| DELETE | `/api/stockpool/{id}` | deleteApiStockpoolId | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/stocks/search` | getApiStocksSearch | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/stocks/search-index` | getApiStocksSearchIndex | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/watchlist` | getApiWatchlist | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/watchlist` | postApiWatchlist | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| DELETE | `/api/watchlist/{code}` | deleteApiWatchlistCode | `code`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| PUT | `/api/watchlist/{code}/group` | putApiWatchlistCodeGroup | `code`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| PUT | `/api/watchlist/{code}/note` | putApiWatchlistCodeNote | `code`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| DELETE | `/api/watchlist/group` | deleteApiWatchlistGroup | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/watchlist/groups` | getApiWatchlistGroups | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |

## K 线、分时与分笔

共 9 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/kline` | getKline | `code`(query) · `type?`(query) · `consistency?`(query) · `refresh?`(query) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/kline/clean` | postApiKlineClean | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/minute` | getApiMinute | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/trade` | getApiTrade | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/trades` | getApiTrades | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/trades` | postApiTrades | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| DELETE | `/api/trades/{id}` | deleteApiTradesId | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/trades/positions` | getApiTradesPositions | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/xdxr` | getApiXdxr | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |

## 指标与信号筛选

共 8 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| POST | `/api/discover/run` | postApiDiscoverRun | — | 是 | 200 / 400 / 422 / 500 / 503 |
| GET | `/api/discover/traces` | getApiDiscoverTraces | `limit?`(query) | — | 200 / 500 / 503 |
| GET | `/api/factors/picks/last` | getApiFactorsPicksLast | — | — | 200 / 500 |
| GET | `/api/factors/research/last` | getApiFactorsResearchLast | — | — | 200 / 500 |
| POST | `/api/factors/research/run` | postApiFactorsResearchRun | — | 是 | 200 / 400 / 500 / 503 |
| GET | `/api/indicator` | getApiIndicator | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/screen` | getApiScreen | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/signal-analysis` | getApiSignalAnalysis | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |

## 财务与公司

共 5 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/company` | getApiCompany | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/company/content` | getApiCompanyContent | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/finance` | getFinance | `code`(query) · `consistency?`(query) · `refresh?`(query) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/finance/metrics` | getApiFinanceMetrics | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/finance/trends` | getApiFinanceTrends | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |

## 方法与选股

共 23 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/automation/jobs` | getApiAutomationJobs | — | — | 200 / 500 / 503 |
| GET | `/api/automation/outbox` | getApiAutomationOutbox | — | — | 200 / 500 / 503 |
| POST | `/api/automation/run` | postApiAutomationRun | — | 是 | 200 / 422 / 500 / 503 |
| GET | `/api/dashboard/today` | getApiDashboardToday | — | — | 200 / 500 / 503 |
| GET | `/api/method-families/{id}` | getApiMethodFamiliesId | `id`(path) | — | 200 / 401 / 500 |
| GET | `/api/methods` | getApiMethods | `status?`(query) · `market?`(query) · `universe?`(query) · `holding_min_days?`(query) · `holding_max_days?`(query) · `family_id?`(query) · `limit?`(query) | — | 200 / 400 / 401 / 500 |
| DELETE | `/api/methods/{id}` | deleteApiMethodsId | `id`(path) · `confirm`(query) | — | 200 / 400 / 404 / 409 / 500 / 503 |
| GET | `/api/methods/{id}` | getApiMethodsId | `id`(path) | — | 200 / 401 / 404 / 500 |
| GET | `/api/methods/{id}/audit` | getApiMethodsIdAudit | `id`(path) | — | 200 / 401 / 500 |
| POST | `/api/methods/{id}/feedback` | postApiMethodsFeedback | — | 是 | 200 / 400 / 404 / 500 / 503 |
| GET | `/api/methods/{id}/pool` | getApiMethodsIdPool | `id`(path) · `market_snapshot_id?`(query) | — | 200 / 400 / 404 / 409 / 500 / 503 |
| GET | `/api/methods/forward-health` | getApiMethodsForwardHealth | — | — | 200 / 500 / 503 |
| GET | `/api/methods/reject-stats` | getApiMethodsRejectStats | — | — | 200 / 500 / 503 |
| GET | `/api/methods/research/last` | getApiMethodsResearchLast | — | — | 200 / 500 |
| POST | `/api/methods/research/run` | postApiMethodsResearchRun | — | 是 | 200 / 409 / 500 / 503 |
| GET | `/api/methods/research/status` | getApiMethodsResearchStatus | — | — | 200 / 500 |
| GET | `/api/methods/research/traces/{id}/summary` | getApiMethodsResearchTracesSummary | `id`(path) | — | 200 / 404 / 500 |
| POST | `/api/methods/seed` | postApiMethodsSeed | — | 是 | 200 / 400 / 409 / 500 / 503 |
| POST | `/api/onboarding/run` | postApiOnboardingRun | — | 是 | 200 / 400 / 500 / 503 |
| POST | `/api/selections/run` | postApiSelectionsRun | — | 是 | 200 / 400 / 409 / 422 / 500 / 503 |
| GET | `/api/selections/runs` | getApiSelectionRuns | `date?`(query) · `method_id?`(query) · `limit?`(query) | — | 200 / 500 / 503 |
| GET | `/api/selections/runs/{id}` | getApiSelectionRun | `id`(path) · `method_id?`(query) | — | 200 / 404 / 500 / 503 |
| GET | `/api/selections/today` | getApiSelectionsToday | `method_id?`(query) | — | 200 / 404 / 500 / 503 |

## 范式、监控与前向跟踪

共 29 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/forward/runs` | getApiForwardRuns | — | — | 200 / 400 / 401 / 404 / 500 |
| POST | `/api/forward/runs` | postApiForwardRuns | — | 是 | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/forward/runs/{id}` | getApiForwardRunsId | `id`(path) | — | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/forward/runs/{id}/compare` | getApiForwardRunsIdCompare | `id`(path) | — | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/forward/runs/{id}/equity` | getApiForwardRunsIdEquity | `id`(path) | — | 200 / 400 / 401 / 404 / 500 |
| POST | `/api/forward/runs/{id}/execute` | postApiForwardRunsIdExecute | `id`(path) | 是 | 200 / 400 / 401 / 404 / 500 |
| POST | `/api/forward/runs/{id}/finalize` | postApiForwardRunsIdFinalize | `id`(path) | 是 | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/forward/runs/{id}/signals` | getApiForwardRunsIdSignals | `id`(path) | — | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/forward/signals` | getApiForwardSignals | — | — | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/forward/signals/{id}` | getApiForwardSignalsId | `id`(path) | — | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/monitoring/alerts` | getApiMonitoringAlerts | — | — | 200 / 400 / 401 / 404 / 500 |
| POST | `/api/monitoring/alerts/{id}/ack` | postApiMonitoringAlertsIdAck | `id`(path) | 是 | 200 / 400 / 401 / 404 / 500 |
| POST | `/api/monitoring/alerts/{id}/resolve` | postApiMonitoringAlertsIdResolve | `id`(path) | 是 | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/monitoring/config` | getApiMonitoringConfig | — | — | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/monitoring/health` | getApiMonitoringHealth | — | — | 200 / 400 / 401 / 404 / 500 |
| GET | `/api/monitoring/report` | getApiMonitoringReport | — | — | 200 / 400 / 401 / 404 / 500 |
| POST | `/api/monitoring/report/refresh` | postApiMonitoringReportRefresh | — | — | 200 / 400 / 401 / 404 / 500 |
| DELETE | `/api/paradigm/{id}` | deleteApiParadigmId | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/paradigm/{id}/evidence` | getApiParadigmIdEvidence | `id`(path) | — | 200 / 401 / 404 / 500 / 503 |
| POST | `/api/paradigm/{id}/promote` | postApiParadigmIdPromote | `id`(path) | 是 | 200 / 401 / 404 / 409 / 422 / 500 / 503 |
| GET | `/api/paradigm/{id}/promotion/status` | getApiParadigmIdPromotionStatus | `id`(path) | — | 200 / 401 / 500 / 503 |
| PUT | `/api/paradigm/{id}/review` | putApiParadigmIdReview | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/paradigm/alerts` | getApiParadigmAlerts | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/paradigm/analyze` | postApiParadigmAnalyze | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/paradigm/backtest` | postApiParadigmBacktest | — | — | 201 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/paradigm/evaluate` | postApiParadigmEvaluate | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/paradigm/list` | getApiParadigmList | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/paradigm/stats` | getApiParadigmStats | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/paradigm/stock/{code}` | getApiParadigmStockCode | `code`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |

## 持仓决策与设置

共 5 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| POST | `/api/position-decisions/run` | postApiPositionDecisionsRun | — | 是 | 200 / 422 / 500 / 503 |
| GET | `/api/position-decisions/runs/{id}` | getApiPositionDecisionRun | `id`(path) | — | 200 / 404 / 500 / 503 |
| GET | `/api/position-decisions/today` | getApiPositionDecisionsToday | — | — | 200 / 404 / 500 / 503 |
| GET | `/api/settings/indicator` | getApiSettingsIndicator | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| PUT | `/api/settings/indicator` | putApiSettingsIndicator | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |

## 资讯

共 26 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/news/alerts` | getApiNewsAlerts | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| PUT | `/api/news/alerts/{id}/read` | putApiNewsAlertsIdRead | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/alerts/count` | getApiNewsAlertsCount | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| PUT | `/api/news/alerts/read-all` | putApiNewsAlertsReadAll | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/news/alerts/rule` | postApiNewsAlertsRule | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| DELETE | `/api/news/alerts/rule/{id}` | deleteApiNewsAlertsRuleId | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| PUT | `/api/news/alerts/rule/{id}` | putApiNewsAlertsRuleId | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/alerts/rules` | getApiNewsAlertsRules | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/alerts/unread` | getApiNewsAlertsUnread | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/news/alerts/watchlist` | postApiNewsAlertsWatchlist | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/events` | getApiNewsEvents | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/events/{id}` | getApiNewsEventsId | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/news/events/refresh` | postApiNewsEventsRefresh | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/feed` | getApiNewsFeed | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/feed/facets` | getApiNewsFeedFacets | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/feed/sources` | getApiNewsFeedSources | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/news/fetch` | postApiNewsFetch | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/news/fetch/browser` | postApiNewsFetchBrowser | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/item/{id}` | getApiNewsItemId | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/search` | getApiNewsSearch | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/sentiment/heatmap` | getApiNewsSentimentHeatmap | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/sentiment/market` | getApiNewsSentimentMarket | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/sentiment/stock/{code}` | getApiNewsSentimentStockCode | `code`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/sentiment/trend` | getApiNewsSentimentTrend | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/news/stock/{code}` | getApiNewsStockCode | `code`(path) · `limit?`(query) · `days?`(query) · `type?`(query) · `consistency?`(query) · `refresh?`(query) · `all_mentions?`(query) · `min_confidence?`(query) · `page?`(query) | — | 200 / 400 / 401 / 404 / 409 / 500 / 502 / 503 |
| GET | `/api/news/topics` | getApiNewsTopics | `date?`(query) · `top?`(query) · `consistency?`(query) · `refresh?`(query) · `include_weekend?`(query) · `min_confidence?`(query) | — | 200 / 400 / 401 / 404 / 409 / 500 / 502 / 503 |

## Agent

共 11 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| POST | `/api/agent/chat` | postApiAgentChat | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/agent/chat/session/{id}` | getApiAgentChatSessionId | `id`(path) | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/agent/chat/session/list` | getApiAgentChatSessionList | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/agent/chat/session/save` | postApiAgentChatSessionSave | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/agent/chat/stream` | streamAgentChat | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/agent/debate` | postApiAgentDebate | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/agent/diagnose` | getApiAgentDiagnose | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/agent/research` | postApiAgentResearch | — | 是 | 201 / 400 / 401 / 422 / 500 / 503 |
| GET | `/api/agent/sessions` | getApiAgentSessions | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/agent/state` | getApiAgentState | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/agent/transcript` | getApiAgentTranscript | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |

## 其他

共 5 个端点。

| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/stock/compare` | getApiStockCompare | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/strategy/overnight` | postApiStrategyOvernight | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| POST | `/api/sync/daily` | postApiSyncDaily | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/sync/freshness` | getApiSyncFreshness | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |
| GET | `/api/sync/state` | getApiSyncState | — | — | 200 / 400 / 401 / 404 / 409 / 500 / 503 |

:::note[为什么不在文档里逐个解释字段]
字段级定义随代码变动，抄一份到文档里必然失真。要看具体响应结构，请直接读
`api/openapi.json` 的 `components.schemas`，或用 `curl` 打本机实例。
:::
