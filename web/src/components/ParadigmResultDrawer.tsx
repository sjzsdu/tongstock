import { useEffect, useState } from 'react';
import { Typography, Tag, Descriptions, Spin, Empty, Space, Alert, Tooltip, Tabs, Table, Button, Input, InputNumber, Select, message } from 'antd';
import { CheckCircleFilled, CloseCircleFilled, QuestionCircleFilled, QuestionCircleOutlined } from '@ant-design/icons';
import MarkdownRenderer from './MarkdownRenderer';
import ResizableDrawer from './ResizableDrawer';
import { api } from '../api/client';
import type { ParadigmItem, EvaluatedItem, EvaluatedCondition } from '../types/api';

interface ParadigmResultDrawerProps {
  open: boolean;
  onClose: () => void;
  loading: boolean;
  paradigm: ParadigmItem | null;
  evaluatedConfirm?: EvaluatedItem[];
  evaluatedInvalid?: EvaluatedItem[];
  agentText: string;
  stockCode: string;
  stockName?: string;
}

const marketCapColors: Record<string, string> = {
  small: 'orange',
  mid: 'blue',
  large: 'green',
  mega: 'purple',
};

const shareholderColors: Record<string, string> = {
  retail: 'gold',
  hot_money: 'red',
  foreign: 'blue',
  institutional: 'cyan',
  state: 'green',
  mixed: 'default',
};

function formatCondition(c: { indicator: string; operator: string; value: string }) {
  if (c.operator === 'describe' || !c.value) return c.indicator;
  const opMap: Record<string, string> = {
    cross_above: '上穿',
    cross_below: '下穿',
    gt: '>',
    lt: '<',
    between: '在...之间',
    near: '接近',
  };
  return `${c.indicator} ${opMap[c.operator] || c.operator} ${c.value}`;
}

const statusIcon: Record<string, React.ReactNode> = {
  met: <CheckCircleFilled style={{ color: '#22c55e' }} />,
  not_met: <CloseCircleFilled style={{ color: '#ef4444' }} />,
  unknown: <QuestionCircleFilled style={{ color: '#888' }} />,
};

const statusLabel: Record<string, string> = {
  met: '已满足',
  not_met: '未满足',
  unknown: '待确认',
};

function EvaluatedList({ items, type }: { items: EvaluatedItem[]; type: 'confirm' | 'invalid' }) {
  if (!items || items.length === 0) return null;
  return (
    <ul style={{ margin: '4px 0', paddingLeft: 0, listStyle: 'none' }}>
      {items.map((item, i) => (
        <li key={i} style={{ display: 'flex', alignItems: 'flex-start', gap: 8, marginBottom: 6 }}>
          <Tooltip title={item.reason || statusLabel[item.status]}>
            <span style={{ marginTop: 2 }}>{statusIcon[item.status]}</span>
          </Tooltip>
          <span style={{
            color: type === 'invalid'
              ? (item.status === 'met' ? '#ef4444' : item.status === 'not_met' ? '#22c55e' : '#888')
              : (item.status === 'met' ? '#22c55e' : '#e0e0e0'),
          }}>
            {item.text}
            {item.reason && (
              <Typography.Text type="secondary" style={{ fontSize: 11, marginLeft: 6 }}>
                ({item.reason})
              </Typography.Text>
            )}
          </span>
        </li>
      ))}
    </ul>
  );
}

