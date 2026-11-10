import { useEffect, useMemo, useState } from 'react';
import { api } from '../../api/client';
import type { StateDim, StateSeriesResponse } from '../../types/api';
import { indexStateSeries, stateMetaMap } from './stateBands';

type ReasonMap = Partial<Record<StateDim, string>>;

interface StateBandOverlayProps {
  /** 股票代码与 K 线类型；为空时跳过「判定依据」的按需拉取（指数页不传）。 */
  code?: string;
  ktype?: string;
  series: StateSeriesResponse | null;
  /** 悬停到的日期（YYYY-MM-DD）；为空时展示最新一根 bar。 */
  hoverDate?: string;
  /** 加载中/失败提示：首次拉全历史较慢、或超时后需先同步。 */
  hint?: React.ReactNode;
}

function fmt(value: unknown): string {
  return typeof value === 'number' ? String(value) : '-';
}

/**
 * 状态带图层（图例与读数部分）：说明图上那 4 条色带是什么，并给出「当前/悬停这一天」
 * 的四维状态、中文文案与判定依据 —— 这正是「状态时序 × 股价」对照视图要交给用户
 * 肉眼确认的东西（今天处在什么位置、这个位置是怎么判出来的）。
 *
 * 所有文案/颜色都来自后端枚举（series.dimensions），前端只负责摆放；
 * 图上的色带本身由 stateBands.ts 的 addStateBands 挂到 K 线主图上。
 */
export default function StateBandOverlay({ code, ktype = 'day', series, hoverDate, hint }: StateBandOverlayProps) {
  const byDate = useMemo(() => indexStateSeries(series), [series]);
  const [reasonCache, setReasonCache] = useState<Record<string, ReasonMap>>({});

  // 判定依据按需拉取：全历史四维 reason 文本约 5MB，只在悬停到某天时单日精确请求一次。
  useEffect(() => {
    if (!code || !hoverDate || !series || reasonCache[hoverDate]) return;
    if (!byDate.has(hoverDate)) return;
    let cancelled = false;
    api.stockStateSeries(code, ktype, { from: hoverDate, to: hoverDate, includeReasons: true })
      .then((res) => {
        const reasons = res.items[0]?.reasons;
        if (cancelled || !reasons) return;
        setReasonCache((prev) => ({ ...prev, [hoverDate]: reasons }));
      })
      .catch(() => {
        // 依据拉取失败不影响状态带本身：图例仍显示状态与收盘价。
      });
    return () => {
      cancelled = true;
    };
  }, [byDate, code, hoverDate, ktype, reasonCache, series]);

  const current = useMemo(
    () => (hoverDate ? byDate.get(hoverDate) : undefined) ?? series?.latest ?? series?.items[series.items.length - 1],
    [byDate, hoverDate, series],
  );
  const reasons: ReasonMap | undefined = (current && reasonCache[current.date]) ?? current?.reasons;

  if (!series) {
    return (
      <div className="px-3 py-2 text-xs text-slate-500 border-t border-slate-800">
        {hint ?? '状态带计算中…'}
      </div>
    );
  }

  const bollZone = (series.definition as { boll_zone_ratio?: number }).boll_zone_ratio;

  return (
    <div className="border-t border-slate-800 bg-slate-900/60 px-3 py-2 space-y-1.5">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-slate-400">
        <span className="text-slate-300 font-medium">{current ? `${current.date} 四维状态` : '四维状态'}</span>
        {series.dimensions.map((meta) => {
          const state = current?.states[meta.dim as StateDim] ?? 'insufficient_data';
          const stateMeta = stateMetaMap(meta).get(state);
          return (
            <span key={meta.dim} className="flex items-center gap-1">
              <span className="text-slate-500">{meta.label}</span>
              <span
                className="inline-flex items-center rounded px-1.5 py-0.5 text-[11px] text-slate-900 font-medium"
                style={{ backgroundColor: stateMeta?.color ?? '#94a3b8' }}
                title={meta.states.find((s) => s.state === state)?.description ?? meta.description}
              >
                {stateMeta?.label ?? state}
              </span>
            </span>
          );
        })}
        {current && <span className="text-slate-500">收盘 {current.close.toFixed(2)}</span>}
        <span className="text-slate-600">口径 {series.adjust} · 版本 {series.meta.calc_version}</span>
      </div>

      {reasons && (
        <ul className="grid gap-x-4 gap-y-0.5 text-[11px] text-slate-500 sm:grid-cols-2">
          {series.dimensions.map((meta) => (
            <li key={meta.dim} className="truncate" title={reasons[meta.dim as StateDim] ?? ''}>
              <span className="text-slate-400">{meta.label}</span>：{reasons[meta.dim as StateDim] ?? '-'}
            </li>
          ))}
        </ul>
      )}

      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-slate-500">
        {series.dimensions.map((meta) => (
          <span key={meta.dim} className="flex items-center gap-1">
            <span className="text-slate-400">{meta.label}</span>
            {meta.states.map((state) => (
              <span key={state.state} className="flex items-center gap-0.5" title={state.description}>
                <span className="inline-block h-2 w-2 rounded-sm" style={{ backgroundColor: state.color }} />
                <span className="text-slate-500">{state.label}</span>
              </span>
            ))}
            <span className="text-slate-600">（热身 {fmt(series.warmup?.[meta.dim as StateDim])} 根）</span>
          </span>
        ))}
      </div>

      <div className="text-[11px] text-slate-600">
        布林触发带宽 {fmt(bollZone)}×布林带宽；琥珀色表示该维度当天指标热身不足（与灰色中间区不同），本期不参与筛选。
      </div>

      {hint && <div className="text-[11px] text-amber-500">{hint}</div>}
    </div>
  );
}
