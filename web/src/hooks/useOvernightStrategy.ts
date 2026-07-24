import { useState, useEffect, useCallback, useMemo } from 'react';
import { api } from '../api/client';
import type { OvernightCandidate } from '../api/client';
import type { CustomPool, OvernightSourceTab, OvernightSortKey } from '../types/strategy';

interface UseOvernightStrategyReturn {
  sourceTab: OvernightSourceTab;
  setSourceTab: React.Dispatch<React.SetStateAction<OvernightSourceTab>>;
  customPools: CustomPool[];
  currentCustomPoolId: string;
  setCurrentCustomPoolId: React.Dispatch<React.SetStateAction<string>>;
  addCustomPool: (name: string) => void;
  deleteCustomPool: (poolId: string) => void;
  updateCustomPool: (poolId: string, updates: Partial<CustomPool>) => void;
  marketCodes: { code: string; name: string }[];
  marketLoading: boolean;
  results: OvernightCandidate[];
  failedCodes: { code: string; reason: string }[];
  total: number;
  hasLoaded: boolean;
  isOvernightTime: boolean;
  currentTime: string;
  loading: boolean;
  sortKey: OvernightSortKey;
  sortAsc: boolean;
  sortedResults: OvernightCandidate[];
  doScreen: (codes: string) => Promise<void>;
  handleSortChange: (key: OvernightSortKey) => void;
  loadMarketCodes: () => Promise<void>;
}

export function useOvernightStrategy(): UseOvernightStrategyReturn {
  const [sourceTab, setSourceTab] = useState<OvernightSourceTab>('watchlist');
  const [customPools, setCustomPools] = useState<CustomPool[]>(() => {
    const saved = localStorage.getItem('overnight_custom_pools');
    return saved ? JSON.parse(saved) : [
      { id: '1', name: '杨永兴池(50-200亿)', minMarketCap: 50, maxMarketCap: 200 },
    ];
  });
  const [currentCustomPoolId, setCurrentCustomPoolId] = useState(customPools[0]?.id || '');
  const [marketCodes, setMarketCodes] = useState<{ code: string; name: string }[]>([]);
  const [marketLoading, setMarketLoading] = useState(false);

  const [results, setResults] = useState<OvernightCandidate[]>([]);
  const [failedCodes, setFailedCodes] = useState<{ code: string; reason: string }[]>([]);
  const [total, setTotal] = useState(0);
  const [hasLoaded, setHasLoaded] = useState(false);
  const [isOvernightTime, setIsOvernightTime] = useState(false);
  const [currentTime, setCurrentTime] = useState('');
  const [loading, setLoading] = useState(false);

  const [sortKey, setSortKey] = useState<OvernightSortKey>('change_pct');
  const [sortAsc, setSortAsc] = useState(false);

  // Save custom pools to localStorage
  useEffect(() => {
    if (customPools.length > 0) {
      localStorage.setItem('overnight_custom_pools', JSON.stringify(customPools));
    }
  }, [customPools]);

  // Custom pool management
  const addCustomPool = useCallback((name: string) => {
    if (!name.trim()) return;
    const newPool: CustomPool = {
      id: Date.now().toString(),
      name: name.trim(),
      minMarketCap: 50,
      maxMarketCap: 200,
    };
    setCustomPools(prev => [...prev, newPool]);
    setCurrentCustomPoolId(newPool.id);
  }, []);

  const deleteCustomPool = useCallback((poolId: string) => {
    if (customPools.length <= 1) return;
    setCustomPools(prev => prev.filter(p => p.id !== poolId));
    if (currentCustomPoolId === poolId) {
      const remaining = customPools.filter(p => p.id !== poolId);
      setCurrentCustomPoolId(remaining[0]?.id || '');
    }
  }, [customPools, currentCustomPoolId]);

  const updateCustomPool = useCallback((poolId: string, updates: Partial<CustomPool>) => {
    setCustomPools(prev => prev.map(p => p.id === poolId ? { ...p, ...updates } : p));
  }, []);

  // Load market codes
  const loadMarketCodes = useCallback(async () => {
    setMarketLoading(true);
    try {
      const currentPool = customPools.find(p => p.id === currentCustomPoolId);
      const minCap = sourceTab === 'custom' && currentPool ? currentPool.minMarketCap : undefined;
      const maxCap = sourceTab === 'custom' && currentPool ? currentPool.maxMarketCap : undefined;
      const result = minCap != null || maxCap != null
        ? await api.codesWithMarketCap(minCap, maxCap)
        : await api.codesMarket();
      if (result.codes) {
        setMarketCodes(result.codes.map((item) => ({ code: item.code, name: item.name })));
      }
    } catch {
      setMarketCodes([]);
    } finally {
      setMarketLoading(false);
    }
  }, [sourceTab, currentCustomPoolId, customPools]);

  // Auto load market codes when source tab changes
  useEffect(() => {
    if (sourceTab === 'market' || sourceTab === 'custom') {
      void loadMarketCodes();
    }
  }, [sourceTab, loadMarketCodes]);

  // Strategy screening
  const doScreen = useCallback(async (codes: string) => {
    const codeArray = codes.split(',').filter(Boolean);
    if (codeArray.length === 0) return;

    setLoading(true);
    try {
      const currentPool = customPools.find(p => p.id === currentCustomPoolId);
      const minCap = sourceTab === 'custom' && currentPool ? currentPool.minMarketCap : undefined;
      const maxCap = sourceTab === 'custom' && currentPool ? currentPool.maxMarketCap : undefined;
      const response = await api.overnightArbitrage(codeArray, minCap, maxCap);

      setResults(response.final_candidates);
      setTotal(response.total);
      setFailedCodes(response.failed ?? []);
      setHasLoaded(true);
      setIsOvernightTime(response.is_overnight_time);
      setCurrentTime(response.current_time);
    } catch {
      // Handle error in calling component
    } finally {
      setLoading(false);
    }
  }, [sourceTab, currentCustomPoolId, customPools]);

  // Sorting
  const handleSortChange = useCallback((key: OvernightSortKey) => {
    if (sortKey === key) {
      setSortAsc((previous) => !previous);
      return;
    }
    setSortKey(key);
    setSortAsc(true);
  }, [sortKey]);

  const sortedResults = useMemo(() => {
    const list = [...results];
    const dir = sortAsc ? 1 : -1;
    list.sort((a, b) => {
      let va: number | string = 0;
      let vb: number | string = 0;
      switch (sortKey) {
        case 'code':
          va = a.code;
          vb = b.code;
          break;
        case 'name':
          va = a.name || '';
          vb = b.name || '';
          break;
        case 'price':
          va = a.price;
          vb = b.price;
          break;
        case 'change_pct':
          va = a.change_pct;
          vb = b.change_pct;
          break;
      }
      if (typeof va === 'string' && typeof vb === 'string') {
        return va.localeCompare(vb) * dir;
      }
      return ((va as number) - (vb as number)) * dir;
    });
    return list;
  }, [results, sortAsc, sortKey]);

  return {
    sourceTab,
    setSourceTab,
    customPools,
    currentCustomPoolId,
    setCurrentCustomPoolId,
    addCustomPool,
    deleteCustomPool,
    updateCustomPool,
    marketCodes,
    marketLoading,
    results,
    failedCodes,
    total,
    hasLoaded,
    isOvernightTime,
    currentTime,
    loading,
    sortKey,
    sortAsc,
    sortedResults,
    doScreen,
    handleSortChange,
    loadMarketCodes,
  };
}