import { useState, useEffect, useCallback, useMemo } from 'react';
import { api } from '../api/client';
import type { ScreenResult, ScreenCodeStatus, KlineBatchSyncResult } from '../types/api';
import type { SourceTab, BlockInfo, SortKey } from '../types/screen';
import { SCREEN_CACHE_KEY, CACHE_EXPIRY, ALL_BLOCK_FILES } from '../types/screen';

interface UseScreenReturn {
  sourceTab: SourceTab;
  setSourceTab: React.Dispatch<React.SetStateAction<SourceTab>>;
  ktype: string;
  setKtype: React.Dispatch<React.SetStateAction<string>>;
  selectedSignals: string[];
  setSelectedSignals: React.Dispatch<React.SetStateAction<string[]>>;
  results: ScreenResult[];
  failedCodes: ScreenCodeStatus[];
  skippedCodes: ScreenCodeStatus[];
  cappedInfo: { maxCodes: number; reason: string } | null;
  hasScreenLoaded: boolean;
  total: number;
  loading: boolean;
  sortKey: SortKey;
  sortAsc: boolean;
  filteredResults: ScreenResult[];
  sortedResults: ScreenResult[];
  signalCounts: Record<string, number>;
  blockFile: string;
  setBlockFile: React.Dispatch<React.SetStateAction<string>>;
  blockData: BlockInfo[];
  selectedBlock: BlockInfo | null;
  setSelectedBlock: React.Dispatch<React.SetStateAction<BlockInfo | null>>;
  blockLoading: boolean;
  blockStocksLoading: boolean;
  blockSearch: string;
  setBlockSearch: React.Dispatch<React.SetStateAction<string>>;
  filteredBlocks: BlockInfo[];
  syncResult: KlineBatchSyncResult | null;
  setSyncResult: React.Dispatch<React.SetStateAction<KlineBatchSyncResult | null>>;
  doScreen: (resolvedCodes: string, retryCodes?: string) => Promise<void>;
  retryFailed: () => Promise<void>;
  handleSortChange: (key: SortKey) => void;
  handleSelectBlock: (block: BlockInfo) => void;
}

function getScreenCacheKey(ktypeVal: string, signalsVal: string[]): string {
  return `${ktypeVal}_${signalsVal.sort().join('_')}`;
}

function saveScreenResult(
  resultsData: ScreenResult[],
  failedData: ScreenCodeStatus[],
  skippedData: ScreenCodeStatus[],
  totalData: number,
  ktypeVal: string,
  signalsVal: string[]
): void {
  try {
    const cache = {
      key: getScreenCacheKey(ktypeVal, signalsVal),
      results: resultsData,
      failed: failedData,
      skipped: skippedData,
      total: totalData,
      timestamp: Date.now(),
    };
    localStorage.setItem(SCREEN_CACHE_KEY, JSON.stringify(cache));
  } catch {
    return;
  }
}

function loadScreenResult(ktypeVal: string, signalsVal: string[]): {
  results: ScreenResult[];
  failed: ScreenCodeStatus[];
  skipped: ScreenCodeStatus[];
  total: number;
} | null {
  try {
    const stored = localStorage.getItem(SCREEN_CACHE_KEY);
    if (!stored) return null;
    const cache = JSON.parse(stored);
    const expectedKey = getScreenCacheKey(ktypeVal, signalsVal);
    if (cache.key !== expectedKey) return null;
    if (Date.now() - cache.timestamp > CACHE_EXPIRY * 2) return null;
    return {
      results: cache.results || [],
      failed: cache.failed || [],
      skipped: cache.skipped || [],
      total: cache.total || 0,
    };
  } catch {
    return null;
  }
}

