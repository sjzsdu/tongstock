import { useEffect, useMemo, useRef, useState } from 'react';
import { createChart, CandlestickSeries, HistogramSeries, LineSeries, type IChartApi, type Time } from 'lightweight-charts';
import type { KlineItem, IndicatorData } from '../../types/api';
import { formatDateTime, formatTdxDate } from '../../lib/datetime';

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

function isIntradayKline(klines: KlineItem[]): boolean {
  return new Set(klines.map((item) => item.Time?.slice(0, 10))).size < klines.length;
}

function toTime(dateStr: string | undefined, intraday: boolean): Time | null {
  if (!dateStr) return null;
  if (intraday) {
    const normalized = dateStr.includes(' ') ? dateStr.replace(' ', 'T') : dateStr;
    const ts = Math.floor(new Date(normalized).getTime() / 1000);
    if (Number.isFinite(ts)) return ts as Time;
    return null;
  }
  const day = dateStr.slice(0, 10);
  return /^\d{4}-\d{2}-\d{2}$/.test(day) ? day as Time : null;
}

function formatKlineTime(dateStr: string | undefined, intraday: boolean): string {
  if (!dateStr) return '-';
  return intraday ? formatDateTime(dateStr) : formatTdxDate(dateStr);
}

function toFiniteNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim() !== '') {
    const n = Number(value);
    if (Number.isFinite(n)) return n;
  }
  return null;
}

function safeTime(klines: KlineItem[], i: number, intraday: boolean): Time | null {
  const t = klines[i]?.Time;
  return toTime(t, intraday);
}

