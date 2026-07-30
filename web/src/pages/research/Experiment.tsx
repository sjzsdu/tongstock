import { useEffect, useState } from 'react';
import { Card, Row, Col, Tag, Typography, Space, Table, Progress, Button, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ExperimentOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons';
import { api } from '../../api/client';
import type { ParadigmItem, ParadigmListResponse } from '../../types/api';

const { Title, Text, Paragraph } = Typography;

export default function Experiment() {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<ParadigmItem[]>([]);
  const [total, setTotal] = useState(0);

  const load = async () => {
    setLoading(true);
    try {
      const resp = await api.paradigmList(undefined, undefined, { limit: 100 });
      setData(resp.paradigms || []);
      setTotal(resp.total || 0);
    } catch (err) {
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  // 按审查状态分组统计
  const groups = {
    pending: data.filter(d => !d.review_status || d.review_status === 'pending').length,
    reviewed: data.filter(d => d.review_status === 'reviewed').length,
    promoted: data.filter(d => d.review_status === 'promoted').length,
    rejected: data.filter(d => d.review_status === 'rejected').length,
  };

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
      title: '审查状态', dataIndex: 'review_status', key: 'review_status', width: 90,
      render: (s: string) => {
        const map: Record<string, { color: string; label: string }> = {
          pending: { color: 'default', label: '待审查' },
          reviewed: { color: 'blue', label: '已审查' },
          promoted: { color: 'green', label: '已晋级' },
          rejected: { color: 'red', label: '已否决' },
        };
        const info = map[s || 'pending'];
        return <Tag color={info.color}>{info.label}</Tag>;
      },
    },
    {
      title: '结构完整性', key: 'score', width: 120,
      render: (_, r) => {
        const score = Math.round((r.validation?.auto_evaluable_ratio ?? 0) * 100);
        return <Progress percent={score} size="small" />;
      },
    },
    {
      title: '操作', key: 'action', width: 120,
      render: (_, r) => (
        <Space>
          <Button size="small" type="link" onClick={() => {
            window.location.href = `/research/candidates?id=${r.id}`;
          }}>审查</Button>
        </Space>
      ),
    },
  ];

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>实验筛选</Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          对假设生成的候选进行快速筛选和验证，淘汰不合格项
        </Paragraph>
      </div>

      {/* 统计 */}
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <Text type="secondary">总数</Text>
              <div><Text strong style={{ fontSize: 24 }}>{total}</Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <Text type="secondary">待审查</Text>
              <div><Text strong style={{ fontSize: 24, color: '#faad14' }}>{groups.pending}</Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <Text type="secondary">已审查</Text>
              <div><Text strong style={{ fontSize: 24, color: '#1677ff' }}>{groups.reviewed}</Text></div>
            </div>
          </Card>
        </Col>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <Text type="secondary">已晋级</Text>
              <div><Text strong style={{ fontSize: 24, color: '#52c41a' }}>{groups.promoted}</Text></div>
            </div>
          </Card>
        </Col>
      </Row>

      {/* 列表 */}
      <Card
        title={
          <Space>
            <ExperimentOutlined />
            <span>实验队列</span>
            <Tag>{data.length} 项</Tag>
          </Space>
        }
        extra={
          <Space>
            <Button icon={<SearchOutlined />} onClick={() => { window.location.href = '/screen'; }}>
              去筛选
            </Button>
            <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>
              刷新
            </Button>
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
