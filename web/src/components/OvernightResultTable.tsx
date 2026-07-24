import { useRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Card, Space, Button, Tag, Typography } from 'antd';
import { ShoppingCartOutlined, MinusCircleOutlined } from '@ant-design/icons';
import type { OvernightCandidate, TradeInfo } from '../api/client';
import type { OvernightSortKey } from '../types/strategy';
import { SortHeader } from './SortHeader';
import { formatPercent, getPriceColor } from '../utils/screen';

const { Text } = Typography;
const ROW_HEIGHT = 48;

interface OvernightResultTableProps {
  results: OvernightCandidate[];
  sortKey: OvernightSortKey;
  sortAsc: boolean;
  onSortChange: (key: OvernightSortKey) => void;
  navigate: (path: string) => void;
  trades: Record<string, TradeInfo>;
  tradingLoading: boolean;
  handleBuy: (result: OvernightCandidate) => void;
  handleSell: (result: OvernightCandidate) => void;
}

export function OvernightResultTable({
  results,
  sortKey,
  sortAsc,
  onSortChange,
  navigate,
  trades,
  tradingLoading,
  handleBuy,
  handleSell,
}: OvernightResultTableProps) {
  const tableContainerRef = useRef<HTMLDivElement>(null);

  const rowVirtualizer = useVirtualizer({
    count: results.length,
    getScrollElement: () => tableContainerRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 18,
  });

  return (
    <Card bodyStyle={{ padding: 0 }}>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '80px 1fr 96px 96px 80px 80px 1fr 140px',
          gap: 0,
          padding: '0 16px',
          borderBottom: '1px solid var(--ant-color-border-secondary)',
          background: 'var(--ant-color-fill-quaternary)',
        }}
      >
        <div style={{ padding: '10px 0' }}><SortHeader sortKey="code" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>代码</SortHeader></div>
        <div style={{ padding: '10px 12px' }}><SortHeader sortKey="name" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>名称</SortHeader></div>
        <div style={{ padding: '10px 0' }}><SortHeader sortKey="price" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">价格</SortHeader></div>
        <div style={{ padding: '10px 0' }}><SortHeader sortKey="change_pct" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">涨幅</SortHeader></div>
        <div style={{ padding: '10px 0', textAlign: 'right', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>量比</div>
        <div style={{ padding: '10px 0', textAlign: 'right', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>换手</div>
        <div style={{ padding: '10px 12px', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>选股标准</div>
        <div style={{ padding: '10px 0', textAlign: 'center', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>虚拟操作</div>
      </div>

      <div ref={tableContainerRef} style={{ maxHeight: 'calc(100vh - 480px)', minHeight: 320, overflow: 'auto' }}>
        <div style={{ height: rowVirtualizer.getTotalSize(), position: 'relative' }}>
          {rowVirtualizer.getVirtualItems().map((virtualRow) => {
            const result = results[virtualRow.index];
            const criteria = result.criteria;
            const criteriaKeys = Object.keys(criteria) as (keyof typeof criteria)[];

            return (
              <div
                key={result.code}
                onClick={() => navigate(`/stock/${result.code}/chart`)}
                style={{
                  position: 'absolute',
                  top: virtualRow.start,
                  left: 0,
                  width: '100%',
                  height: ROW_HEIGHT,
                  padding: '0 16px',
                  display: 'grid',
                  gridTemplateColumns: '80px 1fr 96px 96px 80px 80px 1fr 140px',
                  alignItems: 'center',
                  borderBottom: '1px solid var(--ant-color-border-secondary)',
                  cursor: 'pointer',
                  background: virtualRow.index % 2 === 0 ? 'transparent' : 'var(--ant-color-fill-quaternary)',
                }}
              >
                <Text code>{result.code}</Text>
                <Text ellipsis style={{ padding: '0 12px' }}>{result.name || '-'}</Text>
                <Text style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{result.price.toFixed(2)}</Text>
                <Text style={{ textAlign: 'right', color: getPriceColor(result.change_pct), fontVariantNumeric: 'tabular-nums' }}>{formatPercent(result.change_pct)}</Text>
                <Text style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{result.volume_ratio.toFixed(2)}</Text>
                <Text style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{result.turnover_rate.toFixed(2)}%</Text>
                <div style={{ paddingLeft: 12 }}>
                  <Space size={[4, 0]} wrap>
                    {criteriaKeys.map((key) => (
                      <Tag key={key} color={criteria[key] ? 'green' : 'red'} style={{ fontSize: 10 }}>
                        {criteria[key] ? '✓' : '✗'}
                      </Tag>
                    ))}
                  </Space>
                </div>
                <div style={{ textAlign: 'center' }}>
                  <Space size={4}>
                    <Button
                      size="small"
                      type="primary"
                      icon={<ShoppingCartOutlined />}
                      onClick={(e) => { e.stopPropagation(); handleBuy(result); }}
                      disabled={tradingLoading || (trades[result.code]?.action === 'buy')}
                      style={{ padding: '4px 8px', fontSize: 11 }}
                    >
                      买入
                    </Button>
                    <Button
                      size="small"
                      danger
                      icon={<MinusCircleOutlined />}
                      onClick={(e) => { e.stopPropagation(); handleSell(result); }}
                      disabled={tradingLoading || (!trades[result.code] || trades[result.code].action !== 'buy')}
                      style={{ padding: '4px 8px', fontSize: 11 }}
                    >
                      卖出
                    </Button>
                  </Space>
                  {trades[result.code] && (
                    <div style={{ fontSize: 10, marginTop: 2, color: trades[result.code].action === 'buy' ? '#ff4d4f' : '#52c41a' }}>
                      {trades[result.code].action === 'buy' ? `已买入@${trades[result.code].price.toFixed(2)}` : '已卖出'}
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </Card>
  );
}