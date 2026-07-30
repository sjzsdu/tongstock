import { useEffect, useState } from 'react';
import { Card, Row, Col, Tag, Typography, Space, Table, Progress, Button, message, Select, Drawer, Modal, Form, Input } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { AuditOutlined, CheckCircleOutlined, CloseCircleOutlined, FileSearchOutlined } from '@ant-design/icons';
import { api } from '../../api/client';
import type { ParadigmItem, EvidenceCard, ParadigmTransitionRequest } from '../../types/api';
import EvidenceCardView from '../../components/research/EvidenceCard';

const { Title, Text, Paragraph } = Typography;

const STATE_LABEL: Record<string, string> = {
  pending: '候选',
  reviewed: '已评审',
  verified: '已验证',
  promoted: '已晋级',
  degraded: '已降级',
  suspended: '已暂停',
  rejected: '已淘汰',
};

const STATE_ACTIONS: Record<string, { label: string; to: string }[]> = {
  pending: [
    { label: '提交评审', to: 'reviewed' },
    { label: '淘汰', to: 'rejected' },
  ],
  reviewed: [
    { label: '验证通过', to: 'verified' },
    { label: '淘汰', to: 'rejected' },
  ],
  verified: [
    { label: '晋级', to: 'promoted' },
    { label: '降级', to: 'degraded' },
    { label: '暂停', to: 'suspended' },
    { label: '淘汰', to: 'rejected' },
  ],
  promoted: [
    { label: '降级', to: 'degraded' },
    { label: '暂停', to: 'suspended' },
    { label: '淘汰', to: 'rejected' },
  ],
  degraded: [
    { label: '重新晋级', to: 'promoted' },
    { label: '淘汰', to: 'rejected' },
  ],
  suspended: [
    { label: '恢复验证', to: 'verified' },
    { label: '直接晋级', to: 'promoted' },
    { label: '淘汰', to: 'rejected' },
  ],
};

