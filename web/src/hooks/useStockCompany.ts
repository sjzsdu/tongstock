import { useState, useEffect } from 'react';
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

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    setCompanyLoading(true);
    api.company(code).then((cats) => {
      setCompanyCats(cats);
      if (cats.length > 0 && !selectedCat) void loadCompanyContent(cats[0]);
    }).catch(() => {}).finally(() => setCompanyLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [code, detailStatus]);

  const loadCompanyContent = async (cat: string | CompanyCategory) => {
    const catName = typeof cat === 'string' ? cat : cat.Name;
    setSelectedCat(catName);
    setCompanyContent('');
    try {
      const r = await api.companyContent(code, cat);
      setCompanyContent((r.content || '').replace(/\r/g, ''));
    } catch {
      setCompanyContent('加载失败');
    }
  };

  return {
    companyCats,
    companyContent,
    selectedCat,
    companyLoading,
    loadCompanyContent,
  };
}
