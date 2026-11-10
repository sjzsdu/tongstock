import type { MethodCard, MethodRuleExpr, SelectionCandidate, SelectionTrigger } from '../api/client';
import { formatRuleExpr, indicatorLabel, isPercentIndicator, universeLabel } from './methodPresentation';

export interface ExplainedFact { path: string; condition: string; actual: string; passed: boolean; mapped: boolean; indicatorKey?: string }

export function ruleNodes(expr?: MethodRuleExpr, path = 'entry', result = new Map<string, MethodRuleExpr>()) {
  if (!expr) return result;
  result.set(path, expr);
  expr.children?.forEach((child, index) => ruleNodes(child, `${path}[${index}]`, result));
  if (expr.left) ruleNodes(expr.left, `${path}.left`, result);
  if (expr.right) ruleNodes(expr.right, `${path}.right`, result);
  return result;
}

function valueText(value: number, indicator?: string) {
  if (indicator && isPercentIndicator(indicator)) return `${Number((value * 100).toFixed(2))}%`;
  const unit = indicator && /^volratio\d+$/.test(indicator) ? '倍'
    : indicator && /^(close|open|high|low|ma\d+|ema\d+)$/.test(indicator) ? '元' : '';
  return `${Number(value.toFixed(4))}${unit}`;
}

function operand(expr: MethodRuleExpr | undefined, value: number, context?: string): string {
  if (expr?.type === 'indicator') return `${indicatorLabel(expr.indicator || '')} ${valueText(value, expr.indicator)}`;
  if (expr?.type === 'constant') return `阈值 ${valueText(value, context)}`;
  return '未保留指标信息';
}

/** Only AST paths identify indicators; numeric trace values never determine their names. */
export function explainTriggerFacts(trigger: SelectionTrigger, method?: MethodCard): ExplainedFact[] {
  const versionMatches = !!method?.method_version_id && method.method_version_id === trigger.method_version_id;
  const nodes = ruleNodes(versionMatches ? method?.rules?.entry_rule : undefined);
  return (trigger.facts || []).flatMap<ExplainedFact>((fact) => {
    const node = nodes.get(fact.path);
    // Constant/indicator leaves are operands, and boolean nodes are summaries.
    if (/\.(left|right)$/.test(fact.path) || ['constant', 'indicator', 'and', 'or', 'not'].includes(node?.type || fact.rule || '')) return [];
    if (!node || node.type !== 'compare' && !['cross', 'rank', 'in_window'].includes(node?.type || '')) {
      return [{ path: fact.path, condition: '未保留匹配版本的指标信息，无法还原条件', actual: '原始事实可在审计信息中查看', passed: fact.passed, mapped: false }];
    }
    const values = /left=([-+\d.eE]+)\s+op=\S+\s+right=([-+\d.eE]+)\s+ok=(true|false)/.exec(fact.detail || '');
    const actual = values && values[3] === 'true'
      ? `${operand(node.left, Number(values[1]), node.right?.indicator)}；${operand(node.right, Number(values[2]), node.left?.indicator)}`
      : '未保留可用的当前指标值';
    return [{ path: fact.path, condition: formatRuleExpr(node), actual, passed: fact.passed, mapped: true, indicatorKey: node.left?.indicator }];
  });
}

/** Keep OR branches intact; only the outer AND can become a list of required blocks. */
export function entryConditions(method?: MethodCard) {
  const rule = method?.rules?.entry_rule;
  if (!rule) return ['未保留结构化入场规则'];
  if (rule.type === 'and') return (rule.children || []).map(formatRuleExpr);
  return [formatRuleExpr(rule)];
}

export function methodPurpose(method?: MethodCard) {
  if (method?.description) return method.description;
  if (!method?.rules?.entry_rule) return '按已登记的历史方法筛选；当前未保留完整规则。';
  const nodes = [...ruleNodes(method.rules.entry_rule).values()];
  const has = (name: string) => nodes.some((node) => node.type === 'indicator' && node.indicator === name);
  const trend = nodes.some((node) => node.type === 'compare' && node.op === 'gt' && node.left?.indicator === 'close' && node.right?.indicator === 'ma20');
  if (trend && has('bias5') && has('volratio5')) return '寻找收盘站在20日均线上、短线回踩且成交量收缩的股票。';
  if (trend && has('volatility20')) return '寻找收盘站在20日均线上、近期日收益波动较低的股票。';
  return `依据以下结构化条件筛选：${formatRuleExpr(method.rules.entry_rule)}。`;
}

export function methodScopeText(method?: MethodCard) {
  if (!method) return '方法资料暂不可用';
  const scope = method.scope;
  const parts = [universeLabel(method.universe)];
  if (scope?.market_cap_min !== undefined) parts.push(`流通市值至少 ${scope.market_cap_min} 亿`);
  if (scope?.market_cap_max !== undefined) parts.push(`流通市值至多 ${scope.market_cap_max} 亿`);
  if (scope?.avg_shares_per_holder_min !== undefined) parts.push(`户均持股至少 ${scope.avg_shares_per_holder_min.toLocaleString()} 股`);
  if (scope?.avg_shares_per_holder_max !== undefined) parts.push(`户均持股至多 ${scope.avg_shares_per_holder_max.toLocaleString()} 股`);
  if (scope?.board_filter?.length) parts.push(`板块：${scope.board_filter.join('、')}`);
  if (scope?.exclude_st) parts.push('排除ST');
  if (scope?.avg_turnover_min !== undefined) parts.push(`近120日平均换手率至少 ${scope.avg_turnover_min}%`);
  if (scope?.avg_turnover_max !== undefined) parts.push(`近120日平均换手率至多 ${scope.avg_turnover_max}%`);
  return parts.join('；');
}

