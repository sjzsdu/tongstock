import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search, X, Trash2, ChevronDown, ChevronUp, Bookmark, FolderOpen, ArrowUpDown } from 'lucide-react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { api } from '../api/client';
import type { ScreenResult, BlockItem } from '../types/api';

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

const BLOCK_FILES = [
  { file: 'block_fg.dat', label: '行业' },
  { file: 'block_gn.dat', label: '概念' },
];

const PRESET_KEY = 'tongstock_screen_presets';

// ── Types ──────────────────────────────────────────────────────────────────────

interface Preset {
  name: string;
  codes: string;
}

type SourceTab = 'manual' | 'block' | 'preset';
type SortKey = 'code' | 'name' | 'close' | 'change' | 'dif' | 'k' | 'j';

// ── Helpers ────────────────────────────────────────────────────────────────────

function loadPresets(): Preset[] {
  try {
    return JSON.parse(localStorage.getItem(PRESET_KEY) || '[]');
  } catch { return []; }
}

function savePresets(presets: Preset[]) {
  localStorage.setItem(PRESET_KEY, JSON.stringify(presets));
}

function groupBlocks(items: BlockItem[]): Map<string, string[]> {
  const map = new Map<string, string[]>();
  for (const item of items) {
    if (!item.StockCode || !item.BlockName) continue;
    const codes = map.get(item.BlockName) || [];
    codes.push(item.StockCode);
    map.set(item.BlockName, codes);
  }
  return map;
}

function getLastValue(arr: number[] | undefined): number {
  if (!arr || arr.length === 0) return 0;
  return arr[arr.length - 1];
}