function ParadigmInfo({ paradigm }: { paradigm: ParadigmItem }) {
  const reliability = paradigm.validation?.reliability_label || 'unknown';
  const reliabilityColor: Record<string, string> = { high: 'green', medium: 'orange', low: 'red', unknown: 'default' };
  return (
    <Space direction="vertical" size={12} style={{ display: 'flex' }}>
      <Alert
        type="warning"
        showIcon
        message="仅供研究，不构成投资建议"
        description="AI 范式需要结合数据完整性、可自动验证比例和人工复盘结果使用。市场有风险，交易需自担风险。"
      />
      <Descriptions bordered column={1} size="small">
        <Descriptions.Item label="范式名称">{paradigm.name}</Descriptions.Item>
        <Descriptions.Item label="范式 ID">
          <Typography.Text code>{paradigm.id}</Typography.Text>
        </Descriptions.Item>
        <Descriptions.Item label="股票">
          {paradigm.stock_name}（{paradigm.stock_code}）
        </Descriptions.Item>
        <Descriptions.Item label="生成信息">
          <Space wrap>
            {paradigm.source?.agent_version && <Tag>{paradigm.source.agent_version}</Tag>}
            {paradigm.source?.kline_type && <Tag>K线: {paradigm.source.kline_type}</Tag>}
            {paradigm.source?.days && <Tag>窗口: {paradigm.source.days}日</Tag>}
            {paradigm.source?.generated_at && <Tag>生成: {new Date(paradigm.source.generated_at).toLocaleString()}</Tag>}
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label="可靠性">
          <Space wrap>
            <Tag color={reliabilityColor[reliability]}>{reliability}</Tag>
            {paradigm.validation && (
              <Tag>自动可验: {paradigm.validation.auto_evaluable}/{paradigm.validation.total_conditions} ({(paradigm.validation.auto_evaluable_ratio * 100).toFixed(0)}%)</Tag>
            )}
            {paradigm.review_status && <Tag>复盘: {paradigm.review_status}</Tag>}
            {typeof paradigm.actual_return === 'number' && <Tag color={paradigm.actual_return >= 0 ? 'red' : 'green'}>实际收益: {paradigm.actual_return.toFixed(2)}%</Tag>}
          </Space>
        </Descriptions.Item>
      </Descriptions>

      {paradigm.validation?.warnings && paradigm.validation.warnings.length > 0 && (
        <Alert type="info" message="校验提示" description={paradigm.validation.warnings.join('；')} />
      )}

      <div>
        <Typography.Text strong style={{ marginRight: 8 }}>适用上下文：</Typography.Text>
        <Space wrap>
          <Tag color={marketCapColors[paradigm.context.market_cap] || 'default'}>
            市值: {paradigm.context.market_cap}
          </Tag>
          <Tag color={shareholderColors[paradigm.context.shareholder_dominant] || 'default'}>
            股东: {paradigm.context.shareholder_dominant}
          </Tag>
          {paradigm.context.activity && <Tag>活跃度: {paradigm.context.activity}</Tag>}
          {paradigm.context.trend && <Tag>趋势: {paradigm.context.trend}</Tag>}
        </Space>
      </div>

      {(paradigm.expectation.holding_period || paradigm.expectation.expected_return || paradigm.expectation.risk_reward_ratio || paradigm.expectation.confidence > 0) && (
        <Descriptions bordered column={2} size="small">
          {paradigm.expectation.holding_period && <Descriptions.Item label="持仓周期">{paradigm.expectation.holding_period}</Descriptions.Item>}
          {paradigm.expectation.expected_return && <Descriptions.Item label="预期收益">{paradigm.expectation.expected_return}</Descriptions.Item>}
          {paradigm.expectation.risk_reward_ratio && <Descriptions.Item label="盈亏比">{paradigm.expectation.risk_reward_ratio}</Descriptions.Item>}
          {paradigm.expectation.confidence > 0 && <Descriptions.Item label="置信度">{(paradigm.expectation.confidence * 100).toFixed(0)}%</Descriptions.Item>}
        </Descriptions>
      )}

      {paradigm.rationale && (
        <Alert type="info" message="范式逻辑" description={paradigm.rationale} />
      )}
    </Space>
  );
}

const condStatusIcon: Record<string, React.ReactNode> = {
  met: <CheckCircleFilled style={{ color: '#22c55e' }} />,
  not_met: <CloseCircleFilled style={{ color: '#ef4444' }} />,
  unknown: <QuestionCircleFilled style={{ color: '#888' }} />,
};

const condStatusLabel: Record<string, string> = {
  met: '已满足',
  not_met: '未满足',
  unknown: '待确认',
};

