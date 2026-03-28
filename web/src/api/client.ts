import type {
  Quote, KlineItem, IndicatorData, Finance, XdXrItem,
  CompanyCategory, MinuteItem, TradeItem, AuctionItem,
  BlockItem, CodeItem, IndexBar, ScreenResponse, SignalAnalysis,
  HistoryStock,
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
  return res.json();
}

export const api = {
  quote: (code: string) =>
    fetchJSON<Quote>(`/api/quote?code=${code}`),

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

  company: (code: string) =>
    fetchJSON<CompanyCategory[]>(`/api/company?code=${code}`),

  companyContent: (code: string, block: string) =>
    fetchJSON<{ content: string }>(`/api/company/content?code=${code}&block=${encodeURIComponent(block)}`),

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

  screen: (codes: string, type = 'day', signal?: string) => {
    const p = new URLSearchParams({ codes, type });
    if (signal) p.set('signal', signal);
    return fetchJSON<ScreenResponse>(`/api/screen?${p}`);
  },

  signalAnalysis: (code: string, type = 'day') =>
    fetchJSON<SignalAnalysis>(`/api/signal-analysis?code=${code}&type=${type}`),

  history: () =>
    fetchJSON<{ data: HistoryStock[] }>('/api/history').then(r => r.data),

  historyAdd: (code: string) =>
    fetchJSON<{ message: string }>('/api/history', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),

  historyDelete: (code: string) =>
    fetchJSON<{ message: string }>(`/api/history/${code}`, {
      method: 'DELETE',
    }),
};
