import { useEffect, useState } from 'react';
import { Card, Row, Col, Tag, Typography, Space, Alert, Table, Progress, Button, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { EyeOutlined, WarningOutlined, CheckCircleFilled } from '@ant-design/icons';
import { api } from '../../api/client';
import type { ParadigmAlertItem, ParadigmAlertsResponse, ParadigmItem } from '../../types/api';

const { Title, Text, Paragraph } = Typography;

export default function Observation() {
  const [loading, setLoading] = useState(false);
  const [alerts, setAlerts] = useState<ParadigmAlertItem[]>([]);
  const [total, setTotal] = useState(0);
  const [paradigms, setParadigms] = useState<ParadigmItem[]>([]);

  const load = async () => {
    setLoading(true);
    try {
      const [alertResp, paradigmResp] = await Promise.all([
        api.paradigmAlerts(),
        api.paradigmList(undefined, undefined, { review_status: 'promoted', limit: 50 }),
      ]);
      setAlerts(alertResp.alerts || []);
      setTotal(alertResp.total || 0);
      setParadigms(paradigmResp.paradigms || []);
    } catch (err) {
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  // 按严重程度分组
  const criticalCount = alerts.filter(a => a.severity === 'critical').length;
  const warningCount = alerts.filter(a => a.severity === 'warning').length;
  const infoCount = alerts.filter(a => a.severity === 'info').length;

  const alertColumns: ColumnsType<ParadigmAlertItem> = [
    {
      title: '严重度', dataIndex: 'severity', key: 'severity', width: 90,
      render: (s: string) => {
        const colors: Record<string, string> = { critical: 'red', warning: 'orange', info: 'blue' };
        return <Tag color={colors[s] || 'default'}>{s}</Tag>;
      },
    },
    { title: '范式', dataIndex: 'paradigm_id', key: 'pid', width: 160, ellipsis: true },
    { title: '股票', dataIndex: 'stock_code', key: 'code', width: 80 },
    { title: '类型', dataIndex: 'type', key: 'type', width: 100 },
    { title: '条件', dataIndex: 'condition', key: 'condition', width: 180, ellipsis: true },
    { title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (s: string) => <Tag>{s}</Tag>,
    },
  ];

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>前向观察</Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          不可回填的实时信号监测，追踪已验证范式的实时表现与风险状态
        </Paragraph>
      </div>

      {/* 统计 */}
      <Row gutter={[16, 16]}>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <EyeOutlined style={{ fontSize: 24, color: '#1677ff' }} />
              <div><Text type="secondary">监测中</Text></div>
              <div><Text strong style={{ fontSize: 20 }}>{paradigms.length}</Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <WarningOutlined style={{ fontSize: 24, color: '#faad14' }} />
              <div><Text type="secondary">警告</Text></div>
              <div><Text strong style={{ fontSize: 20, color: '#faad14' }}>{warningCount}</Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <WarningOutlined style={{ fontSize: 24, color: '#ff4d4f' }} />
              <div><Text type="secondary">严重</Text></div>
              <div><Text strong style={{ fontSize: 20, color: '#ff4d4f' }}>{criticalCount}</Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <CheckCircleFilled style={{ fontSize: 24, color: '#13c2c2' }} />
              <div><Text type="secondary">通知</Text></div>
              <div><Text strong style={{ fontSize: 20 }}>{infoCount}</Text></div>
            </div>
          </Card>
        </Col>
      </Row>

      {/* 严重警告提示 */}
      {criticalCount > 0 && (
        <Alert
          type="error"
          showIcon
          icon={<WarningOutlined />}
          message={`发现 ${criticalCount} 条严重警告`}
          description="多个已验证范式出现严重异常，建议立即检查相关持仓"
          action={<Button danger size="small" onClick={() => {
            window.location.href = '/portfolio';
          }}>查看持仓</Button>}
        />
      )}

      {/* 信号监控表 */}
      <Card
        title={
          <Space>
            <EyeOutlined />
            <span>实时信号监控</span>
            <Tag>{total} 条</Tag>
          </Space>
        }
        extra={<Button onClick={load} loading={loading}>刷新</Button>}
      >
        <Table
          rowKey={(r) => `${r.paradigm_id}-${r.stock_code}-${r.type}`}
          columns={alertColumns}
          dataSource={alerts}
          loading={loading}
          pagination={{ pageSize: 10, showSizeChanger: true }}
          size="middle"
        />
      </Card>

      {/* 前向表现 */}
      <Card title="前向表现追踪">
        {paradigms.length === 0 ? (
          <Text type="secondary">暂无已晋级范式在观察中</Text>
        ) : (
          <Row gutter={[16, 16]}>
            {paradigms.map((p) => (
              <Col xs={24} sm={12} lg={8} key={p.id}>
                <Card size="small">
                  <Space direction="vertical" size={4} style={{ width: '100%' }}>
                    <Text strong ellipsis style={{ maxWidth: 200 }}>{p.name}</Text>
                    <Space>
                      <Tag color={p.side === 'buy' ? 'green' : 'red'}>{p.side === 'buy' ? '买' : '卖'}</Tag>
                      <Tag color={p.validation?.reliability_label === 'high' ? 'green' : 'orange'}>
                        {p.validation?.reliability_label || '未知'}
                      </Tag>
                    </Space>
                    {p.actual_return !== undefined && (
                      <div>
                        <Text type="secondary" style={{ fontSize: 12 }}>实际收益</Text>
                        <div>
                          <Text style={{
                            color: p.actual_return >= 0 ? '#52c41a' : '#ff4d4f',
                            fontSize: 18,
                            fontWeight: 600,
                          }}>
                            {(p.actual_return * 100).toFixed(2)}%
                          </Text>
                        </div>
                      </div>
                    )}
                    {p.expectation?.win_rate !== undefined && (
                      <Progress
                        percent={Math.round((p.expectation?.win_rate ?? 0) * 100)}
                        size="small"
                        format={() => `胜率 ${((p.expectation?.win_rate ?? 0) * 100).toFixed(0)}%`}
                      />
                    )}
                  </Space>
                </Card>
              </Col>
            ))}
          </Row>
        )}
      </Card>
    </Space>
  );
}
