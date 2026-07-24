import { PlusOutlined, EditOutlined } from '@ant-design/icons';
import { Button, Card, Empty, Space, Tag, Typography } from 'antd';
import type { CustomStockPool, StockPoolFilter, FilterField } from '../types/api';

const filterFieldOptions: { field: FilterField; label: string; type: 'range' | 'select' | 'boolean'; unit?: string }[] = [
  { field: 'marketCap', label: '流通市值', type: 'range', unit: '亿' },
  { field: 'price', label: '股价', type: 'range', unit: '元' },
  { field: 'turnoverRate', label: '换手率', type: 'range', unit: '%' },
  { field: 'changePct', label: '涨跌幅', type: 'range', unit: '%' },
  { field: 'volumeRatio', label: '量比', type: 'range' },
  { field: 'exchange', label: '交易所', type: 'select' },
  { field: 'board', label: '板块类型', type: 'select' },
  { field: 'excludeST', label: '排除ST', type: 'boolean' },
];

interface StockPoolFilterPanelProps {
  pool: CustomStockPool;
  onEdit: () => void;
}

function renderFilterLabel(filter: StockPoolFilter): React.ReactNode {
  const fieldOption = filterFieldOptions.find(f => f.field === filter.field);
  if (!fieldOption) return filter.field;
  
  if (fieldOption.type === 'boolean') {
    return <>{fieldOption.label}</>;
  }
  if (filter.operator === 'between') {
    return `${fieldOption.label} ${filter.value[0]}~${filter.value[1]}${fieldOption.unit || ''}`;
  }
  return `${fieldOption.label} ${filter.value.join(', ')}`;
}

export function StockPoolFilterPanel({ pool, onEdit }: StockPoolFilterPanelProps) {
  return (
    <Card
      title={
        <Space>
          <Typography.Text strong>{pool.name}</Typography.Text>
          <Button size="small" icon={<EditOutlined />} onClick={onEdit}>
            编辑条件
          </Button>
        </Space>
      }
      bodyStyle={{ padding: '16px' }}
    >
      {pool.filters.length === 0 ? (
        <Space direction="vertical" size={12} style={{ display: 'flex', width: '100%' }}>
          <Empty description="暂无筛选条件" />
          <Button type="primary" size="small" icon={<PlusOutlined />} onClick={onEdit}>
            添加筛选条件
          </Button>
        </Space>
      ) : (
        <Space direction="vertical" size={8} style={{ display: 'flex', width: '100%' }}>
          {pool.filters.map((filter, index) => (
            <div key={index}>
              <Tag color="blue">{filterFieldOptions.find(f => f.field === filter.field)?.label}</Tag>
              <Typography.Text style={{ marginLeft: 8 }}>{renderFilterLabel(filter)}</Typography.Text>
            </div>
          ))}
          <Button type="primary" size="small" icon={<PlusOutlined />} onClick={onEdit}>
            添加条件
          </Button>
        </Space>
      )}
    </Card>
  );
}