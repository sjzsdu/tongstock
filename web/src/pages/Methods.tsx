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
  type FactorEval,
  type FactorPickEntry,
  type FactorPickRun,
  type FactorResearchResult,
  type FactorTopPick,
  type MethodAuditEvent,
  type MethodCard,
  type MethodForwardHealth,
  type MethodRejectStat,
  type MethodResearchOutcome,
  type MethodResearchResult,
  type MethodResearchStatus,
  type MethodSeedResult,
  TongStockAPIError,
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

/** 把机器可读的拒绝原因码翻译成用户能懂的一句话。 */
function researchReasonText(reason?: string): string {
  if (!reason) return '未通过证据门槛';
  if (reason.startsWith('hard_blocker:ss-trades') || reason === 'insufficient_oos_trades') return '样本外交易数不足，无法区分运气与规律';
  if (reason.startsWith('hard_blocker:bl-underperform')) return '跑输「等权买入并持有整个股票池」基准';
  if (reason === 'multiple_testing_not_significant') return '与同期随机模板相比优势不显著（多重检验校正后）';
  if (reason === 'oos_sharpe_below_threshold') return '样本外风险调整后收益过低';
  if (reason === 'oos_max_drawdown_above_threshold') return '样本外最大回撤过大';
  return reason;
}

/** 候选排序键：交易数优先（有统计意义的才谈胜率），其次胜率，最后收益。 */
function candidateEvidenceScore(o: MethodResearchOutcome): number {
  return (o.oos_trades ?? 0) * 1_000_000 + (o.oos_win_rate ?? 0) * 1_000 + (o.oos_return ?? 0);
}

/**
 * 自动研究结果卡：答案优先。
 * 用户关心的是「哪些方法能筛出上涨股票」：先给本轮结论与样本外胜率最高的
 * 候选榜单（含被拒原因的人话解释），原始明细折叠在最后供审计。
 */
