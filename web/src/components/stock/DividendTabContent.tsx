import 'react';
import { Card, Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { XdXrItem } from '../../types/api';
import { formatTdxDate } from '../../lib/datetime';

interface DividendTabContentProps {
  dividends: XdXrItem[];
}

// TDX xdxr 类别码 → 业务文案（与 pkg/tdx/protocol/xdxr.go 的 XdXrCategory 对齐）
const CATEGORY_LABELS: Record<string, string> = {
  '1': '除权除息',
  '2': '送配股上市',
  '3': '非流通股上市',
  '4': '未知股本变动',
  '5': '股本变化',
  '6': '增发新股',
  '7': '股份回购',
  '8': '增发新股上市',
  '9': '转配股上市',
  '10': '可转债上市',
  '11': '扩缩股',
  '12': '非流通股缩股',
  '13': '送认购权证',
  '14': '送认沽权证',
};

// 字段值单位已是“万股”：≥1万股即以“亿”呈现，否则“万股”
function formatShareWan(value: number): string {
  if (value >= 10000) return `${(value / 10000).toFixed(2)}亿`;
  return `${value.toFixed(0)}万`;
}

export function DividendTabContent({ dividends }: DividendTabContentProps) {
  const columns: ColumnsType<XdXrItem> = [
    { title: '日期', dataIndex: 'Date', render: (value) => formatTdxDate(value) },
    { title: '类型', dataIndex: 'Category', render: (value: string) => CATEGORY_LABELS[value] || `类别${value}` },
    { title: '分红(元)', dataIndex: 'FenHong', align: 'right', render: (value) => value > 0 ? value.toFixed(4) : '-' },
    { title: '送转(股)', dataIndex: 'SongZhuanGu', align: 'right', render: (value) => value > 0 ? value.toFixed(2) : '-' },
    { title: '配股价', dataIndex: 'PeiGuJia', align: 'right', render: (value) => value > 0 ? value.toFixed(2) : '-' },
    { title: '流通盘(股)', dataIndex: 'PanHouLiuTong', align: 'right', render: (value) => value > 0 ? formatShareWan(value) : '-' },
    { title: '总股本(股)', dataIndex: 'HouZongGuBen', align: 'right', render: (value) => value > 0 ? formatShareWan(value) : '-' },
  ];

  return (
    <Card title="分红与除权除息">
      <Table rowKey={(row) => `${row.Date}-${row.Category}`} columns={columns} dataSource={dividends} size="small" pagination={{ pageSize: 12 }} />
    </Card>
  );
}
