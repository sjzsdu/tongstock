import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ClockCircleOutlined, FireOutlined, RestOutlined, SearchOutlined, ThunderboltOutlined, WarningOutlined } from '@ant-design/icons';
import { Button, Card, Col, Empty, Flex, Input, List, Row, Segmented, Select, Space, Spin, Tag, Typography, message } from 'antd';
import { api } from '../../api/client';
import { HOT_TAG_THRESHOLD, mergeNews, stockChips } from '../../lib/newsList';
import type { NewsSummary, HotEvent, MarketSentiment, NewsFacets } from '../../types/api';

const { Search } = Input;

/** 每页条数 */
const PAGE_SIZE = 20;

type SortBy = 'time' | 'hot';

/**
 * 信息流查询条件。筛选项与页码收敛在一个对象里：换条件时整体替换并回到
 * 第 1 页，取数 effect 只依赖这一个值，避免「条件变了但停在旧页码」。
 */
interface FeedQuery {
  page: number;
  source: string;
  newsType: string;
  sortBy: SortBy;
  keyword: string;
}

const INITIAL_QUERY: FeedQuery = { page: 1, source: '', newsType: '', sortBy: 'time', keyword: '' };

/** facets 请求失败时的兜底选项，保证筛选器不至于只剩「全部来源」 */
const FALLBACK_SOURCES = ['财联社', '证券时报', '东方财富', '21世纪经济报道', '第一财经', '新浪财经', '腾讯财经', '雪球', '巨潮资讯'];
const FALLBACK_TYPES = ['快讯', '公告', '讨论', '研报', '其他'];

const TYPE_COLORS: Record<string, string> = {
  快讯: 'gold',
  公告: 'geekblue',
  研报: 'purple',
  讨论: 'cyan',
  其他: 'default',
};

