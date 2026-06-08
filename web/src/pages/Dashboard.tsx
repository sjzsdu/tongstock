import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ArrowRightOutlined,
  ClockCircleOutlined,
  DeleteOutlined,
  FundOutlined,
  HeartOutlined,
  RadarChartOutlined,
  RiseOutlined,
  SearchOutlined,
  StockOutlined,
  TrophyOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Col,
  Empty,
  List,
  Row,
  Skeleton,
  Space,
  Statistic,
  Tag,
  Typography,
  message,
} from 'antd';
import { api } from '../api/client';
import type { HistoryStock, Quote, WatchlistStock } from '../types/api';
import StockSearchInput from '../components/StockSearchInput';
import { formatDateTime } from '../lib/datetime';

const INDICES = [
  { code: '999999', name: '上证指数' },
  { code: '399001', name: '深证成指' },
  { code: '399006', name: '创业板指' },
  { code: '399300', name: '沪深300' },
];

type IndexRow = (typeof INDICES)[number] & {
  last: { Close: number } | null;
  change: number;
};

function getValueColor(value: number) {
  if (value > 0) return '#ef4444';
  if (value < 0) return '#22c55e';
  return '#cbd5e1';
}

function formatSignedPercent(value: number) {
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`;
}

export default function Dashboard() {
  const navigate = useNavigate();
  const [indices, setIndices] = useState<IndexRow[]>(() => INDICES.map((item) => ({ ...item, last: null, change: 0 })));
  const [watchlist, setWatchlist] = useState<WatchlistStock[]>([]);
  const [watchlistQuotes, setWatchlistQuotes] = useState<Record<string, Quote>>({});
  const [history, setHistory] = useState<HistoryStock[]>([]);
  const [historyQuotes, setHistoryQuotes] = useState<Record<string, Quote>>({});
  const [loadingIndices, setLoadingIndices] = useState(true);
  const [loadingWatchlist, setLoadingWatchlist] = useState(true);
  const [loadingHistory, setLoadingHistory] = useState(true);

  useEffect(() => {
    void loadDashboardData();
  }, []);

  const watchlistRows = useMemo(() => watchlist.map((stock) => {
    const quote = watchlistQuotes[stock.code];
    const change = quote ? ((quote.Price - quote.LastClose) / quote.LastClose) * 100 : 0;
    return {
      ...stock,
      quote,
      change,
    };
  }), [watchlist, watchlistQuotes]);

  const historyRows = useMemo(() => history.map((stock) => {
    const quote = historyQuotes[stock.code];
    const change = quote ? ((quote.Price - quote.LastClose) / quote.LastClose) * 100 : 0;
    return {
      ...stock,
      quote,
      change,
    };
  }), [history, historyQuotes]);

  const loadDashboardData = async () => {
    setLoadingIndices(true);
    setLoadingWatchlist(true);
    setLoadingHistory(true);

    // 加载指数数据
    const indexResults = await Promise.all(
      INDICES.map(async (idx) => {
        try {
          const bars = await api.index(idx.code, 'day');
          const last = bars?.[bars.length - 1] ?? null;
          const prev = bars?.[bars.length - 2];
          const change = last && prev ? ((last.Close - prev.Close) / prev.Close) * 100 : 0;
          return { ...idx, last, change };
        } catch {
          return { ...idx, last: null, change: 0 };
        }
      }),
    );
    setIndices(indexResults);
    setLoadingIndices(false);

    // 加载自选股
    try {
      const savedWatchlist = await api.watchlist();
      setWatchlist(savedWatchlist);
      await Promise.all(savedWatchlist.map(async (stock) => {
        try {
          const quote = await api.quote(stock.code);
          setWatchlistQuotes((prev) => ({ ...prev, [stock.code]: quote }));
        } catch {
          // ignore single quote failure
        }
      }));
    } finally {
      setLoadingWatchlist(false);
    }

    // 加载历史记录
    try {
      const savedHistory = await api.history();
      setHistory(savedHistory);
      await Promise.all(savedHistory.map(async (stock) => {
        try {
          const quote = await api.quote(stock.code);
          setHistoryQuotes((prev) => ({ ...prev, [stock.code]: quote }));
        } catch {
          // ignore single quote failure
        }
      }));
    } finally {
      setLoadingHistory(false);
    }
  };

  const deleteWatchlistStock = async (code: string) => {
    try {
      await api.watchlistDelete(code);
      setWatchlist((prev) => prev.filter((item) => item.code !== code));
      setWatchlistQuotes((prev) => {
        const next = { ...prev };
        delete next[code];
        return next;
      });
      void message.success(`已从自选移除 ${code}`);
    } catch (error) {
      void message.error(error instanceof Error ? error.message : '删除失败');
    }
  };

  const deleteHistoryStock = async (code: string) => {
    try {
      await api.historyDelete(code);
      setHistory((prev) => prev.filter((item) => item.code !== code));
      setHistoryQuotes((prev) => {
        const next = { ...prev };
        delete next[code];
        return next;
      });
      void message.success(`已删除 ${code}`);
    } catch (error) {
      void message.error(error instanceof Error ? error.message : '删除失败');
    }
  };

  return (
    <Space direction="vertical" size={24} style={{ display: 'flex' }}>
      {/* 查公司 - 快速搜索入口 */}
      <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.22), rgba(14,165,233,0.12))' }}>
        <Row gutter={[24, 24]} align="middle">
          <Col xs={24} xl={15}>
            <Space direction="vertical" size={10} style={{ display: 'flex' }}>
              <Tag color="blue" style={{ width: 'fit-content', marginInlineEnd: 0 }}>TongStock 工作台</Tag>
              <Typography.Title level={2} style={{ margin: 0 }}>
                轻量投研工作台
              </Typography.Title>
              <Typography.Text type="secondary">
                场景化入口：看市场、找机会、跟自选、查公司、做复盘，快速定位投资机会。
              </Typography.Text>
            </Space>
          </Col>
          <Col xs={24} xl={9}>
            <Card size="small" style={{ background: 'rgba(15, 23, 42, 0.45)', borderColor: 'rgba(148, 163, 184, 0.18)' }}>
              <Space direction="vertical" size={16} style={{ display: 'flex' }}>
                <Space>
                  <SearchOutlined />
                  <Typography.Text strong>查公司 - 快速分析</Typography.Text>
                </Space>
                <StockSearchInput
                  limit={10}
                  placeholder="输入股票代码、简称或拼音"
                  onSelect={(match) => navigate(`/stock/${match.code}`)}
                />
                <Typography.Text type="secondary">
                  输入股票代码、简称或拼音，直接进入个股分析页面。
                </Typography.Text>
              </Space>
            </Card>
          </Col>
        </Row>
      </Card>

      {/* 看市场 - 指数总览 */}
      <div>
        <Typography.Title level={4} style={{ marginBottom: 12 }}>
          <RiseOutlined style={{ marginRight: 8 }} />
          看市场
        </Typography.Title>
        <Row gutter={[16, 16]}>
          {indices.map((idx) => {
            const color = getValueColor(idx.change);
            return (
              <Col xs={24} sm={12} lg={6} key={idx.code}>
                <Card hoverable onClick={() => navigate(`/stock/${idx.code}`)}>
                  {loadingIndices && !idx.last ? (
                    <Skeleton active paragraph={{ rows: 2 }} title={false} />
                  ) : idx.last ? (
                    <Space direction="vertical" size={8} style={{ display: 'flex' }}>
                      <Typography.Text type="secondary">{idx.name}</Typography.Text>
                      <Statistic
                        value={idx.last.Close}
                        precision={2}
                        valueStyle={{ color }}
                        prefix={<RiseOutlined />}
                      />
                      <Tag color={idx.change >= 0 ? 'red' : 'green'} style={{ width: 'fit-content' }}>
                        {formatSignedPercent(idx.change)}
                      </Tag>
                    </Space>
                  ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="数据加载失败" />
                  )}
                </Card>
              </Col>
            );
          })}
        </Row>
      </div>

      {/* 场景化入口区域 */}
      <Row gutter={[16, 16]}>
        {/* 找机会 */}
        <Col xs={24} lg={8}>
          <Card
            title={
              <Space>
                <RadarChartOutlined />
                <span>找机会</span>
              </Space>
            }
            extra={<Button type="link" onClick={() => navigate('/screen')}>信号筛选</Button>}
          >
            <Space direction="vertical" size={12} style={{ display: 'flex' }}>
              <Card
                size="small"
                hoverable
                style={{ cursor: 'pointer' }}
                onClick={() => navigate('/screen')}
              >
                <Space>
                  <TrophyOutlined style={{ color: '#1677ff' }} />
                  <Typography.Text strong>信号筛选</Typography.Text>
                </Space>
                <div><Typography.Text type="secondary">按技术指标信号筛选股票，发现金叉、超卖等机会。</Typography.Text></div>
              </Card>
              <Card
                size="small"
                hoverable
                style={{ cursor: 'pointer' }}
                onClick={() => navigate('/blocks')}
              >
                <Space>
                  <FundOutlined style={{ color: '#1677ff' }} />
                  <Typography.Text strong>板块热点</Typography.Text>
                </Space>
                <div><Typography.Text type="secondary">查看行业板块、概念板块涨跌排行，把握市场热点。</Typography.Text></div>
              </Card>
            </Space>
          </Card>
        </Col>

        {/* 跟自选 */}
        <Col xs={24} lg={8}>
          <Card
            title={
              <Space>
                <HeartOutlined />
                <span>跟自选</span>
              </Space>
            }
            extra={<Button type="link" onClick={() => navigate('/watchlist')}>管理</Button>}
          >
            {loadingWatchlist ? (
              <Skeleton active paragraph={{ rows: 4 }} title={false} />
            ) : watchlistRows.length === 0 ? (
              <Empty description="暂无自选股" image={Empty.PRESENTED_IMAGE_SIMPLE}>
                <Button type="primary" onClick={() => navigate('/stock/choose')}>
                  添加自选
                </Button>
              </Empty>
            ) : (
              <List
                dataSource={watchlistRows.slice(0, 5)}
                renderItem={(item) => {
                  const color = getValueColor(item.change);
                  return (
                    <List.Item
                      actions={[
                        <Button key="open" type="link" size="small" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${item.code}`)} />,
                        <Button key="delete" type="link" size="small" danger icon={<DeleteOutlined />} onClick={() => void deleteWatchlistStock(item.code)} />,
                      ]}
                    >
                      <List.Item.Meta
                        avatar={<StockOutlined style={{ fontSize: 16, color: '#1677ff' }} />}
                        title={<Space size={4}><span>{item.quote?.Name || item.name || item.code}</span><Typography.Text type="secondary" style={{ fontSize: 12 }}>{item.code}</Typography.Text></Space>}
                      />
                      <Space direction="vertical" size={0} style={{ alignItems: 'flex-end' }}>
                        <Typography.Text style={{ fontSize: 14 }}>{item.quote?.Price?.toFixed(2) ?? '--'}</Typography.Text>
                        <Typography.Text style={{ color, fontSize: 12 }}>
                          {item.quote ? formatSignedPercent(item.change) : '--'}
                        </Typography.Text>
                      </Space>
                    </List.Item>
                  );
                }}
              />
            )}
          </Card>
        </Col>

        {/* 做复盘 */}
        <Col xs={24} lg={8}>
          <Card
            title={
              <Space>
                <ClockCircleOutlined />
                <span>做复盘</span>
              </Space>
            }
            extra={<Button type="link" onClick={() => navigate('/stock/choose')}>新增分析</Button>}
          >
            {loadingHistory ? (
              <Skeleton active paragraph={{ rows: 4 }} title={false} />
            ) : historyRows.length === 0 ? (
              <Empty description="暂无历史记录" image={Empty.PRESENTED_IMAGE_SIMPLE}>
                <Button type="primary" onClick={() => navigate('/stock/choose')}>
                  开始分析
                </Button>
              </Empty>
            ) : (
              <List
                dataSource={historyRows.slice(0, 5)}
                renderItem={(item) => {
                  const color = getValueColor(item.change);
                  return (
                    <List.Item
                      actions={[
                        <Button key="open" type="link" size="small" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${item.code}`)} />,
                        <Button key="delete" type="link" size="small" danger icon={<DeleteOutlined />} onClick={() => void deleteHistoryStock(item.code)} />,
                      ]}
                    >
                      <List.Item.Meta
                        avatar={<StockOutlined style={{ fontSize: 16, color: '#1677ff' }} />}
                        title={<Space size={4}><span>{item.quote?.Name || item.name || item.code}</span><Typography.Text type="secondary" style={{ fontSize: 12 }}>{item.code}</Typography.Text></Space>}
                        description={item.analyzed_at ? formatDateTime(item.analyzed_at) : ''}
                      />
                      <Space direction="vertical" size={0} style={{ alignItems: 'flex-end' }}>
                        <Typography.Text style={{ fontSize: 14 }}>{item.quote?.Price?.toFixed(2) ?? '--'}</Typography.Text>
                        <Typography.Text style={{ color, fontSize: 12 }}>
                          {item.quote ? formatSignedPercent(item.change) : '--'}
                        </Typography.Text>
                      </Space>
                    </List.Item>
                  );
                }}
              />
            )}
          </Card>
        </Col>
      </Row>
    </Space>
  );
}