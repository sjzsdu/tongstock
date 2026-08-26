import { useEffect, useState } from 'react';
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  Progress,
  Row,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  BugOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  InfoCircleOutlined,
  LoadingOutlined,
  SafetyCertificateOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import type {
  AlertItem,
  AlertSummary,
  ConcentrationResult,
  DecayDetectionResult,
  DriftDetectionResult,
  MonitoringReport,
} from '../types/api';
import { api } from '../api/client';

const { Title, Text } = Typography;

// 严重度颜色映射
const severityColor: Record<string, string> = {
  normal: 'green',
  mild: 'blue',
  watch: 'gold',
  active: 'orange',
  warning: 'gold',
  early: 'orange',
  moderate: 'orange',
  critical: 'red',
  severe: 'red',
  insufficient_data: 'default',
};

const levelColor: Record<string, string> = {
  critical: 'red',
  danger: 'volcano',
  warning: 'gold',
  info: 'blue',
};

const levelIcon: Record<string, React.ReactNode> = {
  critical: <CloseCircleOutlined />,
  danger: <BugOutlined />,
  warning: <WarningOutlined />,
  info: <InfoCircleOutlined />,
};

const statusColor: Record<string, string> = {
  active: 'red',
  acknowledged: 'gold',
  resolved: 'green',
  suppressed: 'default',
};