function safeData(values: unknown[] | undefined, klines: KlineItem[], intraday: boolean): { time: Time; value: number }[] {
  const data: { time: Time; value: number }[] = [];
  if (!Array.isArray(values)) return data;
  for (let i = 0; i < values.length && i < klines.length; i++) {
    const time = safeTime(klines, i, intraday);
    const value = toFiniteNumber(values[i]);
    if (time !== null && value !== null) data.push({ time, value });
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
  const [mousePos, setMousePos] = useState<{ x: number; y: number } | null>(null);

  const MAIN_H = 320;
  const SUB_H = 150;

  const chartKlines = useMemo(() => {
    const intraday = isIntradayKline(klines);
    const seen = new Set<string>();
    const cleaned: KlineItem[] = [];
    for (const k of klines) {
      const time = toTime(k.Time, intraday);
      if (time === null) continue;
      const key = String(time);
      if (seen.has(key)) continue;
      const open = toFiniteNumber(k.Open);
      const high = toFiniteNumber(k.High);
      const low = toFiniteNumber(k.Low);
      const close = toFiniteNumber(k.Close);
      const volume = toFiniteNumber(k.Volume) ?? 0;
      const amount = toFiniteNumber(k.Amount) ?? 0;
      if ([open, high, low, close].some((v) => v === null)) continue;
      if (high! < low!) continue;
      seen.add(key);
      cleaned.push({ ...k, Open: open!, High: high!, Low: low!, Close: close!, Volume: volume, Amount: amount });
    }
    return cleaned;
  }, [klines]);

  useEffect(() => {
    if (chartKlines.length === 0) return;
    const charts: IChartApi[] = [];
    chartRefs.current = [];
    const intraday = isIntradayKline(chartKlines);

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
        timeScale: { borderColor: '#334155', timeVisible: intraday, secondsVisible: false, rightOffset: 5 },
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

      const candleData = chartKlines.map(k => ({
        time: toTime(k.Time, intraday)!,
        open: k.Open, high: k.High, low: k.Low, close: k.Close,
      }));
      candleSeries.setData(candleData);

      const volumeSeries = mainChart.addSeries(HistogramSeries, {
        priceFormat: { type: 'volume' },
        priceScaleId: '',
      });
      volumeSeries.priceScale().applyOptions({ scaleMargins: { top: 0.80, bottom: 0 } });
      volumeSeries.setData(chartKlines.map(k => ({
        time: toTime(k.Time, intraday)!,
        value: k.Volume,
        color: k.Close >= k.Open ? 'rgba(239,68,68,0.35)' : 'rgba(34,197,94,0.35)',
      })));

      // MA lines on main chart
      if (mainOverlay === 'MA' && indicator?.ma) {
        const maColors: Record<string, string> = { '5': '#f59e0b', '10': '#3b82f6', '20': '#8b5cf6', '60': '#ec4899', '120': '#06b6d4' };
        for (const [period, values] of Object.entries(indicator.ma)) {
          const color = maColors[period];
          if (!color) continue;
          const series = mainChart.addSeries(LineSeries, { color, lineWidth: 1, priceLineVisible: false, lastValueVisible: false });
          const data = [];
          for (let j = 0; j < values.length && j < chartKlines.length; j++) {
            const time = safeTime(chartKlines, j, intraday);
            const value = toFiniteNumber(values[j]);
            if (value !== null && value > 0 && time) data.push({ time, value });
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
          series.setData(safeData(values, chartKlines, intraday));
        }
      }

      // Crosshair move for hover info
      mainChart.subscribeCrosshairMove((param) => {
        if (!param.time) { setHover(null); setMousePos(null); return; }
        const idx = chartKlines.findIndex(k => toTime(k.Time, intraday) === param.time);
        if (idx < 0) { setHover(null); setMousePos(null); return; }
        const k = chartKlines[idx];
        const prev = idx > 0 ? chartKlines[idx - 1].Close : k.Close;
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
          time: formatKlineTime(k.Time, intraday),
          open: k.Open, high: k.High, low: k.Low, close: k.Close, volume: k.Volume, pct,
          ma,
          macd: indicator?.macd ? { dif: indicator.macd.DIF[idx], dea: indicator.macd.DEA[idx], hist: indicator.macd.Hist[idx] } : undefined,
          kdj: indicator?.kdj ? { k: indicator.kdj.K[idx], d: indicator.kdj.D[idx], j: indicator.kdj.J[idx] } : undefined,
          boll: indicator?.boll ? { upper: indicator.boll.Upper[idx], middle: indicator.boll.Middle[idx], lower: indicator.boll.Lower[idx] } : undefined,
          rsi: Object.keys(rsi).length > 0 ? rsi : undefined,
        });
      });

      // Track mouse position for tooltip
      const chartContainer = mainRef.current;
      if (chartContainer) {
        const handleMouseMove = (e: MouseEvent) => {
          const rect = chartContainer.getBoundingClientRect();
          setMousePos({ x: e.clientX - rect.left, y: e.clientY - rect.top });
        };
        chartContainer.addEventListener('mousemove', handleMouseMove);
        // Clean up on chart removal
        const origRemove = charts[0]?.remove.bind(charts[0]);
        if (origRemove) {
          charts[0].remove = () => {
            chartContainer.removeEventListener('mousemove', handleMouseMove);
            origRemove();
          };
        }
      }

      const visibleBars = Math.max(80, Math.floor((mainRef.current?.clientWidth || 1000) / 7));
      mainChart.timeScale().setVisibleLogicalRange({ from: Math.max(0, chartKlines.length - visibleBars), to: chartKlines.length });
    }

    // MACD sub-panel
    if (subPanel && subRef.current) {
      const subChart = makeChart(subRef.current, SUB_H);
      if (subChart) {
        if (subPanel === 'MACD' && indicator?.macd) {
          subChart.addSeries(LineSeries, { color: '#f59e0b', lineWidth: 1, priceLineVisible: false, lastValueVisible: false })
            .setData(safeData(indicator.macd.DIF, chartKlines, intraday));
          subChart.addSeries(LineSeries, { color: '#3b82f6', lineWidth: 1, priceLineVisible: false, lastValueVisible: false })
            .setData(safeData(indicator.macd.DEA, chartKlines, intraday));
          const histData: { time: Time; value: number; color: string }[] = [];
          for (let i = 0; i < indicator.macd.Hist.length && i < chartKlines.length; i++) {
            const time = safeTime(chartKlines, i, intraday);
            const value = toFiniteNumber(indicator.macd.Hist[i]);
            if (time && value !== null) histData.push({ time, value, color: value >= 0 ? 'rgba(239,68,68,0.6)' : 'rgba(34,197,94,0.6)' });
          }
          subChart.addSeries(HistogramSeries, { priceLineVisible: false, lastValueVisible: false }).setData(histData);
        }

        if (subPanel === 'KDJ' && indicator?.kdj) {
          const addLine = (values: number[], color: string) => {
            subChart.addSeries(LineSeries, { color, lineWidth: 1, priceLineVisible: false, lastValueVisible: false })
              .setData(safeData(values, chartKlines, intraday));
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
              .setData(safeData(values, chartKlines, intraday));
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
    charts.forEach(c => c.timeScale().setVisibleLogicalRange({ from: Math.max(0, chartKlines.length - visibleBars), to: chartKlines.length }));

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
  }, [chartKlines, indicator, mainOverlay, subPanel]);

  const last = chartKlines[chartKlines.length - 1];
  const intraday = isIntradayKline(chartKlines);
  const defaultPct = last && chartKlines.length > 1 ? ((last.Close - chartKlines[chartKlines.length - 2].Close) / chartKlines[chartKlines.length - 2].Close * 100) : 0;
  const h = hover;

  return (
    <div className="relative">
      {/* Static header - always shows last kline info */}
      <div className="bg-slate-900 border border-slate-800 border-b-0 rounded-t-lg px-3 py-1.5 flex flex-wrap gap-x-4 gap-y-0.5 text-xs min-h-[28px]">
        <span className="text-slate-400 font-medium">{formatKlineTime(last?.Time, intraday)}</span>
        <span>开 <span className="text-white">{fmtN(last?.Open)}</span></span>
        <span>高 <span className="text-red-400">{fmtN(last?.High)}</span></span>
        <span>低 <span className="text-green-400">{fmtN(last?.Low)}</span></span>
        <span>收 <span className={defaultPct >= 0 ? 'text-red-400' : 'text-green-400'}>{fmtN(last?.Close)}</span></span>
        <span className={defaultPct >= 0 ? 'text-red-400' : 'text-green-400'}>{fmtPct(defaultPct)}</span>
      </div>

      {/* Chart area */}
      <div className="rounded-b-lg overflow-hidden border border-slate-800 border-t-0">
        <div ref={mainRef} style={{ height: MAIN_H }} />
        {subPanel && (
          <div className="border-t border-slate-800">
            <div ref={subRef} style={{ height: SUB_H }} />
          </div>
        )}
      </div>

      {/* Floating tooltip */}
      {h && mousePos && (
        <div
          className="absolute z-50 pointer-events-none bg-slate-800/95 border border-slate-700 rounded-lg px-3 py-2 text-xs shadow-xl"
          style={{
            left: mousePos.x + 16,
            top: mousePos.y + 16,
            transform: mousePos.x > 300 ? 'translateX(-110%)' : 'none',
          }}
        >
          <div className="font-medium text-slate-300 mb-1">{h.time}</div>
          <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
            <span className="text-slate-400">开</span><span className="text-white text-right">{fmtN(h.open)}</span>
            <span className="text-slate-400">高</span><span className="text-red-400 text-right">{fmtN(h.high)}</span>
            <span className="text-slate-400">低</span><span className="text-green-400 text-right">{fmtN(h.low)}</span>
            <span className="text-slate-400">收</span><span className={h.pct >= 0 ? 'text-red-400 text-right' : 'text-green-400 text-right'}>{fmtN(h.close)}</span>
            <span className="text-slate-400">涨跌</span><span className={h.pct >= 0 ? 'text-red-400 text-right' : 'text-green-400 text-right'}>{fmtPct(h.pct)}</span>
            <span className="text-slate-400">量</span><span className="text-slate-300 text-right">{(h.volume / 10000).toFixed(1)}万</span>
          </div>
          {mainOverlay === 'MA' && Object.keys(h.ma).length > 0 && (
            <div className="mt-1 pt-1 border-t border-slate-700 grid grid-cols-3 gap-x-3 gap-y-0.5">
              {Object.entries(h.ma).map(([p, v]) => (
                <span key={p} className="text-slate-400">MA{p} <span className="text-slate-200">{fmtN(v)}</span></span>
              ))}
            </div>
          )}
          {mainOverlay === 'BOLL' && h.boll && (
            <div className="mt-1 pt-1 border-t border-slate-700 grid grid-cols-3 gap-x-3 gap-y-0.5">
              <span className="text-slate-400">上轨 <span className="text-red-400">{fmtN(h.boll.upper)}</span></span>
              <span className="text-slate-400">中轨 <span className="text-yellow-400">{fmtN(h.boll.middle)}</span></span>
              <span className="text-slate-400">下轨 <span className="text-green-400">{fmtN(h.boll.lower)}</span></span>
            </div>
          )}
          {subPanel === 'MACD' && h.macd && (
            <div className="mt-1 pt-1 border-t border-slate-700 grid grid-cols-3 gap-x-3 gap-y-0.5">
              <span className="text-slate-400">DIF <span className="text-yellow-400">{fmtN(h.macd.dif)}</span></span>
              <span className="text-slate-400">DEA <span className="text-blue-400">{fmtN(h.macd.dea)}</span></span>
              <span className="text-slate-400">HIST <span className={h.macd.hist >= 0 ? 'text-red-400' : 'text-green-400'}>{fmtN(h.macd.hist)}</span></span>
            </div>
          )}
          {subPanel === 'KDJ' && h.kdj && (
            <div className="mt-1 pt-1 border-t border-slate-700 grid grid-cols-3 gap-x-3 gap-y-0.5">
              <span className="text-slate-400">K <span className="text-yellow-400">{fmtN(h.kdj.k, 1)}</span></span>
              <span className="text-slate-400">D <span className="text-blue-400">{fmtN(h.kdj.d, 1)}</span></span>
              <span className="text-slate-400">J <span className={h.kdj.j > 100 ? 'text-red-400' : h.kdj.j < 0 ? 'text-green-400' : 'text-white'}>{fmtN(h.kdj.j, 1)}</span></span>
            </div>
          )}
          {subPanel === 'RSI' && h.rsi && Object.keys(h.rsi).length > 0 && (
            <div className="mt-1 pt-1 border-t border-slate-700 grid grid-cols-3 gap-x-3 gap-y-0.5">
              {Object.entries(h.rsi).map(([p, v]) => (
                <span key={p} className="text-slate-400">RSI{p} <span className={v > 80 ? 'text-red-400' : v < 20 ? 'text-green-400' : 'text-slate-200'}>{fmtN(v, 1)}</span></span>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
