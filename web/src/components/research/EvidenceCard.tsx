import { Card, Row, Col, Tag, Typography, Space, Progress, Table, Divider, Alert, Descriptions } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  SafetyOutlined,
  WarningOutlined,
  CheckCircleFilled,
  CloseCircleFilled,
  InfoCircleOutlined,
  FundOutlined,
  FallOutlined,
  RiseOutlined,
  DatabaseOutlined,
} from '@ant-design/icons';
import type {
  EvidenceCard as EvidenceCardType,
  CounterExample,
  RiskFlag,
  TradeRecord,
  ScoreComponent,
  HardKillResult,
} from '../../types/api';

const { Title, Text, Paragraph } = Typography;

interface Props {
  card: EvidenceCardType;
}

function formatPercent(v: number | undefined, decimals = 2): string {
  if (v === undefined) return '未知';
  return `${(v * 100).toFixed(decimals)}%`;
}

function formatNumber(v: number | undefined, decimals = 2): string {
  if (v === undefined) return '未知';
  return v.toFixed(decimals);
}

function severityTag(severity: string) {
  const map: Record<string, string> = {
    critical: 'red',
    high: 'orange',
    medium: 'gold',
    low: 'blue',
  };
  return <Tag color={map[severity] || 'default'}>{severity}</Tag>;
}

function riskLevelTag(level: string) {
  const map: Record<string, string> = {
    critical: 'red',
    high: 'orange',
    medium: 'gold',
    low: 'blue',
  };
  return <Tag color={map[level] || 'default'}>{level}</Tag>;
}

function ScoreBar({ score }: { score: number }) {
  const color = score >= 0.7 ? '#52c41a' : score >= 0.4 ? '#faad14' : '#ff4d4f';
  return <Progress percent={Math.round(score * 100)} strokeColor={color} size="small" style={{ width: 120 }} />;
}

