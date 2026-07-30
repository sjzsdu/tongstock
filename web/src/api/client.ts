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
  EvidenceCard,
  HypothesisPreviewRequest,
  HypothesisPreviewResponse,
  ChatSessionInfo,
  NewsItem,
  NewsSummary,
  FeedResult,
  HotEvent,
  EventResult,
  MarketSentiment,
  SentimentTrend,
  SentimentHeatmapItem,
  AlertRecord,
  AlertRule,
  SyncFreshnessResult,
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
      throw new TongStockAPIError(payload.error.code, payload.error.message, payload.error.request_id, res.status);
    }
    throw new TongStockAPIError('http_error', typeof payload?.error === 'string' ? payload.error : '请求失败', undefined, res.status);
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

  constructor(
    code: string,
    message: string,
    requestId?: string,
    status?: number,
  ) {
    super(message);
    this.name = 'TongStockAPIError';
    this.code = code;
    this.requestId = requestId;
    this.status = status;
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

  companyContent: (code: string, blockOrCategory: string | { Name: string; Filename: string; Start: number; Length: number }) => {
    const params = new URLSearchParams({ code });
    if (typeof blockOrCategory === 'string') {
      params.set('block', blockOrCategory);
    } else {
      params.set('filename', blockOrCategory.Filename);
      params.set('start', String(blockOrCategory.Start));
      params.set('length', String(blockOrCategory.Length));
    }
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
  paradigmAnalyze: (stockCode: string, stockName?: string, days?: number, forceRefresh = false) =>
    fetchJSON<ParadigmAnalyzeResponse>('/api/paradigm/analyze', {
      method: 'POST',
      body: JSON.stringify({ stock_code: stockCode, stock_name: stockName, days, force_refresh: forceRefresh }),
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

  paradigmGet: (id: string) =>
    fetchJSON<ParadigmItem>(`/api/paradigm/${id}`),

  paradigmEvidence: (id: string) =>
    fetchJSON<EvidenceCard>(`/api/paradigm/${id}/evidence`),

  paradigmCreate: (payload: {
    name: string;
    side: string;
    stock_code: string;
    stock_name?: string;
    rationale?: string;
    logic?: string;
    features?: string[];
    baseline?: string;
    buy_conditions: { indicator: string; operator: string; value: string }[];
    sell_conditions?: {
      take_profit?: { indicator: string; operator: string; value: string }[];
      stop_loss?: { indicator: string; operator: string; value: string }[];
    };
    confirmations?: string[];
    invalidations: string[];
    expectation: {
      holding_period: string;
      expected_return: string;
      risk_reward_ratio: string;
      confidence: number;
    };
    tags?: string[];
  }) =>
    fetchJSON<{ paradigm: ParadigmItem; valid: boolean; errors?: string[]; warnings?: string[] }>(
      '/api/paradigm/hypothesis',
      {
        method: 'POST',
        body: JSON.stringify(payload),
      }
    ),

  paradigmPreview: (payload: HypothesisPreviewRequest) =>
    fetchJSON<HypothesisPreviewResponse>(
      '/api/paradigm/hypothesis/preview',
      {
        method: 'POST',
        body: JSON.stringify(payload),
      }
    ),

  paradigmEvaluate: (stockCode: string) =>
    fetchJSON<ParadigmEvaluateResponse>('/api/paradigm/evaluate', {
      method: 'POST',
      body: JSON.stringify({ stock_code: stockCode }),
    }),

  paradigmAlerts: (stockCode?: string) =>
    fetchJSON<ParadigmAlertsResponse>(`/api/paradigm/alerts${stockCode ? `?stock_code=${encodeURIComponent(stockCode)}` : ''}`),

  paradigmStats: () =>
    fetchJSON<ParadigmStatsResponse>('/api/paradigm/stats'),

  paradigmBacktest: (id?: string, stockCode?: string) => {
    const params = new URLSearchParams();
    if (id) params.set('id', id);
    if (stockCode) params.set('stock_code', stockCode);
    const q = params.toString();
    return fetchJSON<ParadigmBacktestItem[]>(`/api/paradigm/backtest${q ? `?${q}` : ''}`);
  },

  paradigmHistory: () =>
    fetchJSON<ParadigmListResponse>('/api/paradigm/history'),

  paradigmReview: (id: string, review: { review_status: string; review_note?: string; review_rating?: number; actual_return?: number }) =>
    fetchJSON<ParadigmItem>(`/api/paradigm/${id}/review`, {
      method: 'PUT',
      body: JSON.stringify(review),
    }),

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
		if (params?.startTime) p.set('startTime', params.startTime);
		if (params?.endTime) p.set('endTime', params.endTime);
		if (params?.hotScoreMin != null) p.set('hotScoreMin', String(params.hotScoreMin));
		if (params?.page != null) p.set('page', String(params.page));
		if (params?.pageSize != null) p.set('pageSize', String(params.pageSize));
		if (params?.sortBy) p.set('sortBy', params.sortBy);
		const q = p.toString();
		return fetchJSON<FeedResult>(`/api/news/feed${q ? '?' + q : ''}`);
	},

	newsItem: (id: string) =>
		fetchJSON<NewsItem>(`/api/news/item/${id}`),

	newsStock: (code: string) =>
		fetchJSON<FeedResult>(`/api/news/stock/${code}`),

	newsSearch: (keyword: string) =>
		fetchJSON<{ total: number; items: NewsSummary[] }>(`/api/news/search?keyword=${encodeURIComponent(keyword)}`),

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
};

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