const typeLabel: Record<string, string> = {
  buy: '买入条件',
  take_profit: '止盈',
  stop_loss: '止损',
};

const typeColor: Record<string, string> = {
  buy: 'blue',
  take_profit: 'green',
  stop_loss: 'red',
};

function ConditionTable({ conditions }: { conditions: EvaluatedCondition[] }) {
  if (!conditions || conditions.length === 0) return null;
  return (
    <Table
      size="small"
      pagination={false}
      rowKey={(r, i) => `${r.type}-${i}`}
      dataSource={conditions}
      columns={[
        { title: '类型', dataIndex: 'type', width: 90, render: (v: string) => <Tag color={typeColor[v]}>{typeLabel[v] || v}</Tag> },
        { title: '条件', dataIndex: 'condition' },
        { title: '当前值', dataIndex: 'value', render: (v: string) => <Typography.Text code style={{ fontSize: 12 }}>{v || '-'}</Typography.Text> },
        {
          title: '状态', dataIndex: 'status', width: 110,
          render: (v: string) => (
            <Space size={4}>
              {condStatusIcon[v]}
              <span style={{ color: v === 'met' ? '#22c55e' : v === 'not_met' ? '#ef4444' : '#888', fontSize: 12 }}>
                {condStatusLabel[v]}
              </span>
            </Space>
          ),
        },
      ]}
    />
  );
}

function AnalysisTab({ agentText, stockCode, stockName }: { agentText: string; stockCode: string; stockName?: string }) {
  const [conditions, setConditions] = useState<EvaluatedCondition[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!stockCode) return;
    setLoading(true);
    api.paradigmEvaluate(stockCode)
      .then(r => setConditions(r.conditions || []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [stockCode]);

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      <div>
        <Typography.Text strong style={{ display: 'block', marginBottom: 8 }}>
          条件对照 — {stockName || stockCode}
        </Typography.Text>
        {loading ? <Spin size="small" /> : conditions.length > 0 ? (
          <ConditionTable conditions={conditions} />
        ) : (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>暂无结构化条件数据</Typography.Text>
        )}
      </div>
      {agentText && (
        <div>
          <Typography.Text type="secondary" style={{ display: 'block', marginBottom: 8, fontSize: 12 }}>完整分析</Typography.Text>
          <div style={{ background: '#1a1a1a', padding: 16, borderRadius: 8, overflow: 'auto', maxHeight: 500 }}>
            <MarkdownRenderer content={agentText} />
          </div>
        </div>
      )}
      {!agentText && <Empty description="暂无分析文本" />}
    </Space>
  );
}

