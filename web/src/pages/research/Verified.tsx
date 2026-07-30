import { useEffect, useState } from 'react';
import { Card, Row, Col, Tag, Typography, Space, Table, Progress, Button, message, Drawer } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { SafetyCertificateOutlined, TrophyOutlined, FileSearchOutlined } from '@ant-design/icons';
import { api } from '../../api/client';
import type { ParadigmItem, EvidenceCard } from '../../types/api';
import EvidenceCardView from '../../components/research/EvidenceCard';

const { Title, Text, Paragraph } = Typography;

export default function Verified() {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<ParadigmItem[]>([]);
  const [evidenceDrawerOpen, setEvidenceDrawerOpen] = useState(false);
  const [evidenceLoading, setEvidenceLoading] = useState(false);
  const [currentEvidence, setCurrentEvidence] = useState<EvidenceCard | null>(null);

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

  useEffect(() => { load(); }, []);

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

  const columns: ColumnsType<ParadigmItem> = [
    { title: '范式名', dataIndex: 'name', key: 'name', width: 200, ellipsis: true },
    {
      title: '方向', dataIndex: 'side', key: 'side', width: 70,
      render: (s: string) => <Tag color={s === 'buy' ? 'green' : 'red'}>{s === 'buy' ? '买' : '卖'}</Tag>,
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
      title: '操作', key: 'action', width: 100,
      render: (_, r) => (
        <Button size="small" icon={<FileSearchOutlined />} onClick={() => loadEvidence(r.id)}>证据</Button>
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
          通过回归门验证、晋级生产的稳定范式，可用于前向观察
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
    </Space>
  );
}
