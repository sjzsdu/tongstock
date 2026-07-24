import { useState, useEffect } from 'react';
import { api } from '../api/client';
import type { StockCompareResponse } from '../types/api';
import type { DetailStatus } from './useStockDetail';

export interface UseStockCompareReturn {
  compareData: StockCompareResponse | null;
  compareLoading: boolean;
}

export function useStockCompare(code: string, detailStatus: DetailStatus): UseStockCompareReturn {
  const [compareData, setCompareData] = useState<StockCompareResponse | null>(null);
  const [compareLoading, setCompareLoading] = useState(false);

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    setCompareLoading(true);
    api.stockCompare(code).then((d) => {
      setCompareData(d);
      setCompareLoading(false);
    }).catch(() => {
      setCompareLoading(false);
    });
  }, [code, detailStatus]);

  return {
    compareData,
    compareLoading,
  };
}
