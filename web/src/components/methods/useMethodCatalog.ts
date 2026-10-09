import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { message } from 'antd';
import { api, type MethodAuditEvent, type MethodCard, type MethodResearchTraceSummary } from '../../api/client';
import { methodCanScreen, methodUnavailableReason } from '../../lib/methodPresentation';

const DEFAULT_STATUSES = 'verified,observing,degraded,candidate';

export function useMethodCatalog() {
  const navigate = useNavigate();
  const [items, setItems] = useState<MethodCard[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [screeningId, setScreeningId] = useState('');
  const [seeding, setSeeding] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [detail, setDetail] = useState<MethodCard>();
  const [audit, setAudit] = useState<MethodAuditEvent[]>();
  const [researchTrace, setResearchTrace] = useState<MethodResearchTraceSummary>();
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const response = await api.methodCards({ status: DEFAULT_STATUSES, limit: 100 });
      setItems(response.items ?? []);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '读取选股方法失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    queueMicrotask(() => void load());
  }, [load]);

  const screenMethod = useCallback(async (method: MethodCard) => {
    // Pre-check before navigating: never land the user on a result page that
    // is guaranteed to be empty (method not eligible / rule unsupported).
    if (!methodCanScreen(method)) {
      void message.warning(methodUnavailableReason(method) ?? '该方法当前不能用于今日筛选');
      return;
    }
    setScreeningId(method.id);
    try {
      const run = await api.selectionRunCreate({ method_ids: [method.id] });
      navigate(`/methods/selection/${run.id}`, { state: { run } });
    } catch (cause) {
      void message.error(cause instanceof Error ? cause.message : '今日股票筛选失败');
    } finally {
      setScreeningId('');
    }
  }, [navigate]);

  const loadValidation = useCallback(async (method: MethodCard) => {
    setDrawerOpen(true);
    setDetail(method);
    setAudit(undefined);
    setResearchTrace(undefined);
    setDetailError('');
    setDetailLoading(true);
    // 验证窗口/股票池大小只存研究批次记录；方法卡带 source_research_id 时
    // 按轨迹摘要补齐（老方法 evidence 里没存这些字段）。
    const [cardResult, auditResult] = await Promise.allSettled([
      api.methodCard(method.id),
      api.methodAudit(method.id),
    ]);
    let card = method;
    if (cardResult.status === 'fulfilled') {
      card = cardResult.value;
      setDetail(card);
    }
    if (auditResult.status === 'fulfilled') setAudit(auditResult.value.items ?? []);
    if (card.source_research_id) {
      try {
        setResearchTrace(await api.methodResearchTraceSummary(card.source_research_id));
      } catch {
        // 没有轨迹时保持 undefined，抽屉会如实显示「未记录」。
      }
    }
    const failures = [cardResult, auditResult]
      .filter((result): result is PromiseRejectedResult => result.status === 'rejected')
      .map((result) => result.reason instanceof Error ? result.reason.message : '读取失败');
    setDetailError(failures.join('；'));
    setDetailLoading(false);
  }, []);

  const seedMethods = useCallback(async () => {
    setSeeding(true);
    try {
      const result = await api.seedMethods({});
      if (result.verified > 0) void message.success(`已加入 ${result.verified} 个通过历史验证的入门方法`);
      else void message.info('入门方法已验证，但本轮没有方法达到选股门槛');
      await load();
    } catch (cause) {
      void message.error(cause instanceof Error ? cause.message : '加载入门方法失败');
    } finally {
      setSeeding(false);
    }
  }, [load]);

  return {
    items, loading, error, screeningId, seeding,
    drawerOpen, setDrawerOpen, detail, audit, researchTrace, detailLoading, detailError,
    load, screenMethod, loadValidation, seedMethods,
  };
}
