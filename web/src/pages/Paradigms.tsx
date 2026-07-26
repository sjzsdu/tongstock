import { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, Input, Popconfirm, Select, Space, Table, Tag, Typography, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { api } from '../api/client';
import type { ParadigmBacktestItem, ParadigmItem, ParadigmStatsResponse } from '../types/api';

const reliabilityColor: Record<string, string> = { high: 'green', medium: 'orange', low: 'red' };

export default function Paradigms() {
  const [items, setItems] = useState<ParadigmItem[]>([]);
  const [total, setTotal] = useState(0);
  const [stats, setStats] = useState<ParadigmStatsResponse | null>(null);
  const [backtests, setBacktests] = useState<Record<string, ParadigmBacktestItem>>({});
  const [loading, setLoading] = useState(false);
  const [q, setQ] = useState('');
  const [side, setSide] = useState<string | undefined>();
  const [reviewStatus, setReviewStatus] = useState<string | undefined>();
  const [reliability, setReliability] = useState<string | undefined>();

  const filters = useMemo(() => ({ q, side, review_status: reviewStatus, reliability, limit: 100 }), [q, side, reviewStatus, reliability]);

  const load = async () => {
    setLoading(true);
    try {
      const [list, s] = await Promise.all([api.paradigmList(undefined, undefined, filters), api.paradigmStats()]);
      setItems(list.paradigms || []);
      setTotal(list.total || 0);
      setStats(s);
    } catch (err) {
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, [filters]);

  const runBacktest = async (record: ParadigmItem) => {
    try {
      const result = await api.paradigmBacktest(record.id);
      if (result[0]) setBacktests(prev => ({ ...prev, [record.id]: result[0] }));
    } catch (err) {
      message.error(String(err));
    }
  };

  const remove = async (record: ParadigmItem) => {
    await api.paradigmDelete(record.id);
    message.success('已删除');
    load();
  };

  const renderParadigmSummary = (name: string, record: ParadigmItem) => (
    <Space direction="vertical" size={4} style={{ minWidth: 360, maxWidth: 520 }}>
      <Typography.Paragraph
        strong
        ellipsis={{ rows: 2, tooltip: name }}
        style={{ marginBottom: 0, lineHeight: 1.35 }}
      >
        {name || '未命名范式'}
      </Typography.Paragraph>
      {record.rationale && (
        <Typography.Paragraph
          type="secondary"
          ellipsis={{ rows: 2, tooltip: record.rationale }}
          style={{ marginBottom: 0, fontSize: 12, lineHeight: 1.35 }}
        >
          {record.rationale}
        </Typography.Paragraph>
      )}
      <Typography.Text code style={{ fontSize: 11, whiteSpace: 'normal', wordBreak: 'break-all' }}>
        {record.id}
      </Typography.Text>
    </Space>
  );

  const columns: ColumnsType<ParadigmItem> = [
    { title: '股票', width: 150, render: (_, r) => <span>{r.stock_name || r.stock_code}<br /><Typography.Text type="secondary">{r.stock_code}</Typography.Text></span> },
    { title: '范式', dataIndex: 'name', width: 460, render: renderParadigmSummary },
    { title: '方向', dataIndex: 'side', width: 80, render: v => <Tag color={v === 'sell' ? 'red' : 'green'}>{v}</Tag> },
    { title: '可靠性', width: 120, render: (_, r) => <Tag color={reliabilityColor[r.validation?.reliability_label || '']}>{r.validation?.reliability_label || '-'}</Tag> },
    { title: '复盘', width: 110, render: (_, r) => <Space direction="vertical" size={0}><span>{r.review_status || 'pending'}</span>{typeof r.actual_return === 'number' && <Typography.Text type="secondary">{r.actual_return.toFixed(2)}%</Typography.Text>}</Space> },
    { title: '更新时间', dataIndex: 'updated_at', width: 180, render: v => v ? new Date(v).toLocaleString() : '-' },
    { title: '回测', width: 200, fixed: 'right', render: (_, r) => {
      const b = backtests[r.id];
      if (!b) return <Button size="small" onClick={() => runBacktest(r)}>回测</Button>;
      if (b.error) return <Typography.Text type="danger">{b.error}</Typography.Text>;
      return <Space direction="vertical" size={0}>
        <Typography.Text>样本 {b.sample_size}</Typography.Text>
        <Typography.Text type="secondary">20日胜率 {(b.win_rate_20 * 100).toFixed(1)}%，均值 {b.avg_return_20.toFixed(2)}%</Typography.Text>
        <Typography.Text type="secondary">最大回撤 {b.max_drawdown.toFixed(2)}%</Typography.Text>
      </Space>;
    } },
    { title: '操作', width: 130, fixed: 'right', render: (_, r) => <Space><Button size="small" onClick={() => window.location.href = `/stock/${r.stock_code}`}>查看</Button><Popconfirm title="删除该范式？" onConfirm={() => remove(r)}><Button danger size="small">删</Button></Popconfirm></Space> },
  ];

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      <Typography.Title level={3}>范式管理</Typography.Title>
      {stats && (
        <Alert type="info" showIcon message={`共 ${stats.total} 个范式，高可靠 ${stats.high_reliability} 个，复盘胜率 ${(stats.win_rate * 100).toFixed(1)}%，平均收益 ${stats.average_return.toFixed(2)}%`} />
      )}
      <Card>
        <Space wrap>
          <Input.Search placeholder="搜索名称/股票/ID" value={q} onChange={e => setQ(e.target.value)} style={{ width: 220 }} allowClear />
          <Select placeholder="方向" allowClear value={side} onChange={setSide} style={{ width: 120 }} options={[{ value: 'buy', label: 'buy' }, { value: 'sell', label: 'sell' }]} />
          <Select placeholder="复盘状态" allowClear value={reviewStatus} onChange={setReviewStatus} style={{ width: 140 }} options={['pending', 'reviewed', 'verified', 'rejected'].map(v => ({ value: v, label: v }))} />
          <Select placeholder="可靠性" allowClear value={reliability} onChange={setReliability} style={{ width: 120 }} options={['high', 'medium', 'low'].map(v => ({ value: v, label: v }))} />
          <Button onClick={load}>刷新</Button>
        </Space>
      </Card>
      <Table
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={items}
        pagination={{ total, pageSize: 20 }}
        scroll={{ x: 1390 }}
        tableLayout="fixed"
      />
    </Space>
  );
}
