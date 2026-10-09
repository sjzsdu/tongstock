import type { MethodCard, MethodEvidence, MethodRuleExpr } from '../api/client';

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
  if (!outcome?.horizon_days) return PENDING_VALUE;
  // Mined methods validate "positive N-day return" with no explicit target
  // (TargetReturnPct == nil, close_gte_target fallback in the researcher).
  // That is a knowable definition — do not show 待验证 for it.
  if (outcome.target_return_pct === undefined) {
    const fallbackBasis = ({ close: '收盘', open: '开盘', high: '最高', low: '最低' } as Record<string, string>)[outcome.price_basis ?? 'close'] ?? '收盘';
    return `${outcome.horizon_days} 个交易日内${fallbackBasis}收益 > 0`;
  }
  const basis = ({ close: '收盘价', open: '开盘价', high: '最高价', low: '最低价' } as Record<string, string>)[outcome.price_basis ?? ''] ?? '价格';
  const direction = outcome.success === 'price_lte_target' ? '跌到' : '达到';
  return `${outcome.horizon_days} 个交易日内${basis}${direction} ${(outcome.target_return_pct * 100).toFixed(1)}%`;
}

const UNIVERSE_LABELS: Record<string, string> = {
  universe_usable: '今日可用股票池',
  universe_all: '全市场股票池',
  universe_all_a: '全A股池',
  universe_csi800: '中证800成分池',
  researched_stocks: '研究样本池',
};

export function universeLabel(universe?: string): string {
  if (!universe) return '未记录';
  return UNIVERSE_LABELS[universe] ? `${UNIVERSE_LABELS[universe]}（${universe}）` : universe;
}

const INDICATOR_LABELS: Record<string, string> = {
  close: '收盘价',
  open: '开盘价',
  high: '最高价',
  low: '最低价',
  volume: '成交量',
  amount: '成交额',
  pe: '市盈率',
  turnover_rate: '换手率',
  macd_dif: 'DIF',
  macd_dea: 'DEA',
  macd_hist: 'MACD柱',
  gap_pct: '跳空幅度',
};

function indicatorLabel(name: string): string {
  if (INDICATOR_LABELS[name]) return INDICATOR_LABELS[name];
  const ma = /^ma(\d+)$/.exec(name);
  if (ma) return `${ma[1]}日均线`;
  const volma = /^volma(\d+)$/.exec(name);
  if (volma) return `${volma[1]}日均量`;
  const prevhigh = /^prevhigh(\d+)$/.exec(name);
  if (prevhigh) return `前${prevhigh[1]}日最高`;
  const prevlow = /^prevlow(\d+)$/.exec(name);
  if (prevlow) return `前${prevlow[1]}日最低`;
  return name;
}

const CMP_LABELS: Record<string, string> = { gt: '>', gte: '≥', lt: '<', lte: '≤', eq: '=' };

function numberText(v: number): string {
  return Number.isInteger(v) ? String(v) : String(Math.round(v * 1000) / 1000);
}

/** 把编译产物的规则 AST 渲染成人能读懂的中文条件式。 */
export function formatRuleExpr(expr?: MethodRuleExpr): string {
  if (!expr) return '';
  switch (expr.type) {
    case 'indicator':
      return indicatorLabel(expr.indicator ?? '');
    case 'constant':
      return numberText(expr.value ?? 0);
    case 'compare': {
      const op = CMP_LABELS[expr.op ?? ''] ?? expr.op ?? '?';
      return `${formatRuleExpr(expr.left)} ${op} ${formatRuleExpr(expr.right)}`;
    }
    case 'and':
      return (expr.children ?? []).map((c) => formatRuleExpr(c)).filter(Boolean).map((s) => `(${s})`).join(' 且 ') || '未记录条件';
    case 'or':
      return (expr.children ?? []).map((c) => formatRuleExpr(c)).filter(Boolean).map((s) => `(${s})`).join(' 或 ') || '未记录条件';
    case 'not':
      return `非（${formatRuleExpr(expr.children?.[0] ?? expr.left)}）`;
    case 'cross':
      return `${formatRuleExpr(expr.left)} ${expr.cross === 'below' ? '下穿' : '上穿'} ${formatRuleExpr(expr.right)}`;
    case 'in_window':
      return `最近 ${expr.window_days ?? '?'} 天内${expr.window_mode === 'all' ? '每天都' : '出现过'}（${formatRuleExpr(expr.children?.[0] ?? expr.left)}）`;
    case 'rank':
      return `${expr.rank_by ?? '指标'}在全市场${expr.rank_side === 'bottom' ? '最低' : '最高'} ${((expr.rank_pct ?? 0) * 100).toFixed(1)}%`;
    case 'ambiguous':
      return `歧义条件（${expr.ambiguous_source ?? '原始描述未保留'}）`;
    default:
      return expr.type;
  }
}

