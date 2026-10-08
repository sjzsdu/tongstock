import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Card, Input, Popconfirm, Select, Space, Table, Tag, Tooltip, Typography, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { api } from '../api/client';
import type { ParadigmBacktestItem, ParadigmItem, ParadigmPromotionStatusResponse, ParadigmStatsResponse } from '../types/api';

const reliabilityColor: Record<string, string> = { high: 'green', medium: 'orange', low: 'red' };
const reviewStatusColor: Record<string, string> = { promoted: 'gold', rejected: 'red', verified: 'green', reviewed: 'blue' };

export default function Paradigms() {
  const [items, setItems] = useState<ParadigmItem[]>([]);
  const [total, setTotal] = useState(0);
  const [stats, setStats] = useState<ParadigmStatsResponse | null>(null);
  const [backtests, setBacktests] = useState<Record<string, ParadigmBacktestItem>>({});
  const [promotions, setPromotions] = useState<Record<string, ParadigmPromotionStatusResponse>>({});
  const [loading, setLoading] = useState(false);
  const [q, setQ] = useState('');
  const [side, setSide] = useState<string | undefined>();
  const [reviewStatus, setReviewStatus] = useState<string | undefined>();
  const [reliability, setReliability] = useState<string | undefined>();
  const pollTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

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

  useEffect(() => { queueMicrotask(() => void load()); }, [filters]);

  const refreshPromotion = async (id: string) => {
    try {
      const status = await api.paradigmPromotionStatus(id);
      setPromotions(prev => ({ ...prev, [id]: status }));
      if (status.run?.status === 'done' || status.run?.status === 'failed') {
        void load();
        return false; // 结束，停止轮询
      }
      return true; // 仍在运行，继续轮询
    } catch {
      return false;
    }
  };

  useEffect(() => {
    const running = Object.values(promotions).some(p => p.run?.status === 'running');
    if (!running) return;
    const runningIds = Object.entries(promotions)
      .filter(([, p]) => p.run?.status === 'running')
      .map(([id]) => id);
    pollTimer.current = setTimeout(async () => {
      for (const id of runningIds) {
        await refreshPromotion(id);
      }
    }, 2000);
    return () => { if (pollTimer.current) clearTimeout(pollTimer.current); };
  }, [promotions]);

  const runBacktest = async (record: ParadigmItem) => {
    try {
      const result = await api.paradigmBacktest(record.id);
      setBacktests(prev => ({ ...prev, [record.id]: result }));
    } catch (err) {
      message.error(String(err));
    }
  };

  const promote = async (record: ParadigmItem) => {
    try {
      await api.paradigmPromote(record.id);
      setPromotions(prev => ({
        ...prev,
        [record.id]: { paradigm_id: record.id, run: { status: 'running' } },
      }));
      message.info('晋级已启动：编译 → 真实冻结数据机器验证 → 方法库注册，可在此列查看进度');
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

  const renderPromotion = (record: ParadigmItem) => {
    const run = promotions[record.id]?.run;
    const busy = run?.status === 'running';
    const evidence = record.evidence;
    const blocked = evidence && evidence.must_fix && evidence.must_fix.length > 0 && !evidence.eligible;
    const parts: React.ReactNode[] = [];
    if (busy) {
      parts.push(<Tag key="running" color="processing">晋级验证中…</Tag>);
    } else if (record.review_status === 'promoted' && record.method_id) {
      parts.push(
        <Tooltip key="method" title="已晋级为可信方法，点击进入方法市场">
          <a href="/methods">方法 {record.method_id.slice(0, 18)}…</a>
        </Tooltip>,
      );
    } else if (record.review_status) {
      parts.push(
        <Tag key="review" color={reviewStatusColor[record.review_status] || 'default'}>
          {record.review_status}
        </Tag>,
      );
    }
    if (blocked) {
      parts.push(
        <Tooltip key="blockers" title={evidence!.must_fix!.join('；')}>
          <Tag color="orange" style={{ marginTop: 2 }}>不可晋级</Tag>
        </Tooltip>,
      );
    }
    parts.push(
      <Popconfirm
        key="btn"
        title="对该范式执行晋级？将编译为方法候选，在真实冻结数据上做样本外回测，由机器证据决定 verified/rejected。"
        onConfirm={() => promote(record)}
        disabled={busy || record.side !== 'buy'}
      >
        <Button size="small" loading={busy} disabled={record.side !== 'buy'}>晋级为方法</Button>
      </Popconfirm>,
    );
    return <Space direction="vertical" size={2}>{parts}</Space>;
  };

  const columns: ColumnsType<ParadigmItem> = [
    { title: '股票', width: 150, render: (_, r) => <span>{r.stock_name || r.stock_code}<br /><Typography.Text type="secondary">{r.stock_code}</Typography.Text></span> },
    { title: '范式', dataIndex: 'name', width: 420, render: renderParadigmSummary },
    { title: '方向', dataIndex: 'side', width: 80, render: v => <Tag color={v === 'sell' ? 'green' : 'red'}>{v}</Tag> },
    { title: '可靠性', width: 120, render: (_, r) => <Tag color={reliabilityColor[r.validation?.reliability_label || '']}>{r.validation?.reliability_label || '-'}</Tag> },
    {
      title: '晋级资格', width: 150,
      render: (_, r) => {
        const card = r.evidence;
        if (!card) return <Typography.Text type="secondary">待体检</Typography.Text>;
        if (card.eligible) return <Tag color="green">可晋级</Tag>;
        const reason = card.must_fix?.[0] || card.warnings?.[0] || '';
        return (
          <Tooltip title={reason}>
            <Tag color={r.review_status === 'rejected' ? 'red' : 'orange'}>{r.review_status === 'rejected' ? '已淘汰' : '不可晋级'}</Tag>
          </Tooltip>
        );
      },
    },
    { title: '复盘', width: 110, render: (_, r) => <Space direction="vertical" size={0}><span>{r.review_status || 'pending'}</span>{typeof r.actual_return === 'number' && <Typography.Text type="secondary">{r.actual_return.toFixed(2)}%</Typography.Text>}</Space> },
    { title: '更新时间', dataIndex: 'updated_at', width: 180, render: v => v ? new Date(v).toLocaleString() : '-' },
    { title: '回测', width: 200, fixed: 'right', render: (_, r) => {
      const b = backtests[r.id];
      if (!b) return <Button size="small" onClick={() => runBacktest(r)}>回测</Button>;
      return <Space direction="vertical" size={0}>
        <Typography.Text>样本 {b.metrics.total_trades ?? 0}</Typography.Text>
        <Typography.Text type="secondary">样本外胜率 {((b.metrics.win_rate ?? 0) * 100).toFixed(1)}%，净收益 {((b.metrics.total_return ?? 0) * 100).toFixed(2)}%</Typography.Text>
        <Typography.Text type="secondary">最大回撤 {((b.metrics.max_drawdown ?? 0) * 100).toFixed(2)}%</Typography.Text>
        <Typography.Text type="secondary" ellipsis={{ tooltip: b.experiment_id }}>实验 {b.experiment_id}</Typography.Text>
      </Space>;
    } },
    { title: '晋级', width: 170, fixed: 'right', render: (_, r) => renderPromotion(r) },
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
          <Select placeholder="复盘状态" allowClear value={reviewStatus} onChange={setReviewStatus} style={{ width: 140 }} options={['pending', 'reviewed', 'verified', 'promoted', 'rejected'].map(v => ({ value: v, label: v }))} />
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
        scroll={{ x: 1600 }}
        tableLayout="fixed"
      />
    </Space>
  );
}
