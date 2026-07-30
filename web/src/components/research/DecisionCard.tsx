import { useState } from 'react';
import { Button, Card, Descriptions, Space, Tag, Typography, Divider, Empty, List, Modal, DatePicker, Form, Input, Select } from 'antd';
import { CheckCircleFilled, CloseCircleFilled, ExclamationCircleFilled, ThunderboltFilled, ClockCircleOutlined, FileTextOutlined, CalendarOutlined, AuditOutlined } from '@ant-design/icons';
import type { ParadigmDecisionCard, ParadigmTransitionRequest } from '../../types/api';
import { api } from '../../api/client';
import { message } from 'antd';

const { Text, Paragraph } = Typography;

const RELIABILITY_COLOR: Record<string, string> = {
  high: 'green',
  medium: 'orange',
  low: 'red',
};

const SIDE_COLOR: Record<string, string> = {
  buy: 'green',
  sell: 'red',
};

const STATUS_COLOR: Record<string, string> = {
  verified: 'green',
  promoted: 'gold',
  reviewed: 'blue',
  pending: 'default',
  degraded: 'orange',
  suspended: 'purple',
  rejected: 'red',
};

const STATE_ACTIONS: Record<string, { label: string; to: string }[]> = {
  verified: [
    { label: '晋级', to: 'promoted' },
    { label: '暂停', to: 'suspended' },
    { label: '降级', to: 'degraded' },
    { label: '淘汰', to: 'rejected' },
  ],
  promoted: [
    { label: '降级', to: 'degraded' },
    { label: '暂停', to: 'suspended' },
    { label: '淘汰', to: 'rejected' },
  ],
  degraded: [
    { label: '重新晋级', to: 'promoted' },
    { label: '暂停', to: 'suspended' },
    { label: '淘汰', to: 'rejected' },
  ],
  suspended: [
    { label: '恢复验证', to: 'verified' },
    { label: '直接晋级', to: 'promoted' },
    { label: '淘汰', to: 'rejected' },
  ],
  pending: [
    { label: '提交评审', to: 'reviewed' },
    { label: '淘汰', to: 'rejected' },
  ],
  reviewed: [
    { label: '验证通过', to: 'verified' },
    { label: '淘汰', to: 'rejected' },
  ],
};

interface Props {
  card: ParadigmDecisionCard;
  onViewEvidence?: () => void;
  onViewLineage?: () => void;
  onSave?: () => void;
  onPlan?: () => void;
  onTransition?: (id: string, req: ParadigmTransitionRequest) => Promise<void>;
}

