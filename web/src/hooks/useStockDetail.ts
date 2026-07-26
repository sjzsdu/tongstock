import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { api } from '../api/client';
import type { KlineSyncState } from '../types/api';

export type DetailStatus = 'loading' | 'ready' | 'not_found' | 'no_data';

function getDetailErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

function classifyDetailStatus(error: unknown): DetailStatus {
  const message = error instanceof Error ? error.message : '';
  if (message.includes('未找到') || message.includes('多个匹配股票')) return 'not_found';
  return 'no_data';
}

export interface UseStockDetailReturn {
  code: string;
  quote: any;
  loading: boolean;
  detailStatus: DetailStatus;
  detailError: string;
  syncState: KlineSyncState | null;
  ktype: string;
  setKtype: (ktype: string) => void;
}

export function useStockDetail(): UseStockDetailReturn {
  const { code: paramCode } = useParams();
  const navigate = useNavigate();
  const [code, setCode] = useState(paramCode || '');
  const [quote, setQuote] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [detailStatus, setDetailStatus] = useState<DetailStatus>('loading');
  const [detailError, setDetailError] = useState('');
  const [syncState, setSyncState] = useState<KlineSyncState | null>(null);
  const [ktype, setKtype] = useState('day');

  useEffect(() => {
    if (!paramCode) {
      navigate('/stock/choose');
      return;
    }
    setCode(paramCode);
  }, [paramCode, navigate]);

  useEffect(() => {
    if (!code) return;
    let cancelled = false;
    setLoading(true);
    setDetailStatus('loading');
    setDetailError('');
    setQuote(null);

    // Load sync state (non-blocking, best-effort)
    api.getSyncState(code, ktype).then((state) => {
      if (!cancelled) setSyncState(state);
    }).catch(() => {});

    const loadQuote = async () => {
      try {
        const quoteResult = await api.quote(code);
        if (cancelled) return;
        setQuote(quoteResult);
        api.historyAdd(code, quoteResult.Name).catch(() => {});
        setDetailStatus('ready');
        setLoading(false);
      } catch (error) {
        if (cancelled) return;
        setDetailStatus('not_found');
        setDetailError(getDetailErrorMessage(error, '未找到匹配股票或行情数据'));
        setLoading(false);
        return;
      }
    };

    loadQuote().catch((error) => {
      if (cancelled) return;
      setDetailStatus(classifyDetailStatus(error));
      setDetailError(getDetailErrorMessage(error, '加载股票详情失败'));
      setLoading(false);
    });

    return () => {
      cancelled = true;
    };
  }, [code, ktype]);

  return {
    code,
    quote,
    loading,
    detailStatus,
    detailError,
    syncState,
    ktype,
    setKtype,
  };
}
