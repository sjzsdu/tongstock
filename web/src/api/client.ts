import type {
  Quote, QuoteItem, KlineItem, IndicatorData, Finance, XdXrItem,
  CompanyCategory, MinuteItem, TradeItem, AuctionItem,
  BlockItem, CodeItem, IndexBar, ScreenResponse, SignalAnalysis,
  StockSearchResponse,
  StockSearchIndexResponse,
  HistoryStock,
  WatchlistStock,
  IndicatorConfig,
  FinanceTrendsResponse,
  FinanceMetricsResponse,
  KlineBatchSyncResult,
  KlineSyncState,
  StockCompareResponse,
  AgentState,
  AgentDiagnosticResponse,
  AgentChatResponse,
  AgentResearchRequest,
  AgentResearchResponse,
  AgentSessionsResponse,
  AgentTranscriptResponse,
  AgentDebateResponse,
  CustomStockPool,
  ParadigmAnalyzeResponse,
  ParadigmListResponse,
  ParadigmItem,
  ParadigmEvaluateResponse,
  ParadigmAlertsResponse,
  ParadigmStatsResponse,
  ParadigmBacktestItem,
  ChatSessionInfo,
  NewsItem,
  NewsSummary,
  FeedResult,
  StockNewsResult,
  HotTopicResult,
  HotEvent,
  EventResult,
  MarketSentiment,
  SentimentTrend,
  SentimentHeatmapItem,
  AlertRecord,
  AlertRule,
  SyncFreshnessResult,
  ForwardRun,
  ForwardRunCreateRequest,
  ForwardRunExecuteRequest,
  ForwardRunExecuteResponse,
  ForwardRunCompareRequest,
  ComparisonReport,
  SignalEntry,
  EquityPoint,
  MonitoringReport,
  MonitoringReportEnvelope,
  MonitoringInputStatus,
  AlertItem,
  AlertSummary,
  NewsFacets,
  EvidenceCard,
  ParadigmPromotionStatusResponse,
} from '../types/api';
import type { ErrorEnvelope } from './generated';

const BASE = '';
const ACCESS_TOKEN_KEY = 'tongstock.access_token';

function storedAccessToken(): string {
  if (typeof window === 'undefined') return '';
  return window.localStorage.getItem(ACCESS_TOKEN_KEY)?.trim() || '';
}

export async function fetchWithAccessToken(path: string, init?: RequestInit): Promise<Response> {
  const request = (token: string) => {
    const headers = new Headers(init?.headers);
    if (token) headers.set('Authorization', `Bearer ${token}`);
    return fetch(`${BASE}${path}`, { ...init, headers });
  };

  let token = storedAccessToken();
  let response = await request(token);
  if (response.status !== 401 || typeof window === 'undefined') {
    return response;
  }

  const entered = window.prompt('TongStock 远程访问需要 Access Token', token);
  token = entered?.trim() || '';
  if (!token) return response;
  window.localStorage.setItem(ACCESS_TOKEN_KEY, token);
  response = await request(token);
  return response;
}

// 监控报告特殊：404 不是错误，而是“尚无真实观测输入”，响应体携带 input 诊断。
async function fetchMonitoringReport(path: string, init?: RequestInit): Promise<MonitoringReportEnvelope> {
  const headers = new Headers(init?.headers);
  headers.set('Content-Type', 'application/json');
  const res = await fetchWithAccessToken(path, { ...init, headers });
  const payload = await res.json().catch(() => null) as
    | { report?: MonitoringReport; input?: MonitoringInputStatus; error?: unknown }
    | null;

  if (res.status === 404) {
    return { available: false, report: null, input: payload?.input ?? null };
  }
  if (!res.ok) {
    if (payload && typeof payload.error === 'object') {
      const error = payload.error as { code: string; message: string; request_id?: string };
      throw new TongStockAPIError(error.code, error.message, error.request_id, res.status);
    }
    throw new TongStockAPIError('http_error', '获取监控报告失败', undefined, res.status);
  }
  return {
    available: true,
    report: payload?.report ?? null,
    input: payload?.input ?? null,
  };
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  headers.set('Content-Type', 'application/json');
  const res = await fetchWithAccessToken(path, {
    ...init,
    headers,
  });
  if (!res.ok) {
    const payload = await res.json().catch(() => null) as ErrorEnvelope | { error?: string } | null;
    if (payload && typeof payload.error === 'object') {
      throw new TongStockAPIError(payload.error.code, payload.error.message, payload.error.request_id, res.status, payload);
    }
    throw new TongStockAPIError('http_error', typeof payload?.error === 'string' ? payload.error : '请求失败', undefined, res.status, payload);
  }
  const data = await res.json();
  // 检查响应是否包含错误字段
  if (data && typeof data === 'object' && 'error' in data) {
    throw new Error(data.error || '请求失败');
  }
  return data;
}

export class TongStockAPIError extends Error {
  readonly code: string;
  readonly requestId?: string;
  readonly status?: number;
  /** 原始响应体：422 等状态码的响应体可能携带可用数据（如范式部分结果） */
  readonly payload?: unknown;

  constructor(
    code: string,
    message: string,
    requestId?: string,
    status?: number,
    payload?: unknown,
  ) {
    super(message);
    this.name = 'TongStockAPIError';
    this.code = code;
    this.requestId = requestId;
    this.status = status;
    this.payload = payload;
  }
}

