import {
  ArrowRightOutlined,
  BarChartOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
} from '@ant-design/icons';
import { Button, Card, Space, Statistic, Tag, Tooltip, Typography } from 'antd';
import type { MethodCard } from '../../api/client';
import {
  evidenceLevel,
  evidenceTone,
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
  const hitRate = evidence?.outcome_hit_rate ?? evidence?.oos_win_rate;

  return (
    <Card className="method-summary" styles={{ body: { padding: 0 } }}>
      <div className="method-summary__evidence" aria-label="历史验证结果">
        <Text type="secondary">{evidence?.outcome_hit_rate === undefined ? '样本外交易胜率' : '历史命中率'}</Text>
        <Statistic
          value={hitRate === undefined ? '待验证' : hitRate * 100}
          precision={hitRate === undefined ? undefined : 1}
          suffix={hitRate === undefined ? undefined : '%'}
          styles={{ content: { fontSize: hitRate === undefined ? 24 : 34 } }}
        />
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
          </div>

          <div className="method-summary__metrics" aria-label="历史验证指标">
            <span><Text type="secondary">{evidence?.outcome_hit_rate === undefined ? '样本外交易' : '命中样本'}</Text><strong>{evidence?.outcome_hit_rate === undefined ? (evidence ? `${evidence.oos_trades} 笔` : '待验证') : `${evidence.outcome_observations ?? '待记录'} 个`}</strong></span>
            <span><Text type="secondary">{evidence?.outcome_hit_rate === undefined ? '样本外收益' : '交易收益'}</Text><strong>{formatPercent(evidence?.oos_return)}</strong></span>
            <span><Text type="secondary">最大回撤</Text><strong>{formatPercent(evidence?.oos_max_drawdown)}</strong></span>
            <span><Text type="secondary">验证口径</Text><strong>{outcomeLabel(method.outcome)}</strong></span>
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

      <div className="method-summary__exit">
        <Text type="secondary">退出规则</Text>
        <Text>{method.exit_summary || '未设定'}</Text>
        <ArrowRightOutlined aria-hidden="true" />
      </div>
    </Card>
  );
}
