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

export const ALL_BLOCK_FILES = [
  { file: 'block_zs.dat', label: '指数', type: '2' },
  { file: 'block_fg.dat', label: '行业', type: '2' },
  { file: 'block_gn.dat', label: '概念', type: '2' },
  { file: 'block.dat', label: '综合', type: '' },
];

export const ROW_HEIGHT = 48;

export const STORAGE_KEY = 'tongstock_stocklist';
export const CACHE_EXPIRY = 5 * 60 * 1000;
export interface StateFilter {
  conditions: { dim: import('./api').StateDim; state: Exclude<import('./api').StateValue, 'insufficient_data'> }[];
  transition?: {
    dim: import('./api').StateDim;
    from: Exclude<import('./api').StateValue, 'insufficient_data'>[];
    to: Exclude<import('./api').StateValue, 'insufficient_data'>;
    within_days: number;
  };
}
