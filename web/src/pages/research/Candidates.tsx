import { useEffect, useState } from 'react';
import { Card, Row, Col, Tag, Typography, Space, Table, Progress, Button, message, Select } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { AuditOutlined, CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import { api } from '../../api/client';
import type { ParadigmItem, ParadigmListResponse } from '../../types/api';

const { Title, Text, Paragraph } = Typography;

export default function Candidates() {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<ParadigmItem[]>([]);
  const [total, setTotal] = useState(0);
  const [reviewStatus, setReviewStatus] = useState<string | undefined>('reviewed');

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

  const columns: ColumnsType<ParadigmItem> = [
    { title: '范式名', dataIndex: 'name', key: 'name', width: 200, ellipsis: true },
    {
      title: '方向', dataIndex: 'side', key: 'side', width: 70,
      render: (s: string) => <Tag color={s === 'buy' ? 'green' : 'red'}>{s === 'buy' ? '买' : '卖'}</Tag>,
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
      title: '操作', key: 'action', width: 100,
      render: (_, r) => (
        <Space>
          {r.review_status === 'reviewed' && (
            <Button size="small" type="primary" onClick={async () => {
              try {
                await api.paradigmReview(r.id, { review_status: 'promoted' });
                message.success('已晋级');
                load();
              } catch (err) {
                message.error(String(err));
              }
            }}>晋级</Button>
          )}
          <Button size="small" danger onClick={async () => {
            try {
              await api.paradigmReview(r.id, { review_status: 'rejected' });
              message.success('已否决');
              load();
            } catch (err) {
              message.error(String(err));
            }
          }}>否决</Button>
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
                { value: 'pending', label: '待审查' },
                { value: 'reviewed', label: '已审查' },
                { value: 'promoted', label: '已晋级' },
                { value: 'rejected', label: '已否决' },
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
    </Space>
  );
}
