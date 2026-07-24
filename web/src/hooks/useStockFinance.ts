import { useState, useEffect, useMemo } from 'react';
import { api } from '../api/client';
import type { FinanceTrendsResponse, FinanceMetricsResponse, FinanceTrendRecord } from '../types/api';
import type { DetailStatus } from './useStockDetail';

export type FinanceCompareMode = 'raw' | 'yoy' | 'qoq';
export type FinanceViewMode = 'chart' | 'table';
export interface FinanceTrendMetric {
  key: string;
  label: string;
  color: string;
  axis: 'amount' | 'percent' | 'perShare';
}

const FINANCE_METRICS: FinanceTrendMetric[] = [
  { key: 'revenue', label: '营业收入', color: '#3b82f6', axis: 'amount' },
  { key: 'netProfit', label: '净利润', color: '#22c55e', axis: 'amount' },
  { key: 'grossMargin', label: '销售毛利率', color: '#f59e0b', axis: 'percent' },
  { key: 'netMargin', label: '净利润率', color: '#ef4444', axis: 'percent' },
  { key: 'roe', label: '净资产收益率', color: '#8b5cf6', axis: 'percent' },
  { key: 'eps', label: '每股收益', color: '#ec4899', axis: 'perShare' },
  { key: 'operatingCashPerShare', label: '每股经营现金流', color: '#14b8a6', axis: 'perShare' },
];

const FINANCE_CHART_GROUPS = [
  {
    key: 'amount',
    title: '规模趋势',
    description: '收入、利润与每股指标',
    axes: ['amount', 'perShare'] as FinanceTrendMetric['axis'][],
  },
  {
    key: 'margin',
    title: '利润率趋势',
    description: '毛利率、净利率与 ROE',
    axes: ['percent'] as FinanceTrendMetric['axis'][],
  },
] as const;

function formatFinanceMetricValue(value: number | undefined, metric: FinanceTrendMetric): string {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-';
  if (metric.axis === 'percent') return `${value.toFixed(2)}%`;
  if (metric.axis === 'perShare') return `${value.toFixed(2)}元`;
  if (Math.abs(value) >= 100000000) return `${(value / 100000000).toFixed(2)}亿`;
  if (Math.abs(value) >= 10000) return `${(value / 10000).toFixed(2)}万`;
  return value.toFixed(2);
}

function calcFinanceCompareValue(records: FinanceTrendRecord[], index: number, metricKey: string, mode: FinanceCompareMode): number | undefined {
  const current = (records[index] as unknown as Record<string, number | undefined>)?.[metricKey];
  if (typeof current !== 'number' || Number.isNaN(current)) return undefined;
  if (mode === 'raw') return current;

  const offset = mode === 'yoy' ? 4 : 1;
  const previous = (records[index - offset] as unknown as Record<string, number | undefined>)?.[metricKey];
  if (typeof previous !== 'number' || Number.isNaN(previous) || previous === 0) return undefined;
  return ((current - previous) / Math.abs(previous)) * 100;
}

function buildFinanceComparisonRecords(
  records: FinanceTrendRecord[],
  metrics: FinanceTrendMetric[],
  mode: FinanceCompareMode,
): FinanceTrendRecord[] {
  if (mode === 'raw') return records;
  return records
    .map((record, index) => {
      const next: FinanceTrendRecord = { ...record };
      const nextRecord = next as unknown as Record<string, number | undefined>;
      let hasValue = false;
      metrics.forEach((metric) => {
        const value = calcFinanceCompareValue(records, index, metric.key, mode);
        if (typeof value === 'number' && !Number.isNaN(value)) {
          nextRecord[metric.key] = value;
          hasValue = true;
        } else {
          delete nextRecord[metric.key];
        }
      });
      return hasValue ? next : null;
    })
    .filter((record): record is FinanceTrendRecord => record !== null);
}

export interface UseStockFinanceReturn {
  finance: any;
  financeTrends: FinanceTrendsResponse | null;
  financeMetrics: FinanceMetricsResponse | null;
  financeTrendMode: 'quarter' | 'year';
  setFinanceTrendMode: (mode: 'quarter' | 'year') => void;
  financeCompareMode: FinanceCompareMode;
  setFinanceCompareMode: (mode: FinanceCompareMode) => void;
  financeViewMode: FinanceViewMode;
  setFinanceViewMode: (mode: FinanceViewMode) => void;
  selectedFinanceMetrics: string[];
  setSelectedFinanceMetrics: (metrics: string[]) => void;
  financeTrendLoading: boolean;
  availableFinanceMetrics: FinanceTrendMetric[];
  financeChartGroups: (typeof FINANCE_CHART_GROUPS[number] & { metrics: FinanceTrendMetric[] })[];
  financeDisplayRecords: FinanceTrendRecord[];
  activeFinanceMetrics: FinanceTrendMetric[];
  latestFinanceRecord: FinanceTrendRecord | undefined;
  formatFinanceMetricValue: (value: number | undefined, metric: FinanceTrendMetric) => string;
  financeItems: any[][];
}

