import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';
import type { MinuteItem } from '../types/api';
import type { DetailStatus } from './useStockDetail';
import { formatShortDate } from '../lib/datetime';

function getLastTradingDay(): string {
  const today = new Date();
  const dayOfWeek = today.getDay();
  let adjustDays = 1;
  if (dayOfWeek === 0) adjustDays = 2;
  else if (dayOfWeek === 1) adjustDays = 3;

  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - adjustDays);

  const year = yesterday.getFullYear();
  const month = String(yesterday.getMonth() + 1).padStart(2, '0');
  const day = String(yesterday.getDate()).padStart(2, '0');
  return `${year}${month}${day}`;
}

export interface UseStockMinuteReturn {
  minuteData: MinuteItem[];
  minuteDate: string;
  minuteLoading: boolean;
  minuteError: string;
  highlightedIdx: number;
  setHighlightedIdx: (idx: number) => void;
}

export function useStockMinute(code: string, detailStatus: DetailStatus): UseStockMinuteReturn {
  const [minuteData, setMinuteData] = useState<MinuteItem[]>([]);
  const [minuteDate, setMinuteDate] = useState<string>('');
  const [minuteLoading, setMinuteLoading] = useState(false);
  const [minuteError, setMinuteError] = useState('');
  const [highlightedIdx, setHighlightedIdx] = useState(-1);

  const fetchMinute = useCallback(async () => {
    setMinuteLoading(true);
    setMinuteError('');
    let loaded = false;
    try {
      const r = await api.minute(code);
      if (r.List && r.List.length > 0) {
        setMinuteData(r.List);
        const today = new Date();
        setMinuteDate(formatShortDate(today));
        loaded = true;
      } else {
        setMinuteData([]);
      }
    } catch {
      // fall back to last trading day's minute data below
    }

    if (!loaded) {
      const yesterday = getLastTradingDay();
      try {
        const histR = await api.minuteHistory(code, yesterday);
        if (histR.List && histR.List.length > 0) {
          setMinuteData(histR.List);
          setMinuteDate(formatShortDate(yesterday));
          loaded = true;
        }
      } catch {
        // ignore historical minute errors
      }
    }

    if (!loaded) {
      setMinuteData([]);
      setMinuteDate('');
      setMinuteError('暂无可展示的分时数据');
    }
    api.quote(code).then(() => {}).catch(() => {});
    setMinuteLoading(false);
  }, [code]);

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    void fetchMinute();
    const timer = setInterval(fetchMinute, 30000);
    return () => clearInterval(timer);
  }, [code, detailStatus, fetchMinute]);

  return {
    minuteData,
    minuteDate,
    minuteLoading,
    minuteError,
    highlightedIdx,
    setHighlightedIdx,
  };
}
