import { useEffect, useState } from 'react';
import { Alert, Button, Card, Collapse, Drawer, List, Space, Tag, Typography } from 'antd';
import { api, type MethodCard, type SelectionCandidate, type SelectionTrigger } from '../../api/client';
import { actionExplanation, aggregateCalculation, aggregateSummary, retainedEvidenceExplanation, indicatorHelp, entryConditions, explainTriggerFacts, methodPurpose, methodScopeText } from '../../lib/candidateExplanation';
import { exitRuleSummary } from '../../lib/methodPresentation';
const { Text } = Typography;
const actions: Record<string, string> = { buy: '买入候选', watch: '观察', avoid: '回避', insufficient_data: '数据不足' };

export default function CandidateExplanation({ candidates, stockNames, onStock }: {
  candidates: SelectionCandidate[]; stockNames: Record<string, string>; onStock: (code: string) => void;
}) {
  const [methods, setMethods] = useState<Record<string, MethodCard>>({});
  const [methodError, setMethodError] = useState('');
  const [selected, setSelected] = useState<{ candidate: SelectionCandidate; trigger: SelectionTrigger }>();
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
  const evidence = selected?.trigger.evidence;
  const isFactor = selected?.trigger.method_id === 'factor-combo';
  const sameVersion = !!method?.method_version_id && method.method_version_id === selected?.trigger.method_version_id;

  return <>
    <Alert type="info" showIcon title="内置方法是系统预置的筛选规则" description="命中只说明符合条件，是否买入另看评分、退出计划与名额。点击方法查看规则和本股数值。" />
    {methodError && <Alert type="warning" showIcon title={methodError} />}
    <List dataSource={candidates} renderItem={(candidate) => {
      const seen = new Set<string>();
      const summary = candidate.triggers.flatMap((trigger) => explainTriggerFacts(trigger, methods[trigger.method_id])).filter((fact) => {
        const key = fact.indicatorKey || fact.condition + fact.actual;
        if (!fact.mapped || seen.has(key)) return false;
        seen.add(key); return true;
      }).slice(0, 3);
      return <List.Item actions={[<Button key="stock" onClick={() => onStock(candidate.code)}>查看个股</Button>]}>
        <List.Item.Meta title={<Space wrap>
          <Text strong>{stockNames[candidate.code] || candidate.code}</Text><Text code>{candidate.code}</Text>
          <Tag color={candidate.action === 'buy' ? 'red' : 'gold'}>{actions[candidate.action] || candidate.action}</Tag>
          <Tag>评分 {candidate.score.toFixed(4)} · 非胜率</Tag>
        </Space>} description={<Space direction="vertical" size={6} style={{ display: 'flex' }}>
          <Text strong>为什么入选：触发 {candidate.triggers.length} 个方法</Text>
          <Space wrap size={4}>{candidate.triggers.map((trigger) => <Button key={trigger.method_id} size="small" type="link" onClick={() => setSelected({ candidate, trigger })}>{trigger.method_name} · 方法说明</Button>)}</Space>
          {summary.length ? summary.map((fact) => <Text key={fact.path + fact.condition} type="secondary">{fact.passed ? '✓' : '未通过'} {fact.condition}；当前 {fact.actual}</Text>) : <Text type="secondary">未保留匹配版本的指标信息，请打开方法说明查看记录。</Text>}
          <Text>{actionExplanation(candidate)}</Text>
        </Space>} />
      </List.Item>;
    }} />
    <Drawer open={!!selected} onClose={() => setSelected(undefined)} title={selected ? `${selected.trigger.method_name} · ${stockNames[selected.candidate.code] || selected.candidate.code}` : '方法说明'} size={drawerWidth}>
      {selected && <Space direction="vertical" size={16} style={{ display: 'flex' }}>
        <Space wrap size={4} aria-label="切换本股触发方法">
          {selected.candidate.triggers.map((trigger) => <Button key={trigger.method_id} size="small" type={trigger.method_id === selected.trigger.method_id ? 'primary' : 'default'} onClick={() => setSelected({ candidate: selected.candidate, trigger })}>{trigger.method_name.replace(/^\[内置\]\s*/, '')}</Button>)}
        </Space>
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
        <Card size="small" title="评分怎么计算 · 不是胜率">
          {isFactor ? <Text>来自截面因子排序，仅观察，无普通方法聚合买入门槛；因子分不是胜率。</Text> : <Space direction="vertical" size={8} style={{ display: 'flex' }}>
            <Text>方法评分 {selected.trigger.score.toFixed(4)}：历史证据及数据质量的确定性排序分，条件是否命中由上方事实单独判断。</Text>
            <Text>评分规则：普通证据基础0.55，较强证据(strong)基础0.72；≥100笔样本外交易加0.04；最大回撤低于−20%扣0.08；成交额缺失或非正扣0.08；存在健康值时按75%调整后基础分 + 25%健康值混合。</Text>
            {score ? <>
              <Text>证据基础分 {score.evidence_base.toFixed(4)}（普通 0.55，strong 为 0.72）；样本≥100加分 +{score.sample_bonus.toFixed(4)}；回撤超过20%扣分 −{score.drawdown_penalty.toFixed(4)}；成交额缺失或非正扣分 −{score.missing_amount_penalty.toFixed(4)}。</Text>
              <Text>健康混合前：{score.before_health.toFixed(4)}。{score.health_score !== undefined ? `0.75 × ${score.before_health.toFixed(4)} + 0.25 × ${score.health_score.toFixed(4)} = ${score.score.toFixed(4)}` : `未应用健康混合，结果 ${score.score.toFixed(4)}`}（限制在0–1，四位小数）。</Text>
            </> : <Text type="secondary">旧记录仅保留方法最终评分，无法还原每项加减；成交额和健康明细未记录，不能认定没有调整。使用当时保存的触发方法评分复核聚合。</Text>}
            {!score && <Text>{retainedEvidenceExplanation(selected.trigger)}</Text>}
            <Text strong>{aggregateSummary(selected.candidate)}</Text>
            <Text type="secondary">完整聚合公式：{aggregateCalculation(selected.candidate).formula}</Text>
            <Text>运行记录原始总评分：{selected.candidate.score.toFixed(4)}。额外触发方法逐项加 min(0.03, 方法评分×0.04)，总分上限0.9。</Text>
            <Text>买入门槛为0.65，同时需要完整退出计划及方法内名额（每方法最多 {selected.candidate.score_explanation?.buy_cap_per_method || 5} 只）；分数不足则观察，达到门槛但退出不完整或名额已满也观察。</Text>
            <Text strong>{actionExplanation(selected.candidate)}</Text>
          </Space>}
        </Card>
        <Card size="small" title="退出和风险">
          <p><Text strong>当前查看方法的退出规则</Text></p>
          <Text>{method ? exitRuleSummary(method) : '未保留完整退出规则。'}</Text>
          <p><Text>本次候选采用「{selected.candidate.triggers[0]?.method_name || '主方法'}」的退出计划：{selected.candidate.exit.complete ? '完整' : '不完整'}；最长持有 {selected.candidate.exit.max_holding_days ?? '未记录'} 个交易日；仓位上限 {(selected.candidate.position_cap_pct * 100).toFixed(0)}%。</Text></p>
          <Text type="warning">{selected.candidate.risks?.join('；') || '历史表现不保证未来收益。'}</Text>
        </Card>
        {evidence && <Card size="small" title="触发时的历史验证"><Text>置信度 {evidence.confidence}；样本外 {evidence.oos_trades} 笔；收益 {(evidence.oos_return * 100).toFixed(2)}%；最大回撤 {(evidence.oos_max_drawdown * 100).toFixed(2)}%。</Text></Card>}
        <Collapse items={[{ key: 'audit', label: '原始规则与事实（审计）', children: <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{JSON.stringify({ trigger: selected.trigger, current_method_version_id: method?.method_version_id, current_rules: method?.rules, score_explanation: selected.candidate.score_explanation }, null, 2)}</pre> }]} />
      </Space>}
    </Drawer>
  </>;
}
