import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';
import type { KlineBatchSyncResult } from '../types/api';
import type { StockItem } from '../types/screen';
import { STORAGE_KEY } from '../types/screen';

interface UseWatchlistReturn {
  stockList: StockItem[];
  setStockList: React.Dispatch<React.SetStateAction<StockItem[]>>;
  addCodes: (codes: string[], cache: Record<string, { list: { Code?: string; Name?: string }[] }>) => Promise<void>;
  removeCode: (code: string) => void;
  syncWatchlistDaily: () => Promise<KlineBatchSyncResult | undefined>;
}

export function mergeWatchlist(previous: StockItem[], remote: { code: string; name?: string }[]): StockItem[] {
  const merged = [...previous];
  for (const item of remote) {
    if (!merged.some((stock) => stock.code === item.code)) {
      merged.push({ code: item.code, name: item.name || item.code });
    }
  }
  return merged;
}

export function resolveWatchlistAdditions(
  codes: string[],
  cache: Record<string, { list: { Code?: string; Name?: string }[] }>,
): { code: string; name: string }[] {
  const grouped: Record<string, string[]> = { sz: [], sh: [], bj: [] };
  for (const code of codes) {
    if (code.startsWith('6')) grouped.sh.push(code);
    else if (code.startsWith('8') || code.startsWith('9')) grouped.bj.push(code);
    else grouped.sz.push(code);
  }
  const results: { code: string; name: string }[] = [];
  for (const [exchange, codeList] of Object.entries(grouped)) {
    const cached = cache[exchange];
    if (!cached) continue;
    for (const code of codeList) {
      const stockInfo = cached.list.find((item) => item.Code === code);
      if (stockInfo?.Name && !results.some((item) => item.code === code)) {
        results.push({ code, name: stockInfo.Name });
      }
    }
  }
  return results;
}

export function useWatchlist(): UseWatchlistReturn {
  const [stockList, setStockList] = useState<StockItem[]>(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      return stored ? JSON.parse(stored) : [];
    } catch {
      return [];
    }
  });

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(stockList));
  }, [stockList]);

  useEffect(() => {
    api.watchlist()
      .then((items) => {
        if (items.length === 0) return;
        setStockList((previous) => mergeWatchlist(previous, items));
      })
      .catch(() => {});
  }, []);

  const addCodes = useCallback(async (codes: string[], cache: Record<string, { list: { Code?: string; Name?: string }[] }>) => {
    const results = resolveWatchlistAdditions(codes, cache);

    if (results.length > 0) {
      setStockList((previous) => [...previous, ...results]);
      results.forEach((stock) => api.watchlistAdd(stock.code, stock.name).catch(() => {}));
    }
  }, []);

  const removeCode = useCallback((code: string) => {
    api.watchlistDelete(code).catch(() => {});
    setStockList((previous) => previous.filter((item) => item.code !== code));
  }, []);

  const syncWatchlistDaily = useCallback(async () => {
    const codes = stockList.map((stock) => stock.code);
    if (codes.length === 0) return;
    try {
      const result = await api.syncDaily(codes, 'auto', 3);
      if (result.failed > 0) {
        console.warn(`同步完成：成功 ${result.success} 只，失败 ${result.failed} 只`);
      } else {
        console.log(`同步完成：${result.success} 只自选股日K已更新`);
      }
      return result;
    } catch (error) {
      console.error('同步失败:', error);
      throw error;
    }
  }, [stockList]);

  return { stockList, setStockList, addCodes, removeCode, syncWatchlistDaily };
}