export default function Monitoring() {
  const [loading, setLoading] = useState(true);
  const [report, setReport] = useState<MonitoringReport | null>(null);
  const [alerts, setAlerts] = useState<AlertItem[]>([]);
  const [alertSummary, setAlertSummary] = useState<AlertSummary | null>(null);

  const loadData = async () => {
    setLoading(true);
    try {
      const [reportResp, alertsResp] = await Promise.all([
        api.monitoringReport().catch(() => null),
        api.monitoringAlerts().catch(() => null),
      ]);
      if (reportResp) {
        setReport(reportResp.report);
      }
      if (alertsResp) {
        setAlerts(alertsResp.alerts);
        setAlertSummary(alertsResp.summary);
      }
    } catch (err) {
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    queueMicrotask(() => void loadData());
  }, []);

  const handleAckAlert = async (id: string) => {
    try {
      await api.monitoringAlertAck(id, 'admin');
      message.success('预警已确认');
      const alertsResp = await api.monitoringAlerts();
      setAlerts(alertsResp.alerts);
      setAlertSummary(alertsResp.summary);
    } catch (err) {
      message.error(String(err));
    }
  };

  const handleResolveAlert = async (id: string) => {
    try {
      await api.monitoringAlertResolve(id);
      message.success('预警已解决');
      const alertsResp = await api.monitoringAlerts();
      setAlerts(alertsResp.alerts);
      setAlertSummary(alertsResp.summary);
    } catch (err) {
      message.error(String(err));
    }
  };

  if (loading && !report) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <LoadingOutlined style={{ fontSize: 48 }} />
        <p style={{ marginTop: 16 }}>加载监控数据中...</p>
      </div>
    );
  }

  if (!report) {
    return (
      <div style={{ padding: 24 }}>
        <Card>
          <Space direction="vertical" style={{ width: '100%', justifyContent: 'center', alignItems: 'center' }}>
            <SafetyCertificateOutlined style={{ fontSize: 48, color: '#1890ff' }} />
            <Title level={4}>暂无监控数据</Title>
            <Text>尚未收到真实前向收益和持仓观测，系统不会生成模拟监控报告。</Text>
            <Text type="secondary">请先由真实账本或研究任务提交带来源和观测日期的监控输入。</Text>
            <Button type="primary" onClick={loadData}>刷新</Button>
          </Space>
        </Card>
      </div>
    );
  }

  const healthColor =
    report.health_score >= 70 ? 'green' : report.health_score >= 40 ? 'orange' : 'red';
  const healthStatus =
    report.health_score >= 70 ? '良好' : report.health_score >= 40 ? '警告' : '危险';

  return (
    <div style={{ padding: 24 }}>
      {/* 顶部概览 */}
      <Card
        style={{ marginBottom: 16 }}
        title={
          <Space>
            <SafetyCertificateOutlined />
            <span>范式监控概览</span>
          </Space>
        }
        extra={
          <Space>
            <Button onClick={loadData}>刷新</Button>
          </Space>
        }
      >
        <Row gutter={16}>
          <Col span={4}>
            <Card>
              <Statistic
                title="健康分数"
                value={report.health_score.toFixed(1)}
                prefix={<Tag color={healthColor}>{healthStatus}</Tag>}
              />
              <Progress
                percent={Math.round(report.health_score)}
                strokeColor={healthColor}
                showInfo={false}
              />
            </Card>
          </Col>
          <Col span={4}>
            <Card>
              <Statistic
                title="漂移检测"
                value={report.drift_summary.overall_status}
                prefix={
                  <Tag color={severityColor[report.drift_summary.overall_status]}>
                    {report.drift_summary.severe_count > 0
                      ? '严重'
                      : report.drift_summary.significant_count > 0
                      ? '警告'
                      : '正常'}
                  </Tag>
                }
              />
              <Text type="secondary">
                {report.drift_summary.significant_count} 项显著 / {report.drift_summary.total_detections} 项检测
              </Text>
            </Card>
          </Col>
          <Col span={4}>
            <Card>
              <Statistic
                title="衰减检测"
                value={report.decay_summary.overall_status}
                prefix={
                  <Tag color={severityColor[report.decay_summary.overall_status]}>
                    {report.decay_summary.critical_count > 0
                      ? '严重'
                      : report.decay_summary.decaying_count > 0
                      ? '警告'
                      : '正常'}
                  </Tag>
                }
              />
              <Text type="secondary">
                {report.decay_summary.decaying_count} 项衰减 / {report.decay_summary.total_detections} 项检测
              </Text>
            </Card>
          </Col>
          <Col span={4}>
            <Card>
              <Statistic
                title="集中度"
                value={report.concentration_summary.overall_status}
                prefix={
                  <Tag color={severityColor[report.concentration_summary.overall_status]}>
                    {report.concentration_summary.critical_count > 0
                      ? '严重'
                      : report.concentration_summary.concentrated_count > 0
                      ? '警告'
                      : '正常'}
                  </Tag>
                }
              />
              <Text type="secondary">
                平均 HHI: {report.concentration_summary.avg_hhi.toFixed(3)}
              </Text>
            </Card>
          </Col>
          <Col span={4}>
            <Card>
              <Statistic
                title="预警汇总"
                value={alertSummary?.total_alerts || report.alert_summary.total_alerts}
                prefix={
                  (alertSummary?.critical_count ?? 0) > 0 || report.alert_summary.critical_count > 0 ? (
                    <Tag color="red">
                      <WarningOutlined /> {alertSummary?.critical_count || report.alert_summary.critical_count}
                    </Tag>
                  ) : (
                    <Tag color="green">正常</Tag>
                  )
                }
              />
              <Text type="secondary">
                活跃 {alertSummary?.active_count || report.alert_summary.active_count} /
                已确认 {alertSummary?.acked_count || report.alert_summary.acked_count}
              </Text>
            </Card>
          </Col>
          <Col span={4}>
            <Card>
              <Statistic title="监控周期" value={`${report.period.window_days} 天`} />
              <Text type="secondary">
                {report.period.start_date?.slice(0, 10)} ~ {report.period.end_date?.slice(0, 10)}
              </Text>
            </Card>
          </Col>
        </Row>
      </Card>

      {/* 建议 */}
      {report.recommendations.length > 0 && (
        <Card title="监控建议" style={{ marginBottom: 16 }}>
          {report.recommendations.map((rec, idx) => (
            <Alert
              key={idx}
              type={idx === 0 ? 'error' : 'warning'}
              message={rec}
              showIcon
              style={{ marginBottom: 8 }}
            />
          ))}
        </Card>
      )}

      {/* 漂移检测 */}
      <Card
        title={
          <Space>
            <BugOutlined />
            <span>分布漂移检测</span>
            <Tag color={severityColor[report.drift_summary.overall_status]}>
              {report.drift_summary.overall_status}
            </Tag>
          </Space>
        }
        style={{ marginBottom: 16 }}
      >
        <DriftTable results={report.drift_results} />
      </Card>

      {/* 衰减检测 */}
      <Card
        title={
          <Space>
            <ExclamationCircleOutlined />
            <span>性能衰减检测</span>
            <Tag color={severityColor[report.decay_summary.overall_status]}>
              {report.decay_summary.overall_status}
            </Tag>
          </Space>
        }
        style={{ marginBottom: 16 }}
      >
        <DecayTable results={report.decay_results} />
      </Card>

      {/* 集中度监控 */}
      <Card
        title={
          <Space>
            <WarningOutlined />
            <span>风险集中度监控</span>
            <Tag color={severityColor[report.concentration_summary.overall_status]}>
              {report.concentration_summary.overall_status}
            </Tag>
          </Space>
        }
        style={{ marginBottom: 16 }}
      >
        <ConcentrationTable results={report.concentration_results} />
      </Card>

      {/* 预警管理 */}
      <Card
        title={
          <Space>
            <SafetyCertificateOutlined />
            <span>预警管理</span>
            {alertSummary && (
              <Badge count={alertSummary.active_count} showZero>
                <Tag color="red">活跃</Tag>
              </Badge>
            )}
          </Space>
        }
      >
        <AlertManagement
          alerts={alerts}
          onAck={handleAckAlert}
          onResolve={handleResolveAlert}
        />
      </Card>
    </div>
  );
}

