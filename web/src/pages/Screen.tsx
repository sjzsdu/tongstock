import { useState, useEffect } from 'react';
import { getScreen } from '../api/client';
import type { ScreenResult } from '../types/api';
import { Search, AlertCircle } from 'lucide-react';

const signalOptions = [
  { value: '', label: '全部' },
  { value: 'golden_cross', label: '金叉' },
  { value: 'death_cross', label: '死叉' },
  { value: 'overbought', label: '超买' },
  { value: 'oversold', label: '超卖' },
];

export default function Screen() {
  const [codes, setCodes] = useState('000001,600519,000858,601318,000568');
  const [signal, setSignal] = useState('');
  const [results, setResults] = useState<ScreenResult[]>([]);
  const [total, setTotal] = useState(0);
  const [matched, setMatched] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const load = async () => {
    if (!codes) return;
    setLoading(true);
    setError('');
    try {
      const res = await getScreen(codes, 'day', signal || undefined);
      setResults(res.results);
      setTotal(res.total);
      setMatched(res.matched ?? res.results.length);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4 flex-wrap">
        <h1 className="text-2xl font-bold text-white flex items-center gap-2">
          <Search size={24} /> 信号筛选
        </h1>
        <input
          type="text"
          value={codes}
          onChange={e => setCodes(e.target.value)}
          placeholder="股票代码，逗号分隔"
          className="bg-slate-800 border border-slate-700 rounded-lg px-4 py-2 text-white flex-1 min-w-64 focus:outline-none focus:border-blue-500"
        />
        <select
          value={signal}
          onChange={e => setSignal(e.target.value)}
          className="bg-slate-800 border border-slate-700 rounded-lg px-4 py-2 text-white focus:outline-none focus:border-blue-500"
        >
          {signalOptions.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <button
          onClick={load}
          disabled={loading}
          className="bg-blue-600 hover:bg-blue-700 px-4 py-2 rounded-lg text-white font-medium disabled:opacity-50"
        >
          {loading ? '筛选中...' : '筛选'}
        </button>
      </div>

      {error && <div className="bg-red-900/50 border border-red-700 rounded-lg p-4 text-red-200">{error}</div>}

      <div className="text-slate-400 text-sm">
        共 {total} 只股票，{signal ? `匹配 ${matched} 只` : `全部显示`}
      </div>

      {results.length > 0 && (
        <div className="bg-slate-900 rounded-xl border border-slate-800 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-800 text-slate-400">
                <th className="text-left p-3">代码</th>
                <th className="text-right p-3">收盘</th>
                <th className="text-right p-3">MA5</th>
                <th className="text-right p-3">MA10</th>
                <th className="text-right p-3">MA20</th>
                <th className="text-right p-3">DIF</th>
                <th className="text-right p-3">K</th>
                <th className="text-right p-3">J</th>
                <th className="text-left p-3">信号</th>
              </tr>
            </thead>
            <tbody>
              {results.map((r, i) => {
                const n = r.ma?.['5']?.length || 0;
                const lastMa5 = r.ma?.['5']?.[n - 1] ?? 0;
                const lastMa10 = r.ma?.['10']?.[n - 1] ?? 0;
                const lastMa20 = r.ma?.['20']?.[n - 1] ?? 0;
                const lastDif = r.macd?.DIF?.[n - 1] ?? 0;
                const lastK = r.kdj?.K?.[n - 1] ?? 0;
                const lastJ = r.kdj?.J?.[n - 1] ?? 0;
                const recentSignals = r.signals?.slice(-3) || [];

                return (
                  <tr key={i} className="border-b border-slate-800/50 hover:bg-slate-800/50">
                    <td className="p-3 font-mono text-blue-400">{r.code}</td>
                    <td className="p-3 text-right">{r.last?.Close?.toFixed(2)}</td>
                    <td className="p-3 text-right">{lastMa5.toFixed(2)}</td>
                    <td className="p-3 text-right">{lastMa10.toFixed(2)}</td>
                    <td className="p-3 text-right">{lastMa20.toFixed(2)}</td>
                    <td className={`p-3 text-right ${lastDif > 0 ? 'text-red-400' : 'text-green-400'}`}>{lastDif.toFixed(2)}</td>
                    <td className="p-3 text-right">{lastK.toFixed(1)}</td>
                    <td className={`p-3 text-right ${lastJ > 100 ? 'text-orange-400' : lastJ < 0 ? 'text-blue-400' : ''}`}>{lastJ.toFixed(1)}</td>
                    <td className="p-3">
                      <div className="flex gap-1 flex-wrap">
                        {recentSignals.map((s, j) => (
                          <span key={j} className="px-1.5 py-0.5 rounded text-xs bg-slate-700 text-slate-300">
                            {s.Indicator}{s.Type}
                          </span>
                        ))}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {results.length === 0 && !loading && !error && (
        <div className="text-center text-slate-500 py-12">
          <AlertCircle size={48} className="mx-auto mb-4 opacity-50" />
          <p>请输入股票代码开始筛选</p>
        </div>
      )}
    </div>
  );
}
