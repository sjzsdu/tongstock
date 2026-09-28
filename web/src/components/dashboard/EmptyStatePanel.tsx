import { Alert, Button, Card, Space, Typography } from 'antd';
import {
  CloudSyncOutlined,
  ExperimentOutlined,
  FileSearchOutlined,
  PlayCircleOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import type { DashboardAction, DashboardEmptyState, EmptyStateReason } from '../../api/client';

const { Text, Paragraph } = Typography;

/** 每种空榜原因对应的图标与色调，让「为什么空」一眼可辨。 */
const REASON_STYLE: Record<EmptyStateReason, { icon: React.ReactNode; color: string }> = {
  data_not_synced: { icon: <CloudSyncOutlined />, color: '#1677ff' },
  no_verified_methods: { icon: <SafetyCertificateOutlined />, color: '#faad14' },
  selection_not_run: { icon: <PlayCircleOutlined />, color: '#1677ff' },
  no_candidates: { icon: <FileSearchOutlined />, color: '#52c41a' },
  has_candidates: { icon: <ExperimentOutlined />, color: '#52c41a' },
};

const ACTION_ICON: Record<string, React.ReactNode> = {
  sync: <CloudSyncOutlined />,
  seed_methods: <SafetyCertificateOutlined />,
  run_selection: <PlayCircleOutlined />,
  view_exclusions: <FileSearchOutlined />,
  research: <ExperimentOutlined />,
};

/**
 * P1・空状态「分诊 + 下一步」。
 *
 * 每张空卡片都强制回答两个问题：为什么空、现在该点哪。
 * 文案与按钮全部来自后端 empty_state（它按真实原因判定），
 * 前端只负责渲染，不在本地猜测原因或编造「下一步」。
 */
export default function EmptyStatePanel({
  empty,
  onAction,
  busyKind,
}: {
  empty: DashboardEmptyState;
  onAction: (action: DashboardAction) => void;
  /** 正在执行的动作类型；执行期间禁用按钮，避免重复触发重活。 */
  busyKind?: string;
}) {
  const style = REASON_STYLE[empty.reason] ?? REASON_STYLE.no_candidates;
  const busy = Boolean(busyKind);

  return (
    <Card
      bordered={false}
      style={{ borderLeft: `3px solid ${style.color}` }}
      styles={{ body: { padding: 16 } }}
    >
      <Space direction="vertical" size={10} style={{ display: 'flex' }}>
        <Space size={10}>
          <span style={{ color: style.color, fontSize: 18 }}>{style.icon}</span>
          <Text strong style={{ fontSize: 15 }}>
            {empty.title}
          </Text>
        </Space>
        <Paragraph type="secondary" style={{ margin: 0 }}>
          {empty.message}
        </Paragraph>
        {empty.actions.length > 0 && (
          <Space size={12} wrap>
            {empty.actions.map((action, idx) => (
              <Button
                key={`${action.kind}-${idx}`}
                type={action.primary ? 'primary' : 'default'}
                icon={ACTION_ICON[action.kind]}
                loading={busyKind === action.kind}
                disabled={busy && busyKind !== action.kind}
                onClick={() => onAction(action)}
              >
                {action.label}
              </Button>
            ))}
          </Space>
        )}
      </Space>
    </Card>
  );
}

/** 已扫描但没有候选时，展示排除原因明细（配合 P4 的「查看排除原因」动作）。 */
export function ExclusionBreakdown({
  counts,
  samples,
}: {
  counts: Record<string, number>;
  samples: Array<{ reason_code: string; detail: string; code?: string }>;
}) {
  const entries = Object.entries(counts).filter(([, n]) => n > 0);
  if (entries.length === 0 && samples.length === 0) {
    return null;
  }
  return (
    <Alert
      type="info"
      showIcon
      message="排除原因明细"
      description={
        <Space direction="vertical" size={6} style={{ display: 'flex' }}>
          <Space size={16} wrap>
            {entries.map(([code, n]) => (
              <Text key={code} type="secondary" style={{ fontSize: 12 }}>
                {code}：{n}
              </Text>
            ))}
          </Space>
          {samples.length > 0 && (
            <Space direction="vertical" size={2} style={{ display: 'flex' }}>
              {samples.slice(0, 5).map((s, i) => (
                <Text key={`${s.reason_code}-${i}`} type="secondary" style={{ fontSize: 12 }}>
                  {s.code ? `${s.code} · ` : ''}
                  {s.reason_code}：{s.detail}
                </Text>
              ))}
            </Space>
          )}
        </Space>
      }
    />
  );
}