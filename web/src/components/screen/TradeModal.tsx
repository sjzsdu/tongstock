import { Modal, Button, Card, Flex, Space, Typography, Input } from 'antd';
import type { ScreenResult } from '../../types/api';
import type { TradeInfo } from '../../types/screen';

const { Text } = Typography;

interface TradeModalProps {
  visible: boolean;
  onCancel: () => void;
  onConfirm: () => void;
  currentTradeAction: 'buy' | 'sell';
  currentTradeStock: ScreenResult | null;
  tradeReason: string;
  onTradeReasonChange: (value: string) => void;
  tradingLoading: boolean;
  trades: Record<string, TradeInfo>;
}

export function TradeModal({
  visible,
  onCancel,
  onConfirm,
  currentTradeAction,
  currentTradeStock,
  tradeReason,
  onTradeReasonChange,
  tradingLoading,
  trades,
}: TradeModalProps) {
  if (!currentTradeStock) return null;

  const close = currentTradeStock.last?.Close || 0;
  const currentTrade = trades[currentTradeStock.code];
  const profit = currentTradeAction === 'sell' && currentTrade
    ? ((close - currentTrade.price) / currentTrade.price * 100)
    : 0;

  return (
    <Modal
      title={currentTradeAction === 'buy' ? '确认买入' : '确认卖出'}
      open={visible}
      onCancel={onCancel}
      footer={[
        <Button key="cancel" onClick={onCancel}>取消</Button>,
        <Button key="confirm" type="primary" loading={tradingLoading} onClick={onConfirm}>
          {currentTradeAction === 'buy' ? '确认买入' : '确认卖出'}
        </Button>,
      ]}
      width={480}
    >
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Card size="small" style={{ marginBottom: 0 }}>
          <Flex justify="space-between" align="center">
            <Space>
              <Text code>{currentTradeStock.code}</Text>
              <Text>{currentTradeStock.name}</Text>
            </Space>
            <Text color={currentTradeAction === 'buy' ? 'red' : 'green'}>
              {currentTradeAction === 'buy' ? '买入' : '卖出'}
            </Text>
          </Flex>
          <Flex justify="space-between" style={{ marginTop: 12 }}>
            <Text type="secondary">当前价格</Text>
            <Text strong style={{ fontVariantNumeric: 'tabular-nums' }}>
              {close.toFixed(2)}
            </Text>
          </Flex>
          {currentTradeAction === 'sell' && currentTrade && (
            <>
              <Flex justify="space-between" style={{ marginTop: 12 }}>
                <Text type="secondary">买入成本</Text>
                <Text style={{ fontVariantNumeric: 'tabular-nums' }}>
                  {currentTrade.price.toFixed(2)}
                </Text>
              </Flex>
              <Flex justify="space-between" style={{ marginTop: 12 }}>
                <Text type="secondary">预估盈亏</Text>
                <Text
                  style={{
                    fontVariantNumeric: 'tabular-nums',
                    color: profit >= 0 ? '#22c55e' : '#ef4444',
                  }}
                >
                  {profit >= 0 ? `+${profit.toFixed(2)}%` : `${profit.toFixed(2)}%`}
                </Text>
              </Flex>
            </>
          )}
        </Card>
        <div>
          <Text type="secondary" style={{ fontSize: 12 }}>交易理由（可选）</Text>
          <Input.TextArea
            value={tradeReason}
            onChange={(e) => onTradeReasonChange(e.target.value)}
            placeholder="输入买入/卖出的理由，便于后期复盘分析..."
            rows={3}
            style={{ marginTop: 8 }}
          />
        </div>
      </Space>
    </Modal>
  );
}