function ResearchResultCard({ result }: { result: MethodResearchResult }) {
  const withTrades = result.outcomes.filter((o) => (o.oos_trades ?? 0) > 0);
  const ranked = [...withTrades].sort((a, b) => candidateEvidenceScore(b) - candidateEvidenceScore(a));
  const noTradeRejected = result.outcomes.filter((o) => (o.oos_trades ?? 0) === 0 && o.status === 'rejected').length;
  const passed = result.verified > 0;

  return (
    <Card size="small" title={`最近一轮自动研究 · ${formatDateTime(result.started_at)}`}>
      <Space direction="vertical" size={8} style={{ display: 'flex' }}>
        <Alert
          type={passed ? 'success' : ranked.length > 0 ? 'info' : 'warning'}
          showIcon
          message={
            passed
              ? `本轮 ${result.verified} 个候选通过全部证据门槛，已晋级为可用方法（见下方列表）`
              : ranked.length > 0
                ? '本轮没有候选通过全部证据门槛，但样本外表现如下，供参考'
                : '本轮所有候选在样本外窗口几乎没有触发交易，不构成可用方法'
          }
          description={
            !passed && ranked.length > 0 ? (
              <Space direction="vertical" size={6} style={{ display: 'flex' }}>
                {ranked.slice(0, 3).map((o, i) => (
                  <Text key={`${o.template_id}-${i}`} style={{ fontSize: 12 }}>
                    {i + 1}. <Text strong>{o.template_id}</Text>
                    {' '}—— 样本外胜率 <Text strong>{pct(o.oos_win_rate ?? 0)}</Text>
                    （{o.oos_trades} 笔交易，收益 {pct(o.oos_return ?? 0)}），被拒原因：
                    {researchReasonText(o.reason)}
                  </Text>
                ))}
                <Text type="secondary" style={{ fontSize: 12 }}>
                  为什么胜率不低还被拒：方法必须同时跑赢「等权买入并持有整个股票池」——
                  只挑出会涨的股票但整体市场都在涨，这样的方法没有选股价值。这是可信度红线，不会放宽。
                </Text>
              </Space>
            ) : undefined
          }
        />
        <Space wrap size={16}>
          <Tag color="blue">股票池 {result.universe_size} 只</Tag>
          <Tag color="green">验证通过 {result.verified}</Tag>
          <Tag color="red">拒绝 {result.rejected}</Tag>
          {noTradeRejected > 0 && <Tag>样本外无交易 {noTradeRejected}</Tag>}
          <Tag>本批多重检验预算 {result.trials_this_batch}</Tag>
          <Tag>累计预算 {result.trials_cumulative}</Tag>
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
              label: `候选明细（${result.outcomes.length} 条，按样本外胜率与交易数排序）`,
              children: (
                <Space direction="vertical" size={6} style={{ display: 'flex' }}>
                  {[...result.outcomes]
                    .sort((a, b) => candidateEvidenceScore(b) - candidateEvidenceScore(a))
                    .map((o, i) => (
                      <Space key={`${o.template_id}-${i}`} size={8} wrap>
                        <Text strong>{o.template_id}</Text>
                        <Tag color={o.status === 'verified' ? 'green' : o.status.startsWith('skipped') ? 'default' : 'red'}>
                          {o.status}
                        </Tag>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {o.oos_trades !== undefined ? `OOS ${o.oos_trades} 笔` : ''}
                          {o.oos_win_rate !== undefined ? ` · 胜率 ${pct(o.oos_win_rate)}` : ''}
                          {o.oos_return !== undefined ? ` · ${pct(o.oos_return)}` : ''}
                          {o.sharpe_ratio !== undefined ? ` · 夏普 ${o.sharpe_ratio.toFixed(2)}` : ''}
                          {o.reason ? ` · ${researchReasonText(o.reason)}` : ''}
                        </Text>
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

/**
 * 因子研究卡：把选股问题重述为「预测未来 N 日收益的截面排序」。
 * 答案优先：先给结论 Note 与 Top 名单（含各因子贡献分解，保持「为何选它」可解释），
 * 因子级 IC 证据折叠在下层供审计；不显著的因子如实标注，绝不包装成必涨名单。
 */function FactorResearchCard() {
  const [result, setResult] = useState<FactorResearchResult>();
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');
  const [loaded, setLoaded] = useState(false);
  // write_to_selection=true：批次完成后显著因子 TopN 额外落库为因子通道产出
  // （factor_pick_run 表，pick-<截面日期> 幂等），经「因子名单」卡片可复核。
  const [writeToSelection, setWriteToSelection] = useState(true);

  const loadLast = useCallback(async () => {
    const res = await api.factorResearchLast();
    if (!('status' in res)) {
      setResult(res);
      setRunning(Boolean(res.running));
      return Boolean(res.running);
    }
    setResult(undefined);
    setRunning('running' in res ? Boolean(res.running) : false);
    return false;
  }, []);

  useEffect(() => {
    loadLast()
      .then(() => setLoaded(true))
      .catch(() => setLoaded(true));
  }, [loadLast]);

  // 轮询等待异步批次：run 只是启动，结果经 /last 获取。
  useEffect(() => {
    if (!running) return;
    const timer = setInterval(() => {
      void loadLast().catch(() => undefined);
    }, 2000);
    return () => clearInterval(timer);
  }, [running, loadLast]);

  const runResearch = useCallback(async () => {
    setRunning(true);
    setError('');
    try {
      // 异步启动：立即返回 started 包封，结果经 /last 轮询（useEffect 上面已挂）。
      await api.factorResearchRun({ write_to_selection: writeToSelection });
    } catch (e) {
      setError(e instanceof Error ? e.message : '因子研究启动失败');
      setRunning(false);
    }
  }, [writeToSelection]);

  const factorColumns: ColumnsType<FactorEval> = [
    {
      title: '因子',
      dataIndex: 'name',
      render: (_v, f) => (
        <Tooltip title={f.description}>
          <Text strong>{f.name}</Text>
        </Tooltip>
      ),
    },
    {
      title: '数据方向',
      dataIndex: 'direction',
      width: 110,
      render: (d: number, f) => (
        <Tooltip
          title={`先验假设：${f.prior > 0 ? '值大领涨' : '值小领涨'}；方向由数据决定${
            f.prior !== 0 && d !== f.prior ? '（与先验相反，如实反向）' : '（与先验一致）'
          }`}
        >
          <span>{d > 0 ? '值大 → 领涨' : d < 0 ? '值小 → 领涨' : '—'}</span>
        </Tooltip>
      ),
    },
    {
      title: '截面（独立）',
      dataIndex: 'sections',
      width: 120,
      render: (_v: number, f) => (
        <Tooltip title={`独立截面 = 去重叠（步长=持有期）后 t 统计量的真实样本量，避免相邻前向窗口重叠导致显著性高估`}>
          <span>
            {f.sections}（{f.effective_sections}）
          </span>
        </Tooltip>
      ),
    },
    { title: 'MeanIC', dataIndex: 'mean_ic', width: 90, render: (v: number) => v.toFixed(3) },
    { title: 'ICIR', dataIndex: 'icir', width: 80, render: (v: number) => v.toFixed(2) },
    { title: 't 统计', dataIndex: 't_stat', width: 90, render: (v: number) => v.toFixed(2) },
    {
      title: '预测力',
      dataIndex: 'significant',
      width: 100,
      render: (sig: boolean, f) =>
        sig ? (
          <Tag color="green">显著</Tag>
        ) : (
          <Tooltip title={`参考线：|t| ≥ 2 且 |MeanIC| ≥ 0.03（t 用去重叠独立截面计算，实际 t=${f.t_stat.toFixed(2)}）`}>
            <Tag>不显著</Tag>
          </Tooltip>
        ),
    },
  ];

  const pickColumns: ColumnsType<FactorTopPick> = [
    { title: '#', key: 'rank', width: 50, render: (_v, _r, i) => i + 1 },
    { title: '代码', dataIndex: 'code', width: 90 },
    { title: '组合分', dataIndex: 'score', width: 90, render: (v: number) => v.toFixed(3) },
    {
      title: '得分构成（各合格因子的贡献）',
      dataIndex: 'contributions',
      render: (c?: Record<string, number>) =>
        c ? (
          <Space wrap size={4}>
            {Object.entries(c).map(([k, v]) => (
              <Tag key={k}>
                {k} {v >= 0 ? '+' : ''}
                {v.toFixed(2)}
              </Tag>
            ))}
          </Space>
        ) : (
          '—'
        ),
    },
  ];

  return (
    <Card
      size="small"
      title="因子研究 · 截面排序预测"
      extra={
        <Space size={8}>
          <Tooltip title="完成后把显著因子 TopN 名单落库为因子通道产出（按截面日期幂等，可追溯复核）">
            <span>
              <Switch size="small" checked={writeToSelection} onChange={setWriteToSelection} />
              <Text type="secondary" style={{ fontSize: 12, marginLeft: 4 }}>名单入库</Text>
            </span>
          </Tooltip>
          <Button icon={<ExperimentOutlined />} loading={running} onClick={() => void runResearch()} disabled={running}>
            {running ? '研究中…' : '运行因子研究'}
          </Button>
        </Space>
      }
    >
      <Space direction="vertical" size={8} style={{ display: 'flex' }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          在冻结快照的全部股票上，逐日计算动量/反转、量比换手、价格水平、波动等可解释因子的截面值，
          用 RankIC/t 统计量度量每个因子「预测未来 5 日收益」的能力（t 用去重叠独立截面计算），
          再把显著因子按 |IC| 加权成组合分（方向由数据决定，不写死先验），
          输出最后截面日得分最高的 Top 30。这是研究证据，不直接生成买入指令。
        </Text>
        {error ? (
          <Alert
            type="error"
            showIcon
            message={error}
            description={
              /frozen snapshot|universe/i.test(error) ? (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  因子研究需要一个股票池 ≥ 5 只的多股票冻结快照；让 AI 用多只股票研究一次（会自动冻结快照）后再试。
                </Text>
              ) : undefined
            }
          />
        ) : running && !result ? (
          <Space size={8}>
            <Spin size="small" />
            <Text type="secondary">因子研究批次运行中（全市场一轮可达分钟级），结果出来后自动刷新…</Text>
          </Space>
        ) : result ? (
          <>
            {result.stale_days > 14 && (
              <Alert
                type="warning"
                showIcon
                message={`快照数据止于 ${result.snapshot_date_end ?? result.last_date ?? '?'}（距今 ${result.stale_days} 天）——截面排序基于过时行情`}
                description="请先更新行情并冻结新的多股票快照，再跑因子研究；过时快照产出的 Top 名单没有预测意义。"
              />
            )}
            <Alert
              type={result.factors.some((f) => f.significant) ? 'info' : 'warning'}
              showIcon
              message={result.note}
              description={
                <Space wrap size={4} style={{ marginTop: 4 }}>
                  <Tag color="blue">股票池 {result.codes} 只</Tag>
                  <Tag>有效截面 {result.sections}</Tag>
                  <Tag>持有期 {result.horizon_days} 天</Tag>
                  {result.last_date && <Tag>截面日 {result.last_date}</Tag>}
                </Space>
              }
            />
            {result.top_picks.length > 0 && (
              <Table
                size="small"
                columns={pickColumns}
                dataSource={result.top_picks}
                rowKey="code"
                pagination={false}
              />
            )}
            <Collapse
              size="small"
              items={[
                {
                  key: 'factors',
                  label: `因子预测力明细（${result.factors.length} 个因子，按显著性与 |t| 排序）`,
                  children: (
                    <Table
                      size="small"
                      columns={factorColumns}
                      dataSource={result.factors}
                      rowKey="key"
                      pagination={false}
                    />
                  ),
                },
              ]}
            />
          </>
        ) : (
          <Text type="secondary">
            {loaded ? '还没有因子研究记录。点击「运行因子研究」用冻结快照跑一轮。' : '正在读取最近一次研究…'}
          </Text>
        )}
      </Space>
    </Card>
  );
}

/**
 * 因子名单卡：read-only 查看 write_to_selection=true 落库的因子通道产出。
 * 这是最小生产闭环的持久化证据：TopN 名单按截面日期幂等更新，永远可追溯
 * 「那天为什么是这些股票」——因子评估快照与贡献分解随行存档。
 */
function FactorPicksCard() {
  const [pickRun, setPickRun] = useState<FactorPickRun>();
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    const res = await api.factorPicksLast();
    setPickRun('status' in res ? undefined : res);
    setLoaded(true);
  }, []);

  useEffect(() => {
    void load().catch(() => setLoaded(true));
  }, [load]);

  const columns: ColumnsType<FactorPickEntry> = [
    { title: '#', key: 'rank', width: 50, render: (_v, _r, i) => i + 1 },
    { title: '代码', dataIndex: 'code', width: 90 },
    { title: '组合分', dataIndex: 'score', width: 90, render: (v: number) => v.toFixed(3) },
    {
      title: '得分构成（各合格因子的贡献）',
      dataIndex: 'contributions',
      render: (c?: Record<string, number>) =>
        c ? (
          <Space wrap size={4}>
            {Object.entries(c).map(([k, v]) => (
              <Tag key={k}>
                {k} {v >= 0 ? '+' : ''}
                {v.toFixed(2)}
              </Tag>
            ))}
          </Space>
        ) : (
          '—'
        ),
    },
  ];

  return (
    <Card
      size="small"
      title="因子名单 · 落库观察名单"
      extra={
        <Button icon={<ReloadOutlined />} size="small" onClick={() => void load().catch(() => undefined)}>
          刷新
        </Button>
      }
    >
      {pickRun ? (
        <Space direction="vertical" size={8} style={{ display: 'flex' }}>
          <Space wrap size={4}>
            <Tag color="geekblue">run {pickRun.run_id}</Tag>
            <Tag>截面日 {pickRun.as_of}</Tag>
            {pickRun.snapshot_date_end && <Tag>数据止于 {pickRun.snapshot_date_end}</Tag>}
            <Tag color={pickRun.stale_days > 14 ? 'volcano' : 'default'}>距今天数 {pickRun.stale_days}</Tag>
            <Tag>Top {pickRun.picks.length}</Tag>
          </Space>
          <Text type="secondary" style={{ fontSize: 12 }}>{pickRun.note}</Text>
          {pickRun.stale_days > 14 && (
            <Alert
              type="warning"
              showIcon
              message={`名单基于 ${pickRun.as_of} 的截面（距今 ${pickRun.stale_days} 天）——过旧，仅供追溯；动态使用前请先跑一轮新快照研究并勾选「名单入库」`}
            />
          )}
          <Collapse
            size="small"
            items={[
              {
                key: 'factors',
                label: `落库时因子评估快照（${pickRun.factors_snapshot.length} 个）`,
                children: <Text type="secondary" style={{ fontSize: 12 }}>{pickRun.factors_snapshot.map((f) => `${f.name}${f.significant ? '✓' : '·'}(IC ${(f.mean_ic >= 0 ? '+' : '') + f.mean_ic.toFixed(3)})`).join('　')}</Text>,
              },
            ]}
          />
          <Table size="small" columns={columns} dataSource={pickRun.picks} rowKey="code" pagination={false} />
        </Space>
      ) : (
        <Text type="secondary">
          {loaded
            ? '还没有落库名单。在上方因子研究卡勾选「名单入库」跑一轮，显著因子的 TopN 名单会按截面日存在这里。'
            : '正在读取因子名单…'}
        </Text>
      )}
    </Card>
  );
}

export default function Methods() {
  const navigate = useNavigate();
  const [items, setItems] = useState<MethodCard[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  // 默认只展示「用户可用的方法」（已晋级/候选中）：rejected 是研究审计痕迹，
  // 整批被拒时满屏 rejected 对选股毫无价值；想看诊断可手动勾选 rejected。
  const [statuses, setStatuses] = useState<string[]>(['verified', 'observing', 'degraded', 'candidate']);
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
  const [researchStatus, setResearchStatus] = useState<MethodResearchStatus>();
  // 每秒跳动一次，让运行中的批次显示实时「已运行时长」而不是冻结的开始时间。
  const [nowTick, setNowTick] = useState(() => Date.now());
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
        // 后端 GET /api/methods 校验 limit ∈ [1,100]（method_registry_handlers.go），
        // 超过 100 会被整页 400；数量靠筛选条件缩小范围。
        limit: 100,
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
    // last 端点对「从未完成过」返回 200 空结构（status=no_completed_batch），
    // 不当作结果展示；运行中信息由 status 端点负责。
    api
      .methodResearchLast()
      .then((res) => setResearch(res.status === 'no_completed_batch' ? undefined : res))
      .catch(() => setResearch(undefined));
    api
      .methodResearchStatus()
      .then(setResearchStatus)
      .catch(() => setResearchStatus(undefined)); // 启动调度器可能已在跑一轮，必须可见
  }, [loadForwardHealth, loadRejectStats]);

  // 批次运行中每 15 秒轮询；结束后自动拉取最新结果与列表（覆盖启动调度那一轮）。
  useEffect(() => {
    if (!researchStatus?.running) return;
    const timer = setInterval(() => {
      void api
        .methodResearchStatus()
        .then((st) => {
          setResearchStatus(st);
          if (!st.running) {
            void load();
            void loadRejectStats();
            api
              .methodResearchLast()
              .then((res) => setResearch(res.status === 'no_completed_batch' ? undefined : res))
              .catch(() => setResearch(undefined));
          }
        })
        .catch(() => undefined);
    }, 15000);
    return () => clearInterval(timer);
  }, [researchStatus?.running, load, loadRejectStats]);

  // 批次运行中的实时进度：后端每 15 秒上报阶段，本地每秒补已运行时长，
  // 让用户在几十分钟的批次里能看到「卡在哪一步、跑了多久」。
  useEffect(() => {
    if (!researchStatus?.running) return;
    const timer = setInterval(() => setNowTick(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [researchStatus?.running]);

  const researchProgressText = useCallback((st: MethodResearchStatus): string[] => {
    const lines: string[] = [];
    if (st.running_since) {
      const elapsed = Math.max(0, Math.floor((nowTick - new Date(st.running_since).getTime()) / 1000));
      const h = Math.floor(elapsed / 3600);
      const m = Math.floor((elapsed % 3600) / 60);
      const s = elapsed % 60;
      const clock = h > 0 ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}` : `${m}:${String(s).padStart(2, '0')}`;
      lines.push(`已运行 ${clock}`);
    }
    const phase =
      st.phase === 'discovery'
        ? '规律发现：在冻结快照上逐只扫描模板'
        : st.phase === 'validation'
          ? '样本外验证：在保留窗口上回测候选'
          : '准备中：解析快照与股票池';
    lines.push(`阶段：${phase}`);
    const p = st.progress;
    if (p) {
      const parts: string[] = [];
      if (p.universe_size) parts.push(`股票池 ${p.universe_size} 只`);
      if (p.hold_days) parts.push(`持有期 ${p.hold_days} 天`);
      if (st.phase === 'discovery' && p.discovery_codes_total) {
        parts.push(`扫描进度 ${p.discovery_codes_done ?? 0}/${p.discovery_codes_total}`);
      }
      if (p.total_candidates) parts.push(`待验证候选 ${p.candidates_done ?? 0}/${p.total_candidates}`);
      parts.push(`已通过 ${p.verified} / 已拒绝 ${p.rejected}`);
      lines.push(parts.join(' · '));
    }
    return lines;
  }, [nowTick]);

  const runSeed = useCallback(async () => {
    setSeeding(true);
    setSeedResult(undefined);
    try {
      const res = await api.seedMethods({});
      setSeedResult(res);
      const skipped = res.outcomes.filter((o) => o.status === 'skipped_registered').length;
      if (res.verified > 0) void message.success(`已登记 ${res.verified} 个通过证据门槛的内置方法`);
      else if (skipped > 0 && skipped === res.outcomes.length)
        void message.info('内置方法此前已载入过（同内容已登记），无需重复载入');
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
      // 异步启动：批次在后台跑（一轮可能数十分钟）。乐观置 running 触发既有轮询，
      // 并立即拉一次真实状态补 running_since；结束后轮询自动展示结果/列表/拒绝统计。
      setResearchStatus((prev) => ({ ...(prev ?? {}), running: true }));
      void message.info(
        res.started
          ? '研究批次已启动，将在后台运行，完成后自动展示结果'
          : '研究批次已在运行，完成后自动展示结果',
      );
      api
        .methodResearchStatus()
        .then(setResearchStatus)
        .catch(() => undefined);
    } catch (e) {
      if (e instanceof TongStockAPIError && e.code === 'method_research_busy') {
        // 启动时调度器已在跑：转为跟踪状态，完成后自动展示结果，不当作错误。
        void message.info('已有研究批次在运行，完成后自动展示结果');
        try {
          setResearchStatus(await api.methodResearchStatus());
        } catch {
          // 状态接口失败时保持原状，轮询会在下次打开页面时恢复
        }
        return;
      }
      setResearchError(e instanceof Error ? e.message : '自动研究失败');
    } finally {
      setResearchRunning(false);
    }
  }, []);

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
            description="模板扫描 → 保留窗口机器验证 → 按证据晋级/拒绝，可能耗时数十分钟，期间可离开页面。"
            okText="开始"
            cancelText="取消"
            onConfirm={() => void runResearch()}
          >
            <Button
              type="primary"
              icon={<ThunderboltOutlined />}
              loading={researchRunning}
              disabled={researchStatus?.running === true}
            >
              {researchRunning || researchStatus?.running ? '研究进行中…' : '启动一轮自动研究'}
            </Button>
          </Popconfirm>
        }
      >
        {researchStatus?.running && (
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 12 }}
            message={
              researchProgressText(researchStatus)[0] ?? '研究批次运行中'
            }
            description={
              <div>
                {researchProgressText(researchStatus).slice(1).map((line, i) => (
                  <div key={i} style={i === 0 ? { fontWeight: 600 } : { fontSize: 12, color: 'var(--ant-color-text-secondary, #888)' }}>
                    {line}
                  </div>
                ))}
                <div style={{ fontSize: 12, color: 'var(--ant-color-text-secondary, #888)', marginTop: 4 }}>
                  开始于 {researchStatus.running_since ? formatDateTime(researchStatus.running_since) : '—'} · 单轮硬上限
                  {' '}
                  90 分钟（防止卡死批次占用后台，正常远快于此） · 页面可以离开，完成后回来会自动展示结果
                  {researchStatus.last_error ? ` · 上次批次错误：${researchStatus.last_error}` : ''}
                </div>
              </div>
            }
          />
        )}
        {!researchStatus?.running && researchStatus?.last_error && (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
            message="上一轮研究批次失败"
            description={
              <div>
                <div style={{ fontSize: 12 }}>{researchStatus.last_error}</div>
                {/frozen snapshot|universe/i.test(researchStatus.last_error) && (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    自动研究需要一个多股票冻结快照；让 AI 用多只股票研究一次（会自动冻结快照）后再试。
                  </Text>
                )}
              </div>
            }
          />
        )}
        {researchError ? (
          <Alert
            type="error"
            showIcon
            message={researchError}
            description={
              /frozen snapshot|universe/i.test(researchError) ? (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  自动研究需要一个股票池 ≥ 5 只的多股票冻结快照；个股分析产生的是单票快照，不能当研究池。
                  让 AI 用多只股票研究一次（会自动冻结快照），或跑一次一键走通后再试。
                </Text>
              ) : undefined
            }
          />
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

      {/* 横截面多因子研究：选股问题的另一种重述——预测未来 N 日收益的截面排序 */}
      <FactorResearchCard />

      {/* 因子通道落库名单：write_to_selection=true 产出的 TopN 可追溯观察名单 */}
      <FactorPicksCard />

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
            <Empty description={filtersActive || onlyGate ? '当前筛选条件下没有方法' : '尚无可用方法'}>
              {filtersActive || onlyGate ? (
                <Button onClick={clearFilters}>清空筛选</Button>
              ) : (
                <Space direction="vertical" size={8} style={{ display: 'flex', maxWidth: 560 }}>
                  {rejectStats.length > 0 && (
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      库里有 {rejectStats.reduce((n, s) => n + s.count, 0)} 条研究记录但全部未通过证据门槛
                      （样本外交易不足 / 跑输基准 / 多重检验不显著），被拦下的方法不会进入选股——
                      这是可信度红线的正常表现，不是故障。再跑几轮研究、换不同持有期组合，通过的会自动出现在这里。
                    </Text>
                  )}
                  <Space>
                    <Button type="primary" icon={<ThunderboltOutlined />} loading={researchRunning} onClick={() => void runResearch()}>
                      启动自动研究
                    </Button>
                    <Button icon={<DownloadOutlined />} loading={seeding} onClick={() => void runSeed()}>
                      载入内置方法
                    </Button>
                    {rejectStats.length > 0 && (
                      <Button onClick={() => setStatuses(STATUS_OPTIONS.map((o) => o.value))}>查看被拒的诊断记录</Button>
                    )}
                  </Space>
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
