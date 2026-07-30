import { useEffect, useMemo, useState } from 'react';
import {
  Card,
  Row,
  Col,
  Tag,
  Typography,
  Space,
  Table,
  Select,
  Button,
  Empty,
  Spin,
  Alert,
  Descriptions,
  Divider,
  Timeline,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  ShareAltOutlined,
  SwapOutlined,
  HistoryOutlined,
  FileTextOutlined,
  ExperimentOutlined,
  CloseCircleFilled,
  LoadingOutlined,
  ThunderboltOutlined,
  SafetyOutlined,
  RollbackOutlined,
} from '@ant-design/icons';
import { api } from '../../api/client';
import type {
  ParadigmLineageGraph,
  ParadigmVersionRecord,
  ParadigmVersionDiff,
  LineageNode,
} from '../../types/api';

const { Text, Paragraph } = Typography;

interface Props {
  paradigmId: string;
  paradigmName?: string;
}

const NODE_TYPE_META: Record<string, { label: string; color: string; icon: React.ReactNode }> = {
  hypothesis: { label: '假设', color: 'blue', icon: <FileTextOutlined /> },
  paradigm: { label: '范式', color: 'cyan', icon: <ExperimentOutlined /> },
  evidence: { label: '证据', color: 'purple', icon: <SafetyOutlined /> },
  review: { label: '审查', color: 'gold', icon: <HistoryOutlined /> },
  promote: { label: '晋级', color: 'green', icon: <ThunderboltOutlined /> },
  reject: { label: '否决', color: 'red', icon: <CloseCircleFilled /> },
};

const STATUS_COLOR: Record<string, string> = {
  accepted: 'green',
  promoted: 'green',
  rejected: 'red',
  open: 'blue',
  in_progress: 'orange',
};

function formatTime(ts: string) {
  try {
    return new Date(ts).toLocaleString();
  } catch {
    return ts;
  }
}

