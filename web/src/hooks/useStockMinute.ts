import { useState, useEffect, useCallback, useRef } from 'react';
import { api } from '../api/client';
import type { MinuteItem } from '../types/api';
import type { DetailStatus } from './useStockDetail';
import { formatAPIDate, formatShortDate, recentWeekdayDates } from '../lib/datetime';

export interface UseStockMinuteReturn {
  minuteData: MinuteItem[];
  minuteDate: string;
  minuteLoading: boolean;
  minuteError: string;
  highlightedIdx: number;
  setHighlightedIdx: (idx: number) => void;
}

export function useStockMinute(code: string, detailStatus: DetailStatus, enabled: boolean): UseStockMinuteReturn {
  const [minuteData, setMinuteData] = useState<MinuteItem[]>([]);
  const [minuteDate, setMinuteDate] = useState<string>('');
  const [minuteLoading, setMinuteLoading] = useState(false);
  const [minuteError, setMinuteError] = useState('');
  const [highlightedIdx, setHighlightedIdx] = useState(-1);
  // 记住最近一次命中的历史分时日期：非交易时段/长假里每 30s 的轮询
  // 直接打这个日期，不用每次都从头逐日探测（停牌股会扫满全部候选）。
  const lastHistoryDateRef = useRef<{ code: string; date: Date } | null>(null);

  const fetchMinute = useCallback(async () => {
    setMinuteLoading(true);
    setMinuteError('');
    let loaded = false;
    try {
      const r = await api.minute(code);
      if (r.List && r.List.length > 0) {
        setMinuteData(r.List);
        setMinuteDate(formatShortDate(new Date()));
        loaded = true;
      }
    } catch {
      // 实时分时不可用时走历史回退
    }

    if (!loaded) {
      // 回退到最近一个交易日的分时。只按星期回退一天会在长假（国庆/春节）
      // 全部落到休市日上拿不到数据，因此按候选日逐日探测，取到即止；
      // 候选日只排除了周末，法定节假日靠"查无数据"自然跳过。
      const cachedDate = lastHistoryDateRef.current?.code === code ? lastHistoryDateRef.current.date : null;
      const seen = new Set<string>();
      const candidates: Date[] = [];
      for (const date of [cachedDate, ...recentWeekdayDates()]) {
        if (!date) continue;
        const key = formatAPIDate(date);
        if (seen.has(key)) continue;
        seen.add(key);
        candidates.push(date);
      }
      for (const date of candidates) {
        try {
          const histR = await api.minuteHistory(code, formatAPIDate(date));
          if (histR.List && histR.List.length > 0) {
            setMinuteData(histR.List);
            setMinuteDate(formatShortDate(date));
            lastHistoryDateRef.current = { code, date };
            loaded = true;
            break;
          }
        } catch {
          // 该日无数据或请求失败，继续前一个候选
        }
      }
    }

    if (!loaded) {
      setMinuteData([]);
      setMinuteDate('');
      setMinuteError('暂无可展示的分时数据');
      if (lastHistoryDateRef.current?.code === code) lastHistoryDateRef.current = null;
    }
    api.quote(code).then(() => {}).catch(() => {});
    setMinuteLoading(false);
  }, [code]);

  // 仅在分时 tab 激活时拉取并轮询，避免换股时后台压多只股票的分时请求。
  useEffect(() => {
    if (!enabled || !code || detailStatus !== 'ready') return;
    void fetchMinute();
    const timer = setInterval(fetchMinute, 30000);
    return () => clearInterval(timer);
  }, [code, detailStatus, enabled, fetchMinute]);

  return {
    minuteData,
    minuteDate,
    minuteLoading,
    minuteError,
    highlightedIdx,
    setHighlightedIdx,
  };
}