export function useStockFinance(code: string, detailStatus: DetailStatus): UseStockFinanceReturn {
  const [finance, setFinance] = useState<any>(null);
  const [financeTrends, setFinanceTrends] = useState<FinanceTrendsResponse | null>(null);
  const [financeMetrics, setFinanceMetrics] = useState<FinanceMetricsResponse | null>(null);
  const [financeTrendMode, setFinanceTrendMode] = useState<'quarter' | 'year'>('quarter');
  const [financeCompareMode, setFinanceCompareMode] = useState<FinanceCompareMode>('raw');
  const [financeViewMode, setFinanceViewMode] = useState<FinanceViewMode>('chart');
  const [selectedFinanceMetrics, setSelectedFinanceMetrics] = useState<string[]>(['revenue', 'netProfit', 'roe']);
  const [financeTrendLoading, setFinanceTrendLoading] = useState(false);

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    api.finance(code).then(setFinance).catch(() => {});
    api.financeMetrics(code).then(setFinanceMetrics).catch(() => setFinanceMetrics(null));
  }, [code, detailStatus]);

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    let cancelled = false;
    setFinanceTrendLoading(true);
    api.financeTrends(code, financeTrendMode)
      .then((data) => {
        if (cancelled) return;
        setFinanceTrends(data);
      })
      .catch(() => {
        if (cancelled) return;
        setFinanceTrends(null);
      })
      .finally(() => {
        if (!cancelled) setFinanceTrendLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [code, detailStatus, financeTrendMode]);

  const availableFinanceMetrics = useMemo(() => {
    const trendMetrics = new Set(financeTrends?.metrics ?? []);
    return FINANCE_METRICS.filter((metric) => trendMetrics.has(metric.key));
  }, [financeTrends]);

  const financeChartGroups = useMemo(() => {
    return FINANCE_CHART_GROUPS.map((group) => ({
      ...group,
      metrics: availableFinanceMetrics.filter((metric) => group.axes.includes(metric.axis)),
    })).filter((group) => group.metrics.length > 0);
  }, [availableFinanceMetrics]);

  const financeDisplayRecords = useMemo(() => {
    if (!financeTrends) return [];
    return buildFinanceComparisonRecords(financeTrends.records, availableFinanceMetrics, financeCompareMode);
  }, [availableFinanceMetrics, financeCompareMode, financeTrends]);

  const activeFinanceMetrics = useMemo(() => {
    const selected = availableFinanceMetrics.filter((metric) => selectedFinanceMetrics.includes(metric.key));
    return selected.length > 0 ? selected : availableFinanceMetrics.slice(0, 3);
  }, [availableFinanceMetrics, selectedFinanceMetrics]);

  const latestFinanceRecord = financeDisplayRecords[financeDisplayRecords.length - 1];

  const financeItems = finance ? [
    ['总股本', finance.ZongGuBen, '万股'],
    ['流通股本', finance.LiuTongGuBen, '万股'],
    ['总资产', finance.ZongZiChan, '万元'],
    ['净资产', finance.JingZiChan, '万元'],
    ['主营收入', finance.ZhuYingShouRu, '万元'],
    ['净利润', finance.JingLiRun, '万元'],
    ['每股净资产', finance.MeiGuJingZiChan, '元'],
    ['股东人数', finance.GuDongRenShu, '人'],
  ] : [];

  return {
    finance,
    financeTrends,
    financeMetrics,
    financeTrendMode,
    setFinanceTrendMode,
    financeCompareMode,
    setFinanceCompareMode,
    financeViewMode,
    setFinanceViewMode,
    selectedFinanceMetrics,
    setSelectedFinanceMetrics,
    financeTrendLoading,
    availableFinanceMetrics,
    financeChartGroups,
    financeDisplayRecords,
    activeFinanceMetrics,
    latestFinanceRecord,
    formatFinanceMetricValue,
    financeItems,
  };
}
