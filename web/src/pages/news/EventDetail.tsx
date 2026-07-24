import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { ArrowLeftOutlined, ClockCircleOutlined, StockOutlined, TagOutlined, WarningOutlined, FilterOutlined } from '@ant-design/icons';
import { Card, Empty, Flex, List, Segmented, Space, Spin, Tag, Timeline, Typography } from 'antd';
import { api } from '../../api/client';
import type { HotEvent, NewsItem } from '../../types/api';

type ViewMode = 'timeline' | 'list' | 'source';

export default function EventDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [event, setEvent] = useState<HotEvent | null>(null);
  const [newsItems, setNewsItems] = useState<NewsItem[]>([]);
  const [viewMode, setViewMode] = useState<ViewMode>('timeline');
  const [filterSource, setFilterSource] = useState<string>('all');

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    api.hotEventDetail(id)
      .then((result) => {
        setEvent(result.event);
        setNewsItems(result.newsItems);
      })
      .catch(() => {
        setEvent(null);
        setNewsItems([]);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [id]);

  const sources = [...new Set(newsItems.map((n) => n.source))];
  const filteredNews = filterSource === 'all' ? newsItems : newsItems.filter((n) => n.source === filterSource);
  const sortedNews = [...filteredNews].sort((a, b) => new Date(b.publishTime).getTime() - new Date(a.publishTime).getTime());

  const formatTime = (timeStr: string) => {
    const date = new Date(timeStr);
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
  };

  const getHotColor = (hotIndex: number) => {
    if (hotIndex > 200) return 'red';
    if (hotIndex > 100) return 'orange';
    return 'gold';
  };

  if (loading) {
    return (
      <Card>
        <Flex justify="center" align="center" style={{ minHeight: 400 }}>
          <Spin size="large" />
        </Flex>
      </Card>
    );
  }

  if (!event) {
    return (
      <Card>
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="事件不存在或已删除" />
      </Card>
    );
  }

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {/* 返回按钮 */}
      <Button
        icon={<ArrowLeftOutlined />}
        onClick={() => navigate(-1)}
        style={{ alignSelf: 'flex-start' }}
      >
        返回
      </Button>

      {/* 事件概览卡片 */}
      <Card>
        <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
          <div>
            <Typography.Title level={2} style={{ marginBottom: 8 }}>
              {event.title}
            </Typography.Title>
            <Space size={16}>
              <Tag color={getHotColor(event.hotIndex)} icon={<WarningOutlined />}>
                热点指数 {event.hotIndex}
              </Tag>
              <Tag icon={<ClockCircleOutlined />}>
                更新于 {formatTime(event.updatedAt)}
              </Tag>
              <Tag>{event.status}</Tag>
            </Space>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
            {/* 关键词 */}
            <div>
              <Typography.Text type="secondary" style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                <TagOutlined /> 关键词
              </Typography.Text>
              <Space wrap size={8}>
                {event.keywords.map((kw) => (
                  <Tag key={kw}>{kw}</Tag>
                ))}
              </Space>
            </div>

            {/* 关联股票 */}
            <div>
              <Typography.Text type="secondary" style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                <StockOutlined /> 关联股票
              </Typography.Text>
              <Space wrap size={8}>
                {event.relatedStocks.map((stock) => (
                  <Tag key={stock} color="cyan">{stock}</Tag>
                ))}
              </Space>
            </div>
          </div>

          {/* 来源统计 */}
          <div>
            <Typography.Text type="secondary" style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
              <FilterOutlined /> 来源分布
            </Typography.Text>
            <Space wrap size={8}>
              {Object.entries(event.sourceCounts).map(([source, count]) => (
                <Tag key={source}>{source} ({count})</Tag>
              ))}
            </Space>
          </div>
        </Space>
      </Card>

      {/* 视图切换和筛选 */}
      <Card>
        <Flex justify="space-between" align="center" style={{ flexWrap: 'wrap', gap: 12 }}>
          <Segmented
            value={viewMode}
            onChange={(value) => setViewMode(value as ViewMode)}
            options={[
              { label: '时间线', value: 'timeline' },
              { label: '列表', value: 'list' },
              { label: '按来源', value: 'source' },
            ]}
          />
          <Segmented
            value={filterSource}
            onChange={(value) => setFilterSource(value as string)}
            options={[
              { label: '全部来源', value: 'all' },
              ...sources.map((source) => ({ label: source, value: source })),
            ]}
          />
        </Flex>
      </Card>

      {/* 新闻内容 */}
      {viewMode === 'timeline' ? (
        <Card title="新闻时间线">
          {sortedNews.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无新闻" />
          ) : (
            <Timeline
              items={sortedNews.map((news) => ({
                color: news.hotScore > 100 ? '#ef4444' : '#1677ff',
                dot: <span style={{ fontSize: 12 }}>{news.hotScore}</span>,
                children: (
                  <Space direction="vertical" size={8} style={{ display: 'flex', width: '100%' }}>
                    <Typography.Title level={4} style={{ margin: 0 }}>
                      {news.title}
                    </Typography.Title>
                    <Typography.Text type="secondary" style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                      <span>{news.source}</span>
                      <span>{formatTime(news.publishTime)}</span>
                      <span>热度: {news.hotScore}</span>
                    </Typography.Text>
                    <Typography.Text>{news.summary}</Typography.Text>
                    {news.tags && news.tags.length > 0 && (
                      <Space size={4} wrap>
                        {news.tags.map((tag) => (
                          <Tag key={tag}>{tag}</Tag>
                        ))}
                      </Space>
                    )}
                  </Space>
                ),
              }))}
            />
          )}
        </Card>
      ) : viewMode === 'list' ? (
        <Card title="新闻列表">
          {sortedNews.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无新闻" />
          ) : (
            <List
              dataSource={sortedNews}
              renderItem={(news) => (
                <List.Item
                  actions={[
                    <Typography.Text type="secondary">{news.source}</Typography.Text>,
                    <Typography.Text type="secondary">{formatTime(news.publishTime)}</Typography.Text>,
                    <Tag color={news.hotScore > 100 ? 'red' : 'blue'}>{news.hotScore}</Tag>,
                  ]}
                >
                  <List.Item.Meta
                    title={<Typography.Text strong>{news.title}</Typography.Text>}
                    description={news.summary}
                  />
                  {news.tags && news.tags.length > 0 && (
                    <Space size={4} wrap>
                      {news.tags.map((tag) => (
                        <Tag key={tag}>{tag}</Tag>
                      ))}
                    </Space>
                  )}
                </List.Item>
              )}
              pagination={{ pageSize: 10, showSizeChanger: false, showTotal: (total) => `共 ${total} 条` }}
            />
          )}
        </Card>
      ) : (
        <Card title="按来源分组">
          {sources.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无新闻" />
          ) : (
            <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
              {sources.map((source) => {
                const sourceNews = sortedNews.filter((n) => n.source === source);
                if (sourceNews.length === 0) return null;
                return (
                  <Card key={source} title={`${source} (${sourceNews.length})`} size="small">
                    <List
                      dataSource={sourceNews}
                      renderItem={(news) => (
                        <List.Item>
                          <List.Item.Meta
                            title={<Typography.Text strong style={{ fontSize: 14 }}>{news.title}</Typography.Text>}
                            description={
                              <Space size={8}>
                                <Typography.Text type="secondary" style={{ fontSize: 12 }}>{formatTime(news.publishTime)}</Typography.Text>
                                <Tag color={news.hotScore > 100 ? 'red' : 'blue'}>{news.hotScore}</Tag>
                              </Space>
                            }
                          />
                        </List.Item>
                      )}
                      pagination={{ pageSize: 5, showSizeChanger: false }}
                    />
                  </Card>
                );
              })}
            </Space>
          )}
        </Card>
      )}
    </Space>
  );
}

function Button(props: { icon?: React.ReactNode; onClick?: () => void; children?: React.ReactNode; style?: React.CSSProperties }) {
  const { icon, onClick, children, style } = props;
  return (
    <button
      onClick={onClick}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 8,
        padding: '8px 16px',
        backgroundColor: '#1677ff',
        color: '#fff',
        border: 'none',
        borderRadius: 8,
        cursor: 'pointer',
        ...style,
      }}
    >
      {icon}
      {children}
    </button>
  );
}
