import { describe, expect, it } from 'vitest';
import type { MethodCard, MethodRuleExpr, SelectionCandidate, SelectionTrigger } from '../api/client';
import { aggregateCalculation, retainedEvidenceExplanation, entryConditions, explainTriggerFacts, methodScopeText } from './candidateExplanation';
const indicator = (name: string): MethodRuleExpr => ({ type: 'indicator', indicator: name });
const compare = (name: string, value: number): MethodRuleExpr => ({ type: 'compare', op: 'lt', left: indicator(name), right: { type: 'constant', value } });
const method = (entry: MethodRuleExpr): MethodCard => ({ id: 'a', method_version_id: 'v1', rules: { entry_rule: entry } } as MethodCard);
const trigger: SelectionTrigger = { method_id: 'a', method_version_id: 'v1', method_name: '规则', score: .59, facts: [
  { path: 'entry', rule: 'and', passed: true },
  { path: 'entry[0]', rule: '(bias5 lt 0)', passed: true, detail: 'left=-0.0112 op=lt right=0.0000 ok=true' },
  { path: 'entry[0].left', rule: 'indicator(bias5)', passed: true, detail: 'bias5=-0.0112' },
  { path: 'entry[0].right', rule: 'constant', passed: true, detail: '0.0000' },
  { path: 'entry[1]', rule: '(volratio5 lt 0.6)', passed: false, detail: 'left=0.7000 op=lt right=0.6000 ok=true' },
] };

describe('candidate explanation from retained AST and traces', () => {
  it('labels retained price facts with their actual indicators and currency', () => {
    const priceRule: MethodRuleExpr = { type: 'compare', op: 'gt', left: indicator('close'), right: indicator('ma20') };
    const recorded = { ...trigger, facts: [{ path: 'entry', passed: true, detail: 'left=5.2700 op=gt right=5.2405 ok=true' }] };
    const [fact] = explainTriggerFacts(recorded, method(priceRule));
    expect(fact.actual).toBe('收盘价 5.27元；20日均线 5.2405元');
    expect(fact.indicatorKey).toBe('close');
  });
  it('maps actual executor paths, filters operand leaves and summaries, uses percentage and ratio units', () => {
    const facts = explainTriggerFacts(trigger, method({ type: 'and', children: [compare('bias5', 0), compare('volratio5', .6)] }));
    expect(facts).toHaveLength(2);
    expect(facts[0].condition).toContain('5日乖离率');
    expect(facts[0].actual).toContain('-1.12%');
    expect(facts[1].condition).toContain('0.6');
    expect(facts[1].condition).not.toContain('60%');
    expect(facts[1].actual).toContain('0.7倍');
    expect(facts[1].passed).toBe(false);
  });
  it('keeps nested OR alternatives as one condition, rather than claiming all must pass', () => {
    const conditions = entryConditions(method({ type: 'and', children: [{ type: 'or', children: [compare('bias5', 0), compare('volatility20', .02)] }, compare('volratio5', .6)] }));
    expect(conditions).toHaveLength(2);
    expect(conditions[0]).toContain(' 或 ');
    expect(conditions[0]).toContain('2%');
  });
  it('does not guess indicators from numbers or from another method version', () => {
    const wrongVersion = { ...method(compare('bias5', 0)), method_version_id: 'v2' };
    const facts = explainTriggerFacts(trigger, wrongVersion);
    expect(facts.every((fact) => !fact.mapped)).toBe(true);
    expect(facts[0].condition).toContain('未保留');
    expect(facts[0].condition).not.toContain('乖离率');
  });
  it('reconstructs the exact 0.6372 aggregation for old runs without inventing score inputs', () => {
    const candidate = { score: .6372, triggers: [.59, .59, .59].map((score) => ({ ...trigger, score })) } as SelectionCandidate;
    expect(candidate.score_explanation).toBeUndefined();
    const result = aggregateCalculation(candidate);
    expect(result.value).toBe(.6372);
    expect(result.formula).toContain('0.0236');
    expect(result.formula).toContain('0.6372');
    expect(result.formula).not.toContain('健康');
    const known = retainedEvidenceExplanation({ ...trigger, evidence: { confidence:'moderate',oos_trades:100,oos_max_drawdown:-.1 } as NonNullable<SelectionTrigger['evidence']> });
    expect(known).toContain('0.5500');
    expect(known).toContain('加0.0400');
    expect(known).toContain('不代表成交额和健康调整为零');
  });
  it('explains scope from stored limits using turnover percentage units', () => {
    const card = { ...method(compare('bias5', 0)), scope: { avg_turnover_min: 2, market_cap_max: 100 } };
    expect(methodScopeText(card)).toContain('近120日平均换手率至少 2%');
    expect(methodScopeText(card)).toContain('100 亿');
  });
});
