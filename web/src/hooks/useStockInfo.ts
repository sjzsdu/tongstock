import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';

export interface StockInfoItem {
  code: string;
  name: string;
  exchange: string;
  price: number;
  marketCap: number;
  turnoverRate: number;
  changePct: number;
  volumeRatio?: number; // API may not return this field
}

export interface StockInfoFilters {
  minMarketCap: number | undefined;
  maxMarketCap: number | undefined;
  minPrice: number | undefined;
  maxPrice: number | undefined;
  minTurnoverRate: number | undefined;
  maxTurnoverRate: number | undefined;
  exchange: string;
  excludeST: boolean;
}

interface UseStockInfoReturn {
  stocks: StockInfoItem[];
  total: number;
  loading: boolean;
  filters: StockInfoFilters;
  setFilters: (filters: Partial<StockInfoFilters>) => void;
  resetFilters: () => void;
  refresh: () => void;
}

export function useStockInfo(): UseStockInfoReturn {
  const [stocks, setStocks] = useState<StockInfoItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [filters, setFiltersState] = useState<StockInfoFilters>({
    minMarketCap: undefined,
    maxMarketCap: undefined,
    minPrice: undefined,
    maxPrice: undefined,
    minTurnoverRate: undefined,
    maxTurnoverRate: undefined,
    exchange: '',
    excludeST: false,
  });

  const loadStocks = useCallback(async () => {
    setLoading(true);
    try {
      const result = await api.stockinfoList(
        filters.minMarketCap,
        filters.maxMarketCap,
        filters.exchange || undefined
      );
      let stockList = result.infos || [];
      
      if (filters.minPrice != null) {
        stockList = stockList.filter(s => s.price >= filters.minPrice!);
      }
      if (filters.maxPrice != null) {
        stockList = stockList.filter(s => s.price <= filters.maxPrice!);
      }
      if (filters.minTurnoverRate != null) {
        stockList = stockList.filter(s => s.turnoverRate >= filters.minTurnoverRate!);
      }
      if (filters.maxTurnoverRate != null) {
        stockList = stockList.filter(s => s.turnoverRate <= filters.maxTurnoverRate!);
      }
      if (filters.excludeST) {
        stockList = stockList.filter(s => !s.name.includes('ST') && !s.name.includes('*ST'));
      }
      
      setStocks(stockList);
      setTotal(stockList.length);
    } catch {
      setStocks([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [filters]);

  // 修复：添加防抖处理，500ms 延迟
  useEffect(() => {
    const timer = setTimeout(() => {
      void loadStocks();
    }, 500);
    
    return () => clearTimeout(timer);
  }, [loadStocks]);

  const setFilters = useCallback((newFilters: Partial<StockInfoFilters>) => {
    setFiltersState(prev => ({ ...prev, ...newFilters }));
  }, []);

  const resetFilters = useCallback(() => {
    setFiltersState({
      minMarketCap: undefined,
      maxMarketCap: undefined,
      minPrice: undefined,
      maxPrice: undefined,
      minTurnoverRate: undefined,
      maxTurnoverRate: undefined,
      exchange: '',
      excludeST: false,
    });
  }, []);

  return {
    stocks,
    total,
    loading,
    filters,
    setFilters,
    resetFilters,
    refresh: loadStocks,
  };
}