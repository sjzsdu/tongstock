import { useVirtualizer } from '@tanstack/react-virtual';
import { Card, Space, Button, Tag, Typography } from 'antd';
import { ShoppingCartOutlined, MinusCircleOutlined } from '@ant-design/icons';
import type { RefObject } from 'react';
import type { ScreenResult, DimensionMeta } from '../types/api';
import type { TradeInfo, SortKey } from '../types/screen';
import { ROW_HEIGHT } from '../types/screen';
import { SortHeader } from './SortHeader';
import { formatPercent, getChangePct, getPriceColor } from '../utils/screen';

const { Text } = Typography;

interface VirtualResultTableProps {
  results: ScreenResult[];
  dimensions: DimensionMeta[];
  tableContainerRef: RefObject<HTMLDivElement | null>;
  sortKey: SortKey;
  sortAsc: boolean;
  onSortChange: (key: SortKey) => void;
  navigate: (path: string) => void;
  extra?: React.ReactNode;
  trades: Record<string, TradeInfo>;
  tradingLoading: boolean;
  handleBuy: (result: ScreenResult) => void;
  handleSell: (result: ScreenResult) => void;
}

export function VirtualResultTable({
  results,
  dimensions,
  tableContainerRef,
  sortKey,
  sortAsc,
  onSortChange,
  navigate,
  extra,
  trades,
  tradingLoading,
  handleBuy,
  handleSell,
}: VirtualResultTableProps) {
  const rowVirtualizer = useVirtualizer({
    count: results.length,
    getScrollElement: () => tableContainerRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 18,
  });

  return (
    <Card styles={{ body: { padding: 0, overflowX: 'auto' } }} extra={extra}>
      <div style={{ minWidth: 1120 }}>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '80px 100px 110px 90px 90px repeat(4, minmax(120px, 1fr)) 140px',
          gap: 0,
          padding: '0 16px',
          borderBottom: '1px solid var(--ant-color-border-secondary)',
          background: 'var(--ant-color-fill-quaternary)',
        }}
      >
        <div style={{ padding: '10px 0' }}><SortHeader sortKey="code" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>代码</SortHeader></div>
        <div style={{ padding: '10px 12px' }}><SortHeader sortKey="name" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>名称</SortHeader></div>
        <div style={{ padding: '10px 0' }}>快照日期</div>
        <div style={{ padding: '10px 0' }}><SortHeader sortKey="close" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">收盘</SortHeader></div>
        <div style={{ padding: '10px 0' }}><SortHeader sortKey="change" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">涨跌幅</SortHeader></div>
        {dimensions.map((dim) => <div key={dim.dim} style={{ padding: '10px 8px' }}>{dim.label}</div>)}
        <div style={{ padding: '10px 0', textAlign: 'center', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>虚拟操作</div>
      </div>

      <div ref={tableContainerRef} style={{ maxHeight: 'calc(100vh - 360px)', minHeight: 320, overflow: 'auto' }}>
        <div style={{ height: rowVirtualizer.getTotalSize(), position: 'relative' }}>
          {rowVirtualizer.getVirtualItems().map((virtualRow) => {
            const result = results[virtualRow.index];
            const close = result.last?.Close || 0;
            const changePct = getChangePct(result);

            return (
              <div
                key={`${result.code}-${result.date}`}
                role="link"
                tabIndex={0}
                aria-label={`${result.name || result.code} ${result.date} 状态时序`}
                onKeyDown={(event) => { if (event.target === event.currentTarget && event.key === 'Enter') navigate(`/stock/${result.code}/chart?date=${result.date}`); }}
                onClick={() => navigate(`/stock/${result.code}/chart?date=${result.date}`)}
                style={{
                  position: 'absolute',
                  top: virtualRow.start,
                  left: 0,
                  width: '100%',
                  height: ROW_HEIGHT,
                  padding: '0 16px',
                  display: 'grid',
                  gridTemplateColumns: '80px 100px 110px 90px 90px repeat(4, minmax(120px, 1fr)) 140px',
                  alignItems: 'center',
                  borderBottom: '1px solid var(--ant-color-border-secondary)',
                  cursor: 'pointer',
                  background: virtualRow.index % 2 === 0 ? 'transparent' : 'var(--ant-color-fill-quaternary)',
                }}
              >
                <Text code>{result.code}</Text>
                <Text ellipsis style={{ padding: '0 12px' }}>{result.name || '-'}</Text>
                <Text>{result.date}</Text>
                <Text style={{ textAlign: 'right', color: getPriceColor(changePct), fontVariantNumeric: 'tabular-nums' }}>{result.last?.Close == null ? '-' : close.toFixed(2)}</Text>
                <Text style={{ textAlign: 'right', color: getPriceColor(changePct), fontVariantNumeric: 'tabular-nums' }}>{result.last?.PrevClose ? formatPercent(changePct) : '-'}</Text>
                {dimensions.map((dim) => {
                  const meta = dim.states.find((state) => state.state === result.states[dim.dim]);
                  return <Tag key={dim.dim} color={meta?.color} style={{ marginInline: 8 }} title={meta?.description}>{meta?.label}（{meta?.direction_label}）</Tag>;
                })}
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
      </div>
    </Card>
  );
}