export function actionExplanation(candidate: SelectionCandidate) {
  if (candidate.action === 'insufficient_data') return '数据不足：无法完成入场判断。';
  if (candidate.action === 'avoid') return '回避：命中风险或失效条件。';
  if (candidate.triggers.some((trigger) => trigger.method_id === 'factor-combo' || trigger.method_id === 'factor_top_picks')) return '因子观察名单，不产生买入指令。';
  if (candidate.truncated_from_buy) return '观察：评分及退出计划达标，但各触发方法的买入名额已满。';
  if (candidate.score < .65) return `观察：总评分 ${candidate.score.toFixed(4)} 低于买入门槛 0.65。`;
  if (!candidate.exit.complete) return '观察：退出计划不完整，尚不具备买入资格。';
  if (candidate.action === 'buy') return '买入候选：总评分达到 0.65、退出计划完整，并通过本次名额筛选。';
  return '观察：以本次运行记录的动作和风险约束为准。';
}

export function aggregateCalculation(candidate: SelectionCandidate) {
  if (candidate.triggers.some((trigger) => trigger.method_id === 'factor-combo')) return { formula: '来自截面因子排序，仅观察，无普通方法聚合买入门槛', value: candidate.score };
  const scores = candidate.triggers.map((trigger) => trigger.score).sort((a, b) => b - a);
  if (!scores.length) return { formula: '没有保留触发方法评分', value: candidate.score };
  const saved = candidate.score_explanation;
  const primary = saved?.primary_score ?? scores[0];
  const bonuses = saved?.extra_trigger_bonuses ?? scores.slice(1).map((score) => Math.min(.03, score * .04));
  const cap = saved?.aggregate_cap ?? .9;
  const value = Math.round(Math.min(cap, primary + bonuses.reduce((a, b) => a + b, 0)) * 10000) / 10000;
  const terms = saved ? bonuses.map((bonus) => ` + ${bonus.toFixed(4)}`).join('') : bonuses.map((bonus, index) => ` + min(0.03, ${scores[index + 1].toFixed(4)} × 0.04) [${bonus.toFixed(4)}]`).join('');
  return { formula: `${saved ? '运行时保存的分项' : '旧记录按v4公式复核'}：${primary.toFixed(4)}${terms} = ${value.toFixed(4)}（上限 ${cap}）`, value };
}

export function indicatorHelp(method?: MethodCard) {
  const names = new Set([...ruleNodes(method?.rules?.entry_rule).values()].filter((node) => node.type === 'indicator').map((node) => node.indicator));
  const help: string[] = [];
  for (const name of names) {
    const ma = /^ma(\d+)$/.exec(name || '');
    const bias = /^bias(\d+)$/.exec(name || '');
    const vol = /^volratio(\d+)$/.exec(name || '');
    const volatility = /^volatility(\d+)$/.exec(name || '');
    if (ma) help.push(`${ma[1]}日均线：近${ma[1]}个交易日的平均收盘价。`);
    if (bias) help.push(`${bias[1]}日乖离率 = (收盘价/${bias[1]}日均线−1)；例如 -3% 到0%表示低于均线但偏离不到3%。`);
    if (vol) help.push(`${vol[1]}日量比 = 当日成交量/前${vol[1]}日均量；0.6倍表示当日成交量不足前期均量的六成。`);
    if (volatility) help.push(`${volatility[1]}日波动率：近${volatility[1]}日收益的波动程度，数值越低表示近期波动越小。`);
  }
  return help;
}

export function retainedEvidenceExplanation(trigger: SelectionTrigger) {
  const ev = trigger.evidence;
  if (!ev) return '本次记录未保留证据输入，基础分和证据加减项无法确认。';
  return `触发时证据等级 ${ev.confidence}，对应基础分 ${ev.confidence === 'strong' ? '0.7200' : '0.5500'}；样本外 ${ev.oos_trades} 笔${ev.oos_trades >= 100 ? '，满足≥100笔加0.0400' : '，未达到100笔、不加样本分'}；最大回撤 ${(ev.oos_max_drawdown * 100).toFixed(2)}%${ev.oos_max_drawdown < -.2 ? '，触发扣0.0800' : '，未触发回撤扣分'}。这些已知项不代表成交额和健康调整为零。`;
}

export function aggregateSummary(candidate: SelectionCandidate) {
  const scores = candidate.triggers.map((trigger) => trigger.score).sort((a,b) => b-a);
  const primary = candidate.score_explanation?.primary_score ?? scores[0] ?? 0;
  const bonuses = candidate.score_explanation?.extra_trigger_bonuses ?? scores.slice(1).map((score) => Math.min(.03, score*.04));
  return `最高方法分 ${primary.toFixed(4)}${bonuses.length ? ` + ${bonuses.length}份附加分（${bonuses.map((bonus) => bonus.toFixed(4)).join('、')}）` : ''} = ${aggregateCalculation(candidate).value.toFixed(4)}`;
}
