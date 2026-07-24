import type { OvernightCandidate } from '../api/client';

export type OvernightSourceTab = 'watchlist' | 'block' | 'market' | 'custom';
export type OvernightSortKey = 'code' | 'name' | 'price' | 'change_pct';

export interface CustomPool {
  id: string;
  name: string;
  minMarketCap: number;
  maxMarketCap: number;
}

export interface StrategyResult {
  candidates: OvernightCandidate[];
  failedCodes: { code: string; reason: string }[];
  total: number;
  hasLoaded: boolean;
  isOvernightTime: boolean;
  currentTime: string;
}

export const OVERNIGHT_CRITERIA = [
  { key: 'price_change', label: '涨幅3%-5%' },
  { key: 'volume_ratio', label: '量比>1' },
  { key: 'turnover_rate', label: '换手5%-10%' },
  { key: 'market_cap', label: '市值50-200亿' },
  { key: 'recent_limit_up', label: '近20日涨停' },
  { key: 'ma_trend', label: 'MA多头排列' },
  { key: 'above_avg_price', label: '站均线上方' },
];