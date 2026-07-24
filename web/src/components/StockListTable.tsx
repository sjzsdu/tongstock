import { useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { ArrowRightOutlined } from '@ant-design/icons';
import { Button, Card, Empty, Pagination, Space, Table, Tag, Typography, Skeleton } from 'antd';
import type { ColumnsType } from 'antd/es/table';

export interface StockTableItem {
  code: string;
  name: string;
  exchange: string;
  price?: number;
  marketCap?: number;
  changePct?: number;
  turnoverRate?: number;
  volumeRatio?: number;
}

interface StockListTableProps {
  title?: string;
  dataSource: StockTableItem[];
  total?: number;
  loading?: boolean;
  pageSize?: number;
  columns?: ColumnsType<StockTableItem>;
}

export function StockListTable({ 
  title = '股票列表', 
  dataSource, 
  total, 
  loading = false, 
  pageSize = 20,
  columns,
}: StockListTableProps) {
  const navigate = useNavigate();
  
  const defaultColumns: ColumnsType<StockTableItem> = [
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
      title: '交易所',
      dataIndex: 'exchange',
      width: 80,
      render: (ex: string) => <Tag color={ex === 'sh' ? 'blue' : ex === 'sz' ? 'green' : 'orange'}>{ex.toUpperCase()}</Tag>,
    },
    {
      title: '股价(元)',
      dataIndex: 'price',
      width: 100,
      render: (price?: number) => price?.toFixed(2) || '-',
    },
    {
      title: '流通市值(亿)',
      dataIndex: 'marketCap',
      width: 120,
      render: (cap?: number) => cap?.toFixed(2) || '-',
    },
    {
      title: '涨跌幅(%)',
      dataIndex: 'changePct',
      width: 100,
      render: (pct?: number) => pct !== undefined ? (
        <Typography.Text type={pct >= 0 ? 'success' : 'danger'}>
          {pct >= 0 ? '+' : ''}{pct.toFixed(2)}
        </Typography.Text>
      ) : '-',
    },
    {
      title: '换手率(%)',
      dataIndex: 'turnoverRate',
      width: 100,
      render: (rate?: number) => rate?.toFixed(2) || '-',
    },
    {
      title: '量比',
      dataIndex: 'volumeRatio',
      width: 80,
      render: (ratio?: number) => ratio?.toFixed(2) || '-',
    },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button type="link" size="small" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${record.code}`)} />
      ),
    },
  ];

  const [page, setPage] = useState(1);
  const displayTotal = total ?? dataSource.length;
  
  const paginatedData = useMemo(() => {
    const start = (page - 1) * pageSize;
    return dataSource.slice(start, start + pageSize);
  }, [dataSource, page, pageSize]);

  return (
    <Card
      title={
        <Space>
          <Typography.Text strong>{title}</Typography.Text>
          <Typography.Text type="secondary">共 {displayTotal} 只</Typography.Text>
        </Space>
      }
      bodyStyle={{ padding: '16px' }}
    >
      {loading ? (
        <Skeleton active paragraph={{ rows: 4 }} title={false} />
      ) : dataSource.length === 0 ? (
        <Empty description="暂无符合条件的股票" />
      ) : (
        <>
          <Table
            columns={columns || defaultColumns}
            dataSource={paginatedData}
            rowKey="code"
            size="small"
            pagination={false}
            scroll={{ x: 'max-content' }}
          />
          <Pagination
            current={page}
            pageSize={pageSize}
            total={displayTotal}
            onChange={setPage}
            style={{ marginTop: 12, textAlign: 'center' }}
          />
        </>
      )}
    </Card>
  );
}