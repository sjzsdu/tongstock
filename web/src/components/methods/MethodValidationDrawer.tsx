import { Alert, Button, Card, Collapse, Descriptions, Drawer, Empty, Skeleton, Space, Tag, Timeline, Typography } from 'antd';
import type { MethodAuditEvent, MethodCard } from '../../api/client';
import { formatDateTime } from '../../lib/datetime';
import {
  evidenceLevel,
  evidenceTone,
  formatPercent,
  formatRatio,
  methodCanScreen,
  methodStatusLabel,
  outcomeLabel,
  scopeLabel,
} from '../../lib/methodPresentation';

const { Paragraph, Text, Title } = Typography;
const TONE_COLOR = { success: 'green', warning: 'gold', error: 'red', default: 'default' } as const;

interface MethodValidationDrawerProps {
  open: boolean;
  method?: MethodCard;
  audit?: MethodAuditEvent[];
  loading: boolean;
  error?: string;
  onClose: () => void;
  onRetry: () => void;
}

export default function MethodValidationDrawer({ open, method, audit, loading, error, onClose, onRetry }: MethodValidationDrawerProps) {
  const evidence = method?.evidence;

  return (
    <Drawer
      title="验证记录"
      open={open}
      onClose={onClose}
      size="large"
      destroyOnHidden
    >
      {loading && !method ? (
        <Skeleton active paragraph={{ rows: 9 }} />
      ) : error && !method ? (
        <Alert type="error" showIcon title="验证记录读取失败" description={error} action={<Button size="small" onClick={onRetry}>重试</Button>} />
      ) : method ? (
        <Space orientation="vertical" size={16} style={{ display: 'flex' }}>
          <div>
            <Text type="secondary">{method.name}</Text>
            <Title level={3} style={{ margin: '4px 0 0' }}>{method.entry_summary}</Title>
          </div>

          {!evidence ? (
            <Alert type="info" showIcon title="还没有历史验证记录" description="该方法当前不会进入今日选股。" />
          ) : (
            <>
              <Alert
                type={methodCanScreen(method) ? 'success' : 'warning'}
                showIcon
                title={methodCanScreen(method) ? '历史验证已达到选股门槛' : '历史验证未达到选股门槛'}
                description="这是在样本外数据上的历史统计结果，不是对未来上涨的保证。"
              />

              <Card size="small" title="验证结果">
                <Descriptions column={{ xs: 1, sm: 2 }} size="small" colon={false}>
                  <Descriptions.Item label={evidence.outcome_hit_rate === undefined ? '样本外交易胜率' : '历史命中率'}><Text strong>{formatPercent(evidence.outcome_hit_rate ?? evidence.oos_win_rate)}</Text></Descriptions.Item>
                  <Descriptions.Item label="证据等级"><Tag color={TONE_COLOR[evidenceTone(evidence)]}>{evidenceLevel(evidence)}</Tag></Descriptions.Item>
                  <Descriptions.Item label="样本外交易">{evidence.oos_trades} 笔</Descriptions.Item>
                  <Descriptions.Item label="样本外收益">{formatPercent(evidence.oos_return)}</Descriptions.Item>
                  <Descriptions.Item label="最大回撤">{formatPercent(evidence.oos_max_drawdown)}</Descriptions.Item>
                  <Descriptions.Item label="Sharpe / Sortino">{formatRatio(evidence.sharpe_ratio)} / {formatRatio(evidence.sortino_ratio)}</Descriptions.Item>
                  <Descriptions.Item label="成功定义">{outcomeLabel(method.outcome)}</Descriptions.Item>
                  {evidence.outcome_hit_rate !== undefined && <Descriptions.Item label="命中样本">{evidence.outcome_observations ?? '未记录'} 个</Descriptions.Item>}
                </Descriptions>
              </Card>

              <Card size="small" title="在哪些数据上验证">
                <Descriptions column={1} size="small" colon={false}>
                  <Descriptions.Item label="快照 ID"><Text code copyable>{evidence.snapshot_id || '未记录'}</Text></Descriptions.Item>
                  <Descriptions.Item label="股票池 / Universe"><Text>{scopeLabel(method)}</Text></Descriptions.Item>
                  <Descriptions.Item label="市场"><Text>{method.market || '未记录'}</Text></Descriptions.Item>
                  <Descriptions.Item label="结果哈希"><Text code copyable>{evidence.result_hash || '未记录'}</Text></Descriptions.Item>
                  <Descriptions.Item label="数据日期区间">
                    <Text type="secondary">当前方法卡未提供日期区间，不做推测。</Text>
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            </>
          )}

          <Card size="small" title="方法边界">
            <Descriptions column={1} size="small" colon={false}>
              <Descriptions.Item label="状态"><Tag>{methodStatusLabel(method.status)}</Tag></Descriptions.Item>
              <Descriptions.Item label="退出规则">{method.exit_summary || '未设定'}</Descriptions.Item>
              <Descriptions.Item label="失效条件">{method.invalidations?.length ? method.invalidations.join('；') : '未记录'}</Descriptions.Item>
              <Descriptions.Item label="最后更新">{formatDateTime(method.updated_at)}</Descriptions.Item>
            </Descriptions>
          </Card>

          <Collapse
            items={[{
              key: 'audit',
              label: `审计轨迹（${audit?.length ?? 0}）`,
              children: error ? (
                <Alert type="error" showIcon title={error} action={<Button size="small" onClick={onRetry}>重试</Button>} />
              ) : loading && audit === undefined ? (
                <Skeleton active paragraph={{ rows: 3 }} />
              ) : audit?.length ? (
                <Timeline items={audit.map((event) => ({
                  key: event.id,
                  children: (
                    <Space orientation="vertical" size={2} style={{ display: 'flex' }}>
                      <Text>{methodStatusLabel(event.from || '无')} → <Text strong>{methodStatusLabel(event.to)}</Text></Text>
                      <Paragraph type="secondary" style={{ margin: 0 }}>{event.reason || '未记录原因'}</Paragraph>
                      <Text type="secondary">{event.actor} · {formatDateTime(event.created_at)}{event.automatic ? ' · 系统自动' : ''}</Text>
                    </Space>
                  ),
                }))} />
              ) : (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无审计记录" />
              ),
            }]}
          />

          <Button href="/methods/advanced">前往高级研究与管理</Button>
        </Space>
      ) : null}
    </Drawer>
  );
}
