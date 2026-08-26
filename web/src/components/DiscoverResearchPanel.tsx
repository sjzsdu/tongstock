import { useEffect, useState } from 'react';
import { ExperimentOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Empty, Input, List, Space, Spin, Table, Tag, Typography } from 'antd';
import { api } from '../api/client';

type DiscoverRunResponse = Awaited<ReturnType<typeof api.discoverRun>>;

interface CandidateRow {
  rank: number;
  template_id: string;
  method_name: string;
  observations: number;
  win_rate: number;
  mean_forward_return: number;
  baseline_return: number;
  lift: number;
  confidence: string;
}

interface TraceRow {
  research_id: string;
  created_at: string;
  candidate_count: number;
  passable_count: number;
  stock_codes?: string[];
}

function toCandidateRows(candidates: DiscoverRunResponse['candidates']): CandidateRow[] {
  return candidates.map((c) => {
    const ev = c.validation_evidence?.[0];
    return {
      rank: c.rank,
      template_id: c.template_id,
      method_name: c.method?.name ?? c.template_id,
      observations: c.observations,
      win_rate: c.win_rate,
      mean_forward_return: c.mean_forward_return,
      baseline_return: c.baseline_return,
      lift: c.lift,
      confidence: ev?.passable ? 'passable' : ev?.confidence ?? ev?.status ?? 'pending',
    };
  });
}

const pct = (v: number) => `${(v * 100).toFixed(2)}%`;

export default function DiscoverResearchPanel({ poolId, poolName }: { poolId: string; poolName: string }) {
  const [running, setRunning] = useState(false);
  const [question, setQuestion] = useState('');
  const [result, setResult] = useState<DiscoverRunResponse | null>(null);
  const [error, setError] = useState('');
  const [traces, setTraces] = useState<TraceRow[]>([]);

  const runDiscover = async () => {
    if (!poolId) return;
    setRunning(true);
    setError('');
    try {
      const r = await api.discoverRun({ pool_id: poolId, question: question || undefined });
      setResult(r);
    } catch (e) {
      setError(e instanceof Error ? e.message : '研究运行失败');
    } finally {
      setRunning(false);
    }
  };

  useEffect(() => {
    if (!poolId) return;
    api.discoverTraces(10)
      .then((r) => setTraces(r.traces))
      .catch(() => setTraces([]));
  }, [poolId, result?.research_id]);

  const candidates = result ? toCandidateRows(result.candidates) : [];
  const passable = candidates.filter((c) => c.confidence === 'passable').length;

  return (
    <Card
      size="small"
      title={
        <Space>
          <ExperimentOutlined />
          <span>规律发现研究</span>
        </Space>
      }
      extra={<Typography.Text type="secondary">在真实冻结数据上挖掘候选规律，并在样本外验证</Typography.Text>}
    >
      <Space direction="vertical" size={12} style={{ display: 'flex', width: '100%' }}>
        <Space.Compact style={{ width: '100%' }}>
          <Input
            placeholder="可选：研究问题（默认：研究上涨前反复出现的共同特征）"
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            onPressEnter={() => void runDiscover()}
            disabled={running}
          />
          <Button
            type="primary"
            icon={<ExperimentOutlined />}
            loading={running}
            onClick={() => void runDiscover()}
          >
            对该池运行发现研究
          </Button>
        </Space.Compact>

        {error && <Alert type="error" showIcon message="研究失败" description={error} />}

        {running && (
          <Card size="small">
            <Spin tip={`正在为「${poolName}」冻结快照、挖掘候选并样本外验证…（多股票可能需要几分钟）`}>
              <div style={{ height: 64 }} />
            </Spin>
          </Card>
        )}

        {result && !running && (
          <Card size="small" title={`研究结果 · ${result.research_id.slice(0, 20)}…`}>
            <Space direction="vertical" size={12} style={{ display: 'flex', width: '100%' }}>
              <Space wrap>
                <Tag color="blue">候选 {result.candidate_count}</Tag>
                <Tag color={passable > 0 ? 'green' : 'orange'}>样本外通过 {passable}</Tag>
                <Tag>结论 {result.conclusion}</Tag>
              </Space>
              {candidates.length === 0 ? (
                <Empty description="没有候选规律（所有模板在发现阶段被拒绝）" />
              ) : (
                <Table<CandidateRow>
                  size="small"
                  rowKey="template_id"
                  dataSource={candidates}
                  pagination={false}
                  columns={[
                    { title: '排名', dataIndex: 'rank', width: 60 },
                    { title: '规律', dataIndex: 'method_name', ellipsis: true },
                    { title: '触发次数', dataIndex: 'observations', width: 90 },
                    { title: '胜率', dataIndex: 'win_rate', width: 80, render: pct },
                    { title: '条件收益', dataIndex: 'mean_forward_return', width: 90, render: pct },
                    { title: '超额收益', dataIndex: 'lift', width: 90, render: pct },
                    {
                      title: '验证结论',
                      dataIndex: 'confidence',
                      width: 110,
                      render: (v: string) =>
                        v === 'passable' ? <Tag color="green">通过</Tag> : <Tag color="red">{v === 'rejected' ? '拒绝' : v}</Tag>,
                    },
                  ]}
                />
              )}
              <Alert
                type={passable > 0 ? 'success' : 'info'}
                showIcon
                message={passable > 0 ? '存在通过样本外验证的方法，可考虑入库' : '全部候选均被拒绝是有效结果'}
                description="系统不会为没有证据的候选放行。可换更大股票池、调整研究问题或使用 AI 助手研究更复杂的方法。"
              />
            </Space>
          </Card>
        )}

        {traces.length > 0 && (
          <Card size="small" title="历史研究轨迹">
            <List<TraceRow>
              size="small"
              dataSource={traces}
              locale={{ emptyText: <Empty description="暂无历史轨迹" /> }}
              renderItem={(t) => (
                <List.Item>
                  <List.Item.Meta
                    title={
                      <Space wrap>
                        <Typography.Text code>{t.research_id.slice(0, 24)}…</Typography.Text>
                        <Tag>{t.stock_codes?.length ? `${t.stock_codes.length} 只股票` : '—'}</Tag>
                        <Tag color={t.passable_count > 0 ? 'green' : 'orange'}>通过 {t.passable_count}/{t.candidate_count}</Tag>
                      </Space>
                    }
                    description={new Date(t.created_at).toLocaleString('zh-CN')}
                  />
                </List.Item>
              )}
            />
          </Card>
        )}
      </Space>
    </Card>
  );
}
