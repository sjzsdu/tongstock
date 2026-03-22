import { useEffect, useRef } from 'react';
import { createChart, LineSeries, HistogramSeries, type IChartApi, type Time } from 'lightweight-charts';
import type { MinuteItem } from '../../types/api';

interface Props {
  data: MinuteItem[];
  lastClose: number;
  onIndexClick?: (index: number) => void;
}

function toFakeDate(baseDate: string, index: number): Time {
  const d = new Date(baseDate);
  d.setDate(d.getDate() + index);
  return d.toISOString().slice(0, 10) as Time;
}

export default function MinuteChart({ data, lastClose, onIndexClick }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const sortedRef = useRef<MinuteItem[]>([]);

  useEffect(() => {
    if (!containerRef.current || data.length === 0) return;

    const sorted = [...data].reverse();
    sortedRef.current = sorted;
    const baseDate = '2000-01-01';

    const chart = createChart(containerRef.current, {
      width: containerRef.current.clientWidth,
      height: 300,
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
      rightPriceScale: { borderColor: '#334155', scaleMargins: { top: 0.05, bottom: 0.2 } },
      timeScale: {
        borderColor: '#334155',
        timeVisible: false,
        tickMarkFormatter: (time: Time) => {
          const idx = sorted.findIndex((_, i) => toFakeDate(baseDate, i) === time);
          return idx >= 0 ? sorted[idx].Time : '';
        },
      },
    });

    const priceLine = chart.addSeries(LineSeries, {
      color: '#3b82f6',
      lineWidth: 1,
      priceLineVisible: false,
      lastValueVisible: true,
      crosshairMarkerVisible: true,
    });

    priceLine.setData(sorted.map((m, i) => ({
      time: toFakeDate(baseDate, i),
      value: m.Price,
    })));

    if (lastClose > 0) {
      priceLine.createPriceLine({
        price: lastClose,
        color: '#f59e0b',
        lineWidth: 1,
        lineStyle: 3,
        axisLabelVisible: true,
        title: '昨收',
      });
    }

    const volSeries = chart.addSeries(HistogramSeries, {
      priceFormat: { type: 'volume' },
      priceScaleId: '',
    });
    volSeries.priceScale().applyOptions({ scaleMargins: { top: 0.82, bottom: 0 } });
    volSeries.setData(sorted.map((m, i) => ({
      time: toFakeDate(baseDate, i),
      value: m.Number || 0,
      color: m.Price >= lastClose ? 'rgba(239,68,68,0.35)' : 'rgba(34,197,94,0.35)',
    })));

    chart.timeScale().fitContent();

    chart.subscribeClick((param) => {
      if (param.time && onIndexClick) {
        const idx = sorted.findIndex((_, i) => toFakeDate(baseDate, i) === param.time);
        if (idx >= 0) onIndexClick(idx);
      }
    });

    chartRef.current = chart;

    const handleResize = () => {
      if (containerRef.current) chart.applyOptions({ width: containerRef.current.clientWidth });
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      chart.remove();
    };
  }, [data, lastClose, onIndexClick]);

  return <div ref={containerRef} className="w-full rounded-lg overflow-hidden" />;
}
