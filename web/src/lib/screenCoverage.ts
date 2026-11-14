import type { ScreenCoverage, ScreenSkipReason } from '../types/api';

export const SKIP_REASON_LABEL: Record<ScreenSkipReason, string> = {
  no_quote: '当日无行情',
  insufficient_history: '历史数据不足',
  not_computed: '状态待生成',
};

export const SKIP_REASON_HINT: Record<ScreenSkipReason, string> = {
  no_quote: '停牌、未上市或已退市，当天没有成交行情',
  insufficient_history: '上市时间较短，指标所需的历史数据不够',
  not_computed: '当天行情已经有了，但当日状态还没生成',
};

export function countSkipped(coverage: ScreenCoverage, reason: ScreenSkipReason): number {
  return coverage.skipped.filter((item) => item.reason === reason).length;
}
