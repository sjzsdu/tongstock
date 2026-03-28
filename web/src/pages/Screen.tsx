import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search, X, ChevronDown, ChevronUp, ArrowUpDown, ExternalLink } from 'lucide-react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { api } from '../api/client';
import type { ScreenResult } from '../types/api';

// ── Constants ──────────────────────────────────────────────────────────────────

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

// 板块文件列表
const ALL_BLOCK_FILES = [
  { file: 'block_zs.dat', label: '指数', type: '2' },
  { file: 'block_fg.dat', label: '行业', type: '2' },
  { file: 'block_gn.dat', label: '概念', type: '2' },
  { file: 'block.dat', label: '综合', type: '' },
];

// ── Types ──────────────────────────────────────────────────────────────────────

type SourceTab = 'watchlist' | 'block';
type SortKey = 'code' | 'name' | 'close' | 'change' | 'dif' | 'k' | 'j';

// ── Helpers ────────────────────────────────────────────────────────────────────

// 手动输入的股票项
interface StockItem {
  code: string;
  name?: string;
}

// 使用 blockList API 返回的板块维度数据
interface BlockInfo {
  name: string;
  type: number;
  count: number;
  stocks?: string[];
  /** 板块接口已带名称，弹窗直接使用，避免依赖异步 codes 缓存闭包 */
  stocksWithNames?: { code: string; name: string }[];
}

function getLastValue(arr: number[] | undefined): number {
  if (!arr || arr.length === 0) return 0;
  return arr[arr.length - 1];
}

function isBuySignal(type: string): boolean {
  return SIGNAL_OPTIONS.find(s => s.value === type)?.buy ?? false;
}

type CodesCacheEntry = { list: { Code?: string; Name?: string }[]; timestamp: number };

/** 用已合并的代码表缓存解析股票名称（不依赖 React state 闭包） */
function stockNamesFromCodesCache(codes: string[], codesCache: Record<string, CodesCacheEntry>): { code: string; name: string }[] {
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
      const stockInfo = cached.list.find(c => c.Code === code);
      if (stockInfo?.Name) {
        results.push({ code, name: stockInfo.Name });
      }
    }
  }
  return results;
}

// ── Virtual Result Table ───────────────────────────────────────────────────────

const ROW_HEIGHT = 40;

