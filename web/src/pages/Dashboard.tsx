import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { TrendingUp, TrendingDown, BarChart3, Clock } from 'lucide-react';
import { api } from '../api/client';
import type { HistoryStock, Quote, CodeItem } from '../types/api';

const INDICES = [
  { code: '999999', name: '上证指数' },
  { code: '399001', name: '深证成指' },
  { code: '399006', name: '创业板指' },
  { code: '399300', name: '沪深300' },
];

type IndexRow = (typeof INDICES)[number] & {
  last: { Close: number } | null;
  change: number;
  up: boolean;
};

function initialIndexPlaceholders(): IndexRow[] {
  return INDICES.map(idx => ({ ...idx, last: null, change: 0, up: true }));
}

export default function Dashboard() {
  const navigate = useNavigate();
  /** 首屏即渲染与 INDICES 等量的卡片，避免请求返回前网格高度为 0 造成跳动 */
  const [indices, setIndices] = useState<IndexRow[]>(initialIndexPlaceholders);
  const [history, setHistory] = useState<HistoryStock[]>([]);
  const [historyQuotes, setHistoryQuotes] = useState<Record<string, Quote>>({});
  const [stockNames, setStockNames] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    (async () => {
      const results = [];
      for (const idx of INDICES) {
        try {
          const bars = await api.index(idx.code, 'day');
          const last = bars?.[bars.length - 1] ?? null;
          const prev = bars?.[bars.length - 2];
          const change = last && prev ? ((last.Close - prev.Close) / prev.Close * 100) : 0;
          results.push({ ...idx, last, change, up: change >= 0 });
        } catch {
          results.push({ ...idx, last: null, change: 0, up: true });
        }
      }
      setIndices(results as IndexRow[]);
      setLoading(false);
    })();

    api.history().then(h => {
      setHistory(h);
      const codes = h.map(s => s.code);
      const sz = codes.filter(c => c.startsWith('0') || c.startsWith('3'));
      const sh = codes.filter(c => c.startsWith('6'));
      Promise.all([
        sz.length > 0 ? api.codes('sz') : Promise.resolve([]),
        sh.length > 0 ? api.codes('sh') : Promise.resolve([]),
      ]).then(([szCodes, shCodes]) => {
        const names: Record<string, string> = {};
        [...szCodes, ...shCodes].forEach((c: CodeItem) => { names[c.Code] = c.Name; });
        setStockNames(names);
      });
      h.forEach(stock => {
        api.quote(stock.code).then(q => {
          setHistoryQuotes(prev => ({ ...prev, [stock.code]: q }));
        }).catch(() => {});
      });
    }).catch(() => {});
  }, []);

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-white flex items-center gap-2">
        <BarChart3 size={24} /> 市场总览
      </h1>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 min-h-[7.5rem]">
        {indices.map((idx) => (
          <div
            key={idx.code}
            className="bg-slate-900 rounded-xl border border-slate-800 p-5 min-h-[7rem] flex flex-col justify-between"
          >
            <div className="text-slate-400 text-sm">{idx.name}</div>
            {loading && !idx.last ? (
              <div className="space-y-2 flex-1 flex flex-col justify-end">
                <div className="h-8 w-28 bg-slate-800 rounded animate-pulse" />
                <div className="h-4 w-20 bg-slate-800/80 rounded animate-pulse" />
              </div>
            ) : idx.last ? (
              <>
                <div className={`text-2xl font-bold tabular-nums ${idx.up ? 'text-red-400' : 'text-green-400'}`}>
                  {idx.last.Close.toFixed(2)}
                </div>
                <div className={`flex items-center gap-1 text-sm tabular-nums ${idx.up ? 'text-red-400' : 'text-green-400'}`}>
                  {idx.up ? <TrendingUp size={16} /> : <TrendingDown size={16} />}
                  {idx.change > 0 ? '+' : ''}{idx.change.toFixed(2)}%
                </div>
              </>
            ) : (
              <div className="text-slate-500 text-sm">数据加载失败</div>
            )}
          </div>
        ))}
      </div>

      {history?.length > 0 && (
        <div className="bg-slate-900 rounded-xl border border-slate-800 p-6">
          <div className="flex items-center gap-2 mb-4">
            <Clock size={18} className="text-slate-400" />
            <h2 className="text-lg font-bold text-white">历史个股</h2>
          </div>
          <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-3">
            {history.map(stock => {
              const q = historyQuotes[stock.code];
              const change = q ? ((q.Price - q.LastClose) / q.LastClose * 100) : 0;
              return (
                <div key={stock.code} className="flex items-center justify-between bg-slate-800 rounded-lg px-3 py-2">
                  <button
                    onClick={() => navigate(`/stock/${stock.code}`)}
                    className="flex flex-col text-left hover:text-blue-400 transition-colors"
                  >
                    <span className="text-white font-medium text-sm">{stockNames[stock.code] || q?.Name || stock.code}</span>
                    <span className="text-slate-500 text-xs">{stock.code}</span>
                  </button>
                  <div className="flex flex-col items-end">
                    <span className="text-white text-sm">{q?.Price?.toFixed(2)}</span>
                    <span className={`text-xs ${change >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                      {change > 0 ? '+' : ''}{change?.toFixed(2)}%
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      <div className="bg-slate-900 rounded-xl border border-slate-800 p-6">
        <h2 className="text-lg font-bold text-white mb-4">快速分析</h2>
        <p className="text-slate-400 mb-4">输入股票代码查看技术指标</p>
        <QuickSearch />
      </div>
    </div>
  );
}

function QuickSearch() {
  const [code, setCode] = useState('');
  const navigate = useNavigate();
  const go = () => { if (code) navigate(`/stock/${code}`); };
  return (
    <div className="flex gap-3">
      <input
        type="text"
        value={code}
        onChange={e => setCode(e.target.value)}
        onKeyDown={e => { if (e.key === 'Enter') go(); }}
        placeholder="输入股票代码，如 000001"
        className="bg-slate-800 border border-slate-700 rounded-lg px-4 py-2 text-white flex-1 focus:outline-none focus:border-blue-500"
      />
      <button
        onClick={go}
        className={`bg-blue-600 hover:bg-blue-700 px-6 py-2 rounded-lg text-white font-medium ${!code && 'opacity-50 pointer-events-none'}`}
      >
        分析
      </button>
    </div>
  );
}
