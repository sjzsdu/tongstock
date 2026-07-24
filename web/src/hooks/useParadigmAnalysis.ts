import { useState, useCallback } from 'react';
import { api } from '../api/client';
import type { ParadigmItem } from '../types/api';

export interface UseParadigmAnalysisReturn {
  paradigmResult: ParadigmItem | null;
  paradigmLoading: boolean;
  paradigmAgentText: string;
  paradigmEvalConfirm: any[];
  paradigmEvalInvalid: any[];
  paradigmDrawerOpen: boolean;
  setParadigmDrawerOpen: (open: boolean) => void;
  analyzeParadigm: (code: string, name?: string, bypassCache?: boolean) => Promise<void>;
}

export function useParadigmAnalysis(): UseParadigmAnalysisReturn {
  const [paradigmResult, setParadigmResult] = useState<ParadigmItem | null>(null);
  const [paradigmLoading, setParadigmLoading] = useState(false);
  const [paradigmAgentText, setParadigmAgentText] = useState('');
  const [paradigmEvalConfirm, setParadigmEvalConfirm] = useState<any[]>([]);
  const [paradigmEvalInvalid, setParadigmEvalInvalid] = useState<any[]>([]);
  const [paradigmDrawerOpen, setParadigmDrawerOpen] = useState(false);

  const analyzeParadigm = useCallback(async (code: string, name?: string, bypassCache?: boolean) => {
    setParadigmLoading(true);
    setParadigmResult(null);
    setParadigmAgentText(bypassCache ? '正在绕过缓存重新分析...' : '');
    try {
      const result = await api.paradigmAnalyze(code, name, undefined, bypassCache);
      if (result.error) {
        setParadigmAgentText(result.error);
      } else {
        setParadigmResult(result.paradigm || null);
        setParadigmEvalConfirm(result.evaluated_confirm || []);
        setParadigmEvalInvalid(result.evaluated_invalid || []);
        setParadigmAgentText(result.agent_text || '');
      }
    } catch (err) {
      setParadigmAgentText(String(err));
    } finally {
      setParadigmLoading(false);
    }
  }, []);

  return {
    paradigmResult,
    paradigmLoading,
    paradigmAgentText,
    paradigmEvalConfirm,
    paradigmEvalInvalid,
    paradigmDrawerOpen,
    setParadigmDrawerOpen,
    analyzeParadigm,
  };
}
