import { useEffect, useRef, useState } from 'react';
import { createChart, CandlestickSeries, HistogramSeries, LineSeries, type IChartApi, type Time } from 'lightweight-charts';
import type { KlineItem, IndicatorData } from '../../types/api';

interface Props {
  klines: KlineItem[];
  indicator?: IndicatorData;
  mainOverlay: string;
  subPanel: string;
}

interface HoverInfo {
  time: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  pct: number;
  ma: Record<string, number>;
  macd?: { dif: number; dea: number; hist: number };
  kdj?: { k: number; d: number; j: number };
  boll?: { upper: number; middle: number; lower: number };
  rsi?: Record<string, number>;
}

function toTime(dateStr: string): Time {
  return dateStr.slice(0, 10) as Time;
}

function safeTime(klines: KlineItem[], i: number): Time | null {
  const t = klines[i]?.Time;
  return t ? toTime(t.slice(0, 10)) : null;
}

function safeData(values: number[], klines: KlineItem[]): { time: Time; value: number }[] {
  const data: { time: Time; value: number }[] = [];
  for (let i = 0; i < values.length && i < klines.length; i++) {
    const time = safeTime(klines, i);
    if (time) data.push({ time, value: values[i] });
  }
  return data;
}

function fmtN(v: number, d = 2): string {
  return typeof v === 'number' && !isNaN(v) ? v.toFixed(d) : '-';
}

function fmtPct(v: number): string {
  if (typeof v !== 'number' || isNaN(v)) return '-';
  const sign = v > 0 ? '+' : '';
  return `${sign}${v.toFixed(2)}%`;
}

