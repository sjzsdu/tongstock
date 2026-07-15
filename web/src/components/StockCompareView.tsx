import {
  ArrowRightOutlined,
  BarChartOutlined,
  FundOutlined,
  RiseOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Col,
  Empty,
  Progress,
  Row,
  Skeleton,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useNavigate } from 'react-router-dom';
import type { BlockComparison, BlockComparisonStock } from '../types/api';

function getValueColor(value: number) {
  if (value > 0) return '#ef4444';
  if (value < 0) return '#22c55e';
  return '#cbd5e1';
}

function safeNumber(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function formatSignedPercent(value: number) {
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`;
}

interface StockCompareViewProps {
  code: string;
  stockName: string;
  stockChange: number;
  comparisons: BlockComparison[];
  loading?: boolean;
}

export default function StockCompareView({
  code,
  stockName,
  stockChange,
  comparisons,
  loading,
}: StockCompareViewProps) {
  const navigate = useNavigate();

  const safeComparisons = Array.isArray(comparisons) ? comparisons : [];

  if (loading) {
    return <Skeleton active paragraph={{ rows: 8 }} title={false} />;
  }

  if (safeComparisons.length === 0) {
    return (
      <Empty
        description="该股票暂无板块归属数据"
        image={Empty.PRESENTED_IMAGE_SIMPLE}
      />
    );
  }

  const stockColumns: ColumnsType<BlockComparisonStock> = [
    {
      title: '代码',
      dataIndex: 'code',
      width: 100,
      render: (code: string) => (
        <Button type="link" size="small" onClick={() => navigate(`/stock/${code}`)}>
          {code}
        </Button>
      ),
    },
    {
      title: '名称',
      dataIndex: 'name',
      width: 120,
    },
    {
      title: '现价',
      dataIndex: 'price',
      width: 80,
      align: 'right',
      render: (price: number) => safeNumber(price).toFixed(2),
    },
    {
      title: '涨跌幅',
      dataIndex: 'change',
      width: 80,
      align: 'right',
      render: (change: number) => (
        <Typography.Text style={{ color: getValueColor(change) }}>
          {formatSignedPercent(change)}
        </Typography.Text>
      ),
    },
  ];

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {/* 个股概览 */}
      <Card size="small" style={{ background: 'rgba(22,119,255,0.08)' }}>
        <Row gutter={[16, 16]}>
          <Col xs={24} sm={8}>
            <Statistic
              title="股票"
              value={stockName}
              suffix={<Typography.Text type="secondary">{code}</Typography.Text>}
            />
          </Col>
          <Col xs={24} sm={8}>
            <Statistic
              title="涨跌幅"
              value={stockChange}
              precision={2}
              suffix="%"
              valueStyle={{ color: getValueColor(stockChange) }}
              prefix={<RiseOutlined />}
            />
          </Col>
          <Col xs={24} sm={8}>
            <Statistic
              title="所属板块"
              value={safeComparisons.length}
              suffix="个"
            />
          </Col>
        </Row>
      </Card>

      {/* 板块对比列表 */}
      {safeComparisons.map((comparison, index) => {
        const topStocks = Array.isArray(comparison.top_stocks) ? comparison.top_stocks : [];
        const bottomStocks = Array.isArray(comparison.bottom_stocks) ? comparison.bottom_stocks : [];
        const validStocks = safeNumber(comparison.valid_stocks);
        const upCount = safeNumber(comparison.up_count);
        const downCount = safeNumber(comparison.down_count);
        const avgChange = safeNumber(comparison.avg_change);
        const blockFile = comparison.block_file || '';

        return (
          <Card
            key={`${comparison.block_name || 'block'}-${index}`}
          title={
            <Space>
              <FundOutlined style={{ color: '#1677ff' }} />
              <Typography.Text strong>{comparison.block_name || '未知板块'}</Typography.Text>
              <Tag color={comparison.block_type === 1 ? 'blue' : 'green'}>
                {blockFile.includes('fg') ? '行业' : blockFile.includes('gn') ? '概念' : '指数'}
              </Tag>
              {comparison.capped && (
                <Tag color="warning">部分数据</Tag>
              )}
            </Space>
          }
          extra={
            <Space>
              <Typography.Text type="secondary">
                板块内排名
              </Typography.Text>
              <Tag color={validStocks > 0 && comparison.stock_rank <= validStocks / 3 ? 'red' : validStocks > 0 && comparison.stock_rank >= validStocks * 2 / 3 ? 'green' : 'default'}>
                第 {comparison.stock_rank || '-'} / {validStocks || '-'} 名
              </Tag>
              <Button type="link" icon={<ArrowRightOutlined />} onClick={() => navigate(`/blocks`)}>
                查看板块
              </Button>
            </Space>
          }
        >
          <Row gutter={[16, 16]}>
            <Col xs={24} md={12}>
              <Space direction="vertical" size={12} style={{ display: 'flex' }}>
                <Typography.Text type="secondary">板块涨跌分布</Typography.Text>
                <Row gutter={[8, 8]}>
                  <Col span={8}>
                    <Statistic
                      title="上涨"
                      value={upCount}
                      valueStyle={{ color: '#ef4444', fontSize: 16 }}
                    />
                  </Col>
                  <Col span={8}>
                    <Statistic
                      title="下跌"
                      value={downCount}
                      valueStyle={{ color: '#22c55e', fontSize: 16 }}
                    />
                  </Col>
                  <Col span={8}>
                    <Statistic
                      title="平盘"
                      value={Math.max(0, validStocks - upCount - downCount)}
                      valueStyle={{ fontSize: 16 }}
                    />
                  </Col>
                </Row>
                <Progress
                  percent={validStocks > 0 ? (upCount / validStocks) * 100 : 0}
                  strokeColor="#ef4444"
                  trailColor="#22c55e"
                  showInfo={false}
                />
              </Space>
            </Col>
            <Col xs={24} md={12}>
              <Space direction="vertical" size={8} style={{ display: 'flex' }}>
                <Typography.Text type="secondary">板块平均涨跌幅</Typography.Text>
                <Statistic
                  value={avgChange}
                  precision={2}
                  suffix="%"
                  valueStyle={{ color: getValueColor(avgChange) }}
                />
                <Space>
                  <Typography.Text type="secondary">个股相对板块：</Typography.Text>
                  <Typography.Text style={{ color: getValueColor(stockChange - avgChange) }}>
                    {formatSignedPercent(stockChange - avgChange)}
                  </Typography.Text>
                </Space>
              </Space>
            </Col>
          </Row>

          {/* 领涨股 */}
          {topStocks.length > 0 && (
            <div style={{ marginTop: 16 }}>
              <Typography.Text strong style={{ marginBottom: 8, display: 'block' }}>
                <BarChartOutlined style={{ marginRight: 4 }} />
                领涨股
              </Typography.Text>
              <Table
                columns={stockColumns}
                dataSource={topStocks}
                rowKey="code"
                size="small"
                pagination={false}
              />
            </div>
          )}

          {/* 领跌股 */}
          {bottomStocks.length > 0 && (
            <div style={{ marginTop: 16 }}>
              <Typography.Text strong style={{ marginBottom: 8, display: 'block' }}>
                <BarChartOutlined style={{ marginRight: 4 }} />
                领跌股
              </Typography.Text>
              <Table
                columns={stockColumns}
                dataSource={bottomStocks}
                rowKey="code"
                size="small"
                pagination={false}
              />
            </div>
          )}
        </Card>
        );
      })}
    </Space>
  );
}
