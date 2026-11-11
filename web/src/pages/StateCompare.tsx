import { useEffect, useMemo, useState } from 'react';
import { Alert, Card, DatePicker, Empty, Flex, Space, Spin, Tag, Typography } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import type { DimensionMeta, StateCompareResponse, StateCompareStock, StateDim, StateValue } from '../types/api';
import { stateMetaMap } from '../components/charts/stateBands';

const { Text, Title } = Typography;
const DIMS: StateDim[] = ['cross', 'osc', 'boll', 'ma'];
const MAX_CODES = 100;

function dateValue(value: string | null, fallback: string): string {
  return value && /^\d{4}-\d{2}-\d{2}$/.test(value) ? value : fallback;
}

function signedPct(value: number | null): string {
  if (value == null || !Number.isFinite(value)) return '-';
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`;
}

function price(value: number | null): string {
  return value == null || !Number.isFinite(value) ? '-' : value.toFixed(2);
}

function stateLabel(dimensions: DimensionMeta[], dim: StateDim, state: StateValue): string {
  const meta = dimensions.find((entry) => entry.dim === dim);
  return meta?.states.find((entry) => entry.state === state)?.label || state;
}

export default function StateCompare() {
  const [params, setParams] = useSearchParams();
  const allCodes = useMemo(
    () => Array.from(new Set((params.get('codes') || '').split(',').map((v) => v.trim()).filter(Boolean))),
    [params],
  );
  const codes = useMemo(() => allCodes.slice(0, MAX_CODES), [allCodes]);
  const defaultTo = dayjs().format('YYYY-MM-DD');
  const [from, setFrom] = useState(dateValue(params.get('from'), dayjs().subtract(30, 'day').format('YYYY-MM-DD')));
  const [to, setTo] = useState(dateValue(params.get('to'), defaultTo));
  const [data, setData] = useState<StateCompareResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (codes.length === 0 || from > to) return;
    let cancelled = false;
    setLoading(true);
    setError('');
    setData(null);
    api.stateCompare(codes, from, to).then((result) => {
      if (!cancelled) setData(result);
    }).catch((err: unknown) => {
      if (!cancelled) setError(err instanceof Error ? err.message : '读取状态对照失败');
    }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [codes, from, to]);

  const updateDate = (key: 'from' | 'to', value: Dayjs | null) => {
    const next = value?.format('YYYY-MM-DD') || '';
    if (key === 'from') setFrom(next); else setTo(next);
    setParams((prev) => { const nextParams = new URLSearchParams(prev); if (next) nextParams.set(key, next); else nextParams.delete(key); return nextParams; }, { replace: true });
  };

  if (codes.length === 0) return <Empty description="请从筛选结果带入股票，或在地址中提供 codes" />;
  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      <Card>
        <Flex justify="space-between" align="center" wrap gap={12}>
          <div>
            <Title level={4} style={{ margin: 0 }}>状态与股价对照</Title>
            <Text type="secondary">{codes.length} 只 · {from || '?'} ～ {to || '?'} · 同一交易日轴对齐</Text>
            <div><Text type="secondary" style={{ fontSize: 12 }}>收盘价为未复权原始价；持续段按窗口内首个有效状态起算，区间涨跌幅可用页面给出的起止价直接复算。</Text></div>
          </div>
          <Space>
            <DatePicker value={from ? dayjs(from) : null} onChange={(v) => updateDate('from', v)} />
            <Text type="secondary">至</Text>
            <DatePicker value={to ? dayjs(to) : null} onChange={(v) => updateDate('to', v)} />
          </Space>
        </Flex>
      </Card>
      {allCodes.length > MAX_CODES && (
        <Alert
          type="warning"
          showIcon
          message={`当前命中 ${allCodes.length} 只，单次对照最多 ${MAX_CODES} 只；已取前 ${MAX_CODES} 只。请缩小筛选或分批对照，避免只看到部分结果却以为已看全。`}
        />
      )}
      {from > to && <Alert type="error" message="开始日期不能晚于结束日期" />}
      {error && <Alert type="error" showIcon message="读取状态对照失败" description={error} />}
      {loading && (
        <Card>
          <Space>
            <Spin />
            <Text type="secondary">正在计算状态持续段与涨跌幅；若标的较多请稍候（单次最长 60 秒，超时会明确报错）。</Text>
          </Space>
        </Card>
      )}
      {!loading && data?.stocks.map((stock) => <StockBandCard key={stock.code} stock={stock} dimensions={data.dimensions} />)}
    </Space>
  );
}

function StockBandCard({ stock, dimensions }: { stock: StateCompareStock; dimensions: StateCompareResponse['dimensions'] }) {
  return (
    <Card title={<Space><Text strong>{stock.name || stock.code}</Text><Text type="secondary">{stock.code}</Text></Space>}>
      <div style={{ overflowX: 'auto' }}>
        <div style={{ minWidth: Math.max(900, stock.items.length * 40) }}>
          <Flex gap={8} style={{ marginBottom: 8 }}>
            <Text type="secondary" style={{ width: 76 }}>日期</Text>
            {stock.items.map((item) => <Text key={item.date} type="secondary" style={{ width: 36, fontSize: 10, writingMode: 'vertical-rl' }}>{item.date.slice(5)}</Text>)}
          </Flex>
          <div style={{ display: 'grid', gap: 4 }}>
            {DIMS.map((dim) => {
              const meta = dimensions.find((entry) => entry.dim === dim);
              const colors = meta ? stateMetaMap(meta) : new Map<StateValue, { color: string; label: string }>();
              return (
                <Flex key={dim} gap={8} align="center">
                  <Text style={{ width: 76 }}>{meta?.label || dim}</Text>
                  {stock.items.map((item) => {
                    const state = item.states[dim];
                    const stateMeta = colors.get(state);
                    return <span key={`${dim}-${item.date}`} title={`${item.date} ${stateMeta?.label || state} · 收盘 ${price(item.close)}`} style={{ display: 'inline-flex', width: 36, height: 18, borderRadius: 3, background: stateMeta?.color || '#94a3b8' }} />;
                  })}
                </Flex>
              );
            })}
          </div>
          <Flex gap={8} style={{ marginTop: 8 }}>
            <Text type="secondary" style={{ width: 76 }}>收盘</Text>
            {stock.items.map((item) => <Text key={item.date} title={item.close == null ? '-' : String(item.close)} style={{ width: 36, fontSize: 9, fontVariantNumeric: 'tabular-nums' }}>{price(item.close)}</Text>)}
          </Flex>
        </div>
      </div>
      <Flex gap={8} wrap style={{ marginTop: 20 }}>
        {stock.transitions.length === 0
          ? <Text type="secondary">区间内没有有效状态迁移</Text>
          : stock.transitions.map((transition) => (
            <Tag key={`${transition.dim}-${transition.date}-${transition.from}-${transition.to}`}>
              {transition.date} · {dimensions.find((entry) => entry.dim === transition.dim)?.label || transition.dim} {stateLabel(dimensions, transition.dim, transition.from)} → {stateLabel(dimensions, transition.dim, transition.to)}
            </Tag>
          ))}
      </Flex>
      <div style={{ marginTop: 12 }}>
        <Text strong>状态持续期 × 区间涨跌</Text>
        <div style={{ marginTop: 8, display: 'grid', gap: 4 }}>
          {stock.durations.map((duration) => (
            <Text key={`${duration.dim}-${duration.from}-${duration.to}`} type="secondary">
              {duration.from}～{duration.to} · {dimensions.find((entry) => entry.dim === duration.dim)?.label || duration.dim}/{stateLabel(dimensions, duration.dim, duration.state)} · {duration.trading_days} 个交易日 · {price(duration.start_close)} → {price(duration.end_close)} · {signedPct(duration.return_pct)}
            </Text>
          ))}
        </div>
      </div>
    </Card>
  );
}
