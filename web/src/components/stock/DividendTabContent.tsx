import { useEffect } from 'react';
import { Card, Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { XdXrItem } from '../../types/api';
import { formatTdxDate } from '../../lib/datetime';

// Keep react import for JSX runtime
useEffect(() => {}, []);

interface DividendTabContentProps {
  dividends: XdXrItem[];
}

export function DividendTabContent({ dividends }: DividendTabContentProps) {
  const columns: ColumnsType<XdXrItem> = [
    { title: '日期', dataIndex: 'Date', render: (value) => formatTdxDate(value) },
    { title: '类型', dataIndex: 'Category' },
    { title: '分红(元)', dataIndex: 'FenHong', align: 'right', render: (value) => value > 0 ? value.toFixed(4) : '-' },
    { title: '送转(股)', dataIndex: 'SongZhuanGu', align: 'right', render: (value) => value > 0 ? value.toFixed(2) : '-' },
    { title: '配股价', dataIndex: 'PeiGuJia', align: 'right', render: (value) => value > 0 ? value.toFixed(2) : '-' },
    { title: '流通盘', dataIndex: 'PanHouLiuTong', align: 'right', render: (value) => value > 0 ? `${(value / 10000).toFixed(1)}万` : '-' },
    { title: '总股本', dataIndex: 'HouZongGuBen', align: 'right', render: (value) => value > 0 ? `${(value / 10000).toFixed(1)}万` : '-' },
  ];

  return (
    <Card title="分红与除权除息">
      <Table rowKey={(row) => `${row.Date}-${row.Category}`} columns={columns} dataSource={dividends} size="small" pagination={{ pageSize: 12 }} />
    </Card>
  );
}
