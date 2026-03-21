import { useState, useEffect } from 'react';
import { getIndicator } from '../api/client';
import type { IndicatorData, Signal } from '../types/api';
import IndicatorChart from '../components/IndicatorChart';
import { Activity } from 'lucide-react';

function SignalBadge({ signal }: { signal: Signal }) {
  const colors: Record<string, string> = {
    '金叉': 'bg-green-600',
    '死叉': 'bg-red-600',
    '超买': 'bg-orange-600',
    '超卖': 'bg-blue-600',
    '多头排列': 'bg-green-700',
    '空头排列': 'bg-red-700',
    '突破上轨': 'bg-purple-600',
    '跌破下轨': 'bg-rose-600',
  };
  return (
    <span className={`px-2 py-1 rounded text-xs font-medium ${colors[signal.Type] || 'bg-slate-600'}`}>
      {signal.Indicator} {signal.Type}
    </span>
  );
}

export default function Stock() {
  const [code, setCode] = useState('000001');
  const [data, setData] = useState<IndicatorData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const load = async () => {
    if (!code) return;
    setLoading(true);
    setError('');
    try {
      const result = await getIndicator(code);
      setData(result);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const recentSignals = data?.signals?.slice(-10).reverse() || [];

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <h1 className="text-2xl font-bold text-white flex items-center gap-2">
          <Activity size={24} /> 指标分析
        </h1>
        <input
          type="text"
          value={code}
          onChange={e => setCode(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && load()}
          placeholder="股票代码"
          className="bg-slate-800 border border-slate-700 rounded-lg px-4 py-2 text-white w-32 focus:outline-none focus:border-blue-500"
        />
        <button
          onClick={load}
          disabled={loading}
          className="bg-blue-600 hover:bg-blue-700 px-4 py-2 rounded-lg text-white font-medium disabled:opacity-50"
        >
          {loading ? '加载中...' : '查询'}
        </button>
        {data && (
          <span className="text-slate-400 text-sm">
            {data.code} · {data.category} · {data.count} 条K线
          </span>
        )}
      </div>

      {error && <div className="bg-red-900/50 border border-red-700 rounded-lg p-4 text-red-200">{error}</div>}

      {data && (
        <>
          <IndicatorChart data={data} />

          {recentSignals.length > 0 && (
            <div className="bg-slate-900 rounded-xl border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-3">最新信号</h3>
              <div className="flex flex-wrap gap-2">
                {recentSignals.map((s, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <span className="text-slate-500 text-xs">{s.Date?.slice(5, 10)}</span>
                    <SignalBadge signal={s} />
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
