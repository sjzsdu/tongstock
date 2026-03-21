import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { BarChart3, DollarSign, Building2, Gift, Clock } from 'lucide-react';
import { api } from '../../api/client';
import CandlestickChart from '../../components/charts/CandlestickChart';

type Tab = 'chart' | 'finance' | 'company' | 'dividend' | 'intraday';

const TABS: { key: Tab; label: string; icon: any }[] = [
  { key: 'chart', label: 'K线+指标', icon: BarChart3 },
  { key: 'finance', label: '财务', icon: DollarSign },
  { key: 'company', label: '公司', icon: Building2 },
  { key: 'dividend', label: '分红', icon: Gift },
  { key: 'intraday', label: '分时', icon: Clock },
];

export default function StockDetail() {
  const { code: paramCode, tab: paramTab } = useParams();
  const navigate = useNavigate();
  const [code, setCode] = useState(paramCode || '000001');
  const [tab, setTab] = useState<Tab>((paramTab as Tab) || 'chart');
  const [quote, setQuote] = useState<any>(null);
  const [klines, setKlines] = useState<any[]>([]);
  const [indicator, setIndicator] = useState<any>(null);
  const [finance, setFinance] = useState<any>(null);
  const [companyCats, setCompanyCats] = useState<any[]>([]);
  const [companyContent, setCompanyContent] = useState('');
  const [selectedCat, setSelectedCat] = useState('');
  const [dividends, setDividends] = useState<any[]>([]);
  const [minuteData, setMinuteData] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (paramCode) setCode(paramCode);
    if (paramTab) setTab(paramTab as Tab);
  }, [paramCode, paramTab]);

  const switchTab = (t: Tab) => {
    setTab(t);
    navigate(`/stock/${code}/${t}`, { replace: true });
  };

  useEffect(() => {
    if (!code) return;
    setLoading(true);
    api.quote(code).then(setQuote).catch(() => {});
    api.kline(code, 'day').then(setKlines).catch(() => {});
    api.indicator(code, 'day').then(setIndicator).catch(() => {});
    setLoading(false);
  }, [code]);

  useEffect(() => {
    if (!code) return;
    if (tab === 'finance') api.finance(code).then(setFinance).catch(() => {});
    if (tab === 'company') api.company(code).then(setCompanyCats).catch(() => {});
    if (tab === 'dividend') api.xdxr(code).then(setDividends).catch(() => {});
    if (tab === 'intraday') api.minute(code).then(r => setMinuteData(r.List || [])).catch(() => {});
  }, [code, tab]);

  const loadCompanyContent = async (filename: string) => {
    setSelectedCat(filename);
    try {
      const r = await api.companyContent(code, filename);
      setCompanyContent(r.content || '');
    } catch { setCompanyContent('加载失败'); }
  };

  const pct = quote ? ((quote.Price - quote.LastClose) / quote.LastClose * 100) : 0;
  const up = pct >= 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <input
          type="text" value={code}
          onChange={e => setCode(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') navigate(`/stock/${code}`); }}
          className="bg-slate-800 border border-slate-700 rounded-lg px-4 py-2 text-white w-32 font-mono focus:outline-none focus:border-blue-500"
          placeholder="股票代码"
        />
        {quote && (
          <div className="flex items-center gap-4">
            <span className="text-white font-bold text-lg">{quote.Name}</span>
            <span className={`text-2xl font-bold ${up ? 'text-red-400' : 'text-green-400'}`}>
              {quote.Price?.toFixed(2)}
            </span>
            <span className={`text-sm ${up ? 'text-red-400' : 'text-green-400'}`}>
              {up ? '+' : ''}{pct.toFixed(2)}%
            </span>
          </div>
        )}
      </div>

      <div className="flex gap-1 border-b border-slate-800">
        {TABS.map(t => (
          <button
            key={t.key}
            onClick={() => switchTab(t.key)}
            className={`flex items-center gap-2 px-4 py-2 text-sm rounded-t-lg transition-colors ${
              tab === t.key ? 'bg-slate-800 text-white border-b-2 border-blue-500' : 'text-slate-400 hover:text-white'
            }`}
          >
            <t.icon size={16} /> {t.label}
          </button>
        ))}
      </div>

      {loading && <div className="text-slate-500 text-center py-8">加载中...</div>}

      {tab === 'chart' && klines.length > 0 && (
        <div className="space-y-4">
          <CandlestickChart klines={klines} indicator={indicator} height={500} />
          {indicator?.signals?.length > 0 && (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-2">最新信号</h3>
              <div className="flex flex-wrap gap-2">
                {indicator.signals.slice(-8).reverse().map((s: any, i: number) => (
                  <span key={i} className={`px-2 py-1 rounded text-xs ${
                    s.Type === '金叉' ? 'bg-red-600' : s.Type === '死叉' ? 'bg-green-600' : 'bg-slate-700'
                  }`}>
                    {s.Date?.slice(5, 10)} {s.Indicator} {s.Type}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {tab === 'finance' && finance && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {[
            ['总股本', finance.ZongGuBen, '万股'],
            ['流通股本', finance.LiuTongGuBen, '万股'],
            ['总资产', finance.ZongZiChan, '万元'],
            ['净资产', finance.JingZiChan, '万元'],
            ['主营收入', finance.ZhuYingShouRu, '万元'],
            ['净利润', finance.JingLiRun, '万元'],
            ['每股净资产', finance.MeiGuJingZiChan, '元'],
            ['股东人数', finance.GuDongRenShu, '人'],
          ].map(([label, value, unit]) => (
            <div key={label as string} className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <div className="text-slate-400 text-sm">{label}</div>
              <div className="text-white text-xl font-bold mt-1">
                {typeof value === 'number' ? value.toLocaleString() : value}
                <span className="text-slate-500 text-sm ml-1">{unit}</span>
              </div>
            </div>
          ))}
        </div>
      )}

      {tab === 'company' && (
        <div className="flex gap-4">
          <div className="w-48 bg-slate-900 rounded-lg border border-slate-800 p-2 flex flex-col gap-1 max-h-96 overflow-auto">
            {companyCats.map(cat => (
              <button
                key={cat.Filename}
                onClick={() => loadCompanyContent(cat.Filename)}
                className={`text-left px-3 py-2 rounded text-sm ${
                  selectedCat === cat.Filename ? 'bg-blue-600 text-white' : 'text-slate-400 hover:bg-slate-800'
                }`}
              >
                {cat.Name}
              </button>
            ))}
          </div>
          <div className="flex-1 bg-slate-900 rounded-lg border border-slate-800 p-4 max-h-96 overflow-auto">
            <pre className="text-slate-300 text-sm whitespace-pre-wrap font-sans">{companyContent || '点击左侧目录查看内容'}</pre>
          </div>
        </div>
      )}

      {tab === 'dividend' && dividends.length > 0 && (
        <div className="bg-slate-900 rounded-lg border border-slate-800 overflow-hidden">
          <table className="w-full text-sm">
            <thead><tr className="border-b border-slate-800 text-slate-400">
              <th className="text-left p-3">日期</th>
              <th className="text-left p-3">类型</th>
              <th className="text-right p-3">分红(元)</th>
              <th className="text-right p-3">送转(股)</th>
              <th className="text-right p-3">配股价</th>
            </tr></thead>
            <tbody>
              {dividends.map((d, i) => (
                <tr key={i} className="border-b border-slate-800/50 hover:bg-slate-800/50">
                  <td className="p-3">{d.Date?.slice(0, 10)}</td>
                  <td className="p-3">{d.Category}</td>
                  <td className="p-3 text-right">{d.FenHong > 0 ? d.FenHong.toFixed(4) : '-'}</td>
                  <td className="p-3 text-right">{d.SongZhuanGu > 0 ? d.SongZhuanGu.toFixed(2) : '-'}</td>
                  <td className="p-3 text-right">{d.PeiGuJia > 0 ? d.PeiGuJia.toFixed(2) : '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'intraday' && (
        <div className="space-y-4">
          {minuteData.length > 0 && (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-2">分时走势</h3>
              <div className="grid grid-cols-4 md:grid-cols-8 gap-1 text-xs">
                {minuteData.slice(0, 48).map((m, i) => (
                  <div key={i} className="bg-slate-800 rounded p-1 text-center">
                    <div className="text-slate-400">{m.Time}</div>
                    <div className="text-white">{m.Price?.toFixed(2)}</div>
                  </div>
                ))}
              </div>
            </div>
          )}
          {quote && (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-2">五档盘口</h3>
              <div className="text-sm text-slate-400">
                内盘: {quote.SVol?.toLocaleString()} | 外盘: {quote.BVol?.toLocaleString()}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
