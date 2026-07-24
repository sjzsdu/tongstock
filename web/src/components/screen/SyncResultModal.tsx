import { Modal, Button, Space, Flex, Typography, Collapse, List, Tag } from 'antd';
import type { KlineBatchSyncResult } from '../../types/api';

const { Text } = Typography;

interface SyncResultModalProps {
  result: KlineBatchSyncResult | null;
  onCancel: () => void;
}

export function SyncResultModal({ result, onCancel }: SyncResultModalProps) {
  if (!result) return null;

  return (
    <Modal
      title="同步结果详情"
      open={!!result}
      onCancel={onCancel}
      footer={<Button onClick={onCancel}>关闭</Button>}
      width={600}
    >
      <Space direction="vertical" size={12} style={{ display: 'flex' }}>
        <Flex gap={24}>
          <Text>总数: <strong>{result.total}</strong></Text>
          <Text style={{ color: '#22c55e' }}>成功: <strong>{result.success}</strong></Text>
          <Text style={{ color: '#ef4444' }}>失败: <strong>{result.failed}</strong></Text>
        </Flex>
        {result.results.filter((r) => r.status !== 'ok').length > 0 && (
          <Collapse
            size="small"
            items={[
              {
                key: 'failed',
                label: <Text type="danger">失败详情 ({result.results.filter((r) => r.status !== 'ok').length})</Text>,
                children: (
                  <List
                    size="small"
                    dataSource={result.results.filter((r) => r.status !== 'ok')}
                    renderItem={(item) => (
                      <List.Item>
                        <Space>
                          <Text code>{item.code}</Text>
                          <Text type="danger">{item.error || item.status}</Text>
                        </Space>
                      </List.Item>
                    )}
                  />
                ),
              },
            ]}
          />
        )}
        {result.results.filter((r) => r.status === 'ok' && r.state).length > 0 && (
          <Collapse
            size="small"
            items={[
              {
                key: 'success',
                label: <Text type="success">成功详情 ({result.results.filter((r) => r.status === 'ok' && r.state).length})</Text>,
                children: (
                  <List
                    size="small"
                    dataSource={result.results.filter((r) => r.status === 'ok' && r.state)}
                    renderItem={(item) => (
                      <List.Item>
                        <Space>
                          <Text code>{item.code}</Text>
                          {item.state?.last_date && <Tag>{item.state.last_date}</Tag>}
                          {item.state?.row_count !== undefined && <Text type="secondary">{item.state.row_count} 条</Text>}
                        </Space>
                      </List.Item>
                    )}
                  />
                ),
              },
            ]}
          />
        )}
      </Space>
    </Modal>
  );
}