export function entryRuleText(method: MethodCard): string {
  return formatRuleExpr(method.rules?.entry_rule);
}

export function invalidRuleText(method: MethodCard): string {
  return formatRuleExpr(method.rules?.invalid_rule);
}

/** 由结构化退出/持仓规则生成摘要；没有结构化数据时回退到注册时的文字。 */
export function exitRuleSummary(method: MethodCard): string {
  const holding = method.rules?.holding;
  const parts: string[] = [];
  if (holding?.stop_loss_pct !== undefined) parts.push(`亏损 ${trimPct(Math.abs(holding.stop_loss_pct))}% 止损`);
  if (holding?.take_profit_pct !== undefined) parts.push(`盈利 ${trimPct(holding.take_profit_pct)}% 止盈`);
  if (holding?.trailing_stop_pct !== undefined) parts.push(`从最高点回撤 ${trimPct(holding.trailing_stop_pct)}% 移动止盈`);
  if (holding?.max_days) parts.push(`最长持有 ${holding.max_days} 个交易日`);
  if (holding?.min_days) parts.push(`至少持有 ${holding.min_days} 个交易日`);
  const exitRule = formatRuleExpr(method.rules?.exit_rule);
  if (exitRule && exitRule !== '未记录条件') parts.push(`触发退出条件（${exitRule}）即卖出`);
  if (!parts.length) return method.exit_summary || '未设定';
  return parts.join('；');
}

function trimPct(v: number): string {
  return String(Math.round(v * 10000) / 100);
}

/**
 * 选股引擎只持有当日冻结特征快照；与后端 hasTemporal 保持同一口径，
 * 只看入场规则——入场规则含 cross/in_window 时引擎会整方法排除。
 */
export function ruleNeedsHistory(method: MethodCard): boolean {
  const needs = (expr?: MethodRuleExpr): boolean => {
    if (!expr) return false;
    if (expr.type === 'cross' || expr.type === 'in_window') return true;
    return needs(expr.left) || needs(expr.right) || (expr.children ?? []).some(needs);
  };
  return needs(method.rules?.entry_rule);
}

export function scopeLabel(method: MethodCard): string {
  const scope = method.scope;
  if (!scope) return universeLabel(method.universe);
  const parts: string[] = [];
  if (scope.market_cap_min !== undefined || scope.market_cap_max !== undefined) {
    const min = scope.market_cap_min === undefined ? '不限' : `${scope.market_cap_min}亿`;
    const max = scope.market_cap_max === undefined ? '不限' : `${scope.market_cap_max}亿`;
    parts.push(`市值 ${min}–${max}`);
  }
  if (scope.board_filter?.length) parts.push(scope.board_filter.join('、'));
  if (scope.exclude_st) parts.push('排除 ST');
  return parts.length ? parts.join(' · ') : universeLabel(scope.universe || method.universe);
}

export function evidenceLevel(evidence?: MethodEvidence): string {
  return evidence ? (EVIDENCE_LABELS[evidence.confidence] ?? evidence.confidence) : PENDING_VALUE;
}

export function methodStatusLabel(status: string): string {
  return STATUS_LABELS[status] ?? status;
}

export function methodCanScreen(method: MethodCard): boolean {
  if (ruleNeedsHistory(method)) return false;
  return (
    (method.status === 'verified' || method.status === 'observing') &&
    method.evidence?.passable === true &&
    (method.evidence.confidence === 'moderate' || method.evidence.confidence === 'strong')
  );
}

export function methodUnavailableReason(method: MethodCard): string | undefined {
  if (methodCanScreen(method)) return undefined;
  if (ruleNeedsHistory(method)) return '该方法的规则需要回看历史特征，今日筛选引擎暂不支持，点击会被整体排除';
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