// ============================================================================
// 子组件
// ============================================================================

function DriftTable({ results }: { results: DriftDetectionResult[] }) {
  if (results.length === 0) {
    return <Text type="secondary">暂无漂移检测结果</Text>;
  }

  return (
    <Table
      size="small"
      rowKey={(r) => `${r.type}-${r.metric_name}`}
      columns={[
        {
          title: '类型',
          dataIndex: 'type',
          width: 120,
          render: (v: string) => <Tag>{v}</Tag>,
        },
        {
          title: '指标',
          dataIndex: 'metric_name',
          width: 140,
        },
        {
          title: '基准',
          dataIndex: 'old_value',
          width: 100,
          render: (v: number) => v.toFixed(4),
        },
        {
          title: '前向',
          dataIndex: 'new_value',
          width: 100,
          render: (v: number) => v.toFixed(4),
        },
        {
          title: '变化',
          dataIndex: 'delta_pct',
          width: 100,
          render: (v: number) => (
            <Text style={{ color: v > 0 ? '#ef4444' : v < 0 ? '#22c55e' : undefined }}>
              {v > 0 ? '+' : ''}
              {(v * 100).toFixed(2)}%
            </Text>
          ),
        },
        {
          title: 'p-value',
          dataIndex: 'p_value',
          width: 100,
          render: (v: number) => v.toFixed(4),
        },
        {
          title: '严重度',
          dataIndex: 'severity',
          width: 100,
          render: (v: string) => <Tag color={severityColor[v]}>{v}</Tag>,
        },
        {
          title: '描述',
          dataIndex: 'description',
          render: (v: string) => <Text>{v}</Text>,
        },
      ]}
      dataSource={results}
      pagination={false}
    />
  );
}

function DecayTable({ results }: { results: DecayDetectionResult[] }) {
  if (results.length === 0) {
    return <Text type="secondary">暂无衰减检测结果</Text>;
  }

  return (
    <Table
      size="small"
      rowKey={(r) => r.type}
      columns={[
        {
          title: '类型',
          dataIndex: 'type',
          width: 160,
          render: (v: string) => <Tag>{v}</Tag>,
        },
        {
          title: '状态',
          dataIndex: 'is_decaying',
          width: 100,
          render: (v: boolean) =>
            v ? (
              <Tag color="red">
                <WarningOutlined /> 衰减中
              </Tag>
            ) : (
              <Tag color="green">
                <CheckCircleOutlined /> 正常
              </Tag>
            ),
        },
        {
          title: '当前值',
          dataIndex: 'current_value',
          width: 100,
          render: (v: number) => v.toFixed(4),
        },
        {
          title: '历史均值',
          dataIndex: 'historical_avg',
          width: 100,
          render: (v: number) => v.toFixed(4),
        },
        {
          title: '变化',
          dataIndex: 'change_pct',
          width: 100,
          render: (v: number) => (
            <Text style={{ color: v > 0 ? '#ef4444' : v < 0 ? '#22c55e' : undefined }}>
              {v > 0 ? '+' : ''}
              {(v * 100).toFixed(2)}%
            </Text>
          ),
        },
        {
          title: '严重度',
          dataIndex: 'severity',
          width: 100,
          render: (v: string) => <Tag color={severityColor[v]}>{v}</Tag>,
        },
        {
          title: '置信度',
          dataIndex: 'confidence',
          width: 120,
          render: (v: number) => (
            <Progress percent={Math.round(v * 100)} size="small" showInfo={false} />
          ),
        },
        {
          title: '描述',
          dataIndex: 'description',
          render: (v: string) => <Text>{v}</Text>,
        },
      ]}
      dataSource={results}
      pagination={false}
    />
  );
}

