import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import CandidateExplanation from '../components/dashboard/CandidateExplanation';
import { describe, expect, it } from 'vitest';
import type { MethodCard, MethodRuleExpr, SelectionCandidate, SelectionTrigger } from '../api/client';
import { historicalPerformance, referenceExitPrices, performancePercent, methodPurpose, aggregateCalculation, aggregateSummary, scoreContributions, tiedScoreGroups, retainedEvidenceExplanation, entryConditions, explainTriggerFacts, methodScopeText } from './candidateExplanation';
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
  it('explains equal method scores without equating different OOS results', () => {
    const candidate = { score: .6372, triggers: [
      { ...trigger, method_id: 'base', method_name: '基础策略', evidence: { confidence: 'moderate', oos_trades: 101, oos_return: .1 } },
      { ...trigger, method_id: 'active', method_name: '活跃股变体', evidence: { confidence: 'moderate', oos_trades: 800, oos_return: .3 } },
      { ...trigger, method_id: 'small', method_name: '小盘变体' },
    ] } as SelectionCandidate;
    expect(tiedScoreGroups(candidate)).toEqual([{ score: .59, names: ['基础策略', '活跃股变体', '小盘变体'] }]);
    const rows = scoreContributions(candidate);
    expect(rows).toHaveLength(3);
    expect(rows.map((row) => row.score)).toEqual([.59, .59, .59]);
    expect(rows[0].primary).toBe(true);
    expect(rows[0].contribution).toBe(.59);
    expect(rows[1].contribution).toBeCloseTo(.0236);
    expect(rows[2].contribution).toBeCloseTo(.0236);
    expect(aggregateSummary(candidate)).toContain('2份附加分');
    expect(candidate.score_explanation).toBeUndefined();
  });
  it('keeps factor-only scores out of normal method aggregation', () => {
    const candidate = { score: 8, triggers: [{ ...trigger, method_id: 'factor-combo', score: 8 }] } as SelectionCandidate;
    expect(scoreContributions(candidate)).toEqual([]);
    expect(aggregateSummary(candidate)).toContain('不使用普通方法聚合公式');
    expect(aggregateCalculation(candidate).value).toBe(8);
  });

  it('uses one router-aware stock link, drops extra buttons and keeps a visible aggregate summary', () => {
    const candidate = { code: '600000', action: 'watch', score: .59, triggers: [trigger], exit: { complete: true } } as SelectionCandidate;
    const html = renderToStaticMarkup(createElement(MemoryRouter, { basename: '/app', initialEntries: ['/app/'] },
      createElement(CandidateExplanation, { candidates: [candidate], stockNames: { '600000': '浦发银行' }, onStock: () => undefined })));
    expect(html).toContain('href="/app/stock/600000"');
    expect(html).not.toContain('查看个股');
    expect(html).not.toContain('规则 · 方法说明');
    expect(html).toContain('合分：');
    const stockLink = html.match(/<a[^>]*href="\/app\/stock\/600000"[^>]*>([\s\S]*?)<\/a>/)?.[1] || '';
    expect(stockLink).toContain('浦发银行');
    expect(stockLink.match(/600000/g)).toHaveLength(1);
    const fallback = renderToStaticMarkup(createElement(MemoryRouter, null,
      createElement(CandidateExplanation, { candidates: [candidate], stockNames: {}, onStock: () => undefined })));
    const fallbackLink = fallback.match(/<a[^>]*href="\/stock\/600000"[^>]*>([\s\S]*?)<\/a>/)?.[1] || '';
    expect(fallbackLink.match(/600000/g)).toHaveLength(1);
  });

  it('uses the three actual retained method rates individually even though their scores match', () => {
    const cases = [[.6228716868527295,11394,'62.29%'],[.3484409412310189,8627,'34.84%'],[.6889714993804213,807,'68.90%']] as const;
    for (const [rate,trades,label] of cases) {
      const recorded={...trigger,evidence:{confidence:'moderate',oos_trades:trades,oos_win_rate:rate,oos_max_drawdown:.0034180195201189653,validation_start:'2024-01-30',validation_end:'2024-11-07',oos_return:0}};
      const result=historicalPerformance(recorded);
      expect(performancePercent(result.winRate)).toBe(label);
      expect(result.trades).toBe(trades);
      expect(performancePercent(result.drawdown)).toBe('0.34%');
      expect(result.start).toBe('2024-01-30');
    }
  });
  it('preserves a real zero win rate and only fills missing rates from matching version and result hash', () => {
    const recorded={...trigger,evidence:{confidence:'moderate',oos_trades:10,oos_return:0,oos_max_drawdown:-.12,result_hash:'same'}};
    const card={...method(compare('bias5',0)),evidence:{...recorded.evidence,oos_win_rate:.6}};
    expect(historicalPerformance(recorded,card).winRate).toBe(.6);
    expect(historicalPerformance(recorded,card).source).toContain('同版本');
    expect(historicalPerformance({...recorded,evidence:{...recorded.evidence,oos_win_rate:0}},card).winRate).toBe(0);
    expect(historicalPerformance(recorded,{...card,method_version_id:'other'}).winRate).toBeUndefined();
    expect(historicalPerformance(recorded,{...card,evidence:{...card.evidence,result_hash:'other'}}).winRate).toBeUndefined();
    expect(historicalPerformance({...recorded,evidence:{...recorded.evidence,oos_trades:0,oos_win_rate:0}}).winRate).toBeUndefined();
    expect(historicalPerformance(recorded).drawdown).toBe(.12);
  });
  it('derives reference exits only from explicit frozen close plus the candidate primary exit plan', () => {
    const candidate={code:'600000',exit:{complete:true,max_holding_days:25,stop_loss_pct:-.08,take_profit_pct:.2},reference_price:{close:10,date:'2026-11-10',price_adjustment:'forward',feature_snapshot_id:'frozen'}} as SelectionCandidate;
    const prices=referenceExitPrices(candidate);
    expect(prices.takeProfit).toBe(12);
    expect(prices.stopLoss).toBeCloseTo(9.2);
    expect(prices.reference?.date).toBe('2026-11-10');
    expect(referenceExitPrices({...candidate,reference_price:undefined}).takeProfit).toBeUndefined();
    expect(referenceExitPrices({...candidate,reference_price:{...candidate.reference_price!,close:0}}).stopLoss).toBeUndefined();
    expect(referenceExitPrices({...candidate,exit:{complete:false}}).takeProfit).toBeUndefined();
    expect(referenceExitPrices({...candidate,exit:{...candidate.exit,stop_loss_pct:-2}}).stopLoss).toBeUndefined();
  });

  it('puts the primary method history and frozen price exits on the default row without averaging methods', () => {
    const primary={...trigger,method_name:'主方法',evidence:{confidence:'moderate',oos_win_rate:.6228716868527295,oos_trades:11394,oos_return:0,oos_max_drawdown:.0034,validation_start:'2024-01-30',validation_end:'2024-11-07'}};
    const other={...trigger,method_id:'other',method_name:'其他方法',evidence:{...primary.evidence,oos_win_rate:.6889714993804213,oos_trades:807}};
    const candidate={code:'600000',action:'watch',score:.6136,data_date:'2026-11-10',triggers:[primary,other],exit:{complete:true,max_holding_days:25,stop_loss_pct:-.08,take_profit_pct:.2},reference_price:{close:10,date:'2026-11-10',price_adjustment:'forward',feature_snapshot_id:'frozen'}} as SelectionCandidate;
    const html=renderToStaticMarkup(createElement(MemoryRouter,null,createElement(CandidateExplanation,{candidates:[candidate],stockNames:{},onStock:()=>undefined})));
    expect(html).toContain('62.29%');
    expect(html).toContain('11,394笔样本');
    expect(html).not.toContain('68.90%');
    expect(html).toContain('2024-01-30');
    expect(html).toContain('2024-11-07');
    expect(html).toContain('12.00');
    expect(html).toContain('9.20');
    expect(html).toContain('25 天');
    expect(html).toContain('不是成交价或预测');
    expect(html).toContain('tabindex="0"');
  });

  it('keeps strategy and scope prose but does not repeat obsolete descriptive grid win rates', () => {
    const card={...method(compare('bias5',0)),description:'趋势回踩；市值10~40亿（历史分格胜率69.7%）；跌破20日线离场。'};
    expect(methodPurpose(card)).toContain('市值10~40亿');
    expect(methodPurpose(card)).toContain('跌破20日线离场');
    expect(methodPurpose(card)).not.toContain('69.7%');
  });

});
