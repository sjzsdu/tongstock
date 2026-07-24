export interface StockItem {
  code: string;
  name?: string;
}

export interface BlockInfo {
  name: string;
  type: number;
  count: number;
  stocks?: string[];
  stocksWithNames?: { code: string; name: string }[];
}

export type CodesCacheEntry = { list: { Code?: string; Name?: string }[]; timestamp: number };

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

export type SourceTab = 'watchlist' | 'block';
export type SortKey = 'code' | 'name' | 'close' | 'change';

export const KTYPE_OPTIONS = [
  { value: 'day', label: '日K' },
  { value: 'week', label: '周K' },
  { value: '60m', label: '60分' },
  { value: '30m', label: '30分' },
  { value: '15m', label: '15分' },
];

export const SIGNAL_OPTIONS: { value: string; label: string; buy: boolean; desc: string }[] = [
  { value: '金叉', label: '金叉', buy: true, desc: 'DIF上穿DEA，MACD看涨信号' },
  { value: '死叉', label: '死叉', buy: false, desc: 'DIF下穿DEA，MACD看跌信号' },
  { value: '超卖', label: '超卖', buy: true, desc: 'KDJ指标J值低于0，可能反弹' },
  { value: '超买', label: '超买', buy: false, desc: 'KDJ指标J值高于100，可能回调' },
  { value: '跌破下轨', label: '跌破下轨', buy: true, desc: '价格跌破布林带下轨，超卖信号' },
  { value: '突破上轨', label: '突破上轨', buy: false, desc: '价格突破布林带上轨，超买信号' },
  { value: '多头排列', label: '多头排列', buy: true, desc: 'MA5>MA10>MA20，上升趋势' },
  { value: '空头排列', label: '空头排列', buy: false, desc: 'MA5<MA10<MA20，下降趋势' },
];

export const ALL_BLOCK_FILES = [
  { file: 'block_zs.dat', label: '指数', type: '2' },
  { file: 'block_fg.dat', label: '行业', type: '2' },
  { file: 'block_gn.dat', label: '概念', type: '2' },
  { file: 'block.dat', label: '综合', type: '' },
];

export const ROW_HEIGHT = 48;

export const STORAGE_KEY = 'tongstock_stocklist';
export const SCREEN_CACHE_KEY = 'tongstock_screen_cache';
export const CACHE_EXPIRY = 5 * 60 * 1000;