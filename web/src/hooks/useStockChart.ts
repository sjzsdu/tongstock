import { useCallback, useState, useEffect, useMemo } from 'react';
import { api } from '../api/client';
import type { Signal, SignalAnalysis as SignalAnalysisType, SignalOutcome, StateSeriesResponse } from '../types/api';
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
  /** 逐日四维状态时序（状态型模型，后端现算） */
  stateSeries: StateSeriesResponse | null;
  /** 状态带加载/失败提示：首次拉全历史可能较慢、或超时后需先同步 */
  stateHint: React.ReactNode;
  reloadStateSeries: () => void;
  mainOverlay: string;
  setMainOverlay: (overlay: string) => void;
  subPanel: string;
  setSubPanel: (panel: string) => void;
}

// 状态序列失败时给可操作的中文提示：超时提示先同步，无数据提示同步，
// 其余按接口返回的错误文案显示（后端已带 code/message）。
function stateSeriesErrorMessage(err: unknown): string {
  if (err instanceof Error) {
    if (/timeout|超时/i.test(err.message)) {
      return `状态带计算超时：${err.message}`;
    }
    return `状态带暂不可用：${err.message}`;
  }
  return '状态带暂不可用，请稍后重试。';
}

export function useStockChart(
  code: string,
  ktype: string,
  detailStatus: DetailStatus,
  onDataLoaded?: () => void,
  stateSeriesEnabled = true,
): UseStockChartReturn {
  const [klines, setKlines] = useState<any[]>([]);
  const [indicator, setIndicator] = useState<any>(null);
  const [chartLoading, setChartLoading] = useState(false);
  const [analysis, setAnalysis] = useState<SignalAnalysisType | null>(null);
  const [mainOverlay, setMainOverlay] = useState('MA');
  const [subPanel, setSubPanel] = useState('MACD');
  const [stateSeries, setStateSeries] = useState<StateSeriesResponse | null>(null);
  const [stateError, setStateError] = useState<string | null>(null);
  const [stateRetry, setStateRetry] = useState(0);

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    let cancelled = false;
    setChartLoading(true);
    // stale-while-revalidate：换股时保留旧 klines/indicator/analysis 继续渲染，
    // 新数据到齐后一次性替换，避免整页 Spin（详见 useStockDetail 同口径注释）。

    const loadChart = async () => {
      const [indicatorResult, analysisResult] = await Promise.allSettled([
        api.indicator(code, ktype),
        api.signalAnalysis(code, ktype),
      ]);

      if (cancelled) return;
      onDataLoaded?.();

      if (indicatorResult.status === 'rejected') {
        // 拉取失败：清空旧股票数据。stale-while-revalidate 只覆盖成功路径，
        // 若保留上一只股票的 klines/indicator，会在新股票页被当作当前数据
        // 无限期展示（换 ktype 或重进页面才消失）。
        setIndicator(null);
        setKlines([]);
        setAnalysis(null);
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

      // analysis 失败时同步清空：新 K 线不能配上一只股票的信号
      setAnalysis(analysisResult.status === 'fulfilled' ? analysisResult.value : null);
      setChartLoading(false);
    };

    loadChart().catch(() => {
      if (cancelled) return;
      setChartLoading(false);
    });

    return () => {
      cancelled = true;
    };
  }, [code, ktype, detailStatus, onDataLoaded]);

  // 状态序列单独一路请求：它可能因为本地无水位而拉全历史（较慢），
  // 不能和 K 线/信号挤在同一个 Promise.all 里，否则整页等它。
  // 注意 setState 只出现在 promise 回调里（不在 effect 同步段），保持 lint 基线。
  useEffect(() => {
    if (!code || detailStatus !== 'ready' || !stateSeriesEnabled) return;
    let cancelled = false;
    api.stockStateSeries(code, ktype)
      .then((res) => {
        if (cancelled) return;
        setStateSeries(res);
        setStateError(null);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setStateSeries(null);
        setStateError(stateSeriesErrorMessage(err));
      });
    return () => {
      cancelled = true;
    };
  }, [code, ktype, detailStatus, stateSeriesEnabled, stateRetry]);

  const stateHint = useMemo(() => {
    if (stateError) return stateError;
    if (stateSeries) return null;
    // 没有数据也没有错误 = 正在算：说明首次可能较慢，避免用户以为页面卡住。
    return '状态带计算中…首次拉取该股全历史日线可能稍慢，超时会提示先同步。';
  }, [stateError, stateSeries]);

  const reloadStateSeries = useCallback(() => setStateRetry((n) => n + 1), []);

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
    stateSeries,
    stateHint,
    reloadStateSeries,
    mainOverlay,
    setMainOverlay,
    subPanel,
    setSubPanel,
  };
}
