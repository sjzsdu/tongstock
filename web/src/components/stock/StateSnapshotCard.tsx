import { Button, Card, Empty, Space, Tag, Tooltip, Typography } from 'antd';
import type { DimensionMeta, StateDim, StateSeriesResponse } from '../../types/api';
import { stateMetaMap } from '../charts/stateBands';

interface Props {
  series: StateSeriesResponse | null;
  hint?: React.ReactNode;
  onReload?: () => void;
}

function latestState(series: StateSeriesResponse, dim: StateDim) {
  const latest = series.latest ?? series.items[series.items.length - 1];
  return latest?.states[dim] ?? 'insufficient_data';
}

/**
 * 「信号」tab 上的四维状态快照：把状态型模型（买入/卖出/中性）放在事件型信号列表之上，
 * 让用户先看到「今天处在什么位置」，再看「今天发生了什么事件」。
 *
 * 文案、方向、配色全部来自后端下发的 dimensions 元数据，前端不写任何买卖方向映射。
 */
export function StateSnapshotCard({ series, hint, onReload }: Props) {
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

  const latest = series.latest ?? series.items[series.items.length - 1];
  if (!latest) {
    return (
      <Card size="small" title="四维状态（状态型模型）">
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该股票暂无可用日线，无法计算状态" />
      </Card>
    );
  }

  return (
    <Card
      size="small"
      title="四维状态（状态型模型）"
      extra={<Typography.Text type="secondary">{latest.date} · 收盘 {latest.close.toFixed(2)}</Typography.Text>}
    >
      <Space direction="vertical" size={10} style={{ display: 'flex' }}>
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
          {series.dimensions.map((meta: DimensionMeta) => {
            const state = latestState(series, meta.dim);
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
          判定依据：{series.dimensions.map((meta) => latest.reasons?.[meta.dim] ?? `${meta.label} -`).join('；')}
        </Typography.Text>
        <Typography.Text type="secondary" className="text-xs">
          口径 {series.adjust}（未复权） · 计算版本 {series.meta.calc_version} · 共 {series.meta.rows} 个交易日；
          琥珀色为指标热身不足，不是中性，也不参与筛选。
        </Typography.Text>
      </Space>
    </Card>
  );
}
