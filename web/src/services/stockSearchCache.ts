import { api } from '../api/client';
import type { SearchStockMatch, StockSearchIndexItem, StockSearchResponse } from '../types/api';

const CACHE_KEY = 'tongstock.stockSearchIndex.v2';
const CACHE_TTL_MS = 10 * 60 * 1000;
const BACKGROUND_REFRESH_INTERVAL_MS = 60 * 1000;

interface CachePayload {
  fetchedAt: number;
  items: StockSearchIndexItem[];
}

let memoryCache: CachePayload | null = null;
let inflight: Promise<CachePayload> | null = null;
let lastBackgroundRefreshAt = 0;

function readStoredCache(): CachePayload | null {
  if (typeof window === 'undefined') return null;
  if (memoryCache) return memoryCache;
  try {
    const raw = window.localStorage.getItem(CACHE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as CachePayload;
    if (!Array.isArray(parsed.items) || typeof parsed.fetchedAt !== 'number') return null;
    memoryCache = parsed;
    return parsed;
  } catch {
    return null;
  }
}

function writeCache(items: StockSearchIndexItem[]): CachePayload {
  const payload = { fetchedAt: Date.now(), items };
  memoryCache = payload;
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(CACHE_KEY, JSON.stringify(payload));
    } catch {
      // localStorage may be full or disabled, memory cache is enough for this session.
    }
  }
  return payload;
}

function isFresh(cache: CachePayload | null): cache is CachePayload {
  return !!cache && Date.now() - cache.fetchedAt < CACHE_TTL_MS;
}

async function refreshStockIndex(): Promise<CachePayload> {
  if (!inflight) {
    inflight = api.stockSearchIndex().then((response) => writeCache(response.items)).finally(() => {
      inflight = null;
    });
  }
  return inflight;
}

export async function getStockSearchIndex(forceRefresh = false): Promise<CachePayload> {
  const cached = readStoredCache();
  if (!forceRefresh && isFresh(cached)) return cached;
  if (!forceRefresh && cached) {
    void refreshStockIndex().catch(() => undefined);
    return cached;
  }
  return refreshStockIndex();
}

export function maybeRefreshStockSearchIndex(): void {
  const now = Date.now();
  if (now - lastBackgroundRefreshAt < BACKGROUND_REFRESH_INTERVAL_MS) return;
  lastBackgroundRefreshAt = now;
  const cached = readStoredCache();
  if (!isFresh(cached)) {
    void refreshStockIndex().catch(() => undefined);
  }
}

export function searchCachedStocks(query: string, items: StockSearchIndexItem[], limit: number): StockSearchResponse {
  const normalizedQuery = normalizeSearchText(query);
  const normalizedCode = normalizeCodeQuery(query);
  const scored = items
    .map((item) => {
      const result = scoreStock(item, normalizedQuery, normalizedCode);
      return result ? { item, ...result } : null;
    })
    .filter((item): item is { item: StockSearchIndexItem; score: number; matchType: string } => !!item)
    .sort((a, b) => b.score - a.score || a.item.code.localeCompare(b.item.code) || a.item.name.localeCompare(b.item.name))
    .slice(0, Math.max(1, limit));

  const matches: SearchStockMatch[] = scored.map(({ item, matchType }) => ({
    code: item.code,
    name: item.name,
    exchange: item.exchange,
    matchType,
  }));
  return { query, total: matches.length, exact: matches.length === 1 && matches[0].matchType.startsWith('exact_'), resolved: matches.length === 1, matches };
}

function scoreStock(item: StockSearchIndexItem, normalizedQuery: string, normalizedCode: string): { score: number; matchType: string } | null {
  if (normalizedCode) {
    if (item.code === normalizedCode) return { score: 1000, matchType: 'exact_code' };
    if (item.code.startsWith(normalizedCode)) return { score: 900, matchType: 'prefix_code' };
    if (item.code.includes(normalizedCode)) return { score: 760, matchType: 'contains_code' };
  }
  if (!normalizedQuery) return null;
  const pinyin = item.pinyin || '';
  const initials = item.initials || '';
  if (item.nameNorm === normalizedQuery) return { score: 980, matchType: 'exact_name' };
  if (pinyin === normalizedQuery) return { score: 970, matchType: 'exact_pinyin' };
  if (initials === normalizedQuery) return { score: 960, matchType: 'exact_initials' };
  if (item.nameNorm.startsWith(normalizedQuery)) return { score: 880, matchType: 'prefix_name' };
  if (pinyin.startsWith(normalizedQuery)) return { score: 870, matchType: 'prefix_pinyin' };
  if (initials.startsWith(normalizedQuery)) return { score: 860, matchType: 'prefix_initials' };
  if (item.nameNorm.includes(normalizedQuery)) return { score: 780, matchType: 'contains_name' };
  if (pinyin.includes(normalizedQuery)) return { score: 770, matchType: 'contains_pinyin' };
  if (initials.includes(normalizedQuery)) return { score: 765, matchType: 'contains_initials' };
  return null;
}

function normalizeSearchText(input: string): string {
  return input.trim().toLowerCase().replace(/[\s_-]/g, '');
}

function normalizeCodeQuery(input: string): string {
  const normalized = normalizeSearchText(input);
  const withoutExchange = /^(sh|sz|bj)\d{6}$/.test(normalized) ? normalized.slice(2) : normalized;
  return /^\d{1,6}$/.test(withoutExchange) ? withoutExchange : '';
}