export default function EvidenceCardView({ card }: Props) {
  const sampleSizeNote = (card.in_sample?.sample_size ?? 0) < 30
    ? <Tag color="orange">样本量不足 ({card.in_sample?.sample_size ?? 0}/30)</Tag>
    : null;
  if (!card.available) {
    return (
      <Alert
        type="error"
        showIcon
        message="真实证据不可用"
        description={
          <Space direction="vertical">
            {(card.unavailable_reasons ?? ['没有完整的持久化实验证据']).map(reason => <Text key={reason}>{reason}</Text>)}
            <Text type="secondary">该范式当前不能晋级。</Text>
          </Space>
        }
      />
    );
  }
  const inSample = card.in_sample!;
  const outOfSample = card.out_of_sample!;
  const confidence = card.confidence_interval;
  const costs = card.cost_analysis!;
  const drawdown = card.drawdown_analysis!;
  const lineage = card.lineage!;

  const counterColumns: ColumnsType<CounterExample> = [
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 120,
      render: (t: string) => <Tag color="red">{t}</Tag>,
    },
    { title: '描述', dataIndex: 'description', key: 'description' },
    { title: '期间', dataIndex: 'period', key: 'period', width: 100 },
    {
      title: '收益',
      dataIndex: 'return',
      key: 'return',
      width: 100,
      render: (r?: number) => (
        <Text style={{ color: r !== undefined && r >= 0 ? '#52c41a' : '#ff4d4f' }}>
          {formatPercent(r)}
        </Text>
      ),
    },
    { title: '原因', dataIndex: 'reason', key: 'reason', width: 200 },
    {
      title: '严重度',
      dataIndex: 'severity',
      key: 'severity',
      width: 80,
      render: (s: string) => severityTag(s),
    },
  ];

  const tradeColumns: ColumnsType<TradeRecord> = [
    { title: '买入执行日', dataIndex: 'buy_execution_date', key: 'buy_execution_date', width: 120, render: (v: string) => new Date(v).toLocaleDateString() },
    { title: '卖出执行日', dataIndex: 'sell_execution_date', key: 'sell_execution_date', width: 120, render: (v: string) => new Date(v).toLocaleDateString() },
    { title: '分段', dataIndex: 'segment', key: 'segment', width: 80 },
    { title: '数量', dataIndex: 'quantity', key: 'quantity', width: 80 },
    { title: '买价', dataIndex: 'buy_price', key: 'buy_price', width: 80, render: (v: number) => formatNumber(v) },
    { title: '卖价', dataIndex: 'sell_price', key: 'sell_price', width: 80, render: (v: number) => formatNumber(v) },
    {
      title: '收益',
      dataIndex: 'return',
      key: 'return',
      width: 100,
      render: (r?: number) => (
        <Text style={{ color: r !== undefined && r >= 0 ? '#52c41a' : '#ff4d4f' }}>
          {formatPercent(r)}
        </Text>
      ),
    },
    { title: '净盈亏', dataIndex: 'net_pnl', key: 'net_pnl', render: (v: number) => formatNumber(v) },
  ];

  const scoreColumns: ColumnsType<ScoreComponent> = [
    { title: '组件', dataIndex: 'name', key: 'name', width: 120 },
    {
      title: '评分',
      dataIndex: 'score',
      key: 'score',
      width: 150,
      render: (s: number) => <ScoreBar score={s} />,
    },
    { title: '权重', dataIndex: 'weight', key: 'weight', width: 80, render: (v: number) => formatNumber(v, 3) },
    { title: '贡献', dataIndex: 'contribution', key: 'contribution', width: 80, render: (v: number) => formatNumber(v, 3) },
    {
      title: '通过',
      dataIndex: 'pass',
      key: 'pass',
      width: 70,
      render: (p: boolean) => p
        ? <CheckCircleFilled style={{ color: '#52c41a' }} />
        : <CloseCircleFilled style={{ color: '#ff4d4f' }} />,
    },
    { title: '说明', dataIndex: 'reason', key: 'reason' },
  ];

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      {/* 标题区 */}
      <Card>
        <Space direction="vertical" size={4} style={{ width: '100%' }}>
          <Space>
            <Title level={4} style={{ margin: 0 }}>{card.paradigm_name}</Title>
            <Tag color="blue">{card.stock_code}</Tag>
            {sampleSizeNote}
          </Space>
          <Paragraph type="secondary" style={{ margin: 0 }}>
            生成时间: {new Date(card.generated_at).toLocaleString()}
          </Paragraph>
        </Space>
      </Card>

      {!card.promotion_eligible && (
        <Alert
          type="warning"
          showIcon
          message="证据可追溯，但尚不满足晋级条件"
          description={
            <Space direction="vertical">
              {(card.promotion_blockers ?? []).map(reason => <Text key={reason}>{reason}</Text>)}
              <Text type="secondary">缺失指标保持“未知”，不会用默认值补齐。</Text>
            </Space>
          }
        />
      )}

      {/* 样本内/外对比 */}
      <Row gutter={[16, 16]}>
        <Col xs={12}>
          <Card
            title={
              <Space>
                <FundOutlined />
                <span>样本内结果</span>
              </Space>
            }
            size="small"
          >
            <Descriptions column={1} size="small">
              <Descriptions.Item label="样本量">{inSample.sample_size}</Descriptions.Item>
              <Descriptions.Item label="总收益">
                <Text style={{ color: inSample.total_return !== undefined && inSample.total_return >= 0 ? '#52c41a' : '#ff4d4f' }}>
                  {formatPercent(inSample.total_return)}
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label="年化">
                <Text strong>{formatPercent(inSample.annual_return)}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="Sharpe">{formatNumber(inSample.sharpe_ratio)}</Descriptions.Item>
              <Descriptions.Item label="胜率">{formatPercent(inSample.win_rate)}</Descriptions.Item>
              <Descriptions.Item label="最大回撤">
                <Text type="danger">{formatPercent(inSample.max_drawdown)}</Text>
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
        <Col xs={12}>
          <Card
            title={
              <Space>
                <FundOutlined />
                <span>样本外结果</span>
              </Space>
            }
            size="small"
          >
            <Descriptions column={1} size="small">
              <Descriptions.Item label="样本量">{outOfSample.sample_size}</Descriptions.Item>
              <Descriptions.Item label="总收益">
                <Text style={{ color: outOfSample.total_return !== undefined && outOfSample.total_return >= 0 ? '#52c41a' : '#ff4d4f' }}>
                  {formatPercent(outOfSample.total_return)}
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label="年化">
                <Text strong>{formatPercent(outOfSample.annual_return)}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="Sharpe">{formatNumber(outOfSample.sharpe_ratio)}</Descriptions.Item>
              <Descriptions.Item label="胜率">{formatPercent(outOfSample.win_rate)}</Descriptions.Item>
              <Descriptions.Item label="最大回撤">
                <Text type="danger">{formatPercent(outOfSample.max_drawdown)}</Text>
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
      </Row>

      {/* 置信区间 + 成本 + 回撤 */}
      <Row gutter={[16, 16]}>
        <Col xs={8}>
          <Card
            title={<Space><InfoCircleOutlined />置信区间</Space>}
            size="small"
          >
            <div style={{ textAlign: 'center', marginBottom: 8 }}>
              <Text type="secondary">95% CI 均值</Text>
              <div>
                <Text strong style={{ fontSize: 18 }}>
                  {formatPercent(confidence?.mean_return)}
                </Text>
              </div>
              <Text type="secondary">
                [{formatPercent(confidence?.ci_95_lower)}, {formatPercent(confidence?.ci_95_upper)}]
              </Text>
            </div>
            <div style={{ textAlign: 'center' }}>
              {confidence?.significant
                ? <Tag color="green">{'统计显著 (p < 0.05)'}</Tag>
                : <Tag color="orange">统计不显著</Tag>
              }
            </div>
            {confidence?.notes && confidence.notes.length > 0 && (
              <div style={{ marginTop: 8 }}>
                {confidence.notes.map((n, i) => (
                  <Alert key={i} message={n} type="warning" style={{ marginBottom: 4 }} />
                ))}
              </div>
            )}
          </Card>
        </Col>
        <Col xs={8}>
          <Card
            title={<Space><RiseOutlined />成本分析</Space>}
            size="small"
          >
            <Descriptions column={1} size="small">
              <Descriptions.Item label="毛收益">{formatPercent(costs.gross_return)}</Descriptions.Item>
              <Descriptions.Item label="净收益">
                <Text strong style={{ color: costs.net_return !== undefined && costs.net_return >= 0 ? '#52c41a' : '#ff4d4f' }}>
                  {formatPercent(costs.net_return)}
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label="成本占比">
                <Text type="warning">{formatPercent(costs.cost_ratio)}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="净保留率">{formatPercent(costs.net_retention)}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
        <Col xs={8}>
          <Card
            title={<Space><FallOutlined />回撤分析</Space>}
            size="small"
          >
            <Descriptions column={1} size="small">
              <Descriptions.Item label="最大回撤">
                <Text type="danger">{formatPercent(drawdown.max_drawdown)}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="回撤/收益比">
                <Text style={{ color: drawdown.drawdown_ratio !== undefined && drawdown.drawdown_ratio > 1 ? '#ff4d4f' : undefined }}>
                  {formatNumber(drawdown.drawdown_ratio)}
                </Text>
              </Descriptions.Item>
            </Descriptions>
            {drawdown.warning && (
              <Alert message={drawdown.warning} type="warning" style={{ marginTop: 8 }} />
            )}
          </Card>
        </Col>
      </Row>

      {/* 稳健性评分 + 阶段门 */}
      {card.robustness_score && (
        <Card
          title={
            <Space>
              <SafetyOutlined />
              <span>稳健性评分</span>
              <Tag color={card.robustness_score.stage === 'promote' ? 'green' : card.robustness_score.stage === 'observe' ? 'blue' : 'red'}>
                {card.robustness_score.stage === 'promote' ? '晋级' : card.robustness_score.stage === 'observe' ? '观察' : '拒绝'}
              </Tag>
            </Space>
          }
        >
          <Row gutter={[16, 16]}>
            <Col xs={12}>
              <div style={{ textAlign: 'center', marginBottom: 16 }}>
                <Text type="secondary">综合评分</Text>
                <div style={{ fontSize: 48, fontWeight: 'bold', color: card.robustness_score.final_score >= 0.65 ? '#52c41a' : card.robustness_score.final_score >= 0.5 ? '#1677ff' : '#ff4d4f' }}>
                  {formatNumber(card.robustness_score.final_score)}
                </div>
                {card.robustness_score.hard_killed && (
                  <Tag color="red"><WarningOutlined /> 硬性否决</Tag>
                )}
              </div>
              {card.robustness_score.hard_kills.length > 0 && (
                <div style={{ marginBottom: 16 }}>
                  <Text type="secondary">硬性否决项:</Text>
                  {card.robustness_score.hard_kills.map((k: HardKillResult, i: number) => (
                    <Alert key={i} message={k.reason} type="error" style={{ marginTop: 4 }} />
                  ))}
                </div>
              )}
            </Col>
            <Col xs={12}>
              <Table
                rowKey="category"
                columns={scoreColumns}
                dataSource={card.robustness_score.components}
                pagination={false}
                size="small"
              />
            </Col>
          </Row>
        </Card>
      )}

      {/* 反证 (始终展示) */}
      <Card
        title={
          <Space>
            <WarningOutlined style={{ color: '#ff4d4f' }} />
            <span style={{ color: '#ff4d4f' }}>反证与风险</span>
            <Tag>{card.counter_evidence.length} 项反证 · {card.risk_flags.length} 项风险</Tag>
          </Space>
        }
        style={{ borderColor: card.counter_evidence.length > 0 ? '#ff4d4f' : undefined }}
      >
        {card.counter_evidence.length > 0 ? (
          <Table
            rowKey={(r) => r.description}
            columns={counterColumns}
            dataSource={card.counter_evidence}
            pagination={false}
            size="small"
          />
        ) : (
          <Text type="secondary">暂无反证</Text>
        )}
        <Divider />
        <Text strong>风险标记:</Text>
        <div style={{ marginTop: 8 }}>
          {card.risk_flags.map((flag: RiskFlag, i: number) => (
            <Alert
              key={i}
              type={flag.level === 'critical' ? 'error' : flag.level === 'high' ? 'warning' : 'info'}
              message={
                <Space>
                  {riskLevelTag(flag.level)}
                  <span>{flag.message}</span>
                  {flag.mitigation && <Text type="secondary">· 缓解: {flag.mitigation}</Text>}
                </Space>
              }
              style={{ marginBottom: 8 }}
            />
          ))}
          {card.risk_flags.length === 0 && <Text type="secondary">暂无风险标记</Text>}
        </div>
      </Card>

      {/* 数据血缘 */}
      <Card
        title={
          <Space>
            <DatabaseOutlined />
            <span>数据血缘</span>
          </Space>
        }
        size="small"
      >
        <Descriptions column={2} size="small">
          <Descriptions.Item label="数据源">{lineage.data_source}</Descriptions.Item>
          <Descriptions.Item label="快照 ID">{lineage.snapshot_id}</Descriptions.Item>
          <Descriptions.Item label="实验 ID">{lineage.experiment_id}</Descriptions.Item>
          <Descriptions.Item label="运行 ID">{lineage.run_id}</Descriptions.Item>
          <Descriptions.Item label="数据范围">{lineage.data_range}</Descriptions.Item>
          <Descriptions.Item label="更新时间">
            {new Date(lineage.last_updated).toLocaleString()}
          </Descriptions.Item>
          <Descriptions.Item label="结果哈希">{lineage.result_hash}</Descriptions.Item>
          <Descriptions.Item label="快照哈希">{lineage.source_hash}</Descriptions.Item>
        </Descriptions>
      </Card>

      {/* 交易样本 (下钻) */}
      {card.trade_samples && card.trade_samples.length > 0 && (
        <Card
          title={
            <Space>
              <span>交易样本 (下钻)</span>
              <Tag>{card.trade_samples.length} 笔</Tag>
            </Space>
          }
          size="small"
        >
          <Table
            rowKey="trade_id"
            columns={tradeColumns}
            dataSource={card.trade_samples}
            pagination={{ pageSize: 10, size: 'small' }}
            size="small"
          />
        </Card>
      )}
    </Space>
  );
}
