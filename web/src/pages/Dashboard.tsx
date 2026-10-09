import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Col,
  Collapse,
  Input,
  List,
  Row,
  Space,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  DashboardOutlined,
  RadarChartOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
  StockOutlined,
} from '@ant-design/icons';
import {
  api,
  type DashboardAction,
  type DashboardToday,
  type MethodSeedResult,
  type PositionDecisionRun,
} from '../api/client';
import SystemHealthPanel from '../components/dashboard/SystemHealthPanel';
import WorkLogPanel from '../components/dashboard/WorkLogPanel';
import EmptyStatePanel, { ExclusionBreakdown } from '../components/dashboard/EmptyStatePanel';
import OnboardingWizard from '../components/dashboard/OnboardingWizard';
import MethodMechanismDiagram from '../components/dashboard/MethodMechanismDiagram';

const { Text, Title } = Typography;

const ONBOARDING_KEY = 'tongstock.onboarding.done';

function QuickStartCard({
  icon,
  title,
  desc,
  links,
}: {
  icon: React.ReactNode;
  title: string;
  desc: string;
  links: { to: string; label: string }[];
}) {
  return (
    <Col xs={12} md={6}>
      <Space direction="vertical" size={6} style={{ display: 'flex' }}>
        <Space size={8}>
          <span style={{ color: '#1677ff', fontSize: 16 }}>{icon}</span>
          <Text strong>{title}</Text>
        </Space>
        <Text type="secondary" style={{ fontSize: 12 }}>
          {desc}
        </Text>
        <Space size={12} wrap>
          {links.map((l) => (
            <Link key={l.to} to={l.to}>
              {l.label}
            </Link>
          ))}
        </Space>
      </Space>
    </Col>
  );
}

