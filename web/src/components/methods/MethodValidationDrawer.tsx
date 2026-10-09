import { Alert, Button, Card, Collapse, Descriptions, Drawer, Empty, Skeleton, Space, Tag, Timeline, Typography } from 'antd';
import type { MethodAuditEvent, MethodCard, MethodResearchTraceSummary } from '../../api/client';
import { formatDateTime } from '../../lib/datetime';
import {
  evidenceLevel,
  evidenceTone,
  exitRuleSummary,
  formatPercent,
  formatRatio,
  invalidRuleText,
  methodCanScreen,
  methodStatusLabel,
  outcomeLabel,
  universeLabel,
} from '../../lib/methodPresentation';

const { Paragraph, Text, Title } = Typography;
const TONE_COLOR = { success: 'green', warning: 'gold', error: 'red', default: 'default' } as const;

interface MethodValidationDrawerProps {
  open: boolean;
  method?: MethodCard;
  audit?: MethodAuditEvent[];
  researchTrace?: MethodResearchTraceSummary;
  loading: boolean;
  error?: string;
  onClose: () => void;
  onRetry: () => void;
}

export default function MethodValidationDrawer({ open, method, audit, researchTrace, loading, error, onClose, onRetry }: MethodValidationDrawerProps) {
  const evidence = method?.evidence;
  const invalidRule = method ? invalidRuleText(method) : '';
  // 旧证据没存验证窗口/池大小；方法卡带 source_research_id 时，用持久化的
  // 研究轨迹摘要补齐（这些数据只存在于批次记录上）。
  const validationSize = evidence?.universe_size ?? researchTrace?.universe_size;
  const validationStart = evidence?.validation_start ?? researchTrace?.validation_start;
  const validationEnd = evidence?.validation_end ?? researchTrace?.validation_end;
  const fromBatch = !evidence?.universe_size && !evidence?.validation_start && (validationSize !== undefined || !!validationStart || !!validationEnd);
  const validationUniverse = universeLabel(method?.universe);
  // 研究样本池（researched_stocks 等）≠ 今日筛选用的快照池，必须明确说明。
  const poolDiffers = method?.universe !== undefined && method.universe !== 'universe_usable' && method.universe !== 'universe_all' && method.universe !== 'universe_all_a';

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
                  <Descriptions.Item label="历史命中率"><Text strong>{formatPercent(evidence.outcome_hit_rate)}</Text></Descriptions.Item>
                  <Descriptions.Item label="样本外交易胜率"><Text strong>{formatPercent(evidence.oos_win_rate)}</Text></Descriptions.Item>
                  <Descriptions.Item label="证据等级"><Tag color={TONE_COLOR[evidenceTone(evidence)]}>{evidenceLevel(evidence)}</Tag></Descriptions.Item>
                  <Descriptions.Item label="样本外交易">{evidence.oos_trades} 笔</Descriptions.Item>
                  <Descriptions.Item label="样本外收益">{formatPercent(evidence.oos_return)}</Descriptions.Item>
                  <Descriptions.Item label="最大回撤">{formatPercent(evidence.oos_max_drawdown)}</Descriptions.Item>
                  <Descriptions.Item label="Sharpe / Sortino">{formatRatio(evidence.sharpe_ratio)} / {formatRatio(evidence.sortino_ratio)}</Descriptions.Item>
                  <Descriptions.Item label="成功定义">{outcomeLabel(method.outcome)}</Descriptions.Item>
                  {evidence.outcome_hit_rate !== undefined && <Descriptions.Item label="命中样本">{evidence.outcome_observations ?? '未记录'} 个</Descriptions.Item>}
                </Descriptions>
                <Paragraph type="secondary" style={{ margin: '8px 0 0', fontSize: 12 }}>
                  历史命中率按「成功定义」逐样本统计；样本外交易胜率按真实回测交易的盈亏笔数统计，两者口径不同，数值可能不一致。
                </Paragraph>
              </Card>

              <Card size="small" title="在哪些数据上验证">
                <Descriptions column={1} size="small" colon={false}>
                  <Descriptions.Item label="验证股票池">
                    <Text>{validationUniverse}{validationSize ? ` · ${validationSize} 只` : ''}</Text>
                  </Descriptions.Item>
                  <Descriptions.Item label="数据日期区间">
                    {validationStart || validationEnd ? (
                      <Text>{validationStart || '未记录'} ~ {validationEnd || '未记录'}{fromBatch ? '（来自研究批次记录）' : ''}</Text>
                    ) : (
                      <Text type="secondary">历史数据未记录验证窗口，不做推测。</Text>
                    )}
                  </Descriptions.Item>
                  <Descriptions.Item label="市场"><Text>{method.market || '未记录'}</Text></Descriptions.Item>
                </Descriptions>
                {poolDiffers && (
                  <Paragraph type="secondary" style={{ margin: '8px 0 0', fontSize: 12 }}>
                    上面的股票池是历史研究/验证用的样本池，与「筛选今日股票」实际扫描的当日市场快照股票池不同；今日筛选会在当日快照成员上逐股执行该方法，并用结构化的范围条件（板块/市值/ST 等）二次过滤。
                  </Paragraph>
                )}
                <Collapse
                  ghost
                  size="small"
                  style={{ margin: '4px 0 0' }}
                  items={[{
                    key: 'ids',
                    label: '技术标识（用于审计复现）',
                    children: (
                      <Descriptions column={1} size="small" colon={false}>
                        <Descriptions.Item label="快照 ID"><Text code copyable style={{ fontSize: 11 }}>{evidence.snapshot_id || '未记录'}</Text></Descriptions.Item>
                        <Descriptions.Item label="结果哈希"><Text code copyable style={{ fontSize: 11, wordBreak: 'break-all' }}>{evidence.result_hash || '未记录'}</Text></Descriptions.Item>
                      </Descriptions>
                    ),
                  }]}
                />
              </Card>
            </>
          )}

          <Card size="small" title="方法边界">
            <Descriptions column={1} size="small" colon={false}>
              <Descriptions.Item label="状态"><Tag>{methodStatusLabel(method.status)}</Tag></Descriptions.Item>
              <Descriptions.Item label="退出规则">{exitRuleSummary(method)}</Descriptions.Item>
              <Descriptions.Item label="失效条件">
                {method.invalidations?.length
                  ? method.invalidations.join('；')
                  : method.rules?.invalid_rule
                    ? (invalidRule && invalidRule !== '未记录条件' ? `编译产物中定义了失效判定规则：${invalidRule}` : '编译产物中定义了失效判定规则')
                    : '未记录'}
              </Descriptions.Item>
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
