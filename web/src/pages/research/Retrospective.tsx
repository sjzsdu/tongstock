import { useEffect, useState } from 'react';
import { Card, Row, Col, Tag, Typography, Space, Table, Progress, Statistic, Button, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  FlagOutlined,
  RiseOutlined,
  FallOutlined,
  ThunderboltOutlined,
  CheckSquareOutlined,
  EditOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { api } from '../../api/client';
import type { ParadigmItem, ParadigmStatsResponse } from '../../types/api';

const { Title, Text, Paragraph } = Typography;

export default function Retrospective() {
  const [loading, setLoading] = useState(false);
  const [paradigms, setParadigms] = useState<ParadigmItem[]>([]);
  const [stats, setStats] = useState<ParadigmStatsResponse | null>(null);

  const load = async () => {
    setLoading(true);
    try {
      const [list, s] = await Promise.all([
        api.paradigmList(undefined, undefined, { limit: 100 }),
        api.paradigmStats(),
      ]);
      setParadigms(list.paradigms || []);
      setStats(s);
    } catch (err) {
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  // 计算表现
  const withReturn = paradigms.filter(p => p.actual_return !== undefined && p.actual_return !== null);
  const winners = withReturn.filter(p => (p.actual_return || 0) > 0);
  const losers = withReturn.filter(p => (p.actual_return || 0) < 0);
  const winRate = withReturn.length > 0 ? (winners.length / withReturn.length) * 100 : 0;

  // 按方向统计
  const buyParadigms = paradigms.filter(p => p.side === 'buy');
  const sellParadigms = paradigms.filter(p => p.side === 'sell');
  const highReliability = paradigms.filter(p => p.validation?.reliability_label === 'high').length;

  const columns: ColumnsType<ParadigmItem> = [
    { title: '范式名', dataIndex: 'name', key: 'name', width: 180, ellipsis: true },
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
      title: '审查状态', dataIndex: 'review_status', key: 'status', width: 90,
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
      title: '期望收益', key: 'exp_return', width: 100,
      render: (_, r) => <Text type="secondary">{r.expectation?.expected_return || '--'}</Text>,
    },
    {
      title: '实际收益', dataIndex: 'actual_return', key: 'return', width: 100,
      render: (r: number) => {
        if (r === undefined || r === null) return <Tag>观察中</Tag>;
        return <Text strong style={{ color: r >= 0 ? '#52c41a' : '#ff4d4f' }}>{(r * 100).toFixed(2)}%</Text>;
      },
    },
    {
      title: '差异', key: 'diff', width: 80,
      render: (_, r) => {
        if (r.actual_return === undefined || r.actual_return === null) return <Tag>--</Tag>;
        const diff = r.actual_return - (parseFloat(r.expectation?.expected_return || '0') / 100);
        return <Tag color={diff >= 0 ? 'green' : 'red'}>
          {diff >= 0 ? '+' : ''}{(diff * 100).toFixed(1)}%
        </Tag>;
      },
    },
  ];

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>复盘迭代</Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          定期复盘研究结果，分析漂移与衰减，驱动策略持续改进
        </Paragraph>
      </div>

      {/* 总体表现 */}
      <Card title="总体研究表现" extra={<Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>}>
        <Row gutter={[16, 16]}>
          <Col xs={12} md={4}>
            <Statistic
              title="总范式"
              value={paradigms.length}
              valueStyle={{ color: '#1677ff' }}
            />
          </Col>
          <Col xs={12} md={4}>
            <Statistic
              title="胜率"
              value={winRate}
              precision={1}
              suffix="%"
              valueStyle={{ color: winRate >= 50 ? '#52c41a' : '#ff4d4f' }}
              prefix={winRate >= 50 ? <RiseOutlined /> : <FallOutlined />}
            />
          </Col>
          <Col xs={12} md={4}>
            <Statistic
              title="高可靠性"
              value={highReliability}
              valueStyle={{ color: '#faad14' }}
              prefix={<ThunderboltOutlined />}
            />
          </Col>
          <Col xs={12} md={4}>
            <Statistic
              title="平均收益"
              value={stats?.average_return ?? 0}
              precision={2}
              suffix="%"
              valueStyle={{ color: (stats?.average_return ?? 0) >= 0 ? '#52c41a' : '#ff4d4f' }}
            />
          </Col>
          <Col xs={12} md={4}>
            <Statistic
              title="平均评分"
              value={stats?.average_rating ?? 0}
              precision={1}
              valueStyle={{ color: '#722ed1' }}
            />
          </Col>
          <Col xs={12} md={4}>
            <Statistic
              title="否决数"
              value={paradigms.filter(p => p.review_status === 'rejected').length}
              valueStyle={{ color: '#ff4d4f' }}
              prefix={<FlagOutlined />}
            />
          </Col>
        </Row>
      </Card>

      {/* 买卖方向分布 */}
      <Row gutter={[16, 16]}>
        <Col xs={12}>
          <Card title="方向分布">
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text>买入 ({buyParadigms.length})</Text>
                <Progress
                  percent={Math.round((buyParadigms.length / Math.max(paradigms.length, 1)) * 100)}
                  strokeColor="#52c41a"
                />
              </div>
              <div>
                <Text>卖出 ({sellParadigms.length})</Text>
                <Progress
                  percent={Math.round((sellParadigms.length / Math.max(paradigms.length, 1)) * 100)}
                  strokeColor="#ff4d4f"
                />
              </div>
            </Space>
          </Card>
        </Col>
        <Col xs={12}>
          <Card title="研究质量分布">
            <Space direction="vertical" style={{ width: '100%' }}>
              {(['high', 'medium', 'low'] as const).map(level => {
                const count = paradigms.filter(p => p.validation?.reliability_label === level).length;
                const labels: Record<string, string> = { high: '高可靠', medium: '中等', low: '低可靠' };
                const colors: Record<string, string> = { high: '#52c41a', medium: '#faad14', low: '#ff4d4f' };
                return (
                  <div key={level}>
                    <Text>{labels[level]} ({count})</Text>
                    <Progress
                      percent={Math.round((count / Math.max(paradigms.length, 1)) * 100)}
                      strokeColor={colors[level]}
                    />
                  </div>
                );
              })}
            </Space>
          </Card>
        </Col>
      </Row>

      {/* 详细列表 */}
      <Card
        title={
          <Space>
            <CheckSquareOutlined />
            <span>复盘清单</span>
            <Tag>{paradigms.length} 项</Tag>
          </Space>
        }
      >
        <Table
          rowKey="id"
          columns={columns}
          dataSource={paradigms}
          loading={loading}
          pagination={{ pageSize: 10, showSizeChanger: true }}
          size="middle"
        />
      </Card>
    </Space>
  );
}
