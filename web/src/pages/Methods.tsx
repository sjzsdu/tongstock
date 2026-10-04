import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Descriptions,
  Drawer,
  Empty,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Timeline,
  Tooltip,
  Typography,
  message,
} from 'antd';
import {
  CheckOutlined,
  CloseOutlined,
  DownloadOutlined,
  ExperimentOutlined,
  FileSearchOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import {
  api,
  type MethodAuditEvent,
  type MethodCard,
  type MethodForwardHealth,
  type MethodRejectStat,
  type MethodResearchResult,
  type MethodSeedResult,
} from '../api/client';
import { formatDateTime } from '../lib/datetime';

const { Text, Title } = Typography;

const STATUS_OPTIONS = [
  { value: 'verified', label: 'verified · 已验证' },
  { value: 'observing', label: 'observing · 观察中' },
  { value: 'degraded', label: 'degraded · 降级' },
  { value: 'draft', label: 'draft · 草稿' },
  { value: 'candidate', label: 'candidate · 候选' },
  { value: 'rejected', label: 'rejected · 未通过门槛' },
  { value: 'retired', label: 'retired · 已退役' },
];

const CONFIDENCE_FILTER_OPTIONS = [
  { value: 'strong', label: '置信度 ≥ strong' },
  { value: 'moderate', label: '置信度 ≥ moderate' },
  { value: 'weak', label: '置信度 ≥ weak' },
];

// 后端 holding_min_days / holding_max_days 是区间相交语义
// (internal/adapter/methodregistryrepo/sqlite.go:93-98)：holding_min_days=X 只保留
// 「最长持有 ≥ X」的方法，holding_max_days=Y 只保留「最短持有 ≤ Y」的方法。
// 因此选项按这两个真实语义命名，不提供后端无法表达的「长/中/短」分桶。
const HOLDING_MIN_OPTIONS = [
  { value: '5', label: '最长持有 ≥ 5 个交易日' },
  { value: '20', label: '最长持有 ≥ 20 个交易日' },
  { value: '60', label: '最长持有 ≥ 60 个交易日' },
];

const HOLDING_MAX_OPTIONS = [
  { value: '1', label: '最短持有 ≤ 1 个交易日' },
  { value: '5', label: '最短持有 ≤ 5 个交易日' },
  { value: '20', label: '最短持有 ≤ 20 个交易日' },
];

const MIN_HEALTH_OPTIONS = [
  { value: '80', label: '健康分 ≥ 80' },
  { value: '70', label: '健康分 ≥ 70' },
  { value: '60', label: '健康分 ≥ 60' },
];

const PENDING_TIP = '还没有这一指标的机器证据（或该指标尚未产出），如实标为「待验证」，不编造默认值。';

function statusColor(status: string): string {
  if (status === 'verified' || status === 'observing') return 'green';
  if (status === 'degraded') return 'orange';
  if (status === 'retired') return 'default';
  return 'red';
}

// 与 internal/selection/engine.go eligibilityReason 的门槛保持一致：
// 只有 verified/observing、证据 passable 且置信度达到 moderate/strong 的方法才进每日选股。
function passesGate(m: MethodCard): boolean {
  if (m.status !== 'verified' && m.status !== 'observing') return false;
  const ev = m.evidence;
  if (!ev || ev.passable !== true) return false;
  return ev.confidence === 'moderate' || ev.confidence === 'strong';
}

function confidenceRank(confidence?: string): number {
  if (confidence === 'strong') return 4;
  if (confidence === 'moderate') return 3;
  if (confidence === 'weak') return 2;
  if (confidence === 'insufficient') return 1;
  if (confidence === 'rejected') return 0;
  return -1;
}

const pct = (v: number) => `${(v * 100).toFixed(2)}%`;

function PendingTag({ tip }: { tip?: string }) {
  return (
    <Tooltip title={tip ?? PENDING_TIP}>
      <Tag style={{ color: 'rgba(0,0,0,0.45)' }}>待验证</Tag>
    </Tooltip>
  );
}

function holdingQuery(
  holdingMin?: string,
  holdingMax?: string,
): { holding_min_days?: number; holding_max_days?: number } {
  const q: { holding_min_days?: number; holding_max_days?: number } = {};
  if (holdingMin !== undefined) q.holding_min_days = Number.parseInt(holdingMin, 10);
  if (holdingMax !== undefined) q.holding_max_days = Number.parseInt(holdingMax, 10);
  return q;
}

/** 把内置方法的真实回测结论如实展示，未通过门槛的一样照实显示。 */
function SeedResultAlert({ result, onDismiss }: { result: MethodSeedResult; onDismiss: () => void }) {
  const allRejected = result.verified === 0;
  return (
    <Alert
      type={allRejected ? 'warning' : 'success'}
      showIcon
      closable
      onClose={onDismiss}
      message={
        allRejected
          ? `内置方法已用真实数据回测：${result.registered} 个已登记，0 个通过证据门槛`
          : `内置方法已用真实数据回测：${result.verified} 个通过证据门槛`
      }
      description={
        <Space direction="vertical" size={4} style={{ display: 'flex' }}>
          <Text type="secondary" style={{ fontSize: 12 }}>
            验证区间 {result.date_start} ~ {result.date_end}，股票池 {result.universe_size} 只（冻结快照{' '}
            {result.snapshot_id}）
          </Text>
          {result.outcomes.map((o) => (
            <Space key={o.key} size={8} wrap>
              <Text strong>{o.name}</Text>
              <Tag color={o.passable ? 'green' : 'red'}>{o.passable ? '通过门槛' : '未通过门槛'}</Tag>
              <Text type="secondary" style={{ fontSize: 12 }}>
                样本外 {o.oos_trades} 笔，收益 {pct(o.oos_return)}，最大回撤 {pct(o.oos_max_drawdown)}
              </Text>
              {o.error && (
                <Text type="danger" style={{ fontSize: 12 }}>
                  {o.error}
                </Text>
              )}
            </Space>
          ))}
          <Text type="secondary" style={{ fontSize: 12 }}>
            这是真实回测结论，不是演示数据。未通过门槛的方法不会进入选股。
          </Text>
        </Space>
      }
    />
  );
}

/** 自动研究批次的机器结论：晋级/拒绝与多重检验预算如实展示。 */
function ResearchResultCard({ result }: { result: MethodResearchResult }) {
  return (
    <Card size="small" title={`最近一轮自动研究 · ${formatDateTime(result.started_at)}`}>
      <Space direction="vertical" size={8} style={{ display: 'flex' }}>
        <Space wrap size={16}>
          <Tag color="blue">股票池 {result.universe_size} 只</Tag>
          <Tag>本批多重检验预算 {result.trials_this_batch}</Tag>
          <Tag>累计预算 {result.trials_cumulative}</Tag>
          <Tag color="green">验证通过 {result.verified}</Tag>
          <Tag color="red">拒绝 {result.rejected}</Tag>
          <Tag>登记 {result.registered}</Tag>
        </Space>
        {result.validation_start && (
          <Text type="secondary" style={{ fontSize: 12 }}>
            保留窗口验证区间 {result.validation_start} ~ {result.validation_end ?? '?'}（快照 {result.snapshot_id}）
          </Text>
        )}
        <Collapse
          size="small"
          items={[
            {
              key: 'outcomes',
              label: `候选明细（${result.outcomes.length} 条）`,
              children: (
                <Space direction="vertical" size={6} style={{ display: 'flex' }}>
                  {result.outcomes.map((o, i) => (
                    <Space key={`${o.template_id}-${i}`} size={8} wrap>
                      <Text strong>{o.template_id}</Text>
                      <Tag color={o.status === 'verified' ? 'green' : o.status.startsWith('skipped') ? 'default' : 'red'}>
                        {o.status}
                      </Tag>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {o.stage}
                        {o.confidence ? ` · 置信度 ${o.confidence}` : ''}
                        {o.oos_trades !== undefined ? ` · OOS ${o.oos_trades} 笔` : ''}
                        {o.oos_return !== undefined ? ` · ${pct(o.oos_return)}` : ''}
                        {o.sharpe_ratio !== undefined ? ` · 夏普 ${o.sharpe_ratio.toFixed(2)}` : ''}
                      </Text>
                      {o.reason && (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {o.reason}
                        </Text>
                      )}
                    </Space>
                  ))}
                </Space>
              ),
            },
          ]}
        />
      </Space>
    </Card>
  );
}

export default function Methods() {
  const navigate = useNavigate();
  const [items, setItems] = useState<MethodCard[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [statuses, setStatuses] = useState<string[]>([]);
  const [market, setMarket] = useState<string>();
  const [universe, setUniverse] = useState<string>();
  const [holdingMin, setHoldingMin] = useState<string>();
  const [holdingMax, setHoldingMax] = useState<string>();
  const [minHealth, setMinHealth] = useState<string>();
  const [minConfidence, setMinConfidence] = useState<string>();
  const [onlyGate, setOnlyGate] = useState(false);
  const [facets, setFacets] = useState<{ markets: string[]; universes: string[] }>({ markets: [], universes: [] });
  const [seeding, setSeeding] = useState(false);
  const [seedResult, setSeedResult] = useState<MethodSeedResult>();

  // 多选：对比 / 用选中方法筛选
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([]);
  const [compareOpen, setCompareOpen] = useState(false);
  const [screening, setScreening] = useState(false);

  // 前向健康（阶段 D）
  const [fhMap, setFhMap] = useState<Map<string, MethodForwardHealth>>(new Map());
  const [fhError, setFhError] = useState('');

  // 自动研究（阶段 A）与拒绝原因分布
  const [research, setResearch] = useState<MethodResearchResult>();
  const [researchRunning, setResearchRunning] = useState(false);
  const [researchError, setResearchError] = useState('');
  const [rejectStats, setRejectStats] = useState<MethodRejectStat[]>([]);

  // 详情抽屉
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [detail, setDetail] = useState<MethodCard>();
  const [audit, setAudit] = useState<MethodAuditEvent[]>();
  const [auditError, setAuditError] = useState('');

  // 反馈
  const [fbOpen, setFbOpen] = useState(false);
  const [fbTarget, setFbTarget] = useState<MethodCard>();
  const [fbUseful, setFbUseful] = useState(true);
  const [fbComment, setFbComment] = useState('');
  const [fbSubmitting, setFbSubmitting] = useState(false);

  const requestSeq = useRef(0);

  const filtersActive =
    statuses.length > 0 || Boolean(market) || Boolean(universe) || Boolean(holdingMin) || Boolean(holdingMax);

  const load = useCallback(async () => {
    const seq = ++requestSeq.current;
    setLoading(true);
    setError('');
    try {
      const res = await api.methodCards({
        status: statuses.length > 0 ? statuses.join(',') : undefined,
        market,
        universe,
        ...holdingQuery(holdingMin, holdingMax),
        limit: 200,
      });
      if (seq !== requestSeq.current) return;
      const list = res.items || [];
      setItems(list);
      setTotal(res.total ?? list.length);
      setFacets((prev) => ({
        markets: Array.from(new Set([...prev.markets, ...list.map((x) => x.market).filter(Boolean)])).sort(),
        universes: Array.from(new Set([...prev.universes, ...list.map((x) => x.universe).filter(Boolean)])).sort(),
      }));
    } catch (e) {
      if (seq !== requestSeq.current) return;
      setError(e instanceof Error ? e.message : '加载失败');
    } finally {
      if (seq === requestSeq.current) setLoading(false);
    }
  }, [statuses, market, universe, holdingMin, holdingMax]);

  const loadForwardHealth = useCallback(async () => {
    setFhError('');
    try {
      const res = await api.methodForwardHealth();
      const map = new Map<string, MethodForwardHealth>();
      for (const it of res.items || []) map.set(it.method_id, it);
      setFhMap(map);
    } catch (e) {
      setFhError(e instanceof Error ? e.message : '读取前向健康失败');
    }
  }, []);

  const loadRejectStats = useCallback(async () => {
    try {
      const res = await api.methodRejectStats();
      setRejectStats(res.items || []);
    } catch {
      // 拒绝原因分布是辅助信息，失败不打扰主列表
      setRejectStats([]);
    }
  }, []);

  useEffect(() => {
    queueMicrotask(() => void load());
  }, [load]);

  useEffect(() => {
    void loadForwardHealth();
    void loadRejectStats();
    api
      .methodResearchLast()
      .then((res) => setResearch(res))
      .catch(() => setResearch(undefined)); // 尚未跑过自动研究时保持空态
  }, [loadForwardHealth, loadRejectStats]);

  const runSeed = useCallback(async () => {
    setSeeding(true);
    setSeedResult(undefined);
    try {
      const res = await api.seedMethods({});
      setSeedResult(res);
      if (res.verified > 0) void message.success(`已登记 ${res.verified} 个通过证据门槛的内置方法`);
      else void message.warning('内置方法已用真实数据回测，但没有方法通过证据门槛');
      await load();
    } catch (e) {
      void message.error(e instanceof Error ? e.message : '载入内置方法失败');
    } finally {
      setSeeding(false);
    }
  }, [load]);

  const runResearch = useCallback(async () => {
    setResearchRunning(true);
    setResearchError('');
    try {
      const res = await api.methodResearchRun({});
      setResearch(res);
      void message.success(`自动研究完成：验证通过 ${res.verified}，拒绝 ${res.rejected}`);
      await Promise.all([load(), loadRejectStats()]);
    } catch (e) {
      setResearchError(e instanceof Error ? e.message : '自动研究失败');
    } finally {
      setResearchRunning(false);
    }
  }, [load, loadRejectStats]);

  const openDetail = useCallback(async (m: MethodCard) => {
    setDetail(m);
    setAudit(undefined);
    setAuditError('');
    setDrawerOpen(true);
    try {
      const [card, events] = await Promise.all([api.methodCard(m.id), api.methodAudit(m.id)]);
      setDetail(card);
      setAudit(events.items || []);
    } catch (e) {
      setAuditError(e instanceof Error ? e.message : '读取详情失败');
      setAudit([]);
    }
  }, []);

  const openFeedback = useCallback((m: MethodCard, useful: boolean) => {
    setFbTarget(m);
    setFbUseful(useful);
    setFbComment('');
    setFbOpen(true);
  }, []);

  const submitFeedback = useCallback(async () => {
    if (!fbTarget) return;
    setFbSubmitting(true);
    try {
      await api.methodFeedback(fbTarget.id, fbUseful, fbComment.trim() || undefined);
      void message.success('反馈已写入审计轨迹，将在后续自动研究中生效');
      setFbOpen(false);
      await openDetail(fbTarget);
    } catch (e) {
      void message.error(e instanceof Error ? e.message : '反馈提交失败');
    } finally {
      setFbSubmitting(false);
    }
  }, [fbTarget, fbUseful, fbComment, openDetail]);

  const clearFilters = useCallback(() => {
    setStatuses([]);
    setMarket(undefined);
    setUniverse(undefined);
    setHoldingMin(undefined);
    setHoldingMax(undefined);
    setMinHealth(undefined);
    setMinConfidence(undefined);
    setOnlyGate(false);
  }, []);

  /** 健康分：前向健康优先（有真实前向样本），其次方法卡自带健康摘要。 */
  const healthScoreOf = useCallback(
    (m: MethodCard): number | undefined => fhMap.get(m.id)?.score ?? m.health?.score,
    [fhMap],
  );

  const visible = useMemo(
    () =>
      items.filter((m) => {
        if (onlyGate && !passesGate(m)) return false;
        if (minHealth !== undefined) {
          const h = healthScoreOf(m);
          if (h === undefined || h < Number.parseInt(minHealth, 10)) return false;
        }
        if (minConfidence !== undefined && confidenceRank(m.evidence?.confidence) < confidenceRank(minConfidence)) {
          return false;
        }
        return true;
      }),
    [items, onlyGate, minHealth, minConfidence, healthScoreOf],
  );

  const selectedCards = useMemo(
    () => items.filter((m) => selectedKeys.includes(m.id)),
    [items, selectedKeys],
  );

  const runSelectionWithSelected = useCallback(async () => {
    if (selectedCards.length === 0) return;
    setScreening(true);
    try {
      const run = await api.selectionRunCreate({ method_ids: selectedCards.map((m) => m.id) });
      void message.success(`选股完成：${run.candidates.length} 个候选`);
      navigate(`/methods/selection/${run.id}`, { state: { run } });
    } catch (e) {
      void message.error(e instanceof Error ? e.message : '选股运行失败');
    } finally {
      setScreening(false);
    }
  }, [selectedCards, navigate]);

  const warnHealth = useMemo(
    () => Array.from(fhMap.values()).filter((h) => h.retired || h.degraded),
    [fhMap],
  );

  const columns: ColumnsType<MethodCard> = [
    {
      title: '方法',
      dataIndex: 'name',
      width: 260,
      fixed: 'left',
      render: (_, m) => (
        <Space direction="vertical" size={2} style={{ display: 'flex' }}>
          <Space size={6} wrap>
            <Text strong>{m.name}</Text>
            <Tag color={statusColor(m.status)}>{m.status}</Tag>
            {passesGate(m) && <Tag color="blue">进入选股</Tag>}
          </Space>
          <Text type="secondary" style={{ fontSize: 12 }} ellipsis>
            {m.entry_summary}
          </Text>
        </Space>
      ),
    },
    {
      title: '置信度',
      dataIndex: ['evidence', 'confidence'],
      width: 110,
      sorter: (a, b) => confidenceRank(a.evidence?.confidence) - confidenceRank(b.evidence?.confidence),
      render: (_, m) =>
        m.evidence ? (
          <Tooltip title={m.evidence.confidence_reason ? `机器原因：${m.evidence.confidence_reason}` : undefined}>
            <Tag color={m.evidence.passable ? 'green' : 'red'}>{m.evidence.confidence}</Tag>
          </Tooltip>
        ) : (
          <PendingTag />
        ),
    },
    {
      title: 'OOS 交易',
      dataIndex: ['evidence', 'oos_trades'],
      width: 90,
      sorter: (a, b) => (a.evidence?.oos_trades ?? -1) - (b.evidence?.oos_trades ?? -1),
      render: (_, m) => (m.evidence ? `${m.evidence.oos_trades} 笔` : <PendingTag />),
    },
    {
      title: '样本外收益',
      dataIndex: ['evidence', 'oos_return'],
      width: 110,
      sorter: (a, b) => (a.evidence?.oos_return ?? -Infinity) - (b.evidence?.oos_return ?? -Infinity),
      render: (_, m) => (m.evidence ? pct(m.evidence.oos_return) : <PendingTag />),
    },
    {
      title: '胜率',
      dataIndex: ['evidence', 'oos_win_rate'],
      width: 90,
      sorter: (a, b) => (a.evidence?.oos_win_rate ?? -Infinity) - (b.evidence?.oos_win_rate ?? -Infinity),
      render: (_, m) => (m.evidence?.oos_win_rate !== undefined ? pct(m.evidence.oos_win_rate) : <PendingTag />),
    },
    {
      title: '最大回撤',
      dataIndex: ['evidence', 'oos_max_drawdown'],
      width: 100,
      sorter: (a, b) => (a.evidence?.oos_max_drawdown ?? -Infinity) - (b.evidence?.oos_max_drawdown ?? -Infinity),
      render: (_, m) => (m.evidence ? pct(m.evidence.oos_max_drawdown) : <PendingTag />),
    },
    {
      title: '夏普',
      dataIndex: ['evidence', 'sharpe_ratio'],
      width: 80,
      sorter: (a, b) => (a.evidence?.sharpe_ratio ?? -Infinity) - (b.evidence?.sharpe_ratio ?? -Infinity),
      render: (_, m) => (m.evidence?.sharpe_ratio !== undefined ? m.evidence.sharpe_ratio.toFixed(2) : <PendingTag />),
    },
    {
      title: '健康分',
      dataIndex: 'health',
      width: 110,
      sorter: (a, b) => (healthScoreOf(a) ?? -Infinity) - (healthScoreOf(b) ?? -Infinity),
      render: (_, m) => {
        const fh = fhMap.get(m.id);
        const score = healthScoreOf(m);
        if (score === undefined) return <PendingTag tip="还没有前向样本，健康分待前向监控产出" />;
        return (
          <Space size={4} wrap>
            <Tag color={score >= 70 ? 'green' : score >= 50 ? 'orange' : 'red'}>{score}</Tag>
            {fh && (
              <Tooltip
                title={`前向样本 ${fh.forward_samples}，执行 ${fh.executed_count} / 拒单 ${fh.rejected_count}${
                  fh.hit_rate !== undefined ? `，胜率 ${pct(fh.hit_rate)}` : ''
                }`}
              >
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {fh.forward_samples} 样本
                </Text>
              </Tooltip>
            )}
            {fh?.degraded && !fh.retired && <Tag color="orange">退化</Tag>}
            {fh?.retired && <Tag>已退役</Tag>}
          </Space>
        );
      },
    },
    {
      title: '持有期',
      dataIndex: 'holding_period',
      width: 110,
      render: (v: string) => v || '-',
    },
    {
      title: '触发频率',
      dataIndex: 'trigger_frequency',
      width: 100,
      render: (v?: string) => v || '-',
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      width: 150,
      render: (v: string) => <Text type="secondary" style={{ fontSize: 12 }}>{formatDateTime(v)}</Text>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 130,
      fixed: 'right',
      render: (_, m) => (
        <Button size="small" icon={<FileSearchOutlined />} onClick={() => void openDetail(m)}>
          详情与反馈
        </Button>
      ),
    },
  ];

  const gatePassed = useMemo(() => visible.filter(passesGate), [visible]);

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      <div>
        <Title level={3}>方法市场</Title>
        <Text type="secondary">
          机器证据指标（置信度 / 样本外交易 / 收益 / 胜率 / 回撤 / 夏普）+ 前向健康分，凭指标自主选择方法再用它筛选股票；
          缺失指标如实标「待验证」。当前 {items.length} 条记录中 {gatePassed.length} 条通过选股门槛。
        </Text>
      </div>

      {error && <Alert type="error" message={error} action={<Button size="small" onClick={() => void load()}>重试</Button>} />}
      {fhError && <Alert type="warning" message={`前向健康读取失败：${fhError}`} />}

      {/* 阶段 A：自动研究入口 + 最近批次结果 */}
      <Card
        size="small"
        title="自动研究"
        extra={
          <Popconfirm
            title="在冻结快照上自动发现并验证一批候选方法？"
            description="模板扫描 → 保留窗口机器验证 → 按证据晋级/拒绝，可能耗时数分钟。"
            okText="开始"
            cancelText="取消"
            onConfirm={() => void runResearch()}
          >
            <Button type="primary" icon={<ThunderboltOutlined />} loading={researchRunning}>
              {researchRunning ? '研究进行中…' : '启动一轮自动研究'}
            </Button>
          </Popconfirm>
        }
      >
        {researchError ? (
          <Alert type="error" showIcon message={researchError} />
        ) : research ? (
          <ResearchResultCard result={research} />
        ) : (
          <Text type="secondary">还没有自动研究记录。系统每 30 分钟自动尝试一轮；也可以手动触发。</Text>
        )}
        {rejectStats.length > 0 && (
          <div style={{ marginTop: 12 }}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              未通过门槛的原因分布（{rejectStats.reduce((s, x) => s + x.count, 0)} 个被拒方法）：
            </Text>
            <Space wrap size={4} style={{ marginTop: 4 }}>
              {rejectStats.map((s) => (
                <Tooltip key={s.reason} title={s.reason}>
                  <Tag>
                    {s.category} × {s.count}
                  </Tag>
                </Tooltip>
              ))}
            </Space>
          </div>
        )}
      </Card>

      {/* 阶段 D：前向健康警示 */}
      {warnHealth.length > 0 && (
        <Alert
          type="warning"
          showIcon
          message={`${warnHealth.length} 个方法前向表现退化或已退役`}
          description={
            <Space wrap size={4}>
              {warnHealth.map((h) => (
                <Tag key={h.method_id} color={h.retired ? 'default' : 'orange'}>
                  {h.name}：{h.retired ? '已退役' : '退化'}（健康分 {h.score}）
                </Tag>
              ))}
            </Space>
          }
        />
      )}

      <Card size="small">
        <Space wrap>
          <Select
            mode="multiple"
            allowClear
            placeholder="状态"
            value={statuses}
            onChange={setStatuses}
            style={{ minWidth: 220, maxWidth: 400 }}
            maxTagCount="responsive"
            options={STATUS_OPTIONS}
          />
          <Select
            allowClear
            showSearch
            placeholder="市场"
            value={market}
            onChange={setMarket}
            style={{ width: 130 }}
            options={facets.markets.map((v) => ({ value: v, label: v }))}
          />
          <Select
            allowClear
            showSearch
            placeholder="股票池 universe"
            value={universe}
            onChange={setUniverse}
            style={{ width: 200 }}
            options={facets.universes.map((v) => ({ value: v, label: v }))}
          />
          <Select
            allowClear
            placeholder="最长持有 ≥"
            value={holdingMin}
            onChange={setHoldingMin}
            style={{ width: 200 }}
            options={HOLDING_MIN_OPTIONS}
          />
          <Select
            allowClear
            placeholder="最短持有 ≤"
            value={holdingMax}
            onChange={setHoldingMax}
            style={{ width: 190 }}
            options={HOLDING_MAX_OPTIONS}
          />
          <Select
            allowClear
            placeholder="置信度下限"
            value={minConfidence}
            onChange={setMinConfidence}
            style={{ width: 170 }}
            options={CONFIDENCE_FILTER_OPTIONS}
          />
          <Select
            allowClear
            placeholder="健康分下限"
            value={minHealth}
            onChange={setMinHealth}
            style={{ width: 150 }}
            options={MIN_HEALTH_OPTIONS}
          />
          <span>
            <Text type="secondary" style={{ fontSize: 12 }}>只看通过门槛</Text>{' '}
            <Switch size="small" checked={onlyGate} onChange={setOnlyGate} />
          </span>
          <Button icon={<ReloadOutlined />} onClick={() => void load()}>
            刷新
          </Button>
          <Button icon={<DownloadOutlined />} loading={seeding} onClick={() => void runSeed()} title="在冻结真实日线上回测并登记内置方法">
            载入内置方法
          </Button>
          {filtersActive && <Button onClick={clearFilters}>清空筛选</Button>}
        </Space>
      </Card>

      {seedResult && <SeedResultAlert result={seedResult} onDismiss={() => setSeedResult(undefined)} />}

      {/* 阶段 C：用选中方法筛选 */}
      <Card size="small">
        <Space wrap>
          <Text strong>已选 {selectedKeys.length} 个方法</Text>
          <Button
            type="primary"
            icon={<ExperimentOutlined />}
            disabled={selectedKeys.length === 0}
            loading={screening}
            onClick={() => void runSelectionWithSelected()}
            title="用勾选的方法在最新冻结快照上运行一次选股"
          >
            用选中方法筛选
          </Button>
          <Button disabled={selectedKeys.length < 2} onClick={() => setCompareOpen(true)}>
            对比选中方法
          </Button>
          {selectedKeys.length > 0 && (
            <Button onClick={() => setSelectedKeys([])}>清空选择</Button>
          )}
          <Text type="secondary" style={{ fontSize: 12 }}>
            未过门槛的方法也可参与运行，后端会如实给出排除原因（如 evidence_below_gate）。
          </Text>
        </Space>
      </Card>

      <Table<MethodCard>
        rowKey="id"
        size="middle"
        loading={loading}
        columns={columns}
        dataSource={visible}
        scroll={{ x: 1500 }}
        pagination={{ pageSize: 20, showTotal: () => `服务端共 ${total} 条记录${filtersActive ? '（已按服务端条件筛选）' : ''}` }}
        rowSelection={{ selectedRowKeys: selectedKeys, onChange: (keys) => setSelectedKeys(keys) }}
        locale={{
          emptyText: (
            <Empty description={filtersActive || onlyGate ? '当前筛选条件下没有方法' : '尚无已登记方法'}>
              {filtersActive || onlyGate ? (
                <Button onClick={clearFilters}>清空筛选</Button>
              ) : (
                <Space>
                  <Button type="primary" icon={<ThunderboltOutlined />} loading={researchRunning} onClick={() => void runResearch()}>
                    启动自动研究
                  </Button>
                  <Button icon={<DownloadOutlined />} loading={seeding} onClick={() => void runSeed()}>
                    载入内置方法
                  </Button>
                </Space>
              )}
            </Empty>
          ),
        }}
      />

      {/* 多选对比 */}
      <Modal
        title="方法对比"
        open={compareOpen}
        onCancel={() => setCompareOpen(false)}
        footer={null}
        width={Math.min(280 * Math.max(selectedCards.length, 1) + 80, 1100)}
      >
        <Space direction="vertical" size={12} style={{ display: 'flex' }}>
          <Space wrap>
            {selectedCards.map((m) => (
              <Card
                key={m.id}
                size="small"
                title={
                  <Space size={6} wrap>
                    <span>{m.name}</span>
                    <Tag color={statusColor(m.status)}>{m.status}</Tag>
                  </Space>
                }
                style={{ width: 260 }}
              >
                <Descriptions size="small" column={1} colon={false}>
                  <Descriptions.Item label="置信度">
                    {m.evidence ? <Tag color={m.evidence.passable ? 'green' : 'red'}>{m.evidence.confidence}</Tag> : <PendingTag />}
                  </Descriptions.Item>
                  <Descriptions.Item label="OOS 交易">{m.evidence ? `${m.evidence.oos_trades} 笔` : <PendingTag />}</Descriptions.Item>
                  <Descriptions.Item label="样本外收益">{m.evidence ? pct(m.evidence.oos_return) : <PendingTag />}</Descriptions.Item>
                  <Descriptions.Item label="胜率">
                    {m.evidence?.oos_win_rate !== undefined ? pct(m.evidence.oos_win_rate) : <PendingTag />}
                  </Descriptions.Item>
                  <Descriptions.Item label="最大回撤">{m.evidence ? pct(m.evidence.oos_max_drawdown) : <PendingTag />}</Descriptions.Item>
                  <Descriptions.Item label="夏普">
                    {m.evidence?.sharpe_ratio !== undefined ? m.evidence.sharpe_ratio.toFixed(2) : <PendingTag />}
                  </Descriptions.Item>
                  <Descriptions.Item label="健康分">
                    {healthScoreOf(m) !== undefined ? healthScoreOf(m) : <PendingTag tip="还没有前向样本" />}
                  </Descriptions.Item>
                  <Descriptions.Item label="持有期">{m.holding_period || '-'}</Descriptions.Item>
                  <Descriptions.Item label="入场">{m.entry_summary}</Descriptions.Item>
                  <Descriptions.Item label="退出">{m.exit_summary}</Descriptions.Item>
                </Descriptions>
              </Card>
            ))}
          </Space>
          <Text type="secondary" style={{ fontSize: 12 }}>
            对比只呈现机器证据与登记信息，不代替你判断；「待验证」表示该指标暂无证据。
          </Text>
        </Space>
      </Modal>

      <Drawer
        title={detail ? detail.name : '方法详情'}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={600}
        destroyOnHidden
      >
        {!detail ? (
          <Spin />
        ) : (
          <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
            <Descriptions bordered size="small" column={1}>
              <Descriptions.Item label="状态">
                <Tag color={statusColor(detail.status)}>{detail.status}</Tag>
                {passesGate(detail) && <Tag color="blue">进入选股</Tag>}
                {fhMap.get(detail.id)?.degraded && <Tag color="orange">前向退化</Tag>}
                {fhMap.get(detail.id)?.retired && <Tag>已退役</Tag>}
              </Descriptions.Item>
              <Descriptions.Item label="市场 / 股票池">
                {detail.market} / {detail.universe}
              </Descriptions.Item>
              <Descriptions.Item label="持有期">{detail.holding_period || '-'}</Descriptions.Item>
              <Descriptions.Item label="触发频率">{detail.trigger_frequency || '-'}</Descriptions.Item>
              <Descriptions.Item label="入场">{detail.entry_summary}</Descriptions.Item>
              <Descriptions.Item label="退出">{detail.exit_summary}</Descriptions.Item>
              <Descriptions.Item label="失效条件">
                {detail.invalidations?.length ? detail.invalidations.join('；') : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="家族 / 变体">
                <Text code style={{ fontSize: 11, wordBreak: 'break-all' }}>
                  {detail.family_id}
                </Text>
                <br />
                <Text code style={{ fontSize: 11, wordBreak: 'break-all' }}>
                  {detail.variant_id}
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label="方法 ID">
                <Text code copyable style={{ fontSize: 11 }}>
                  {detail.id}
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label="更新时间">{formatDateTime(detail.updated_at)}</Descriptions.Item>
            </Descriptions>

            {detail.evidence ? (
              <Card size="small" title="样本外证据（机器验证）">
                <Descriptions size="small" column={1} colon={false}>
                  <Descriptions.Item label="置信度">
                    <Tag color={detail.evidence.passable ? 'green' : 'red'}>{detail.evidence.confidence}</Tag>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {detail.evidence.passable ? '证据可用' : '证据不足，不进选股'}
                    </Text>
                  </Descriptions.Item>
                  {detail.evidence.confidence_reason && (
                    <Descriptions.Item label="机器原因">
                      <Text code style={{ fontSize: 11 }}>{detail.evidence.confidence_reason}</Text>
                    </Descriptions.Item>
                  )}
                  <Descriptions.Item label="样本外交易">{detail.evidence.oos_trades} 笔</Descriptions.Item>
                  <Descriptions.Item label="样本外收益">{pct(detail.evidence.oos_return)}</Descriptions.Item>
                  <Descriptions.Item label="样本外胜率">
                    {detail.evidence.oos_win_rate !== undefined ? pct(detail.evidence.oos_win_rate) : '待验证'}
                  </Descriptions.Item>
                  <Descriptions.Item label="样本外最大回撤">{pct(detail.evidence.oos_max_drawdown)}</Descriptions.Item>
                  <Descriptions.Item label="夏普 / 索提诺">
                    {detail.evidence.sharpe_ratio !== undefined
                      ? `${detail.evidence.sharpe_ratio.toFixed(2)} / ${
                          detail.evidence.sortino_ratio !== undefined ? detail.evidence.sortino_ratio.toFixed(2) : '待验证'
                        }`
                      : '待验证'}
                  </Descriptions.Item>
                  <Descriptions.Item label="冻结快照">
                    <Text code style={{ fontSize: 11, wordBreak: 'break-all' }}>
                      {detail.evidence.snapshot_id || '-'}
                    </Text>
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            ) : (
              <Alert type="info" showIcon message="该方法还没有证据包，指标均为「待验证」" />
            )}

            {/* 阶段 D：前向健康 */}
            <Card size="small" title="前向健康（监控反馈）">
              {(() => {
                const fh = fhMap.get(detail.id);
                if (!fh) return <Text type="secondary">还没有前向样本：方法进入每日选股并产生前向信号后，这里会给出健康分。</Text>;
                return (
                  <Descriptions size="small" column={1} colon={false}>
                    <Descriptions.Item label="健康分">
                      <Tag color={fh.score >= 70 ? 'green' : fh.score >= 50 ? 'orange' : 'red'}>{fh.score}</Tag>
                      <Text type="secondary" style={{ fontSize: 12 }}>截至 {formatDateTime(fh.as_of)}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label="前向样本">{fh.forward_samples}（执行 {fh.executed_count} / 拒单 {fh.rejected_count}）</Descriptions.Item>
                    {fh.hit_rate !== undefined && (
                      <Descriptions.Item label="前向胜率">{pct(fh.hit_rate)}</Descriptions.Item>
                    )}
                    {fh.avg_return !== undefined && (
                      <Descriptions.Item label="前向均收益">{pct(fh.avg_return)}</Descriptions.Item>
                    )}
                    <Descriptions.Item label="警示">
                      {fh.execution_deviation && <Tag color="orange">执行偏差</Tag>}
                      {fh.decay && <Tag color="orange">收益衰减</Tag>}
                      {fh.drift && <Tag color="orange">行为漂移</Tag>}
                      {fh.consecutive_severe > 0 && <Tag color="red">连续严重亏损 {fh.consecutive_severe}</Tag>}
                      {!fh.execution_deviation && !fh.decay && !fh.drift && fh.consecutive_severe === 0 && (
                        <Tag color="green">无警示</Tag>
                      )}
                    </Descriptions.Item>
                  </Descriptions>
                );
              })()}
            </Card>

            {/* 反馈：写入审计并反哺自动研究 */}
            <Card size="small" title="使用反馈">
              <Space wrap>
                <Button
                  size="small"
                  icon={<CheckOutlined />}
                  onClick={() => openFeedback(detail, true)}
                >
                  好用
                </Button>
                <Button
                  size="small"
                  icon={<CloseOutlined />}
                  danger
                  onClick={() => openFeedback(detail, false)}
                >
                  不好用
                </Button>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  连续「不好用」的家族会被自动研究跳过。
                </Text>
              </Space>
            </Card>

            <Card size="small" title="状态审计轨迹">
              {audit === undefined ? (
                <Spin size="small" />
              ) : auditError ? (
                <Alert type="error" message={auditError} />
              ) : audit.length === 0 ? (
                <Text type="secondary">暂无审计事件</Text>
              ) : (
                <Timeline
                  items={audit.map((e) => ({
                    key: e.id,
                    color: statusColor(e.to),
                    children: (
                      <Space direction="vertical" size={2} style={{ display: 'flex' }}>
                        <Space size={8} wrap>
                          <Text code>{e.from || '-'}</Text>
                          <span>→</span>
                          <Tag color={statusColor(e.to)}>{e.to}</Tag>
                          <Text type="secondary" style={{ fontSize: 12 }}>
                            {e.action}
                          </Text>
                          {e.automatic && <Tag>自动</Tag>}
                        </Space>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {e.reason || '（无原因）'}
                        </Text>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {e.actor} · {formatDateTime(e.created_at)}
                        </Text>
                      </Space>
                    ),
                  }))}
                />
              )}
            </Card>
          </Space>
        )}
      </Drawer>

      {/* 反馈弹窗 */}
      <Modal
        title={`反馈：${fbTarget?.name ?? ''}`}
        open={fbOpen}
        okText={fbUseful ? '提交「好用」' : '提交「不好用」'}
        onOk={() => void submitFeedback()}
        confirmLoading={fbSubmitting}
        onCancel={() => setFbOpen(false)}
        destroyOnHidden
      >
        <Space direction="vertical" size={8} style={{ display: 'flex' }}>
          <Text>你觉得该方法{fbUseful ? '好用' : '不好用'}，可选填一句原因：</Text>
          <Input.TextArea
            rows={3}
            value={fbComment}
            onChange={(e) => setFbComment(e.target.value)}
            placeholder="例如：信号太频繁 / 近一个月失效（选填）"
          />
          <Text type="secondary" style={{ fontSize: 12 }}>
            反馈写入方法审计轨迹（actor=user），负反馈为主的家族会被自动研究跳过。
          </Text>
        </Space>
      </Modal>
    </Space>
  );
}
