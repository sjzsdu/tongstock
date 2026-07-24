import { useState, useEffect, useCallback, useMemo } from 'react';
import { api } from '../api/client';
import type { CustomStockPool, StockPoolFilter, MarketCodeItem } from '../types/api';

interface UseStockPoolReturn {
  pools: CustomStockPool[];
  currentPool: CustomStockPool | undefined;
  currentPoolId: string;
  loading: boolean;
  setCurrentPoolId: (id: string) => void;
  addPool: (pool: Omit<CustomStockPool, 'id' | 'createdAt' | 'updatedAt'>) => Promise<void>;
  updatePool: (pool: CustomStockPool) => Promise<void>;
  deletePool: (id: string) => Promise<void>;
}

export function useStockPool(): UseStockPoolReturn {
  const [pools, setPools] = useState<CustomStockPool[]>([]);
  const [currentPoolId, setCurrentPoolId] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadPools = async () => {
      try {
        const result = await api.stockpoolList();
        let poolData = result.pools;
        if (poolData.length === 0) {
          const defaultPool: CustomStockPool = {
            id: 'default',
            name: '杨永兴池(50-200亿)',
            description: '适合杨永兴策略的市值范围',
            filters: [{ field: 'marketCap', operator: 'between', value: [50, 200] }],
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          };
          await api.stockpoolUpsert(defaultPool);
          poolData = [defaultPool];
        }
        setPools(poolData);
        if (poolData.length > 0) {
          setCurrentPoolId(poolData[0].id);
        }
      } catch {
        const defaultPool: CustomStockPool = {
          id: 'default',
          name: '杨永兴池(50-200亿)',
          description: '适合杨永兴策略的市值范围',
          filters: [{ field: 'marketCap', operator: 'between', value: [50, 200] }],
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        };
        setPools([defaultPool]);
        setCurrentPoolId('default');
      } finally {
        setLoading(false);
      }
    };
    void loadPools();
  }, []);

  const currentPool = useMemo(() => pools.find(p => p.id === currentPoolId), [pools, currentPoolId]);

  const addPool = useCallback(async (poolData: Omit<CustomStockPool, 'id' | 'createdAt' | 'updatedAt'>) => {
    const now = new Date().toISOString();
    const newPool: CustomStockPool = {
      ...poolData,
      id: Date.now().toString(),
      createdAt: now,
      updatedAt: now,
    };
    await api.stockpoolUpsert(newPool);
    setPools(prev => [...prev, newPool]);
    setCurrentPoolId(newPool.id);
  }, []);

  const updatePool = useCallback(async (pool: CustomStockPool) => {
    const updated = { ...pool, updatedAt: new Date().toISOString() };
    await api.stockpoolUpsert(updated);
    setPools(prev => prev.map(p => p.id === pool.id ? updated : p));
  }, []);

  const deletePool = useCallback(async (id: string) => {
    await api.stockpoolDelete(id);
    setPools(prev => {
      const newPools = prev.filter(p => p.id !== id);
      if (currentPoolId === id && newPools.length > 0) {
        setCurrentPoolId(newPools[0].id);
      }
      return newPools;
    });
  }, [currentPoolId]);

  return {
    pools,
    currentPool,
    currentPoolId,
    loading,
    setCurrentPoolId,
    addPool,
    updatePool,
    deletePool,
  };
}

interface UsePoolStocksReturn {
  stocks: MarketCodeItem[];
  filteredStocks: MarketCodeItem[];
  loading: boolean;
  refresh: () => void;
}

export function usePoolStocks(pool: CustomStockPool | undefined): UsePoolStocksReturn {
  const [stocks, setStocks] = useState<MarketCodeItem[]>([]);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    if (!pool) {
      setStocks([]);
      return;
    }

    setLoading(true);
    try {
      const marketCapFilter = pool.filters.find((f) => f.field === 'marketCap');
      
      let codes: MarketCodeItem[] = [];
      if (marketCapFilter && marketCapFilter.operator === 'between') {
        const minCap = marketCapFilter.value[0] as number;
        const maxCap = marketCapFilter.value[1] as number;
        const result = await api.codesWithMarketCap(minCap, maxCap);
        if (result.codes) {
          codes = result.codes;
        }
      } else {
        const result = await api.codesMarket();
        if (result.codes) {
          codes = result.codes;
        }
      }
      setStocks(codes);
    } catch {
      setStocks([]);
    } finally {
      setLoading(false);
    }
  }, [pool]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const applyFilter = useCallback((codes: MarketCodeItem[], filter: StockPoolFilter): MarketCodeItem[] => {
    switch (filter.field) {
      case 'exchange':
        if (filter.operator === 'in') {
          return codes.filter(c => filter.value.includes(c.exchange));
        }
        break;
      case 'board':
        return codes.filter(c => {
          const code = c.code;
          let matched = false;
          if (filter.value.includes('main') && (code.startsWith('000') || code.startsWith('002') || code.startsWith('600') || code.startsWith('601'))) matched = true;
          if (filter.value.includes('chuangye') && (code.startsWith('300') || code.startsWith('301'))) matched = true;
          if (filter.value.includes('kechuang') && code.startsWith('688')) matched = true;
          if (filter.value.includes('beijiao') && code.startsWith('8')) matched = true;
          return matched;
        });
      case 'excludeST':
        if (filter.value[0] === true) {
          return codes.filter(c => !c.name.includes('ST') && !c.name.includes('*ST'));
        }
        break;
    }
    return codes;
  }, []);

  const filteredStocks = useMemo(() => {
    if (!pool || stocks.length === 0) return [];
    
    let filtered = [...stocks];
    for (const filter of pool.filters) {
      if (filter.field === 'marketCap') continue;
      filtered = applyFilter(filtered, filter);
    }
    return filtered;
  }, [pool, stocks, applyFilter]);

  return {
    stocks,
    filteredStocks,
    loading,
    refresh,
  };
}