import { Card, Col, Row, Space, Statistic, Typography } from 'antd';
import type { DashboardWorkLog } from '../../api/client';

const { Text } = Typography;

const TONE_COLOR: Record<string, string> = {
  neutral: '#1677ff',
  positive: '#52c41a',
  warning: '#faad14',
};

/**
 * P4・把「空榜」变成「工作日志」。
 *
 * 展示本次运行的真实现场：扫描了多少只、多少方法通过资格、多少只因数据不足 /
 * 失效条件 / 方法资格被排除。0 候选 与「扫了 N 只、排除 M 只、0 只达标」
 * 传递的信息完全不同——后者证明系统真的在干活，消除「是不是坏了」的焦虑。
 */
export default function WorkLogPanel({ workLog }: { workLog: DashboardWorkLog }) {
  if (!workLog.available) {
    return (
      <Card size="small" title="本次运行日志">
        <Text type="secondary">还没有选股运行记录。运行一次选股后，这里会显示真实的扫描与排除明细。</Text>
      </Card>
    );
  }

  return (
    <Card
      size="small"
      title="本次运行日志"
      extra={
        workLog.snapshot_date ? (
          <Text type="secondary" style={{ fontSize: 12 }}>
            数据日期 {workLog.snapshot_date}
          </Text>
        ) : undefined
      }
    >
      <Row gutter={[16, 16]}>
        {workLog.lines.map((line) => (
          <Col key={line.key} xs={12} sm={8} md={6} lg={4}>
            <Space direction="vertical" size={0} style={{ display: 'flex' }}>
              <Statistic
                title={line.label}
                value={line.value}
                valueStyle={{ fontSize: 22, color: TONE_COLOR[line.tone] ?? undefined }}
              />
              {line.detail && (
                <Text type="secondary" style={{ fontSize: 11, lineHeight: 1.4 }}>
                  {line.detail}
                </Text>
              )}
            </Space>
          </Col>
        ))}
      </Row>
    </Card>
  );
}