import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef } from 'react';
import { createChart, LineSeries, type IChartApi, type Time } from 'lightweight-charts';
import type { FinanceTrendRecord } from '../../types/api';

export type FinanceTrendMetric = {
  key: 'revenue' | 'netProfit' | 'grossMargin' | 'netMargin' | 'roe' | 'eps' | 'operatingCashPerShare';
  label: string;
  color: string;
  axis: 'amount' | 'percent' | 'perShare';
};

export type FinanceTrendChartHandle = {
  exportImage: () => string | null;
};

interface Props {
  records: FinanceTrendRecord[];
  metrics: FinanceTrendMetric[];
  mode: 'quarter' | 'year';
}

function toBusinessDay(index: number): Time {
  const month = String((index % 12) + 1).padStart(2, '0');
  return `2024-${month}-01` as Time;
}

function formatNumber(value: number, axis: FinanceTrendMetric['axis']) {
  if (axis === 'percent') return `${value.toFixed(2)}%`;
  if (axis === 'perShare') return `${value.toFixed(2)}元`;
  if (Math.abs(value) >= 100000000) return `${(value / 100000000).toFixed(2)}亿`;
  if (Math.abs(value) >= 10000) return `${(value / 10000).toFixed(2)}万`;
  return value.toFixed(2);
}

const FinanceTrendChart = forwardRef<FinanceTrendChartHandle, Props>(function FinanceTrendChart({ records, metrics, mode }: Props, ref) {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);

  const normalizedRecords = useMemo(
    () => records.map((record, index) => ({ ...record, chartTime: toBusinessDay(index) })),
    [records],
  );

  useImperativeHandle(ref, () => ({
    exportImage: () => {
      if (!chartRef.current) return null;
      return chartRef.current.takeScreenshot().toDataURL('image/png');
    },
  }), []);

  useEffect(() => {
    if (!containerRef.current || normalizedRecords.length === 0 || metrics.length === 0) return;

    const chart = createChart(containerRef.current, {
      width: containerRef.current.clientWidth,
      height: 360,
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
        vertLine: { color: '#3b82f6', width: 1, style: 2, labelBackgroundColor: '#3b82f6' },
        horzLine: { color: '#3b82f6', width: 1, style: 2, labelBackgroundColor: '#3b82f6' },
      },
      rightPriceScale: {
        borderColor: '#334155',
        scaleMargins: { top: 0.08, bottom: 0.12 },
      },
      leftPriceScale: {
        visible: true,
        borderColor: '#334155',
        scaleMargins: { top: 0.08, bottom: 0.12 },
      },
      timeScale: {
        borderColor: '#334155',
        timeVisible: true,
        secondsVisible: false,
        tickMarkFormatter: () => {
          return '';
        },
      },
      localization: {
        timeFormatter: (time: Time) => {
          const found = normalizedRecords.find((record) => record.chartTime === time);
          return found?.label ?? '';
        },
      },
    });

    const leftAxisSeries = new Set<FinanceTrendMetric['key']>(['grossMargin', 'netMargin', 'roe']);

    metrics.forEach((metric) => {
      const series = chart.addSeries(LineSeries, {
        color: metric.color,
        lineWidth: 2,
        priceLineVisible: false,
        lastValueVisible: true,
        title: metric.label,
        priceScaleId: leftAxisSeries.has(metric.key) ? 'left' : 'right',
      });
      series.setData(
        normalizedRecords
          .map((record) => {
            const value = record[metric.key];
            if (typeof value !== 'number' || Number.isNaN(value)) return null;
            return { time: record.chartTime, value };
          })
          .filter((item): item is { time: Time; value: number } => item !== null),
      );
    });

    chart.timeScale().fitContent();
    chartRef.current = chart;

    const handleResize = () => {
      if (containerRef.current && chartRef.current) {
        chartRef.current.applyOptions({ width: containerRef.current.clientWidth });
      }
    };

    window.addEventListener('resize', handleResize);
    return () => {
      window.removeEventListener('resize', handleResize);
      chart.remove();
      chartRef.current = null;
    };
  }, [metrics, normalizedRecords, mode]);

  const latest = normalizedRecords[normalizedRecords.length - 1];

  return (
    <div>
      <div className="bg-slate-900 border border-slate-800 border-b-0 rounded-t-lg px-3 py-2 flex flex-wrap gap-x-4 gap-y-1 text-xs min-h-[36px]">
        <span className="text-slate-400 font-medium">{latest?.label ?? (mode === 'year' ? '年度' : '季度')}</span>
        {metrics.map((metric) => {
          const value = latest?.[metric.key];
          return (
            <span key={metric.key}>
              {metric.label} <span style={{ color: metric.color }}>{typeof value === 'number' ? formatNumber(value, metric.axis) : '-'}</span>
            </span>
          );
        })}
      </div>
      <div className="rounded-b-lg overflow-hidden border border-slate-800 border-t-0">
        <div ref={containerRef} className="w-full" />
      </div>
    </div>
  );
});

export default FinanceTrendChart;
