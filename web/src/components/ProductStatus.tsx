import { Alert, Button, Empty, Space, Spin, Tag, Tooltip, Typography } from 'antd';
import {
  ClockCircleOutlined,
  CloudUploadOutlined,
  ExclamationCircleOutlined,
  InfoCircleOutlined,
  LoadingOutlined,
  ReloadOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import type { ProductStatusKind, ProductStatusState } from '../hooks/useProductStatus';

const { Text } = Typography;

/**
 * 统一的产品状态显示组件。
 *
 * 设计目标：
 *  - 对所有核心页面提供一致的状态反馈
 *  - 对所有非 ready 状态给出可执行的恢复动作
 *  - 折叠底层工程细节（原因码 / 原始错误）到 tooltip，默认不展示
 *
 * 使用：
 *   <ProductStatusBar state={state} onRetry={refresh} />
 *   <ProductStatusBanner state={state} onRetry={refresh} />
 *   <ProductStatusBadge state={state} />
 *   <ProductStatusBlock state={state} onRetry={refresh} />
 */

const KIND_CONFIG: Record<
  ProductStatusKind,
  {
    color: string;
    icon: React.ReactNode;
    level: 'info' | 'success' | 'warning' | 'error';
  }
> = {
  loading: {
    color: 'blue',
    icon: <Spin indicator={<LoadingOutlined />} size="small" />,
    level: 'info',
  },
  ready: {
    color: 'green',
    icon: <CloudUploadOutlined />,
    level: 'success',
  },
  degraded: {
    color: 'orange',
    icon: <ClockCircleOutlined />,
    level: 'warning',
  },
  failed: {
    color: 'red',
    icon: <WarningOutlined />,
    level: 'error',
  },
  unavailable: {
    color: 'red',
    icon: <ExclamationCircleOutlined />,
    level: 'error',
  },
};

interface StatusBaseProps {
  state: ProductStatusState;
  /** 若提供则显示一个重试按钮 */
  onRetry?: () => void;
  /** 额外说明（产品级补充信息） */
  contextLabel?: string;
}

/** 顶部/内嵌条带状状态反馈 */
export function ProductStatusBar({ state, onRetry, contextLabel }: StatusBaseProps) {
  const cfg = KIND_CONFIG[state.kind];
  const tooltipTitle = state.meta.reason
    ? `${state.meta.label}${state.meta.reason ? ` · ${state.meta.reason}` : ''}${state.meta.lastUpdated ? ` · ${state.meta.lastUpdated}` : ''}`
    : undefined;

  return (
    <Tooltip title={tooltipTitle}>
      <Space size={8} style={{ cursor: 'default' }}>
        {cfg.icon}
        <Text type={state.kind === 'ready' ? 'success' : undefined} strong={state.kind !== 'ready'}>
          {state.meta.label}
        </Text>
        {state.meta.lastUpdated && (
          <Text type="secondary" style={{ fontSize: 12 }}>
            · {state.meta.lastUpdated}
          </Text>
        )}
        {contextLabel && (
          <Text type="secondary" style={{ fontSize: 12 }}>
            · {contextLabel}
          </Text>
        )}
        {(state.kind === 'degraded' ||
          state.kind === 'failed' ||
          state.kind === 'unavailable') &&
          onRetry && (
            <Button
              size="small"
              type="link"
              icon={<ReloadOutlined />}
              onClick={onRetry}
              title={state.meta.actionHint || '重试'}
            >
              重试
            </Button>
          )}
      </Space>
    </Tooltip>
  );
}

/** 顶部 Alert 式横幅，适合整页不可用 / 降级提示 */
export function ProductStatusBanner({ state, onRetry, contextLabel }: StatusBaseProps) {
  if (state.kind === 'ready') return null;

  const cfg = KIND_CONFIG[state.kind];
  const alertType = cfg.level === 'error' ? 'error' : cfg.level === 'warning' ? 'warning' : 'info';

  return (
    <Alert
      type={alertType}
      showIcon
      icon={cfg.icon}
      style={{ margin: 12 }}
      message={
        <Space>
          <span>{state.meta.label}</span>
          {contextLabel && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              · {contextLabel}
            </Text>
          )}
          {state.meta.lastUpdated && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              · 最近数据：{state.meta.lastUpdated}
            </Text>
          )}
        </Space>
      }
      description={
        <Space direction="vertical" size={4}>
          {state.meta.actionHint && <Text>{state.meta.actionHint}</Text>}
          {state.meta.reason && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              原因：{state.meta.reason}
            </Text>
          )}
        </Space>
      }
      action={
        onRetry &&
        (state.kind === 'degraded' || state.kind === 'failed' || state.kind === 'unavailable') ? (
          <Button size="small" onClick={onRetry} icon={<ReloadOutlined />}>
            重试
          </Button>
        ) : undefined
      }
    />
  );
}