function VirtualResultTable({ results, tableContainerRef, SortHeader, navigate }: {
  results: ScreenResult[];
  tableContainerRef: React.RefObject<HTMLDivElement | null>;
  SortHeader: React.FC<{ k: SortKey; children: React.ReactNode; className?: string }>;
  navigate: (path: string) => void;
}) {
  const rowVirtualizer = useVirtualizer({
    count: results.length,
    getScrollElement: () => tableContainerRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 20,
  });

  return (
    <div className="flex-1 min-h-0 bg-slate-900 rounded-lg border border-slate-800 flex flex-col overflow-hidden">
      <div className="grid grid-cols-[80px_1fr_80px_80px_70px_70px_50px_50px_1fr] border-b border-slate-800 text-slate-400 text-xs">
        <SortHeader k="code" className="text-left">代码</SortHeader>
        <SortHeader k="name" className="text-left">名称</SortHeader>
        <SortHeader k="close" className="text-right">收盘</SortHeader>
        <SortHeader k="change" className="text-right">涨跌幅</SortHeader>
        <div className="p-3 text-right text-xs">MA趋势</div>
        <SortHeader k="dif" className="text-right">DIF</SortHeader>
        <SortHeader k="k" className="text-right">K</SortHeader>
        <SortHeader k="j" className="text-right">J</SortHeader>
        <div className="p-3 text-left text-xs">信号</div>
      </div>
      <div ref={tableContainerRef} className="overflow-auto flex-1">
        <div style={{ height: `${rowVirtualizer.getTotalSize()}px`, position: 'relative' }}>
          {rowVirtualizer.getVirtualItems().map(virtualRow => {
            const r = results[virtualRow.index];
            const n = r.ma?.['5']?.length || 0;
            const ma5 = r.ma?.['5']?.[n - 1] ?? 0;
            const ma10 = r.ma?.['10']?.[n - 1] ?? 0;
            const ma20 = r.ma?.['20']?.[n - 1] ?? 0;
            const dif = getLastValue(r.macd?.DIF);
            const kVal = getLastValue(r.kdj?.K);
            const jVal = getLastValue(r.kdj?.J);
            const close = r.last?.Close || 0;
            const open = r.last?.Open || close;
            const chgPct = open > 0 ? (close - open) / open * 100 : 0;
            const up = chgPct >= 0;
            const maTrend = ma5 > ma10 && ma10 > ma20 ? 'bull' : ma5 < ma10 && ma10 < ma20 ? 'bear' : 'mixed';
            const recentSignals = r.signals?.slice(-5) || [];

            return (
              <div
                key={r.code}
                onClick={() => navigate(`/stock/${r.code}/chart`)}
                className="grid grid-cols-[80px_1fr_80px_80px_70px_70px_50px_50px_1fr] items-center text-sm border-b border-slate-800/30 hover:bg-slate-800/50 cursor-pointer transition-colors absolute w-full"
                style={{ height: `${ROW_HEIGHT}px`, top: `${virtualRow.start}px` }}
              >
                <div className="px-3 font-mono text-blue-400 text-xs truncate">{r.code}</div>
                <div className="px-3 text-white truncate">{r.name || '-'}</div>
                <div className={`px-3 text-right font-mono ${up ? 'text-red-400' : 'text-green-400'}`}>{close.toFixed(2)}</div>
                <div className={`px-3 text-right font-mono ${up ? 'text-red-400' : 'text-green-400'}`}>
                  {chgPct > 0 ? '+' : ''}{chgPct.toFixed(2)}%
                </div>
                <div className="px-3 text-right">
                  <span className={`text-xs ${maTrend === 'bull' ? 'text-red-400' : maTrend === 'bear' ? 'text-green-400' : 'text-slate-500'}`}>
                    {maTrend === 'bull' ? '↗多头' : maTrend === 'bear' ? '↘空头' : '→震荡'}
                  </span>
                </div>
                <div className={`px-3 text-right font-mono text-xs ${dif > 0 ? 'text-red-400' : 'text-green-400'}`}>{dif.toFixed(2)}</div>
                <div className="px-3 text-right font-mono text-xs">{kVal.toFixed(1)}</div>
                <div className={`px-3 text-right font-mono text-xs ${jVal > 100 ? 'text-orange-400' : jVal < 0 ? 'text-blue-400' : ''}`}>{jVal.toFixed(1)}</div>
                <div className="px-3 flex gap-1 flex-wrap overflow-hidden">
                  {recentSignals.map((s, j) => (
                    <span
                      key={j}
                      className={`px-1.5 py-0.5 rounded text-xs ${
                        isBuySignal(s.Type)
                          ? 'bg-red-600/15 text-red-400 border border-red-500/20'
                          : 'bg-green-600/15 text-green-400 border border-green-500/20'
                      }`}
                    >
                      {s.Indicator}{s.Type}
                    </span>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

// ── Main Component ─────────────────────────────────────────────────────────────

export default function Screen() {
  const navigate = useNavigate();
  const tableContainerRef = useRef<HTMLDivElement>(null);

  const STORAGE_KEY = 'tongstock_stocklist';

  // 从 localStorage 加载股票列表
  const loadStockListFromStorage = useCallback((): StockItem[] => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      return stored ? JSON.parse(stored) : [];
    } catch { return []; }
  }, []);

  // 保存股票列表到 localStorage
  const saveStockListToStorage = useCallback((list: StockItem[]) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
    } catch { /* 忽略存储错误 */ }
  }, []);

  // 股票代码缓存 - 按交易所存储
  const [codesCache, setCodesCache] = useState<Record<string, CodesCacheEntry>>({});
  const CACHE_EXPIRY = 5 * 60 * 1000; // 缓存5分钟

  /** 拉取各所代码表并返回合并后的缓存（调用方立即用于解析，避免 setState 异步导致闭包仍为空） */
  const preloadCodesCache = useCallback(async (): Promise<Record<string, CodesCacheEntry>> => {
    const exchanges = ['sz', 'sh', 'bj'] as const;
    const merged: Record<string, CodesCacheEntry> = { ...codesCache };
    await Promise.all(
      exchanges.map(async (exchange) => {
        if (!merged[exchange] || Date.now() - merged[exchange].timestamp >= CACHE_EXPIRY) {
          try {
            const codesList = await api.codes(exchange);
            merged[exchange] = { list: codesList, timestamp: Date.now() };
          } catch { /* 忽略错误 */ }
        }
      })
    );
    setCodesCache(merged);
    return merged;
  }, [codesCache]);

  const [sourceTab, setSourceTab] = useState<SourceTab>('watchlist');
  // 使用懒加载初始化，从 localStorage 读取
  const [stockList, setStockList] = useState<StockItem[]>(() => loadStockListFromStorage());

  // 保存到 localStorage
  useEffect(() => {
    saveStockListToStorage(stockList);
  }, [stockList, saveStockListToStorage]);
  const [inputCode, setInputCode] = useState('');
  const [inputLoading, setInputLoading] = useState(false);
  const [ktype, setKtype] = useState('day');
  const [selectedSignals, setSelectedSignals] = useState<string[]>([]);
  const [results, setResults] = useState<ScreenResult[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [sortKey, setSortKey] = useState<SortKey>('code');
  const [sortAsc, setSortAsc] = useState(true);

  const [blockFile, setBlockFile] = useState('block_zs.dat');
  const [blockData, setBlockData] = useState<BlockInfo[]>([]);
  const [selectedBlock, setSelectedBlock] = useState<BlockInfo | null>(null);
  const [blockLoading, setBlockLoading] = useState(false);
  const [blockStocksLoading, setBlockStocksLoading] = useState(false);
  const [blockSearch, setBlockSearch] = useState('');

  // Toast 提示
  const [toast, setToast] = useState<{ message: string; type: 'error' | 'success' } | null>(null);

  // 成分股弹窗
  const [showBlockModal, setShowBlockModal] = useState(false);
  const [blockStocksWithNames, setBlockStocksWithNames] = useState<{ code: string; name: string }[]>([]);
  const [blockStocksLoadingNames, setBlockStocksLoadingNames] = useState(false);

  // 显示 Toast
  const showToast = useCallback((message: string, type: 'error' | 'success' = 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  }, []);

  // ── Block loading ──

  const loadBlocks = useCallback(async (file: string, typeFilter?: string) => {
    setBlockLoading(true);
    try {
      // 使用新的 blockList API，返回板块维度的数据
      const res = await api.blockList(file, typeFilter || undefined, true);
      setBlockData(res.blocks || []);
      setSelectedBlock(null);
    } catch { setBlockData([]); }
    finally { setBlockLoading(false); }
  }, []);

  // 加载板块成分股
  const loadBlockStocks = useCallback(async (block: BlockInfo) => {
    setBlockStocksLoading(true);
    try {
      // 调用 blockShow API 获取成分股列表
      const res = await api.blockShow(block.name, undefined, blockFile);
      if (res.stocks && res.stocks.length > 0) {
        const stocksWithNames = res.stocks.map(s => ({
          code: s.code,
          name: (s.name && s.name.trim()) ? s.name : s.code,
        }));
        const codes = res.stocks.map(s => s.code);
        setSelectedBlock({ ...block, stocks: codes, stocksWithNames });
      } else {
        setSelectedBlock(block);
      }
    } catch {
      // 如果获取失败，仍然保留板块信息
      setSelectedBlock(block);
    } finally {
      setBlockStocksLoading(false);
    }
  }, [blockFile]);

  // 选择板块时加载成分股
  const handleSelectBlock = useCallback((block: BlockInfo) => {
    if (selectedBlock?.name === block.name) {
      // 取消选择
      setSelectedBlock(null);
    } else {
      // 选择新板块，加载成分股
      loadBlockStocks(block);
    }
  }, [selectedBlock, loadBlockStocks]);

  useEffect(() => {
    if (sourceTab === 'block') loadBlocks(blockFile);
  }, [sourceTab, blockFile, loadBlocks]);

  // ── Resolve codes from source ──

  const resolvedCodes = useMemo(() => {
    if (sourceTab === 'block' && selectedBlock && selectedBlock.stocks) {
      return selectedBlock.stocks.join(',');
    }
    // 使用 stockList 的代码
    return stockList.map(s => s.code).join(',');
  }, [sourceTab, stockList, selectedBlock, blockData]);

  // ── Screen ──

  const doScreen = async () => {
    const c = resolvedCodes.trim();
    if (!c) return;
    setLoading(true);
    setError('');
    try {
      const res = await api.screen(c, ktype);
      const valid = res.results.filter(r => r.code);
      setResults(valid);
      setTotal(res.total);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  // ── Client-side signal filter ──

  const filteredResults = useMemo(() => {
    if (selectedSignals.length === 0) return results;
    return results.filter(r =>
      r.signals?.some(s => selectedSignals.includes(s.Type))
    );
  }, [results, selectedSignals]);

  // ── Sorting ──

  const sortedResults = useMemo(() => {
    const list = [...filteredResults];
    const dir = sortAsc ? 1 : -1;
    list.sort((a, b) => {
      let va: number | string = 0, vb: number | string = 0;
      switch (sortKey) {
        case 'code': va = a.code; vb = b.code; break;
        case 'name': va = a.name || ''; vb = b.name || ''; break;
        case 'close': va = a.last?.Close || 0; vb = b.last?.Close || 0; break;
        case 'change': {
          const pa = a.last ? (a.last.Close - a.last.Open) / a.last.Open : 0;
          const pb = b.last ? (b.last.Close - b.last.Open) / b.last.Open : 0;
          va = pa; vb = pb; break;
        }
        case 'dif': va = getLastValue(a.macd?.DIF); vb = getLastValue(b.macd?.DIF); break;
        case 'k': va = getLastValue(a.kdj?.K); vb = getLastValue(b.kdj?.K); break;
        case 'j': va = getLastValue(a.kdj?.J); vb = getLastValue(b.kdj?.J); break;
      }
      if (typeof va === 'string') return va.localeCompare(vb as string) * dir;
      return ((va as number) - (vb as number)) * dir;
    });
    return list;
  }, [filteredResults, sortKey, sortAsc]);

  // ── Signal toggle ──

  const toggleSignal = (s: string) => {
    setSelectedSignals(prev =>
      prev.includes(s) ? prev.filter(x => x !== s) : [...prev, s]
    );
  };

  // ── Sort header helper ──

  const SortHeader = ({ k, children, className = '' }: { k: SortKey; children: React.ReactNode; className?: string }) => (
    <th
      className={`p-3 cursor-pointer select-none hover:text-slate-200 transition-colors ${className}`}
      onClick={() => { if (sortKey === k) setSortAsc(!sortAsc); else { setSortKey(k); setSortAsc(true); } }}
    >
      <span className="inline-flex items-center gap-1">
        {children}
        {sortKey === k ? (sortAsc ? <ChevronUp size={12} /> : <ChevronDown size={12} />) : <ArrowUpDown size={10} className="opacity-30" />}
      </span>
    </th>
  );

  // ── Stats ──

  const signalCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const r of results) {
      for (const s of (r.signals || [])) {
        counts[s.Type] = (counts[s.Type] || 0) + 1;
      }
    }
    return counts;
  }, [results]);

  // ── Filtered blocks for search ──

  const filteredBlocks = useMemo(() => {
    if (!blockSearch) return blockData.sort((a, b) => b.count - a.count);
    const q = blockSearch.toLowerCase();
    return blockData
      .filter(b => b.name.toLowerCase().includes(q))
      .sort((a, b) => b.count - a.count);
  }, [blockData, blockSearch]);

  // ── Render ──

  return (
    <>
      {/* Toast 提示 */}
      {toast && (
        <div className={`fixed top-4 left-1/2 -translate-x-1/2 px-4 py-2 rounded-lg text-sm z-50 animate-fade-in ${
          toast.type === 'error' ? 'bg-red-600/90 text-white' : 'bg-green-600/90 text-white'
        }}`}>
          {toast.message}
        </div>
      )}
      <div className="flex gap-4 h-full min-h-0">

      <div className="w-64 shrink-0 flex flex-col gap-3 min-h-0">
        <h1 className="text-xl font-bold text-white flex items-center gap-2">
          <Search size={20} /> 信号筛选
        </h1>

        <div className="flex gap-1 text-xs">
          {([['watchlist', '自选'], ['block', '板块']] as [SourceTab, string][]).map(([k, label]) => (
            <button
              key={k}
              onClick={() => setSourceTab(k)}
              className={`flex-1 py-1.5 rounded transition-colors ${sourceTab === k ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-white hover:bg-slate-800'}`}
            >
              {label}
            </button>
          ))}
        </div>

        {sourceTab === 'watchlist' && (
          <div className="flex flex-col gap-2 flex-1 min-h-0">
            {/* 股票列表 */}
            <div className="flex-1 overflow-auto space-y-1 min-h-0">
              {stockList.length === 0 ? (
                <div className="text-slate-500 text-xs text-center py-4">
                  输入股票代码，按回车添加
                </div>
              ) : (
                stockList.map((stock, idx) => (
                  <div
                    key={stock.code}
                    onClick={() => navigate(`/stock/${stock.code}/chart`)}
                    className="flex items-center justify-between px-2 py-1.5 bg-slate-800/50 rounded text-sm group hover:bg-slate-800 cursor-pointer"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="text-blue-400 font-mono text-xs">{stock.code}</span>
                      <span className="text-white truncate">{stock.name || '-'}</span>
                    </div>
                    <button
                      onClick={(e) => { e.stopPropagation(); setStockList(prev => prev.filter((_s, i) => i !== idx)); }}
                      className="opacity-0 group-hover:opacity-100 text-slate-500 hover:text-red-400 transition-all shrink-0"
                    >
                      <X size={14} />
                    </button>
                  </div>
                ))
              )}
            </div>
            {/* 输入框 */}
            <div className="relative">
              <input
                type="text"
                value={inputCode}
                onChange={e => setInputCode(e.target.value)}
                onKeyDown={async e => {
                  if (e.key === 'Enter' && inputCode.trim()) {
                    // 支持逗号/空格/换行分隔的多个股票代码，如 "000001,600519,000858" 或 "000001 600519 000858"
                    const codes = inputCode.split(/[, \n]+/).map(c => c.trim().toUpperCase()).filter(c => c);
                    
                    if (codes.length === 0) return;
                    
                    // 验证所有股票代码格式（6位数字）
                    const invalidCodes = codes.filter(c => !/^\d{6}$/.test(c));
                    if (invalidCodes.length > 0) {
                      showToast(`无效的股票代码: ${invalidCodes.join(', ')}`);
                      return;
                    }
                    
                    // 检查是否已存在
                    const existingCodes = codes.filter(c => stockList.some(s => s.code === c));
                    if (existingCodes.length > 0) {
                      showToast(`股票已存在: ${existingCodes.join(', ')}`);
                    }
                    
                    // 过滤掉已存在的代码
                    const newCodes = codes.filter(c => !stockList.some(s => s.code === c));
                    
                    if (newCodes.length === 0) {
                      setInputCode('');
                      return;
                    }
                    
                    setInputLoading(true);
                    try {
                      const cache = await preloadCodesCache();
                      const results = stockNamesFromCodesCache(newCodes, cache);
                      
                      if (results.length === 0) {
                        showToast('股票代码不存在');
                      } else {
                        setStockList(prev => [...prev, ...results]);
                        if (results.length === 1) {
                          showToast(`已添加 ${results[0].name}`, 'success');
                        } else {
                          showToast(`已添加 ${results.length} 只股票`, 'success');
                        }
                      }
                    } catch {
                      showToast('获取股票信息失败');
                    } finally {
                      setInputLoading(false);
                      setInputCode('');
                    }
                  }
                }}
                placeholder="输入股票代码（如 002223,002202），回车添加..."
                disabled={inputLoading}
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white text-sm font-mono focus:outline-none focus:border-blue-500 pr-8"
              />
              {inputLoading && (
                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400">
                  <span className="animate-spin">⟳</span>
                </span>
              )}
            </div>
            <div className="text-xs text-slate-500">
              {stockList.length} 只股票
            </div>
          </div>
        )}

        {sourceTab === 'block' && (
          <div className="flex flex-col gap-2 flex-1 min-h-0">
            <div className="flex gap-1 flex-wrap">
              {ALL_BLOCK_FILES.map(b => (
                <button
                  key={b.file}
                  onClick={() => { setBlockFile(b.file); loadBlocks(b.file, b.type); }}
                  className={`flex-1 py-1 text-xs rounded transition-colors min-w-[60px] ${blockFile === b.file ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-white'}`}
                >
                  {b.label}
                </button>
              ))}
            </div>
            <input
              type="text"
              value={blockSearch}
              onChange={e => setBlockSearch(e.target.value)}
              placeholder="搜索板块..."
              className="bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-white text-sm focus:outline-none focus:border-blue-500"
            />
            {blockLoading ? (
              <div className="text-slate-500 text-xs text-center py-4">加载板块...</div>
            ) : (
              <div className="flex-1 overflow-auto space-y-px">
                {filteredBlocks.map(block => (
                  <button
                    key={block.name}
                    onClick={() => handleSelectBlock(block)}
                    className={`w-full text-left px-2 py-1.5 text-sm rounded transition-colors flex items-center justify-between ${
                      selectedBlock?.name === block.name ? 'bg-blue-600/20 text-blue-400' : 'text-slate-300 hover:bg-slate-800'
                    }`}
                  >
                    <span className="truncate">{block.name}</span>
                    <span className="text-xs text-slate-600 shrink-0 ml-2">{block.count}只</span>
                  </button>
                ))}
              </div>
            )}
            {selectedBlock && (
              <div className="text-xs text-slate-400 border-t border-slate-800 pt-2 flex items-center gap-2">
                {blockStocksLoading ? (
                  <span className="flex items-center gap-1 text-blue-400">
                    <span className="animate-spin">⟳</span> 加载成分股...
                  </span>
                ) : (
                  <>
                    已选 <span className="text-blue-400 font-medium">{selectedBlock.name}</span> · {selectedBlock.stocks?.length || selectedBlock.count} 只
                    <button
                      onClick={() => {
                        if (!selectedBlock.stocks?.length) return;
                        setShowBlockModal(true);
                        const fromApi = selectedBlock.stocksWithNames;
                        if (fromApi?.length) {
                          setBlockStocksWithNames(fromApi);
                          return;
                        }
                        setBlockStocksLoadingNames(true);
                        void (async () => {
                          try {
                            const cache = await preloadCodesCache();
                            const rows = stockNamesFromCodesCache(selectedBlock.stocks!, cache);
                            const byCode = new Map(rows.map(r => [r.code, r.name]));
                            const filled = selectedBlock.stocks!.map(c => ({
                              code: c,
                              name: byCode.get(c) ?? c,
                            }));
                            setBlockStocksWithNames(filled);
                          } finally {
                            setBlockStocksLoadingNames(false);
                          }
                        })();
                      }}
                      className="ml-2 text-blue-400 hover:text-blue-300"
                    >
                      <ExternalLink size={12} className="inline" /> 查看
                    </button>
                  </>
                )}
              </div>
            )}
          </div>
        )}

        

        
      </div>

      <div className="flex-1 min-w-0 flex flex-col gap-3 min-h-0">
        <div className="bg-slate-900/50 rounded-lg border border-slate-800 p-3 space-y-3">
          <div className="flex items-center gap-3 flex-wrap">
            <span className="text-xs text-slate-500">周期</span>
            <div className="flex gap-1">
              {KTYPE_OPTIONS.map(o => (
                <button
                  key={o.value}
                  onClick={() => setKtype(o.value)}
                  className={`px-2.5 py-1 text-xs rounded transition-colors ${
                    ktype === o.value ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-white'
                  }`}
                >
                  {o.label}
                </button>
              ))}
            </div>

            <div className="h-4 w-px bg-slate-700" />

            <span className="text-xs text-slate-500">信号</span>
            <div className="flex gap-1 flex-wrap">
              {SIGNAL_OPTIONS.map(o => (
                <button
                  key={o.value}
                  onClick={() => toggleSignal(o.value)}
                  className={`px-2 py-1 text-xs rounded transition-colors ${
                    selectedSignals.includes(o.value)
                      ? o.buy ? 'bg-red-600/20 text-red-400 border border-red-500/30' : 'bg-green-600/20 text-green-400 border border-green-500/30'
                      : 'bg-slate-800 text-slate-400 hover:text-white border border-transparent'
                  }`}
                >
                  {o.label}
                </button>
              ))}
              {selectedSignals.length > 0 && (
                <button onClick={() => setSelectedSignals([])} className="text-xs text-slate-600 hover:text-slate-400 px-1">
                  <X size={12} />
                </button>
              )}
            </div>

            <button
              onClick={doScreen}
              disabled={loading || !resolvedCodes.trim()}
              className="ml-auto bg-blue-600 hover:bg-blue-700 disabled:opacity-40 px-4 py-1.5 rounded-lg text-white text-sm font-medium transition-colors"
            >
              {loading ? '筛选中...' : '开始筛选'}
            </button>
          </div>
        </div>

        {error && <div className="bg-red-900/30 border border-red-800 rounded-lg px-4 py-2 text-red-300 text-sm">{error}</div>}

        {results.length > 0 && (
          <div className="flex items-center gap-4 text-xs flex-wrap">
            <span className="text-slate-400">
              扫描 <span className="text-white font-medium">{total}</span> 只
              {selectedSignals.length > 0 && <> · 命中 <span className="text-white font-medium">{filteredResults.length}</span> 只</>}
            </span>
            {Object.entries(signalCounts).map(([type, count]) => (
              <span key={type} className={isBuySignal(type) ? 'text-red-400/70' : 'text-green-400/70'}>
                {type} {count}
              </span>
            ))}
          </div>
        )}

        {sortedResults.length > 0 && (
          <VirtualResultTable results={sortedResults} tableContainerRef={tableContainerRef} SortHeader={SortHeader} navigate={navigate} />
        )}

        {results.length === 0 && !loading && !error && (
          <div className="flex-1 flex items-center justify-center text-slate-500 text-sm">
            <div className="text-center">
              <Search size={40} className="mx-auto mb-3 opacity-20" />
              <p>选择股票来源，点击「开始筛选」</p>
            </div>
          </div>
        )}
      </div>

      {/* 成分股弹窗 */}
      {showBlockModal && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setShowBlockModal(false)}>
          <div className="bg-slate-900 rounded-lg border border-slate-700 w-[600px] max-h-[80vh] flex flex-col" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between px-4 py-3 border-b border-slate-800">
              <div>
                <h2 className="text-white font-medium">{selectedBlock?.name}</h2>
                <p className="text-xs text-slate-500">
                  {(blockStocksLoadingNames ? selectedBlock?.stocks?.length : blockStocksWithNames.length) ?? 0} 只成分股
                </p>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => {
                    // 批量添加不在自选中的股票
                    const newStocks = blockStocksWithNames
                      .filter(s => !stockList.some(w => w.code === s.code))
                      .map(s => ({ code: s.code, name: s.name }));
                    if (newStocks.length === 0) {
                      showToast('所有股票已存在', 'error');
                      return;
                    }
                    setStockList(prev => [...prev, ...newStocks]);
                    showToast(`已添加 ${newStocks.length} 只股票`, 'success');
                  }}
                  className="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white text-xs rounded transition-colors"
                >
                  全部加入自选
                </button>
                <button onClick={() => setShowBlockModal(false)} className="text-slate-400 hover:text-white">
                  <X size={20} />
                </button>
              </div>
            </div>
            <div className="flex-1 overflow-auto p-2">
              {blockStocksLoadingNames ? (
                <div className="text-center text-slate-500 py-8">
                  <span className="animate-spin inline-block mr-2">⟳</span> 加载股票名称...
                </div>
              ) : (
                <div className="grid grid-cols-2 gap-1">
                  {blockStocksWithNames.map(stock => {
                    const isInWatchlist = stockList.some(s => s.code === stock.code);
                    return (
                      <div
                        key={stock.code}
                        onClick={() => { setShowBlockModal(false); navigate(`/stock/${stock.code}/chart`); }}
                        className="flex items-center justify-between px-3 py-2 bg-slate-800/50 rounded hover:bg-slate-800 transition-colors cursor-pointer"
                      >
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="text-blue-400 font-mono text-xs shrink-0">{stock.code}</span>
                          <span className="text-white text-sm truncate">{stock.name}</span>
                        </div>
                        <button
                          onClick={() => {
                            if (isInWatchlist) {
                              setStockList(prev => prev.filter(s => s.code !== stock.code));
                              showToast(`已移除 ${stock.name}`, 'success');
                            } else {
                              setStockList(prev => [...prev, { code: stock.code, name: stock.name }]);
                              showToast(`已添加 ${stock.name}`, 'success');
                            }
                          }}
                          disabled={blockStocksLoadingNames}
                          className={`shrink-0 px-2 py-1 text-xs rounded transition-colors ${
                            isInWatchlist
                              ? 'bg-slate-700 text-slate-400 hover:text-white'
                              : 'bg-blue-600 text-white hover:bg-blue-700'
                          }`}
                        >
                          {isInWatchlist ? '移除' : '加入自选'}
                        </button>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
            <div className="px-4 py-3 border-t border-slate-800 flex justify-between items-center">
              <span className="text-xs text-slate-500">
                自选已有 {stockList.length} 只股票
              </span>
              <button
                onClick={() => setShowBlockModal(false)}
                className="px-4 py-1.5 bg-slate-800 hover:bg-slate-700 text-white text-sm rounded transition-colors"
              >
                关闭
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
    </>
  );
}
