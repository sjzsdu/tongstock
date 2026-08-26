import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { SyncFreshnessResult } from '../types/api';

/**
 * Product-level status model.
 *
 * 工程概念（Agent / Model / Session / 内部文件名）不应直接暴露给用户。
 * 所有页面统一使用以下 5 类产品状态：
 *
 *  - loading     正在获取数据（首次加载或主动刷新中）
 *  - ready       数据新鲜可用
 *  - degraded    数据可用但已陈旧（降级显示，提示刷新）
 *  - failed      最近一次获取失败，但有陈旧数据可继续使用
 *  - unavailable 完全不可用（无数据 / 服务不可达 / 功能未就绪）
 */

export type ProductStatusKind =
  | 'loading'
  | 'ready'
  | 'degraded'
  | 'failed'
  | 'unavailable';

export interface ProductStatusMeta {
  /** 人类可读的状态描述（中文） */
  label: string;
  /** 建议用户执行的恢复动作（中文），空字符串表示无需动作 */
  actionHint: string;
  /** 严重级别，用于选择颜色 */
  level: 'info' | 'success' | 'warning' | 'error';
  /** 最近一次数据时间（可选） */
  lastUpdated?: string;
  /** 原始原因说明（可选，将被折叠到 tooltip 中） */
  reason?: string;
}

export interface ProductStatusState {
  kind: ProductStatusKind;
  meta: ProductStatusMeta;
  /** 当前是否允许继续操作（基于陈旧数据或降级模式） */
  canOperate: boolean;
}

const META_TEMPLATES: Record<ProductStatusKind, ProductStatusMeta> = {
  loading: {
    label: '正在加载',
    actionHint: '',
    level: 'info',
  },
  ready: {
    label: '数据新鲜',
    actionHint: '',
    level: 'success',
  },
  degraded: {
    label: '数据延迟',
    actionHint: '点击刷新以获取最新数据',
    level: 'warning',
  },
  failed: {
    label: '连接异常',
    actionHint: '点击重试；若持续异常请检查网络或后端服务',
    level: 'error',
  },
  unavailable: {
    label: '暂不可用',
    actionHint: '请稍后重试，或前往配置页面检查数据源',
    level: 'error',
  },
};

/** 将后端 SyncFreshnessResult 映射为产品状态 */
export function mapFreshnessToStatus(
  result?: SyncFreshnessResult | null,
): ProductStatusKind {
  if (!result) return 'unavailable';
  switch (result.freshness) {
    case 'fresh':
      return 'ready';
    case 'stale':
    case 'outdated':
      return 'degraded';
    case 'failed':
      return 'failed';
    case 'empty':
    case 'unknown':
    default:
      return 'unavailable';
  }
}

function enrichMeta(kind: ProductStatusKind, base?: Partial<ProductStatusMeta>): ProductStatusMeta {
  const tpl = META_TEMPLATES[kind];
  return {
    ...tpl,
    ...(base ?? {}),
  };
}

/**
 * 通用产品状态 Hook。
 *
 * 使用方式：
 *   const { state, load, refresh, markUnavailable } = useProductStatus();
 *
 *   // 首次加载
 *   load(async () => api.fetchSomething());
 *
 *   // 处理后端 freshness 数据
 *   const mapped = useProductStatus.fromFreshness(freshnessResult);
 */
export function useProductStatus<T = unknown>() {
  const [kind, setKind] = useState<ProductStatusKind>('loading');
  const [meta, setMeta] = useState<ProductStatusMeta>(META_TEMPLATES.loading);
  const [data, setData] = useState<T | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const load = useCallback(
    async (fn: () => Promise<T>, opts?: { onUnavailable?: () => ProductStatusMeta }) => {
      setKind('loading');
      setMeta(META_TEMPLATES.loading);
      try {
        const result = await fn();
        if (!mountedRef.current) return result;
        setData(result);
        setKind('ready');
        setMeta(META_TEMPLATES.ready);
        return result;
      } catch (err) {
        if (!mountedRef.current) return null;
        setKind('unavailable');
        setMeta(
          opts?.onUnavailable?.() ??
            enrichMeta('unavailable', {
              reason: err instanceof Error ? err.message : String(err),
            }),
        );
        return null;
      }
    },
    [],
  );

  /** 根据后端返回的 freshness 结果，更新状态（可附带最近一次失败信息） */
  const applyFreshness = useCallback(
    (result?: SyncFreshnessResult | null, failureError?: string) => {
      if (failureError) {
        setKind('failed');
        setMeta(
          enrichMeta('failed', {
            reason: failureError,
          }),
        );
        return;
      }
      const next = mapFreshnessToStatus(result);
      setKind(next);
      setMeta(
        enrichMeta(next, {
          lastUpdated: result?.last_date || result?.last_sync_at,
          reason: result?.stale_reason || result?.error,
        }),
      );
    },
    [],
  );

  const markUnavailable = useCallback(
    (reason?: string) => {
      setKind('unavailable');
      setMeta(enrichMeta('unavailable', { reason }));
    },
    [],
  );

  const markReady = useCallback(
    (lastUpdated?: string) => {
      setKind('ready');
      setMeta(enrichMeta('ready', { lastUpdated }));
    },
    [],
  );

  const state = useMemo<ProductStatusState>(
    () => ({
      kind,
      meta,
      canOperate: kind === 'ready' || kind === 'degraded' || kind === 'failed',
    }),
    [kind, meta],
  );

  return {
    state,
    data,
    load,
    applyFreshness,
    markUnavailable,
    markReady,
    /** 便捷：将 freshness 结果转换为产品状态 */
    fromFreshness: (result?: SyncFreshnessResult | null): ProductStatusState => ({
      kind: mapFreshnessToStatus(result),
      meta: enrichMeta(mapFreshnessToStatus(result), {
        lastUpdated: result?.last_date || result?.last_sync_at,
        reason: result?.stale_reason || result?.error,
      }),
      canOperate:
        mapFreshnessToStatus(result) !== 'unavailable' &&
        mapFreshnessToStatus(result) !== 'loading',
    }),
  };
}
