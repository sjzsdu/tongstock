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
  Tabs,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useNavigate } from 'react-router-dom';
import type { BlockComparison, BlockComparisonStock } from '../types/api';
import { priceColor } from '../lib/palette';

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
      render: (name: string) => name || '-',
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
        <Typography.Text style={{ color: priceColor(change) }}>
          {formatSignedPercent(change)}
        </Typography.Text>
      ),
    },
  ];

  // 板块汇总表：横向扫读所有板块，不必逐个 tab 点开
  const summaryColumns: ColumnsType<BlockComparison & { __key: string }> = [
    {
      title: '板块',
      dataIndex: 'block_name',
      render: (name: string) => <Typography.Text strong>{name || '未知板块'}</Typography.Text>,
    },
    {
      title: '类型',
      dataIndex: 'block_file',
      width: 80,
      render: (file: string) => (
        <Tag color="blue">{file?.includes('fg') ? '行业' : file?.includes('gn') ? '概念' : '指数'}</Tag>
      ),
    },
    {
      title: '平均涨跌幅',
      dataIndex: 'avg_change',
      align: 'right',
      width: 110,
      render: (v: number) => (
        <Typography.Text style={{ color: priceColor(safeNumber(v)) }}>
          {formatSignedPercent(safeNumber(v))}
        </Typography.Text>
      ),
    },
    {
      title: '个股相对板块',
      key: 'relative',
      align: 'right',
      width: 120,
      render: (_: unknown, row: BlockComparison) => {
        const rel = stockChange - safeNumber(row.avg_change);
        return (
          <Typography.Text style={{ color: priceColor(rel) }}>
            {formatSignedPercent(rel)}
          </Typography.Text>
        );
      },
    },
    {
      title: '板块内排名',
      key: 'rank',
      align: 'right',
      width: 120,
      render: (_: unknown, row: BlockComparison) => {
        const valid = safeNumber(row.valid_stocks);
        return `第 ${row.stock_rank || '-'} / ${valid || '-'} 名`;
      },
    },
  ];

  const summaryDataSource = safeComparisons.map((c, i) => ({ ...c, __key: `${c.block_name || 'block'}-${i}` }));

  // 板块明细 tab：标题 = 板块名 + 平均涨跌幅标签，替代原先 17 张卡片纵向堆叠
  const tabItems = safeComparisons.map((comparison, index) => {
    const avgChange = safeNumber(comparison.avg_change);
    return {
      key: `${comparison.block_name || 'block'}-${index}`,
      label: (
        <Space size={6}>
          <span>{comparison.block_name || '未知板块'}</span>
          <Tag color={avgChange >= 0 ? 'red' : 'green'} style={{ marginInlineEnd: 0 }}>
            {formatSignedPercent(avgChange)}
          </Tag>
        </Space>
      ),
      children: (
        <BlockCompareCard
          comparison={comparison}
          stockChange={stockChange}
          stockColumns={stockColumns}
        />
      ),
    };
  });

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {/* 个股概览 */}
      <Card size="small" style={{ background: 'rgba(22,119,255,0.08)' }}>
        <Row gutter={[16, 16]}>
          <Col xs={24} sm={8}>
            <Statistic
              title="股票"
              value={stockName || code}
              suffix={<Typography.Text type="secondary">{code}</Typography.Text>}
            />
          </Col>
          <Col xs={24} sm={8}>
            <Statistic
              title="涨跌幅"
              value={stockChange}
              precision={2}
              suffix="%"
              valueStyle={{ color: priceColor(stockChange) }}
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

      {/* 板块汇总表（标注 maxBlockStocks 口径） */}
      <Card
        title={<Space><FundOutlined style={{ color: '#1677ff' }} />板块汇总</Space>}
        extra={
          <Typography.Text type="secondary">
            每板块最多取前 30 只个股参与统计（maxBlockStocks=30），大板块数据可能不完整
          </Typography.Text>
        }
      >
        <Table
          size="small"
          rowKey="__key"
          dataSource={summaryDataSource}
          columns={summaryColumns}
          pagination={false}
        />
      </Card>

      {/* 板块明细：tab 切换 */}
      <Card title={<Space><BarChartOutlined />板块明细</Space>} styles={{ body: { paddingTop: 4 } }}>
        <Tabs defaultActiveKey={tabItems[0]?.key} items={tabItems} />
      </Card>
    </Space>
  );
}

// 单个板块的明细卡片：涨跌分布 + 领涨/领跌两张表（原卡片布局不变，移入 tab 内容）
function BlockCompareCard({
  comparison,
  stockChange,
  stockColumns,
}: {
  comparison: BlockComparison;
  stockChange: number;
  stockColumns: ColumnsType<BlockComparisonStock>;
}) {
  const navigate = useNavigate();
  const topStocks = Array.isArray(comparison.top_stocks) ? comparison.top_stocks : [];
  const bottomStocks = Array.isArray(comparison.bottom_stocks) ? comparison.bottom_stocks : [];
  const validStocks = safeNumber(comparison.valid_stocks);
  const upCount = safeNumber(comparison.up_count);
  const downCount = safeNumber(comparison.down_count);
  const avgChange = safeNumber(comparison.avg_change);
  const blockFile = comparison.block_file || '';

  return (
    <Card
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
                  valueStyle={{ color: priceColor(1), fontSize: 16 }}
                />
              </Col>
              <Col span={8}>
                <Statistic
                  title="下跌"
                  value={downCount}
                  valueStyle={{ color: priceColor(-1), fontSize: 16 }}
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
              strokeColor={priceColor(1)}
              trailColor={priceColor(-1)}
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
              valueStyle={{ color: priceColor(avgChange) }}
            />
            <Space>
              <Typography.Text type="secondary">个股相对板块：</Typography.Text>
              <Typography.Text style={{ color: priceColor(stockChange - avgChange) }}>
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
}