function formatTime(timeStr: string) {
  const date = new Date(timeStr);
  const hours = Math.floor((Date.now() - date.getTime()) / (1000 * 60 * 60));
  if (hours < 1) return '刚刚';
  if (hours < 24) return `${hours}小时前`;
  return date.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function getHotColor(hotScore: number) {
  if (hotScore >= 100) return 'red';
  return 'orange';
}

/** 下拉选项：名称在左、条数在右，一眼看出哪个来源是主力 */
function FacetOption({ name, count }: { name: string; count?: number }) {
  return (
    <span className="news-facet-option">
      <span>{name}</span>
      {count != null && <span className="news-facet-count">{count}</span>}
    </span>
  );
}

export default function NewsHome() {
  const navigate = useNavigate();
  const [query, setQuery] = useState<FeedQuery>(INITIAL_QUERY);
  const [searchText, setSearchText] = useState('');
  const [newsList, setNewsList] = useState<NewsSummary[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [facets, setFacets] = useState<NewsFacets | null>(null);
  const [hotEvents, setHotEvents] = useState<HotEvent[]>([]);
  const [marketSentiment, setMarketSentiment] = useState<MarketSentiment | null>(null);
  const [fetchingBrowser, setFetchingBrowser] = useState(false);
  const sentinelRef = useRef<HTMLDivElement | null>(null);

  // 取数：依赖整个 query，条件或页码变化必然重新请求。
  // 第 1 页整体替换，之后的页追加并去重。
  useEffect(() => {
    let cancelled = false;
    const isReset = query.page === 1;
    const params: { page: number; pageSize: number; sortBy: string; sources?: string; types?: string; keyword?: string } = {
      page: query.page,
      pageSize: PAGE_SIZE,
      sortBy: query.sortBy,
    };
    if (query.source) params.sources = query.source;
    if (query.newsType) params.types = query.newsType;
    if (query.keyword) params.keyword = query.keyword;

    api.newsFeed(params)
      .then((result) => {
        if (cancelled) return;
        const items = result.items || [];
        setNewsList((prev) => (isReset ? items : mergeNews(prev, items)));
        setTotal(result.total || 0);
      })
      .catch(() => {
        if (cancelled || !isReset) return;
        setNewsList([]);
        setTotal(0);
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
        setLoadingMore(false);
      });

    return () => {
      cancelled = true;
    };
  }, [query]);

  // 筛选器选项由后端下发条数分布，新增数据源时前端不用改硬编码列表
  useEffect(() => {
    let cancelled = false;
    api
      .newsFacets()
      .then((result) => {
        if (!cancelled) setFacets(result);
      })
      .catch(() => {
        if (!cancelled) setFacets(null);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    api
      .hotEvents({ limit: 10 })
      .then((result) => {
        if (!cancelled) setHotEvents(result.items || []);
      })
      .catch(() => {
        if (!cancelled) setHotEvents([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    api
      .sentimentMarket(24)
      .then((result) => {
        if (!cancelled) setMarketSentiment(result);
      })
      .catch(() => {
        if (!cancelled) setMarketSentiment(null);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const hasMore = newsList.length < total;

  // 无限滚动：sentinel 进入视口就翻页。loadingMore 期间不重复触发，
  // 否则一次滑到底会连翻好几页。
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !hasMore || loading || loadingMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries[0]?.isIntersecting) return;
        setQuery((prev) => ({ ...prev, page: prev.page + 1 }));
        setLoadingMore(true);
      },
      { rootMargin: '200px' }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasMore, loading, loadingMore]);

  /** 换筛选条件：一律回到第 1 页 */
  const applyFilter = (patch: Partial<Omit<FeedQuery, 'page'>>) => {
    setQuery((prev) => ({ ...prev, page: 1, ...patch }));
    setLoading(true);
    setLoadingMore(false);
  };

  const resetFilters = () => {
    setSearchText('');
    setQuery({ ...INITIAL_QUERY });
    setLoading(true);
    setLoadingMore(false);
  };

  const hasActiveFilter = Boolean(query.source || query.newsType || query.keyword);

  const handleFetchBrowserNews = async () => {
    setFetchingBrowser(true);
    try {
      const result = await api.newsFetchBrowser('all');
      if (result.count > 0) {
        message.success(`抓取成功，获取到 ${result.count} 条新闻`);
        applyFilter({});
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

  const sourceFacets: { name: string; count?: number }[] = facets
    ? facets.sources
    : FALLBACK_SOURCES.map((name) => ({ name }));
  const typeFacets: { name: string; count?: number }[] = facets ? facets.types : FALLBACK_TYPES.map((name) => ({ name }));

  const sourceOptions = [
    { value: '', label: '全部来源' },
    ...sourceFacets.map((f) => ({ value: f.name, label: <FacetOption name={f.name} count={f.count} /> })),
  ];
  const typeOptions = [
    { value: '', label: '全部类型' },
    ...typeFacets.map((f) => ({ value: f.name, label: <FacetOption name={f.name} count={f.count} /> })),
  ];

  const openNews = (news: NewsSummary) => {
    if (news.url) {
      window.open(news.url, '_blank');
      return;
    }
    navigate(`/news/event/${news.id}`);
  };

  return (
    <Row gutter={[24, 24]}>
      {/* 左侧：资讯信息流 */}
      <Col xs={24} lg={17}>
        {/* 筛选栏 */}
        <Card styles={{ body: { padding: 16 } }}>
          <Flex gap={12} wrap align="center">
            <Search
              placeholder="搜索标题或摘要..."
              allowClear
              value={searchText}
              onChange={(e) => {
                setSearchText(e.target.value);
                if (!e.target.value) applyFilter({ keyword: '' });
              }}
              onSearch={(value) => applyFilter({ keyword: value.trim() })}
              enterButton={<SearchOutlined />}
              style={{ width: 300 }}
            />
            <Select
              value={query.source}
              onChange={(value) => applyFilter({ source: value })}
              options={sourceOptions}
              style={{ width: 190 }}
            />
            <Select
              value={query.newsType}
              onChange={(value) => applyFilter({ newsType: value })}
              options={typeOptions}
              style={{ width: 150 }}
            />
            <Segmented
              value={query.sortBy}
              onChange={(value) => applyFilter({ sortBy: value as SortBy })}
              options={[
                { label: '最新发布', value: 'time' },
                { label: '热度排序', value: 'hot' },
              ]}
            />
            <Button icon={<RestOutlined />} onClick={resetFilters}>
              重置筛选
            </Button>
            <Button
              type="primary"
              icon={<ThunderboltOutlined />}
              loading={fetchingBrowser}
              onClick={handleFetchBrowserNews}
              style={{ marginLeft: 'auto' }}
            >
              抓取实时新闻
            </Button>
          </Flex>
        </Card>

        {/* 新闻列表 */}
        <Card
          styles={{ body: { paddingTop: 8 } }}
          title={
            <Space size={8} wrap>
              <span>资讯列表</span>
              {/* 加载中不显示条数：换筛选条件时总数还是上一次的，显示出来是误导 */}
              {!loading && (
                <Typography.Text type="secondary" style={{ fontWeight: 400, fontSize: 13 }}>
                  共 {total} 条{query.keyword ? ` · 关键词「${query.keyword}」` : ''}
                </Typography.Text>
              )}
            </Space>
          }
          extra={
            !loading && newsList.length > 0 ? (
              <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                已加载 {newsList.length} 条
              </Typography.Text>
            ) : undefined
          }
        >
          {loading ? (
            <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
              {[...Array(5)].map((_, i) => (
                <div key={i} style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <div style={{ height: 14, backgroundColor: '#1f2937', borderRadius: 4, width: 240 }} />
                  <div style={{ height: 18, backgroundColor: '#1f2937', borderRadius: 4, width: '70%' }} />
                  <div style={{ height: 14, backgroundColor: '#1f2937', borderRadius: 4, width: '45%' }} />
                </div>
              ))}
            </Space>
          ) : newsList.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={hasActiveFilter ? '没有匹配的资讯' : '暂无资讯'}>
              {hasActiveFilter && (
                <Button onClick={resetFilters} icon={<RestOutlined />}>
                  清除筛选
                </Button>
              )}
            </Empty>
          ) : (
            <List
              dataSource={newsList}
              renderItem={(news) => {
                const chips = stockChips(news);
                return (
                  <List.Item
                    key={news.id}
                    className="news-item"
                    onClick={() => openNews(news)}
                    style={{ padding: '14px 0', cursor: 'pointer' }}
                  >
                    <div style={{ width: '100%' }}>
                      <Space size={8} wrap style={{ marginBottom: 4 }}>
                        <Tag color={TYPE_COLORS[news.newsType] || 'default'} style={{ marginRight: 0 }}>
                          {news.newsType}
                        </Tag>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          {news.source}
                        </Typography.Text>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          {formatTime(news.publishTime)}
                        </Typography.Text>
                        {news.hotScore >= HOT_TAG_THRESHOLD && (
                          <Tag color={getHotColor(news.hotScore)} style={{ marginRight: 0 }}>
                            <FireOutlined style={{ marginRight: 4 }} />
                            {news.hotScore}
                          </Tag>
                        )}
                      </Space>
                      <div className="news-item-title">{news.title}</div>
                      {news.summary && <div className="news-item-summary">{news.summary}</div>}
                      {chips.length > 0 && (
                        <Space size={4} wrap style={{ marginTop: 6 }}>
                          {chips.map((code) => (
                            <Tag
                              key={code}
                              color="blue"
                              style={{ cursor: 'pointer', marginRight: 0 }}
                              onClick={(e) => {
                                e.stopPropagation();
                                navigate(`/stock/${code}`);
                              }}
                            >
                              {code}
                            </Tag>
                          ))}
                        </Space>
                      )}
                    </div>
                  </List.Item>
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
          {!loading && !hasMore && newsList.length > 0 && (
            <div style={{ textAlign: 'center', padding: '12px 0 4px' }}>
              <Typography.Text type="secondary">已加载全部 {total} 条</Typography.Text>
            </div>
          )}
          {hasMore && !loading && <div ref={sentinelRef} style={{ height: 1 }} />}
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
                      <Typography.Text strong style={{ fontSize: 14, flex: 1 }}>
                        {event.title}
                      </Typography.Text>
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
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-around',
                        alignItems: 'center',
                        padding: '20px 0',
                        gap: 16,
                      }}
                    >
                      <div style={{ textAlign: 'center', flex: 1 }}>
                        <div style={{ fontSize: 32, fontWeight: 'bold', color: '#ef4444' }}>
                          {(positivePct * 100).toFixed(1)}%
                        </div>
                        <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>正面</div>
                      </div>
                      <div style={{ textAlign: 'center', flex: 1 }}>
                        <div style={{ fontSize: 32, fontWeight: 'bold', color: '#f59e0b' }}>
                          {(neutralPct * 100).toFixed(1)}%
                        </div>
                        <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>中性</div>
                      </div>
                      <div style={{ textAlign: 'center', flex: 1 }}>
                        <div style={{ fontSize: 32, fontWeight: 'bold', color: '#22c55e' }}>
                          {(negativePct * 100).toFixed(1)}%
                        </div>
                        <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>负面</div>
                      </div>
                    </div>
                    <div style={{ height: 8, backgroundColor: '#1f2937', borderRadius: 4, overflow: 'hidden' }}>
                      <div
                        style={{
                          height: '100%',
                          width: '100%',
                          background: `linear-gradient(90deg, #ef4444 ${positivePct * 100}%, #f59e0b ${
                            (positivePct + neutralPct) * 100
                          }%, #22c55e 100%)`,
                          borderRadius: 4,
                        }}
                      />
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 8 }}>
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        今日共 {total} 条资讯
                      </Typography.Text>
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
      </Col>
    </Row>
  );
}
