import { useEffect, useMemo, useState } from 'react';
import { Button, Card, Empty, Space, Tag, Tooltip, Typography } from 'antd';
import { api } from '../../api/client';
import type { DimensionMeta, StateDim, StateSeriesItem, StateSeriesResponse } from '../../types/api';
import { stateMetaMap } from '../charts/stateBands';

interface Props {
  series: StateSeriesResponse | null;
  /** 当前查看的交易日（来自 URL 的 date）；有值时卡片显示该日状态，而不是最新一根。 */
  date?: string;
  hint?: React.ReactNode;
  onReload?: () => void;
}

function stateOf(item: StateSeriesItem | undefined, dim: StateDim) {
  return item?.states[dim] ?? 'insufficient_data';
}

/**
 * 「信号」tab 上的四维状态快照：把状态型模型（买入/卖出/中性）放在事件型信号列表之上，
 * 让用户先看到「今天处在什么位置」，再看「今天发生了什么事件」。
 *
 * 文案、方向、配色全部来自后端下发的 dimensions 元数据，前端不写任何买卖方向映射。
 */
export function StateSnapshotCard({ series, date, hint, onReload }: Props) {
  const target = useMemo(
    () => (date && series ? series.items.find((item) => item.date === date) : undefined),
    [date, series],
  );
  const current = target ?? series?.latest ?? series?.items[series.items.length - 1];
  const [dayReasons, setDayReasons] = useState<Partial<Record<StateDim, string>> | undefined>(undefined);

  useEffect(() => {
    setDayReasons(undefined);
    if (!series || !date) return;
    const item = series.items.find((entry) => entry.date === date);
    if (!item || item.reasons) return;
    let cancelled = false;
    api.stockStateSeries(series.code, series.type, { from: date, to: date, includeReasons: true })
      .then((res) => {
        if (!cancelled) setDayReasons(res.items[0]?.reasons);
      })
      .catch(() => {
        // 判定依据失败不影响状态展示；卡片会显示“-”。
      });
    return () => { cancelled = true; };
  }, [date, series]);

  if (!series) {
    return (
      <Card size="small" title="四维状态（状态型模型）">
        <Space direction="vertical" size={4}>
          <Typography.Text type="secondary">{hint ?? '状态带计算中…'}</Typography.Text>
          {onReload && <Button size="small" onClick={onReload}>重试</Button>}
        </Space>
      </Card>
    );
  }

  if (!current) {
    return (
      <Card size="small" title="四维状态（状态型模型）">
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该股票暂无可用日线，无法计算状态" />
      </Card>
    );
  }

  const fallbackToLatest = !!date && !target;
  const reasons = current.reasons ?? dayReasons;
  return (
    <Card
      size="small"
      title="四维状态（状态型模型）"
      extra={
        <Typography.Text type="secondary">
          {current.time ?? current.date}{fallbackToLatest ? `（${date} 无状态，显示最新）` : ''} · 收盘 {current.close.toFixed(2)}
        </Typography.Text>
      }
    >
      <Space direction="vertical" size={10} style={{ display: 'flex' }}>
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
          {series.dimensions.map((meta: DimensionMeta) => {
            const state = stateOf(current, meta.dim);
            const stateMeta = stateMetaMap(meta).get(state);
            const description = meta.states.find((s) => s.state === state)?.description ?? meta.description;
            return (
              <div key={meta.dim} className="rounded border border-slate-800 px-2 py-1.5">
                <Tooltip title={meta.description}>
                  <Typography.Text type="secondary" className="text-xs">{meta.label}</Typography.Text>
                </Tooltip>
                <div className="mt-1 flex items-center gap-2">
                  <Tag color={stateMeta?.color} style={{ marginInlineEnd: 0 }}>{stateMeta?.label ?? state}</Tag>
                  <Typography.Text type="secondary" className="text-xs">{description}</Typography.Text>
                </div>
              </div>
            );
          })}
        </div>
        <Typography.Text type="secondary" className="text-xs">
          判定依据：{series.dimensions.map((meta) => reasons?.[meta.dim] ?? `${meta.label} -`).join('；')}
        </Typography.Text>
        <Typography.Text type="secondary" className="text-xs">
          口径 {series.adjust}（未复权） · 计算版本 {series.meta.calc_version} · 共 {series.meta.rows} 个交易日；
          琥珀色为指标热身不足，不是中性，也不参与筛选。
        </Typography.Text>
      </Space>
    </Card>
  );
}
