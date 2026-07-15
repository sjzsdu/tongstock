import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode, RefObject } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  CloseOutlined,
  DownloadOutlined,
  EyeOutlined,
  PlusOutlined,
  SaveOutlined,
  SearchOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { useVirtualizer } from '@tanstack/react-virtual';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Empty,
  Flex,
  Input,
  List,
  Modal,
  Radio,
  Segmented,
  Space,
  Spin,
  Statistic,
  Tag,
  Typography,
  message,
} from 'antd';
import { api } from '../api/client';
import type { ScreenCodeStatus, ScreenResult } from '../types/api';

const { Paragraph, Text, Title } = Typography;

const KTYPE_OPTIONS = [
  { value: 'day', label: '日K' },
  { value: 'week', label: '周K' },
  { value: '60m', label: '60分' },
  { value: '30m', label: '30分' },
  { value: '15m', label: '15分' },
];

const SIGNAL_OPTIONS: { value: string; label: string; buy: boolean }[] = [
  { value: '金叉', label: '金叉', buy: true },
  { value: '死叉', label: '死叉', buy: false },
  { value: '超买', label: '超买', buy: false },
  { value: '超卖', label: '超卖', buy: true },
  { value: '突破上轨', label: '突破上轨', buy: false },
  { value: '跌破下轨', label: '跌破下轨', buy: true },
  { value: '多头排列', label: '多头排列', buy: true },
  { value: '空头排列', label: '空头排列', buy: false },
];

const ALL_BLOCK_FILES = [
  { file: 'block_zs.dat', label: '指数', type: '2' },
  { file: 'block_fg.dat', label: '行业', type: '2' },
  { file: 'block_gn.dat', label: '概念', type: '2' },
  { file: 'block.dat', label: '综合', type: '' },
];

type SourceTab = 'watchlist' | 'block';
type SortKey = 'code' | 'name' | 'close' | 'change' | 'dif' | 'k' | 'j';

interface StockItem {
  code: string;
  name?: string;
}

interface BlockInfo {
  name: string;
  type: number;
  count: number;
  stocks?: string[];
  stocksWithNames?: { code: string; name: string }[];
}

type CodesCacheEntry = { list: { Code?: string; Name?: string }[]; timestamp: number };

const ROW_HEIGHT = 58;

function getLastValue(arr: number[] | undefined): number {
  if (!arr || arr.length === 0) return 0;
  return arr[arr.length - 1];
}

function isBuySignal(type: string): boolean {
  return SIGNAL_OPTIONS.find((signal) => signal.value === type)?.buy ?? false;
}

function stockNamesFromCodesCache(
  codes: string[],
  codesCache: Record<string, CodesCacheEntry>,
): { code: string; name: string }[] {
  const grouped: Record<string, string[]> = { sz: [], sh: [], bj: [] };
  for (const code of codes) {
    if (code.startsWith('6')) grouped.sh.push(code);
    else if (code.startsWith('8') || code.startsWith('9')) grouped.bj.push(code);
    else grouped.sz.push(code);
  }

  const results: { code: string; name: string }[] = [];
  for (const [exchange, codeList] of Object.entries(grouped)) {
    if (codeList.length === 0) continue;
    const cached = codesCache[exchange];
    if (!cached) continue;
    for (const code of codeList) {
      const stockInfo = cached.list.find((item) => item.Code === code);
      if (stockInfo?.Name) {
        results.push({ code, name: stockInfo.Name });
      }
    }
  }
  return results;
}

function formatPercent(value: number): string {
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`;
}

function getChangePct(result: ScreenResult): number {
  const close = result.last?.Close || 0;
  const open = result.last?.Open || close;
  return open > 0 ? ((close - open) / open) * 100 : 0;
}

function getPriceColor(value: number): string {
  if (value > 0) return 'var(--ant-color-error)';
  if (value < 0) return 'var(--ant-color-success)';
  return 'var(--ant-color-text-secondary)';
}

function getMaTrend(result: ScreenResult): { label: string; color: string } {
  const n = result.ma?.['5']?.length || 0;
  const ma5 = result.ma?.['5']?.[n - 1] ?? 0;
  const ma10 = result.ma?.['10']?.[n - 1] ?? 0;
  const ma20 = result.ma?.['20']?.[n - 1] ?? 0;

  if (ma5 > ma10 && ma10 > ma20) {
    return { label: '↗ 多头', color: 'red' };
  }
  if (ma5 < ma10 && ma10 < ma20) {
    return { label: '↘ 空头', color: 'green' };
  }
  return { label: '→ 震荡', color: 'default' };
}

function exportCsv(filename: string, headers: string[], rows: string[][]) {
  const csv = [headers, ...rows]
    .map((row) => row.map((cell) => `"${String(cell ?? '').replace(/"/g, '""')}"`).join(','))
    .join('\n');
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

