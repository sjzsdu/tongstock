import {
  ArrowRightOutlined,
  BarChartOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
} from '@ant-design/icons';
import { Button, Card, Space, Statistic, Tag, Tooltip, Typography } from 'antd';
import type { MethodCard } from '../../api/client';
import {
  PENDING_VALUE,
  entryRuleText,
  evidenceLevel,
  evidenceTone,
  exitRuleSummary,
  formatPercent,
  methodCanScreen,
  methodStatusLabel,
  methodUnavailableReason,
  outcomeLabel,
  scopeLabel,
} from '../../lib/methodPresentation';

const { Text, Title } = Typography;

const TONE_COLOR = {
  success: 'green',
  warning: 'gold',
  error: 'red',
  default: 'default',
} as const;

interface MethodSummaryCardProps {
  method: MethodCard;
  screening: boolean;
  onScreen: (method: MethodCard) => void;
  onValidation: (method: MethodCard) => void;
}

export default function MethodSummaryCard({ method, screening, onScreen, onValidation }: MethodSummaryCardProps) {
  const canScreen = methodCanScreen(method);
  const unavailableReason = methodUnavailableReason(method);
  const evidence = method.evidence;
  const hasOutcomeHit = evidence?.outcome_hit_rate !== undefined;
  // 旧证据只带样本外交易统计（oos_*），没有 outcome 口径与命中率。
  // 此时大数字直接展示样本外交易胜率，绝不渲染「待验证」——
  // 那会在「已验证」标签旁边自相矛盾。
  const hasOosRate = evidence?.oos_win_rate !== undefined;
  const hasAnyRate = hasOutcomeHit || hasOosRate;
  const rule = entryRuleText(method);

  return (
    <Card className="method-summary" styles={{ body: { padding: 0 } }}>
      <div className="method-summary__evidence" aria-label="历史验证结果">
        {hasOutcomeHit ? (
          <>
            <Text type="secondary">历史命中率</Text>
            <Statistic
              value={evidence!.outcome_hit_rate! * 100}
              precision={1}
              suffix="%"
              styles={{ content: { fontSize: 34 } }}
            />
            <Text type="secondary" style={{ fontSize: 12 }}>
              样本外交易胜率 {formatPercent(evidence?.oos_win_rate)}（{evidence?.oos_trades ?? '待记录'} 笔）
            </Text>
          </>
        ) : hasOosRate ? (
          <>
            <Text type="secondary">样本外交易胜率</Text>
            <Statistic
              value={evidence!.oos_win_rate! * 100}
              precision={1}
              suffix="%"
              styles={{ content: { fontSize: 34 } }}
            />
            <Text type="secondary" style={{ fontSize: 12 }}>
              {evidence!.oos_trades} 笔样本外交易（未记录成功定义口径，无法统计命中率）
            </Text>
          </>
        ) : (
          <>
            <Text type="secondary">历史命中率</Text>
            <Statistic value="待验证" styles={{ content: { fontSize: 24 } }} />
            <Text type="secondary" style={{ fontSize: 12 }}>还没有样本外验证数据</Text>
          </>
        )}
        <Tag color={TONE_COLOR[evidenceTone(evidence)]}>证据等级：{evidenceLevel(evidence)}</Tag>
      </div>

      <div className="method-summary__body">
        <Space orientation="vertical" size={12} style={{ display: 'flex' }}>
          <div>
            <Space size={8} wrap>
              <Text type="secondary">{method.name}</Text>
              <Tag>{methodStatusLabel(method.status)}</Tag>
              {canScreen && <Tag icon={<CheckCircleOutlined />} color="green">可用于今日筛选</Tag>}
            </Space>
            <Title level={3} className="method-summary__rule">{method.entry_summary}</Title>
            {rule && (
              <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }}>
                执行规则：{rule}
              </Text>
            )}
          </div>

          <div className="method-summary__metrics" aria-label="历史验证指标">
            <span><Text type="secondary">{hasOutcomeHit ? '命中样本' : '样本外交易'}</Text><strong>{hasOutcomeHit ? `${evidence!.outcome_observations ?? '待记录'} 个` : (evidence ? `${evidence.oos_trades} 笔` : '待验证')}</strong></span>
            <span><Text type="secondary">{hasOutcomeHit ? '交易收益' : '样本外收益'}</Text><strong>{formatPercent(evidence?.oos_return)}</strong></span>
            <span><Text type="secondary">最大回撤</Text><strong>{formatPercent(evidence?.oos_max_drawdown)}</strong></span>
            <span><Text type="secondary">验证口径</Text><strong>{outcomeLabel(method.outcome) !== PENDING_VALUE ? outcomeLabel(method.outcome) : hasAnyRate ? `${evidence!.oos_trades} 笔样本外回测` : PENDING_VALUE}</strong></span>
            <span><Text type="secondary">股票池</Text><strong>{scopeLabel(method)}</strong></span>
          </div>

          <Space size={8} wrap>
            <Tooltip title={unavailableReason} open={unavailableReason ? undefined : false}>
              <span>
                <Button
                  type="primary"
                  icon={<BarChartOutlined />}
                  disabled={!canScreen}
                  loading={screening}
                  onClick={() => onScreen(method)}
                >
                  筛选今日股票
                </Button>
              </span>
            </Tooltip>
            <Button icon={<ClockCircleOutlined />} onClick={() => onValidation(method)}>
              查看验证记录
            </Button>
            {!canScreen && unavailableReason && <Text type="secondary">{unavailableReason}</Text>}
          </Space>
        </Space>
      </div>

      <button
        type="button"
        className="method-summary__exit"
        aria-label="查看退出规则与验证记录"
        onClick={() => onValidation(method)}
      >
        <Text type="secondary">退出规则</Text>
        <Text>{exitRuleSummary(method)}</Text>
        <Text type="secondary" style={{ fontSize: 12 }}>选股结果的每个候选会带上这份退出计划</Text>
        <ArrowRightOutlined aria-hidden="true" />
      </button>
    </Card>
  );
}