function ReviewTab({ paradigm }: { paradigm: ParadigmItem }) {
  const [status, setStatus] = useState(paradigm.review_status || 'pending');
  const [note, setNote] = useState(paradigm.review_note || '');
  const [rating, setRating] = useState<number | null>(paradigm.review_rating || null);
  const [actualReturn, setActualReturn] = useState<number | null>(typeof paradigm.actual_return === 'number' ? paradigm.actual_return : null);
  const [saving, setSaving] = useState(false);

  const save = async () => {
    setSaving(true);
    try {
      await api.paradigmReview(paradigm.id, {
        review_status: status,
        review_note: note,
        review_rating: rating || undefined,
        actual_return: actualReturn ?? undefined,
      });
      message.success('复盘已保存');
    } catch (err) {
      message.error(String(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Space direction="vertical" size={12} style={{ display: 'flex' }}>
      <Alert type="info" showIcon message="复盘闭环" description="记录实际收益和主观评分后，可以逐步统计范式质量。" />
      <Select
        value={status}
        onChange={setStatus}
        options={[
          { value: 'pending', label: '待复盘' },
          { value: 'reviewed', label: '已复盘' },
          { value: 'verified', label: '验证有效' },
          { value: 'rejected', label: '验证无效' },
        ]}
        style={{ width: 180 }}
      />
      <Space>
        <span>评分</span>
        <InputNumber min={1} max={5} value={rating} onChange={v => setRating(v)} />
        <span>实际收益%</span>
        <InputNumber value={actualReturn} precision={2} onChange={v => setActualReturn(v)} />
      </Space>
      <Input.TextArea rows={4} value={note} onChange={e => setNote(e.target.value)} placeholder="记录买卖点、执行偏差、有效/失效原因..." />
      <Button type="primary" loading={saving} onClick={save}>保存复盘</Button>
    </Space>
  );
}

function SideContent({ paradigm, evaluatedConfirm, evaluatedInvalid, agentText, stockCode, stockName, side }: {
  paradigm: ParadigmItem | null;
  evaluatedConfirm?: EvaluatedItem[];
  evaluatedInvalid?: EvaluatedItem[];
  agentText: string;
  stockCode: string;
  stockName?: string;
  side: 'buy' | 'sell';
}) {
  return (
    <Tabs
      defaultActiveKey="conditions"
      items={[
        {
          key: 'conditions',
          label: side === 'buy' ? '条件' : '条件',
          children: (
            <Space direction="vertical" size={16} style={{ display: 'flex' }}>
              {paradigm && <ParadigmInfo paradigm={paradigm} />}
              {paradigm && paradigm.buy_conditions?.length > 0 && (
                <div>
                  <Typography.Text strong>{side === 'sell' ? '入场条件（做空）：' : '买入条件：'}</Typography.Text>
                  <ul style={{ margin: '4px 0', paddingLeft: 20 }}>
                    {paradigm.buy_conditions.map((c, i) => (
                      <li key={i}><Typography.Text>{formatCondition(c)}</Typography.Text></li>
                    ))}
                  </ul>
                </div>
              )}
              {side === 'buy' && evaluatedConfirm && evaluatedConfirm.length > 0 && (
                <div>
                  <Typography.Text strong>确认项：</Typography.Text>
                  <EvaluatedList items={evaluatedConfirm} type="confirm" />
                </div>
              )}
              {side === 'buy' && evaluatedInvalid && evaluatedInvalid.length > 0 && (
                <div>
                  <Typography.Text strong>失效规则：</Typography.Text>
                  <EvaluatedList items={evaluatedInvalid} type="invalid" />
                </div>
              )}
              {side === 'sell' && paradigm && paradigm.sell_conditions?.take_profit && paradigm.sell_conditions.take_profit.length > 0 && (
                <div>
                  <Typography.Text strong>平仓条件（止盈）：</Typography.Text>
                  <div style={{ marginTop: 4 }}>
                    {paradigm.sell_conditions.take_profit.map((c, i) => (
                      <div key={i} style={{ marginLeft: 12, color: '#22c55e' }}>↑ {formatCondition(c)}</div>
                    ))}
                  </div>
                </div>
              )}
              {side === 'sell' && paradigm && paradigm.sell_conditions?.stop_loss && paradigm.sell_conditions.stop_loss.length > 0 && (
                <div>
                  <Typography.Text strong>止损条件：</Typography.Text>
                  <div style={{ marginTop: 4 }}>
                    {paradigm.sell_conditions.stop_loss.map((c, i) => (
                      <div key={i} style={{ marginLeft: 12, color: '#ef4444' }}>↓ {formatCondition(c)}</div>
                    ))}
                  </div>
                </div>
              )}
            </Space>
          ),
        },
        {
          key: 'analysis',
          label: '对照',
          children: <AnalysisTab agentText={agentText} stockCode={stockCode} stockName={stockName} />,
        },
        ...(paradigm ? [{
          key: 'review',
          label: '复盘',
          children: <ReviewTab paradigm={paradigm} />,
        }] : []),
      ]}
    />
  );
}

export default function ParadigmResultDrawer({
  open, onClose, loading, paradigm, evaluatedConfirm, evaluatedInvalid, agentText, stockCode, stockName,
}: ParadigmResultDrawerProps) {
  const [allParadigms, setAllParadigms] = useState<ParadigmItem[]>([]);

  useEffect(() => {
    if (!open || !stockCode) return;
    api.paradigmListByStock(stockCode)
      .then(r => setAllParadigms(r.paradigms || []))
      .catch(() => {});
  }, [open, stockCode]);

  const buyParadigms = allParadigms.filter(p => p.side === 'buy');
  const sellParadigms = allParadigms.filter(p => p.side === 'sell');

  const hasBuy = buyParadigms.length > 0 || (paradigm?.side !== 'sell');
  const hasSell = sellParadigms.length > 0;

  return (
    <ResizableDrawer
      title={
        <Space>
          范式挖掘 — {stockName || stockCode}
          <Tooltip
            title={
              <div style={{ maxWidth: 360, fontSize: 12, lineHeight: 1.8 }}>
                <div><b>什么是范式？</b></div>
                <div>从K线、指标、市值和股东结构中提炼的交易模式，告诉你什么条件下买入/卖出可能有效。</div>
                <div style={{ marginTop: 8 }}><b>怎么看结果？</b></div>
                <div>• <b>适用上下文</b>：这个范式在什么类型的股票上有效</div>
                <div>• <b>买入条件</b>：触发买入的指标条件</div>
                <div>• <b>确认项</b>：提高胜率的加分条件</div>
                <div>• <b>失效规则</b>：出现这些情况时范式不成立</div>
                <div>• <b>止盈止损</b>：目标价和止损价</div>
                <div style={{ marginTop: 8 }}><b>条件状态</b></div>
                <div>✓ 已满足（绿色）/ ✗ 未满足（红色）/ ? 需人工确认（灰色）</div>
                <div style={{ marginTop: 8 }}><b>预期收益表</b></div>
                <div>• <b>持仓周期</b>：持有该仓位的平均天数</div>
                <div>• <b>预期收益</b>：满足条件时的历史平均收益范围（非保证）</div>
                <div>• <b>盈亏比</b>：每承担1元风险预期赚多少（&lt;2:1 需高胜率才值得做）</div>
                <div>• <b>置信度</b>：历史回测中按条件操作盈利的概率</div>
              </div>
            }
          >
            <QuestionCircleOutlined style={{ color: '#888', fontSize: 16, cursor: 'help' }} />
          </Tooltip>
        </Space>
      }
      placement="right"
      defaultWidth={640}
      minWidth={360}
      maxWidth={1000}
      open={open}
      onClose={onClose}
    >
      {loading && (
        <div style={{ textAlign: 'center', padding: 40 }}>
          <Spin size="large" tip="正在分析 K 线数据并挖掘范式..." />
        </div>
      )}

      {!loading && !paradigm && agentText && (
        <div>
          <Alert type="warning" message="未能提取结构化范式" style={{ marginBottom: 16 }} />
          <AnalysisTab agentText={agentText} stockCode={stockCode} stockName={stockName} />
        </div>
      )}

      {!loading && !paradigm && !agentText && <Empty description="暂无结果" />}

      {!loading && paradigm && (
        <Tabs
          defaultActiveKey={paradigm.side === 'sell' && !hasBuy ? 'sell' : 'buy'}
          items={[
            ...(hasBuy ? [{
              key: 'buy',
              label: <span style={{ color: '#22c55e' }}>📈 买入</span>,
              children: (
                <SideContent
                  paradigm={buyParadigms[0] || paradigm}
                  evaluatedConfirm={evaluatedConfirm}
                  evaluatedInvalid={evaluatedInvalid}
                  agentText={buyParadigms[0]?.agent_text || agentText}
                  stockCode={stockCode}
                  stockName={stockName}
                  side="buy"
                />
              ),
            }] : []),
            ...(hasSell ? [{
              key: 'sell',
              label: <span style={{ color: '#ef4444' }}>📉 卖出</span>,
              children: (
                <SideContent
                  paradigm={sellParadigms[0] || null}
                  agentText={sellParadigms[0]?.agent_text || ''}
                  stockCode={stockCode}
                  stockName={stockName}
                  side="sell"
                />
              ),
            }] : []),
          ]}
        />
      )}
    </ResizableDrawer>
  );
}
