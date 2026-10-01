import { useState, useEffect } from 'react';
import { api } from '../api/client';
import type { StockCompareResponse } from '../types/api';
import type { DetailStatus } from './useStockDetail';

export interface UseStockCompareReturn {
  compareData: StockCompareResponse | null;
  compareLoading: boolean;
}

// 对比接口较重（后端要拉全板块行情），只在对比 tab 激活时才拉取。
export function useStockCompare(code: string, detailStatus: DetailStatus, enabled: boolean): UseStockCompareReturn {
  const [compareData, setCompareData] = useState<StockCompareResponse | null>(null);
  const [compareLoading, setCompareLoading] = useState(false);

  useEffect(() => {
    if (!enabled || !code || detailStatus !== 'ready') return;
    let cancelled = false;
    setCompareLoading(true);
    api.stockCompare(code).then((d) => {
      if (cancelled) return;
      setCompareData(d);
      setCompareLoading(false);
    }).catch(() => {
      if (cancelled) return;
      setCompareLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, [code, detailStatus, enabled]);

  return {
    compareData,
    compareLoading,
  };
}
