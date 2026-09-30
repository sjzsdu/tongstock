import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  List,
  Select,
  Space,
  Spin,
  Tag,
  Timeline,
  Typography,
  message,
} from 'antd';
import { DownloadOutlined, FileSearchOutlined, ReloadOutlined } from '@ant-design/icons';
import { api, type MethodAuditEvent, type MethodCard, type MethodSeedResult } from '../api/client';
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

function holdingQuery(
  holdingMin?: string,
  holdingMax?: string,
): { holding_min_days?: number; holding_max_days?: number } {
  const q: { holding_min_days?: number; holding_max_days?: number } = {};
  if (holdingMin !== undefined) q.holding_min_days = Number.parseInt(holdingMin, 10);
  if (holdingMax !== undefined) q.holding_max_days = Number.parseInt(holdingMax, 10);
  return q;
}

const pct = (v: number) => `${(v * 100).toFixed(2)}%`;

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
  const [facets, setFacets] = useState<{ markets: string[]; universes: string[] }>({ markets: [], universes: [] });
  const [seeding, setSeeding] = useState(false);
  const [seedResult, setSeedResult] = useState<MethodSeedResult>();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [detail, setDetail] = useState<MethodCard>();
  const [audit, setAudit] = useState<MethodAuditEvent[]>();
  const [auditError, setAuditError] = useState('');
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

  useEffect(() => {
    queueMicrotask(() => void load());
  }, [load]);

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

  const clearFilters = useCallback(() => {
    setStatuses([]);
    setMarket(undefined);
    setUniverse(undefined);
    setHoldingMin(undefined);
    setHoldingMax(undefined);
  }, []);

  const gatePassed = useMemo(() => items.filter(passesGate), [items]);
  const showColdStart = !loading && !error && items.length > 0 && gatePassed.length === 0;

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      <div>
        <Title level={3}>可信投资方法库</Title>
        <Text type="secondary">
          只有机器证据门槛通过的方法才会进入每日选股；失败与退役方法仍保留审计记录。当前 {items.length} 条记录中{' '}
          {gatePassed.length} 条通过选股门槛。
        </Text>
      </div>

      {error && <Alert type="error" message={error} action={<Button size="small" onClick={() => void load()}>重试</Button>} />}

      <Card size="small">
        <Space wrap>
          <Select
            mode="multiple"
            allowClear
            placeholder="状态"
            value={statuses}
            onChange={setStatuses}
            style={{ minWidth: 240, maxWidth: 420 }}
            maxTagCount="responsive"
            options={STATUS_OPTIONS}
          />
          <Select
            allowClear
            showSearch
            placeholder="市场"
            value={market}
            onChange={setMarket}
            style={{ width: 140 }}
            options={facets.markets.map((v) => ({ value: v, label: v }))}
          />
          <Select
            allowClear
            showSearch
            placeholder="股票池 universe"
            value={universe}
            onChange={setUniverse}
            style={{ width: 220 }}
            options={facets.universes.map((v) => ({ value: v, label: v }))}
          />
          <Select
            allowClear
            placeholder="最长持有 ≥"
            value={holdingMin}
            onChange={setHoldingMin}
            style={{ width: 210 }}
            options={HOLDING_MIN_OPTIONS}
          />
          <Select
            allowClear
            placeholder="最短持有 ≤"
            value={holdingMax}
            onChange={setHoldingMax}
            style={{ width: 210 }}
            options={HOLDING_MAX_OPTIONS}
          />
          <Button icon={<ReloadOutlined />} onClick={() => void load()}>
            刷新
          </Button>
          <Button
            type="primary"
            icon={<DownloadOutlined />}
            loading={seeding}
            onClick={() => void runSeed()}
            title="在冻结真实日线上回测并登记内置方法"
          >
            {seeding ? '回测中…' : '载入内置方法'}
          </Button>
          {filtersActive && (
            <Button onClick={clearFilters}>清空筛选</Button>
          )}
          <Text type="secondary">共 {total} 条{filtersActive ? '（已筛选）' : ''}</Text>
        </Space>
      </Card>

      {seedResult && <SeedResultAlert result={seedResult} onDismiss={() => setSeedResult(undefined)} />}

      {showColdStart && (
        <Alert
          type="warning"
          showIcon
          message={`当前 ${items.length} 个方法全部未通过机器证据门槛，每日选股不会使用它们`}
          description={
            <Space direction="vertical" size={8} style={{ display: 'flex' }}>
              <Text>
                这些记录来自 AI 规律发现的候选或历史登记，状态为 rejected 表示样本外证据不足；失败记录会保留用于审计，
                但不会进入选股。下一步可以载入内置方法（在冻结真实日线上回测后按真实结论登记），或去范式库/让 AI 研究一个新方法。
              </Text>
              <Space wrap>
                <Button size="small" type="primary" icon={<DownloadOutlined />} loading={seeding} onClick={() => void runSeed()}>
                  载入内置方法（真实回测）
                </Button>
                <Button size="small" icon={<FileSearchOutlined />} onClick={() => navigate('/paradigms')}>
                  去范式库
                </Button>
                <Button size="small" onClick={() => navigate('/agent')}>
                  让 AI 研究一个新方法
                </Button>
              </Space>
            </Space>
          }
        />
      )}

      <List
        grid={{ gutter: 16, column: 2 }}
        loading={loading}
        dataSource={items}
        locale={{
          emptyText: (
            <Empty description={filtersActive ? '当前筛选条件下没有方法' : '尚无已登记方法'}>
              {filtersActive ? (
                <Button onClick={clearFilters}>清空筛选</Button>
              ) : (
                <Button type="primary" icon={<DownloadOutlined />} loading={seeding} onClick={() => void runSeed()}>
                  载入内置方法
                </Button>
              )}
            </Empty>
          ),
        }}
        renderItem={(m) => (
          <List.Item
            key={m.id}
            actions={[
              <Button
                key="detail"
                size="small"
                icon={<FileSearchOutlined />}
                onClick={() => void openDetail(m)}
              >
                详情与审计
              </Button>,
            ]}
          >
            <Card
              style={{ width: '100%' }}
              title={
                <Space wrap size={8}>
                  <span>{m.name}</span>
                  <Tag color={statusColor(m.status)}>{m.status}</Tag>
                  {passesGate(m) && <Tag color="blue">进入选股</Tag>}
                </Space>
              }
            >
              <p>{m.entry_summary}</p>
              <p>{m.exit_summary}</p>
              <Space wrap>
                <Tag>{m.market}</Tag>
                <Tag>{m.universe}</Tag>
                <Tag>{m.holding_period}</Tag>
                {m.evidence && (
                  <Tag color={m.evidence.passable ? 'green' : 'red'}>
                    证据 {m.evidence.confidence} / OOS {m.evidence.oos_trades} 笔 / {pct(m.evidence.oos_return)}
                  </Tag>
                )}
                <Text type="secondary" style={{ fontSize: 12 }}>
                  更新于 {formatDateTime(m.updated_at)}
                </Text>
              </Space>
            </Card>
          </List.Item>
        )}
      />

      <Drawer
        title={detail ? detail.name : '方法详情'}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={560}
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
              <Card size="small" title="样本外证据">
                <Descriptions size="small" column={1} colon={false}>
                  <Descriptions.Item label="置信度">
                    <Tag color={detail.evidence.passable ? 'green' : 'red'}>{detail.evidence.confidence}</Tag>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {detail.evidence.passable ? '证据可用' : '证据不足，不进选股'}
                    </Text>
                  </Descriptions.Item>
                  <Descriptions.Item label="样本外交易">{detail.evidence.oos_trades} 笔</Descriptions.Item>
                  <Descriptions.Item label="样本外收益">{pct(detail.evidence.oos_return)}</Descriptions.Item>
                  <Descriptions.Item label="样本外胜率">
                    {detail.evidence.oos_win_rate !== undefined ? pct(detail.evidence.oos_win_rate) : '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="样本外最大回撤">{pct(detail.evidence.oos_max_drawdown)}</Descriptions.Item>
                  <Descriptions.Item label="冻结快照">
                    <Text code style={{ fontSize: 11, wordBreak: 'break-all' }}>
                      {detail.evidence.snapshot_id || '-'}
                    </Text>
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            ) : (
              <Alert type="info" showIcon message="该方法还没有证据包" />
            )}

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
    </Space>
  );
}
