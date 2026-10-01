import { useState, useCallback, useRef } from 'react';
import { api, TongStockAPIError } from '../api/client';
import type { EvaluatedItem, ParadigmItem, ParadigmAnalyzeResponse } from '../types/api';

export interface UseParadigmAnalysisReturn {
  paradigmResult: ParadigmItem | null;
  paradigmLoading: boolean;
  /** 该股票是否已有范式缓存。挖掘成功即视为有缓存。 */
  paradigmCached: boolean;
  paradigmAgentText: string;
  paradigmEvalConfirm: EvaluatedItem[];
  paradigmEvalInvalid: EvaluatedItem[];
  paradigmDrawerOpen: boolean;
  setParadigmDrawerOpen: (open: boolean) => void;
  analyzeParadigm: (code: string, name?: string, bypassCache?: boolean) => Promise<void>;
}

// 强制重新挖掘走完整的 AI 生成 + 实验验证链路，可能远超普通请求耗时
const REFRESH_TIMEOUT_MS = 120_000;

/** 从 422 等错误响应体中恢复可展示的部分结果（后端在该场景仍返回 Paradigm/AgentText） */
function payloadFromError(err: unknown): Partial<ParadigmAnalyzeResponse> | null {
  if (err instanceof TongStockAPIError && err.payload && typeof err.payload === 'object') {
    return err.payload as Partial<ParadigmAnalyzeResponse>;
  }
  return null;
}

export function useParadigmAnalysis(): UseParadigmAnalysisReturn {
  const [paradigmResult, setParadigmResult] = useState<ParadigmItem | null>(null);
  const [paradigmCached, setParadigmCached] = useState(false);
  const [paradigmLoading, setParadigmLoading] = useState(false);
  const [paradigmAgentText, setParadigmAgentText] = useState('');
  const [paradigmEvalConfirm, setParadigmEvalConfirm] = useState<EvaluatedItem[]>([]);
  const [paradigmEvalInvalid, setParadigmEvalInvalid] = useState<EvaluatedItem[]>([]);
  const [paradigmDrawerOpen, setParadigmDrawerOpen] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const analyzeParadigm = useCallback(async (code: string, name?: string, bypassCache?: boolean) => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    const timeoutId = bypassCache
      ? setTimeout(() => controller.abort(), REFRESH_TIMEOUT_MS)
      : undefined;

    setParadigmLoading(true);
    setParadigmResult(null);
    setParadigmEvalConfirm([]);
    setParadigmEvalInvalid([]);
    setParadigmAgentText(bypassCache
      ? '正在绕过缓存重新分析（AI 生成与验证可能需要 1-2 分钟）...'
      : '');

    try {
      const result = await api.paradigmAnalyze(code, name, undefined, bypassCache, controller.signal);
      if (result.error) {
        setParadigmAgentText(result.error);
        if (result.paradigm) {
          setParadigmResult(result.paradigm);
          setParadigmCached(true);
        }
        if (result.agent_text) setParadigmAgentText((prev) => prev || result.agent_text);
      } else {
        setParadigmResult(result.paradigm || null);
        if (result.paradigm) setParadigmCached(true);
        setParadigmEvalConfirm(result.evaluated_confirm || []);
        setParadigmEvalInvalid(result.evaluated_invalid || []);
        setParadigmAgentText(result.agent_text || '');
      }
    } catch (err) {
      // 422 响应体里可能带着部分结果：保留 Paradigm/AgentText 展示而不是整包丢弃
      const payload = payloadFromError(err);
      if (payload?.paradigm) {
        setParadigmResult(payload.paradigm);
        setParadigmCached(true);
        setParadigmEvalConfirm(payload.evaluated_confirm || []);
        setParadigmEvalInvalid(payload.evaluated_invalid || []);
      }
      if (payload?.agent_text) {
        setParadigmAgentText(payload.agent_text);
      }
      if (!payload?.paradigm && !payload?.agent_text) {
        if (err instanceof DOMException && err.name === 'AbortError') {
          setParadigmAgentText(
            `范式挖掘超时（超过 ${Math.round(REFRESH_TIMEOUT_MS / 1000)} 秒）已取消。` +
            '可关闭抽屉稍后再试，或点击"重新挖掘"重试。',
          );
        } else {
          setParadigmAgentText(err instanceof TongStockAPIError
            ? `范式挖掘失败：${err.message}`
            : String(err));
        }
      }
    } finally {
      if (timeoutId) clearTimeout(timeoutId);
      if (abortRef.current === controller) abortRef.current = null;
      setParadigmLoading(false);
    }
  }, []);

  return {
    paradigmResult,
    paradigmLoading,
    paradigmCached,
    paradigmAgentText,
    paradigmEvalConfirm,
    paradigmEvalInvalid,
    paradigmDrawerOpen,
    setParadigmDrawerOpen,
    analyzeParadigm,
  };
}
