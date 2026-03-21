import { useEffect, useRef } from 'react';
import { createChart, CandlestickSeries, HistogramSeries, LineSeries } from 'lightweight-charts';
import type { IChartApi, CandlestickData, HistogramData, LineData, Time } from 'lightweight-charts';
import type { KlineItem, IndicatorData } from '../../types/api';

interface Props {
  klines: KlineItem[];
  indicator?: IndicatorData;
  height?: number;
}

function toTime(dateStr: string): Time {
  return dateStr.slice(0, 10) as Time;
}

export default function CandlestickChart({ klines, indicator, height = 400 }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const chart = createChart(containerRef.current, {
      width: containerRef.current.clientWidth,
      height,
      layout: {
        background: { color: '#0f172a' },
        textColor: '#94a3b8',
        fontFamily: 'system-ui, sans-serif',
      },
      grid: {
        vertLines: { color: '#1e293b' },
        horzLines: { color: '#1e293b' },
      },
      crosshair: {
        mode: 1,
        vertLine: { color: '#3b82f6', width: 1, style: 2 },
        horzLine: { color: '#3b82f6', width: 1, style: 2 },
      },
      rightPriceScale: { borderColor: '#334155' },
      timeScale: { borderColor: '#334155', timeVisible: false },
    });

    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: '#ef4444',
      downColor: '#22c55e',
      borderUpColor: '#ef4444',
      borderDownColor: '#22c55e',
      wickUpColor: '#ef4444',
      wickDownColor: '#22c55e',
    });

    const candleData: CandlestickData[] = klines.map(k => ({
      time: toTime(k.Time.slice(0, 10)),
      open: k.Open,
      high: k.High,
      low: k.Low,
      close: k.Close,
    }));
    candleSeries.setData(candleData);

    const volumeSeries = chart.addSeries(HistogramSeries, {
      color: '#3b82f6',
      priceFormat: { type: 'volume' },
      priceScaleId: '',
    });
    volumeSeries.priceScale().applyOptions({ scaleMargins: { top: 0.85, bottom: 0 } });
    const volumeData: HistogramData[] = klines.map(k => ({
      time: toTime(k.Time.slice(0, 10)),
      value: k.Volume,
      color: k.Close >= k.Open ? 'rgba(239,68,68,0.4)' : 'rgba(34,197,94,0.4)',
    }));
    volumeSeries.setData(volumeData);

    if (indicator?.ma) {
      const maColors: Record<string, string> = { '5': '#f59e0b', '10': '#3b82f6', '20': '#8b5cf6', '60': '#ec4899' };
      for (const [period, values] of Object.entries(indicator.ma)) {
        const color = maColors[period];
        if (!color) continue;
        const series = chart.addSeries(LineSeries, { color, lineWidth: 1, priceLineVisible: false, lastValueVisible: false });
        const data: LineData[] = [];
        for (let j = 0; j < values.length && j < klines.length; j++) {
          if (values[j] > 0 && klines[j]?.Time) data.push({ time: toTime(klines[j].Time.slice(0, 10)), value: values[j] });
        }
        series.setData(data);
      }
    }

    if (indicator?.signals) {
      const markers = indicator.signals
        .filter(s => ['金叉', '死叉'].includes(s.Type) && s.Date)
        .map(s => ({
          time: toTime(s.Date.slice(0, 10)),
          position: s.Type === '金叉' ? 'belowBar' as const : 'aboveBar' as const,
          color: s.Type === '金叉' ? '#ef4444' : '#22c55e',
          shape: s.Type === '金叉' ? 'arrowUp' as const : 'arrowDown' as const,
          text: `${s.Indicator}${s.Type}`,
        }));
      if (markers.length > 0) {
        try { (candleSeries as any).setMarkers(markers.sort((a, b) => (a.time as string).localeCompare(b.time as string))); } catch {}
      }
    }

    chart.timeScale().fitContent();
    chartRef.current = chart;

    const handleResize = () => {
      if (containerRef.current) {
        chart.applyOptions({ width: containerRef.current.clientWidth });
      }
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      chart.remove();
    };
  }, [klines, indicator, height]);

  return <div ref={containerRef} className="w-full rounded-lg overflow-hidden" />;
}