export default function DecisionCard({ card, onViewEvidence, onViewLineage, onSave, onTransition }: Props) {
  const { triggers, invalidations } = card;
  const [planOpen, setPlanOpen] = useState(false);
  const [planForm] = Form.useForm();
  const [planSubmitting, setPlanSubmitting] = useState(false);
  const [transitionOpen, setTransitionOpen] = useState(false);
  const [transitionForm] = Form.useForm();
  const [transitionSubmitting, setTransitionSubmitting] = useState(false);

  const availableActions = STATE_ACTIONS[card.review_status] || [];

  const handlePlanSubmit = async () => {
    try {
      const values = await planForm.validateFields();
      setPlanSubmitting(true);
      // 保存到自选分组 paradigm, 并附加备注与观察计划
      await api.watchlistAdd(card.stock_code, card.stock_name || undefined, 'paradigm');
      await api.watchlistUpdateNote(
        card.stock_code,
        `范式计划: ${card.paradigm_name} | 版本 v${card.paradigm_version} | 目标日期: ${values.target_date || '无'} | 操作: ${values.action || '观察'} | ${values.note || ''}`,
      );
      message.success(`观察计划已保存至 ${card.stock_code}`);
      setPlanOpen(false);
      planForm.resetFields();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败');
    } finally {
      setPlanSubmitting(false);
    }
  };

  const handleTransitionSubmit = async () => {
    try {
      const values = await transitionForm.validateFields();
      const req: ParadigmTransitionRequest = {
        to: values.to,
        reason: values.reason,
        evidence_hash: values.evidence_hash,
        auto: false,
      };
      if (onTransition) {
        await onTransition(card.paradigm_id, req);
      } else {
        const res = await api.paradigmTransition(card.paradigm_id, req);
        message.success(`已变更为 ${res.transition.to}`);
      }
      setTransitionOpen(false);
      transitionForm.resetFields();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '状态变更失败');
    } finally {
      setTransitionSubmitting(false);
    }
  };

  return (
    <Card
      hoverable
      style={{
        borderLeft: `4px solid ${card.side === 'buy' ? '#52c41a' : '#ef4444'}`,
        marginBottom: 16,
      }}
      title={
        <Space>
          <ThunderboltFilled style={{ color: card.side === 'buy' ? '#52c41a' : '#ef4444' }} />
          <Text strong>{card.paradigm_name}</Text>
          <Tag color={SIDE_COLOR[card.side] || 'default'}>{card.side === 'buy' ? '多头' : '空头'}</Tag>
          <Tag color={STATUS_COLOR[card.review_status] || 'default'}>
            {card.review_status === 'verified' ? '已验证' : card.review_status === 'promoted' ? '已晋级' : card.review_status}
          </Tag>
          {!card.active && (
            <Tag color="warning">失效</Tag>
          )}
          <Text type="secondary" style={{ fontSize: 12 }}>v{card.paradigm_version || 1}</Text>
        </Space>
      }
      extra={
        <Space>
          <Button size="small" type="link" onClick={onViewEvidence} icon={<FileTextOutlined />}>
            证据
          </Button>
          <Button size="small" type="link" onClick={onViewLineage}>
            血缘
          </Button>
          <Button size="small" type="link" onClick={onSave}>
            加入观察
          </Button>
          <Button size="small" type="link" onClick={() => setTransitionOpen(true)} icon={<AuditOutlined />}>
            变更状态
          </Button>
          <Button size="small" type="primary" onClick={() => setPlanOpen(true)}>
            制定计划
          </Button>
        </Space>
      }
    >
      <Descriptions column={2} size="small">
        <Descriptions.Item label="股票">
          {card.stock_name || '--'} ({card.stock_code})
        </Descriptions.Item>
        <Descriptions.Item label="可靠性">
          <Tag color={RELIABILITY_COLOR[card.reliability] || 'default'}>
            {card.reliability || '未评估'}
          </Tag>
        </Descriptions.Item>
        <Descriptions.Item label="证据评分">
          {card.evidence_score > 0 ? card.evidence_score.toFixed(2) : '--'}
        </Descriptions.Item>
        <Descriptions.Item label="有效期">
          <Space>
            <ClockCircleOutlined />
            <Text>{card.ttl}</Text>
          </Space>
        </Descriptions.Item>
      </Descriptions>

      <Divider style={{ margin: '12px 0' }} />

      <Space direction="vertical" size={8} style={{ width: '100%' }}>
        <Text strong>触发条件</Text>
        {triggers.length === 0 ? (
          <Empty description="暂无明确触发条件" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <List
            size="small"
            dataSource={triggers}
            renderItem={(t) => (
              <List.Item>
                <Space>
                  <CheckCircleFilled style={{ color: '#52c41a' }} />
                  <Text>{t}</Text>
                </Space>
              </List.Item>
            )}
          />
        )}

        <Text strong>失效条件</Text>
        {invalidations.length === 0 ? (
          <Text type="secondary">未定义失效条件</Text>
        ) : (
          <List
            size="small"
            dataSource={invalidations}
            renderItem={(t) => (
              <List.Item>
                <Space>
                  <CloseCircleFilled style={{ color: '#ef4444' }} />
                  <Text type="warning">{t}</Text>
                </Space>
              </List.Item>
            )}
          />
        )}

        {!card.active && (
          <Paragraph type="warning" style={{ marginBottom: 0 }}>
            <ExclamationCircleFilled /> 该范式已失效或降级, 需重新评估
          </Paragraph>
        )}
      </Space>

      {/* 观察计划编辑模态框 */}
      <Modal
        title={
          <Space>
            <CalendarOutlined />
            <span>制定观察计划 - {card.paradigm_name}</span>
          </Space>
        }
        open={planOpen}
        onCancel={() => setPlanOpen(false)}
        onOk={() => void handlePlanSubmit()}
        confirmLoading={planSubmitting}
        width={560}
      >
        <Form form={planForm} layout="vertical" initialValues={{ action: '观察' }}>
          <Form.Item label="范式版本" style={{ marginBottom: 8 }}>
            <Tag color="blue">v{card.paradigm_version}</Tag>
            <Text type="secondary" style={{ marginInlineStart: 8 }}>{card.stock_code} ({card.stock_name})</Text>
          </Form.Item>
          <Form.Item label="计划目标日期" name="target_date">
            <DatePicker showTime style={{ width: '100%' }} placeholder="选择计划执行的目标日期" />
          </Form.Item>
          <Form.Item label="计划操作" name="action" rules={[{ required: true, message: '请选择或填写计划操作' }]}>
            <Input placeholder="如: 买入 / 观察 / 止盈 / 止损" />
          </Form.Item>
          <Form.Item label="计划备注" name="note">
            <Input.TextArea
              rows={3}
              placeholder="补充观察要点、失效条件等"
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* 状态变更模态框 */}
      <Modal
        title={
          <Space>
            <AuditOutlined />
            <span>变更状态 - {card.paradigm_name}</span>
          </Space>
        }
        open={transitionOpen}
        onCancel={() => setTransitionOpen(false)}
        onOk={() => void handleTransitionSubmit()}
        confirmLoading={transitionSubmitting}
        width={560}
      >
        <Form form={transitionForm} layout="vertical">
          <Form.Item label="当前状态">
            <Tag color={STATUS_COLOR[card.review_status] || 'default'}>
              {card.review_status}
            </Tag>
          </Form.Item>
          <Form.Item
            label="目标状态"
            name="to"
            rules={[{ required: true, message: '请选择目标状态' }]}
          >
            <Select
              placeholder="选择目标状态"
              options={availableActions.map((a) => ({ label: a.label, value: a.to }))}
            />
          </Form.Item>
          <Form.Item
            label="变更原因"
            name="reason"
            rules={[{ required: true, message: '请填写变更原因' }]}
          >
            <Input.TextArea rows={3} placeholder="描述变更原因, 将作为审计记录" />
          </Form.Item>
          <Form.Item label="关联证据哈希 (可选)" name="evidence_hash">
            <Input placeholder="留空则不关联新证据" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
