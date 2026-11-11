import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Collapse, Drawer, List, Space, Tag, Tooltip, Typography } from 'antd';
import { Link } from 'react-router-dom';
import { api, type MethodCard, type SelectionCandidate, type SelectionTrigger } from '../../api/client';
import { historicalPerformance, referenceExitPrices, performancePercent, actionExplanation, aggregateCalculation, aggregateSummary, scoreContributions, tiedScoreGroups, retainedEvidenceExplanation, indicatorHelp, entryConditions, explainTriggerFacts, methodPurpose, methodScopeText } from '../../lib/candidateExplanation';
import ObservationCreateButton from '../ObservationCreateButton';
import { exitRuleSummary } from '../../lib/methodPresentation';
const { Text } = Typography;
const priceBasis = (basis: string) => ({ forward: '前复权', backward: '后复权', none: '不复权', raw: '不复权' }[basis] || `复权口径：${basis || '未记录'}`);
const actions: Record<string, string> = { buy: '买入候选', watch: '观察', avoid: '回避', insufficient_data: '数据不足' };

export default function CandidateExplanation({ candidates, stockNames, onStock, selectionRunID }: {
  selectionRunID?: string; candidates: SelectionCandidate[]; stockNames: Record<string, string>; onStock: (code: string) => void;
}) {
  const [methods, setMethods] = useState<Record<string, MethodCard>>({});
  const [methodError, setMethodError] = useState('');
  const [selected, setSelected] = useState<{ candidate: SelectionCandidate; trigger: SelectionTrigger }>();
  const scoreRef = useRef<HTMLDivElement>(null);
  const [jumpToScore, setJumpToScore] = useState(false);
  const [drawerWidth, setDrawerWidth] = useState(() => typeof window === 'undefined' ? 640 : Math.min(640, window.innerWidth));
  useEffect(() => {
    const resize = () => setDrawerWidth(Math.min(640, window.innerWidth));
    window.addEventListener('resize', resize);
    return () => window.removeEventListener('resize', resize);
  }, []);
  useEffect(() => {
    let active = true;
    // One batch for the entire list; never one request per row or drawer click.
    void api.methodCards({ limit: 100 }).then((result) => {
      if (active) setMethods(Object.fromEntries(result.items.map((method) => [method.id, method])));
    }).catch(() => { if (active) setMethodError('方法资料加载失败，当前只能查看运行时保留的评分和审计事实。'); });
    return () => { active = false; };
  }, []);
  const method = selected && methods[selected.trigger.method_id];
  const facts = selected ? explainTriggerFacts(selected.trigger, method) : [];
  const score = selected?.candidate.score_explanation?.methods.find((item) => item.method_id === selected.trigger.method_id);
  const performance = historicalPerformance(selected?.trigger, method);
  const exitPrices = selected ? referenceExitPrices(selected.candidate) : undefined;
  const isFactor = selected?.trigger.method_id === 'factor-combo';
  const sameVersion = !!method?.method_version_id && method.method_version_id === selected?.trigger.method_version_id;

  return <>
    <Alert type="info" showIcon title="内置方法是系统预置的筛选规则" description="命中只说明符合条件，是否买入另看评分、退出计划与名额。点击方法查看规则和本股数值。历史胜率按盈利交易统计，不是达到止盈价的概率。" />
    {methodError && <Alert type="warning" showIcon title={methodError} />}
    <List dataSource={candidates} renderItem={(candidate) => {
      const primary = candidate.triggers[0];
      const historical = historicalPerformance(primary, primary && methods[primary.method_id]);
      const prices = referenceExitPrices(candidate);
      const seen = new Set<string>();
      const summary = candidate.triggers.flatMap((trigger) => explainTriggerFacts(trigger, methods[trigger.method_id])).filter((fact) => {
        const key = fact.indicatorKey || fact.condition + fact.actual;
        if (!fact.mapped || seen.has(key)) return false;
        seen.add(key); return true;
      }).slice(0, 3);
      return <List.Item>
        <List.Item.Meta title={<Space wrap>
          <Link to={`/stock/${candidate.code}`} onClick={(event) => {
            if (event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) {
              event.preventDefault(); onStock(candidate.code);
            }
          }}>
            <Space size={6}>
              {stockNames[candidate.code] && stockNames[candidate.code] !== candidate.code && <Text strong>{stockNames[candidate.code]}</Text>}
              <Text code>{candidate.code}</Text>
            </Space>
          </Link>
          <Tag color={candidate.action === 'buy' ? 'red' : 'gold'}>{actions[candidate.action] || candidate.action}</Tag>

        </Space>} description={<Space direction="vertical" size={6} style={{ display: 'flex' }}>
          <Space wrap size={[12, 4]} aria-label="历史表现与退出参考">
            <Tooltip trigger={['hover', 'focus']} title={`主方法：${primary?.method_name || '未记录'}；${historical.source}，不是本股未来胜率。`}><Text strong tabIndex={0}>历史胜率 {performancePercent(historical.winRate)}{historical.trades != null && `（${historical.trades.toLocaleString('en-US')}笔样本）`}</Text></Tooltip>
            <Tooltip trigger={['hover', 'focus']} title="回测中各标的/窗口最大回撤的均值，按幅度显示；不是组合最大回撤，也不是止损比例。"><Text tabIndex={0}>历史回撤（均值）{performancePercent(historical.drawdown)}</Text></Tooltip>
            <Tooltip trigger={['hover', 'focus']} title="当前执行按自然日差计数；不是承诺持有期限，规则退出可能提前触发。"><Text tabIndex={0}>最长持有 {candidate.exit.max_holding_days ? `${candidate.exit.max_holding_days} 天` : '未记录'}</Text></Tooltip>
            <Text>参考止盈价（非预测）{prices.takeProfit == null ? candidate.exit.take_profit_pct != null ? `未换算（阈值 +${performancePercent(candidate.exit.take_profit_pct)}）` : '无固定阈值' : `${prices.takeProfit.toFixed(2)} 元`}</Text>
            <Text>参考止损价 {prices.stopLoss == null ? candidate.exit.stop_loss_pct != null ? `未换算（阈值 ${performancePercent(candidate.exit.stop_loss_pct)}）` : '未记录' : `${prices.stopLoss.toFixed(2)} 元`}</Text>
          </Space>
          <Text type="secondary">历史表现及退出计划采用「{primary?.method_name || '主方法未记录'}」；样本期 {historical.start || '未记录'} ~ {historical.end || '未记录'}。{historical.source}。</Text>
          {prices.reference && <Text type="warning">价格按 {prices.reference.date} 冻结快照收盘价 {prices.reference.close.toFixed(4)} 元换算（{priceBasis(prices.reference.price_adjustment)}），不是成交价或预测；实际止盈止损应以买入成交价计算。</Text>}
          {selectionRunID && <ObservationCreateButton runID={selectionRunID} candidate={candidate} methodCards={methods} />}
          <Text strong>为什么入选：触发 {candidate.triggers.length} 个方法</Text>
          <Space wrap size={4}>{candidate.triggers.map((trigger) => <Button key={trigger.method_id} size="small" type="link" onClick={() => { setJumpToScore(false); setSelected({ candidate, trigger }); }}>{trigger.method_name}</Button>)}</Space>
          {summary.length ? summary.map((fact) => <Text key={fact.path + fact.condition} type="secondary">{fact.passed ? '✓' : '未通过'} {fact.condition}；当前 {fact.actual}</Text>) : <Text type="secondary">未保留匹配版本的指标信息，请打开方法说明查看记录。</Text>}
          <Button type="link" size="small" aria-label={`查看${stockNames[candidate.code] || candidate.code}的评分计算`} disabled={!candidate.triggers.length} onClick={() => {
            setJumpToScore(true); setSelected({ candidate, trigger: candidate.triggers[0] });
          }}>评分 {candidate.score.toFixed(4)} · 非胜率</Button>
          <Text type="secondary">合分：{aggregateSummary(candidate)}</Text>
          <Text>{actionExplanation(candidate)}</Text>
        </Space>} />
      </List.Item>;
    }} />
    <Drawer afterOpenChange={(open) => {
      if (open && jumpToScore) { scoreRef.current?.scrollIntoView({ block: 'start' }); scoreRef.current?.focus({ preventScroll: true }); }
    }} open={!!selected} onClose={() => setSelected(undefined)} title={selected ? `${selected.trigger.method_name} · ${stockNames[selected.candidate.code] || selected.candidate.code}` : '方法说明'} size={drawerWidth}>
      {selected && <Space direction="vertical" size={16} style={{ display: 'flex' }}>
        <Space wrap size={4} aria-label="切换本股触发方法">
          {selected.candidate.triggers.map((trigger) => <Button key={trigger.method_id} size="small" type={trigger.method_id === selected.trigger.method_id ? 'primary' : 'default'} onClick={() => setSelected({ candidate: selected.candidate, trigger })}>{trigger.method_name.replace(/^\[内置\]\s*/, '')}</Button>)}
        </Space>
        <Card size="small" title="该方法的历史表现（不是本股未来概率）">
          <Space direction="vertical" size={6} style={{ display: 'flex' }}>
            <Text strong>历史胜率 {performancePercent(performance.winRate)} · 样本外 {performance.trades ?? '未记录'} 笔</Text>
            <Tooltip title="各标的/样本外窗口的最大回撤均值；不是整个组合最大回撤，也不是候选止损幅度。"><Text>历史回撤（均值）{performancePercent(performance.drawdown)}</Text></Tooltip>
            <Text>样本期：{performance.start || '未记录'} ~ {performance.end || '未记录'}；来源：{performance.source}。</Text>
            <Text type="secondary">胜率按盈利交易数 / 已完成样本外交易数统计，不是达到止盈价的概率，也不表示当前股票上涨的概率。多方法胜率不得平均成“综合胜率”。</Text>
          </Space>
        </Card>
        <Card size="small" title="本股触发方法的各自历史表现">
          <List size="small" dataSource={selected.candidate.triggers} renderItem={(trigger) => {
            const history = historicalPerformance(trigger, methods[trigger.method_id]);
            return <List.Item><Space direction="vertical" size={2}><Text>{trigger.method_name}：胜率 {performancePercent(history.winRate)}（{history.trades?.toLocaleString('en-US') ?? '未记录'}笔样本）；回撤均值 {performancePercent(history.drawdown)}</Text><Text type="secondary">{history.start || '未记录'} ~ {history.end || '未记录'}；{history.source}</Text></Space></List.Item>;
          }} />
        </Card>
        {!sameVersion && <Alert type="warning" showIcon title="以下规则是方法当前版本说明" description="没有匹配到触发时的规则版本，因此不会用当前规则猜测历史事实对应的指标。" />}
        <Card size="small" title="这个方法想抓什么"><Text>{methodPurpose(method)}</Text><p><Text type="secondary">上述策略目的属于策略假设，具体是否命中以结构化规则与事实为准。</Text></p><p><Text type="secondary">适用范围：{methodScopeText(method)}</Text></p><Text type="secondary">{sameVersion ? '入选时已通过股票池门槛；本次触发事实未保留市值、平均换手率等范围判定的实值。' : '上述适用范围为当前版本；旧运行未保存这些范围判定的实值，无法据此核对当前门槛。'}</Text></Card>
        <Card size="small" title="入场条件">
          <Text type="secondary">{method?.rules?.entry_rule?.type === 'and' ? '以下条件必须同时满足；含“或”的条件块满足任一分支即可。' : '按以下条件组合判断，保留“且 / 或”的原始逻辑。'}</Text>
          <List size="small" dataSource={entryConditions(method)} renderItem={(condition) => <List.Item>{condition}</List.Item>} />
          {indicatorHelp(method).map((help) => <p key={help}><Text type="secondary">{help}</Text></p>)}
        </Card>
        <Card size="small" title="为什么这只股票命中">
          <Text>已记录 {facts.filter((fact) => fact.passed && fact.mapped).length} 条通过的条件（共 {facts.filter((fact) => fact.mapped).length} 条可对应条件）；“或”组合允许部分分支未通过。</Text>
          <List size="small" dataSource={facts} renderItem={(fact) => <List.Item><Space direction="vertical" size={2}>
            <span><Tag color={fact.passed ? 'green' : 'default'}>{fact.passed ? '通过' : '未通过'}</Tag>{fact.condition}</span><Text type="secondary">当前：{fact.actual}</Text>
          </Space></List.Item>} />
        </Card>
        <div ref={scoreRef} tabIndex={-1} aria-label="评分计算详情">
        <Card size="small" title="评分怎么计算 · 不是胜率">
          {isFactor ? <Text>来自截面因子排序，仅观察，无普通方法聚合买入门槛；因子分不是胜率。</Text> : <Space direction="vertical" size={8} style={{ display: 'flex' }}>
            <Text>方法评分 {selected.trigger.score.toFixed(4)}：历史证据及数据质量的确定性排序分，条件是否命中由上方事实单独判断。</Text>
            <Text>入场条件是筛选门槛，评分不会按指标超出阈值的幅度加分。不同方法的样本外收益、交易笔数即使不同，只要落在同一证据档位和样本分界，且其他调整相同，就可能同分；同分不表示历史效果相同。</Text>
            {tiedScoreGroups(selected.candidate).map((group) => <Alert key={group.score} type="info" showIcon title={`${group.names.length}个方法同为 ${group.score.toFixed(4)} 分`} description={`本次同分方法：${group.names.join('、')}。这是本次运行保存的最终分；详细加减项以已保存数据为准。`} />)}
            <Text strong>本次所有触发方法的最终评分</Text>
            <List size="small" dataSource={selected.candidate.triggers} renderItem={(trigger) => <List.Item><Space direction="vertical" size={2}><Text>{trigger.method_name}：{trigger.score.toFixed(4)}</Text>{trigger.evidence && <Text type="secondary">触发时证据 {trigger.evidence.confidence}；样本外 {trigger.evidence.oos_trades} 笔；收益 {(trigger.evidence.oos_return * 100).toFixed(2)}%；回撤均值 {performancePercent(Math.abs(trigger.evidence.oos_max_drawdown))}。</Text>}</Space></List.Item>} />
            <Text>评分规则：普通证据基础0.55，较强证据(strong)基础0.72；≥100笔样本外交易加0.04；最大回撤低于−20%扣0.08；成交额缺失或非正扣0.08；存在健康值时按75%调整后基础分 + 25%健康值混合。</Text>
            {score ? <>
              <Text>证据基础分 {score.evidence_base.toFixed(4)}（普通 0.55，strong 为 0.72）；样本≥100加分 +{score.sample_bonus.toFixed(4)}；回撤超过20%扣分 −{score.drawdown_penalty.toFixed(4)}；成交额缺失或非正扣分 −{score.missing_amount_penalty.toFixed(4)}。</Text>
              <Text>健康混合前：{score.before_health.toFixed(4)}。{score.health_score !== undefined ? `0.75 × ${score.before_health.toFixed(4)} + 0.25 × ${score.health_score.toFixed(4)} = ${score.score.toFixed(4)}` : `未应用健康混合，结果 ${score.score.toFixed(4)}`}（限制在0–1，四位小数）。</Text>
            </> : <Text type="secondary">旧记录仅保留方法最终评分，无法还原每项加减；成交额和健康明细未记录，不能认定没有调整。使用当时保存的触发方法评分复核聚合。</Text>}
            {!score && <Text>{retainedEvidenceExplanation(selected.trigger)}</Text>}
            <Text strong>{aggregateSummary(selected.candidate)}</Text>
            <List size="small" header="逐方法合分（按最终评分排序）" dataSource={scoreContributions(selected.candidate)} renderItem={(item) => <List.Item><Space direction="vertical" size={2}>
              <Text>{item.methodName}：方法分 {item.score.toFixed(4)}</Text>
              <Text type="secondary">{item.primary ? `最高分作为基础：${item.contribution.toFixed(4)}` : `附加分 min(0.03, ${item.score.toFixed(4)} × 0.04) = ${item.contribution.toFixed(4)}`}</Text>
            </Space></List.Item>} />
            <Text type="secondary">完整聚合公式：{aggregateCalculation(selected.candidate).formula}</Text>
            <Text>运行记录原始总评分：{selected.candidate.score.toFixed(4)}。额外触发方法逐项加 min(0.03, 方法评分×0.04)，总分上限0.9。</Text>
            <Text type="secondary">基础策略与其活跃股/小盘变体可能相关；多方法加分表示当前规则聚合，不代表多个统计独立证据，也不是上涨概率。系统会按登记的方法族去重，但不同方法族仍可能相关。</Text>
            <Text>买入门槛为0.65，同时需要完整退出计划及方法内名额（每方法最多 {selected.candidate.score_explanation?.buy_cap_per_method || 5} 只）；分数不足则观察，达到门槛但退出不完整或名额已满也观察。</Text>
            <Text strong>{actionExplanation(selected.candidate)}</Text>
          </Space>}
        </Card>
        </div>
        <Card size="small" title="退出和风险">
          <p><Text strong>当前查看方法的退出规则</Text></p>
          <Text>{method ? exitRuleSummary(method).replace(/个交易日/g, '天').replace(/至少持有 (\d+) 天/g, '最短持有配置 $1 天（当前未强制执行）') : '未保留完整退出规则。'}</Text>
          <p><Text type="secondary">当前持有天数执行按自然日差计数；最短持有仅是规则中保留的配置，不能视为禁止提前退出的保证。止盈、止损和规则退出都可能提前触发。</Text></p>
          <p><Text>本次候选采用「{selected.candidate.triggers[0]?.method_name || '主方法'}」的退出计划：{selected.candidate.exit.complete ? '完整' : '不完整'}；最长持有 {selected.candidate.exit.max_holding_days ?? '未记录'} 天；仓位上限 {(selected.candidate.position_cap_pct * 100).toFixed(0)}%。</Text></p>
          <p><Text strong>参考止盈价（非预测）：{exitPrices?.takeProfit == null ? '无可换算的固定阈值' : `${exitPrices.takeProfit.toFixed(2)} 元`}；参考止损价：{exitPrices?.stopLoss == null ? '未记录' : `${exitPrices.stopLoss.toFixed(2)} 元`}。</Text></p>
          {exitPrices?.reference && <Alert type="warning" showIcon title="这是快照价的阈值换算，不是目标价预测或实际成交价" description={`基准为 ${exitPrices.reference.date} 冻结快照收盘 ${exitPrices.reference.close.toFixed(4)} 元（${priceBasis(exitPrices.reference.price_adjustment)}）；采用本次主方法退出计划：止盈 ${performancePercent(selected.candidate.exit.take_profit_pct)}、止损 ${performancePercent(selected.candidate.exit.stop_loss_pct)}。实际阈值应按实际买入价计算；均线/乖离等动态退出规则不对应固定卖出价。`} />}
          <Text type="warning">{selected.candidate.risks?.join('；') || '历史表现不保证未来收益。'}</Text>
        </Card>

        <Collapse items={[{ key: 'audit', label: '原始规则与事实（审计）', children: <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{JSON.stringify({ trigger: selected.trigger, current_method_version_id: method?.method_version_id, current_rules: method?.rules, score_explanation: selected.candidate.score_explanation }, null, 2)}</pre> }]} />
      </Space>}
    </Drawer>
  </>;
}
