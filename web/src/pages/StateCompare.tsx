import { useEffect, useMemo, useState } from 'react';
import { Alert, Card, DatePicker, Empty, Flex, Space, Spin, Tag, Typography } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import type { StateCompareResponse, StateCompareStock, StateDim, StateValue } from '../types/api';
import { stateMetaMap } from '../components/charts/stateBands';

const { Text, Title } = Typography;
const DIMS: StateDim[] = ['cross', 'osc', 'boll', 'ma'];

function dateValue(value: string | null, fallback: string): string {
  return value && /^\d{4}-\d{2}-\d{2}$/.test(value) ? value : fallback;
}

function signedPct(value: number | null): string {
  if (value == null || !Number.isFinite(value)) return '-';
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`;
}

export default function StateCompare() {
  const [params, setParams] = useSearchParams();
  const codes = useMemo(() => Array.from(new Set((params.get('codes') || '').split(',').map((v) => v.trim()).filter(Boolean))).slice(0, 20), [params]);
  const defaultTo = dayjs().format('YYYY-MM-DD');
  const [from, setFrom] = useState(dateValue(params.get('from'), dayjs().subtract(30, 'day').format('YYYY-MM-DD')));
  const [to, setTo] = useState(dateValue(params.get('to'), defaultTo));
  const [data, setData] = useState<StateCompareResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (codes.length === 0 || from > to) return;
    let cancelled = false;
    setLoading(true); setError('');
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
          <div><Title level={4} style={{ margin: 0 }}>状态与股价对照</Title><Text type="secondary">{codes.join('、')} · 同一交易日轴对齐</Text></div>
          <Space>
            <DatePicker value={from ? dayjs(from) : null} onChange={(v) => updateDate('from', v)} />
            <Text type="secondary">至</Text>
            <DatePicker value={to ? dayjs(to) : null} onChange={(v) => updateDate('to', v)} />
          </Space>
        </Flex>
      </Card>
      {from > to && <Alert type="error" message="开始日期不能晚于结束日期" />}
      {error && <Alert type="error" message={error} />}
      {loading && <Card><Spin /></Card>}
      {!loading && data?.stocks.map((stock) => <StockBandCard key={stock.code} stock={stock} dimensions={data.dimensions} />)}
    </Space>
  );
}

function StockBandCard({ stock, dimensions }: { stock: StateCompareStock; dimensions: StateCompareResponse['dimensions'] }) {
  return (
    <Card title={<Space><Text strong>{stock.name || stock.code}</Text><Text type="secondary">{stock.code}</Text></Space>}>
      <div style={{ overflowX: 'auto' }}>
        <div style={{ minWidth: Math.max(760, stock.items.length * 32) }}>
          <Flex gap={8} style={{ marginBottom: 8 }}><Text type="secondary" style={{ width: 76 }}>日期</Text>{stock.items.map((item) => <Text key={item.date} type="secondary" style={{ width: 24, fontSize: 10, writingMode: 'vertical-rl' }}>{item.date.slice(5)}</Text>)}</Flex>
          <div style={{ display: 'grid', gap: 4 }}>
            {DIMS.map((dim) => {
              const meta = dimensions.find((entry) => entry.dim === dim);
              const colors = meta ? stateMetaMap(meta) : new Map<StateValue, { color: string; label: string }>();
              return <Flex key={dim} gap={8} align="center"><Text style={{ width: 76 }}>{meta?.label || dim}</Text>{stock.items.map((item) => { const state = item.states[dim]; const stateMeta = colors.get(state); return <span key={`${dim}-${item.date}`} title={`${item.date} ${stateMeta?.label || state}`} style={{ display: 'inline-flex', width: 24, height: 18, borderRadius: 3, background: stateMeta?.color || '#94a3b8' }} />; })}</Flex>;
            })}
          </div>
          <Flex gap={8} style={{ marginTop: 8 }}><Text type="secondary" style={{ width: 76 }}>收盘</Text>{stock.items.map((item) => <Text key={item.date} style={{ width: 24, fontSize: 10, transform: 'rotate(-55deg)', transformOrigin: 'left top' }}>{item.close == null ? '-' : item.close.toFixed(0)}</Text>)}</Flex>
        </div>
      </div>
      <Flex gap={8} wrap style={{ marginTop: 20 }}>
        {stock.transitions.length === 0 ? <Text type="secondary">区间内没有有效状态迁移</Text> : stock.transitions.map((transition) => <Tag key={`${transition.dim}-${transition.date}-${transition.from}-${transition.to}`}>{transition.date} · {transition.dim} {transition.from} → {transition.to}</Tag>)}
      </Flex>
      <div style={{ marginTop: 12 }}>
        <Text strong>状态持续期 × 区间涨跌</Text>
        <div style={{ marginTop: 8, display: 'grid', gap: 4 }}>
          {stock.durations.map((duration) => <Text key={`${duration.dim}-${duration.from}`} type="secondary">{duration.from}～{duration.to} · {duration.dim}/{duration.state} · {duration.trading_days} 个交易日 · {signedPct(duration.return_pct)}</Text>)}
        </div>
      </div>
    </Card>
  );
}
