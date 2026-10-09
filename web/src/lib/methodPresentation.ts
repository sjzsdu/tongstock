import type { MethodCard, MethodEvidence } from '../api/client';

export const PENDING_VALUE = '待验证';

const EVIDENCE_LABELS: Record<string, string> = {
  strong: '较强',
  moderate: '中等',
  weak: '较弱',
  insufficient: '不足',
  rejected: '未通过',
};

const STATUS_LABELS: Record<string, string> = {
  verified: '已验证',
  observing: '观察中',
  degraded: '已退化',
  candidate: '候选中',
  draft: '草稿',
  rejected: '未通过验证',
  retired: '已退役',
};

const EXCLUSION_LABELS: Record<string, string> = {
  method_not_found: '所选方法不存在或已删除',
  method_not_eligible: '方法还没有通过验证，暂不参与今日筛选',
  method_degraded: '方法近期表现退化，已暂停今日筛选',
  method_not_executable: '方法规则暂无法在当前数据上执行',
  evidence_insufficient: '历史验证证据不足，未达到选股门槛',
  universe_mismatch: '方法验证用的股票池与今日快照不一致',
  market_state_unavailable: '当前快照缺少方法所需的市场状态数据',
  historical_features_unavailable: '当前快照缺少判定该方法所需的历史特征',
  insufficient_data: '该股票缺少判定规则所需的数据',
  execution_failed: '规则执行失败',
  invalidation_matched: '该股票命中了方法的失效条件',
  factor_pick_unavailable: '因子名单暂不可用',
  factor_pick_stale: '因子名单已过期',
  scope_excluded: '不在该方法定义的股票池范围内',
  scope_data_unavailable: '当前快照缺少该方法股票池所需的数据',
};

export function formatPercent(value?: number, digits = 1): string {
  return value === undefined || Number.isNaN(value) ? PENDING_VALUE : `${(value * 100).toFixed(digits)}%`;
}

export function formatRatio(value?: number): string {
  return value === undefined || Number.isNaN(value) ? PENDING_VALUE : value.toFixed(2);
}

export function outcomeLabel(outcome?: MethodCard['outcome']): string {
  if (!outcome?.horizon_days || outcome.target_return_pct === undefined) return PENDING_VALUE;
  const basis = ({ close: '收盘价', open: '开盘价', high: '最高价', low: '最低价' } as Record<string, string>)[outcome.price_basis ?? ''] ?? '价格';
  const direction = outcome.success === 'price_lte_target' ? '跌到' : '达到';
  return `${outcome.horizon_days} 个交易日内${basis}${direction} ${(outcome.target_return_pct * 100).toFixed(1)}%`;
}

export function scopeLabel(method: MethodCard): string {
  const scope = method.scope;
  if (!scope) return method.universe || '未记录';
  const parts: string[] = [];
  if (scope.market_cap_min !== undefined || scope.market_cap_max !== undefined) {
    const min = scope.market_cap_min === undefined ? '不限' : `${scope.market_cap_min}亿`;
    const max = scope.market_cap_max === undefined ? '不限' : `${scope.market_cap_max}亿`;
    parts.push(`市值 ${min}–${max}`);
  }
  if (scope.board_filter?.length) parts.push(scope.board_filter.join('、'));
  if (scope.exclude_st) parts.push('排除 ST');
  return parts.length ? parts.join(' · ') : (scope.universe || method.universe || '未记录');
}

export function evidenceLevel(evidence?: MethodEvidence): string {
  return evidence ? (EVIDENCE_LABELS[evidence.confidence] ?? evidence.confidence) : PENDING_VALUE;
}

export function methodStatusLabel(status: string): string {
  return STATUS_LABELS[status] ?? status;
}

export function methodCanScreen(method: MethodCard): boolean {
  return (
    (method.status === 'verified' || method.status === 'observing') &&
    method.evidence?.passable === true &&
    (method.evidence.confidence === 'moderate' || method.evidence.confidence === 'strong')
  );
}

export function methodUnavailableReason(method: MethodCard): string | undefined {
  if (methodCanScreen(method)) return undefined;
  if (method.status === 'degraded') return '近期表现退化，已暂停用于今日筛选';
  if (method.status === 'retired') return '该方法已退役';
  if (method.status !== 'verified' && method.status !== 'observing') return '该方法还没有通过历史验证';
  if (!method.evidence) return '还没有可用的历史验证记录';
  if (!method.evidence.passable) return '历史验证未达到选股门槛';
  return '证据等级低于今日筛选要求';
}

export function exclusionReasonLabel(reasonCode: string): string {
  return EXCLUSION_LABELS[reasonCode] ?? `其他原因（${reasonCode}）`;
}

export function evidenceTone(evidence?: MethodEvidence): 'success' | 'warning' | 'error' | 'default' {
  if (!evidence) return 'default';
  if (evidence.passable && evidence.confidence === 'strong') return 'success';
  if (evidence.passable) return 'warning';
  return 'error';
}
