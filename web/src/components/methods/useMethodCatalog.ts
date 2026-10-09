import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { message } from 'antd';
import { api, type MethodAuditEvent, type MethodCard } from '../../api/client';

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
    setDetailError('');
    setDetailLoading(true);
    const [cardResult, auditResult] = await Promise.allSettled([
      api.methodCard(method.id),
      api.methodAudit(method.id),
    ]);
    if (cardResult.status === 'fulfilled') setDetail(cardResult.value);
    if (auditResult.status === 'fulfilled') setAudit(auditResult.value.items ?? []);
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
    drawerOpen, setDrawerOpen, detail, audit, detailLoading, detailError,
    load, screenMethod, loadValidation, seedMethods,
  };
}
