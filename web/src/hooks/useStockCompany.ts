import { useState, useEffect, useCallback, useRef } from 'react';
import { api } from '../api/client';
import type { CompanyCategory } from '../types/api';
import type { DetailStatus } from './useStockDetail';

export interface UseStockCompanyReturn {
  companyCats: CompanyCategory[];
  companyContent: string;
  selectedCat: string;
  companyLoading: boolean;
  loadCompanyContent: (cat: string | CompanyCategory) => Promise<void>;
}

export function useStockCompany(code: string, detailStatus: DetailStatus): UseStockCompanyReturn {
  const [companyCats, setCompanyCats] = useState<CompanyCategory[]>([]);
  const [companyContent, setCompanyContent] = useState('');
  const [selectedCat, setSelectedCat] = useState('');
  const [companyLoading, setCompanyLoading] = useState(false);
  // 每只股票只自动加载第一个目录一次，避免覆盖用户后续选择
  const autoLoadedFor = useRef('');

  const loadCompanyContent = useCallback(async (cat: string | CompanyCategory) => {
    const catName = typeof cat === 'string' ? cat : cat.Name;
    setSelectedCat(catName);
    setCompanyContent('');
    try {
      const r = await api.companyContent(code, cat);
      setCompanyContent((r.content || '').replace(/\r/g, ''));
    } catch {
      setCompanyContent('加载失败');
    }
  }, [code]);

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    let cancelled = false;
    setCompanyLoading(true);
    api.company(code).then((cats) => {
      if (cancelled) return;
      setCompanyCats(cats);
      if (cats.length > 0 && autoLoadedFor.current !== code) {
        autoLoadedFor.current = code;
        void loadCompanyContent(cats[0]);
      }
    }).catch(() => {}).finally(() => {
      if (!cancelled) setCompanyLoading(false);
    });
    return () => { cancelled = true; };
  }, [code, detailStatus, loadCompanyContent]);

  return {
    companyCats,
    companyContent,
    selectedCat,
    companyLoading,
    loadCompanyContent,
  };
}