function ConcentrationTable({ results }: { results: ConcentrationResult[] }) {
  if (results.length === 0) {
    return <Text type="secondary">暂无集中度检测结果</Text>;
  }

  return (
    <Table
      size="small"
      rowKey={(r) => r.type}
      columns={[
        {
          title: '类型',
          dataIndex: 'type',
          width: 140,
          render: (v: string) => <Tag>{v}</Tag>,
        },
        {
          title: 'HHI',
          dataIndex: 'hhi',
          width: 100,
          render: (v: number) => (
            <Text strong style={{ color: v > 0.5 ? 'red' : v > 0.25 ? 'orange' : 'green' }}>
              {v.toFixed(3)}
            </Text>
          ),
        },
        {
          title: '有效标的数',
          dataIndex: 'effective_count',
          width: 120,
          render: (v: number) => v.toFixed(1),
        },
        {
          title: '最大贡献者',
          dataIndex: 'top_contributor',
          width: 120,
        },
        {
          title: '权重',
          dataIndex: 'top_weight',
          width: 100,
          render: (v: number) => `${(v * 100).toFixed(1)}%`,
        },
        {
          title: '严重度',
          dataIndex: 'severity',
          width: 100,
          render: (v: string) => <Tag color={severityColor[v]}>{v}</Tag>,
        },
        {
          title: '描述',
          dataIndex: 'description',
          render: (v: string) => <Text>{v}</Text>,
        },
      ]}
      dataSource={results}
      pagination={false}
    />
  );
}

function AlertManagement({
  alerts,
  onAck,
  onResolve,
}: {
  alerts: AlertItem[];
  onAck: (id: string) => void;
  onResolve: (id: string) => void;
}) {
  if (alerts.length === 0) {
    return (
      <Space direction="vertical" style={{ width: '100%', alignItems: 'center', padding: 24 }}>
        <CheckCircleOutlined style={{ fontSize: 48, color: '#52c41a' }} />
        <Text strong style={{ fontSize: 16 }}>
          系统运行正常
        </Text>
        <Text type="secondary">暂无活跃预警</Text>
      </Space>
    );
  }

  // 按级别排序
  const sortedAlerts = [...alerts].sort((a, b) => {
    const order: Record<string, number> = { critical: 4, danger: 3, warning: 2, info: 1 };
    return (order[b.level] || 0) - (order[a.level] || 0);
  });

  return (
    <Table
      size="small"
      rowKey={(r) => r.id}
      columns={[
        {
          title: '级别',
          dataIndex: 'level',
          width: 100,
          render: (v: string) => (
            <Tag color={levelColor[v]} icon={levelIcon[v]}>
              {v}
            </Tag>
          ),
        },
        {
          title: '分类',
          dataIndex: 'category',
          width: 100,
          render: (v: string) => <Tag>{v}</Tag>,
        },
        {
          title: '状态',
          dataIndex: 'status',
          width: 100,
          render: (v: string) => <Tag color={statusColor[v]}>{v}</Tag>,
        },
        {
          title: '标题',
          dataIndex: 'title',
          width: 160,
        },
        {
          title: '描述',
          dataIndex: 'message',
          width: 260,
          render: (v: string) => <Text>{v}</Text>,
        },
        {
          title: '来源',
          dataIndex: 'source',
          width: 120,
        },
        {
          title: '时间',
          dataIndex: 'created_at',
          width: 160,
          render: (v: string) => v?.slice(0, 19)?.replace('T', ' ') || '-',
        },
        {
          title: '操作',
          width: 140,
          render: (_: unknown, r: AlertItem) => (
            <Space>
              {r.status === 'active' && (
                <Button size="small" onClick={() => onAck(r.id)}>
                  确认
                </Button>
              )}
              {r.status !== 'resolved' && (
                <Button size="small" type="primary" onClick={() => onResolve(r.id)}>
                  解决
                </Button>
              )}
            </Space>
          ),
        },
      ]}
      dataSource={sortedAlerts}
      pagination={{ pageSize: 10 }}
    />
  );
}
