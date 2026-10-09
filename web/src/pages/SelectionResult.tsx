import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Descriptions,
  Empty,
  Space,
  Spin,
  Statistic,
  Table,
  Tag,
  Typography,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { api, type SelectionCandidate, type SelectionRun } from '../api/client';
import { formatDateTime } from '../lib/datetime';
import { evidenceLevel, exclusionReasonLabel, formatPercent } from '../lib/methodPresentation';

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

export default function SelectionResult() {
  const { runId } = useParams<{ runId: string }>();
  const location = useLocation();
  const [run, setRun] = useState<SelectionRun | null>(
    (location.state as { run?: SelectionRun } | null)?.run ?? null,
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

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

  const stats = useMemo(() => {
    const counts: Record<string, number> = { ...(run?.action_counts ?? {}) };
    for (const c of run?.candidates ?? []) {
      if (counts[c.action] === undefined) counts[c.action] = 0;
    }
    return {
      buy: counts.buy ?? 0,
      watch: counts.watch ?? 0,
      avoid: counts.avoid ?? 0,
      insufficient: counts.insufficient_data ?? 0,
      total: run?.candidates.length ?? 0,
      scanned: run?.scanned_stocks,
      eligible: run?.eligible_methods,
    };
  }, [run]);

  const exclusionGroups = useMemo(() => {
    const groups = new Map<string, NonNullable<SelectionRun['exclusions']>>();
    for (const exclusion of run?.exclusions ?? []) {
      const entries = groups.get(exclusion.reason_code) ?? [];
      entries.push(exclusion);
      groups.set(exclusion.reason_code, entries);
    }
    return Array.from(groups.entries()).sort((a, b) => b[1].length - a[1].length);
  }, [run]);

  // 候选为空时，真正的拦截原因必须在头部说清楚，而不是把
  // 「没有股票满足入场规则」当头条（那次规则可能根本没被执行）。
  const blockedBy = useMemo(() => {
    if ((run?.candidates.length ?? 0) > 0 || exclusionGroups.length === 0) return undefined;
    const total = run!.exclusions.length;
    return exclusionGroups.slice(0, 3).map(([reasonCode, entries]) => ({
      reasonCode,
      label: exclusionReasonLabel(reasonCode),
      count: entries.length,
      detail: entries[0]?.detail,
      dominant: entries.length === total,
    }));
  }, [run, exclusionGroups]);

  const columns: ColumnsType<SelectionCandidate> = [
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
      width: 90,
      filters: [
        { text: '买入', value: 'buy' },
        { text: '观察', value: 'watch' },
        { text: '回避', value: 'avoid' },
        { text: '数据不足', value: 'insufficient_data' },
      ],
      onFilter: (value, c) => c.action === value,
      render: (v: string) => {
        const meta = actionMeta(v);
        return <Tag color={meta.color}>{meta.label}</Tag>;
      },
    },
    { title: '评分', dataIndex: 'score', width: 80, sorter: (a, b) => a.score - b.score },
    { title: '数据日期', dataIndex: 'data_date', width: 110 },
    {
      title: '触发方法',
      key: 'methods',
      width: 220,
      render: (_, c) => <Text style={{ fontSize: 12 }}>{triggerNames(c)}</Text>,
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
  ];

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
          <Statistic title="回避 / 数据不足" value={stats.avoid + stats.insufficient} />
          <Statistic title="候选总数" value={stats.total} />
          {stats.scanned !== undefined && <Statistic title="扫描股票" value={stats.scanned} />}
          {stats.eligible !== undefined && <Statistic title="可用方法" value={stats.eligible} />}
        </Space>
      </Card>

      <Table<SelectionCandidate>
        rowKey={(c) => `${c.rank}-${c.code}`}
        size="middle"
        loading={loading}
        columns={columns}
        dataSource={run.candidates}
        scroll={{ x: 860 }}
        pagination={{ pageSize: 50, showTotal: (t) => `共 ${t} 个候选` }}
        locale={{
          emptyText: (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={
                blockedBy ? (
                  <Space orientation="vertical" size={2}>
                    <Text strong>没有候选股票：{blockedBy.map((b) => b.label).join('；')}</Text>
                    <Text type="secondary">原因与处理建议见页面上方的提示。</Text>
                  </Space>
                ) : (
                  <Space orientation="vertical" size={2}>
                    <Text strong>今天没有股票满足入场规则且同时未触发失效条件</Text>
                    <Text type="secondary">这是正常结果，不会为了凑数量放宽规则。下方可查看方法或数据被排除的原因。</Text>
                  </Space>
                )
              }
            />
          ),
        }}
        expandable={{
          expandedRowRender: (c) => (
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
        }}
      />

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
