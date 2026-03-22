import { useState, useEffect, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { BarChart3, DollarSign, Building2, Gift, Clock } from 'lucide-react';
import { api } from '../../api/client';
import type { TradeItem, AuctionItem, SignalAnalysis as SignalAnalysisType } from '../../types/api';
import CandlestickChart from '../../components/charts/CandlestickChart';
import ChartToolbar from '../../components/charts/ChartToolbar';
import MinuteChart from '../../components/charts/MinuteChart';
import TabContent from '../../components/TabContent';
import { parseTdxText, renderTdxHtml } from '../../lib/tdx-parser';

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
  const [inputCode, setInputCode] = useState(paramCode || '000001');
  const [tab, setTab] = useState<Tab>((paramTab as Tab) || 'chart');
  const [quote, setQuote] = useState<any>(null);
  const [klines, setKlines] = useState<any[]>([]);
  const [indicator, setIndicator] = useState<any>(null);
  const [ktype, setKtype] = useState('day');
  const [mainOverlay, setMainOverlay] = useState('MA');
  const [subPanel, setSubPanel] = useState('MACD');
  const [finance, setFinance] = useState<any>(null);
  const [companyCats, setCompanyCats] = useState<any[]>([]);
  const [companyContent, setCompanyContent] = useState('');
  const [selectedCat, setSelectedCat] = useState('');
  const [dividends, setDividends] = useState<any[]>([]);
  const [minuteData, setMinuteData] = useState<any[]>([]);
  const [trades, setTrades] = useState<TradeItem[]>([]);
  const [auctions, setAuctions] = useState<AuctionItem[]>([]);
  const [analysis, setAnalysis] = useState<SignalAnalysisType | null>(null);
  const [highlightedIdx, setHighlightedIdx] = useState(-1);
  const tradeRowRefs = useRef<Record<number, HTMLDivElement | null>>({});
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (paramCode) {
      setCode(paramCode);
      setInputCode(paramCode);
    }
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
    api.indicator(code, ktype).then(data => {
      setIndicator(data);
      setKlines(data?.klines || []);
    }).catch(() => {});
    api.signalAnalysis(code, ktype).then(setAnalysis).catch(() => {});
    setLoading(false);
  }, [code, ktype]);

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
          type="text" value={inputCode}
          onChange={e => setInputCode(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && inputCode.length === 6) navigate(`/stock/${inputCode}`); }}
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
          <ChartToolbar
            ktype={ktype}
            onKtypeChange={setKtype}
            mainOverlay={mainOverlay}
            onMainOverlayChange={setMainOverlay}
            subPanel={subPanel}
            onSubPanelChange={setSubPanel}
          />
          <CandlestickChart
            klines={klines}
            indicator={indicator}
            mainOverlay={mainOverlay}
            subPanel={subPanel}
          />
          {analysis && analysis.summary.length > 0 && (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-1">信号回测</h3>
              <p className="text-slate-500 text-xs mb-3">基于历史 {analysis.count} 根K线中的 {analysis.signals} 个信号，统计信号发出后 N 个交易日的上涨概率和平均涨幅</p>
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead><tr className="border-b border-slate-800 text-slate-400">
                    <th className="text-left p-2">信号</th>
                    <th className="text-left p-2">操作建议</th>
                    <th className="text-right p-2">触发次数</th>
                    <th className="text-right p-2">次日上涨率</th>
                    <th className="text-right p-2">5日上涨率</th>
                    <th className="text-right p-2">10日上涨率</th>
                    <th className="text-right p-2">20日上涨率</th>
                    <th className="text-right p-2">次日均涨幅</th>
                    <th className="text-right p-2">5日均涨幅</th>
                  </tr></thead>
                  <tbody>
                    {analysis.summary.map((s, i) => (
                      <tr key={i} className="border-b border-slate-800/50 hover:bg-slate-800/50">
                        <td className="p-2 font-medium text-white">{s.type}</td>
                        <td className={`p-2 ${s.action === '买入参考' ? 'text-red-400' : 'text-green-400'}`}>{s.action}</td>
                        <td className="p-2 text-right">{s.count}</td>
                        <td className={`p-2 text-right ${s.valid1 > 0 && s.win1 >= 50 ? 'text-red-400' : s.valid1 > 0 ? 'text-green-400' : 'text-slate-600'}`}>
                          {s.valid1 > 0 ? `${s.win1.toFixed(0)}% (${s.valid1})` : '-'}
                        </td>
                        <td className={`p-2 text-right ${s.valid5 > 0 && s.win5 >= 50 ? 'text-red-400' : s.valid5 > 0 ? 'text-green-400' : 'text-slate-600'}`}>
                          {s.valid5 > 0 ? `${s.win5.toFixed(0)}% (${s.valid5})` : '-'}
                        </td>
                        <td className={`p-2 text-right ${s.valid10 > 0 && s.win10 >= 50 ? 'text-red-400' : s.valid10 > 0 ? 'text-green-400' : 'text-slate-600'}`}>
                          {s.valid10 > 0 ? `${s.win10.toFixed(0)}% (${s.valid10})` : '-'}
                        </td>
                        <td className={`p-2 text-right ${s.valid20 > 0 && s.win20 >= 50 ? 'text-red-400' : s.valid20 > 0 ? 'text-green-400' : 'text-slate-600'}`}>
                          {s.valid20 > 0 ? `${s.win20.toFixed(0)}% (${s.valid20})` : '-'}
                        </td>
                        <td className={`p-2 text-right ${s.valid1 > 0 && s.avg1 >= 0 ? 'text-red-400' : s.valid1 > 0 ? 'text-green-400' : 'text-slate-600'}`}>
                          {s.valid1 > 0 ? `${s.avg1 > 0 ? '+' : ''}${s.avg1.toFixed(2)}%` : '-'}
                        </td>
                        <td className={`p-2 text-right ${s.valid5 > 0 && s.avg5 >= 0 ? 'text-red-400' : s.valid5 > 0 ? 'text-green-400' : 'text-slate-600'}`}>
                          {s.valid5 > 0 ? `${s.avg5 > 0 ? '+' : ''}${s.avg5.toFixed(2)}%` : '-'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {analysis && analysis.outcomes.length > 0 && (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
              <h3 className="text-white font-medium mb-1">信号明细</h3>
              <p className="text-slate-500 text-xs mb-3">每次信号触发时的价格及后续涨跌，"-" 表示数据不足尚无结果</p>
              <div className="overflow-x-auto max-h-64 overflow-auto">
                <table className="w-full text-xs">
                  <thead className="sticky top-0 bg-slate-900"><tr className="border-b border-slate-800 text-slate-400">
                    <th className="text-left p-2">日期</th>
                    <th className="text-left p-2">指标</th>
                    <th className="text-left p-2">信号</th>
                    <th className="text-left p-2">建议</th>
                    <th className="text-right p-2">触发价</th>
                    <th className="text-right p-2">次日涨跌</th>
                    <th className="text-right p-2">5日涨跌</th>
                    <th className="text-right p-2">10日涨跌</th>
                    <th className="text-right p-2">20日涨跌</th>
                  </tr></thead>
                  <tbody>
                    {analysis.outcomes.slice().reverse().map((o, i) => {
                      const fmtChg = (v: number | null) => {
                        if (v === null || v === undefined) return <span className="text-slate-600">-</span>;
                        const cls = v >= 0 ? 'text-red-400' : 'text-green-400';
                        return <span className={cls}>{v > 0 ? '+' : ''}{v.toFixed(2)}%</span>;
                      };
                      return (
                        <tr key={i} className="border-b border-slate-800/50 hover:bg-slate-800/50">
                          <td className="p-2 text-slate-400">{o.date}</td>
                          <td className="p-2">{o.indicator}</td>
                          <td className="p-2 font-medium text-white">{o.type}</td>
                          <td className={`p-2 ${o.action === '买入参考' ? 'text-red-400' : 'text-green-400'}`}>{o.action}</td>
                          <td className="p-2 text-right">{o.price.toFixed(2)}</td>
                          <td className="p-2 text-right">{fmtChg(o.chg1)}</td>
                          <td className="p-2 text-right">{fmtChg(o.chg5)}</td>
                          <td className="p-2 text-right">{fmtChg(o.chg10)}</td>
                          <td className="p-2 text-right">{fmtChg(o.chg20)}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
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
              {companyContent ? (
                <div className="tdx-content text-sm text-slate-300" dangerouslySetInnerHTML={{
                  __html: renderTdxHtml(parseTdxText(companyContent))
                }} />
              ) : (
                <div className="text-slate-500 text-center py-8">点击左侧目录查看内容</div>
              )}
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
          <div className="h-full flex flex-col gap-4">
            {minuteData.length > 0 && (
              <MinuteChart
                data={minuteData}
                lastClose={quote?.LastClose || 0}
                onIndexClick={(idx) => {
                  setHighlightedIdx(idx);
                  const el = tradeRowRefs.current[idx];
                  if (el) {
                    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
                  }
                }}
              />
            )}

            <div className="flex gap-4 flex-1 min-h-0">
              {trades.length > 0 && (
                <div className="bg-slate-900 rounded-lg border border-slate-800 p-4 flex-1 overflow-hidden flex flex-col">
                  <h3 className="text-white font-medium mb-2">分笔成交</h3>
                  <div className="overflow-auto flex-1">
                    <div className="space-y-px">
                      {trades.map((t, i) => {
                        const tTime = fmtTime(t.Time);
                        const isHighlighted = i === highlightedIdx;
                        return (
                          <div
                            key={i}
                            ref={el => { tradeRowRefs.current[i] = el; }}
                            className={`grid grid-cols-4 gap-2 text-xs px-2 py-1 rounded transition-colors ${
                              isHighlighted ? 'bg-blue-600/30 ring-1 ring-blue-500' : 'hover:bg-slate-800'
                            }`}
                          >
                            <span className={isHighlighted ? 'text-white font-medium' : 'text-slate-400'}>{tTime}</span>
                            <span className="text-right text-white">{t.Price?.toFixed(2)}</span>
                            <span className="text-right text-slate-300">{t.Volume}</span>
                            <span className={`text-right ${t.Status === 0 ? 'text-red-400' : 'text-green-400'}`}>
                              {t.Status === 0 ? '买' : '卖'}
                            </span>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                </div>
              )}

              <div className="flex flex-col gap-4 w-64">
                {auctions.length > 0 && (
                  <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
                    <h3 className="text-white font-medium mb-2">集合竞价</h3>
                    <div className="space-y-px max-h-48 overflow-auto">
                      {auctions.slice(0, 10).map((a, i) => (
                        <div key={i} className="grid grid-cols-3 gap-1 text-xs px-1 py-0.5 hover:bg-slate-800 rounded">
                          <span className="text-slate-400">{fmtTime(a.time)}</span>
                          <span className="text-right text-white">{a.price?.toFixed(2)}</span>
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
                    <h3 className="text-white font-medium mb-2">盘口</h3>
                    <div className="space-y-1 text-sm">
                      <div className="flex justify-between"><span className="text-slate-500">开盘</span><span className="text-white">{quote.Open?.toFixed(2)}</span></div>
                      <div className="flex justify-between"><span className="text-slate-500">最高</span><span className="text-red-400">{quote.High?.toFixed(2)}</span></div>
                      <div className="flex justify-between"><span className="text-slate-500">最低</span><span className="text-green-400">{quote.Low?.toFixed(2)}</span></div>
                      <div className="flex justify-between"><span className="text-slate-500">昨收</span><span className="text-white">{quote.LastClose?.toFixed(2)}</span></div>
                      <div className="border-t border-slate-800 my-1" />
                      <div className="flex justify-between"><span className="text-slate-500">成交量</span><span className="text-white">{quote.Volume?.toLocaleString()}</span></div>
                      <div className="flex justify-between"><span className="text-slate-500">成交额</span><span className="text-white">{quote.Amount?.toLocaleString()}</span></div>
                      <div className="flex justify-between"><span className="text-slate-500">内盘</span><span className="text-green-400">{quote.SVol?.toLocaleString()}</span></div>
                      <div className="flex justify-between"><span className="text-slate-500">外盘</span><span className="text-red-400">{quote.BVol?.toLocaleString()}</span></div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        </TabContent>
      )}
    </div>
  );
}