export default function LineageView({ paradigmId }: Props) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lineage, setLineage] = useState<ParadigmLineageGraph | null>(null);
  const [diff, setDiff] = useState<ParadigmVersionDiff | null>(null);
  const [diffLoading, setDiffLoading] = useState(false);
  const [fromVersion, setFromVersion] = useState<number | null>(null);
  const [toVersion, setToVersion] = useState<number | null>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const g = await api.paradigmLineage(paradigmId);
      setLineage(g);
      if (g.versions.length >= 2) {
        setFromVersion(g.versions[g.versions.length - 2].version);
        setToVersion(g.versions[g.versions.length - 1].version);
      } else if (g.versions.length === 1) {
        setFromVersion(g.versions[0].version);
        setToVersion(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (paradigmId) {
      void load();
    }
  }, [paradigmId]);

  const runDiff = async () => {
    if (!fromVersion || !toVersion || fromVersion === toVersion) {
      setDiff(null);
      return;
    }
    setDiffLoading(true);
    try {
      const d = await api.paradigmDiff(paradigmId, fromVersion, toVersion);
      setDiff(d);
    } catch (err) {
      setDiff(null);
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setDiffLoading(false);
    }
  };

  useEffect(() => {
    if (fromVersion && toVersion && fromVersion !== toVersion) {
      void runDiff();
    } else {
      setDiff(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fromVersion, toVersion]);

  const nodesByType = useMemo(() => {
    const grouped: Record<string, LineageNode[]> = {};
    if (!lineage) return grouped;
    for (const n of lineage.nodes) {
      (grouped[n.type] ||= []).push(n);
    }
    return grouped;
  }, [lineage]);
  void nodesByType;

  const versionColumns: ColumnsType<ParadigmVersionRecord> = [
    {
      title: '版本',
      dataIndex: 'version',
      key: 'version',
      width: 80,
      render: (v: number) => <Tag color="blue">v{v}</Tag>,
    },
    {
      title: '类型',
      dataIndex: 'change_type',
      key: 'change_type',
      width: 100,
      render: (t: string) => {
        const colorMap: Record<string, string> = {
          create: 'blue',
          update: 'cyan',
          review: 'gold',
          promote: 'green',
          rollback: 'orange',
        };
        return <Tag color={colorMap[t] || 'default'}>{t}</Tag>;
      },
    },
    { title: '变更说明', dataIndex: 'change_reason', key: 'change_reason' },
    { title: '作者', dataIndex: 'author', key: 'author', width: 120 },
    {
      title: '哈希',
      dataIndex: 'content_hash',
      key: 'content_hash',
      width: 140,
      render: (h: string) => <Text code>{h}</Text>,
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (t: string) => formatTime(t),
    },
  ];

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 60 }}>
        <Spin />
        <div style={{ marginTop: 8 }}>正在加载研究血缘...</div>
      </div>
    );
  }

  if (error && !lineage) {
    return <Alert type="error" showIcon message="加载血缘失败" description={error} />;
  }

  if (!lineage) {
    return <Empty description="暂无研究血缘数据" />;
  }

  const timelineItems = lineage.nodes.map((n) => {
    const meta = NODE_TYPE_META[n.type] || { label: n.type, color: 'default', icon: <FileTextOutlined /> };
    return {
      color: STATUS_COLOR[n.status] || 'gray',
      children: (
        <Card
          size="small"
          style={{ marginBottom: 8 }}
          title={
            <Space>
              <Tag color={meta.color} icon={meta.icon}>
                {meta.label}
              </Tag>
              <Text strong>{n.title}</Text>
              {n.version != null && <Tag>v{n.version}</Tag>}
            </Space>
          }
        >
          {n.detail && <Paragraph type="secondary" style={{ marginBottom: 0 }}>{n.detail}</Paragraph>}
          <Text type="secondary" style={{ fontSize: 12 }}>
            {formatTime(n.timestamp)}
            {n.actor ? ` · ${n.actor}` : ''}
          </Text>
        </Card>
      ),
    };
  });

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      {/* 摘要 */}
      <Card
        title={
          <Space>
            <ShareAltOutlined />
            <span>研究血缘</span>
          </Space>
        }
        extra={
          <Button size="small" onClick={() => void load()} icon={<RollbackOutlined />}>
            刷新
          </Button>
        }
      >
        <Descriptions column={2} size="small">
          <Descriptions.Item label="范式">
            <Text strong>{lineage.paradigm_name}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="当前状态">
            <Tag color={STATUS_COLOR[lineage.current_state] || 'default'}>
              {lineage.current_state}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="版本数">{lineage.versions.length}</Descriptions.Item>
          <Descriptions.Item label="节点数">{lineage.nodes.length}</Descriptions.Item>
          <Descriptions.Item label="总结" span={2}>
            {lineage.summary}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      {/* 血缘图 (Timeline) */}
      <Card
        title={
          <Space>
            <HistoryOutlined />
            <span>血缘视图 (从假设到晋级的完整链路)</span>
          </Space>
        }
      >
        {timelineItems.length === 0 ? (
          <Empty description="暂无双系节点" />
        ) : (
          <Timeline mode="left" items={timelineItems} />
        )}
      </Card>

      {/* 版本历史 */}
      <Card
        title={
          <Space>
            <HistoryOutlined />
            <span>版本历史 (旧版本不可覆盖)</span>
          </Space>
        }
      >
        {lineage.versions.length === 0 ? (
          <Empty description="暂无版本记录" />
        ) : (
          <Table
            rowKey="id"
            columns={versionColumns}
            dataSource={lineage.versions}
            pagination={false}
            size="small"
          />
        )}
      </Card>

      {/* 版本对比 */}
      {lineage.versions.length >= 2 && (
        <Card
          title={
            <Space>
              <SwapOutlined />
              <span>版本对比</span>
            </Space>
          }
        >
          <Row gutter={[16, 16]}>
            <Col xs={8}>
              <Text type="secondary">源版本</Text>
              <Select
                style={{ width: '100%', marginTop: 4 }}
                value={fromVersion ?? undefined}
                onChange={(v) => setFromVersion(v)}
                options={lineage.versions.map((v) => ({
                  value: v.version,
                  label: `v${v.version} — ${v.change_type}`,
                }))}
              />
            </Col>
            <Col xs={8}>
              <Text type="secondary">目标版本</Text>
              <Select
                style={{ width: '100%', marginTop: 4 }}
                value={toVersion ?? undefined}
                onChange={(v) => setToVersion(v)}
                options={lineage.versions.map((v) => ({
                  value: v.version,
                  label: `v${v.version} — ${v.change_type}`,
                }))}
              />
            </Col>
            <Col xs={8}>
              <Text type="secondary">操作</Text>
              <div style={{ marginTop: 4 }}>
                <Button
                  icon={<SwapOutlined />}
                  onClick={() => {
                    if (fromVersion && toVersion) {
                      setFromVersion(toVersion);
                      setToVersion(fromVersion);
                    }
                  }}
                >
                  交换
                </Button>
              </div>
            </Col>
          </Row>

          <Divider />

          {diffLoading && (
            <div style={{ textAlign: 'center', padding: 20 }}>
              <Spin indicator={<LoadingOutlined />} />
            </div>
          )}

          {!diffLoading && diff && (
            <>
              <Alert
                type={
                  diff.changed_rule || diff.changed_data || diff.changed_meta
                    ? 'info'
                    : 'success'
                }
                showIcon
                message={diff.summary}
                style={{ marginBottom: 16 }}
              />

              <Row gutter={[16, 16]}>
                <Col xs={8}>
                  <Card size="small" title={<Space><ExperimentOutlined />规则变更</Space>}>
                    <div>
                      <Text type="secondary">状态：</Text>
                      <Tag color={diff.changed_rule ? 'orange' : 'green'}>
                        {diff.changed_rule ? '有变更' : '无变更'}
                      </Tag>
                    </div>
                    {diff.rule_diff.added.length > 0 && (
                      <div style={{ marginTop: 8 }}>
                        <Text type="secondary">新增:</Text>
                        <div>
                          {diff.rule_diff.added.map((x, i) => (
                            <Tag key={i} color="green">{x}</Tag>
                          ))}
                        </div>
                      </div>
                    )}
                    {diff.rule_diff.removed.length > 0 && (
                      <div style={{ marginTop: 8 }}>
                        <Text type="secondary">删除:</Text>
                        <div>
                          {diff.rule_diff.removed.map((x, i) => (
                            <Tag key={i} color="red">{x}</Tag>
                          ))}
                        </div>
                      </div>
                    )}
                    {diff.rule_diff.added.length === 0 &&
                      diff.rule_diff.removed.length === 0 && (
                        <Text type="secondary" style={{ fontSize: 12 }}>无规则差异</Text>
                      )}
                  </Card>
                </Col>
                <Col xs={8}>
                  <Card size="small" title={<Space><FileTextOutlined />数据/参数变更</Space>}>
                    <div>
                      <Text type="secondary">状态：</Text>
                      <Tag color={diff.changed_data ? 'orange' : 'green'}>
                        {diff.changed_data ? '已切换' : '未变'}
                      </Tag>
                    </div>
                    <div style={{ marginTop: 8 }}>
                      <Descriptions column={1} size="small">
                        <Descriptions.Item label="数据源">
                          {diff.data_diff.old_data_source || '—'}
                          {' → '}
                          {diff.data_diff.new_data_source || '—'}
                        </Descriptions.Item>
                        <Descriptions.Item label="数据版本">
                          {diff.data_diff.old_version || '—'}
                          {' → '}
                          {diff.data_diff.new_version || '—'}
                        </Descriptions.Item>
                      </Descriptions>
                    </div>
                  </Card>
                </Col>
                <Col xs={8}>
                  <Card size="small" title={<Space><SafetyOutlined />元数据变更</Space>}>
                    <div>
                      <Text type="secondary">状态：</Text>
                      <Tag color={diff.changed_meta ? 'orange' : 'green'}>
                        {diff.changed_meta ? '已变更' : '未变'}
                      </Tag>
                    </div>
                    <div style={{ marginTop: 8 }}>
                      <Descriptions column={1} size="small">
                        <Descriptions.Item label="审查状态">
                          {diff.meta_diff.old_review_status || '—'}
                          {' → '}
                          {diff.meta_diff.new_review_status || '—'}
                        </Descriptions.Item>
                        <Descriptions.Item label="可靠性">
                          {diff.meta_diff.old_reliability || '—'}
                          {' → '}
                          {diff.meta_diff.new_reliability || '—'}
                        </Descriptions.Item>
                      </Descriptions>
                    </div>
                  </Card>
                </Col>
              </Row>
            </>
          )}
        </Card>
      )}

      {/* 节点类型图例 */}
      <Card size="small" title={<Text type="secondary">图例</Text>}>
        <Space wrap>
          {Object.entries(NODE_TYPE_META).map(([k, meta]) => (
            <Tag key={k} color={meta.color} icon={meta.icon}>
              {meta.label}
            </Tag>
          ))}
        </Space>
      </Card>
    </Space>
  );
}