/** 把内置示例方法的真实验证结论如实展示，不美化、不隐藏未通过的方法。 */
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
          ? `内置示例方法已用真实数据回测：${result.registered} 个已登记，0 个通过证据门槛`
          : `内置示例方法已用真实数据回测：${result.verified} 个通过证据门槛`
      }
      description={
        <Space direction="vertical" size={4} style={{ display: 'flex' }}>
          <Text type="secondary" style={{ fontSize: 12 }}>
            验证区间 {result.date_start} ~ {result.date_end}，股票池 {result.universe_size} 只（冻结快照 {result.snapshot_id}）
          </Text>
          {result.outcomes.map((o) => (
            <Space key={o.key} size={8} wrap>
              <Text strong>{o.name}</Text>
              <Tag color={o.passable ? 'green' : 'red'}>{o.passable ? '通过门槛' : '未通过门槛'}</Tag>
              {o.confidence && <Tag>{`置信度 ${o.confidence}`}</Tag>}
              <Text type="secondary" style={{ fontSize: 12 }}>
                样本外 {o.oos_trades} 笔，收益 {(o.oos_return * 100).toFixed(2)}%，最大回撤{' '}
                {(o.oos_max_drawdown * 100).toFixed(2)}%
              </Text>
              {o.error && <Text type="danger" style={{ fontSize: 12 }}>{o.error}</Text>}
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

export default function Dashboard() {
  const navigate = useNavigate();
  const [today, setToday] = useState<DashboardToday>();
  const [positions, setPositions] = useState<PositionDecisionRun>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState('');
  const [question, setQuestion] = useState('');
  const [researching, setResearching] = useState(false);
  const [answer, setAnswer] = useState('');
  const [showExclusions, setShowExclusions] = useState(false);
  const [seedResult, setSeedResult] = useState<MethodSeedResult>();
  const [showGuide, setShowGuide] = useState(() => {
    if (typeof window === 'undefined') return false;
    return window.localStorage.getItem(ONBOARDING_KEY) !== '1';
  });

  const load = useCallback(async () => {
    setLoading(true);
    await Promise.allSettled([
      api.dashboardToday().then(setToday),
      api.positionDecisionToday().then(setPositions),
    ]);
    setLoading(false);
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const dismissGuide = useCallback(() => {
    setShowGuide(false);
    if (typeof window !== 'undefined') window.localStorage.setItem(ONBOARDING_KEY, '1');
  }, []);

  const research = async () => {
    const q = question.trim();
    if (!q) return;
    const stock = q.match(/\b\d{6}\b/)?.[0];
    if (stock && q === stock) {
      navigate(`/stock/${stock}`);
      return;
    }
    setResearching(true);
    setAnswer('');
    try {
      const r = await api.agentChat(
        `请以投资方法研究流程分析：${q}。必须引用真实数据和证据；没有验证结果就明确拒绝结论。`,
      );
      setAnswer(r.response || r.error || 'AI 未返回结果');
    } catch (e) {
      void message.error(e instanceof Error ? e.message : '研究启动失败');
    } finally {
      setResearching(false);
    }
  };

  const handleAction = async (action: DashboardAction) => {
    switch (action.kind) {
      case 'view_exclusions':
        setShowExclusions((v) => !v);
        return;
      case 'research':
        navigate(action.to || '/methods');
        return;
      case 'seed_methods': {
        setBusy('seed');
        try {
          const res = await api.seedMethods({});
          setSeedResult(res);
          if (res.verified > 0) void message.success(`已登记 ${res.verified} 个通过验证的内置方法`);
          else void message.warning('内置示例方法已用真实数据回测，但没有方法通过证据门槛');
          await load();
        } catch (e) {
          void message.error(e instanceof Error ? e.message : '载入内置示例方法失败');
        } finally {
          setBusy('');
        }
        return;
      }
      case 'sync':
      case 'run_selection': {
        setBusy(action.kind);
        try {
          const res = await api.onboardingRun({ sync: action.kind === 'sync' });
          if (res.status === 'completed') {
            dismissGuide();
            void message.success(`已完成：扫描 ${res.scanned_stocks} 只，得到 ${res.candidate_count} 个候选`);
          } else {
            void message.warning(res.blocked_reason || '执行被真实条件阻断');
          }
          await load();
        } catch (e) {
          void message.error(e instanceof Error ? e.message : '执行失败');
        } finally {
          setBusy('');
        }
        return;
      }
      default:
        if (action.to) navigate(action.to);
    }
  };

  const selection = today?.selection;
  const candidates = selection?.candidates ?? [];
  const hasCandidates = candidates.length > 0;
  const urgent = (positions?.decisions || []).filter((x) => x.action === 'exit' || x.action === 'reduce');

  return (
    <Space direction="vertical" size={20} style={{ display: 'flex' }}>
      <Card bordered={false} style={{ background: 'linear-gradient(135deg,#102b50,#111827)' }}>
        <Space direction="vertical" size={14} style={{ display: 'flex' }}>
          <Tag color="blue" style={{ width: 'fit-content' }}>
            AI 原生投资决策
          </Tag>
          <Title level={2} style={{ margin: 0 }}>
            今天买什么，持仓什么时候卖
          </Title>
          <Text type="secondary">
            AI 负责研究与解释；所有分数、行情和证据来自确定性引擎。数据不足时系统会明确拒绝推荐。
          </Text>
          <Input.Search
            size="large"
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            onSearch={() => void research()}
            enterButton={
              <>
                <SearchOutlined /> 开始研究
              </>
            }
            loading={researching}
            placeholder="输入股票代码、方法名称，或自然语言问题"
          />
          {answer && <Alert type="info" showIcon icon={<RobotOutlined />} message="AI 研究结果" description={answer} />}
        </Space>
      </Card>

      {loading ? (
        <Spin />
      ) : !today ? (
        <Alert
          type="error"
          showIcon
          message="无法读取今日状态"
          description="后端未返回今日状态读模型。请确认服务已启动后重试。"
          action={
            <Button size="small" onClick={() => void load()}>
              重试
            </Button>
          }
        />
      ) : (
        <>
          {/* P5・系统健康状态面板 */}
          <SystemHealthPanel health={today.health} />

          {/* P3・首次引导 */}
          {showGuide ? (
            <OnboardingWizard
              onCompleted={() => {
                dismissGuide();
                void load();
              }}
            />
          ) : (
            <Space size={12} wrap>
              <Button size="small" type="link" onClick={() => setShowGuide(true)}>
                重新显示首次引导
              </Button>
            </Space>
          )}

          {seedResult && <SeedResultAlert result={seedResult} onDismiss={() => setSeedResult(undefined)} />}

          {/* P1・空状态分诊 + 下一步（有候选时不显示） */}
          {!hasCandidates && (
            <EmptyStatePanel
              empty={today.empty_state}
              busyKind={busy}
              onAction={(a) => void handleAction(a)}
            />
          )}

          {showExclusions && selection && (
            <ExclusionBreakdown counts={selection.exclusion_counts} samples={selection.sample_exclusions} />
          )}

          {/* P4・工作日志 */}
          <WorkLogPanel workLog={today.work_log} />

          {/* 今日候选 */}
          {selection && (
            <Card
              title={`今日候选 · ${selection.snapshot_date}`}
              extra={
                <Space size={12}>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    扫描 {selection.scanned_stocks} 只 · 通过资格方法 {selection.eligible_methods} 个
                  </Text>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    快照 {selection.snapshot_id}
                  </Text>
                </Space>
              }
            >
              {hasCandidates ? (
                <List
                  dataSource={candidates}
                  renderItem={(c) => (
                    <List.Item
                      actions={[
                        <Button key="detail" onClick={() => navigate(`/stock/${c.code}`)}>
                          查看个股
                        </Button>,
                      ]}
                    >
                      <List.Item.Meta
                        title={
                          <Space>
                            <Text code>{c.code}</Text>
                            <Tag color={c.action === 'buy' ? 'red' : 'orange'}>{c.action}</Tag>
                            <Tag>评分 {c.score.toFixed(2)}</Tag>
                          </Space>
                        }
                        description={
                          <Space direction="vertical" size={2}>
                            <span>{c.explanation}</span>
                            <span>
                              买入窗口：{c.buy_window}；仓位上限 {(c.position_cap_pct * 100).toFixed(0)}%
                            </span>
                            <span>退出计划：{c.exit.complete ? '完整' : '不完整，仅观察'}</span>
                          </Space>
                        }
                      />
                    </List.Item>
                  )}
                />
              ) : (
                <Text type="secondary">本次运行没有候选。上方「运行日志」列出了扫描与排除的真实明细。</Text>
              )}
            </Card>
          )}

          {/* 持仓卖出判断 */}
          {!positions ? (
            <Alert type="info" showIcon message="尚无持仓判断" description="没有真实持仓或尚未运行持仓决策。" />
          ) : (
            <Card
              title="持仓卖出判断"
              extra={
                <Space>
                  {urgent.length > 0 && <Tag color="red">{urgent.length} 个需处理</Tag>}
                  <Button onClick={() => navigate('/portfolio')}>查看持仓</Button>
                </Space>
              }
            >
              <List
                dataSource={positions.decisions}
                locale={{ emptyText: <Text type="secondary">当前没有真实持仓</Text> }}
                renderItem={(d) => (
                  <List.Item>
                    <List.Item.Meta
                      title={
                        <Space>
                          <b>
                            {d.code} {d.name}
                          </b>
                          <Tag color={d.action === 'exit' ? 'red' : d.action === 'reduce' ? 'orange' : 'blue'}>
                            {d.action}
                          </Tag>
                          {d.inferred && <Tag>推断依据</Tag>}
                        </Space>
                      }
                      description={`${d.explanation}；最迟：${d.deadline}${d.constraint ? `；约束：${d.constraint}` : ''}`}
                    />
                  </List.Item>
                )}
              />
            </Card>
          )}

          {/* P6・机制图解（帮助入口） */}
          <Collapse
            ghost
            items={[{ key: 'mechanism', label: '帮助：这套工具怎么思考？', children: <MethodMechanismDiagram /> }]}
          />

          {/* 快速开始 */}
          <Card title="快速开始" bordered={false}>
            <Row gutter={[16, 16]}>
              <QuickStartCard
                icon={<DashboardOutlined />}
                title="决策"
                desc="今天买什么，持仓什么时候卖"
                links={[
                  { to: '/', label: '今日决策' },
                  { to: '/portfolio', label: '持仓卖出' },
                ]}
              />
              <QuickStartCard
                icon={<RadarChartOutlined />}
                title="选股方法"
                desc="看历史验证、筛选今日股票，或挖掘新方法"
                links={[
                  { to: '/methods', label: '我的选股方法' },
                  { to: '/methods/advanced', label: '高级研究与管理' },
                  { to: '/agent', label: 'AI 助手' },
                ]}
              />
              <QuickStartCard
                icon={<StockOutlined />}
                title="行情工具"
                desc="自选股、个股与筛选工具"
                links={[
                  { to: '/watchlist', label: '自选股' },
                  { to: '/stock/choose', label: '个股分析' },
                  { to: '/blocks', label: '股票池' },
                  { to: '/screen', label: '信号筛选' },
                ]}
              />
              <QuickStartCard
                icon={<SettingOutlined />}
                title="系统"
                desc="参数与本地配置"
                links={[{ to: '/settings', label: '配置' }]}
              />
            </Row>
          </Card>

          <Card size="small">
            <Space>
              <SafetyCertificateOutlined />
              <Text type="secondary">
                本页面只消费真实 API。没有快照、证据或持仓时显示分诊后的空状态，不生成演示数据。
              </Text>
            </Space>
          </Card>
        </>
      )}
    </Space>
  );
}
