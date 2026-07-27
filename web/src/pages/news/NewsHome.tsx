import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ArrowRightOutlined, ClockCircleOutlined, FilterOutlined, FireOutlined, RestOutlined, TagOutlined, WarningOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { Card, Col, Empty, Input, List, Pagination, Row, Select, Space, Spin, Tag, Typography, message, Button } from 'antd';
import { api } from '../../api/client';
import type { NewsSummary, HotEvent, MarketSentiment } from '../../types/api';

const { Search } = Input;
const { Option } = Select;

const SOURCES = [
  { value: '', label: '全部来源' },
  { value: '财联社', label: '财联社' },
  { value: '雪球', label: '雪球' },
  { value: '巨潮资讯', label: '巨潮资讯' },
];

const NEWS_TYPES = [
  { value: '', label: '全部类型' },
  { value: '快讯', label: '快讯' },
  { value: '公告', label: '公告' },
  { value: '讨论', label: '讨论' },
  { value: '研报', label: '研报' },
];

const SORT_OPTIONS = [
  { value: 'publishTime', label: '最新发布' },
  { value: 'hotScore', label: '热度排序' },
];

export default function NewsHome() {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [newsList, setNewsList] = useState<NewsSummary[]>([]);
  const [hotEvents, setHotEvents] = useState<HotEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [marketSentiment, setMarketSentiment] = useState<MarketSentiment | null>(null);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);

  // 筛选条件
  const [source, setSource] = useState('');
  const [newsType, setNewsType] = useState('');
  const [sortBy, setSortBy] = useState('publishTime');
  const [fetchingBrowser, setFetchingBrowser] = useState(false);

  const observerRef = useRef<IntersectionObserver | null>(null);
  const lastItemRef = useRef<HTMLDivElement | null>(null);

  const fetchNews = async (reset = false) => {
    const currentPage = reset ? 1 : page;
    setLoading(reset);
    setLoadingMore(!reset);

    try {
      const params: Record<string, string> = {
        page: String(currentPage),
        pageSize: String(pageSize),
        sortBy,
      };
      if (source) params.sources = source;
      if (newsType) params.types = newsType;

      const result = await api.newsFeed(params);

      if (reset) {
        setNewsList(result.items || []);
        setTotal(result.total || 0);
      } else {
        setNewsList((prev) => [...prev, ...(result.items || [])]);
      }
      setPage(currentPage);
    } catch {
      if (reset) {
        setNewsList([]);
        setTotal(0);
      }
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  };

  const fetchHotEvents = async () => {
    try {
      const result = await api.hotEvents({ limit: 10 });
      setHotEvents(result.items || []);
    } catch {
      setHotEvents([]);
    }
  };

  const fetchMarketSentiment = async () => {
    try {
      const result = await api.sentimentMarket(24);
      setMarketSentiment(result);
    } catch {
      setMarketSentiment(null);
    }
  };

  useEffect(() => {
    fetchNews(true);
    fetchHotEvents();
    fetchMarketSentiment();
  }, []);

  useEffect(() => {
    // 无限滚动监听
    observerRef.current = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && !loading && !loadingMore && newsList.length < total) {
          setPage((prev) => prev + 1);
        }
      },
      { rootMargin: '200px' }
    );

    if (lastItemRef.current) {
      observerRef.current.observe(lastItemRef.current);
    }

    return () => {
      if (observerRef.current) {
        observerRef.current.disconnect();
      }
    };
  }, [newsList, loading, loadingMore, total]);

  useEffect(() => {
    // 筛选条件变化时重置
    setPage(1);
    fetchNews(true);
  }, [source, newsType, sortBy]);

  const handleFetchBrowserNews = async () => {
    setFetchingBrowser(true);
    try {
      const result = await api.newsFetchBrowser('all');
      if (result.count > 0) {
        message.success(`抓取成功，获取到 ${result.count} 条新闻`);
        setPage(1);
        fetchNews(true);
      } else {
        message.warning('未抓取到新闻，可能 ego-browser 未启动或网站无法访问');
      }
      if (result.errors && result.errors.length > 0) {
        message.warning(`部分数据源失败：${result.errors.join('；')}`);
      }
    } catch {
      message.error('浏览器抓取失败');
    } finally {
      setFetchingBrowser(false);
    }
  };

  const formatTime = (timeStr: string) => {
    const date = new Date(timeStr);
    const now = new Date();
    const diff = now.getTime() - date.getTime();
    const hours = Math.floor(diff / (1000 * 60 * 60));
    if (hours < 1) return '刚刚';
    if (hours < 24) return `${hours}小时前`;
    return date.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
  };

  const getHotColor = (hotScore: number) => {
    if (hotScore > 100) return 'red';
    if (hotScore > 50) return 'orange';
    return 'default';
  };

  return (
    <Row gutter={[24, 24]}>
      {/* 左侧：资讯信息流 */}
      <Col xs={24} lg={17}>
        {/* 筛选栏 */}
        <Card>
          <Space wrap size={16} style={{ width: '100%' }}>
            <Search
              placeholder="搜索新闻关键词..."
              allowClear
              enterButton={<FilterOutlined />}
              size="large"
              style={{ width: 320 }}
            />
            <Select
              value={source}
              onChange={setSource}
              placeholder="选择来源"
              size="large"
              style={{ width: 160 }}
            >
              {SOURCES.map((s) => (
                <Option key={s.value} value={s.value}>
                  {s.label}
                </Option>
              ))}
            </Select>
            <Select
              value={newsType}
              onChange={setNewsType}
              placeholder="选择类型"
              size="large"
              style={{ width: 160 }}
            >
              {NEWS_TYPES.map((t) => (
                <Option key={t.value} value={t.value}>
                  {t.label}
                </Option>
              ))}
            </Select>
            <Select
              value={sortBy}
              onChange={setSortBy}
              size="large"
              style={{ width: 140 }}
            >
              {SORT_OPTIONS.map((s) => (
                <Option key={s.value} value={s.value}>
                  {s.label}
                </Option>
              ))}
            </Select>
            <button
              onClick={() => {
                setSource('');
                setNewsType('');
                setSortBy('publishTime');
              }}
              style={{
                padding: '8px 16px',
                backgroundColor: '#1f2937',
                color: '#fff',
                border: 'none',
                borderRadius: 8,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: 8,
              }}
            >
              <RestOutlined />
              重置筛选
            </button>
            <Button
              type="primary"
              icon={<ThunderboltOutlined />}
              loading={fetchingBrowser}
              onClick={handleFetchBrowserNews}
              size="large"
            >
              抓取实时新闻
            </Button>
          </Space>
        </Card>

        {/* 新闻列表 */}
        <Card title={`资讯列表 (共 ${total} 条)`}>
          {loading ? (
            <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
              {[...Array(5)].map((_, i) => (
                <div key={i} style={{ display: 'flex', gap: 16 }}>
                  <div style={{ width: 120, height: 80, backgroundColor: '#1f2937', borderRadius: 8 }} />
                  <div style={{ flex: 1 }}>
                    <div style={{ height: 24, backgroundColor: '#1f2937', borderRadius: 4, marginBottom: 8 }} />
                    <div style={{ height: 16, backgroundColor: '#1f2937', borderRadius: 4, width: '60%', marginBottom: 8 }} />
                    <div style={{ height: 16, backgroundColor: '#1f2937', borderRadius: 4, width: '40%' }} />
                  </div>
                </div>
              ))}
            </Space>
          ) : newsList.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无新闻" />
          ) : (
            <List
              dataSource={newsList}
              renderItem={(news, index) => {
                const isLast = index === newsList.length - 1;
                const handleClick = () => {
                  if (news.url) {
                    window.open(news.url, '_blank');
                  } else {
                    navigate(`/news/event/${news.id}`);
                  }
                };
                return (
                  <div ref={isLast ? lastItemRef : null}>
                    <List.Item
                      actions={[
                        <Typography.Text type="secondary">{news.source}</Typography.Text>,
                        <Typography.Text type="secondary">{formatTime(news.publishTime)}</Typography.Text>,
                        <Tag color={getHotColor(news.hotScore)}>{news.hotScore}</Tag>,
                      ]}
                      style={{ padding: '16px 0', borderBottom: '1px solid #1f2937', cursor: 'pointer' }}
                      onClick={handleClick}
                    >
                      <List.Item.Meta
                        title={<Typography.Text strong style={{ fontSize: 15 }}>{news.title}</Typography.Text>}
                        description={news.summary}
                      />
                      {news.tags && news.tags.length > 0 && (
                        <Space size={4} wrap style={{ marginTop: 8 }}>
                          <TagOutlined style={{ fontSize: 12, color: '#9ca3af' }} />
                          {news.tags.map((tag) => (
                            <Tag key={tag}>{tag}</Tag>
                          ))}
                        </Space>
                      )}
                    </List.Item>
                  </div>
                );
              }}
              pagination={false}
            />
          )}

          {/* 加载更多 */}
          {loadingMore && (
            <div style={{ textAlign: 'center', padding: 20 }}>
              <Spin />
            </div>
          )}

          {/* 分页（备用） */}
          {!loading && newsList.length > 0 && (
            <div style={{ textAlign: 'center', marginTop: 20 }}>
              <Pagination
                current={page}
                pageSize={pageSize}
                total={total}
                onChange={(p) => {
                  setPage(p);
                  fetchNews(p === 1);
                }}
                showSizeChanger={false}
                showQuickJumper
                showTotal={(total) => `共 ${total} 条`}
              />
            </div>
          )}
        </Card>
      </Col>

      {/* 右侧：热点TOP10 */}
      <Col xs={24} lg={7}>
        <Card
          title={
            <Space>
              <FireOutlined style={{ color: '#ef4444' }} />
              热点TOP10
            </Space>
          }
          style={{ marginBottom: 16 }}
        >
          {hotEvents.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无热点" />
          ) : (
            <List
              dataSource={hotEvents}
              renderItem={(event, index) => (
                <List.Item
                  style={{ padding: '12px 0', cursor: 'pointer' }}
                  onClick={() => navigate(`/news/event/${event.id}`)}
                >
                  <Space direction="vertical" size={4} style={{ width: '100%' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <span
                        style={{
                          width: 24,
                          height: 24,
                          borderRadius: '50%',
                          backgroundColor: index < 3 ? '#ef4444' : '#374151',
                          color: '#fff',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontSize: 12,
                          fontWeight: 'bold',
                        }}
                      >
                        {index + 1}
                      </span>
                      <Typography.Text strong style={{ fontSize: 14, flex: 1 }}>{event.title}</Typography.Text>
                      <Tag color={event.hotIndex > 100 ? 'red' : 'orange'}>{event.hotIndex}</Tag>
                    </div>
                    <Space size={4} wrap>
                      {event.keywords.slice(0, 2).map((kw) => (
                        <Tag key={kw}>{kw}</Tag>
                      ))}
                    </Space>
                  </Space>
                </List.Item>
              )}
            />
          )}
        </Card>

        {/* 市场情绪概览 */}
          <Card
            title={
              <Space>
                <WarningOutlined />
                市场情绪
              </Space>
            }
            style={{ marginBottom: 16 }}
          >
          {marketSentiment ? (
            <>
              {(() => {
                const total = marketSentiment.positiveCount + marketSentiment.negativeCount + marketSentiment.neutralCount;
                const positivePct = total > 0 ? marketSentiment.positiveCount / total : 0;
                const neutralPct = total > 0 ? marketSentiment.neutralCount / total : 0;
                const negativePct = total > 0 ? marketSentiment.negativeCount / total : 0;
                return (
                  <>
                    <div style={{ display: 'flex', justifyContent: 'space-around', alignItems: 'center', padding: '20px 0', gap: 16 }}>
                      <div style={{ textAlign: 'center', flex: 1 }}>
                        <div style={{ fontSize: 32, fontWeight: 'bold', color: '#22c55e' }}>{(positivePct * 100).toFixed(1)}%</div>
                        <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>正面</div>
                      </div>
                      <div style={{ textAlign: 'center', flex: 1 }}>
                        <div style={{ fontSize: 32, fontWeight: 'bold', color: '#f59e0b' }}>{(neutralPct * 100).toFixed(1)}%</div>
                        <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>中性</div>
                      </div>
                      <div style={{ textAlign: 'center', flex: 1 }}>
                        <div style={{ fontSize: 32, fontWeight: 'bold', color: '#ef4444' }}>{(negativePct * 100).toFixed(1)}%</div>
                        <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>负面</div>
                      </div>
                    </div>
                    <div style={{ height: 8, backgroundColor: '#1f2937', borderRadius: 4, overflow: 'hidden' }}>
                      <div
                        style={{
                          height: '100%',
                          width: '100%',
                          background: `linear-gradient(90deg, #22c55e ${positivePct * 100}%, #f59e0b ${(positivePct + neutralPct) * 100}%, #ef4444 100%)`,
                          borderRadius: 4,
                        }}
                      />
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 8 }}>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>今日共 {total} 条资讯</Typography.Text>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        <ClockCircleOutlined style={{ marginRight: 4 }} />
                        实时更新
                      </Typography.Text>
                    </div>
                  </>
                );
              })()}
            </>
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无情绪数据" />
          )}
        </Card>

        {/* 快捷入口 */}
        <Card
          title="快捷入口"
          size="small"
        >
          <List
            dataSource={[
              { label: '全部资讯', path: '/news', available: true },
              { label: '热点事件', path: '#', available: false },
              { label: '市场情绪', path: '#', available: false },
              { label: '智能预警', path: '#', available: false },
            ]}
            renderItem={(item) => (
              <List.Item
                style={{ padding: '8px 0', cursor: item.available ? 'pointer' : 'not-allowed', opacity: item.available ? 1 : 0.5 }}
                onClick={() => item.available && navigate(item.path)}
              >
                <Typography.Text>{item.label}</Typography.Text>
                {item.available && <ArrowRightOutlined style={{ float: 'right', color: '#9ca3af' }} />}
                {!item.available && <Tag color="default" style={{ float: 'right', fontSize: 10 }}>开发中</Tag>}
              </List.Item>
            )}
          />
        </Card>
      </Col>
    </Row>
  );
}
