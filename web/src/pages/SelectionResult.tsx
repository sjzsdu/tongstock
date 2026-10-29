import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Descriptions,
  Empty,
  Popconfirm,
  Segmented,
  Space,
  Spin,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import { InfoCircleOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { api, type SelectionCandidate, type SelectionRun } from '../api/client';
import { formatDateTime } from '../lib/datetime';
import { evidenceLevel, exclusionReasonLabel, formatPercent } from '../lib/methodPresentation';
import {
  attributeCandidate,
  buyCapPerMethod,
  flatViewNote,
  groupCandidatesByMethod,
  hasMethodAttribution,
  isTruncatedDowngrade,
  truncationBreakdown,
  type MethodGroup,
} from '../lib/selectionGrouping';

const { Text, Title } = Typography;

const ACTION_META: Record<string, { color: string; label: string }> = {
  buy: { color: 'red', label: '买入' },
  watch: { color: 'orange', label: '观察' },
  avoid: { color: 'default', label: '回避' },
  insufficient_data: { color: 'default', label: '数据不足' },
};

function actionMeta(action: string): { color: string; label: string } {
  return ACTION_META[action] ?? { color: 'default', label: action };
}

const pct = (v: number) => formatPercent(v, 2);

type ViewMode = 'grouped' | 'flat';

const VIEW_MODE_OPTIONS: { label: string; value: ViewMode }[] = [
  { label: '按方法分组', value: 'grouped' },
  { label: '平铺列表', value: 'flat' },
];

function exitSummary(c: SelectionCandidate): string {
  const parts: string[] = [];
  if (c.exit.max_holding_days !== undefined) parts.push(`最长持有 ${c.exit.max_holding_days} 日`);
  if (c.exit.stop_loss_pct !== undefined) parts.push(`止损 ${pct(c.exit.stop_loss_pct)}`);
  if (c.exit.take_profit_pct !== undefined) parts.push(`止盈 ${pct(c.exit.take_profit_pct)}`);
  return parts.length > 0 ? parts.join('；') : '按方法退出规则';
}

/** 触发方法名串，展开行里再给完整事实明细。 */
function triggerNames(c: SelectionCandidate): string {
  if (!c.triggers?.length) return '-';
  const names = Array.from(new Set(c.triggers.map((t) => t.method_name)));
  return names.join('、');
}

/**
 * 动作列。per-method 上限落地后，「名额已满被降级」的观察项已经达到买入
 * 门槛，只是排在方法内第 N 名之外，视觉上必须和「股票不达标」的真 watch 区分。
 */
function actionCell(c: SelectionCandidate, cap?: number, groupMethodId?: string) {
  if (isTruncatedDowngrade(c)) {
    const capNote = cap !== undefined ? `（每个方法本次最多 ${cap} 只，名额已满）` : '（该方法本次买入名额已满）';
    return (
      <Tooltip
        title={`已通过买入门槛（评分与退出计划都完整），只因触发它的方法名额被更靠前的候选占满而降级观察${capNote}；这不是股票不达标。`}
      >
        <Tag color="gold" icon={<InfoCircleOutlined />}>
          名额已满 · 降级观察
        </Tag>
      </Tooltip>
    );
  }
  const meta = actionMeta(c.action);
  const truncatedHere = groupMethodId !== undefined && (c.truncated_by_method_ids ?? []).includes(groupMethodId);
  if (truncatedHere && c.action === 'buy') {
    return (
      <Space size={4} wrap>
        <Tag color={meta.color}>{meta.label}</Tag>
        <Tooltip title="该方法本次买入名额已满，本票的买入名额来自其它方法">
          <Tag>本方法未入选</Tag>
        </Tooltip>
      </Space>
    );
  }
  return <Tag color={meta.color}>{meta.label}</Tag>;
}

export default function SelectionResult() {
  const { runId } = useParams<{ runId: string }>();
  const location = useLocation();
  const [run, setRun] = useState<SelectionRun | null>(
    (location.state as { run?: SelectionRun } | null)?.run ?? null,
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  // per-method 口径下 buy 总数会随方法数增长，默认按方法分组把注意力结构还给用户。
  const [viewMode, setViewMode] = useState<ViewMode>('grouped');

  const load = useCallback(async () => {
    if (!runId) return;
    setLoading(true);
    setError('');
    try {
      const res = await api.selectionRunDetail(runId);
      setRun(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : '读取选股结果失败');
    } finally {
      setLoading(false);
    }
  }, [runId]);

  useEffect(() => {
    // 结果页可直接由 state 传入（选完即看），也可按 run id 拉取（刷新/分享链接）。
    if (!run && runId) queueMicrotask(() => void load());
    // eslint 风险低：仅在缺数据时拉取
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runId]);

  const candidates = useMemo(() => run?.candidates ?? [], [run]);
  /** 上限一律读 run 字段：旧 run 没有该字段时不当成 5，也不显示上限数字。 */
  const cap = useMemo(() => buyCapPerMethod(run), [run]);
  const legacyAttribution = useMemo(() => !hasMethodAttribution(candidates), [candidates]);
  const groups = useMemo<MethodGroup[]>(() => groupCandidatesByMethod(candidates, cap), [candidates, cap]);
  const flat = useMemo(() => (run ? flatViewNote(run) : undefined), [run]);
  const truncatedCount = useMemo(() => candidates.filter(isTruncatedDowngrade).length, [candidates]);

  const stats = useMemo(() => {
    const counts: Record<string, number> = { ...(run?.action_counts ?? {}) };
    for (const c of candidates) {
      if (counts[c.action] === undefined) counts[c.action] = 0;
    }
    return {
      buy: counts.buy ?? 0,
      watch: counts.watch ?? 0,
      avoid: counts.avoid ?? 0,
      insufficient: counts.insufficient_data ?? 0,
      total: candidates.length,
      scanned: run?.scanned_stocks,
      eligible: run?.eligible_methods,
    };
  }, [run, candidates]);

  const exclusionGroups = useMemo(() => {
    const groups = new Map<string, NonNullable<SelectionRun['exclusions']>>();
    for (const exclusion of run?.exclusions ?? []) {
      const entries = groups.get(exclusion.reason_code) ?? [];
      entries.push(exclusion);
      groups.set(exclusion.reason_code, entries);
    }
    return Array.from(groups.entries()).sort((a, b) => b[1].length - a[1].length);
  }, [run]);

  // —— 加入自选（用于后期评估方法的选股效果）——
  // 分类 = 方法名-选股日期；单方法运行时可整批加入，多方法时逐只按各自触发方法归组。
  const runMethodNames = useMemo(() => {
    const names = new Set<string>();
    for (const c of candidates) {
      for (const t of c.triggers ?? []) names.add(t.method_name);
    }
    return Array.from(names);
  }, [candidates]);

  const [addedCodes, setAddedCodes] = useState<Set<string>>(new Set());
  const [addingAll, setAddingAll] = useState(false);

  const watchGroupFor = useCallback(
    (methodName?: string) => `${methodName || '选股'}-${run?.snapshot_date ?? '未知日期'}`,
    [run],
  );

  const addToWatchlist = useCallback(
    async (c: SelectionCandidate) => {
      const methodName = runMethodNames.length === 1 ? runMethodNames[0] : c.triggers?.[0]?.method_name;
      const group = watchGroupFor(methodName);
      try {
        await api.watchlistAdd(c.code, undefined, group, `选股动作 ${actionMeta(c.action).label} · ${run?.snapshot_date ?? ''}`);
        setAddedCodes((prev) => new Set(prev).add(c.code));
        void message.success(`已将 ${c.code} 加入自选「${group}」`);
      } catch (e) {
        void message.error(e instanceof Error ? e.message : '加入自选失败');
      }
    },
    [runMethodNames, run, watchGroupFor],
  );

  const addAllToWatchlist = useCallback(async () => {
    // 买入/观察才是可跟踪的候选；回避与数据不足不进评估分组。
    const targets = candidates.filter((c) => c.action === 'buy' || c.action === 'watch');
    if (targets.length === 0) return;
    const group = watchGroupFor(runMethodNames[0]);
    setAddingAll(true);
    let ok = 0;
    let fail = 0;
    try {
      for (const c of targets) {
        try {
          await api.watchlistAdd(c.code, undefined, group, `选股动作 ${actionMeta(c.action).label} · ${run?.snapshot_date ?? ''}`);
          ok += 1;
        } catch {
          fail += 1;
        }
      }
      setAddedCodes((prev) => {
        const next = new Set(prev);
        for (const c of targets) next.add(c.code);
        return next;
      });
      void message.success(`已将 ${ok} 只候选加入自选「${group}」${fail > 0 ? `，${fail} 只失败` : ''}`);
    } finally {
      setAddingAll(false);
    }
  }, [candidates, run, runMethodNames, watchGroupFor]);

  const batchDisabled = runMethodNames.length !== 1;

  // 候选为空时，真正的拦截原因必须在头部说清楚，而不是把
  // 「没有股票满足入场规则」当头条（那次规则可能根本没被执行）。
  const blockedBy = useMemo(() => {
    if (candidates.length > 0 || exclusionGroups.length === 0) return undefined;
    const total = run!.exclusions.length;
    return exclusionGroups.slice(0, 3).map(([reasonCode, entries]) => ({
      reasonCode,
      label: exclusionReasonLabel(reasonCode),
      count: entries.length,
      detail: entries[0]?.detail,
      dominant: entries.length === total,
    }));
  }, [run, candidates, exclusionGroups]);

  const buildColumns = useCallback(
    (groupMethodId?: string): ColumnsType<SelectionCandidate> => [
      { title: '排名', dataIndex: 'rank', width: 70 },
      {
        title: '代码',
        dataIndex: 'code',
        width: 110,
        render: (v: string) => (
          <Link to={`/stock/${v}`} aria-label={`查看股票 ${v}`}>
            <Text strong code style={{ fontSize: 12 }}>{v}</Text>
          </Link>
        ),
      },
      {
        title: '动作',
        dataIndex: 'action',
        width: 170,
        filters: groupMethodId
          ? undefined
          : [
            { text: '买入', value: 'buy' },
            { text: '观察', value: 'watch' },
            { text: '回避', value: 'avoid' },
            { text: '数据不足', value: 'insufficient_data' },
          ],
        onFilter: groupMethodId ? undefined : (value, c) => c.action === value,
        render: (_, c: SelectionCandidate) => actionCell(c, cap, groupMethodId),
      },
      { title: '评分', dataIndex: 'score', width: 80, sorter: (a, b) => a.score - b.score },
      { title: '数据日期', dataIndex: 'data_date', width: 110 },
      {
        title: '触发方法',
        key: 'methods',
        width: 230,
        render: (_, c) => {
          // 「同时被 N 个方法选中」= 把本票计入买入名额的方法数，不是触发方法数。
          const attribution = attributeCandidate(c, legacyAttribution);
          return (
            <Space orientation="vertical" size={2}>
              <Text style={{ fontSize: 12 }}>{triggerNames(c)}</Text>
              {attribution.kept.length >= 2 && (
                <Tag color="geekblue">同时被 {attribution.kept.length} 个方法选中</Tag>
              )}
              {c.action === 'buy' && attribution.kept.length === 1 && (
                <Text type="secondary" style={{ fontSize: 11 }}>来自 1 个方法的买入名额</Text>
              )}
            </Space>
          );
        },
      },
      {
        title: '解释',
        key: 'explanation',
        render: (_, c) => (
          <Text type="secondary" style={{ fontSize: 12 }}>
            {c.explanation}
          </Text>
        ),
      },
      {
        title: '操作',
        key: 'watch',
        width: 110,
        render: (_, c) =>
          addedCodes.has(c.code) ? (
            <Tag color="green">已加入自选</Tag>
          ) : (
            <Button
              size="small"
              type="link"
              icon={<PlusOutlined />}
              onClick={() => void addToWatchlist(c)}
            >
              加入自选
            </Button>
          ),
      },
    ],
    [addedCodes, addToWatchlist, cap, legacyAttribution],
  );

  const expandedRowRender = useCallback(
    (c: SelectionCandidate) => (
      <Space orientation="vertical" size={8} style={{ display: 'flex' }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          买入窗口 {c.buy_window || '-'}；仓位上限 {pct(c.position_cap_pct)}；退出：{exitSummary(c)}
        </Text>
        {c.risks?.length ? (
          <Alert type="warning" showIcon title={`风险提示：${c.risks.join('；')}`} />
        ) : null}
        {c.triggers.map((t, i) => (
          <Card
            key={`${t.method_id}-${i}`}
            size="small"
            title={
              <Space size={8} wrap>
                <Text strong>{t.method_name}</Text>
                <Tag>贡献分 {t.score}</Tag>
                {t.evidence && (
                  <Tag color={t.evidence.passable ? 'green' : 'red'}>证据等级：{evidenceLevel(t.evidence)}</Tag>
                )}
              </Space>
            }
          >
            <Descriptions size="small" column={1} colon={false}>
              <Descriptions.Item label="方法 ID">
                <Text code style={{ fontSize: 11, wordBreak: 'break-all' }}>
                  {t.method_id}
                </Text>
              </Descriptions.Item>
              {t.evidence && (
                <Descriptions.Item label="证据">
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    样本外 {t.evidence.oos_trades} 笔 / 胜率 {formatPercent(t.evidence.oos_win_rate)} / 收益 {pct(t.evidence.oos_return)}
                    {t.evidence.confidence_reason ? ` / 原因 ${t.evidence.confidence_reason}` : ''}
                  </Text>
                </Descriptions.Item>
              )}
              <Descriptions.Item label="触发事实">
                <Space orientation="vertical" size={2} style={{ display: 'flex' }}>
                  {t.facts.map((f, fi) => (
                    <Text
                      key={fi}
                      type={f.passed ? undefined : 'secondary'}
                      style={{ fontSize: 12 }}
                    >
                      {f.passed ? '✓' : '✗'} {f.path}
                      {f.rule ? ` (${f.rule})` : ''}
                      {f.detail ? ` — ${f.detail}` : ''}
                    </Text>
                  ))}
                </Space>
              </Descriptions.Item>
            </Descriptions>
          </Card>
        ))}
      </Space>
    ),
    [],
  );

  const renderCandidateTable = useCallback(
    (rows: SelectionCandidate[], groupMethodId?: string, pageSize?: number) => (
      <Table<SelectionCandidate>
        rowKey={(c) => `${c.rank}-${c.code}`}
        size={groupMethodId ? 'small' : 'middle'}
        loading={loading}
        columns={buildColumns(groupMethodId)}
        dataSource={rows}
        scroll={{ x: 980 }}
        pagination={pageSize
          ? { pageSize, size: 'small', hideOnSinglePage: true, showTotal: (t) => `共 ${t} 只` }
          : { pageSize: 50, showTotal: (t) => `共 ${t} 个候选` }}
        locale={{
          emptyText: (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={
                <Space orientation="vertical" size={2}>
                  <Text strong>当时没有方法通过门槛，也没有方法选中任何股票</Text>
                  <Text type="secondary">这是合法结果：门槛不会为了凑数量被放宽，也不代表加载失败。</Text>
                </Space>
              }
            />
          ),
        }}
        expandable={{ expandedRowRender }}
      />
    ),
    [buildColumns, expandedRowRender, loading],
  );

  if (!run) {
    if (loading) {
      return (
        <div style={{ padding: 48, textAlign: 'center' }}>
          <Spin />
          <div style={{ marginTop: 12 }}>
            <Text type="secondary">正在读取选股结果…</Text>
          </div>
        </div>
      );
    }
    return (
      <Empty
        description={error || '找不到这次选股运行'}
        style={{ padding: 48 }}
      >
        <Space>
          {runId && (
            <Button icon={<ReloadOutlined />} onClick={() => void load()}>
              重试
            </Button>
          )}
          <Link to="/methods">返回我的选股方法</Link>
        </Space>
      </Empty>
    );
  }

  const hasCandidates = candidates.length > 0;
  const capText = cap !== undefined ? `每个方法本次最多 ${cap} 只` : '每个方法各自计额';

  return (
    <Space orientation="vertical" size={16} style={{ display: 'flex' }}>
      <Space align="center" size={12} wrap>
        <Title level={3} style={{ margin: 0 }}>
          今日候选股票
        </Title>
        <Text type="secondary">
          快照 {run.snapshot_id} · 快照日期 {run.snapshot_date}
          {run.requested_method_ids?.length ? ` · 指定方法 ${run.requested_method_ids.length} 个` : ''}
        </Text>
        {hasCandidates && (
          <>
            <Segmented<ViewMode>
              value={viewMode}
              onChange={(value) => setViewMode(value)}
              options={VIEW_MODE_OPTIONS}
              aria-label="结果展示方式"
            />
            <Tooltip
              title={
                batchDisabled
                  ? '本次运行包含多个方法，请使用每行的「加入自选」按各自触发方法归组'
                  : `整批买入/观察候选加入自选分组「${watchGroupFor(runMethodNames[0])}」，便于后续跟踪该方法的选股效果`
              }
            >
              <Popconfirm
                title="整批加入自选"
                description={`将把买入/观察候选加入分组「${watchGroupFor(runMethodNames[0])}」。若某只已在其他分组，会被移动到本分组（自选股以代码为唯一键）。`}
                okText="加入"
                onConfirm={() => void addAllToWatchlist()}
                disabled={batchDisabled}
              >
                <Button icon={<PlusOutlined />} loading={addingAll} disabled={batchDisabled}>
                  整批加入自选
                </Button>
              </Popconfirm>
            </Tooltip>
          </>
        )}
      </Space>

      {error && (
        <Alert
          type="error"
          title={error}
          action={
            <Button size="small" icon={<ReloadOutlined />} onClick={() => void load()}>
              重试
            </Button>
          }
        />
      )}

      {blockedBy && (
        <Alert
          type="warning"
          showIcon
          title={
            blockedBy.length === 1 && blockedBy[0].dominant
              ? `本次筛选没有产生候选：${blockedBy[0].label}（${blockedBy[0].count} 条）`
              : `本次筛选没有产生候选，主要拦截原因：${blockedBy.map((b) => `${b.label} ${b.count} 条`).join('；')}`
          }
          description={
            <Space orientation="vertical" size={2}>
              {blockedBy[0]?.detail && <Text type="secondary" style={{ fontSize: 12 }}>{blockedBy[0].detail}</Text>}
              <Text type="secondary" style={{ fontSize: 12 }}>这种情况下入场规则从未被执行过，不是「股票不达标」；完整分组见下方未入选记录。</Text>
            </Space>
          }
        />
      )}

      <Card size="small">
        <Space size={32} wrap>
          <Statistic title="买入" value={stats.buy} styles={{ content: { color: '#cf1322' } }} />
          <Statistic title="观察" value={stats.watch} styles={{ content: { color: '#d46b08' } }} />
          {truncatedCount > 0 && (
            <Tooltip title={`已达到买入门槛、只因所在方法名额已满而降级观察${cap !== undefined ? `（单方法上限 ${cap} 只）` : ''}；不是股票不达标。`}>
              <Statistic title="名额已满降级" value={truncatedCount} styles={{ content: { color: '#ad6800' } }} />
            </Tooltip>
          )}
          <Statistic title="回避 / 数据不足" value={stats.avoid + stats.insufficient} />
          <Statistic title="候选总数" value={stats.total} />
          {stats.scanned !== undefined && <Statistic title="扫描股票" value={stats.scanned} />}
          {stats.eligible !== undefined && <Statistic title="可用方法" value={stats.eligible} />}
        </Space>
      </Card>

      {!hasCandidates && (
        <Card size="small">
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <Space orientation="vertical" size={4} style={{ display: 'flex', maxWidth: 680 }}>
                <Text strong>当前没有候选股票 —— 这是合法结果，不是加载失败</Text>
                <Text type="secondary">
                  「当前没有方法通过门槛」是最常见的原因：方法要先在真实历史数据上完成验证并达到门槛
                  （足够的样本量与交易笔数、证据等级达标）才参与选股；通过了门槛的方法，今天也可能没有触发任何股票。
                  空榜单是常态，平台不会为了凑数量放宽门槛或降低规则，也不是系统故障。
                </Text>
                {blockedBy && <Text type="secondary">本次的直接拦截原因见页面上方提示与下方「未入选记录」。</Text>}
              </Space>
            }
          />
        </Card>
      )}

      {hasCandidates && viewMode === 'flat' && flat && (
        <Alert
          type={flat.exceedsCap ? 'info' : 'success'}
          showIcon
          title={`本次共 ${flat.buyCount} 只买入候选，来自 ${flat.methodCount} 个方法`}
          description={
            <Space orientation="vertical" size={2}>
              {flat.exceedsCap && flat.cap !== undefined && (
                <Text style={{ fontSize: 12 }}>
                  {`总数超过 ${flat.cap} 只并不是某一个方法给出了这么多：per-method 口径下每个方法各自独立计额（单方法上限 ${flat.cap} 只），方法越多上限越高。按方法分组查看可以看到每个方法各自的 ${flat.cap} 只以内。`}
                </Text>
              )}
              {!flat.exceedsCap && flat.cap !== undefined && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {`${capText}，本次买入总数没有超过单方法上限。`}
                </Text>
              )}
              {flat.truncatedCount > 0 && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  另有 {flat.truncatedCount} 只票已达到买入门槛，只因所在方法名额已满而降级为观察（标记为「名额已满 · 降级观察」，与股票不达标的观察项不同）。
                </Text>
              )}
              {legacyAttribution && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  这次结果产生于 per-method 口径之前，没有逐方法的买入名额记录，来源方法数按触发方法统计。
                </Text>
              )}
            </Space>
          }
        />
      )}

      {hasCandidates && viewMode === 'grouped' && (
        <Space orientation="vertical" size={12} style={{ display: 'flex' }}>
          <Alert
            type="info"
            showIcon
            title={cap !== undefined ? `按方法分组：每个方法本次最多 ${cap} 只买入候选` : '按方法分组：按触发方法拆开本次候选'}
            description={
              <Space orientation="vertical" size={2}>
                <Text style={{ fontSize: 12 }}>
                  每组只列出该方法的买入名额归属与它截断降级的项。同一只票被多个方法保留时会在多个分组里各出现一次，
                  并标注「同时被 N 个方法选中」。
                </Text>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {flat && flat.buyCount > 0
                    ? `本次共 ${flat.buyCount} 只买入候选，来自 ${flat.methodCount} 个方法；分组不改变任何判定，只是换一种看法。`
                    : '本次没有方法产生买入候选：观察项不是买入信号。'}
                  {truncatedCount > 0 ? ` 有 ${truncatedCount} 只因名额已满被降级为观察。` : ''}
                </Text>
              </Space>
            }
          />
          {groups.map((group) => {
            const primary = [...group.kept, ...group.truncated];
            const truncation = truncationBreakdown(group);
            return (
              <Card
                key={group.methodId}
                size="small"
                title={
                  <Space size={8} wrap>
                    <Text strong>{group.methodName}</Text>
                    <Tag color="red">
                      买入 {group.kept.length}
                      {group.cap !== undefined ? ` / 上限 ${group.cap}` : ''}
                    </Tag>
                    {truncation.downgraded > 0 && (
                      <Tooltip title="已达到买入门槛，只因该方法名额已满而降级观察；不是股票不达标。">
                        <Tag color="gold">名额已满降级 {truncation.downgraded}</Tag>
                      </Tooltip>
                    )}
                    {truncation.keptElsewhere > 0 && (
                      <Tooltip title="被该方法的名额截断，但买入名额来自其它方法，本票仍是买入。">
                        <Tag>本方法名额外 {truncation.keptElsewhere}</Tag>
                      </Tooltip>
                    )}
                    {group.others.length > 0 && <Tag>其它候选 {group.others.length}</Tag>}
                  </Space>
                }
              >
                {primary.length === 0 ? (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    该方法本次没有买入候选
                    {group.others.length > 0 ? '，仅产生了未达买入门槛的观察项（见下方折叠区）。' : '。'}
                  </Text>
                ) : (
                  renderCandidateTable(primary, group.methodId)
                )}
                {group.others.length > 0 && (
                  <Collapse
                    ghost
                    size="small"
                    items={[{
                      key: 'others',
                      label: (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          该方法其它候选（未占买入名额：观察 / 回避 / 数据不足，{group.others.length} 只）
                        </Text>
                      ),
                      children: renderCandidateTable(group.others, group.methodId, 10),
                    }]}
                  />
                )}
              </Card>
            );
          })}
        </Space>
      )}

      {hasCandidates && viewMode === 'flat' && renderCandidateTable(candidates)}

      <Card size="small" title={`未入选记录（${run.exclusions.length}）`}>
        {exclusionGroups.length === 0 ? (
          <Text type="secondary">没有额外排除记录。未命中入场规则的股票不会逐只记为排除。</Text>
        ) : (
          <Collapse
            ghost
            items={exclusionGroups.map(([reasonCode, entries]) => ({
              key: reasonCode,
              label: (
                <Space wrap>
                  <Text>{exclusionReasonLabel(reasonCode)}</Text>
                  <Tag>{entries.length} 条</Tag>
                </Space>
              ),
              children: (
                <Space orientation="vertical" size={6} style={{ display: 'flex' }}>
                  {entries.slice(0, 100).map((entry, index) => (
                    <Space key={`${entry.method_id ?? ''}-${entry.code ?? ''}-${index}`} wrap size={8}>
                      {entry.code && <Link to={`/stock/${entry.code}`}>{entry.code}</Link>}
                      {entry.method_id && <Text code style={{ fontSize: 11 }}>{entry.method_id}</Text>}
                      <Text type="secondary">{entry.detail}</Text>
                    </Space>
                  ))}
                  {entries.length > 100 && <Text type="secondary">仅展示前 100 条，共 {entries.length} 条。</Text>}
                </Space>
              ),
            }))}
          />
        )}
      </Card>

      <Text type="secondary" style={{ fontSize: 12 }}>
        生成于 {run.created_at ? formatDateTime(run.created_at) : '-'}。触发事实是方法规则在该快照上的机器判定，历史胜率不代表未来收益，结果不是投资建议。
      </Text>
    </Space>
  );
}
