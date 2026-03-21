import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { BarChart3, DollarSign, Building2, Gift, Clock } from 'lucide-react';
import { api } from '../../api/client';
import type { TradeItem, AuctionItem } from '../../types/api';
import CandlestickChart from '../../components/charts/CandlestickChart';
import TabContent from '../../components/TabContent';

function fmtTime(t: string): string {
  if (!t) return '';
  const m = t.match(/T(\d{2}:\d{2})/);
  return m ? m[1] : t;
}

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
  const [trades, setTrades] = useState<TradeItem[]>([]);
  const [auctions, setAuctions] = useState<AuctionItem[]>([]);
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
    if (tab === 'company') api.company(code).then(cats => {
      setCompanyCats(cats);
      if (cats.length > 0 && !selectedCat) loadCompanyContent(cats[0].Name);
    }).catch(() => {});
    if (tab === 'dividend') api.xdxr(code).then(d => setDividends([...d].reverse())).catch(() => {});
    if (tab === 'intraday') {
      api.minute(code).then(r => setMinuteData([...(r.List || [])].reverse())).catch(() => {});
      api.trade(code).then(r => setTrades([...(r.List || [])].reverse())).catch(() => {});
      api.auction(code).then(r => setAuctions([...(r.List || [])].reverse())).catch(() => {});
    }
  }, [code, tab]);

  const loadCompanyContent = async (catName: string) => {
    setSelectedCat(catName);
    try {
      const r = await api.companyContent(code, catName);
      setCompanyContent((r.content || '').replace(/\r/g, ''));
    } catch { setCompanyContent('加载失败'); }
  };

  const pct = quote ? ((quote.Price - quote.LastClose) / quote.LastClose * 100) : 0;
  const up = pct >= 0;

  return (
    <div className="flex flex-col h-full min-h-0 gap-4">
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
        <TabContent>
          <div className="flex gap-4 h-full">
            <div className="w-48 bg-slate-900 rounded-lg border border-slate-800 p-2 flex flex-col gap-1 overflow-auto">
              {companyCats.map(cat => (
                <button
                  key={cat.Name}
                  onClick={() => loadCompanyContent(cat.Name)}
                  className={`text-left px-3 py-2 rounded text-sm cursor-pointer ${
                    selectedCat === cat.Name ? 'bg-blue-600 text-white' : 'text-slate-400 hover:bg-slate-800'
                  }`}
                >
                  {cat.Name}
                </button>
              ))}
            </div>
            <div className="flex-1 bg-slate-900 rounded-lg border border-slate-800 p-4 overflow-auto">
              <pre className="text-slate-300 text-xs whitespace-pre leading-relaxed" style={{ fontFamily: '"Sarasa Mono SC", "Noto Sans Mono CJK SC", "WenQuanYi Micro Hei Mono", "Microsoft YaHei", Menlo, Consolas, monospace' }}>{companyContent || '点击左侧目录查看内容'}</pre>
            </div>
          </div>
        </TabContent>
      )}

      {tab === 'dividend' && dividends.length > 0 && (
        <TabContent>
          <div className="bg-slate-900 rounded-lg border border-slate-800 overflow-hidden h-full flex flex-col">
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-slate-900 z-10"><tr className="border-b border-slate-800 text-slate-400">
              <th className="text-left p-3">日期</th>
              <th className="text-left p-3">类型</th>
              <th className="text-right p-3">分红(元)</th>
              <th className="text-right p-3">送转(股)</th>
              <th className="text-right p-3">配股价</th>
              <th className="text-right p-3">流通盘</th>
              <th className="text-right p-3">总股本</th>
            </tr></thead>
          </table>
          <div className="overflow-auto flex-1">
            <table className="w-full text-sm">
              <tbody>
                {dividends.map((d, i) => (
                  <tr key={i} className="border-b border-slate-800/50 hover:bg-slate-800/50">
                    <td className="p-3">{d.Date?.slice(0, 10)}</td>
                    <td className="p-3">{d.Category}</td>
                    <td className="p-3 text-right text-red-400">{d.FenHong > 0 ? d.FenHong.toFixed(4) : '-'}</td>
                    <td className="p-3 text-right">{d.SongZhuanGu > 0 ? d.SongZhuanGu.toFixed(2) : '-'}</td>
                    <td className="p-3 text-right">{d.PeiGuJia > 0 ? d.PeiGuJia.toFixed(2) : '-'}</td>
                    <td className="p-3 text-right text-slate-400">{d.PanHouLiuTong > 0 ? (d.PanHouLiuTong / 10000).toFixed(1) + '万' : '-'}</td>
                    <td className="p-3 text-right text-slate-400">{d.HouZongGuBen > 0 ? (d.HouZongGuBen / 10000).toFixed(1) + '万' : '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          </div>
        </TabContent>
      )}

      {tab === 'intraday' && (
        <TabContent>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 h-full overflow-auto">
          {minuteData.length > 0 && (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-3">分时走势</h3>
              <div className="space-y-px max-h-80 overflow-auto">
                <div className="grid grid-cols-4 gap-2 text-xs text-slate-500 px-1 mb-1 sticky top-0 bg-slate-900">
                  <span>时间</span><span className="text-right">价格</span><span className="text-right">均价</span><span className="text-right">成交量</span>
                </div>
                {minuteData.map((m, i) => {
                  const prev = i > 0 ? minuteData[i - 1].Price : (quote?.LastClose || m.Price);
                  const up = m.Price >= prev;
                  return (
                    <div key={i} className="grid grid-cols-4 gap-2 text-xs px-1 py-0.5 hover:bg-slate-800 rounded">
                      <span className="text-slate-400">{m.Time}</span>
                      <span className={`text-right ${up ? 'text-red-400' : 'text-green-400'}`}>{m.Price?.toFixed(2)}</span>
                      <span className="text-right text-yellow-500">{m.Price?.toFixed(2)}</span>
                      <span className="text-right text-slate-300">{m.Number}</span>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {trades.length > 0 && (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-3">分笔成交</h3>
              <div className="space-y-px max-h-80 overflow-auto">
                <div className="grid grid-cols-4 gap-2 text-xs text-slate-500 px-1 mb-1 sticky top-0 bg-slate-900">
                  <span>时间</span><span className="text-right">价格</span><span className="text-right">成交量</span><span className="text-right">方向</span>
                </div>
                {trades.map((t, i) => (
                  <div key={i} className="grid grid-cols-4 gap-2 text-xs px-1 py-0.5 hover:bg-slate-800 rounded">
                    <span className="text-slate-400">{fmtTime(t.Time)}</span>
                    <span className="text-right text-white">{t.Price?.toFixed(2)}</span>
                    <span className="text-right text-slate-300">{t.Volume}</span>
                    <span className={`text-right ${t.Status === 0 ? 'text-red-400' : 'text-green-400'}`}>
                      {t.Status === 0 ? '买' : '卖'}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {auctions.length > 0 && (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-3">集合竞价</h3>
              <div className="space-y-px max-h-80 overflow-auto">
                <div className="grid grid-cols-5 gap-2 text-xs text-slate-500 px-1 mb-1 sticky top-0 bg-slate-900">
                  <span>时间</span><span className="text-right">价格</span><span className="text-right">匹配量</span><span className="text-right">未匹配</span><span className="text-right">方向</span>
                </div>
                {auctions.map((a, i) => (
                  <div key={i} className="grid grid-cols-5 gap-2 text-xs px-1 py-0.5 hover:bg-slate-800 rounded">
                    <span className="text-slate-400">{fmtTime(a.time)}</span>
                    <span className="text-right text-white">{a.price?.toFixed(2)}</span>
                    <span className="text-right text-slate-300">{a.match}</span>
                    <span className="text-right text-slate-400">{a.unmatched}</span>
                    <span className={`text-right ${a.flag >= 0 ? 'text-red-400' : 'text-green-400'}`}>
                      {a.flag >= 0 ? '买' : '卖'}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {quote && (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-3">盘口信息</h3>
              <div className="grid grid-cols-2 gap-4 text-sm">
                <div>
                  <div className="text-slate-400 mb-2">基本信息</div>
                  <div className="space-y-1">
                    <div className="flex justify-between"><span className="text-slate-500">开盘</span><span className="text-white">{quote.Open?.toFixed(2)}</span></div>
                    <div className="flex justify-between"><span className="text-slate-500">最高</span><span className="text-red-400">{quote.High?.toFixed(2)}</span></div>
                    <div className="flex justify-between"><span className="text-slate-500">最低</span><span className="text-green-400">{quote.Low?.toFixed(2)}</span></div>
                    <div className="flex justify-between"><span className="text-slate-500">昨收</span><span className="text-white">{quote.LastClose?.toFixed(2)}</span></div>
                  </div>
                </div>
                <div>
                  <div className="text-slate-400 mb-2">成交信息</div>
                  <div className="space-y-1">
                    <div className="flex justify-between"><span className="text-slate-500">成交量</span><span className="text-white">{quote.Volume?.toLocaleString()}</span></div>
                    <div className="flex justify-between"><span className="text-slate-500">成交额</span><span className="text-white">{quote.Amount?.toLocaleString()}</span></div>
                    <div className="flex justify-between"><span className="text-slate-500">内盘</span><span className="text-green-400">{quote.SVol?.toLocaleString()}</span></div>
                    <div className="flex justify-between"><span className="text-slate-500">外盘</span><span className="text-red-400">{quote.BVol?.toLocaleString()}</span></div>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
        </TabContent>
      )}
    </div>
  );
}