function isBuySignal(type: string): boolean {
  return SIGNAL_OPTIONS.find(s => s.value === type)?.buy ?? false;
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

  const [sourceTab, setSourceTab] = useState<SourceTab>('manual');
  const [codes, setCodes] = useState('000001,600519,000858,601318,000568');
  const [ktype, setKtype] = useState('day');
  const [selectedSignals, setSelectedSignals] = useState<string[]>([]);
  const [results, setResults] = useState<ScreenResult[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [sortKey, setSortKey] = useState<SortKey>('code');
  const [sortAsc, setSortAsc] = useState(true);

  const [blockFile, setBlockFile] = useState(BLOCK_FILES[0].file);
  const [blockData, setBlockData] = useState<Map<string, string[]>>(new Map());
  const [selectedBlock, setSelectedBlock] = useState('');
  const [blockLoading, setBlockLoading] = useState(false);
  const [blockSearch, setBlockSearch] = useState('');

  const [presets, setPresets] = useState<Preset[]>(loadPresets);
  const [newPresetName, setNewPresetName] = useState('');
  const [showSavePreset, setShowSavePreset] = useState(false);

  // ── Block loading ──

  const loadBlocks = useCallback(async (file: string) => {
    setBlockLoading(true);
    try {
      const items = await api.block(file);
      setBlockData(groupBlocks(items));
      setSelectedBlock('');
    } catch { setBlockData(new Map()); }
    finally { setBlockLoading(false); }
  }, []);

  useEffect(() => {
    if (sourceTab === 'block') loadBlocks(blockFile);
  }, [sourceTab, blockFile, loadBlocks]);

  // ── Resolve codes from source ──

  const resolvedCodes = useMemo(() => {
    if (sourceTab === 'block' && selectedBlock) {
      return (blockData.get(selectedBlock) || []).join(',');
    }
    return codes;
  }, [sourceTab, codes, selectedBlock, blockData]);

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

  // ── Presets ──

  const saveCurrentPreset = () => {
    const name = newPresetName.trim();
    if (!name || !resolvedCodes) return;
    const next = [...presets.filter(p => p.name !== name), { name, codes: resolvedCodes }];
    setPresets(next);
    savePresets(next);
    setNewPresetName('');
    setShowSavePreset(false);
  };

  const deletePreset = (name: string) => {
    const next = presets.filter(p => p.name !== name);
    setPresets(next);
    savePresets(next);
  };

  const loadPreset = (p: Preset) => {
    setCodes(p.codes);
    setSourceTab('manual');
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
    const names = Array.from(blockData.keys()).sort();
    if (!blockSearch) return names;
    const q = blockSearch.toLowerCase();
    return names.filter(n => n.toLowerCase().includes(q));
  }, [blockData, blockSearch]);

  // ── Render ──

  return (
    <div className="flex gap-4 h-full min-h-0">

      <div className="w-64 shrink-0 flex flex-col gap-3 min-h-0">
        <h1 className="text-xl font-bold text-white flex items-center gap-2">
          <Search size={20} /> 信号筛选
        </h1>

        <div className="flex gap-1 text-xs">
          {([['manual', '手动'], ['block', '板块'], ['preset', '自选']] as [SourceTab, string][]).map(([k, label]) => (
            <button
              key={k}
              onClick={() => setSourceTab(k)}
              className={`flex-1 py-1.5 rounded transition-colors ${sourceTab === k ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-white hover:bg-slate-800'}`}
            >
              {label}
            </button>
          ))}
        </div>

        {sourceTab === 'manual' && (
          <div className="flex flex-col gap-2 flex-1 min-h-0">
            <textarea
              value={codes}
              onChange={e => setCodes(e.target.value)}
              placeholder="股票代码，逗号或换行分隔&#10;000001&#10;600519"
              className="flex-1 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white text-sm font-mono focus:outline-none focus:border-blue-500 resize-none min-h-[120px]"
            />
            <div className="text-xs text-slate-500">
              {codes.split(/[,\s\n]+/).filter(c => c.trim()).length} 只股票
            </div>
          </div>
        )}

        {sourceTab === 'block' && (
          <div className="flex flex-col gap-2 flex-1 min-h-0">
            <div className="flex gap-1">
              {BLOCK_FILES.map(b => (
                <button
                  key={b.file}
                  onClick={() => setBlockFile(b.file)}
                  className={`flex-1 py-1 text-xs rounded transition-colors ${blockFile === b.file ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-white'}`}
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
                {filteredBlocks.map(name => {
                  const count = blockData.get(name)?.length || 0;
                  return (
                    <button
                      key={name}
                      onClick={() => setSelectedBlock(name === selectedBlock ? '' : name)}
                      className={`w-full text-left px-2 py-1.5 text-sm rounded transition-colors flex items-center justify-between ${
                        name === selectedBlock ? 'bg-blue-600/20 text-blue-400' : 'text-slate-300 hover:bg-slate-800'
                      }`}
                    >
                      <span className="truncate">{name}</span>
                      <span className="text-xs text-slate-600 shrink-0 ml-2">{count}</span>
                    </button>
                  );
                })}
              </div>
            )}
            {selectedBlock && (
              <div className="text-xs text-slate-400 border-t border-slate-800 pt-2">
                已选 <span className="text-blue-400 font-medium">{selectedBlock}</span> · {blockData.get(selectedBlock)?.length || 0} 只
              </div>
            )}
          </div>
        )}

        {sourceTab === 'preset' && (
          <div className="flex flex-col gap-2 flex-1 min-h-0">
            {presets.length === 0 ? (
              <div className="text-slate-500 text-xs text-center py-8">
                <Bookmark size={24} className="mx-auto mb-2 opacity-30" />
                <p>暂无自选组合</p>
                <p className="mt-1">筛选后点击「保存」</p>
              </div>
            ) : (
              <div className="flex-1 overflow-auto space-y-1">
                {presets.map(p => (
                  <div
                    key={p.name}
                    className="flex items-center gap-2 px-2 py-2 rounded hover:bg-slate-800 group cursor-pointer"
                    onClick={() => loadPreset(p)}
                  >
                    <FolderOpen size={14} className="text-slate-500 shrink-0" />
                    <span className="text-sm text-slate-300 flex-1 truncate">{p.name}</span>
                    <span className="text-xs text-slate-600">{p.codes.split(',').length}只</span>
                    <button
                      onClick={e => { e.stopPropagation(); deletePreset(p.name); }}
                      className="text-slate-600 hover:text-red-400 opacity-0 group-hover:opacity-100 transition-opacity"
                    >
                      <Trash2 size={12} />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {resolvedCodes && (
          <div className="border-t border-slate-800 pt-2">
            {showSavePreset ? (
              <div className="flex items-center gap-1">
                <input
                  type="text"
                  value={newPresetName}
                  onChange={e => setNewPresetName(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && saveCurrentPreset()}
                  placeholder="组合名称"
                  autoFocus
                  className="flex-1 bg-slate-800 border border-slate-700 rounded px-2 py-1 text-sm text-white focus:outline-none focus:border-blue-500"
                />
                <button onClick={saveCurrentPreset} className="text-xs text-blue-400 hover:text-blue-300 px-1">保存</button>
                <button onClick={() => setShowSavePreset(false)} className="text-xs text-slate-500 hover:text-slate-300 px-1">取消</button>
              </div>
            ) : (
              <button
                onClick={() => setShowSavePreset(true)}
                className="w-full text-xs text-slate-400 hover:text-blue-400 py-1 transition-colors"
              >
                <Bookmark size={12} className="inline mr-1" />保存为自选组合
              </button>
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
    </div>
  );
}
