import { useEffect, useState } from 'react';
import { Card, Row, Col, Tag, Typography, Space, Table, Progress, Button, message, Drawer, Modal, Form, Select, Input, Descriptions, Timeline } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { SafetyCertificateOutlined, TrophyOutlined, FileSearchOutlined, AuditOutlined, ArrowUpOutlined, ArrowDownOutlined, PauseCircleOutlined, CloseCircleOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { api } from '../../api/client';
import type { ParadigmItem, EvidenceCard, ParadigmTransitionsResponse, ParadigmTransitionRequest } from '../../types/api';
import EvidenceCardView from '../../components/research/EvidenceCard';

const { Title, Text, Paragraph } = Typography;

const STATE_LABEL: Record<string, { label: string; color: string }> = {
  pending: { label: '候选', color: 'default' },
  reviewed: { label: '已评审', color: 'blue' },
  verified: { label: '已验证', color: 'cyan' },
  promoted: { label: '已晋级', color: 'gold' },
  degraded: { label: '已降级', color: 'orange' },
  suspended: { label: '已暂停', color: 'purple' },
  rejected: { label: '已淘汰', color: 'red' },
};

const STATE_ACTIONS: Record<string, { key: string; label: string; to: string; icon: React.ReactNode; danger?: boolean }[]> = {
  reviewed: [
    { key: 'verify', label: '验证通过', to: 'verified', icon: <ThunderboltOutlined /> },
    { key: 'reject', label: '淘汰', to: 'rejected', icon: <CloseCircleOutlined />, danger: true },
  ],
  verified: [
    { key: 'promote', label: '晋级', to: 'promoted', icon: <ArrowUpOutlined /> },
    { key: 'suspend', label: '暂停', to: 'suspended', icon: <PauseCircleOutlined /> },
    { key: 'downgrade', label: '降级', to: 'degraded', icon: <ArrowDownOutlined /> },
    { key: 'reject', label: '淘汰', to: 'rejected', icon: <CloseCircleOutlined />, danger: true },
  ],
  promoted: [
    { key: 'downgrade', label: '降级', to: 'degraded', icon: <ArrowDownOutlined /> },
    { key: 'suspend', label: '暂停', to: 'suspended', icon: <PauseCircleOutlined /> },
    { key: 'reject', label: '淘汰', to: 'rejected', icon: <CloseCircleOutlined />, danger: true },
  ],
  degraded: [
    { key: 'promote', label: '重新晋级', to: 'promoted', icon: <ArrowUpOutlined /> },
    { key: 'suspend', label: '暂停', to: 'suspended', icon: <PauseCircleOutlined /> },
    { key: 'reject', label: '淘汰', to: 'rejected', icon: <CloseCircleOutlined />, danger: true },
  ],
  suspended: [
    { key: 'resume', label: '恢复验证', to: 'verified', icon: <ThunderboltOutlined /> },
    { key: 'promote', label: '直接晋级', to: 'promoted', icon: <ArrowUpOutlined /> },
    { key: 'reject', label: '淘汰', to: 'rejected', icon: <CloseCircleOutlined />, danger: true },
  ],
  pending: [
    { key: 'review', label: '提交评审', to: 'reviewed', icon: <FileSearchOutlined /> },
    { key: 'reject', label: '淘汰', to: 'rejected', icon: <CloseCircleOutlined />, danger: true },
  ],
};

export default function Verified() {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<ParadigmItem[]>([]);
  const [evidenceDrawerOpen, setEvidenceDrawerOpen] = useState(false);
  const [transitionsDrawerOpen, setTransitionsDrawerOpen] = useState(false);
  const [transitionModalOpen, setTransitionModalOpen] = useState(false);
  const [transitionSubmitting, setTransitionSubmitting] = useState(false);
  const [transitionForm] = Form.useForm();
  const [evidenceLoading, setEvidenceLoading] = useState(false);
  const [currentEvidence, setCurrentEvidence] = useState<EvidenceCard | null>(null);
  const [currentId, setCurrentId] = useState<string>('');
  const [currentTransitions, setCurrentTransitions] = useState<ParadigmTransitionsResponse | null>(null);

  const load = async () => {
    setLoading(true);
    try {
      const resp = await api.paradigmList(undefined, undefined, { review_status: 'promoted', limit: 100 });
      setData(resp.paradigms || []);
    } catch (err) {
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { queueMicrotask(() => void load()); }, []);

  const loadEvidence = async (id: string) => {
    setCurrentId(id);
    setEvidenceLoading(true);
    setEvidenceDrawerOpen(true);
    try {
      const card = await api.paradigmEvidence(id);
      setCurrentEvidence(card);
    } catch (err) {
      message.error(String(err));
      setCurrentEvidence(null);
    } finally {
      setEvidenceLoading(false);
    }
  };

  const loadTransitions = async (id: string) => {
    setCurrentId(id);
    try {
      const res = await api.paradigmTransitions(id);
      setCurrentTransitions(res);
      setTransitionsDrawerOpen(true);
    } catch (err) {
      message.error(String(err));
    }
  };

  const openTransition = (id: string) => {
    setCurrentId(id);
    transitionForm.resetFields();
    setTransitionModalOpen(true);
  };

  const currentParadigm = data.find((d) => d.id === currentId);
  const availableActions = currentParadigm
    ? (STATE_ACTIONS[currentParadigm.review_status as string] || [])
    : [];

  const submitTransition = async () => {
    try {
      const values: ParadigmTransitionRequest = await transitionForm.validateFields();
      setTransitionSubmitting(true);
      const result = await api.paradigmTransition(currentId, {
        to: values.to,
        reason: values.reason,
        evidence_hash: values.evidence_hash,
        auto: false,
      });
      message.success(`已变更为 ${result.transition.to}`);
      setTransitionModalOpen(false);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '状态变更失败');
    } finally {
      setTransitionSubmitting(false);
    }
  };

  const columns: ColumnsType<ParadigmItem> = [
    { title: '范式名', dataIndex: 'name', key: 'name', width: 200, ellipsis: true },
    {
      title: '方向', dataIndex: 'side', key: 'side', width: 70,
      render: (s: string) => <Tag color={s === 'buy' ? 'green' : 'red'}>{s === 'buy' ? '买' : '卖'}</Tag>,
    },
    {
      title: '状态', key: 'status', width: 100,
      render: (_, r) => {
        const s = STATE_LABEL[r.review_status || 'pending'] || STATE_LABEL.pending;
        return <Tag color={s.color}>{s.label}</Tag>;
      },
    },
    {
      title: '可靠性', key: 'reliability', width: 100,
      render: (_, r) => r.validation?.reliability_label
        ? <Tag color={r.validation.reliability_label === 'high' ? 'green' : 'orange'}>{r.validation.reliability_label}</Tag>
        : <Tag>未知</Tag>,
    },
    {
      title: '回测胜率', key: 'winrate', width: 100,
      render: (_, r) => r.expectation?.win_rate
        ? <Progress percent={Math.round(r.expectation.win_rate * 100)} size="small" />
        : <Text type="secondary">--</Text>,
    },
    {
      title: '实际收益', dataIndex: 'actual_return', key: 'return', width: 100,
      render: (r: number) => {
        if (r === undefined || r === null) return <Text type="secondary">观察中</Text>;
        return <Text strong style={{ color: r >= 0 ? '#52c41a' : '#ff4d4f' }}>{(r * 100).toFixed(2)}%</Text>;
      },
    },
    {
      title: '晋级时间', dataIndex: 'updated_at', key: 'updated', width: 180,
      render: (v: string) => <Text type="secondary" style={{ fontSize: 12 }}>{v ? new Date(v).toLocaleString() : '--'}</Text>,
    },
    {
      title: '操作', key: 'action', width: 260,
      render: (_, r) => (
        <Space>
          <Button size="small" icon={<FileSearchOutlined />} onClick={() => loadEvidence(r.id)}>证据</Button>
          <Button size="small" icon={<AuditOutlined />} onClick={() => loadTransitions(r.id)}>状态机</Button>
          <Button size="small" type="primary" onClick={() => openTransition(r.id)}>变更状态</Button>
        </Space>
      ),
    },
  ];

  const highReliability = data.filter(d => d.validation?.reliability_label === 'high').length;
  const avgRating = data.length > 0 ? (data.reduce((s, d) => s + (d.review_rating || 0), 0) / data.length).toFixed(1) : '--';

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>已验证范式</Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          通过回归门验证、晋级生产的稳定范式，可用于前向观察。状态机管理范式的晋级、降级、暂停与淘汰。
        </Paragraph>
      </div>

      {/* 统计 */}
      <Row gutter={[16, 16]}>
        <Col xs={8}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <TrophyOutlined style={{ fontSize: 24, color: '#faad14' }} />
              <div><Text type="secondary">已晋级</Text></div>
              <div><Text strong style={{ fontSize: 20 }}>{data.length}</Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={8}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <SafetyCertificateOutlined style={{ fontSize: 24, color: '#52c41a' }} />
              <div><Text type="secondary">高可靠</Text></div>
              <div><Text strong style={{ fontSize: 20 }}>{highReliability}</Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={8}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <div><Text type="secondary">平均评分</Text></div>
              <div><Text strong style={{ fontSize: 20 }}>{avgRating}</Text></div>
            </div>
          </Card>
        </Col>
      </Row>

      {/* 列表 */}
      <Card
        title={
          <Space>
            <SafetyCertificateOutlined />
            <span>已验证范式库</span>
            <Tag>{data.length} 项</Tag>
          </Space>
        }
        extra={
          <Button type="primary" onClick={() => {
            window.location.href = '/research/observation';
          }}>
            进入前向观察 →
          </Button>
        }
      >
        <Table
          rowKey="id"
          columns={columns}
          dataSource={data}
          loading={loading}
          pagination={{ pageSize: 10, showSizeChanger: true }}
          size="middle"
        />
      </Card>

      <Drawer
        title="范式证据卡"
        width={800}
        open={evidenceDrawerOpen}
        onClose={() => setEvidenceDrawerOpen(false)}
      >
        {evidenceLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}>加载中...</div>
        ) : currentEvidence ? (
          <EvidenceCardView card={currentEvidence} />
        ) : (
          <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>暂无证据数据</div>
        )}
      </Drawer>

      <Drawer
        title={
          <Space>
            <AuditOutlined />
            <span>状态机审计</span>
          </Space>
        }
        width={640}
        open={transitionsDrawerOpen}
        onClose={() => setTransitionsDrawerOpen(false)}
      >
        {currentTransitions ? (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions column={1} size="small">
              <Descriptions.Item label="当前状态">
                <Tag color={STATE_LABEL[currentTransitions.current]?.color || 'default'}>
                  {STATE_LABEL[currentTransitions.current]?.label || currentTransitions.current}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="变更总数">{currentTransitions.total}</Descriptions.Item>
            </Descriptions>
            <Timeline
              items={currentTransitions.transitions.map((t) => ({
                color: t.auto ? 'gray' : 'blue',
                children: (
                  <Space direction="vertical" size={0}>
                    <Text strong>
                      {t.from} → <Tag color="blue">{t.to}</Tag>
                      <Tag style={{ marginInlineStart: 8 }}>{t.action}</Tag>
                      {t.auto && <Tag color="default" style={{ marginInlineStart: 4 }}>自动</Tag>}
                    </Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>{t.reason}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {new Date(t.created_at).toLocaleString()} · {t.actor || '系统'}
                    </Text>
                  </Space>
                ),
              }))}
            />
          </Space>
        ) : null}
      </Drawer>

      {/* 状态变更模态框 */}
      <Modal
        title={
          <Space>
            <AuditOutlined />
            <span>状态变更 - {currentParadigm?.name}</span>
          </Space>
        }
        open={transitionModalOpen}
        onCancel={() => setTransitionModalOpen(false)}
        onOk={() => void submitTransition()}
        confirmLoading={transitionSubmitting}
        width={560}
      >
        <Form form={transitionForm} layout="vertical">
          <Form.Item label="当前状态">
            <Tag color={STATE_LABEL[currentParadigm?.review_status || 'pending']?.color}>
              {STATE_LABEL[currentParadigm?.review_status || 'pending']?.label}
            </Tag>
          </Form.Item>
          <Form.Item
            label="目标状态"
            name="to"
            rules={[{ required: true, message: '请选择目标状态' }]}
          >
            <Select
              placeholder="选择目标状态"
              options={availableActions.map((a: { key: string; label: string; to: string }) => ({
                label: a.label,
                value: a.to,
              }))}
            />
          </Form.Item>
          <Form.Item
            label="变更原因"
            name="reason"
            rules={[{ required: true, message: '请填写变更原因 (将成为审计记录)' }]}
          >
            <Input.TextArea rows={3} placeholder="描述变更原因, 将作为审计记录的一部分" />
          </Form.Item>
          <Form.Item label="关联证据哈希 (可选)" name="evidence_hash">
            <Input placeholder="如留空则不关联新证据" />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  );
}
