import { PlusOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons';
import { Button, Card, List, Space, Typography } from 'antd';
import type { CustomStockPool } from '../types/api';

interface StockPoolListProps {
  pools: CustomStockPool[];
  currentPoolId: string;
  onSelect: (id: string) => void;
  onAdd: () => void;
  onEdit: (pool: CustomStockPool) => void;
  onDelete: (id: string) => void;
}

export function StockPoolList({ pools, currentPoolId, onSelect, onAdd, onEdit, onDelete }: StockPoolListProps) {
  return (
    <Card
      title={
        <Space>
          <Typography.Text strong>股票池列表</Typography.Text>
          <Button type="primary" size="small" icon={<PlusOutlined />} onClick={onAdd} />
        </Space>
      }
      bodyStyle={{ padding: '12px' }}
    >
      <List
        dataSource={pools}
        renderItem={(item) => (
          <List.Item
            onClick={() => onSelect(item.id)}
            style={{ 
              cursor: 'pointer', 
              background: currentPoolId === item.id ? 'rgba(22,119,255,0.1)' : undefined,
              marginBottom: '8px',
              borderRadius: '6px',
              padding: '8px',
            }}
            extra={
              <Space size={4}>
                <Button size="small" icon={<EditOutlined />} onClick={(e) => { e.stopPropagation(); onEdit(item); }} />
                <Button size="small" danger icon={<DeleteOutlined />} onClick={(e) => { e.stopPropagation(); onDelete(item.id); }} disabled={pools.length <= 1} />
              </Space>
            }
          >
            <List.Item.Meta
              title={<Typography.Text strong>{item.name}</Typography.Text>}
              description={item.description || `${item.filters.length} 个筛选条件`}
            />
          </List.Item>
        )}
        style={{ maxHeight: 600, overflow: 'auto' }}
      />
    </Card>
  );
}