export function useScreen(): UseScreenReturn {
  const searchParams = new URLSearchParams(window.location.search);
  const urlKtype = searchParams.get('ktype') || 'day';
  const urlSignals = searchParams.get('signals')?.split(',').filter(Boolean) || [];

  const [sourceTab, setSourceTab] = useState<SourceTab>('watchlist');
  const [ktype, setKtype] = useState(urlKtype);
  const [selectedSignals, setSelectedSignals] = useState<string[]>(urlSignals);
  const [results, setResults] = useState<ScreenResult[]>([]);
  const [failedCodes, setFailedCodes] = useState<ScreenCodeStatus[]>([]);
  const [skippedCodes, setSkippedCodes] = useState<ScreenCodeStatus[]>([]);
  const [cappedInfo, setCappedInfo] = useState<{ maxCodes: number; reason: string } | null>(null);
  const [hasScreenLoaded, setHasScreenLoaded] = useState(false);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [sortKey, setSortKey] = useState<SortKey>('code');
  const [sortAsc, setSortAsc] = useState(true);

  const [blockFile, setBlockFile] = useState('block_zs.dat');
  const [blockData, setBlockData] = useState<BlockInfo[]>([]);
  const [selectedBlock, setSelectedBlock] = useState<BlockInfo | null>(null);
  const [blockLoading, setBlockLoading] = useState(false);
  const [blockStocksLoading, setBlockStocksLoading] = useState(false);
  const [blockSearch, setBlockSearch] = useState('');
  const [syncResult, setSyncResult] = useState<KlineBatchSyncResult | null>(null);

  const updateUrlParams = useCallback(() => {
    const params = new URLSearchParams();
    params.set('ktype', ktype);
    if (selectedSignals.length > 0) {
      params.set('signals', selectedSignals.join(','));
    }
    const newUrl = params.toString() ? `?${params.toString()}` : window.location.pathname;
    window.history.replaceState({}, '', newUrl);
  }, [ktype, selectedSignals]);

  useEffect(() => {
    updateUrlParams();
  }, [updateUrlParams]);

  const loadBlocks = useCallback(async (file: string, typeFilter?: string) => {
    setBlockLoading(true);
    try {
      const response = await api.blockList(file, typeFilter || undefined, true);
      setBlockData(response.blocks || []);
      setSelectedBlock(null);
    } catch {
      setBlockData([]);
    } finally {
      setBlockLoading(false);
    }
  }, []);

  useEffect(() => {
    if (sourceTab === 'block') {
      void loadBlocks(blockFile, ALL_BLOCK_FILES.find((item) => item.file === blockFile)?.type);
    }
  }, [sourceTab, blockFile, loadBlocks]);

  const loadBlockStocks = useCallback(async (block: BlockInfo) => {
    setBlockStocksLoading(true);
    try {
      const response = await api.blockShow(block.name, undefined, blockFile);
      if (response.stocks && response.stocks.length > 0) {
        const stocksWithNames = response.stocks.map((stock) => ({
          code: stock.code,
          name: stock.name?.trim() ? stock.name : stock.code,
        }));
        setSelectedBlock({
          ...block,
          stocks: response.stocks.map((stock) => stock.code),
          stocksWithNames,
        });
      } else {
        setSelectedBlock(block);
      }
    } catch {
      setSelectedBlock(block);
    } finally {
      setBlockStocksLoading(false);
    }
  }, [blockFile]);

  const handleSelectBlock = useCallback((block: BlockInfo) => {
    if (selectedBlock?.name === block.name) {
      setSelectedBlock(null);
      return;
    }
    void loadBlockStocks(block);
  }, [loadBlockStocks, selectedBlock]);

  const doScreen = useCallback(async (resolvedCodes: string, retryCodes?: string) => {
    const codes = (retryCodes ?? resolvedCodes).trim();
    if (!codes) return;

    if (!retryCodes) {
      const cached = loadScreenResult(ktype, selectedSignals);
      if (cached && cached.results.length > 0) {
        setResults(cached.results);
        setTotal(cached.total);
        setFailedCodes(cached.failed);
        setSkippedCodes(cached.skipped);
        setHasScreenLoaded(true);
        return;
      }
    }

    setLoading(true);
    try {
      const response = await api.screen(codes, ktype, selectedSignals);
      const valid = response.results.filter((item) => item.code);
      setResults(valid);
      setTotal(response.total);
      setFailedCodes(response.failed ?? []);
      setSkippedCodes(response.skipped ?? []);
      setCappedInfo(response.capped ? { maxCodes: response.maxCodes ?? 0, reason: response.reason ?? '' } : null);
      setHasScreenLoaded(true);

      if (!retryCodes) {
        saveScreenResult(valid, response.failed ?? [], response.skipped ?? [], response.total, ktype, selectedSignals);
      }
    } catch (screenError: unknown) {
      console.error('筛选失败:', screenError);
    } finally {
      setLoading(false);
    }
  }, [ktype, selectedSignals]);

  const retryFailed = useCallback(async () => {
    if (failedCodes.length === 0) return;
    const codes = failedCodes.map((item) => item.code).join(',');
    await doScreen(codes, codes);
  }, [failedCodes, doScreen]);

  const handleSortChange = useCallback((key: SortKey) => {
    if (sortKey === key) {
      setSortAsc((previous) => !previous);
      return;
    }
    setSortKey(key);
    setSortAsc(true);
  }, [sortKey]);

  const filteredResults = useMemo(() => {
    if (selectedSignals.length === 0) return results;
    return results.filter((result) => result.signals?.some((signal) => selectedSignals.includes(signal.Type)));
  }, [results, selectedSignals]);

  const sortedResults = useMemo(() => {
    const list = [...filteredResults];
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
        case 'close':
          va = a.last?.Close || 0;
          vb = b.last?.Close || 0;
          break;
        case 'change':
          const closeA = a.last?.Close || 0;
          const openA = a.last?.Open || closeA;
          va = openA > 0 ? ((closeA - openA) / openA) * 100 : 0;
          const closeB = b.last?.Close || 0;
          const openB = b.last?.Open || closeB;
          vb = openB > 0 ? ((closeB - openB) / openB) * 100 : 0;
          break;
      }
      if (typeof va === 'string' && typeof vb === 'string') {
        return va.localeCompare(vb) * dir;
      }
      return ((va as number) - (vb as number)) * dir;
    });
    return list;
  }, [filteredResults, sortAsc, sortKey]);

  const signalCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const result of results) {
      for (const signal of result.signals || []) {
        counts[signal.Type] = (counts[signal.Type] || 0) + 1;
      }
    }
    return counts;
  }, [results]);

  const filteredBlocks = useMemo(() => {
    const sorted = [...blockData].sort((a, b) => b.count - a.count);
    if (!blockSearch) return sorted;
    const query = blockSearch.toLowerCase();
    return sorted.filter((block) => block.name.toLowerCase().includes(query));
  }, [blockData, blockSearch]);

  return {
    sourceTab,
    setSourceTab,
    ktype,
    setKtype,
    selectedSignals,
    setSelectedSignals,
    results,
    failedCodes,
    skippedCodes,
    cappedInfo,
    hasScreenLoaded,
    total,
    loading,
    sortKey,
    sortAsc,
    filteredResults,
    sortedResults,
    signalCounts,
    blockFile,
    setBlockFile,
    blockData,
    selectedBlock,
    setSelectedBlock,
    blockLoading,
    blockStocksLoading,
    blockSearch,
    setBlockSearch,
    filteredBlocks,
    syncResult,
    setSyncResult,
    doScreen,
    retryFailed,
    handleSortChange,
    handleSelectBlock,
  };
}