export const api = {
  quote: (code: string) =>
    fetchJSON<Quote>(`/api/quote?code=${code}`),

  quotes: (codes: string) =>
    fetchJSON<QuoteItem[]>(`/api/quotes?codes=${codes}`),

  codes: (exchange = 'sz') =>
    fetchJSON<CodeItem[]>(`/api/codes?exchange=${exchange}`),

  kline: (code: string, type = 'day') =>
    fetchJSON<KlineItem[]>(`/api/kline?code=${code}&type=${type}`),

  indicator: (code: string, type = 'day') =>
    fetchJSON<IndicatorData>(`/api/indicator?code=${code}&type=${type}`),

  index: (code: string, type = 'day') =>
    fetchJSON<IndexBar[]>(`/api/index?code=${code}&type=${type}`),

  minute: (code: string) =>
    fetchJSON<{ List: MinuteItem[] }>(`/api/minute?code=${code}`),

  minuteHistory: (code: string, date: string) =>
    fetchJSON<{ List: MinuteItem[] }>(`/api/minute?code=${code}&history=true&date=${date}`),

  trade: (code: string) =>
    fetchJSON<{ List: TradeItem[] }>(`/api/trade?code=${code}`),

  tradeHistory: (code: string, date: string) =>
    fetchJSON<{ List: TradeItem[] }>(`/api/trade?code=${code}&history=true&date=${date}`),

  auction: (code: string) =>
    fetchJSON<{ List: AuctionItem[] }>(`/api/auction?code=${code}`),

  xdxr: (code: string) =>
    fetchJSON<XdXrItem[]>(`/api/xdxr?code=${code}`),

  finance: (code: string) =>
    fetchJSON<Finance>(`/api/finance?code=${code}`),

  financeTrends: (code: string, mode: 'quarter' | 'year' = 'quarter') =>
    fetchJSON<FinanceTrendsResponse>(`/api/finance/trends?code=${code}&mode=${mode}`),

  financeMetrics: (code: string) =>
    fetchJSON<FinanceMetricsResponse>(`/api/finance/metrics?code=${code}`),

  company: (code: string) =>
    fetchJSON<CompanyCategory[]>(`/api/company?code=${code}`),

  // Blocks are always requested by name: the F10 file is regenerated daily
  // while its byte-offset catalogue is cached for far longer, so passing
  // Start/Length would hand back a window that has slid into a neighbouring
  // block. The server resolves the name and verifies it against the document.
  companyContent: (code: string, blockOrCategory: string | { Name: string }) => {
    const params = new URLSearchParams({ code });
    params.set('block', typeof blockOrCategory === 'string' ? blockOrCategory : blockOrCategory.Name);
    return fetchJSON<{ content: string }>(`/api/company/content?${params}`);
  },

  block: (file = 'block_zs.dat', stocksOnly = true) =>
    fetchJSON<BlockItem[]>(`/api/block?file=${file}${stocksOnly ? '&stocks_only=true' : ''}`),

  // Block APIs with new structure
  blockFiles: () =>
    fetchJSON<{ files: { file: string; name: string; desc: string }[] }>('/api/block/files'),

  blockList: (file = 'block_zs.dat', type?: string, sort = false) => {
    const params = new URLSearchParams({ file });
    if (type) params.set('type', type);
    if (sort) params.set('sort', 'true');
    return fetchJSON<{ blocks: { name: string; type: number; count: number }[] }>(`/api/block/list?${params}`);
  },

  blockShow: (name?: string, code?: string, file = 'block_zs.dat') => {
    const params = new URLSearchParams({ file });
    if (name) params.set('name', name);
    if (code) params.set('code', code);
    return fetchJSON<{ stocks?: { code: string; name: string; exchange: string }[]; blocks?: { name: string; type: number; count: number }[] }>(`/api/block/show?${params}`);
  },

  // Codes APIs with new structure
  codesList: (exchange = 'sz', category?: string) => {
    const params = new URLSearchParams({ exchange });
    if (category) params.set('category', category);
    return fetchJSON<{ exchange: string; category: string; total: number; codes: { code: string; name: string; cat: string; exchange: string }[] }>(`/api/codes/list?${params}`);
  },

  codesStats: (exchange = 'sz', all = false) => {
    const params = new URLSearchParams({ exchange });
    if (all) params.set('all', 'true');
    return fetchJSON<{ stats: { exchange: string; name: string; total: number; categories: Record<string, number> }[] }>(`/api/codes/stats?${params}`);
  },

  // Market-wide stock codes with deduplication
  codesMarket: () => {
    return fetchJSON<{ total: number; codes: { code: string; name: string; exchange: string }[] }>('/api/codes/market');
  },

  // Market-wide stock codes with market cap info and filtering
  codesWithMarketCap: (minMarketCap?: number, maxMarketCap?: number) => {
    const params = new URLSearchParams();
    if (minMarketCap != null && minMarketCap > 0) params.set('minMarketCap', String(minMarketCap));
    if (maxMarketCap != null && maxMarketCap > 0) params.set('maxMarketCap', String(maxMarketCap));
    return fetchJSON<{ total: number; codes: { code: string; name: string; exchange: string; marketCap: number; price: number }[] }>(`/api/codes/marketcap?${params}`);
  },

  screen: (codes: string, type = 'day', signals?: string[]) => {
    const p = new URLSearchParams({ codes, type });
    if (signals && signals.length > 0) {
      p.set('signals', signals.join(','));
    }
    return fetchJSON<ScreenResponse>(`/api/screen?${p}`);
  },

  signalAnalysis: (code: string, type = 'day') =>
    fetchJSON<SignalAnalysis>(`/api/signal-analysis?code=${code}&type=${type}`),

  searchStocks: (query: string, limit = 10) =>
    fetchJSON<StockSearchResponse>(`/api/stocks/search?query=${encodeURIComponent(query)}&limit=${limit}`),

  stockSearchIndex: () =>
    fetchJSON<StockSearchIndexResponse>('/api/stocks/search-index'),

  history: () =>
    fetchJSON<{ data: HistoryStock[] }>('/api/history').then(r => r.data),

  historyAdd: (code: string, name?: string) =>
    fetchJSON<{ message: string }>('/api/history', {
      method: 'POST',
      body: JSON.stringify({ code, name }),
    }),

  historyDelete: (code: string) =>
    fetchJSON<{ message: string }>(`/api/history/${code}`, {
      method: 'DELETE',
    }),

  watchlist: (group?: string) => {
    const params = new URLSearchParams();
    if (group) params.set('group', group);
    const query = params.toString();
    return fetchJSON<{ data: WatchlistStock[] }>(`/api/watchlist${query ? `?${query}` : ''}`).then(r => r.data);
  },

  watchlistAdd: (code: string, name?: string, group?: string, note?: string) =>
    fetchJSON<{ message: string }>('/api/watchlist', {
      method: 'POST',
      body: JSON.stringify({ code, name, group, note }),
    }),

  watchlistDelete: (code: string) =>
    fetchJSON<{ message: string }>(`/api/watchlist/${code}`, {
      method: 'DELETE',
    }),

  watchlistUpdateNote: (code: string, note: string) =>
    fetchJSON<{ message: string }>(`/api/watchlist/${code}/note`, {
      method: 'PUT',
      body: JSON.stringify({ note }),
    }),

  watchlistUpdateGroup: (code: string, group: string) =>
    fetchJSON<{ message: string }>(`/api/watchlist/${code}/group`, {
      method: 'PUT',
      body: JSON.stringify({ group }),
    }),

  watchlistGroups: () =>
    fetchJSON<{ groups: { name: string; count: number }[] }>('/api/watchlist/groups'),

  // Stockpool APIs
  stockpoolList: () =>
    fetchJSON<{ pools: CustomStockPool[] }>('/api/stockpool'),

  stockpoolUpsert: (pool: CustomStockPool) =>
    fetchJSON<{ success: boolean }>('/api/stockpool', {
      method: 'POST',
      body: JSON.stringify(pool),
    }),

  stockpoolDelete: (id: string) =>
    fetchJSON<{ success: boolean }>(`/api/stockpool/${id}`, {
      method: 'DELETE',
    }),

  // Discovery research APIs
  discoverRun: (req: { pool_id?: string; codes?: string[]; question?: string; hold_days?: number; search_budget?: number }) =>
    fetchJSON<{
      research_id: string;
      snapshot_id: string;
      conclusion: string;
      candidate_count: number;
      candidates: {
        rank: number;
        template_id: string;
        method: { name: string; content_hash: string };
        observations: number;
        mean_forward_return: number;
        win_rate: number;
        baseline_return: number;
        lift: number;
        t_statistic: number;
        validation_evidence: { stock_code: string; status: string; confidence?: string; passable?: boolean }[];
      }[];
      rejected_count: number;
    }>('/api/discover/run', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  discoverTraces: (limit?: number) =>
    fetchJSON<{
      traces: {
        research_id: string;
        snapshot_id: string;
        conclusion: string;
        discovery_trials: number;
        created_at: string;
        candidate_count: number;
        passable_count: number;
        stock_codes?: string[];
      }[];
      total: number;
    }>(`/api/discover/traces${limit ? `?limit=${limit}` : ''}`),

  // Stockinfo APIs
  stockinfoList: (minMarketCap?: number, maxMarketCap?: number, exchange?: string) => {
    const params = new URLSearchParams();
    if (minMarketCap != null && minMarketCap > 0) params.set('minMarketCap', String(minMarketCap));
    if (maxMarketCap != null && maxMarketCap > 0) params.set('maxMarketCap', String(maxMarketCap));
    if (exchange) params.set('exchange', exchange);
    return fetchJSON<{ total: number; infos: { code: string; name: string; exchange: string; price: number; marketCap: number; turnoverRate: number; changePct: number; volumeRatio: number }[] }>(`/api/stockinfo?${params}`);
  },

  stockinfoGet: (code: string) =>
    fetchJSON<{ code: string; name: string; exchange: string; price: number; marketCap: number; turnoverRate: number; changePct: number; volumeRatio: number }>(`/api/stockinfo/${code}`),

  stockinfoSync: (force = false) =>
    fetchJSON<{ total: number; success: number; failed: number; duration: string; updated_at: number }>('/api/stockinfo/sync', {
      method: 'POST',
      body: JSON.stringify({ force }),
    }),

  stockinfoCount: () =>
    fetchJSON<{ count: number }>('/api/stockinfo/count'),

  saveScreenResults: (results: { code: string; name?: string }[]) =>
    Promise.all(results.map((item) => api.watchlistAdd(item.code, item.name))),

  syncDaily: (codes: string[], mode = 'auto', concurrency = 3) =>
    fetchJSON<KlineBatchSyncResult>('/api/sync/daily', {
      method: 'POST',
      body: JSON.stringify({ codes, mode, concurrency }),
    }),

  getSyncState: (code: string, ktype = 'day') =>
    fetchJSON<KlineSyncState>(`/api/sync/state?code=${encodeURIComponent(code)}&ktype=${ktype}`),

  getSyncFreshness: (codes: string[]) =>
    fetchJSON<{ results: SyncFreshnessResult[] }>(`/api/sync/freshness?codes=${encodeURIComponent(codes.join(','))}`),

  indicatorSettings: () =>
    fetchJSON<IndicatorConfig>('/api/settings/indicator'),

  saveIndicatorSettings: (config: IndicatorConfig) =>
    fetchJSON<{ message: string; config: IndicatorConfig }>('/api/settings/indicator', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),

  stockCompare: (code: string) =>
    fetchJSON<StockCompareResponse>(`/api/stock/compare?code=${code}`),

  tradeCreate: (data: { code: string; name?: string; action: 'buy' | 'sell'; price: number; signal?: string; ktype?: string; reason?: string }) =>
    fetchJSON<{ id: number; code: string; action: string }>('/api/trades', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  trades: (codes?: string) => {
    const params = codes ? `?codes=${encodeURIComponent(codes)}` : '';
    return fetchJSON<Record<string, TradeInfo>>(`/api/trades${params}`);
  },

  tradePositions: () =>
    fetchJSON<{ positions: TradeInfo[] }>('/api/trades/positions'),

  tradeDelete: (id: number) =>
    fetchJSON<{ success: boolean }>(`/api/trades/${id}`, { method: 'DELETE' }),

  // Agent APIs
  agentState: () =>
    fetchJSON<AgentState>('/api/agent/state'),

  agentDiagnose: () =>
    fetchJSON<AgentDiagnosticResponse>('/api/agent/diagnose'),

  agentChat: (message: string, agent?: string, session?: string) =>
    fetchJSON<AgentChatResponse>('/api/agent/chat', {
      method: 'POST',
      body: JSON.stringify({ message, agent, session }),
    }),

  agentResearch: (payload: AgentResearchRequest) =>
    fetchJSON<AgentResearchResponse>('/api/agent/research', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  agentSessions: () =>
    fetchJSON<AgentSessionsResponse>('/api/agent/sessions'),

  agentTranscript: (session: string, agent?: string) => {
    const params = new URLSearchParams({ session });
    if (agent) params.set('agent', agent);
    return fetchJSON<AgentTranscriptResponse>(`/api/agent/transcript?${params}`);
  },

  agentDebate: (stockCode: string, stockName?: string, topic?: string, agents?: string[]) =>
    fetchJSON<AgentDebateResponse>('/api/agent/debate', {
      method: 'POST',
      body: JSON.stringify({ stock_code: stockCode, stock_name: stockName, topic, agents }),
    }),

  // Paradigm APIs
  paradigmAnalyze: (stockCode: string, stockName?: string, days?: number, forceRefresh = false, signal?: AbortSignal) =>
    fetchJSON<ParadigmAnalyzeResponse>('/api/paradigm/analyze', {
      method: 'POST',
      body: JSON.stringify({ stock_code: stockCode, stock_name: stockName, days, force_refresh: forceRefresh }),
      signal,
    }),

  paradigmListByStock: (code: string) =>
    fetchJSON<ParadigmListResponse>(`/api/paradigm/stock/${code}`),

  paradigmList: (marketCap?: string, shareholder?: string, extra?: Record<string, string | number | undefined>) => {
    const params = new URLSearchParams();
    if (marketCap) params.set('market_cap', marketCap);
    if (shareholder) params.set('shareholder', shareholder);
    if (extra) Object.entries(extra).forEach(([k, v]) => { if (v !== undefined && v !== '') params.set(k, String(v)); });
    const q = params.toString();
    return fetchJSON<ParadigmListResponse>(`/api/paradigm/list${q ? '?' + q : ''}`);
  },

  paradigmEvaluate: (stockCode: string) =>
    fetchJSON<ParadigmEvaluateResponse>('/api/paradigm/evaluate', {
      method: 'POST',
      body: JSON.stringify({ stock_code: stockCode }),
    }),

  paradigmAlerts: (stockCode?: string) =>
    fetchJSON<ParadigmAlertsResponse>(`/api/paradigm/alerts${stockCode ? `?stock_code=${encodeURIComponent(stockCode)}` : ''}`),

  paradigmStats: () =>
    fetchJSON<ParadigmStatsResponse>('/api/paradigm/stats'),

  paradigmBacktest: (paradigmId: string, snapshotId?: string) =>
    fetchJSON<ParadigmBacktestItem>('/api/paradigm/backtest', {
      method: 'POST',
      body: JSON.stringify({
        paradigm_id: paradigmId,
        ...(snapshotId ? { snapshot_id: snapshotId } : {}),
      }),
    }),

  paradigmReview: (id: string, review: { review_status: string; review_note?: string; review_rating?: number; actual_return?: number }) =>
    fetchJSON<ParadigmItem>(`/api/paradigm/${id}/review`, {
      method: 'PUT',
      body: JSON.stringify(review),
    }),

  paradigmPromote: (id: string, snapshotId?: string) =>
    fetchJSON<{ started: boolean; status_url: string }>(`/api/paradigm/${id}/promote`, {
      method: 'POST',
      body: JSON.stringify(snapshotId ? { snapshot_id: snapshotId } : {}),
    }),

  paradigmPromotionStatus: (id: string) =>
    fetchJSON<ParadigmPromotionStatusResponse>(`/api/paradigm/${id}/promotion/status`),

  paradigmEvidence: (id: string) =>
    fetchJSON<EvidenceCard>(`/api/paradigm/${id}/evidence`),

  // Chat session persistence
  chatSave: (id: string, stockCode: string, stockName: string, agent: string, messages: { role: string; content: string }[]) =>
    fetchJSON<{ id: string }>('/api/agent/chat/session/save', {
      method: 'POST',
      body: JSON.stringify({ id, stock_code: stockCode, stock_name: stockName, agent, messages }),
    }),

  chatList: (stockCode?: string) => {
    const params = stockCode ? `?stock_code=${stockCode}` : '';
    return fetchJSON<{ sessions: ChatSessionInfo[] }>(`/api/agent/chat/session/list${params}`);
  },

  chatGet: (id: string) =>
    fetchJSON<ChatSessionInfo>(`/api/agent/chat/session/${id}`),

  paradigmDelete: (id: string) =>
	fetchJSON<{ message: string }>(`/api/paradigm/${id}`, { method: 'DELETE' }),

	// Strategy APIs
	overnightArbitrage: (codes: string[], minMarketCap?: number, maxMarketCap?: number) =>
		fetchJSON<OvernightArbitrageResponse>('/api/strategy/overnight', {
			method: 'POST',
			body: JSON.stringify({ codes, minMarketCap, maxMarketCap }),
		}),

	// Newsfeed APIs
	newsFeed: (params?: {
		sources?: string;
		types?: string;
		keyword?: string;
		startTime?: string;
		endTime?: string;
		hotScoreMin?: number;
		page?: number;
		pageSize?: number;
		sortBy?: string;
	}) => {
		const p = new URLSearchParams();
		if (params?.sources) p.set('sources', params.sources);
		if (params?.types) p.set('types', params.types);
		if (params?.keyword) p.set('keyword', params.keyword);
		if (params?.startTime) p.set('startTime', params.startTime);
		if (params?.endTime) p.set('endTime', params.endTime);
		if (params?.hotScoreMin != null) p.set('hotScoreMin', String(params.hotScoreMin));
		if (params?.page != null) p.set('page', String(params.page));
		if (params?.pageSize != null) p.set('pageSize', String(params.pageSize));
		if (params?.sortBy) p.set('sortBy', params.sortBy);
		const q = p.toString();
		return fetchJSON<FeedResult>(`/api/news/feed${q ? '?' + q : ''}`);
	},

	/** 已注册的数据源及其健康状态 */
	newsFeedSources: () =>
		fetchJSON<{ sources: { name: string; interval: string; healthy: boolean }[] }>('/api/news/feed/sources'),

	/** 来源与类型在库中的条数分布，供信息流筛选器生成选项 */
	newsFacets: () => fetchJSON<NewsFacets>('/api/news/feed/facets'),

	newsItem: (id: string) =>
		fetchJSON<NewsItem>(`/api/news/item/${id}`),

	newsStock: (code: string, params?: { limit?: number; days?: number; allMentions?: boolean }) => {
		const q = new URLSearchParams();
		if (params?.limit) q.set('limit', String(params.limit));
		if (params?.days) q.set('days', String(params.days));
		if (params?.allMentions) q.set('all_mentions', 'true');
		const suffix = q.toString() ? '?' + q.toString() : '';
		return fetchJSON<StockNewsResult>(`/api/news/stock/${code}${suffix}`);
	},

	newsSearch: (keyword: string) =>
		fetchJSON<{ total: number; items: NewsSummary[] }>(`/api/news/search?keyword=${encodeURIComponent(keyword)}`),

	/** 指定日期新闻里的热门股票榜单（date 缺省为今天） */
	newsTopics: (params?: { date?: string; top?: number; consistency?: string; includeWeekend?: boolean; minConfidence?: number }) => {
		const q = new URLSearchParams();
		if (params?.date) q.set('date', params.date);
		if (params?.top) q.set('top', String(params.top));
		if (params?.consistency) q.set('consistency', params.consistency);
		if (params?.includeWeekend) q.set('include_weekend', 'true');
		if (params?.minConfidence) q.set('min_confidence', String(params.minConfidence));
		const suffix = q.toString() ? '?' + q.toString() : '';
		return fetchJSON<HotTopicResult>(`/api/news/topics${suffix}`);
	},

	newsFetch: () =>
		fetchJSON<{ count: number; msg: string }>('/api/news/fetch', { method: 'POST' }),

	newsFetchBrowser: (site?: string) =>
		fetchJSON<{ count: number; msg: string; errors?: string[] }>(`/api/news/fetch/browser${site ? '?site=' + site : ''}`, { method: 'POST' }),

	// Hot events APIs
	hotEvents: (params?: { minHotIndex?: number; status?: string; limit?: number }) => {
		const p = new URLSearchParams();
		if (params?.minHotIndex != null) p.set('minHotIndex', String(params.minHotIndex));
		if (params?.status) p.set('status', params.status);
		if (params?.limit != null) p.set('limit', String(params.limit));
		const q = p.toString();
		return fetchJSON<EventResult>(`/api/news/events${q ? '?' + q : ''}`);
	},

	hotEventDetail: (id: string) =>
		fetchJSON<{ event: HotEvent; newsItems: NewsItem[] }>(`/api/news/events/${id}`),

	/** 重新聚类生成热点事件，返回本次生成的事件数 */
	refreshHotEvents: () =>
		fetchJSON<{ count: number }>('/api/news/events/refresh', { method: 'POST' }),

	// Sentiment APIs
	sentimentMarket: (hours?: number) => {
		const params = hours != null ? `?hours=${hours}` : '';
		return fetchJSON<MarketSentiment>(`/api/news/sentiment/market${params}`);
	},

	sentimentTrend: (hours?: number, intervals?: number) => {
		const p = new URLSearchParams();
		if (hours != null) p.set('hours', String(hours));
		if (intervals != null) p.set('intervals', String(intervals));
		const q = p.toString();
		return fetchJSON<SentimentTrend[]>(`/api/news/sentiment/trend${q ? '?' + q : ''}`);
	},

	sentimentHeatmap: (hours?: number, topN?: number) => {
		const p = new URLSearchParams();
		if (hours != null) p.set('hours', String(hours));
		if (topN != null) p.set('topN', String(topN));
		const q = p.toString();
		return fetchJSON<SentimentHeatmapItem[]>(`/api/news/sentiment/heatmap${q ? '?' + q : ''}`);
	},

	sentimentStock: (code: string, hours?: number) => {
		const params = hours != null ? `?hours=${hours}` : '';
		return fetchJSON<MarketSentiment>(`/api/news/sentiment/stock/${code}${params}`);
	},

	// Alert APIs
	alerts: (limit?: number, read?: boolean) => {
		const p = new URLSearchParams();
		if (limit != null) p.set('limit', String(limit));
		if (read != null) p.set('read', String(read));
		const q = p.toString();
		return fetchJSON<AlertRecord[]>(`/api/news/alerts${q ? '?' + q : ''}`);
	},

	unreadAlerts: (limit?: number) => {
		const params = limit != null ? `?limit=${limit}` : '';
		return fetchJSON<AlertRecord[]>(`/api/news/alerts/unread${params}`);
	},

	alertUnreadCount: () =>
		fetchJSON<{ count: number }>('/api/news/alerts/count'),

	markAlertRead: (id: string) =>
		fetchJSON<{ msg: string }>(`/api/news/alerts/${id}/read`, { method: 'PUT' }),

	markAllAlertsRead: () =>
		fetchJSON<{ msg: string }>('/api/news/alerts/read-all', { method: 'PUT' }),

	alertRules: () =>
		fetchJSON<AlertRule[]>('/api/news/alerts/rules'),

	addAlertRule: (rule: Omit<AlertRule, 'id' | 'createdAt' | 'lastTrigger'>) =>
		fetchJSON<AlertRule>('/api/news/alerts/rule', {
			method: 'POST',
			body: JSON.stringify(rule),
		}),

	updateAlertRule: (id: string, updates: Partial<AlertRule>) =>
		fetchJSON<{ msg: string }>(`/api/news/alerts/rule/${id}`, {
			method: 'PUT',
			body: JSON.stringify(updates),
		}),

	deleteAlertRule: (id: string) =>
		fetchJSON<{ msg: string }>(`/api/news/alerts/rule/${id}`, { method: 'DELETE' }),

	setWatchlist: (stockCodes: string[]) =>
		fetchJSON<{ msg: string; count: number }>('/api/news/alerts/watchlist', {
			method: 'POST',
			body: JSON.stringify({ stockCodes }),
		}),

	// Forward Run APIs
	forwardRuns: (limit?: number) =>
		fetchJSON<{ runs: ForwardRun[]; total: number }>(
			'/api/forward/runs' + (limit ? `?limit=${limit}` : '')),

	forwardRunCreate: (payload: ForwardRunCreateRequest) =>
		fetchJSON<{ run: ForwardRun }>('/api/forward/runs', {
			method: 'POST',
			body: JSON.stringify(payload),
		}),

	forwardRunGet: (id: string) =>
		fetchJSON<{ run: ForwardRun }>(`/api/forward/runs/${id}`),

	forwardRunExecute: (id: string, payload?: ForwardRunExecuteRequest) =>
		fetchJSON<ForwardRunExecuteResponse>(`/api/forward/runs/${id}/execute`, {
			method: 'POST',
			body: JSON.stringify(payload || {}),
		}),

	forwardRunFinalize: (id: string, endDate?: string) =>
		fetchJSON<{ run: ForwardRun }>(
			`/api/forward/runs/${id}/finalize` + (endDate ? `?end_date=${endDate}` : '')),

	forwardRunSignals: (runId: string) =>
		fetchJSON<{ signals: SignalEntry[]; total: number }>(`/api/forward/runs/${runId}/signals`),

	forwardSignalGet: (id: string) =>
		fetchJSON<{ signal: SignalEntry }>(`/api/forward/signals/${id}`),

	forwardSignalsList: (params: { paradigm_version_id?: string; run_id?: string; date?: string }) => {
		const q = new URLSearchParams();
		if (params.paradigm_version_id) q.set('paradigm_version_id', params.paradigm_version_id);
		if (params.run_id) q.set('run_id', params.run_id);
		if (params.date) q.set('date', params.date);
		return fetchJSON<{ signals: SignalEntry[]; total: number }>('/api/forward/signals' + (q.toString() ? `?${q}` : ''));
	},

	forwardRunEquity: (id: string) =>
		fetchJSON<{ run: ForwardRun; curve: EquityPoint[] }>(`/api/forward/runs/${id}/equity`),

	forwardRunCompare: (id: string, payload: ForwardRunCompareRequest) =>
		fetchJSON<{ report: ComparisonReport; pass: boolean; warnings: string[] }>(
			`/api/forward/runs/${id}/compare`, {
				method: 'POST',
				body: JSON.stringify(payload),
			}),

	// Monitoring APIs
	monitoringReport: () => fetchMonitoringReport('/api/monitoring/report'),

	monitoringReportRefresh: () =>
		fetchMonitoringReport('/api/monitoring/report/refresh', { method: 'POST' }),

	monitoringAlerts: () =>
		fetchJSON<{ alerts: AlertItem[]; summary: AlertSummary }>('/api/monitoring/alerts'),

	monitoringAlertAck: (id: string, user?: string) =>
		fetchJSON<{ status: string; id: string }>(`/api/monitoring/alerts/${id}/ack` + (user ? `?user=${user}` : ''), {
			method: 'POST',
		}),

	monitoringAlertResolve: (id: string) =>
		fetchJSON<{ status: string; id: string }>(`/api/monitoring/alerts/${id}/resolve`, {
			method: 'POST',
		}),

	monitoringConfig: () =>
		fetchJSON<{ config: Record<string, unknown> }>('/api/monitoring/config'),

  monitoringHealth: () =>
		fetchJSON<{ status: string; engine_source: string; alert_summary: AlertSummary }>('/api/monitoring/health'),

  selectionToday: () => fetchJSON<SelectionRun>('/api/selections/today'),
  positionDecisionToday: () => fetchJSON<PositionDecisionRun>('/api/position-decisions/today'),
  methodCards: (params: MethodCardQuery = {}) => {
    const query = new URLSearchParams();
    if (params.status) query.set('status', params.status);
    if (params.market) query.set('market', params.market);
    if (params.universe) query.set('universe', params.universe);
    if (params.family_id) query.set('family_id', params.family_id);
    if (params.holding_min_days !== undefined) query.set('holding_min_days', String(params.holding_min_days));
    if (params.holding_max_days !== undefined) query.set('holding_max_days', String(params.holding_max_days));
    if (params.limit) query.set('limit', String(params.limit));
    const search = query.toString();
    return fetchJSON<{ items: MethodCard[]; total: number }>(`/api/methods${search ? `?${search}` : ''}`);
  },
  methodCard: (id: string) => fetchJSON<MethodCard>(`/api/methods/${encodeURIComponent(id)}`),
  methodAudit: (id: string) =>
    fetchJSON<{ items: MethodAuditEvent[]; total: number }>(`/api/methods/${encodeURIComponent(id)}/audit`),

  dashboardToday: () => fetchJSON<DashboardToday>('/api/dashboard/today'),
  onboardingRun: (req: OnboardingRunRequest = {}) =>
    fetchJSON<OnboardingResult>('/api/onboarding/run', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  seedMethods: (req: MethodSeedRequest = {}) =>
    fetchJSON<MethodSeedResult>('/api/methods/seed', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  // ===== 可信方法自动化闭环（自动研究 / 方法市场 / 选方法筛选 / 前向监控）=====

  /** 触发一轮自动方法研究：异步启动立即返回，结果通过 methodResearchStatus 轮询 + methodResearchLast 获取 */
  methodResearchRun: (req: MethodResearchRequest = {}) =>
    fetchJSON<MethodResearchStart>('/api/methods/research/run', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  /** 最近一轮自动研究批次结果；从未完成过时返回 200 空结构（running 标明是否有批次在跑） */
  methodResearchLast: () =>
    fetchJSON<MethodResearchResult & { status?: string; running?: boolean; running_since?: string }>('/api/methods/research/last'),

  /** 自动研究批次运行状态（启动时调度器会立即跑一轮） */
  methodResearchStatus: () =>
    fetchJSON<MethodResearchStatus>('/api/methods/research/status'),

  /** 异步启动一轮横截面因子研究：立即返回 started 包封，用 factorResearchLast 轮询结果 */
  factorResearchRun: (req: FactorResearchRequest = {}) =>
    fetchJSON<FactorResearchStart>('/api/factors/research/run', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  /** 最近一次完成的因子研究结果；从未跑过时返回 { status:'no_completed_run' }，批次运行中带 running:true。
   *  后端包封为 { running, last_error?, result? }，这里拆包成扁平结果并透传 running/last_error。 */
  factorResearchLast: async () => {
    const res = await fetchJSON<
      | { running?: boolean; last_error?: string; result: FactorResearchResult }
      | { running?: boolean; last_error?: string; status: string }
    >('/api/factors/research/last');
    if ('result' in res) {
      return { ...res.result, running: res.running, last_error: res.last_error } as FactorResearchResult & {
        running?: boolean;
        last_error?: string;
      };
    }
    return res as { status: string; running?: boolean; last_error?: string };
  },

  /** 因子通道最近一次落库的 TopN 观察名单（write_to_selection=true 的产出）；尚无产出时返回 { status:'no_pick_run' } */
  factorPicksLast: () =>
    fetchJSON<FactorPickRun | { status: string }>('/api/factors/picks/last'),

  /** 未通过证据门槛的拒绝原因分布 */
  methodRejectStats: () =>
    fetchJSON<{ items: MethodRejectStat[]; total: number; rejected_methods: number }>(
      '/api/methods/reject-stats',
    ),

  /** 已选方法的前向健康度（来自前向账本的只读评估） */
  methodForwardHealth: () =>
    fetchJSON<{ items: MethodForwardHealth[]; total: number }>('/api/methods/forward-health'),

  /** 方法好用/不好用反馈：写入审计轨迹并反哺自动研究 */
  methodFeedback: (id: string, useful: boolean, comment?: string) =>
    fetchJSON<{ id: string; status: string; updated_at: string }>(
      `/api/methods/${encodeURIComponent(id)}/feedback`,
      {
        method: 'POST',
        body: JSON.stringify({ useful, comment }),
      },
    ),

  /** 用选中的方法在冻结快照上运行一次选股 */
  selectionRunCreate: (req: SelectionRunRequest) =>
    fetchJSON<SelectionRun>('/api/selections/run', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  /** 单次选股运行详情（与 GET /api/selections/runs/:id 同源） */
  selectionRunDetail: (id: string) =>
    fetchJSON<SelectionRun>(`/api/selections/runs/${encodeURIComponent(id)}`),
};

export interface SelectionTriggerFact { path:string;rule?:string;passed:boolean;detail?:string }
export interface SelectionTrigger { method_id:string;method_version_id?:string;method_name:string;family_id?:string;score:number;facts:SelectionTriggerFact[];evidence?:MethodEvidence }
export interface SelectionCandidate { rank:number;code:string;action:'buy'|'watch'|'avoid'|'insufficient_data';score:number;data_date:string;buy_window:string;position_cap_pct:number;exit:{max_holding_days?:number;stop_loss_pct?:number;take_profit_pct?:number;complete:boolean};risks?:string[];explanation:string;triggers:SelectionTrigger[] }
export interface SelectionRun { id:string;run_hash?:string;snapshot_id:string;feature_snapshot_id:string;snapshot_date:string;eligible_methods?:number;scanned_stocks?:number;candidate_count:number;buy_count:number;action_counts?:Record<string,number>;candidates:SelectionCandidate[];exclusions:Array<{method_id?:string;code?:string;reason_code:string;detail:string}>;requested_method_ids?:string[];created_at?:string }
export interface SelectionRunRequest { market_snapshot_id?:string;feature_snapshot_id?:string;method_ids:string[] }
export interface PositionDecision { code:string;name:string;action:'hold'|'watch'|'reduce'|'exit'|'insufficient_data';priority:string;deadline:string;inferred:boolean;executable:boolean;constraint?:string;return_pct:number;price_time:string;explanation:string }
export interface PositionDecisionRun { id:string;snapshot_id:string;snapshot_date:string;decisions:PositionDecision[] }
export interface MethodHealthState { score:number;forward_samples:number;drift?:boolean;decay?:boolean;execution_deviation?:boolean;critical_alerts?:number;consecutive_severe?:number;as_of?:string }
export interface MethodEvidence { confidence:string;confidence_reason?:string;passable?:boolean;oos_trades:number;oos_return:number;oos_win_rate?:number;oos_max_drawdown:number;sharpe_ratio?:number;sortino_ratio?:number;snapshot_id?:string;result_hash?:string }
export interface MethodCard { id:string;family_id?:string;variant_id?:string;name:string;status:string;market:string;universe:string;holding_period:string;trigger_frequency?:string;entry_summary:string;exit_summary:string;invalidations?:string[];evidence?:MethodEvidence;health?:MethodHealthState;updated_at:string }
export interface MethodCardQuery { status?:string;market?:string;universe?:string;family_id?:string;holding_min_days?:number;holding_max_days?:number;limit?:number }
export interface MethodAuditEvent { id:string;method_id:string;from:string;to:string;action:string;reason:string;actor:string;evidence_hash?:string;automatic:boolean;created_at:string }

/** GET /api/methods/reject-stats 单条原因分布 */
export interface MethodRejectStat { category:string;reason:string;count:number }

/** GET /api/methods/forward-health 单条方法前向健康 */
export interface MethodForwardHealth { method_id:string;name:string;status:string;score:number;forward_samples:number;executed_count:number;rejected_count:number;hit_rate?:number;avg_return?:number;execution_deviation:boolean;decay:boolean;drift:boolean;consecutive_severe:number;degraded:boolean;retired:boolean;as_of:string }

/** POST /api/methods/research/run 请求与响应 */
export interface MethodResearchRequest { snapshot_id?:string;codes?:string[];hold_days?:number[];search_budget?:number;max_codes?:number }
/** POST /api/methods/research/run 响应：批次后台运行，不携带结果 */
export interface MethodResearchStart { started:boolean;status_url?:string;result_url?:string }
export interface MethodResearchOutcome { template_id:string;method_id?:string;method_hash?:string;status:'verified'|'rejected'|'failed'|'skipped_registered'|'skipped_feedback';stage:string;confidence?:string;reason?:string;oos_trades?:number;oos_return?:number;oos_win_rate?:number;sharpe_ratio?:number }
export interface MethodResearchResult { started_at:string;finished_at:string;snapshot_id:string;universe_size:number;validation_start?:string;validation_end?:string;trials_this_batch:number;trials_cumulative:number;batches:Array<{hold_days:number;research_id?:string;discovery_trials:number;candidates:number;registered:number;verified:number;rejected:number;error?:string}>;outcomes:MethodResearchOutcome[];registered:number;verified:number;rejected:number }

/** GET /api/methods/research/status：批次实时进度快照 */
export interface MethodResearchProgress { phase:'preparing'|'discovery'|'validation';hold_days?:number;universe_size?:number;discovery_codes_done?:number;discovery_codes_total?:number;total_candidates?:number;candidates_done?:number;verified:number;rejected:number }
/** GET /api/methods/research/status：批次是否在运行（含启动调度器那一轮） */
export interface MethodResearchStatus { running:boolean;running_since?:string;last_finished_at?:string;last_error?:string;phase?:'preparing'|'discovery'|'validation';progress?:MethodResearchProgress }

// ===== 横截面多因子研究（factorlab）：预测未来 N 日收益的截面排序 =====

/** POST /api/factors/research/run 请求 */
export interface FactorResearchRequest { snapshot_id?:string;horizon_days?:number;top_k?:number;max_codes?:number;write_to_selection?:boolean }
/** POST /api/factors/research/run 响应：批次后台运行，结果经 /last 轮询获取 */
export interface FactorResearchStart { started:boolean;status_url?:string;result_url?:string }
/** 单因子预测力评估：RankIC 序列统计 + 显著性参考线（|t|≥2 且 |MeanIC|≥0.03，t 用去重叠独立截面计算） */
export interface FactorEval { key:string;name:string;description:string;prior:number;sections:number;pairs:number;coverage:number;mean_ic:number;ic_std:number;icir:number;t_stat:number;effective_sections:number;significant:boolean;direction:number }
/** 最后截面日的头部股票：得分 + 各合格因子的贡献分解（可解释） */
export interface FactorTopPick { code:string;score:number;contributions?:Record<string,number> }
/** 一轮因子研究的完整结果 */
export interface FactorResearchResult { engine_version:string;snapshot_id:string;started_at:string;finished_at:string;horizon_days:number;top_k:number;codes:number;sections:number;last_date?:string;snapshot_date_end?:string;stale_days:number;factors:FactorEval[];top_picks:FactorTopPick[];note:string }
/** 落库后的因子通道产出：run_id = pick-<截面日期>，同截面日重跑幂等更新 */
export interface FactorPickEntry { code:string;score:number;contributions?:Record<string,number> }
export interface FactorPickRun { run_id:string;snapshot_id:string;snapshot_date_end?:string;as_of:string;stale_days:number;factors_snapshot:FactorEval[];note:string;picks:FactorPickEntry[];created_at:number;updated_at:number }

export interface OvernightCriteria {
	change_pct: boolean;
	volume_ratio: boolean;
	turnover_rate: boolean;
	market_cap: boolean;
	limit_up_history: boolean;
	ma_multiple: boolean;
	above_vwap: boolean;
}

export interface OvernightCandidate {
	code: string;
	name: string;
	price: number;
	change_pct: number;
	volume_ratio: number;
	turnover_rate: number;
	market_cap: number;
	criteria: OvernightCriteria;
	passed: boolean;
	fail_reason: string;
}

export interface OvernightArbitrageResponse {
	total: number;
	stage1_passed: number;
	stage1_failed: number;
	stage2_passed: number;
	stage2_failed: number;
	stage3_passed: number;
	stage3_failed: number;
	stage4_passed: number;
	stage4_failed: number;
	final_candidates: OvernightCandidate[];
	failed: { code: string; reason: string }[];
	current_time: string;
	is_overnight_time: boolean;
}

export interface TradeInfo {
  id: number;
  code: string;
  name: string;
  action: 'buy' | 'sell';
  price: number;
  signal: string;
  ktype: string;
  reason: string;
  created_at: string;
}

// ===== 首屏「今日状态」读模型（GET /api/dashboard/today）=====

/** 空榜单原因。前端据此决定展示哪种分诊文案。 */
export type EmptyStateReason =
  | 'data_not_synced'
  | 'no_verified_methods'
  | 'selection_not_run'
  | 'no_candidates'
  | 'has_candidates';

/** 空状态下一步动作类型。 */
export type ActionKind =
  | 'sync'
  | 'seed_methods'
  | 'run_selection'
  | 'view_exclusions'
  | 'research';

export interface DashboardAction {
  kind: ActionKind;
  label: string;
  to?: string;
  primary: boolean;
}

export interface DashboardEmptyState {
  reason: EmptyStateReason;
  title: string;
  message: string;
  actions: DashboardAction[];
}

export interface DashboardDataStatus {
  latest_kline_date?: string;
  latest_snapshot_date?: string;
  latest_snapshot_id?: string;
  snapshot_status?: string;
  snapshot_frozen: boolean;
  coverage_pct: number;
  ready_codes: number;
  expected_codes: number;
  fresh: boolean;
  detail: string;
}

export interface DashboardMethodStatus {
  total: number;
  verified: number;
  observing: number;
  candidate: number;
  rejected: number;
  degraded: number;
  retired: number;
  by_status: Record<string, number>;
}

export interface DashboardSelectionStatus {
  run_id: string;
  snapshot_id: string;
  feature_snapshot_id: string;
  snapshot_date: string;
  status: string;
  created_at: string;
  scanned_stocks: number;
  eligible_methods: number;
  candidate_count: number;
  buy_count: number;
  action_counts: Record<string, number>;
  exclusion_counts: Record<string, number>;
  sample_exclusions: Array<{ method_id?: string; code?: string; reason_code: string; detail: string }>;
  candidates: SelectionCandidate[];
}

export interface DashboardWorkLogLine {
  key: string;
  label: string;
  value: number;
  detail?: string;
  tone: 'neutral' | 'positive' | 'warning';
}

export interface DashboardWorkLog {
  available: boolean;
  snapshot_date?: string;
  lines: DashboardWorkLogLine[];
}

export interface DashboardPositionStatus {
  holding_count: number;
  urgent_actions: number;
  latest_run_date?: string;
  has_decision_run: boolean;
}

export interface DashboardSignal {
  key: string;
  label: string;
  value: string;
  status: 'ok' | 'attention' | 'blocked';
  detail?: string;
}

export interface DashboardHealth {
  overall: 'ok' | 'attention' | 'blocked';
  signals: DashboardSignal[];
}

export interface DashboardToday {
  as_of: string;
  data: DashboardDataStatus;
  methods: DashboardMethodStatus;
  selection?: DashboardSelectionStatus;
  positions: DashboardPositionStatus;
  empty_state: DashboardEmptyState;
  work_log: DashboardWorkLog;
  health: DashboardHealth;
}

// ===== 一键走通（POST /api/onboarding/run）=====

export interface OnboardingRunRequest {
  date?: string;
  universe?: string;
  coverage_threshold?: number;
  max_gapped_codes?: number;
  sync?: boolean;
  force?: boolean;
}

export interface OnboardingStep {
  key: string;
  label: string;
  status: 'done' | 'skipped' | 'blocked' | 'failed';
  detail?: string;
}

export interface OnboardingResult {
  status: 'completed' | 'blocked' | 'failed';
  trade_date?: string;
  snapshot_id?: string;
  feature_snapshot_id?: string;
  selection_run_id?: string;
  candidate_count: number;
  buy_count: number;
  scanned_stocks: number;
  eligible_methods: number;
  steps: OnboardingStep[];
  blocked_reason?: string;
  finished_at: string;
}

// ===== 内置示例方法（POST /api/methods/seed）=====

export interface MethodSeedRequest {
  snapshot_id?: string;
  keys?: string[];
  max_codes?: number;
  date_start?: string;
  date_end?: string;
  universe?: string[];
  split_type?: string;
}

export interface MethodSeedOutcome {
  key: string;
  name: string;
  method_id?: string;
  method_hash?: string;
  status: string;
  confidence?: string;
  passable: boolean;
  oos_trades: number;
  oos_return: number;
  oos_max_drawdown: number;
  result_hash?: string;
  blockers?: string[];
  error?: string;
}

export interface MethodSeedResult {
  snapshot_id: string;
  universe_size: number;
  date_start: string;
  date_end: string;
  outcomes: MethodSeedOutcome[];
  registered: number;
  verified: number;
  finished_at: string;
}
