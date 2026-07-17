import { useEffect, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { ShoppingCartOutlined, ArrowUpOutlined, ArrowDownOutlined, CalendarOutlined, TagOutlined, MessageOutlined, MinusCircleOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Empty, Flex, Space, Statistic, Tag, Typography, message } from 'antd';
import { api, type TradeInfo } from '../api/client';
import type { QuoteItem } from '../types/api';

const { Paragraph, Text, Title } = Typography;

export default function Portfolio() {
  const navigate = useNavigate();
  const [messageApi, contextHolder] = message.useMessage();
  const [positions, setPositions] = useState<TradeInfo[]>([]);
  const [quotes, setQuotes] = useState<Record<string, QuoteItem>>({});
  const [loading, setLoading] = useState(true);
  const [sellingCode, setSellingCode] = useState<string | null>(null);

  const loadPositions = useCallback(async () => {
    setLoading(true);
    try {
      const response = await api.tradePositions();
      const pos = response.positions || [];
      setPositions(pos);

      if (pos.length > 0) {
        const codes = pos.map((p) => p.code).join(',');
        const quoteResponse = await api.quotes(codes);
        const quoteMap: Record<string, QuoteItem> = {};
        for (const q of quoteResponse) {
          quoteMap[q.Code] = q;
        }
        setQuotes(quoteMap);
      }
    } catch {
      setPositions([]);
      setQuotes({});
    } finally {
      setLoading(false);
    }
  }, []);

  const handleSell = async (position: TradeInfo) => {
    if (sellingCode) return;

    const quote = quotes[position.code];
    if (!quote) {
      messageApi.error('无法获取当前价格');
      return;
    }

    setSellingCode(position.code);
    try {
      await api.tradeCreate({
        code: position.code,
        name: position.name,
        action: 'sell',
        price: quote.Price,
        signal: position.signal,
        ktype: position.ktype,
      });

      const profit = ((quote.Price - position.price) / position.price * 100).toFixed(2);
      const profitText = parseFloat(profit) >= 0 ? `+${profit}%` : `${profit}%`;
      messageApi.success(`卖出成功 @ ${quote.Price.toFixed(2)} (${profitText})`);
      await loadPositions();
    } catch {
      messageApi.error('卖出失败');
    } finally {
      setSellingCode(null);
    }
  };

  useEffect(() => {
    void loadPositions();
  }, [loadPositions]);

  const totalCost = positions.reduce((sum, p) => sum + p.price, 0);
  const currentValue = positions.reduce((sum, p) => {
    const quote = quotes[p.code];
    return sum + (quote?.Price || p.price);
  }, 0);
  const totalProfit = currentValue - totalCost;
  const totalProfitPct = totalCost > 0 ? (totalProfit / totalCost * 100) : 0;

  const profitableCount = positions.filter((p) => {
    const quote = quotes[p.code];
    return quote && quote.Price > p.price;
  }).length;

  return (
    <>
      {contextHolder}
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Flex justify="space-between" align="center" wrap="wrap" gap={12}>
          <div>
            <Title level={3} style={{ margin: 0 }}>虚拟持仓</Title>
            <Paragraph type="secondary" style={{ marginBottom: 0 }}>
              模拟交易的当前持仓，实时显示盈亏情况。
            </Paragraph>
          </div>
          <Button icon={<ShoppingCartOutlined />} onClick={() => navigate('/screen')}>
            去筛选
          </Button>
        </Flex>

        {positions.length > 0 ? (
          <Card>
            <Flex wrap="wrap" gap={24}>
              <Statistic
                title="持仓数量"
                value={positions.length}
                suffix="只"
                valueStyle={{ color: '#1677ff' }}
              />
              <Statistic
                title="持仓成本"
                value={totalCost}
                precision={2}
                prefix="¥"
              />
              <Statistic
                title="当前市值"
                value={currentValue}
                precision={2}
                prefix="¥"
              />
              <Statistic
                title="总盈亏"
                value={totalProfit}
                precision={2}
                prefix="¥"
                valueStyle={{ color: totalProfit >= 0 ? '#22c55e' : '#ef4444' }}
              />
              <Statistic
                title="盈亏比例"
                value={totalProfitPct}
                precision={2}
                suffix="%"
                valueStyle={{ color: totalProfitPct >= 0 ? '#22c55e' : '#ef4444' }}
              />
              <Statistic
                title="盈利占比"
                value={(profitableCount / positions.length * 100).toFixed(1)}
                suffix="%"
                valueStyle={{ color: '#1677ff' }}
              />
            </Flex>
          </Card>
        ) : null}

        {loading ? (
          <Card>
            <Flex justify="center" align="center" style={{ minHeight: 200 }}>
              <Text type="secondary">加载中...</Text>
            </Flex>
          </Card>
        ) : positions.length === 0 ? (
          <Card>
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={
                <Space direction="vertical">
                  <Text type="secondary">暂无持仓</Text>
                  <Button icon={<ShoppingCartOutlined />} onClick={() => navigate('/screen')}>
                    去信号筛选选股
                  </Button>
                </Space>
              }
            />
          </Card>
        ) : (
          <Flex wrap="wrap" gap={16}>
            {positions.map((position) => {
              const quote = quotes[position.code];
              const currentPrice = quote?.Price || position.price;
              const profit = currentPrice - position.price;
              const profitPct = (profit / position.price * 100);
              const isProfit = profit >= 0;

              return (
                <Card
                  key={position.code}
                  hoverable
                  style={{ width: 360, cursor: 'pointer' }}
                  onClick={() => navigate(`/stock/${position.code}/chart`)}
                  actions={[
                    <Button
                      key="sell"
                      danger
                      size="small"
                      icon={<MinusCircleOutlined />}
                      onClick={(e) => {
                        e.stopPropagation();
                        handleSell(position);
                      }}
                      loading={sellingCode === position.code}
                    >
                      卖出
                    </Button>,
                  ]}
                >
                  <Flex justify="space-between" align="center" style={{ marginBottom: 12 }}>
                    <Space style={{ flex: 1, overflow: 'hidden' }}>
                      <Text code style={{ fontSize: 14, flexShrink: 0 }}>{position.code}</Text>
                      <Text strong style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 160 }}>{position.name}</Text>
                    </Space>
                    <Tag color={isProfit ? 'green' : 'red'} style={{ flexShrink: 0 }}>
                      {isProfit ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
                      {isProfit ? `+${profitPct.toFixed(2)}%` : `${profitPct.toFixed(2)}%`}
                    </Tag>
                  </Flex>

                  <Flex justify="space-between" style={{ marginBottom: 8 }}>
                    <Text type="secondary" style={{ width: 80 }}>买入成本</Text>
                    <Text style={{ fontVariantNumeric: 'tabular-nums', textAlign: 'right', flex: 1 }}>¥{position.price.toFixed(2)}</Text>
                  </Flex>
                  <Flex justify="space-between" style={{ marginBottom: 8 }}>
                    <Text type="secondary" style={{ width: 80 }}>当前价格</Text>
                    <Text strong style={{ fontVariantNumeric: 'tabular-nums', textAlign: 'right', flex: 1, color: isProfit ? '#22c55e' : '#ef4444' }}>
                      ¥{currentPrice.toFixed(2)}
                    </Text>
                  </Flex>
                  <Flex justify="space-between" style={{ marginBottom: 12 }}>
                    <Text type="secondary" style={{ width: 80 }}>盈亏金额</Text>
                    <Text style={{ fontVariantNumeric: 'tabular-nums', textAlign: 'right', flex: 1, color: isProfit ? '#22c55e' : '#ef4444' }}>
                      {isProfit ? '+' : ''}¥{profit.toFixed(2)}
                    </Text>
                  </Flex>

                  <div style={{ borderTop: '1px solid var(--ant-color-border-secondary)', paddingTop: 12 }}>
                    {position.signal && (
                      <Flex align="center" gap={4} style={{ marginBottom: 6 }}>
                        <TagOutlined style={{ fontSize: 12, color: '#8c8c8c', flexShrink: 0 }} />
                        <Text type="secondary" style={{ fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>{position.signal}</Text>
                      </Flex>
                    )}
                    {position.reason && (
                      <Flex align="center" gap={4}>
                        <MessageOutlined style={{ fontSize: 12, color: '#8c8c8c', flexShrink: 0 }} />
                        <Text type="secondary" style={{ fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>{position.reason}</Text>
                      </Flex>
                    )}
                    <Flex align="center" gap={4} style={{ marginTop: 6 }}>
                      <CalendarOutlined style={{ fontSize: 12, color: '#8c8c8c', flexShrink: 0 }} />
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {new Date(position.created_at).toLocaleDateString('zh-CN')}
                      </Text>
                    </Flex>
                  </div>
                </Card>
              );
            })}
          </Flex>
        )}

        {positions.length > 0 && (
          <Card size="small" title="交易提示">
            <Alert
              type="info"
              showIcon
              message="交易风险提示"
              description={
                <Text type="secondary" style={{ fontSize: 12 }}>
                  本页面为模拟交易，不涉及真实资金。所有交易记录仅供学习和回测分析使用。
                </Text>
              }
            />
          </Card>
        )}
      </Space>
    </>
  );
}