function SortHeader({
  sortKey,
  sortAsc,
  current,
  onChange,
  align = 'left',
  children,
}: {
  sortKey: SortKey;
  sortAsc: boolean;
  current: SortKey;
  onChange: (key: SortKey) => void;
  align?: 'left' | 'right';
  children: ReactNode;
}) {
  const active = current === sortKey;

  return (
    <button
      type="button"
      onClick={() => onChange(sortKey)}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: align === 'right' ? 'flex-end' : 'flex-start',
        gap: 4,
        width: '100%',
        border: 'none',
        background: 'transparent',
        color: active ? 'var(--ant-color-text)' : 'var(--ant-color-text-secondary)',
        fontSize: 12,
        cursor: 'pointer',
      }}
    >
      <span>{children}</span>
      {active ? (sortAsc ? <ArrowUpOutlined /> : <ArrowDownOutlined />) : <span style={{ opacity: 0.35 }}>↕</span>}
    </button>
  );
}

function VirtualResultTable({
  results,
  tableContainerRef,
  sortKey,
  sortAsc,
  onSortChange,
  navigate,
}: {
  results: ScreenResult[];
  tableContainerRef: RefObject<HTMLDivElement | null>;
  sortKey: SortKey;
  sortAsc: boolean;
  onSortChange: (key: SortKey) => void;
  navigate: (path: string) => void;
}) {
  const rowVirtualizer = useVirtualizer({
    count: results.length,
    getScrollElement: () => tableContainerRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 18,
  });

  return (
    <Card bodyStyle={{ padding: 0 }}>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '88px 1.2fr 96px 96px 88px 72px 56px 56px 1.5fr',
          gap: 0,
          padding: '0 16px',
          borderBottom: '1px solid var(--ant-color-border-secondary)',
          background: 'var(--ant-color-fill-quaternary)',
        }}
      >
        <div style={{ padding: '12px 0' }}><SortHeader sortKey="code" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>代码</SortHeader></div>
        <div style={{ padding: '12px 12px' }}><SortHeader sortKey="name" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>名称</SortHeader></div>
        <div style={{ padding: '12px 0' }}><SortHeader sortKey="close" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">收盘</SortHeader></div>
        <div style={{ padding: '12px 0' }}><SortHeader sortKey="change" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">涨跌幅</SortHeader></div>
        <div style={{ padding: '12px 0', textAlign: 'right', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>MA趋势</div>
        <div style={{ padding: '12px 0' }}><SortHeader sortKey="dif" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">DIF</SortHeader></div>
        <div style={{ padding: '12px 0' }}><SortHeader sortKey="k" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">K</SortHeader></div>
        <div style={{ padding: '12px 0' }}><SortHeader sortKey="j" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">J</SortHeader></div>
        <div style={{ padding: '12px 12px', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>信号</div>
      </div>

      <div ref={tableContainerRef} style={{ maxHeight: 'calc(100vh - 360px)', minHeight: 320, overflow: 'auto' }}>
        <div style={{ height: rowVirtualizer.getTotalSize(), position: 'relative' }}>
          {rowVirtualizer.getVirtualItems().map((virtualRow) => {
            const result = results[virtualRow.index];
            const close = result.last?.Close || 0;
            const changePct = getChangePct(result);
            const dif = getLastValue(result.macd?.DIF);
            const kValue = getLastValue(result.kdj?.K);
            const jValue = getLastValue(result.kdj?.J);
            const recentSignals = result.signals?.slice(-5) || [];
            const maTrend = getMaTrend(result);

            return (
              <div
                key={result.code}
                onClick={() => navigate(`/stock/${result.code}/chart`)}
                style={{
                  position: 'absolute',
                  top: virtualRow.start,
                  left: 0,
                  width: '100%',
                  height: ROW_HEIGHT,
                  padding: '0 16px',
                  display: 'grid',
                  gridTemplateColumns: '88px 1.2fr 96px 96px 88px 72px 56px 56px 1.5fr',
                  alignItems: 'center',
                  borderBottom: '1px solid var(--ant-color-border-secondary)',
                  cursor: 'pointer',
                  background: virtualRow.index % 2 === 0 ? 'transparent' : 'var(--ant-color-fill-quaternary)',
                }}
              >
                <Text code>{result.code}</Text>
                <Text ellipsis style={{ padding: '0 12px' }}>{result.name || '-'}</Text>
                <Text style={{ textAlign: 'right', color: getPriceColor(changePct), fontVariantNumeric: 'tabular-nums' }}>{close.toFixed(2)}</Text>
                <Text style={{ textAlign: 'right', color: getPriceColor(changePct), fontVariantNumeric: 'tabular-nums' }}>{formatPercent(changePct)}</Text>
                <Tag color={maTrend.color} style={{ justifySelf: 'end' }}>{maTrend.label}</Tag>
                <Text style={{ textAlign: 'right', color: getPriceColor(dif), fontVariantNumeric: 'tabular-nums' }}>{dif.toFixed(2)}</Text>
                <Text style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{kValue.toFixed(1)}</Text>
                <Text style={{ textAlign: 'right', color: jValue > 100 ? '#fa8c16' : jValue < 0 ? '#1677ff' : undefined, fontVariantNumeric: 'tabular-nums' }}>{jValue.toFixed(1)}</Text>
                <Space size={[4, 4]} wrap style={{ paddingLeft: 12 }}>
                  {recentSignals.map((signal, index) => (
                    <Tag key={`${result.code}-${signal.Type}-${index}`} color={isBuySignal(signal.Type) ? 'red' : 'green'}>
                      {signal.Indicator}{signal.Type}
                    </Tag>
                  ))}
                </Space>
              </div>
            );
          })}
        </div>
      </div>
    </Card>
  );
}

export default function Screen() {
  const navigate = useNavigate();
  const tableContainerRef = useRef<HTMLDivElement>(null);
  const [messageApi, contextHolder] = message.useMessage();

  const searchParams = new URLSearchParams(window.location.search);
  const urlKtype = searchParams.get('ktype') || 'day';
  const urlSignals = searchParams.get('signals')?.split(',').filter(Boolean) || [];

  const STORAGE_KEY = 'tongstock_stocklist';
  const CACHE_EXPIRY = 5 * 60 * 1000;

  const loadStockListFromStorage = useCallback((): StockItem[] => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      return stored ? JSON.parse(stored) : [];
    } catch {
      return [];
    }
  }, []);

  const saveStockListToStorage = useCallback((list: StockItem[]) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
    } catch {
      return;
    }
  }, []);

  const [codesCache, setCodesCache] = useState<Record<string, CodesCacheEntry>>({});
  const [sourceTab, setSourceTab] = useState<SourceTab>('watchlist');
  const [stockList, setStockList] = useState<StockItem[]>(() => loadStockListFromStorage());
  const [inputCode, setInputCode] = useState('');
  const [inputLoading, setInputLoading] = useState(false);
  const [ktype, setKtype] = useState(urlKtype);
  const [selectedSignals, setSelectedSignals] = useState<string[]>(urlSignals);
  const [results, setResults] = useState<ScreenResult[]>([]);
  const [failedCodes, setFailedCodes] = useState<ScreenCodeStatus[]>([]);
  const [skippedCodes, setSkippedCodes] = useState<ScreenCodeStatus[]>([]);
  const [cappedInfo, setCappedInfo] = useState<{ maxCodes: number; reason: string } | null>(null);
  const [hasScreenLoaded, setHasScreenLoaded] = useState(false);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [syncLoading, setSyncLoading] = useState(false);
  const [error, setError] = useState('');
  const [sortKey, setSortKey] = useState<SortKey>('code');
  const [sortAsc, setSortAsc] = useState(true);
  const [blockFile, setBlockFile] = useState('block_zs.dat');
  const [blockData, setBlockData] = useState<BlockInfo[]>([]);
  const [selectedBlock, setSelectedBlock] = useState<BlockInfo | null>(null);
  const [blockLoading, setBlockLoading] = useState(false);
  const [blockStocksLoading, setBlockStocksLoading] = useState(false);
  const [blockSearch, setBlockSearch] = useState('');
  const [showBlockModal, setShowBlockModal] = useState(false);
  const [blockStocksWithNames, setBlockStocksWithNames] = useState<{ code: string; name: string }[]>([]);
  const [blockStocksLoadingNames, setBlockStocksLoadingNames] = useState(false);

  useEffect(() => {
    saveStockListToStorage(stockList);
  }, [stockList, saveStockListToStorage]);

  useEffect(() => {
    api.watchlist()
      .then((items) => {
        if (items.length === 0) return;
        setStockList((previous) => {
          const merged = [...previous];
          for (const item of items) {
            if (!merged.some((stock) => stock.code === item.code)) {
              merged.push({ code: item.code, name: item.name });
            }
          }
          return merged;
        });
      })
      .catch(() => {});
  }, []);

  const preloadCodesCache = useCallback(async (): Promise<Record<string, CodesCacheEntry>> => {
    const exchanges = ['sz', 'sh', 'bj'] as const;
    const merged: Record<string, CodesCacheEntry> = { ...codesCache };
    await Promise.all(
      exchanges.map(async (exchange) => {
        if (!merged[exchange] || Date.now() - merged[exchange].timestamp >= CACHE_EXPIRY) {
          try {
            const codesList = await api.codes(exchange);
            merged[exchange] = { list: codesList, timestamp: Date.now() };
          } catch {
            return;
          }
        }
      }),
    );
    setCodesCache(merged);
    return merged;
  }, [codesCache]);

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

  useEffect(() => {
    if (sourceTab === 'block') {
      void loadBlocks(blockFile, ALL_BLOCK_FILES.find((item) => item.file === blockFile)?.type);
    }
  }, [sourceTab, blockFile, loadBlocks]);

  const resolvedCodes = useMemo(() => {
    if (sourceTab === 'block' && selectedBlock?.stocks) {
      return selectedBlock.stocks.join(',');
    }
    return stockList.map((stock) => stock.code).join(',');
  }, [selectedBlock, sourceTab, stockList]);

  const updateUrlParams = useCallback(() => {
    const params = new URLSearchParams();
    params.set('ktype', ktype);
    if (selectedSignals.length > 0) {
      params.set('signals', selectedSignals.join(','));
    }
    const newUrl = params.toString() ? `?${params.toString()}` : window.location.pathname;
    window.history.replaceState({}, '', newUrl);
  }, [ktype, selectedSignals]);

  const doScreen = async (retryCodes?: string) => {
    const codes = (retryCodes ?? resolvedCodes).trim();
    if (!codes) return;

    setLoading(true);
    setError('');
    try {
      const response = await api.screen(codes, ktype);
      const valid = response.results.filter((item) => item.code);
      setResults(valid);
      setTotal(response.total);
      setFailedCodes(response.failed ?? []);
      setSkippedCodes(response.skipped ?? []);
      setCappedInfo(response.capped ? { maxCodes: response.maxCodes ?? 0, reason: response.reason ?? '' } : null);
      setHasScreenLoaded(true);
    } catch (screenError: unknown) {
      setError(screenError instanceof Error ? screenError.message : '筛选失败');
    } finally {
      setLoading(false);
    }
  };

  const retryFailed = async () => {
    if (failedCodes.length === 0) return;
    const codes = failedCodes.map((item) => item.code).join(',');
    await doScreen(codes);
  };

  useEffect(() => {
    const codes = resolvedCodes.trim();
    if (codes && !hasScreenLoaded && !loading) {
      void doScreen();
    }
  }, [resolvedCodes, hasScreenLoaded, loading]);

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
          va = getChangePct(a);
          vb = getChangePct(b);
          break;
        case 'dif':
          va = getLastValue(a.macd?.DIF);
          vb = getLastValue(b.macd?.DIF);
          break;
        case 'k':
          va = getLastValue(a.kdj?.K);
          vb = getLastValue(b.kdj?.K);
          break;
        case 'j':
          va = getLastValue(a.kdj?.J);
          vb = getLastValue(b.kdj?.J);
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

  const toggleSignal = (signal: string) => {
    setSelectedSignals((previous) => (
      previous.includes(signal)
        ? previous.filter((item) => item !== signal)
        : [...previous, signal]
    ));
  };

  useEffect(() => {
    updateUrlParams();
  }, [updateUrlParams]);

  const handleSortChange = (key: SortKey) => {
    if (sortKey === key) {
      setSortAsc((previous) => !previous);
      return;
    }
    setSortKey(key);
    setSortAsc(true);
  };

  const addCodesFromInput = async () => {
    const codes = inputCode
      .split(/[, \n]+/)
      .map((value) => value.trim().toUpperCase())
      .filter(Boolean);

    if (codes.length === 0) return;

    const invalidCodes = codes.filter((value) => !/^\d{6}$/.test(value));
    if (invalidCodes.length > 0) {
      messageApi.error(`无效的股票代码: ${invalidCodes.join(', ')}`);
      return;
    }

    const existingCodes = codes.filter((value) => stockList.some((stock) => stock.code === value));
    if (existingCodes.length > 0) {
      messageApi.warning(`股票已存在: ${existingCodes.join(', ')}`);
    }

    const newCodes = codes.filter((value) => !stockList.some((stock) => stock.code === value));
    if (newCodes.length === 0) {
      setInputCode('');
      return;
    }

    setInputLoading(true);
    try {
      const cache = await preloadCodesCache();
      const resolved = stockNamesFromCodesCache(newCodes, cache);
      if (resolved.length === 0) {
        messageApi.error('股票代码不存在');
      } else {
        setStockList((previous) => [...previous, ...resolved]);
        resolved.forEach((stock) => api.watchlistAdd(stock.code, stock.name).catch(() => {}));
        messageApi.success(resolved.length === 1 ? `已添加 ${resolved[0].name}` : `已添加 ${resolved.length} 只股票`);
      }
    } catch {
      messageApi.error('获取股票信息失败');
    } finally {
      setInputLoading(false);
      setInputCode('');
    }
  };

  const openBlockModal = async () => {
    if (!selectedBlock?.stocks?.length) return;
    setShowBlockModal(true);

    if (selectedBlock.stocksWithNames?.length) {
      setBlockStocksWithNames(selectedBlock.stocksWithNames);
      return;
    }

    setBlockStocksLoadingNames(true);
    try {
      const cache = await preloadCodesCache();
      const rows = stockNamesFromCodesCache(selectedBlock.stocks, cache);
      const byCode = new Map(rows.map((row) => [row.code, row.name]));
      const filled = selectedBlock.stocks.map((code) => ({
        code,
        name: byCode.get(code) ?? code,
      }));
      setBlockStocksWithNames(filled);
    } finally {
      setBlockStocksLoadingNames(false);
    }
  };

  const addAllBlockStocksToWatchlist = () => {
    const newStocks = blockStocksWithNames
      .filter((stock) => !stockList.some((watch) => watch.code === stock.code))
      .map((stock) => ({ code: stock.code, name: stock.name }));

    if (newStocks.length === 0) {
      messageApi.warning('所有股票已存在');
      return;
    }

    setStockList((previous) => [...previous, ...newStocks]);
    newStocks.forEach((stock) => api.watchlistAdd(stock.code, stock.name).catch(() => {}));
    messageApi.success(`已添加 ${newStocks.length} 只股票`);
  };

  const exportScreenResults = () => {
    exportCsv(
      `tongstock-screen-${new Date().toISOString().slice(0, 10)}.csv`,
      ['代码', '名称', '收盘', '涨跌幅', 'MA趋势', 'DIF', 'K', 'J', '信号'],
      sortedResults.map((result) => {
        const maTrend = getMaTrend(result);
        return [
          result.code,
          result.name || '',
          String(result.last?.Close ?? ''),
          formatPercent(getChangePct(result)),
          maTrend.label,
          getLastValue(result.macd?.DIF).toFixed(4),
          getLastValue(result.kdj?.K).toFixed(2),
          getLastValue(result.kdj?.J).toFixed(2),
          (result.signals || []).map((signal) => `${signal.Indicator}${signal.Type}`).join(';'),
        ];
      }),
    );
  };

  const saveScreenResults = async () => {
    if (sortedResults.length === 0) return;
    try {
      await api.saveScreenResults(sortedResults.map((item) => ({ code: item.code, name: item.name })));
      setStockList((previous) => {
        const merged = [...previous];
        for (const item of sortedResults) {
          if (!merged.some((stock) => stock.code === item.code)) merged.push({ code: item.code, name: item.name });
        }
        return merged;
      });
      messageApi.success(`已保存 ${sortedResults.length} 只命中股票到自选股`);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '保存失败');
    }
  };

  const syncWatchlistDaily = async () => {
    const codes = stockList.map((stock) => stock.code);
    if (codes.length === 0) return;
    setSyncLoading(true);
    try {
      const result = await api.syncDaily(codes, 'auto', 3);
      if (result.failed > 0) {
        messageApi.warning(`同步完成：成功 ${result.success} 只，失败 ${result.failed} 只`);
      } else {
        messageApi.success(`同步完成：${result.success} 只自选股日K已更新`);
      }
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '同步失败');
    } finally {
      setSyncLoading(false);
    }
  };

  return (
    <>
      {contextHolder}
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Flex justify="space-between" align="center" wrap="wrap" gap={12}>
          <div>
            <Title level={3} style={{ margin: 0 }}>信号筛选</Title>
            <Paragraph type="secondary" style={{ marginBottom: 0 }}>
              从自选股或板块成分股中批量计算指标信号，并快速跳转到个股详情。
            </Paragraph>
          </div>
          <Segmented<SourceTab>
            value={sourceTab}
            onChange={(value) => setSourceTab(value)}
            options={[
              { label: '自选股', value: 'watchlist' },
              { label: '板块', value: 'block' },
            ]}
          />
        </Flex>

        <div style={{ display: 'grid', gridTemplateColumns: '320px minmax(0, 1fr)', gap: 16, alignItems: 'start' }}>
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Card title={sourceTab === 'watchlist' ? '自选股列表' : '板块来源'}>
              {sourceTab === 'watchlist' ? (
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                  <Input
                    prefix={<SearchOutlined />}
                    value={inputCode}
                    onChange={(event) => setInputCode(event.target.value)}
                    onPressEnter={() => void addCodesFromInput()}
                    placeholder="输入股票代码，支持逗号/空格分隔"
                    suffix={inputLoading ? <Spin size="small" /> : null}
                  />
                  <Flex justify="space-between" align="center" gap={8}>
                    <Text type="secondary">共 {stockList.length} 只股票</Text>
                    <Button
                      size="small"
                      icon={<SyncOutlined spin={syncLoading} />}
                      loading={syncLoading}
                      disabled={stockList.length === 0}
                      onClick={() => void syncWatchlistDaily()}
                    >
                      同步日K
                    </Button>
                  </Flex>
                  <div style={{ maxHeight: 420, overflow: 'auto' }}>
                    {stockList.length === 0 ? (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="输入股票代码后回车添加" />
                    ) : (
                      <List
                        size="small"
                        dataSource={stockList}
                        renderItem={(stock, index) => (
                          <List.Item
                            style={{ cursor: 'pointer' }}
                            onClick={() => navigate(`/stock/${stock.code}/chart`)}
                            actions={[
                              <Button
                                key="remove"
                                type="text"
                                danger
                                icon={<CloseOutlined />}
								  onClick={(event) => {
								    event.stopPropagation();
								    api.watchlistDelete(stock.code).catch(() => {});
								    setStockList((previous) => previous.filter((_, itemIndex) => itemIndex !== index));
								  }}
                              />,
                            ]}
                          >
                            <List.Item.Meta
                              title={<Space><Text code>{stock.code}</Text><Text>{stock.name || '-'}</Text></Space>}
                            />
                          </List.Item>
                        )}
                      />
                    )}
                  </div>
                </Space>
              ) : (
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                  <Segmented
                    block
                    value={blockFile}
                    onChange={(value) => {
                      const nextFile = String(value);
                      const config = ALL_BLOCK_FILES.find((item) => item.file === nextFile);
                      setBlockFile(nextFile);
                      void loadBlocks(nextFile, config?.type);
                    }}
                    options={ALL_BLOCK_FILES.map((item) => ({ label: item.label, value: item.file }))}
                  />
                  <Input
                    prefix={<SearchOutlined />}
                    value={blockSearch}
                    onChange={(event) => setBlockSearch(event.target.value)}
                    placeholder="搜索板块..."
                  />
                  <div style={{ maxHeight: 420, overflow: 'auto' }}>
                    {blockLoading ? (
                      <Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin /></Flex>
                    ) : (
                      <List
                        size="small"
                        dataSource={filteredBlocks}
                        renderItem={(block) => (
                          <List.Item
                            style={{
                              cursor: 'pointer',
                              borderRadius: 8,
                              paddingInline: 12,
                              background: selectedBlock?.name === block.name ? 'var(--ant-color-primary-bg)' : undefined,
                            }}
                            onClick={() => handleSelectBlock(block)}
                          >
                            <Flex justify="space-between" align="center" style={{ width: '100%' }}>
                              <Text ellipsis style={{ maxWidth: 180 }}>{block.name}</Text>
                              <Tag>{block.count}只</Tag>
                            </Flex>
                          </List.Item>
                        )}
                      />
                    )}
                  </div>
                  {selectedBlock && (
                    <Alert
                      type="info"
                      showIcon
                      message={`已选 ${selectedBlock.name}`}
                      description={
                        <Space wrap>
                          <Text>{blockStocksLoading ? '加载成分股中...' : `${selectedBlock.stocks?.length || selectedBlock.count} 只股票`}</Text>
                          <Button size="small" icon={<EyeOutlined />} onClick={() => void openBlockModal()} disabled={!selectedBlock.stocks?.length}>
                            查看成分股
                          </Button>
                        </Space>
                      }
                    />
                  )}
                </Space>
              )}
            </Card>
          </Space>

          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Card>
					<Space direction="vertical" size={16} style={{ width: '100%' }}>
						<Flex justify="space-between" align="center" wrap="wrap" gap={12}>
						  <Space wrap>
                    <Text type="secondary">周期</Text>
                    <Radio.Group
                      size="small"
                      optionType="button"
                      buttonStyle="solid"
                      value={ktype}
                      onChange={(event) => setKtype(event.target.value)}
                      options={KTYPE_OPTIONS}
                    />
						  </Space>
						  <Space wrap>
						    <Button icon={<DownloadOutlined />} onClick={exportScreenResults} disabled={sortedResults.length === 0}>
						      导出CSV
						    </Button>
						    <Button icon={<SaveOutlined />} onClick={() => void saveScreenResults()} disabled={sortedResults.length === 0}>
						      保存结果
						    </Button>
						    <Button
						      type="primary"
						      icon={<SearchOutlined />}
						      loading={loading}
						      onClick={() => void doScreen()}
						      disabled={!resolvedCodes.trim()}
						    >
						      开始筛选
						    </Button>
						  </Space>
						</Flex>

                <div>
                  <Text type="secondary">信号过滤</Text>
                  <div style={{ marginTop: 8 }}>
                    <Space size={[8, 8]} wrap>
                      {SIGNAL_OPTIONS.map((option) => {
                        const active = selectedSignals.includes(option.value);
                        return (
                          <Tag
                            key={option.value}
                            color={active ? (option.buy ? 'red' : 'green') : 'default'}
                            style={{ cursor: 'pointer', paddingInline: 10, paddingBlock: 4 }}
                            onClick={() => toggleSignal(option.value)}
                          >
                            {option.label}
                          </Tag>
                        );
                      })}
                      {selectedSignals.length > 0 && (
                        <Button size="small" type="text" onClick={() => setSelectedSignals([])}>
                          清空
                        </Button>
                      )}
                    </Space>
                  </div>
                </div>
              </Space>
            </Card>

            {error && <Alert type="error" showIcon message="筛选失败" description={error} />}

            {cappedInfo && (
              <Alert type="warning" showIcon message="批量已截断" description={cappedInfo.reason} />
            )}

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: 16 }}>
              <Card><Statistic title="扫描总数" value={total} suffix="只" /></Card>
              <Card><Statistic title="命中结果" value={filteredResults.length} suffix="只" /></Card>
              <Card><Statistic title="活跃信号" value={Object.keys(signalCounts).length} suffix="种" /></Card>
              {hasScreenLoaded && failedCodes.length > 0 && (
                <Card><Statistic title="失败" value={failedCodes.length} suffix="只" valueStyle={{ color: '#cf1322' }} /></Card>
              )}
              {hasScreenLoaded && skippedCodes.length > 0 && (
                <Card><Statistic title="跳过" value={skippedCodes.length} suffix="只" valueStyle={{ color: '#faad14' }} /></Card>
              )}
            </div>

            {hasScreenLoaded && failedCodes.length > 0 && (
              <Collapse
                items={[{
                  key: 'failed',
                  label: <Space><Tag color="error">失败 {failedCodes.length}</Tag><Text type="secondary">点击查看详情</Text></Space>,
                  children: (
                    <Space direction="vertical" size={8} style={{ width: '100%' }}>
                      <Flex justify="flex-end">
                        <Button size="small" icon={<SyncOutlined />} loading={loading} onClick={() => void retryFailed()}>
                          重试失败项
                        </Button>
                      </Flex>
                      <List
                        size="small"
                        dataSource={failedCodes}
                        renderItem={(item) => (
                          <List.Item>
                            <Flex justify="space-between" align="center" style={{ width: '100%' }}>
                              <Space>
                                <Text code>{item.code}</Text>
                                {item.name && <Text type="secondary">{item.name}</Text>}
                              </Space>
                              <Text type="danger" style={{ fontSize: 12 }}>{item.reason}</Text>
                            </Flex>
                          </List.Item>
                        )}
                      />
                    </Space>
                  ),
                }]}
              />
            )}

            {results.length > 0 && (
              <Card>
                <Space size={[8, 8]} wrap>
                  {Object.entries(signalCounts).map(([type, count]) => (
                    <Tag key={type} color={isBuySignal(type) ? 'red' : 'green'}>
                      {type} {count}
                    </Tag>
                  ))}
                </Space>
              </Card>
            )}

            {sortedResults.length > 0 ? (
              <VirtualResultTable
                results={sortedResults}
                tableContainerRef={tableContainerRef}
                sortKey={sortKey}
                sortAsc={sortAsc}
                onSortChange={handleSortChange}
                navigate={navigate}
              />
            ) : !loading && !error ? (
              <Card>
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={hasScreenLoaded ? '当前筛选条件下没有命中结果' : '选择股票来源后点击“开始筛选”'}
                />
              </Card>
            ) : null}
          </Space>
        </div>
      </Space>

      <Modal
        open={showBlockModal}
        onCancel={() => setShowBlockModal(false)}
        footer={[
          <Button key="close" onClick={() => setShowBlockModal(false)}>关闭</Button>,
          <Button key="add-all" type="primary" icon={<PlusOutlined />} onClick={addAllBlockStocksToWatchlist}>
            全部加入自选
          </Button>,
        ]}
        width={760}
        title={selectedBlock ? `${selectedBlock.name} 成分股` : '成分股'}
      >
        {blockStocksLoadingNames ? (
          <Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin /></Flex>
        ) : (
          <List
            grid={{ gutter: 12, column: 2 }}
            dataSource={blockStocksWithNames}
            renderItem={(stock) => {
              const inWatchlist = stockList.some((item) => item.code === stock.code);
              return (
                <List.Item>
                  <Card size="small" hoverable onClick={() => {
                    setShowBlockModal(false);
                    navigate(`/stock/${stock.code}/chart`);
                  }}>
                    <Flex justify="space-between" align="center" gap={12}>
                      <Space direction="vertical" size={2}>
                        <Text code>{stock.code}</Text>
                        <Text>{stock.name}</Text>
                      </Space>
                      <Button
                        size="small"
                        type={inWatchlist ? 'default' : 'primary'}
                        icon={inWatchlist ? <CloseOutlined /> : <PlusOutlined />}
                        onClick={(event) => {
                          event.stopPropagation();
								  if (inWatchlist) {
								    api.watchlistDelete(stock.code).catch(() => {});
								    setStockList((previous) => previous.filter((item) => item.code !== stock.code));
								    messageApi.success(`已移除 ${stock.name}`);
								  } else {
								    api.watchlistAdd(stock.code, stock.name).catch(() => {});
								    setStockList((previous) => [...previous, { code: stock.code, name: stock.name }]);
                            messageApi.success(`已添加 ${stock.name}`);
                          }
                        }}
                      >
                        {inWatchlist ? '移除' : '加入自选'}
                      </Button>
                    </Flex>
                  </Card>
                </List.Item>
              );
            }}
          />
        )}
      </Modal>
    </>
  );
}
