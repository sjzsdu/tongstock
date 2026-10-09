import { useCallback, useEffect, useState } from 'react';
import { message } from 'antd';
import {
  api,
  TongStockAPIError,
  type MethodResearchResult,
  type MethodResearchStatus,
} from '../../api/client';

export function useMethodDiscovery(onCompleted: () => Promise<void>) {
  const [status, setStatus] = useState<MethodResearchStatus>();
  const [result, setResult] = useState<MethodResearchResult>();
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState('');

  const refresh = useCallback(async () => {
    const [statusResponse, lastResponse] = await Promise.allSettled([
      api.methodResearchStatus(),
      api.methodResearchLast(),
    ]);
    if (statusResponse.status === 'fulfilled') setStatus(statusResponse.value);
    if (lastResponse.status === 'fulfilled' && lastResponse.value.status !== 'no_completed_batch') {
      setResult(lastResponse.value);
    }
  }, []);

  useEffect(() => {
    queueMicrotask(() => void refresh());
  }, [refresh]);

  useEffect(() => {
    if (!status?.running) return;
    const timer = window.setInterval(async () => {
      try {
        const latest = await api.methodResearchStatus();
        setStatus(latest);
        if (!latest.running) {
          await Promise.all([onCompleted(), refresh()]);
          void message.success('新方法挖掘已完成，结果已更新');
        }
      } catch {
        // 轮询失败不覆盖主页数据，下一轮继续恢复。
      }
    }, 10_000);
    return () => window.clearInterval(timer);
  }, [status?.running, onCompleted, refresh]);

  const start = useCallback(async () => {
    setStarting(true);
    setError('');
    try {
      await api.methodResearchRun({});
      setStatus((previous) => ({ ...(previous ?? {}), running: true }));
      void message.info('已开始挖掘并验证新方法，可以先离开本页');
      await refresh();
    } catch (cause) {
      if (cause instanceof TongStockAPIError && cause.code === 'method_research_busy') {
        setStatus((previous) => ({ ...(previous ?? {}), running: true }));
        void message.info('已有一轮方法挖掘在进行');
      } else {
        setError(cause instanceof Error ? cause.message : '新方法挖掘启动失败');
      }
    } finally {
      setStarting(false);
    }
  }, [refresh]);

  return { status, result, starting, error, start };
}
