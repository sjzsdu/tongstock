import { Card, Col, Empty, Flex, Progress, Row, Space, Spin, Statistic, Table, Tag, Typography } from 'antd';
import { ThunderboltOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import type { Signal, SignalAnalysis, SignalOutcome } from '../../types/api';
import { formatTdxDate } from '../../lib/datetime';
import SignalInterpretationCard from '../SignalInterpretationCard';

interface SignalTabContentProps {
  chartLoading: boolean;
  analysis: SignalAnalysis | null;
  sortedSignals: Signal[];
  sortedSignalOutcomes: SignalOutcome[];
  pct: number;
  up: boolean;
}

function formatChange(value: number | null | undefined): React.ReactNode {
  if (value === null || value === undefined) return '-';
  return <span className={value >= 0 ? 'price-up' : 'price-down'}>{value > 0 ? '+' : ''}{value.toFixed(2)}%</span>;
}

export function SignalTabContent({ chartLoading, analysis, sortedSignals, sortedSignalOutcomes, pct, up }: SignalTabContentProps) {
  const signalColumns: ColumnsType<Signal> = [
    {
      title: '日期',
      dataIndex: 'Date',
      defaultSortOrder: 'ascend',
      sorter: (a: Signal, b: Signal) => String(b.Date ?? '').localeCompare(String(a.Date ?? '')),
      render: (value: string | undefined) => formatTdxDate(value),
    },
    { title: '指标', dataIndex: 'Indicator' },
    { title: '信号', dataIndex: 'Type' },
    { title: '强度', dataIndex: 'Strength', align: 'right', render: (value: number | undefined) => typeof value === 'number' ? value.toFixed(3) : '-' },
    { title: '详情', dataIndex: 'Details', render: (value: string | undefined) => <Tag>{value || '触发'}</Tag> },
  ];

  if (chartLoading) {
    return <Card><Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin size="large" /></Flex></Card>;
  }

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {analysis && (
        <SignalInterpretationCard
          interpretations={analysis.interpretations ?? []}
          overallSummary={analysis.overall_summary ?? '当前无明显技术信号'}
          trend={analysis.trend ?? '趋势不明'}
        />
      )}

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={12}>
          <Card>
            <Statistic
              title="信号回测样本"
              value={analysis?.signals ?? 0}
              suffix={`/ ${analysis?.count ?? 0}`}
            />
          </Card>
        </Col>
        <Col xs={24} xl={12}>
          <Card>
            <Space direction="vertical" size={8} style={{ display: 'flex' }}>
              <Typography.Text type="secondary">上涨/下跌强度</Typography.Text>
              <Progress
                percent={Math.min(100, Math.max(0, 50 + pct * 5))}
                strokeColor={up ? '#ef4444' : '#22c55e'}
                showInfo={false}
              />
              <Typography.Text>{pct > 0 ? '+' : ''}{pct.toFixed(2)}%</Typography.Text>
            </Space>
          </Card>
        </Col>
      </Row>

      <Card title={<Space><ThunderboltOutlined />信号列表</Space>}>
        <Table<Signal>
          size="small"
          pagination={{ pageSize: 20, showSizeChanger: false, showTotal: (total: number) => `共 ${total} 条` }}
          rowKey={(row: Signal, index?: number) => `${row.Date}-${row.Type}-${row.Indicator}-${index ?? 0}`}
          dataSource={sortedSignals}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无信号" /> }}
          columns={signalColumns}
        />
      </Card>

      {!analysis ? (
        <Card><Flex justify="center" align="center" style={{ minHeight: 160 }}><Spin /></Flex></Card>
      ) : analysis.summary.length > 0 ? (
        <Card title="信号回测" extra={<Typography.Text type="secondary">基于历史 {analysis.count} 根K线中的 {analysis.signals} 个信号</Typography.Text>}>
          <Table
            pagination={false}
            rowKey={(row: any) => row.type}
            size="small"
            dataSource={analysis.summary}
            columns={[
              { title: '信号', dataIndex: 'type' },
              { title: '建议', dataIndex: 'action', render: (value: string) => <Tag color={value === '买入参考' ? 'red' : 'green'}>{value}</Tag> },
              { title: '触发次数', dataIndex: 'count', align: 'right' },
              { title: '次日上涨率', dataIndex: 'win1', align: 'right', render: (_: unknown, row: any) => row.valid1 > 0 ? `${row.win1.toFixed(0)}% (${row.valid1})` : '-' },
              { title: '5日上涨率', dataIndex: 'win5', align: 'right', render: (_: unknown, row: any) => row.valid5 > 0 ? `${row.win5.toFixed(0)}% (${row.valid5})` : '-' },
              { title: '10日上涨率', dataIndex: 'win10', align: 'right', render: (_: unknown, row: any) => row.valid10 > 0 ? `${row.win10.toFixed(0)}% (${row.valid10})` : '-' },
              { title: '20日上涨率', dataIndex: 'win20', align: 'right', render: (_: unknown, row: any) => row.valid20 > 0 ? `${row.win20.toFixed(0)}% (${row.valid20})` : '-' },
              { title: '次日均涨幅', dataIndex: 'avg1', align: 'right', render: (value: number, row: any) => row.valid1 > 0 ? `${value > 0 ? '+' : ''}${value.toFixed(2)}%` : '-' },
              { title: '5日均涨幅', dataIndex: 'avg5', align: 'right', render: (value: number, row: any) => row.valid5 > 0 ? `${value > 0 ? '+' : ''}${value.toFixed(2)}%` : '-' },
            ]}
          />
        </Card>
      ) : (
        <Card><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无信号回测数据" /></Card>
      )}

      {sortedSignalOutcomes.length > 0 && (
        <Card title="信号明细">
          <Table
            size="small"
            pagination={{ pageSize: 20, showSizeChanger: false, showTotal: (total: number) => `共 ${total} 条` }}
            rowKey={(row: SignalOutcome) => `${row.date}-${row.type}-${row.indicator}`}
            dataSource={sortedSignalOutcomes}
            columns={[
              { title: '日期', dataIndex: 'date', render: (value: string) => formatTdxDate(value) },
              { title: '指标', dataIndex: 'indicator' },
              { title: '信号', dataIndex: 'type' },
              { title: '建议', dataIndex: 'action', render: (value: string) => <Tag color={value === '买入参考' ? 'red' : 'green'}>{value}</Tag> },
              { title: '触发价', dataIndex: 'price', align: 'right', render: (value: number) => value.toFixed(2) },
              { title: '次日涨跌', dataIndex: 'chg1', align: 'right', render: (value: number | null | undefined) => formatChange(value) },
              { title: '5日涨跌', dataIndex: 'chg5', align: 'right', render: (value: number | null | undefined) => formatChange(value) },
              { title: '10日涨跌', dataIndex: 'chg10', align: 'right', render: (value: number | null | undefined) => formatChange(value) },
              { title: '20日涨跌', dataIndex: 'chg20', align: 'right', render: (value: number | null | undefined) => formatChange(value) },
            ]}
          />
        </Card>
      )}
    </Space>
  );
}