export default function CandlestickChart({ klines, indicator, mainOverlay, subPanel }: Props) {
  const mainRef = useRef<HTMLDivElement>(null);
  const subRef = useRef<HTMLDivElement>(null);
  const chartRefs = useRef<IChartApi[]>([]);
  const [hover, setHover] = useState<HoverInfo | null>(null);

  const MAIN_H = 320;
  const SUB_H = 150;

  useEffect(() => {
    const charts: IChartApi[] = [];
    chartRefs.current = [];

    const makeChart = (container: HTMLDivElement | null, h: number): IChartApi | null => {
      if (!container) return null;
      const chart = createChart(container, {
        width: container.clientWidth,
        height: h,
        layout: {
          background: { color: '#0f172a' },
          textColor: '#64748b',
          fontFamily: 'system-ui, sans-serif',
        },
        grid: {
          vertLines: { color: '#1e293b' },
          horzLines: { color: '#1e293b' },
        },
        crosshair: {
          mode: 1,
          vertLine: { color: '#3b82f6', width: 1, style: 2, labelBackgroundColor: '#3b82f6' },
          horzLine: { color: '#3b82f6', width: 1, style: 2, labelBackgroundColor: '#3b82f6' },
        },
        rightPriceScale: { borderColor: '#334155', scaleMargins: h === MAIN_H ? { top: 0.05, bottom: 0.2 } : { top: 0.1, bottom: 0.1 } },
        timeScale: { borderColor: '#334155', timeVisible: false, rightOffset: 5 },
      });
      charts.push(chart);
      return chart;
    };

    // Main chart
    const mainChart = makeChart(mainRef.current, MAIN_H);
    if (mainChart) {
      const candleSeries = mainChart.addSeries(CandlestickSeries, {
        upColor: '#ef4444', downColor: '#22c55e',
        borderUpColor: '#ef4444', borderDownColor: '#22c55e',
        wickUpColor: '#ef4444', wickDownColor: '#22c55e',
      });

      const candleData = klines.map(k => ({
        time: toTime(k.Time.slice(0, 10)),
        open: k.Open, high: k.High, low: k.Low, close: k.Close,
      }));
      candleSeries.setData(candleData);

      const volumeSeries = mainChart.addSeries(HistogramSeries, {
        priceFormat: { type: 'volume' },
        priceScaleId: '',
      });
      volumeSeries.priceScale().applyOptions({ scaleMargins: { top: 0.80, bottom: 0 } });
      volumeSeries.setData(klines.map(k => ({
        time: toTime(k.Time.slice(0, 10)),
        value: k.Volume,
        color: k.Close >= k.Open ? 'rgba(239,68,68,0.35)' : 'rgba(34,197,94,0.35)',
      })));

      // MA lines on main chart
      if (mainOverlay === 'MA' && indicator?.ma) {
        const maColors: Record<string, string> = { '5': '#f59e0b', '10': '#3b82f6', '20': '#8b5cf6', '60': '#ec4899' };
        for (const [period, values] of Object.entries(indicator.ma)) {
          const color = maColors[period];
          if (!color) continue;
          const series = mainChart.addSeries(LineSeries, { color, lineWidth: 1, priceLineVisible: false, lastValueVisible: false });
          const data = [];
          for (let j = 0; j < values.length && j < klines.length; j++) {
            if (values[j] > 0 && klines[j]?.Time) data.push({ time: toTime(klines[j].Time.slice(0, 10)), value: values[j] });
          }
          series.setData(data);
        }
      }

      if (mainOverlay === 'BOLL' && indicator?.boll) {
        const bollColors = { Upper: '#ef4444', Middle: '#f59e0b', Lower: '#22c55e' };
        for (const [key, color] of Object.entries(bollColors)) {
          const values = indicator.boll[key as keyof typeof indicator.boll] as number[];
          if (!values) continue;
          const series = mainChart.addSeries(LineSeries, { color, lineWidth: 1, priceLineVisible: false, lastValueVisible: false });
          series.setData(safeData(values, klines));
        }
      }

      // Signal markers
      if (indicator?.signals) {
        const markerMap: Record<string, { color: string; shape: 'arrowUp' | 'arrowDown'; pos: 'aboveBar' | 'belowBar' }> = {
          '金叉': { color: '#ef4444', shape: 'arrowUp', pos: 'belowBar' },
          '死叉': { color: '#22c55e', shape: 'arrowDown', pos: 'aboveBar' },
          '超买': { color: '#f59e0b', shape: 'arrowDown', pos: 'aboveBar' },
          '超卖': { color: '#3b82f6', shape: 'arrowUp', pos: 'belowBar' },
          '突破上轨': { color: '#a855f7', shape: 'arrowDown', pos: 'aboveBar' },
          '跌破下轨': { color: '#8b5cf6', shape: 'arrowUp', pos: 'belowBar' },
          '多头排列': { color: '#ef4444', shape: 'arrowUp', pos: 'belowBar' },
          '空头排列': { color: '#22c55e', shape: 'arrowDown', pos: 'aboveBar' },
        };
        const markers = indicator.signals
          .filter(s => s.Date)
          .map(s => {
            const m = markerMap[s.Type];
            return {
              time: toTime(s.Date.slice(0, 10)),
              position: m ? m.pos : 'belowBar',
              color: m ? m.color : '#64748b',
              shape: m ? m.shape : 'arrowUp',
              text: `${s.Indicator}${s.Type}`,
            };
          });
        if (markers.length > 0) {
          try { (candleSeries as any).setMarkers(markers.sort((a, b) => (a.time as string).localeCompare(b.time as string))); } catch {}
        }
      }

      // Crosshair move for hover info
      mainChart.subscribeCrosshairMove((param) => {
        if (!param.time) { setHover(null); return; }
        const idx = klines.findIndex(k => toTime(k.Time.slice(0, 10)) === param.time);
        if (idx < 0) { setHover(null); return; }
        const k = klines[idx];
        const prev = idx > 0 ? klines[idx - 1].Close : k.Close;
        const pct = prev > 0 ? (k.Close - prev) / prev * 100 : 0;
        const ma: Record<string, number> = {};
        if (indicator?.ma) {
          for (const [p, v] of Object.entries(indicator.ma)) {
            if (v[idx] > 0) ma[p] = v[idx];
          }
        }
        const rsi: Record<string, number> = {};
        if (indicator?.rsi) {
          for (const [p, v] of Object.entries(indicator.rsi)) {
            if (v[idx]) rsi[p] = v[idx];
          }
        }
        setHover({
          time: k.Time.slice(0, 10),
          open: k.Open, high: k.High, low: k.Low, close: k.Close, volume: k.Volume, pct,
          ma,
          macd: indicator?.macd ? { dif: indicator.macd.DIF[idx], dea: indicator.macd.DEA[idx], hist: indicator.macd.Hist[idx] } : undefined,
          kdj: indicator?.kdj ? { k: indicator.kdj.K[idx], d: indicator.kdj.D[idx], j: indicator.kdj.J[idx] } : undefined,
          boll: indicator?.boll ? { upper: indicator.boll.Upper[idx], middle: indicator.boll.Middle[idx], lower: indicator.boll.Lower[idx] } : undefined,
          rsi: Object.keys(rsi).length > 0 ? rsi : undefined,
        });
      });

      const visibleBars = Math.max(80, Math.floor((mainRef.current?.clientWidth || 1000) / 7));
      mainChart.timeScale().setVisibleLogicalRange({ from: klines.length - visibleBars, to: klines.length });
    }

    // MACD sub-panel
    if (subPanel && subRef.current) {
      const subChart = makeChart(subRef.current, SUB_H);
      if (subChart) {
        if (subPanel === 'MACD' && indicator?.macd) {
          subChart.addSeries(LineSeries, { color: '#f59e0b', lineWidth: 1, priceLineVisible: false, lastValueVisible: false })
            .setData(safeData(indicator.macd.DIF, klines));
          subChart.addSeries(LineSeries, { color: '#3b82f6', lineWidth: 1, priceLineVisible: false, lastValueVisible: false })
            .setData(safeData(indicator.macd.DEA, klines));
          const histData: { time: Time; value: number; color: string }[] = [];
          for (let i = 0; i < indicator.macd.Hist.length && i < klines.length; i++) {
            const time = safeTime(klines, i);
            if (time) histData.push({ time, value: indicator.macd.Hist[i], color: indicator.macd.Hist[i] >= 0 ? 'rgba(239,68,68,0.6)' : 'rgba(34,197,94,0.6)' });
          }
          subChart.addSeries(HistogramSeries, { priceLineVisible: false, lastValueVisible: false }).setData(histData);
        }

        if (subPanel === 'KDJ' && indicator?.kdj) {
          const addLine = (values: number[], color: string) => {
            subChart.addSeries(LineSeries, { color, lineWidth: 1, priceLineVisible: false, lastValueVisible: false })
              .setData(safeData(values, klines));
          };
          addLine(indicator.kdj.K, '#f59e0b');
          addLine(indicator.kdj.D, '#3b82f6');
          addLine(indicator.kdj.J, '#ef4444');
        }

        if (subPanel === 'RSI' && indicator?.rsi) {
          const rsiColors = ['#f59e0b', '#3b82f6', '#8b5cf6', '#ec4899'];
          let ci = 0;
          for (const [, values] of Object.entries(indicator.rsi)) {
            subChart.addSeries(LineSeries, { color: rsiColors[ci++ % rsiColors.length], lineWidth: 1, priceLineVisible: false, lastValueVisible: false })
              .setData(safeData(values, klines));
          }
        }
      }
    }

    // Sync time scale
    if (charts.length > 1) {
      const main = charts[0];
      main.timeScale().subscribeVisibleLogicalRangeChange(() => {
        const range = main.timeScale().getVisibleLogicalRange();
        if (!range) return;
        for (let i = 1; i < charts.length; i++) {
          charts[i].timeScale().setVisibleLogicalRange(range);
        }
      });
    }

    chartRefs.current = charts;

    // Fit all
    const visibleBars = Math.max(80, Math.floor((mainRef.current?.clientWidth || 1000) / 7));
    charts.forEach(c => c.timeScale().setVisibleLogicalRange({ from: klines.length - visibleBars, to: klines.length }));

    const handleResize = () => {
      charts.forEach((c, i) => {
        const container = [mainRef, subRef][i]?.current;
        if (container) c.applyOptions({ width: container.clientWidth });
      });
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      charts.forEach(c => c.remove());
    };
  }, [klines, indicator, mainOverlay, subPanel]);

  const last = klines[klines.length - 1];
  const defaultPct = last && klines.length > 1 ? ((last.Close - klines[klines.length - 2].Close) / klines[klines.length - 2].Close * 100) : 0;
  const h = hover;

  return (
    <div>
      <div className="bg-slate-900 border border-slate-800 border-b-0 rounded-t-lg px-3 py-1.5 flex flex-wrap gap-x-4 gap-y-0.5 text-xs min-h-[28px]">
        {h ? (
          <>
            <span className="text-slate-400 font-medium">{h.time}</span>
            <span>开 <span className="text-white">{fmtN(h.open)}</span></span>
            <span>高 <span className="text-red-400">{fmtN(h.high)}</span></span>
            <span>低 <span className="text-green-400">{fmtN(h.low)}</span></span>
            <span>收 <span className={h.pct >= 0 ? 'text-red-400' : 'text-green-400'}>{fmtN(h.close)}</span></span>
            <span className={h.pct >= 0 ? 'text-red-400' : 'text-green-400'}>{fmtPct(h.pct)}</span>
            <span>量 <span className="text-slate-300">{(h.volume / 10000).toFixed(1)}万</span></span>
            {mainOverlay === 'MA' && Object.entries(h.ma).map(([p, v]) => (
              <span key={p}>MA{p} <span className="text-slate-300">{fmtN(v)}</span></span>
            ))}
            {mainOverlay === 'BOLL' && h.boll && (
              <>
                <span>上轨 <span className="text-red-400">{fmtN(h.boll.upper)}</span></span>
                <span>中轨 <span className="text-yellow-400">{fmtN(h.boll.middle)}</span></span>
                <span>下轨 <span className="text-green-400">{fmtN(h.boll.lower)}</span></span>
              </>
            )}
            {subPanel === 'MACD' && h.macd && (
              <>
                <span>DIF <span className="text-yellow-400">{fmtN(h.macd.dif)}</span></span>
                <span>DEA <span className="text-blue-400">{fmtN(h.macd.dea)}</span></span>
                <span>HIST <span className={h.macd.hist >= 0 ? 'text-red-400' : 'text-green-400'}>{fmtN(h.macd.hist)}</span></span>
              </>
            )}
            {subPanel === 'KDJ' && h.kdj && (
              <>
                <span>K <span className="text-yellow-400">{fmtN(h.kdj.k, 1)}</span></span>
                <span>D <span className="text-blue-400">{fmtN(h.kdj.d, 1)}</span></span>
                <span>J <span className={h.kdj.j > 100 ? 'text-red-400' : h.kdj.j < 0 ? 'text-green-400' : 'text-white'}>{fmtN(h.kdj.j, 1)}</span></span>
              </>
            )}
            {subPanel === 'RSI' && h.rsi && Object.entries(h.rsi).map(([p, v]) => (
              <span key={p}>RSI{p} <span className={v > 80 ? 'text-red-400' : v < 20 ? 'text-green-400' : 'text-slate-300'}>{fmtN(v, 1)}</span></span>
            ))}
          </>
        ) : (
          <>
            <span className="text-slate-400 font-medium">{last?.Time?.slice(0, 10)}</span>
            <span>开 <span className="text-white">{fmtN(last?.Open)}</span></span>
            <span>高 <span className="text-red-400">{fmtN(last?.High)}</span></span>
            <span>低 <span className="text-green-400">{fmtN(last?.Low)}</span></span>
            <span>收 <span className={defaultPct >= 0 ? 'text-red-400' : 'text-green-400'}>{fmtN(last?.Close)}</span></span>
            <span className={defaultPct >= 0 ? 'text-red-400' : 'text-green-400'}>{fmtPct(defaultPct)}</span>
          </>
        )}
      </div>
      <div className="rounded-b-lg overflow-hidden border border-slate-800 border-t-0">
        <div ref={mainRef} style={{ height: MAIN_H }} />
        {subPanel && (
          <div className="border-t border-slate-800">
            <div ref={subRef} style={{ height: SUB_H }} />
          </div>
        )}
      </div>
    </div>
  );
}
