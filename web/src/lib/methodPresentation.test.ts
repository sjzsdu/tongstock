import { describe, expect, it } from 'vitest';
import type { MethodCard } from '../api/client';
import {
  evidenceLevel,
  exclusionReasonLabel,
  formatPercent,
  methodCanScreen,
  methodStatusLabel,
  methodUnavailableReason,
  outcomeLabel,
  scopeLabel,
} from './methodPresentation';

function method(overrides: Partial<MethodCard> = {}): MethodCard {
  return {
    id: 'method-1',
    name: '连续放量',
    status: 'verified',
    market: 'CN-A',
    universe: 'universe_usable',
    holding_period: '5 日',
    entry_summary: '连续 3 日成交量放大',
    exit_summary: '持有 5 日',
    updated_at: '2026-01-01T00:00:00Z',
    evidence: {
      confidence: 'strong',
      passable: true,
      oos_trades: 120,
      oos_return: 0.12,
      oos_win_rate: 0.71,
      oos_max_drawdown: -0.08,
    },
    ...overrides,
  };
}

describe('methodPresentation', () => {
  it('把历史胜率格式化为百分比，不把它叫做可信度', () => {
    expect(formatPercent(0.713)).toBe('71.3%');
    expect(formatPercent(undefined)).toBe('待验证');
  });

  it('把机器枚举翻译成用户语言', () => {
    expect(evidenceLevel(method().evidence)).toBe('较强');
    expect(methodStatusLabel('observing')).toBe('观察中');
    expect(exclusionReasonLabel('universe_mismatch')).toContain('股票池');
  });

  it('只允许已验证且证据达标的方法筛选今日股票', () => {
    expect(methodCanScreen(method())).toBe(true);
    const rejected = method({ status: 'candidate' });
    expect(methodCanScreen(rejected)).toBe(false);
    expect(methodUnavailableReason(rejected)).toContain('没有通过历史验证');
  });

  it('展示方法自己声明的验证口径和股票池', () => {
    const target = method({
      scope: { market_cap_min: 30, market_cap_max: 100, exclude_st: true },
      outcome: { horizon_days: 20, target_return_pct: 0.1, price_basis: 'high', success: 'price_gte_target' },
    });
    expect(outcomeLabel(target.outcome)).toBe('20 个交易日内最高价达到 10.0%');
    expect(scopeLabel(target)).toContain('市值 30亿–100亿');
    expect(scopeLabel(target)).toContain('排除 ST');
  });
});
