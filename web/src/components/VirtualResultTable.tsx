import { useVirtualizer } from '@tanstack/react-virtual';
import { Card, Space, Button, Tag, Typography } from 'antd';
import { ShoppingCartOutlined, MinusCircleOutlined } from '@ant-design/icons';
import type { RefObject } from 'react';
import type { ScreenResult } from '../types/api';
import type { TradeInfo, SortKey } from '../types/screen';
import { ROW_HEIGHT } from '../types/screen';
import { SortHeader } from './SortHeader';
import { formatPercent, getChangePct, getPriceColor, getMaTrend, isBuySignal } from '../utils/screen';

const { Text } = Typography;

interface VirtualResultTableProps {
  results: ScreenResult[];
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
    <Card bodyStyle={{ padding: 0 }} extra={extra}>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '80px 1fr 96px 96px 80px 1fr 140px',
          gap: 0,
          padding: '0 16px',
          borderBottom: '1px solid var(--ant-color-border-secondary)',
          background: 'var(--ant-color-fill-quaternary)',
        }}
      >
        <div style={{ padding: '10px 0' }}><SortHeader sortKey="code" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>代码</SortHeader></div>
        <div style={{ padding: '10px 12px' }}><SortHeader sortKey="name" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>名称</SortHeader></div>
        <div style={{ padding: '10px 0' }}><SortHeader sortKey="close" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">收盘</SortHeader></div>
        <div style={{ padding: '10px 0' }}><SortHeader sortKey="change" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">涨跌幅</SortHeader></div>
        <div style={{ padding: '10px 0', textAlign: 'right', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>MA趋势</div>
        <div style={{ padding: '10px 12px', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>信号</div>
        <div style={{ padding: '10px 0', textAlign: 'center', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>虚拟操作</div>
      </div>

      <div ref={tableContainerRef} style={{ maxHeight: 'calc(100vh - 360px)', minHeight: 320, overflow: 'auto' }}>
        <div style={{ height: rowVirtualizer.getTotalSize(), position: 'relative' }}>
          {rowVirtualizer.getVirtualItems().map((virtualRow) => {
            const result = results[virtualRow.index];
            const close = result.last?.Close || 0;
            const changePct = getChangePct(result);
            const signals = result.signals || [];
            const maTrend = getMaTrend(result);

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
                  gridTemplateColumns: '80px 1fr 96px 96px 80px 1fr 140px',
                  alignItems: 'center',
                  borderBottom: '1px solid var(--ant-color-border-secondary)',
                  cursor: 'pointer',
                  background: virtualRow.index % 2 === 0 ? 'transparent' : 'var(--ant-color-fill-quaternary)',
                }}
              >
                <Text code>{result.code}</Text>
                <Text ellipsis style={{ padding: '0 12px' }}>{result.name || '-'}</Text>
                <Text style={{ textAlign: 'right', color: getPriceColor(changePct), fontVariantNumeric: 'tabular-nums' }}>{close.toFixed(2)}</Text>
                <Text style={{ textAlign: 'right', color: getPriceColor(changePct), fontVariantNumeric: 'tabular-nums' }}>{formatPercent(changePct)}</Text>
                <Tag color={maTrend.color} style={{ justifySelf: 'end', fontSize: 12 }}>{maTrend.label}</Tag>
                <div style={{ paddingLeft: 12 }}>
                  {signals.length > 0 ? (
                    <Space size={[4, 0]} wrap>
                      {signals.map((signal, index) => (
                        <Tag key={`${result.code}-${signal.Type}-${index}`} color={isBuySignal(signal.Type) ? 'red' : 'green'} style={{ fontSize: 11 }}>
                          {signal.Indicator}{signal.Type}
                        </Tag>
                      ))}
                    </Space>
                  ) : (
                    <Text type="secondary" style={{ fontSize: 12 }}>无信号</Text>
                  )}
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