export default function Candidates() {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<ParadigmItem[]>([]);
  const [total, setTotal] = useState(0);
  const [reviewStatus, setReviewStatus] = useState<string | undefined>('reviewed');
  const [evidenceDrawerOpen, setEvidenceDrawerOpen] = useState(false);
  const [evidenceLoading, setEvidenceLoading] = useState(false);
  const [currentEvidence, setCurrentEvidence] = useState<EvidenceCard | null>(null);
  const [transitionModalOpen, setTransitionModalOpen] = useState(false);
  const [transitionSubmitting, setTransitionSubmitting] = useState(false);
  const [transitionForm] = Form.useForm();
  const [currentId, setCurrentId] = useState<string>('');

  const load = async () => {
    setLoading(true);
    try {
      const filters: Record<string, string | number | undefined> = { limit: 100 };
      if (reviewStatus) filters.review_status = reviewStatus;
      const resp = await api.paradigmList(undefined, undefined, filters);
      setData(resp.paradigms || []);
      setTotal(resp.total || 0);
    } catch (err) {
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, [reviewStatus]);

  const loadEvidence = async (id: string) => {
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

  const openTransition = (id: string) => {
    setCurrentId(id);
    transitionForm.resetFields();
    setTransitionModalOpen(true);
  };

  const submitTransition = async () => {
    try {
      const values: ParadigmTransitionRequest = await transitionForm.validateFields();
      setTransitionSubmitting(true);
      const res = await api.paradigmTransition(currentId, {
        to: values.to,
        reason: values.reason,
        evidence_hash: values.evidence_hash,
        auto: false,
      });
      message.success(`已变更为 ${res.transition.to}`);
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
      title: '状态', key: 'status', width: 110,
      render: (_, r) => <Tag>{STATE_LABEL[(r.review_status as string) || ''] || r.review_status || 'pending'}</Tag>,
    },
    {
      title: '可靠性', key: 'reliability', width: 90,
      render: (_, r) => r.validation?.reliability_label
        ? <Tag color={r.validation.reliability_label === 'high' ? 'green' : r.validation.reliability_label === 'medium' ? 'orange' : 'red'}>{r.validation.reliability_label}</Tag>
        : <Tag>未知</Tag>,
    },
    {
      title: '审查评级', dataIndex: 'review_rating', key: 'rating', width: 100,
      render: (r: number) => r ? <Progress percent={r * 20} size="small" /> : <Text type="secondary">未评级</Text>,
    },
    {
      title: '实际收益', dataIndex: 'actual_return', key: 'return', width: 100,
      render: (r: number) => {
        if (r === undefined || r === null) return <Text type="secondary">--</Text>;
        return <Text strong style={{ color: r >= 0 ? '#52c41a' : '#ff4d4f' }}>{(r * 100).toFixed(2)}%</Text>;
      },
    },
    {
      title: '审查备注', dataIndex: 'review_note', key: 'note', width: 200, ellipsis: true },
    {
      title: '操作', key: 'action', width: 180,
      render: (_, r) => (
        <Space>
          <Button size="small" icon={<FileSearchOutlined />} onClick={() => loadEvidence(r.id)}>证据</Button>
          <Button size="small" type="primary" onClick={() => openTransition(r.id)}>状态变更</Button>
        </Space>
      ),
    },
  ];

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>候选评估</Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          经 AI 反证审查的候选策略，等待人工确认晋级或否决
        </Paragraph>
      </div>

      {/* 统计 */}
      <Row gutter={[16, 16]}>
        <Col xs={8}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <CheckCircleOutlined style={{ fontSize: 24, color: '#52c41a' }} />
              <div><Text type="secondary">已审查</Text></div>
              <div><Text strong style={{ fontSize: 20 }}>{data.length}</Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={8}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <AuditOutlined style={{ fontSize: 24, color: '#1677ff' }} />
              <div><Text type="secondary">平均评分</Text></div>
              <div><Text strong style={{ fontSize: 20 }}>
                {(data.reduce((s, d) => s + (d.review_rating || 0), 0) / Math.max(data.length, 1)).toFixed(1)}
              </Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={8}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <CloseCircleOutlined style={{ fontSize: 24, color: '#ff4d4f' }} />
              <div><Text type="secondary">否决/总</Text></div>
              <div><Text strong style={{ fontSize: 20 }}>{data.filter(d => d.review_status === 'rejected').length}/{total}</Text></div>
            </div>
          </Card>
        </Col>
      </Row>

      {/* 列表 */}
      <Card
        title={
          <Space>
            <AuditOutlined />
            <span>候选队列</span>
            <Tag>{data.length} 项</Tag>
          </Space>
        }
        extra={
          <Space>
            <Select
              value={reviewStatus}
              onChange={setReviewStatus}
              options={[
                { value: '', label: '全部' },
                { value: 'pending', label: '候选' },
                { value: 'reviewed', label: '已评审' },
                { value: 'verified', label: '已验证' },
                { value: 'promoted', label: '已晋级' },
                { value: 'degraded', label: '已降级' },
                { value: 'suspended', label: '已暂停' },
                { value: 'rejected', label: '已淘汰' },
              ]}
              style={{ width: 120 }}
              allowClear
            />
            <Button onClick={load} loading={loading}>刷新</Button>
          </Space>
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

      {/* 状态变更模态框 */}
      <Modal
        title="状态变更"
        open={transitionModalOpen}
        onCancel={() => setTransitionModalOpen(false)}
        onOk={() => void submitTransition()}
        confirmLoading={transitionSubmitting}
        width={560}
      >
        <Form form={transitionForm} layout="vertical">
          <Form.Item
            label="目标状态"
            name="to"
            rules={[{ required: true, message: '请选择目标状态' }]}
          >
            <Select
              placeholder="选择目标状态 (根据当前状态机合法迁移)"
              options={(() => {
                const cur = data.find((d) => d.id === currentId);
                const actions = STATE_ACTIONS[(cur?.review_status as string) || ''] || [];
                return actions.map((a) => ({ label: a.label, value: a.to }));
              })()}
            />
          </Form.Item>
          <Form.Item
            label="变更原因"
            name="reason"
            rules={[{ required: true, message: '请填写变更原因 (将成为审计记录)' }]}
          >
            <Input.TextArea rows={3} placeholder="描述变更原因" />
          </Form.Item>
          <Form.Item label="关联证据哈希 (可选)" name="evidence_hash">
            <Input placeholder="如留空则不关联新证据" />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  );
}
