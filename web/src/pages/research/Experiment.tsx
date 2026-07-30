import { useEffect, useState } from 'react';
import {
  Card,
  Row,
  Col,
  Tag,
  Typography,
  Space,
  Table,
  Progress,
  Button,
  Drawer,
  Alert,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  ExperimentOutlined,
  SearchOutlined,
  ReloadOutlined,
  FileSearchOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { api } from '../../api/client';
import type { ParadigmItem, EvidenceCard } from '../../types/api';
import EvidenceCardView from '../../components/research/EvidenceCard';
import LineageView from '../../components/research/LineageView';
import {
  ProductStatusBanner,
  ProductStatusBlock,
} from '../../components/ProductStatus';
import type { ProductStatusState } from '../../hooks/useProductStatus';

const { Title, Text, Paragraph } = Typography;

function buildInitialStatus(loading: boolean, count: number): ProductStatusState {
  if (loading) {
    return {
      kind: 'loading',
      meta: {
        label: '正在加载实验队列',
        actionHint: '',
        level: 'info',
      },
      canOperate: false,
    };
  }
  if (count === 0) {
    return {
      kind: 'unavailable',
      meta: {
        label: '暂无实验数据',
        actionHint: '前往假设页创建第一个可复现实验',
        level: 'info',
      },
      canOperate: false,
    };
  }
  return {
    kind: 'ready',
    meta: {
      label: '实验数据可用',
      actionHint: '',
      level: 'success',
    },
    canOperate: true,
  };
}

export default function Experiment() {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<ParadigmItem[]>([]);
  const [total, setTotal] = useState(0);

  const [evidenceDrawerOpen, setEvidenceDrawerOpen] = useState(false);
  const [lineageDrawerOpen, setLineageDrawerOpen] = useState(false);
  const [currentEvidence, setCurrentEvidence] = useState<EvidenceCard | null>(null);
  const [currentParadigmId, setCurrentParadigmId] = useState<string>('');
  const [evidenceLoading, setEvidenceLoading] = useState(false);

  const [errMsg, setErrMsg] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    setErrMsg(null);
    try {
      const resp = await api.paradigmList(undefined, undefined, { limit: 100 });
      setData(resp.paradigms || []);
      setTotal(resp.total || 0);
    } catch (err) {
      setErrMsg(err instanceof Error ? err.message : String(err));
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  // 按审查状态分组统计
  const groups = {
    pending: data.filter(d => !d.review_status || d.review_status === 'pending').length,
    reviewed: data.filter(d => d.review_status === 'reviewed').length,
    promoted: data.filter(d => d.review_status === 'promoted').length,
    rejected: data.filter(d => d.review_status === 'rejected').length,
  };

  const showEvidence = async (id: string) => {
    setCurrentParadigmId(id);
    setEvidenceDrawerOpen(true);
    setEvidenceLoading(true);
    setCurrentEvidence(null);
    try {
      const ev = await api.paradigmEvidence(id);
      setCurrentEvidence(ev);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载证据卡失败');
    } finally {
      setEvidenceLoading(false);
    }
  };

  const showLineage = (id: string) => {
    setCurrentParadigmId(id);
    setLineageDrawerOpen(true);
  };

  const columns: ColumnsType<ParadigmItem> = [
    {
      title: '范式名',
      dataIndex: 'name',
      key: 'name',
      width: 200,
      ellipsis: true,
    },
    {
      title: '方向',
      dataIndex: 'side',
      key: 'side',
      width: 70,
      render: (s: string) => (
        <Tag color={s === 'buy' ? 'green' : 'red'}>{s === 'buy' ? '买' : '卖'}</Tag>
      ),
    },
    {
      title: '可靠性',
      key: 'reliability',
      width: 90,
      render: (_, r) =>
        r.validation?.reliability_label ? (
          <Tag
            color={
              r.validation.reliability_label === 'high'
                ? 'green'
                : r.validation.reliability_label === 'medium'
                  ? 'orange'
                  : 'red'
            }
          >
            {r.validation.reliability_label}
          </Tag>
        ) : (
          <Tag>未知</Tag>
        ),
    },
    {
      title: '审查状态',
      dataIndex: 'review_status',
      key: 'review_status',
      width: 90,
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
      title: '结构完整性',
      key: 'score',
      width: 120,
      render: (_, r) => {
        const score = Math.round((r.validation?.auto_evaluable_ratio ?? 0) * 100);
        return <Progress percent={score} size="small" />;
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 220,
      render: (_, r) => (
        <Space>
          <Button
            size="small"
            type="link"
            icon={<FileSearchOutlined />}
            onClick={() => showEvidence(r.id)}
          >
            证据卡
          </Button>
          <Button
            size="small"
            type="link"
            onClick={() => showLineage(r.id)}
          >
            血缘
          </Button>
          <Button
            size="small"
            type="link"
            onClick={() => {
              navigate(`/research/candidates?id=${r.id}`);
            }}
          >
            审查
          </Button>
        </Space>
      ),
    },
  ];

  const pageStatus = buildInitialStatus(loading, total);

  return (
    <Space style={{ width: '100%' }} direction="vertical" size="large">
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>
          实验筛选
        </Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          对假设生成的候选进行快速筛选和验证，通过证据卡下钻查看样本交易与反证
        </Paragraph>
      </div>

      {/* 统一状态反馈 */}
      {errMsg && (
        <ProductStatusBanner
          state={{
            kind: 'failed',
            meta: {
              label: '实验列表加载失败',
              actionHint: '点击刷新以重新加载实验列表',
              level: 'error',
              reason: errMsg,
            },
            canOperate: true,
          }}
          onRetry={() => void load()}
        />
      )}

      {!errMsg && (
        <ProductStatusBanner state={pageStatus} onRetry={() => void load()} />
      )}

      {!errMsg && pageStatus.kind === 'unavailable' && total === 0 && (
        <ProductStatusBlock state={pageStatus} onRetry={() => void load()} />
      )}

      {/* 统计 */}
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <Text type="secondary">总数</Text>
              <div>
                <Text strong style={{ fontSize: 24 }}>
                  {total}
                </Text>
              </div>
            </div>
          </Card>
        </Col>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <Text type="secondary">待审查</Text>
              <div>
                <Text strong style={{ fontSize: 24, color: '#faad14' }}>
                  {groups.pending}
                </Text>
              </div>
            </div>
          </Card>
        </Col>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <Text type="secondary">已审查</Text>
              <div>
                <Text strong style={{ fontSize: 24, color: '#1677ff' }}>
                  {groups.reviewed}
                </Text>
              </div>
            </div>
          </Card>
        </Col>
        <Col xs={6} md={6}>
          <Card>
            <div style={{ textAlign: 'center' }}>
              <Text type="secondary">已晋级</Text>
              <div>
                <Text strong style={{ fontSize: 24, color: '#52c41a' }}>
                  {groups.promoted}
                </Text>
              </div>
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
            <Button
              icon={<SearchOutlined />}
              onClick={() => {
                navigate('/screen');
              }}
            >
              去筛选
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => void load()} loading={loading}>
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

      {/* 证据卡抽屉：下钻到样本交易 */}
      <Drawer
        title={
          <Space>
            <SafetyCertificateOutlined />
            <span>范式证据卡</span>
          </Space>
        }
        width={880}
        open={evidenceDrawerOpen}
        onClose={() => setEvidenceDrawerOpen(false)}
      >
        {evidenceLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}>加载中...</div>
        ) : currentEvidence ? (
          <EvidenceCardView card={currentEvidence} />
        ) : (
          <Alert
            type="warning"
            showIcon
            message="暂无证据数据"
            description="该范式尚未生成证据卡，可能是缺少样本数据或回测失败。"
          />
        )}
      </Drawer>

      {/* 血缘视图抽屉 */}
      <Drawer
        title={
          <Space>
            <ExperimentOutlined />
            <span>研究血缘与版本对比</span>
          </Space>
        }
        width={1000}
        open={lineageDrawerOpen}
        onClose={() => setLineageDrawerOpen(false)}
        destroyOnClose
      >
        {currentParadigmId ? (
          <LineageView paradigmId={currentParadigmId} />
        ) : null}
      </Drawer>
    </Space>
  );
}
