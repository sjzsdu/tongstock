import { useState, useEffect, useMemo } from 'react';
import { api } from '../api/client';
import type { Signal, SignalAnalysis as SignalAnalysisType, SignalOutcome } from '../types/api';
import type { DetailStatus } from './useStockDetail';



function compareDateDesc(a: string | undefined, b: string | undefined): number {
  return String(b ?? '').localeCompare(String(a ?? ''));
}

export interface UseStockChartReturn {
  klines: any[];
  indicator: any;
  chartLoading: boolean;
  analysis: SignalAnalysisType | null;
  sortedSignals: Signal[];
  sortedSignalOutcomes: SignalOutcome[];
  latestClose: number | undefined;
  mainOverlay: string;
  setMainOverlay: (overlay: string) => void;
  subPanel: string;
  setSubPanel: (panel: string) => void;
}

export function useStockChart(code: string, ktype: string, detailStatus: DetailStatus): UseStockChartReturn {
  const [klines, setKlines] = useState<any[]>([]);
  const [indicator, setIndicator] = useState<any>(null);
  const [chartLoading, setChartLoading] = useState(false);
  const [analysis, setAnalysis] = useState<SignalAnalysisType | null>(null);
  const [mainOverlay, setMainOverlay] = useState('MA');
  const [subPanel, setSubPanel] = useState('MACD');

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    let cancelled = false;
    setChartLoading(true);
    setIndicator(null);
    setKlines([]);
    setAnalysis(null);

    const loadChart = async () => {
      const [indicatorResult, analysisResult] = await Promise.allSettled([
        api.indicator(code, ktype),
        api.signalAnalysis(code, ktype),
      ]);

      if (cancelled) return;

      if (indicatorResult.status === 'rejected') {
        setChartLoading(false);
        return;
      }

      const indicatorData = indicatorResult.value;
      const nextKlines = indicatorData?.klines || [];
      setIndicator(indicatorData);
      setKlines(nextKlines);

      if (nextKlines.length === 0) {
        setChartLoading(false);
        return;
      }

      if (analysisResult.status === 'fulfilled') setAnalysis(analysisResult.value);
      setChartLoading(false);
    };

    loadChart().catch(() => {
      if (cancelled) return;
      setChartLoading(false);
    });

    return () => {
      cancelled = true;
    };
  }, [code, ktype, detailStatus]);

  const sortedSignals = useMemo(
    () => [...(indicator?.signals ?? [])].sort((a, b) => compareDateDesc(a.Date, b.Date)),
    [indicator?.signals],
  );

  const sortedSignalOutcomes = useMemo<SignalOutcome[]>(
    () => [...(analysis?.outcomes ?? [])].sort((a, b) => compareDateDesc(a.date, b.date)),
    [analysis?.outcomes],
  );

  const latestClose = klines.length > 0 ? klines[klines.length - 1].Close : undefined;

  return {
    klines,
    indicator,
    chartLoading,
    analysis,
    sortedSignals,
    sortedSignalOutcomes,
    latestClose,
    mainOverlay,
    setMainOverlay,
    subPanel,
    setSubPanel,
  };
}
