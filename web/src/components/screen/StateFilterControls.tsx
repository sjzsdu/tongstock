import { DatePicker, Flex, InputNumber, Select, Switch, Typography } from 'antd';
import dayjs from 'dayjs';
import type { DimensionMeta } from '../../types/api';
import type { StateFilter } from '../../types/screen';

type Props = {
  dimensions: DimensionMeta[];
  value: StateFilter;
  onChange: (value: StateFilter) => void;
  date: string;
  onDateChange: (date: string) => void;
};

export function StateFilterControls({ dimensions, value, onChange, date, onDateChange }: Props) {
  const triStates = (dim: DimensionMeta) => dim.states.filter((s) => s.direction !== 'insufficient_data');
  const options = (dim: DimensionMeta) => triStates(dim).map((s) => ({ value: s.state, label: `${s.label}（${s.direction_label}）` }));
  const transition = value.transition;
  const transitionDim = dimensions.find((d) => d.dim === transition?.dim);
  const updateTransition = (next: Partial<NonNullable<StateFilter['transition']>>) => {
    if (transition) onChange({ ...value, transition: { ...transition, ...next } });
  };
  return (
    <Flex vertical gap={12}>
      <Flex wrap gap={12} align="center">
        <Typography.Text>快照日期（日线 / 未复权）</Typography.Text>
        <DatePicker aria-label="快照日期" value={date && dayjs(date).isValid() ? dayjs(date) : null} onChange={(d) => onDateChange(d?.format('YYYY-MM-DD') || '')} placeholder="最新快照交易日" />
        <Typography.Text type="secondary">全部条件同时满足（AND）；不限定与中性是不同条件。</Typography.Text>
      </Flex>
      <Flex wrap gap={12}>
        {dimensions.map((dim) => (
          <Flex key={dim.dim} vertical gap={4}>
            <Typography.Text>{dim.label}</Typography.Text>
            <Select
              aria-label={dim.label}
              style={{ minWidth: 190 }}
              allowClear
              placeholder="不限定"
              value={value.conditions.find((c) => c.dim === dim.dim)?.state}
              options={options(dim)}
              onChange={(state) => onChange({ ...value, conditions: [...value.conditions.filter((c) => c.dim !== dim.dim), ...(state ? [{ dim: dim.dim, state }] : [])] })}
            />
          </Flex>
        ))}
      </Flex>
      <Flex wrap gap={8} align="center">
        <Switch aria-label="启用迁移条件" checked={!!transition} disabled={!dimensions.length} onChange={(enabled) => {
          const dim = dimensions[0];
          const states = dim && triStates(dim);
          onChange({ ...value, transition: enabled && states?.length ? { dim: dim.dim, from: [states[1].state, states[2].state] as NonNullable<StateFilter['transition']>['from'], to: states[0].state as NonNullable<StateFilter['transition']>['to'], within_days: 5 } : undefined });
        }} />
        <Typography.Text>同时满足迁移条件</Typography.Text>
        {transition && transitionDim && <>
          <Select aria-label="迁移维度" value={transition.dim} options={dimensions.map((d) => ({ value: d.dim, label: d.label }))} onChange={(dim) => updateTransition({ dim })} />
          <Typography.Text>由</Typography.Text>
          <Select aria-label="迁移起态" mode="multiple" style={{ minWidth: 220 }} value={transition.from} options={options(transitionDim).filter((o) => o.value !== transition.to)} onChange={(from) => updateTransition({ from })} />
          <Typography.Text>转为</Typography.Text>
          <Select aria-label="迁移终态" style={{ minWidth: 180 }} value={transition.to} options={options(transitionDim)} onChange={(to) => updateTransition({ to, from: transition.from.filter((s) => s !== to) })} />
          <Typography.Text>近</Typography.Text>
          <InputNumber aria-label="迁移窗口天数" min={1} max={365} value={transition.within_days} onChange={(within_days) => updateTransition({ within_days: within_days ?? 5 })} />
          <Select
            aria-label="迁移窗口单位"
            style={{ width: 108 }}
            value={transition.within_unit ?? 'natural'}
            options={[{ value: 'natural', label: '自然日' }, { value: 'trading', label: '交易日' }]}
            onChange={(within_unit) => updateTransition({ within_unit })}
          />
          <Typography.Text>内（含所选日）</Typography.Text>
        </>}
      </Flex>
      {transition && (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          自然日按日历回数；交易日按快照库实际交易日回数（跨周末会自动延长到第 N 个交易日）。
        </Typography.Text>
      )}
    </Flex>
  );
}
