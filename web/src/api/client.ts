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
} from '../types/api';

const BASE = '';

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || '请求失败');
  }
  const data = await res.json();
  // 检查响应是否包含错误字段
  if (data && typeof data === 'object' && 'error' in data) {
    throw new Error(data.error || '请求失败');
  }
  return data;
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

  saveScreenResults: (results: { code: string; name?: string }[]) =>
    Promise.all(results.map((item) => api.watchlistAdd(item.code, item.name))),

  syncDaily: (codes: string[], mode = 'auto', concurrency = 3) =>
    fetchJSON<KlineBatchSyncResult>('/api/sync/daily', {
      method: 'POST',
      body: JSON.stringify({ codes, mode, concurrency }),
    }),

  getSyncState: (code: string, ktype = 'day') =>
    fetchJSON<KlineSyncState>(`/api/sync/state?code=${encodeURIComponent(code)}&ktype=${ktype}`),

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
};

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
