import { useState, useCallback } from 'react';
import { api } from '../api/client';
import type { TradeInfo, OvernightCandidate } from '../api/client';

interface UseOvernightTradeReturn {
  trades: Record<string, TradeInfo>;
  tradingLoading: boolean;
  loadTrades: (codes: string[]) => Promise<void>;
  handleBuy: (result: OvernightCandidate) => { canBuy: boolean; reason?: string };
  handleSell: (result: OvernightCandidate) => { canSell: boolean; reason?: string };
  confirmTrade: (result: OvernightCandidate, action: 'buy' | 'sell', reason: string) => Promise<{ success: boolean; message: string }>;
}

export function useOvernightTrade(): UseOvernightTradeReturn {
  const [trades, setTrades] = useState<Record<string, TradeInfo>>({});
  const [tradingLoading, setTradingLoading] = useState(false);

  const loadTrades = useCallback(async (codes: string[]) => {
    if (codes.length === 0) return;
    try {
      const response = await api.trades(codes.join(','));
      setTrades(response);
    } catch {
      setTrades({});
    }
  }, []);

  const handleBuy = useCallback((result: OvernightCandidate) => {
    const currentTrade = trades[result.code];
    if (currentTrade && currentTrade.action === 'buy') {
      return { canBuy: false, reason: '已持有该股票' };
    }
    if (result.price <= 0) {
      return { canBuy: false, reason: '无法获取当前价格' };
    }
    return { canBuy: true };
  }, [trades]);

  const handleSell = useCallback((result: OvernightCandidate) => {
    const currentTrade = trades[result.code];
    if (!currentTrade || currentTrade.action !== 'buy') {
      return { canSell: false, reason: '未持有该股票' };
    }
    if (result.price <= 0) {
      return { canSell: false, reason: '无法获取当前价格' };
    }
    return { canSell: true };
  }, [trades]);

  const confirmTrade = useCallback(async (
    result: OvernightCandidate,
    action: 'buy' | 'sell',
    reason: string
  ): Promise<{ success: boolean; message: string }> => {
    const price = result.price;
    setTradingLoading(true);
    try {
      await api.tradeCreate({
        code: result.code,
        name: result.name || '',
        action,
        price,
        signal: '隔夜套利',
        ktype: 'day',
        reason,
      });

      if (action === 'buy') {
        await loadTrades([result.code]);
        return { success: true, message: `买入成功 @ ${price.toFixed(2)}` };
      } else {
        const currentTrade = trades[result.code];
        const profit = ((price - currentTrade.price) / currentTrade.price * 100).toFixed(2);
        const profitText = parseFloat(profit) >= 0 ? `+${profit}%` : `${profit}%`;
        await loadTrades([result.code]);
        return { success: true, message: `卖出成功 @ ${price.toFixed(2)} (${profitText})` };
      }
    } catch {
      return { success: false, message: `${action === 'buy' ? '买入' : '卖出'}失败` };
    } finally {
      setTradingLoading(false);
    }
  }, [trades, loadTrades]);

  return { trades, tradingLoading, loadTrades, handleBuy, handleSell, confirmTrade };
}