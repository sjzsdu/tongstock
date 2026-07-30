import { useEffect, useState } from 'react';
import { Card, Col, Row, Statistic, Tag, Typography, Space, Progress } from 'antd';
import { api } from '../../api/client';
import type { ParadigmStatsResponse } from '../../types/api';
import { Link } from 'react-router-dom';

const { Title, Text, Paragraph } = Typography;

const workflowStages = [
  {
    key: 'hypothesis',
    title: '假设生成',
    subtitle: '研究起点',
    path: '/research/hypothesis',
    color: '#1677ff',
    desc: '基于内部数据和 AI 生成可证伪的研究假设',
  },
  {
    key: 'experiment',
    title: '实验筛选',
    subtitle: '快速验证',
    path: '/research/experiment',
    color: '#722ed1',
    desc: '通过筛选和回测快速验证假设有效性',
  },
  {
    key: 'candidates',
    title: '候选评估',
    subtitle: '深度审查',
    path: '/research/candidates',
    color: '#fa8c16',
    desc: '经 AI 反证审查的候选策略',
  },
  {
    key: 'verified',
    title: '已验证范式',
    subtitle: '晋级生产',
    path: '/research/verified',
    color: '#52c41a',
    desc: '通过回归门验证的稳定范式',
  },
  {
    key: 'observation',
    title: '前向观察',
    subtitle: 'Paper Trading',
    path: '/research/observation',
    color: '#13c2c2',
    desc: '不可回填的实时信号监测与收益追踪',
  },
  {
    key: 'retrospective',
    title: '复盘迭代',
    subtitle: '持续改进',
    path: '/research/retrospective',
    color: '#eb2f96',
    desc: '定期复盘、漂移检测与研究反馈',
  },
];

export default function ResearchHome() {
  const [stats, setStats] = useState<ParadigmStatsResponse | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const load = async () => {
      try {
        const s = await api.paradigmStats();
        setStats(s);
      } catch {
        // stats may not be available yet
      } finally {
        setLoading(false);
      }
    };
    load();
  }, []);

  const progress = stats ? Math.round((stats.reviewed / Math.max(stats.total, 1)) * 100) : 0;

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>研究工作台</Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          围绕研究阶段组织工作流：从假设生成到前向观察的完整闭环
        </Paragraph>
      </div>

      {/* 统计概览 */}
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card loading={loading}>
            <Statistic
              title="总范式数"
              value={stats?.total ?? 0}
              valueStyle={{ color: '#1677ff' }}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card loading={loading}>
            <Statistic
              title="已审查"
              value={stats?.reviewed ?? 0}
              valueStyle={{ color: '#722ed1' }}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card loading={loading}>
            <Statistic
              title="已验证"
              value={stats?.verified ?? 0}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card loading={loading}>
            <Statistic
              title="高可靠性"
              value={stats?.high_reliability ?? 0}
              valueStyle={{ color: '#fa8c16' }}
            />
          </Card>
        </Col>
      </Row>

      {/* 研究进度 */}
      <Card title="研究进度">
        <Progress
          percent={progress}
          strokeColor={{ '0%': '#1677ff', '100%': '#52c41a' }}
          format={() => `${progress}% 已审查`}
        />
        <Row gutter={[16, 8]} style={{ marginTop: 16 }}>
          <Col span={8}>
            <Text type="secondary">平均收益</Text>
            <div><Text strong>{stats?.average_return ? `${(stats.average_return * 100).toFixed(2)}%` : '--'}</Text></div>
          </Col>
          <Col span={8}>
            <Text type="secondary">平均评分</Text>
            <div><Text strong>{stats?.average_rating?.toFixed(1) ?? '--'}</Text></div>
          </Col>
          <Col span={8}>
            <Text type="secondary">胜率</Text>
            <div><Text strong>{stats?.win_rate ? `${(stats.win_rate * 100).toFixed(1)}%` : '--'}</Text></div>
          </Col>
        </Row>
      </Card>

      {/* 工作流阶段 */}
      <Card title="研究工作流" extra={<Tag color="blue">6 阶段</Tag>}>
        <Row gutter={[16, 16]}>
          {workflowStages.map((stage, idx) => (
            <Col xs={24} sm={12} lg={8} key={stage.key}>
              <Card
                hoverable
                styles={{ body: { padding: 20 } }}
                onClick={() => { window.location.href = stage.path; }}
                style={{ cursor: 'pointer', borderLeft: `4px solid ${stage.color}` }}
              >
                <Space direction="vertical" size={4} style={{ width: '100%' }}>
                  <Space>
                    <Tag color={stage.color} style={{ fontSize: 14, fontWeight: 600 }}>
                      {idx + 1}. {stage.title}
                    </Tag>
                  </Space>
                  <Text type="secondary">{stage.subtitle}</Text>
                  <Paragraph style={{ marginBottom: 0, fontSize: 13, color: '#595959' }}>
                    {stage.desc}
                  </Paragraph>
                  <Link to={stage.path} style={{ color: stage.color, fontSize: 13 }}>
                    进入 →
                  </Link>
                </Space>
              </Card>
            </Col>
          ))}
        </Row>
      </Card>

      {/* 快捷入口 */}
      <Card title="快捷入口">
        <Row gutter={[16, 16]}>
          <Col xs={12} md={6}>
            <Card size="small" hoverable onClick={() => { window.location.href = '/screen'; }}>
              <Space direction="vertical">
                <Text strong>信号筛选</Text>
                <Text type="secondary" style={{ fontSize: 12 }}>快速生成候选信号</Text>
              </Space>
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card size="small" hoverable onClick={() => { window.location.href = '/agent'; }}>
              <Space direction="vertical">
                <Text strong>AI 助手</Text>
                <Text type="secondary" style={{ fontSize: 12 }}>自然语言分析与生成</Text>
              </Space>
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card size="small" hoverable onClick={() => { window.location.href = '/paradigms'; }}>
              <Space direction="vertical">
                <Text strong>范式库</Text>
                <Text type="secondary" style={{ fontSize: 12 }}>浏览和管理已有范式</Text>
              </Space>
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card size="small" hoverable onClick={() => { window.location.href = '/watchlist'; }}>
              <Space direction="vertical">
                <Text strong>自选股</Text>
                <Text type="secondary" style={{ fontSize: 12 }}>关注池快速访问</Text>
              </Space>
            </Card>
          </Col>
        </Row>
      </Card>
    </Space>
  );
}