/** 紧凑 Tag 式徽章，适合表格或小区域 */
export function ProductStatusBadge({ state }: { state: ProductStatusState }) {
  const cfg = KIND_CONFIG[state.kind];
  const tooltipTitle = [state.meta.label, state.meta.lastUpdated, state.meta.reason]
    .filter(Boolean)
    .join(' · ');
  return (
    <Tooltip title={tooltipTitle || undefined}>
      <Tag color={cfg.color} icon={cfg.icon} style={{ marginInlineEnd: 0 }}>
        {state.meta.label}
      </Tag>
    </Tooltip>
  );
}

/** 全屏占位：加载 / 不可用 / 失败 */
export function ProductStatusBlock({ state, onRetry }: StatusBaseProps) {
  const cfg = KIND_CONFIG[state.kind];

  if (state.kind === 'loading') {
    return (
      <div style={{ padding: 48, textAlign: 'center' }}>
        <Spin indicator={<LoadingOutlined style={{ fontSize: 36 }} />} />
        <div style={{ marginTop: 12 }}>
          <Text type="secondary">{state.meta.label}</Text>
        </div>
      </div>
    );
  }

  if (state.kind === 'ready') {
    return null;
  }

  return (
    <Empty
      image={state.kind === 'unavailable' ? Empty.PRESENTED_IMAGE_SIMPLE : undefined}
      description={
        <Space direction="vertical" size={4}>
          <Space>
            {cfg.icon}
            <Text strong>{state.meta.label}</Text>
          </Space>
          {state.meta.actionHint && <Text type="secondary">{state.meta.actionHint}</Text>}
          {state.meta.lastUpdated && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              最近数据：{state.meta.lastUpdated}
            </Text>
          )}
          {state.meta.reason && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              原因：{state.meta.reason}
            </Text>
          )}
          {onRetry && (state.kind === 'degraded' || state.kind === 'failed' || state.kind === 'unavailable') && (
            <Button icon={<ReloadOutlined />} onClick={onRetry}>
              重试
            </Button>
          )}
        </Space>
      }
    />
  );
}

/** 便捷：将 SyncFreshnessResult 直接渲染为徽章 */
export function FreshnessBadgeFromResult({
  result,
}: {
  result?: {
    freshness: string;
    last_date?: string;
    last_sync_at?: string;
    stale_reason?: string;
    error?: string;
  };
}) {
  const kind: ProductStatusKind = (() => {
    switch (result?.freshness) {
      case 'fresh':
        return 'ready';
      case 'stale':
      case 'outdated':
        return 'degraded';
      case 'failed':
        return 'failed';
      default:
        return 'unavailable';
    }
  })();

  const state: ProductStatusState = {
    kind,
    meta: {
      label:
        kind === 'ready'
          ? '数据新鲜'
          : kind === 'degraded'
            ? '数据延迟'
            : kind === 'failed'
              ? '同步失败'
              : '暂不可用',
      actionHint:
        kind === 'degraded'
          ? '点击刷新'
          : kind === 'failed'
            ? '点击重试'
            : kind === 'unavailable'
              ? '请稍后重试'
              : '',
      level:
        kind === 'ready'
          ? 'success'
          : kind === 'degraded'
            ? 'warning'
            : 'error',
      lastUpdated: result?.last_date || result?.last_sync_at,
      reason: result?.stale_reason || result?.error,
    },
    canOperate: kind === 'ready' || kind === 'degraded',
  };

  return <ProductStatusBadge state={state} />;
}

// 供调试/复用的图标组件（避免外部再次 import）
export { InfoCircleOutlined };
