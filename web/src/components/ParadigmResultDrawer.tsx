import { Typography, Tag, Descriptions, Spin, Empty, Space, Alert, Tooltip } from 'antd';
import { CheckCircleFilled, CloseCircleFilled, QuestionCircleFilled } from '@ant-design/icons';
import MarkdownRenderer from './MarkdownRenderer';
import ResizableDrawer from './ResizableDrawer';
import type { ParadigmItem, EvaluatedItem } from '../types/api';

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
  unknown: '需人工确认',
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

export default function ParadigmResultDrawer({
  open, onClose, loading, paradigm, evaluatedConfirm, evaluatedInvalid, agentText, stockCode, stockName,
}: ParadigmResultDrawerProps) {
  return (
    <ResizableDrawer
      title={`范式挖掘结果 — ${stockName || stockCode}`}
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
          <Alert
            type="warning"
            message="未能提取结构化范式"
            description="Agent 返回了分析文本，但未生成标准范式。以下是原始分析："
            style={{ marginBottom: 16 }}
          />
          <div style={{ background: '#1a1a1a', padding: 16, borderRadius: 8, maxHeight: 500, overflow: 'auto' }}>
            <MarkdownRenderer content={agentText} />
          </div>
        </div>
      )}

      {!loading && !paradigm && !agentText && (
        <Empty description="暂无结果" />
      )}

      {!loading && paradigm && (
        <Space direction="vertical" size={16} style={{ display: 'flex' }}>
          {/* Basic info */}
          <Descriptions bordered column={1} size="small">
            <Descriptions.Item label="范式名称">{paradigm.name}</Descriptions.Item>
            <Descriptions.Item label="范式 ID">
              <Typography.Text code>{paradigm.id}</Typography.Text>
            </Descriptions.Item>
            <Descriptions.Item label="股票">
              {paradigm.stock_name}（{paradigm.stock_code}）
            </Descriptions.Item>
          </Descriptions>

          {/* Context tags */}
          <div>
            <Typography.Text strong style={{ marginRight: 8 }}>适用上下文：</Typography.Text>
            <Space wrap>
              <Tag color={marketCapColors[paradigm.context.market_cap] || 'default'}>
                市值: {paradigm.context.market_cap}
              </Tag>
              <Tag color={shareholderColors[paradigm.context.shareholder_dominant] || 'default'}>
                股东: {paradigm.context.shareholder_dominant}
              </Tag>
              {paradigm.context.activity && (
                <Tag>活跃度: {paradigm.context.activity}</Tag>
              )}
              {paradigm.context.trend && (
                <Tag>趋势: {paradigm.context.trend}</Tag>
              )}
            </Space>
          </div>

          {/* Buy conditions */}
          {paradigm.buy_conditions?.length > 0 && (
            <div>
              <Typography.Text strong>买入条件：</Typography.Text>
              <ul style={{ margin: '4px 0', paddingLeft: 20 }}>
                {paradigm.buy_conditions.map((c, i) => (
                  <li key={i}><Typography.Text>{formatCondition(c)}</Typography.Text></li>
                ))}
              </ul>
            </div>
          )}

          {/* Sell conditions */}
          {(paradigm.sell_conditions?.take_profit?.length || paradigm.sell_conditions?.stop_loss?.length) && (
            <div>
              <Typography.Text strong>止盈止损：</Typography.Text>
              <div style={{ marginTop: 4 }}>
                {paradigm.sell_conditions.take_profit?.map((c, i) => (
                  <div key={`tp-${i}`} style={{ marginLeft: 12, color: '#22c55e' }}>↑ {formatCondition(c)}</div>
                ))}
                {paradigm.sell_conditions.stop_loss?.map((c, i) => (
                  <div key={`sl-${i}`} style={{ marginLeft: 12, color: '#ef4444' }}>↓ {formatCondition(c)}</div>
                ))}
              </div>
            </div>
          )}

          {/* Confirmations */}
          {evaluatedConfirm && evaluatedConfirm.length > 0 && (
            <div>
              <Typography.Text strong>确认项：</Typography.Text>
              <EvaluatedList items={evaluatedConfirm} type="confirm" />
            </div>
          )}

          {/* Invalidations */}
          {evaluatedInvalid && evaluatedInvalid.length > 0 && (
            <div>
              <Typography.Text strong>失效规则：</Typography.Text>
              <EvaluatedList items={evaluatedInvalid} type="invalid" />
            </div>
          )}

          {/* Expectation */}
          {(paradigm.expectation.holding_period || paradigm.expectation.expected_return || paradigm.expectation.risk_reward_ratio || paradigm.expectation.confidence > 0) && (
          <Descriptions bordered column={2} size="small">
            {paradigm.expectation.holding_period && <Descriptions.Item label="持仓周期">{paradigm.expectation.holding_period}</Descriptions.Item>}
            {paradigm.expectation.expected_return && <Descriptions.Item label="预期收益">{paradigm.expectation.expected_return}</Descriptions.Item>}
            {paradigm.expectation.risk_reward_ratio && <Descriptions.Item label="盈亏比">{paradigm.expectation.risk_reward_ratio}</Descriptions.Item>}
            {paradigm.expectation.confidence > 0 && <Descriptions.Item label="置信度">{(paradigm.expectation.confidence * 100).toFixed(0)}%</Descriptions.Item>}
            {paradigm.expectation.win_rate != null && paradigm.expectation.win_rate > 0 && (
              <Descriptions.Item label="胜率">{(paradigm.expectation.win_rate * 100).toFixed(0)}%</Descriptions.Item>
            )}
            {paradigm.expectation.sample_size != null && paradigm.expectation.sample_size > 0 && (
              <Descriptions.Item label="样本量">{paradigm.expectation.sample_size}</Descriptions.Item>
            )}
          </Descriptions>
          )}

          {/* Rationale */}
          {paradigm.rationale && (
            <Alert type="info" message="范式逻辑" description={paradigm.rationale} />
          )}

          {/* Raw agent text (collapsible) */}
          <details>
            <summary style={{ cursor: 'pointer', color: '#888', fontSize: 12 }}>
              查看原始分析文本
            </summary>
            <div style={{ background: '#1a1a1a', padding: 16, borderRadius: 8, maxHeight: 400, overflow: 'auto', marginTop: 8 }}>
              <MarkdownRenderer content={agentText} />
            </div>
          </details>
        </Space>
      )}
    </ResizableDrawer>
  );
}
