import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { getIndex } from '../api/client';
import { TrendingUp, TrendingDown, BarChart3 } from 'lucide-react';

const INDICES = [
  { code: '999999', name: '上证指数' },
  { code: '399001', name: '深证成指' },
  { code: '399006', name: '创业板指' },
  { code: '399300', name: '沪深300' },
];

export default function Dashboard() {
  const [indices, setIndices] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    (async () => {
      const results = [];
      for (const idx of INDICES) {
        try {
          const bars = await getIndex(idx.code, 'day');
          const last = bars?.[bars.length - 1];
          const prev = bars?.[bars.length - 2];
          const change = last && prev ? ((last.Close - prev.Close) / prev.Close * 100) : 0;
          results.push({ ...idx, last, change, up: change >= 0 });
        } catch {
          results.push({ ...idx, last: null, change: 0, up: true });
        }
      }
      setIndices(results);
      setLoading(false);
    })();
  }, []);

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-white flex items-center gap-2">
        <BarChart3 size={24} /> 市场总览
      </h1>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {indices.map((idx) => (
          <div key={idx.code} className="bg-slate-900 rounded-xl border border-slate-800 p-5">
            <div className="text-slate-400 text-sm mb-1">{idx.name}</div>
            {loading ? (
              <div className="h-8 bg-slate-800 rounded animate-pulse" />
            ) : idx.last ? (
              <>
                <div className={`text-2xl font-bold ${idx.up ? 'text-red-400' : 'text-green-400'}`}>
                  {idx.last.Close?.toFixed(2)}
                </div>
                <div className={`flex items-center gap-1 text-sm mt-1 ${idx.up ? 'text-red-400' : 'text-green-400'}`}>
                  {idx.up ? <TrendingUp size={16} /> : <TrendingDown size={16} />}
                  {idx.change > 0 ? '+' : ''}{idx.change.toFixed(2)}%
                </div>
              </>
            ) : (
              <div className="text-slate-500">数据加载失败</div>
            )}
          </div>
        ))}
      </div>

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
  return (
    <div className="flex gap-3">
      <input
        type="text"
        value={code}
        onChange={e => setCode(e.target.value)}
        placeholder="输入股票代码，如 000001"
        className="bg-slate-800 border border-slate-700 rounded-lg px-4 py-2 text-white flex-1 focus:outline-none focus:border-blue-500"
      />
      <Link
        to={`/stock/${code}`}
        className={`bg-blue-600 hover:bg-blue-700 px-6 py-2 rounded-lg text-white font-medium ${!code && 'opacity-50 pointer-events-none'}`}
      >
        分析
      </Link>
    </div>
  );
}
