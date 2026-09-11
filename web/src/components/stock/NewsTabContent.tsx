import { Card, Empty, Flex, List, Space, Spin, Tag, Typography } from 'antd';
import type { StockNewsResult, StockRef } from '../../types/api';

interface NewsTabContentProps {
  result: StockNewsResult | null;
  loading: boolean;
}

/** 把关联来源翻译成中文标签。区分「原生」与「推测」是刻意的：
 *  不能让推测结果看起来和数据源明确给出的信息一样可靠。 */
function matchLabel(ref?: StockRef): { text: string; color: string } {
  if (!ref) return { text: '未知', color: 'default' };
  if (ref.match_type === 'native') return { text: '原生关联', color: 'green' };
  if (ref.confidence >= 0.9) return { text: '标题命中', color: 'blue' };
  if (ref.confidence >= 0.5) return { text: '正文提及', color: 'orange' };
  return { text: '弱关联', color: 'default' };
}

function formatTime(value: string): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function NewsTabContent({ result, loading }: NewsTabContentProps) {
  if (loading) {
    return (
      <Card>
        <Flex justify="center" align="center" style={{ minHeight: 240 }}>
          <Spin size="large" />
        </Flex>
      </Card>
    );
  }

  if (!result) return null;

  const items = result.items || [];

  // 数据不足时如实说明，不用无关内容填充。
  if (items.length === 0) {
    return (
      <Card title="关联资讯">
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={result.message || '暂无关联资讯'}
        />
        {result.degraded && result.degraded.length > 0 && (
          <Flex vertical gap={4} style={{ marginTop: 12 }}>
            {result.degraded.map((d) => (
              <Typography.Text key={d.source} type="secondary">
                数据源 {d.source} 暂不可用：{d.error}
              </Typography.Text>
            ))}
          </Flex>
        )}
      </Card>
    );
  }

  return (
    <Space direction="vertical" size={12} style={{ display: 'flex' }}>
      {result.status === 'stale' && result.degraded && result.degraded.length > 0 && (
        <Card size="small">
          <Typography.Text type="warning">
            部分数据源不可用，以下为库内已有数据：
            {result.degraded.map((d) => ` ${d.source}`).join('、')}
          </Typography.Text>
        </Card>
      )}

      <Card
        title="关联资讯"
        extra={
          result.weakCount && result.weakCount > 0 ? (
            <Typography.Text type="secondary">
              另有 {result.weakCount} 条仅正文提及
            </Typography.Text>
          ) : null
        }
      >
        <List
          dataSource={items}
          renderItem={(item) => {
            const ref = item.stockRefs?.[0];
            const label = matchLabel(ref);
            return (
              <List.Item
                actions={[
                  <Typography.Text type="secondary">{item.source}</Typography.Text>,
                  <Typography.Text type="secondary">{formatTime(item.publishTime)}</Typography.Text>,
                ]}
              >
                <List.Item.Meta
                  title={
                    <Space size={8} wrap>
                      <Tag color={label.color}>{label.text}</Tag>
                      {item.url ? (
                        <Typography.Link href={item.url} target="_blank" rel="noreferrer" strong>
                          {item.title}
                        </Typography.Link>
                      ) : (
                        <Typography.Text strong>{item.title}</Typography.Text>
                      )}
                    </Space>
                  }
                  description={
                    <Space direction="vertical" size={4}>
                      {item.summary && (
                        <Typography.Text type="secondary">{item.summary}</Typography.Text>
                      )}
                      {item.tags && item.tags.length > 0 && (
                        <Space size={4} wrap>
                          {item.tags.map((tag) => (
                            <Tag key={tag}>{tag}</Tag>
                          ))}
                        </Space>
                      )}
                    </Space>
                  }
                />
              </List.Item>
            );
          }}
          pagination={{ pageSize: 10, showSizeChanger: false, showTotal: (total) => `共 ${total} 条` }}
        />
      </Card>
    </Space>
  );
}
