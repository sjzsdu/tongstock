import { Button, Card, Flex, Space, Tag, Tooltip, Typography } from 'antd';
import { CompressOutlined, ExpandOutlined, RobotOutlined, SyncOutlined } from '@ant-design/icons';
import type { KlineSyncState } from '../../types/api';
import { formatDate } from '../../lib/datetime';
import { formatSigned } from '../../lib/stock-detail';

interface StockInfoHeaderProps {
  code: string;
  quote: any;
  ktype: string;
  pct: number;
  valueColor: string;
  up: boolean;
  syncState: KlineSyncState | null;
  fullscreen: boolean;
  onFullscreenChange: (fullscreen: boolean) => void;
  onAgentClick: () => void;
  onParadigmClick: () => void;
  onParadigmRefresh: () => void;
}

export function StockInfoHeader({
  code,
  quote,
  ktype,
  pct,
  valueColor,
  up,
  syncState,
  fullscreen,
  onFullscreenChange,
  onAgentClick,
  onParadigmClick,
  onParadigmRefresh,
}: StockInfoHeaderProps) {
  return (
    <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(30,41,59,0.95), rgba(15,23,42,0.92))' }}>
      <Flex justify="space-between" align="flex-start" gap={16} wrap>
        <Space direction="vertical" size={14} style={{ flex: 1, minWidth: 280 }}>
          <Space direction="vertical" size={12} style={{ display: 'flex' }}>
            <Space wrap size={[8, 8]}>
              <Tag color="blue">{quote.Code || code}</Tag>
              <Tag color={ktype === 'day' ? 'geekblue' : 'purple'}>{ktype.toUpperCase()}</Tag>
              <Tag color="success">实时分析</Tag>
              {syncState?.last_date && (
                <Tooltip title={syncState.last_sync_at ? `最后同步: ${formatDate(new Date(syncState.last_sync_at))}` : '同步时间未知'}>
                  <Tag color={syncState.status === 'ok' ? 'cyan' : 'orange'}>
                    数据截至 {syncState.last_date}
                  </Tag>
                </Tooltip>
              )}
              {syncState?.status === 'failed' && syncState.error && (
                <Tooltip title={syncState.error}>
                  <Tag color="red">同步异常</Tag>
                </Tooltip>
              )}
            </Space>
            <Space wrap size={[8, 8]} align="center">
              <Typography.Title level={2} style={{ margin: 0 }}>{quote.Name}</Typography.Title>
              <Typography.Text style={{ fontSize: 32, fontWeight: 700, color: valueColor }}>
                {quote.Price?.toFixed(2)}
              </Typography.Text>
              <Tag color={up ? 'red' : 'green'}>{formatSigned(pct, '%')}</Tag>
              <Typography.Text style={{ color: valueColor }}>
                {formatSigned(quote.Price - quote.LastClose)}
              </Typography.Text>
            </Space>
            <Typography.Text type="secondary">
              结合行情、技术指标、分时与 F10 数据，适合做单只股票的快速研判。
            </Typography.Text>
          </Space>
        </Space>
        <Button
          icon={fullscreen ? <CompressOutlined /> : <ExpandOutlined />}
          onClick={() => onFullscreenChange(!fullscreen)}
        >
          {fullscreen ? '退出全屏' : '全屏'}
        </Button>
        <Button
          type="primary"
          icon={<RobotOutlined />}
          onClick={onAgentClick}
        >
          AI 分析
        </Button>
        <Button
          icon={<RobotOutlined />}
          onClick={onParadigmClick}
        >
          范式挖掘
        </Button>
        <Button
          icon={<SyncOutlined />}
          onClick={onParadigmRefresh}
        >
          重新挖掘
        </Button>
      </Flex>
    </Card>
  );
}
