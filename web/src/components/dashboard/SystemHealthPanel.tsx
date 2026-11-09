import { Card, Space, Tooltip, Typography } from 'antd';
import type { DashboardDataStatus, DashboardHealth, DashboardSignal } from '../../api/client';

const { Text } = Typography;

/** 信号灯颜色：ok=绿 / attention=橙 / blocked=红。 */
const DOT_COLOR: Record<DashboardSignal['status'], string> = {
  ok: '#52c41a',
  attention: '#faad14',
  blocked: '#ff4d4f',
};

const OVERALL_LABEL: Record<DashboardHealth['overall'], string> = {
  ok: '系统正常',
  attention: '需要关注',
  blocked: '存在阻断',
};

const OVERALL_COLOR: Record<DashboardHealth['overall'], string> = {
  ok: '#52c41a',
  attention: '#faad14',
  blocked: '#ff4d4f',
};

/**
 * P5・首屏「系统健康状态」面板。
 *
 * 一行信号灯式总览：数据新鲜度 / 已验证方法 / 今日候选 / 持仓。
 * 让用户第一眼就知道系统处于什么状态，才敢开始操作。
 * 所有数值都来自后端聚合的真实事实，不在这里臆造任何指标。
 */
export default function SystemHealthPanel({ health, data, asOf }: { health: DashboardHealth; data: DashboardDataStatus; asOf: string }) {
  const observedAt = Number.isNaN(Date.parse(asOf)) ? asOf : new Date(asOf).toLocaleString('zh-CN', { hour12: false });
  return (
    <Card
      size="small"
      title={
        <Space size={8}>
          <span
            style={{
              display: 'inline-block',
              width: 8,
              height: 8,
              borderRadius: '50%',
              background: OVERALL_COLOR[health.overall],
            }}
          />
          <Text strong>系统状态</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {OVERALL_LABEL[health.overall]}
          </Text>
        </Space>
      }
    >
      <Space size={24} wrap>
        {health.signals.map((signal) => (
          <Tooltip key={signal.key} title={signal.detail}>
            <Space size={8} style={{ cursor: 'default' }}>
              <span
                style={{
                  display: 'inline-block',
                  width: 8,
                  height: 8,
                  borderRadius: '50%',
                  background: DOT_COLOR[signal.status],
                }}
              />
              <Text type="secondary" style={{ fontSize: 12 }}>
                {signal.label}
              </Text>
              <Text strong>{signal.value}</Text>
            </Space>
          </Tooltip>
        ))}
      </Space>
      <Text type="secondary" style={{ display: 'block', marginTop: 10, fontSize: 12 }}>
        首页行情来源：本地日线库（最新交易日 {data.latest_kline_date || '暂无'}） · 快照 {data.latest_snapshot_date || '暂无'} · 读模型时间 {observedAt}
      </Text>
    </Card>
  );
}
