import { Modal, Button, Card, Flex, Space, Typography, Input } from 'antd';
import type { OvernightCandidate } from '../../api/client';
import type { TradeInfo } from '../../api/client';
import { formatPercent, getPriceColor } from '../../utils/screen';

const { Text } = Typography;

interface OvernightTradeModalProps {
  visible: boolean;
  onCancel: () => void;
  onConfirm: () => void;
  currentTradeAction: 'buy' | 'sell';
  currentTradeStock: OvernightCandidate | null;
  tradeReason: string;
  onTradeReasonChange: (value: string) => void;
  tradingLoading: boolean;
  trades: Record<string, TradeInfo>;
}

export function OvernightTradeModal({
  visible,
  onCancel,
  onConfirm,
  currentTradeAction,
  currentTradeStock,
  tradeReason,
  onTradeReasonChange,
  tradingLoading,
  trades,
}: OvernightTradeModalProps) {
  if (!currentTradeStock) return null;

  const price = currentTradeStock.price;
  const currentTrade = trades[currentTradeStock.code];
  const profit = currentTradeAction === 'sell' && currentTrade
    ? ((price - currentTrade.price) / currentTrade.price * 100)
    : 0;

  return (
    <Modal
      title={currentTradeAction === 'buy' ? '买入确认' : '卖出确认'}
      open={visible}
      onCancel={onCancel}
      footer={[
        <Button key="cancel" onClick={onCancel}>取消</Button>,
        <Button key="confirm" type={currentTradeAction === 'buy' ? 'primary' : undefined} danger={currentTradeAction === 'sell'} onClick={onConfirm} loading={tradingLoading}>
          {currentTradeAction === 'buy' ? '确认买入' : '确认卖出'}
        </Button>,
      ]}
      width={520}
    >
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Card size="small">
          <Flex justify="space-between" align="center">
            <Space direction="vertical">
              <Text code style={{ fontSize: 16 }}>{currentTradeStock.code}</Text>
              <Text style={{ fontSize: 16 }}>{currentTradeStock.name}</Text>
            </Space>
            <Text style={{ fontSize: 20, color: getPriceColor(currentTradeStock.change_pct) }}>
              {price.toFixed(2)}
            </Text>
          </Flex>
        </Card>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>涨幅</Text>
            <div style={{ fontSize: 14, color: getPriceColor(currentTradeStock.change_pct) }}>
              {formatPercent(currentTradeStock.change_pct)}
            </div>
          </div>
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>量比</Text>
            <div style={{ fontSize: 14 }}>{currentTradeStock.volume_ratio.toFixed(2)}</div>
          </div>
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>换手率</Text>
            <div style={{ fontSize: 14 }}>{currentTradeStock.turnover_rate.toFixed(2)}%</div>
          </div>
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>流通市值</Text>
            <div style={{ fontSize: 14 }}>{currentTradeStock.market_cap.toFixed(2)}亿</div>
          </div>
        </div>

        {currentTradeAction === 'sell' && currentTrade && (
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>预估盈亏</Text>
            <div style={{ fontSize: 16, color: profit >= 0 ? '#ef4444' : '#22c55e' }}>
              {profit >= 0 ? `+${profit.toFixed(2)}%` : `${profit.toFixed(2)}%`}
            </div>
          </div>
        )}

        <Input.TextArea
          value={tradeReason}
          onChange={(event) => onTradeReasonChange(event.target.value)}
          placeholder="输入交易备注（可选）"
          rows={3}
        />
      </Space>
    </Modal>
  );
}