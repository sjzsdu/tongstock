import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { getChangePct } from '../utils/screen';
import { api } from '../api/client';
import type { ScreenResult, KlineBatchSyncResult, DimensionMeta, ScreenCoverage } from '../types/api';
import type { SourceTab, BlockInfo, SortKey, StateFilter } from '../types/screen';
import { ALL_BLOCK_FILES } from '../types/screen';

interface UseScreenReturn {
  sourceTab: SourceTab;
  setSourceTab: React.Dispatch<React.SetStateAction<SourceTab>>;
  ktype: string;
  stateFilter: StateFilter;
  setStateFilter: React.Dispatch<React.SetStateAction<StateFilter>>;
  date: string;
  setDate: React.Dispatch<React.SetStateAction<string>>;
  dimensions: DimensionMeta[];
  insufficientCount: number;
  missingCount: number;
  snapshotDate: string;
  coverage: ScreenCoverage | null;
  /** 场景 B：补齐当前筛选股票池的当日状态后自动重筛；返回同步结果，无池时返回 null */
  syncPoolAndRescreen: () => Promise<KlineBatchSyncResult | null>;
  gapSyncing: boolean;
  error: string;
  conditionsChanged: boolean;
  results: ScreenResult[];
  hasScreenLoaded: boolean;
  total: number;
  loading: boolean;
  /** 本次筛选执行时间；快照交易日另行展示。 */
  screenedAt: number | null;
  /** 用当前条件重新筛选上次完整股票池。 */
  forceRefresh: () => Promise<void>;
  sortKey: SortKey;
  sortAsc: boolean;
  filteredResults: ScreenResult[];
  sortedResults: ScreenResult[];
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
  doScreen: (resolvedCodes: string, dateOverride?: string) => Promise<void>;
  handleSortChange: (key: SortKey) => void;
  handleSelectBlock: (block: BlockInfo) => void;
}

export function useScreen(): UseScreenReturn {
  const [sourceTab, setSourceTab] = useState<SourceTab>('watchlist');
  const ktype = 'day';
  const [stateFilter, setStateFilter] = useState<StateFilter>(() => {
    try {
      const parsed = JSON.parse(new URLSearchParams(window.location.search).get('state_filter') || '{"conditions":[]}');
      if (!parsed || !Array.isArray(parsed.conditions) || parsed.conditions.some((c: unknown) => !c || typeof c !== 'object' || !('dim' in c) || !('state' in c))) return { conditions: [] };
      if (parsed.transition && (!Array.isArray(parsed.transition.from) || typeof parsed.transition.dim !== 'string' || typeof parsed.transition.to !== 'string' || typeof parsed.transition.within_days !== 'number')) return { conditions: [] };
      return parsed;
    }
    catch { return { conditions: [] }; }
  });
  const [date, setDate] = useState(new URLSearchParams(window.location.search).get('date') || '');
  const [dimensions, setDimensions] = useState<DimensionMeta[]>([]);
  const [insufficientCount, setInsufficientCount] = useState(0);
  const [missingCount, setMissingCount] = useState(0);
  const [snapshotDate, setSnapshotDate] = useState('');
  const [coverage, setCoverage] = useState<ScreenCoverage | null>(null);
  const [gapSyncing, setGapSyncing] = useState(false);
  const [error, setError] = useState('');
  const [submittedQuery, setSubmittedQuery] = useState('');
  const queryKey = JSON.stringify({ stateFilter, date, ktype });
  const conditionsChanged = !!submittedQuery && submittedQuery !== queryKey;
  const lastPool = useRef('');
  const requestId = useRef(0);
  useEffect(() => {
    api.screenOptions().then((res) => setDimensions(res.dimensions)).catch((err: unknown) => setError(err instanceof Error ? err.message : '状态枚举加载失败'));
  }, []);
  const [results, setResults] = useState<ScreenResult[]>([]);
  const [hasScreenLoaded, setHasScreenLoaded] = useState(false);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [screenedAt, setScreenedAt] = useState<number | null>(null);
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
    params.set('state_filter', JSON.stringify(stateFilter));
    if (date) params.set('date', date);
    const newUrl = params.toString() ? `?${params.toString()}` : window.location.pathname;
    window.history.replaceState({}, '', newUrl);
  }, [ktype, stateFilter, date]);

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

  const doScreen = useCallback(async (resolvedCodes: string, dateOverride?: string) => {
    const codes = resolvedCodes.trim();
    if (!codes) return;
    const queryDate = dateOverride ?? date;
    const submitted = JSON.stringify({ stateFilter, date: queryDate, ktype });
    lastPool.current = codes;
    const id = ++requestId.current;
    setLoading(true);
    setError('');
    try {
      const response = await api.screen(codes, ktype, stateFilter, queryDate);
      if (id !== requestId.current) return;
      setSubmittedQuery(submitted);
      setResults(response.results.filter((item) => item.code));
      setTotal(response.total);
      setDimensions(response.dimensions);
      setInsufficientCount(response.insufficientCount);
      setMissingCount(response.missingCount);
      setCoverage(response.coverage ?? null);
      setSnapshotDate(response.date);
      setScreenedAt(Date.now());
    } catch (err: unknown) {
      if (id !== requestId.current) return;
      setError(err instanceof Error ? err.message : '筛选失败');
      setResults([]);
      setCoverage(null);
    } finally {
      if (id === requestId.current) { setLoading(false); setHasScreenLoaded(true); }
    }
  }, [ktype, stateFilter, date]);

  const forceRefresh = useCallback(async () => {
    await doScreen(lastPool.current);
  }, [doScreen]);

  // 场景 B：补齐「当前筛选用的股票池」的当日状态（不是自选股），完成后自动重筛。
  const syncPoolAndRescreen = useCallback(async () => {
    const pool = lastPool.current.split(',').map((code) => code.trim()).filter(Boolean);
    if (pool.length === 0) return null;
    setGapSyncing(true);
    try {
      const result = await api.syncDaily(pool);
      await doScreen(lastPool.current);
      return result;
    } finally {
      setGapSyncing(false);
    }
  }, [doScreen]);

  const handleSortChange = useCallback((key: SortKey) => {
    if (sortKey === key) {
      setSortAsc((previous) => !previous);
      return;
    }
    setSortKey(key);
    setSortAsc(true);
  }, [sortKey]);

  // The snapshot API is authoritative; do not reapply legacy event OR matching.
  const filteredResults = results;

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
      va = getChangePct(a);
      vb = getChangePct(b);
      break;
      }
      if (typeof va === 'string' && typeof vb === 'string') {
        return va.localeCompare(vb) * dir;
      }
      return ((va as number) - (vb as number)) * dir;
    });
    return list;
  }, [filteredResults, sortAsc, sortKey]);

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
    stateFilter, setStateFilter, date, setDate, dimensions, insufficientCount, missingCount, snapshotDate, coverage, syncPoolAndRescreen, gapSyncing, error, conditionsChanged,
    results,
    hasScreenLoaded,
    total,
    loading,
    screenedAt,
    forceRefresh,
    sortKey,
    sortAsc,
    filteredResults,
    sortedResults,
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
    handleSortChange,
    handleSelectBlock,
  };
}