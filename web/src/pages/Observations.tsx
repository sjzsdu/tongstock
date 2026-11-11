import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import {
  Alert, Button, Card, Collapse, Descriptions, Drawer, Empty, Select, Space,
  Statistic, Table, Tag, Typography, message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { api, type ForwardObservation } from '../api/client';
import CandlestickChart from '../components/charts/CandlestickChart';
import { distanceToObservationTarget, observationTargetDescription } from '../lib/observations';
import { formatDateTime } from '../lib/datetime';

const { Text, Title } = Typography;
const labels: Record<string, string> = {
  ongoing: '进行中', hit: '已达标', expired: '到期未达标',
  insufficient_data: '数据不足', target_undefined: '目标未定义',
};
const colors: Record<string, string> = {
  ongoing: 'blue', hit: 'green', expired: 'orange', insufficient_data: 'red', target_undefined: 'default',
};

export default function Observations() {
  const [search, setSearch] = useSearchParams();
  const selectedID = search.get('id') || '';
  const [items, setItems] = useState<ForwardObservation[]>([]);
  const [total, setTotal] = useState(0);
  const [summary, setSummary] = useState<Record<string, number>>({});
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [detail, setDetail] = useState<ForwardObservation>();
  const [detailError, setDetailError] = useState('');
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async () => {
    try {
      const result = await api.observations(30, (page - 1) * 30, status);
      setItems(result.items);
      setTotal(result.total);
      setSummary(result.summary);
      setError('');
    } catch (error) {
      setError(error instanceof Error ? error.message : '观察列表加载失败');
    } finally {
      setLoading(false);
    }
  }, [page, status]);

  const loadDetail = useCallback(async () => {
    if (!selectedID) return;
    try {
      setDetail(await api.observation(selectedID));
      setDetailError('');
    } catch (error) {
      setDetailError(error instanceof Error ? error.message : '观察详情加载失败');
    }
  }, [selectedID]);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    try {
      await api.observationRefresh();
      void message.info('已在后台更新观察；公司行为缺失会自动补取，价格只读本地已同步日线');
    } catch (error) {
      void message.error(error instanceof Error ? error.message : '刷新失败');
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => { queueMicrotask(() => void load()); }, [load]);
  useEffect(() => { queueMicrotask(() => void loadDetail()); }, [loadDetail]);
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') {
        void load();
        void loadDetail();
      }
    }, 15000);
    return () => window.clearInterval(timer);
  }, [load, loadDetail]);
  const columns: ColumnsType<ForwardObservation> = [
    {
      title: '股票 / 方法', key: 'stock',
      render: (_, item) => (
        <Space orientation="vertical" size={2}>
          <Link to={`/stock/${item.contract.code}`}>
            {item.contract.name !== item.contract.code && `${item.contract.name} `}{item.contract.code}
          </Link>
          <Text>{item.contract.method_name}</Text>
        </Space>
      ),
    },
    { title: '选股日期', dataIndex: ['contract', 'signal_date'], width: 115 },
    { title: '冻结目标', key: 'target', render: (_, item) => <Text>{observationTargetDescription(item)}</Text> },
    {
      title: '进度', key: 'progress', width: 160,
      render: (_, item) => (
        <Space orientation="vertical" size={2}>
          <Text>已覆盖 {item.evaluation?.progress || 0}/{item.contract.outcome.horizon_days || '?'} 日线</Text>
          <Text type="secondary">{distanceToObservationTarget(item)}</Text>
        </Space>
      ),
    },
    {
      title: '观察期涨跌', key: 'return', width: 140,
      render: (_, item) => item.evaluation?.return_pct != null
        ? `${(item.evaluation.return_pct * 100).toFixed(2)}%` : '0.00%（截至信号日）',
    },
    {
      title: '验证结果', key: 'result', width: 150,
      render: (_, item) => (
        <Space orientation="vertical" size={2}>
          <Tag color={colors[item.evaluation?.status || 'ongoing']}>{labels[item.evaluation?.status || 'ongoing']}</Tag>
          {item.evaluation?.status === 'hit' && !item.evaluation.deadline && <Text type="secondary">提前达标，窗口未结束</Text>}
          <Button type="link" size="small" onClick={() => {
            setDetail(undefined);
            setSearch({ id: item.contract.id });
          }}>查看走势</Button>
        </Space>
      ),
    },
  ];

  const visibleDetail = detail?.contract.id === selectedID ? detail : undefined;
  const evaluation = visibleDetail?.evaluation;
  const contract = visibleDetail?.contract;
  const klines = useMemo(() => (evaluation?.bars || []).map((bar) => ({
    Time: bar.date, Open: bar.open, High: bar.high, Low: bar.low,
    Close: bar.close, Volume: bar.volume, Amount: 0,
  })), [evaluation?.bars]);
  const annotation = useMemo(() => contract ? {
    baseline: contract.baseline,
    target: contract.outcome.target_return_pct == null ? undefined : contract.baseline * (1 + contract.outcome.target_return_pct),
    signalDate: contract.signal_date,
    hitDate: evaluation?.hit_date,
  } : undefined, [contract, evaluation?.hit_date]);

  return (
    <Space orientation="vertical" size={16} style={{ display: 'flex' }}>
      <Space wrap style={{ justifyContent: 'space-between', width: '100%' }}>
        <Title level={2} style={{ margin: 0 }}>走势验证</Title>
        <Button onClick={() => void refresh()} loading={refreshing}>刷新观察</Button>
      </Space>
      <Alert
        type="info" showIcon title="保留选股当时的目标，持续看真实走势"
        description="基准是信号日冻结收盘价，不是成交价或回测的次日开盘价。目标取该版本编译出的 outcome（止盈阈值 + 最长持有），冻结后不随方法版本变化；不模拟交易，不影响方法健康。未来胜率与本页达标率无关。"
      />
      <Space wrap size={24}>
        <Statistic title="观察条数" value={summary.total || 0} />
        <Statistic title="已达标" value={summary.hit || 0} />
        <Statistic title="进行中" value={summary.ongoing || 0} />
        <Statistic title="到期且数据完整样本" value={summary.concluded_complete || 0} />
        <Statistic title="到期样本达标率"
          value={summary.concluded_complete ? ((summary.hit_complete || 0) / summary.concluded_complete * 100).toFixed(1) : '—'}
          suffix={summary.concluded_complete ? '%' : undefined}
        />
        {!!summary.corrupt && <Statistic title="评估记录损坏" value={summary.corrupt} valueStyle={{ color: '#dc2626' }} />}
      </Space>
      <Text type="secondary">达标率仅统计整个H交易日窗口结束、数据完整且已得结论的观察；提前达标但未到期、进行中和数据不足均不计分母。</Text>
      <Select
        aria-label="筛选验证状态" value={status} style={{ width: 200 }}
        onChange={(value) => { setPage(1); setStatus(value); }}
        options={[{ value: '', label: '全部状态' }, ...Object.entries(labels).map(([value, label]) => ({ value, label }))]}
      />
      {error && <Alert type="error" showIcon title={error} action={<Button onClick={() => void load()}>重试</Button>} />}
      <Table
        rowKey={(item) => item.contract.id} loading={loading} dataSource={items} columns={columns}
        scroll={{ x: 1100 }} pagination={{ current: page, pageSize: 30, total, onChange: setPage }}
        locale={{ emptyText: <Empty description="还没有观察记录；从今日候选或选股结果中按方法加入走势验证" /> }}
      />
      <Drawer
        open={!!selectedID}
        onClose={() => { setSearch({}); setDetail(undefined); }}
        title={contract ? `${contract.name !== contract.code ? `${contract.name} ${contract.code}` : contract.code} · ${contract.method_name}` : '走势验证详情'}
        size="large"
      >
        {detailError && <Alert type="error" title={detailError} />}
        {contract && visibleDetail && (
          <Space orientation="vertical" size={16} style={{ display: 'flex' }}>
            <Alert
              type={evaluation?.status === 'insufficient_data' ? 'warning' : 'info'} showIcon
              title={labels[evaluation?.status || 'ongoing']}
              description={evaluation?.reason || '已加入，等待后台读取本地行情评估'}
            />
            <Card size="small" title="当时冻结的验证目标">
              <Text strong>{observationTargetDescription(visibleDetail)}</Text>
              <p><Text>基准 {contract.signal_date} 收盘 {contract.baseline.toFixed(4)}元；只统计之后的完整交易日，同日不计入未来。</Text></p>
              <Text type="secondary">与实际买入盈亏不同；历史回测使用次日开盘基准，本页是信号收盘基准的独立验证。</Text>
            </Card>
            {klines.length > 0 ? (
              <Card size="small" title="实际走势 vs 冻结目标">
                <CandlestickChart klines={klines} mainOverlay="none" observation={annotation} />
                <Text type="secondary">蓝线：冻结基准；黄线：冻结目标；箭头：信号日与首次观测达标。图为本地原始日线，口径未对齐时只展示、不据此结案。</Text>
              </Card>
            ) : <Empty description="尚无可展示的本地日线；先同步行情后刷新观察" />}
            <Descriptions size="small" bordered column={1}>
              <Descriptions.Item label="窗口进度">已覆盖 {evaluation?.progress || 0}/{contract.outcome.horizon_days || '?'} 日线</Descriptions.Item>
              <Descriptions.Item label="行情截止（市场）">{evaluation?.as_of || '等待首个完整交易日'}</Descriptions.Item>
              <Descriptions.Item label="本股最新日线">{evaluation?.data_as_of || '该窗口内尚未取到有效日线'}</Descriptions.Item>
              <Descriptions.Item label="首次观测达标">
                {evaluation?.hit_date || '尚未观测到'}{evaluation?.missing_dates?.length ? '（存在缺口，不能保证这是最早命中）' : ''}
              </Descriptions.Item>
              <Descriptions.Item label="窗口终点">{evaluation?.deadline || '等待完整窗口数据'}</Descriptions.Item>
              <Descriptions.Item label="数据缺口">{evaluation?.missing_dates?.join('、') || '暂无已记录缺口'}</Descriptions.Item>
              <Descriptions.Item label="加入时间">{formatDateTime(contract.created_at)}</Descriptions.Item>
              <Descriptions.Item label="方法版本">{contract.method_version_id}</Descriptions.Item>
              <Descriptions.Item label="选股运行"><Link to={`/methods/selection/${contract.selection_run_id}`}>{contract.selection_run_id}</Link></Descriptions.Item>
              <Descriptions.Item label="原快照口径标记">{contract.snapshot_adjustment || '未记录'}；必须通过锚日原始价校验，存在公司行为时暂不判定。</Descriptions.Item>
              <Descriptions.Item label="评估时间">{evaluation ? formatDateTime(evaluation.evaluated_at) : '未评估'}</Descriptions.Item>
            </Descriptions>
            <Collapse items={[{
              key: 'audit', label: '冻结合约与评估依据（审计）',
              children: <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{JSON.stringify({ contract, evaluation: { ...evaluation, bars: undefined } }, null, 2)}</pre>,
            }]} />
          </Space>
        )}
      </Drawer>
    </Space>
  );
}
