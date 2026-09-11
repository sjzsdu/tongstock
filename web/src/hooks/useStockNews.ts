import { useCallback, useEffect, useState } from 'react';
import { api } from '../api/client';
import type { StockNewsResult } from '../types/api';

/**
 * 个股资讯拉取状态。
 *
 * 用 idle/loading/done 三态而不是「loading 布尔 + 数据」：useEffect 在绘制之后
 * 才执行，若在 effect 内同步置 loading，首帧会先渲染出「暂无资讯」再翻转成
 * 加载中，产生闪烁。idle 与 loading 都渲染加载态即可避免。
 *
 * 同理，开启加载的动作放在微任务里：在 effect 体内同步 setState 会触发级联
 * 渲染，也被 react-hooks/set-state-in-effect 规则禁止。
 */
type NewsState =
  | { status: 'idle'; data: null }
  | { status: 'loading'; data: StockNewsResult | null }
  | { status: 'done'; data: StockNewsResult | null };

export interface UseStockNewsReturn {
  news: StockNewsResult | null;
  loading: boolean;
  reload: () => void;
}

export function useStockNews(code: string | undefined, enabled: boolean): UseStockNewsReturn {
  const [state, setState] = useState<NewsState>({ status: 'idle', data: null });
  const [nonce, setNonce] = useState(0);

  const reload = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    if (!enabled || !code) return;
    let cancelled = false;
    Promise.resolve().then(() => {
      if (!cancelled) setState((prev) => ({ status: 'loading', data: prev.data }));
    });
    api
      .newsStock(code, { limit: 50 })
      .then((result) => {
        if (!cancelled) setState({ status: 'done', data: result });
      })
      .catch(() => {
        if (!cancelled) setState({ status: 'done', data: null });
      });
    // 切换股票或重复请求时丢弃过期响应，避免慢请求覆盖新结果。
    return () => {
      cancelled = true;
    };
  }, [code, enabled, nonce]);

  return {
    news: state.data,
    loading: state.status !== 'done',
    reload,
  };
}
