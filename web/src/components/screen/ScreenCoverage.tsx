import { Button, Flex, Modal, Popover, Space, Tag, Tooltip, Typography } from 'antd';
import type { ScreenCoverage, ScreenSkipReason, ScreenSkippedStock } from '../../types/api';
import { SKIP_REASON_HINT, SKIP_REASON_LABEL, countSkipped } from '../../lib/screenCoverage';

const { Text } = Typography;

type CoverageSummaryProps = {
  coverage: ScreenCoverage;
  onShowSkipped: () => void;
};

/**
 * 摘要卡右端的覆盖行：「覆盖 519/530 只」常驻可见；展开只做分类说明，收起的只是明细。
 * 全覆盖时只给数字，不给任何提示。
 */
export function CoverageSummary({ coverage, onShowSkipped }: CoverageSummaryProps) {
  if (coverage.total === 0) return null;
  const label = `覆盖 ${coverage.covered}/${coverage.total} 只`;
  if (coverage.covered >= coverage.total) {
    return <Text type="secondary">{label}</Text>;
  }
  const reasons: ScreenSkipReason[] = ['no_quote', 'insufficient_history', 'not_computed'];
  const rows = reasons
    .map((reason) => ({ reason, count: countSkipped(coverage, reason) }))
    .filter((row) => row.count > 0);
  return (
    <Popover
      trigger="click"
      placement="bottomRight"
      title="覆盖情况"
      content={
        <Space orientation="vertical" size={6} style={{ width: 240 }}>
          <Text strong>已参与筛选 {coverage.covered} 只</Text>
          {rows.map((row) => (
            <Tooltip key={row.reason} title={SKIP_REASON_HINT[row.reason]}>
              <Text type={row.reason === 'not_computed' ? 'warning' : 'secondary'}>
                {SKIP_REASON_LABEL[row.reason]} {row.count} 只
              </Text>
            </Tooltip>
          ))}
          <Button size="small" type="link" style={{ padding: 0 }} onClick={onShowSkipped}>
            查看名单
          </Button>
        </Space>
      }
    >
      <Button size="small" type="text">
        {label} ▾
      </Button>
    </Popover>
  );
}

type SkippedStocksModalProps = {
  coverage: ScreenCoverage | null;
  date: string;
  open: boolean;
  onClose: () => void;
};

export function SkippedStocksModal({ coverage, date, open, onClose }: SkippedStocksModalProps) {
  const items: ScreenSkippedStock[] = coverage?.skipped ?? [];
  return (
    <Modal
      title={`未参与筛选的股票（${items.length} 只）`}
      open={open}
      onCancel={onClose}
      footer={<Button onClick={onClose}>关闭</Button>}
      width={560}
      styles={{ body: { maxHeight: '60vh', overflowY: 'auto' } }}
    >
      <Space orientation="vertical" size={12} style={{ width: '100%' }}>
        <Text type="secondary">
          这些股票没有参与 {date || '该状态日'} 的筛选，命中结果里不会出现它们；其余股票的结果不受影响。
        </Text>
        <div style={{ maxHeight: 360, overflowY: 'auto' }}>
          {items.map((item) => (
            <Flex
              key={item.code}
              justify="space-between"
              align="center"
              gap={12}
              style={{ padding: '6px 0', borderBottom: '1px solid var(--ant-color-border-secondary)' }}
            >
              <Space size={8}>
                <Text code>{item.code}</Text>
                <Text>{item.name || '—'}</Text>
              </Space>
              <Tooltip title={SKIP_REASON_HINT[item.reason]}>
                <Tag color={item.reason === 'not_computed' ? 'orange' : 'default'}>{SKIP_REASON_LABEL[item.reason]}</Tag>
              </Tooltip>
            </Flex>
          ))}
        </div>
      </Space>
    </Modal>
  );
}
