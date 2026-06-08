import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ArrowRightOutlined,
  DeleteOutlined,
  HeartOutlined,
  StockOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Col,
  Empty,
  List,
  Row,
  Space,
  Skeleton,
  Statistic,
  Tag,
  Typography,
  message,
} from 'antd';
import { api } from '../api/client';
import type { Quote, WatchlistStock } from '../types/api';
import StockSearchInput from '../components/StockSearchInput';

function getValueColor(value: number) {
  if (value > 0) return '#ef4444';
  if (value < 0) return '#22c55e';
  return '#cbd5e1';
}

function formatSignedPercent(value: number) {
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`;
}

export default function Watchlist() {
  const navigate = useNavigate();
  const [watchlist, setWatchlist] = useState<WatchlistStock[]>([]);
  const [quotes, setQuotes] = useState<Record<string, Quote>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    void loadWatchlist();
  }, []);

  const rows = useMemo(() => watchlist.map((stock) => {
    const quote = quotes[stock.code];
    const change = quote ? ((quote.Price - quote.LastClose) / quote.LastClose) * 100 : 0;
    return {
      ...stock,
      quote,
      change,
    };
  }), [watchlist, quotes]);

  const loadWatchlist = async () => {
    setLoading(true);
    try {
      const saved = await api.watchlist();
      setWatchlist(saved);
      await Promise.all(saved.map(async (stock) => {
        try {
          const quote = await api.quote(stock.code);
          setQuotes((prev) => ({ ...prev, [stock.code]: quote }));
        } catch {
          // ignore single quote failure
        }
      }));
    } finally {
      setLoading(false);
    }
  };

  const addStock = async (code: string, name?: string) => {
    try {
      await api.watchlistAdd(code, name);
      void message.success(`已添加 ${code} 到自选`);
      void loadWatchlist();
    } catch (error) {
      void message.error(error instanceof Error ? error.message : '添加失败');
    }
  };

  const deleteStock = async (code: string) => {
    try {
      await api.watchlistDelete(code);
      setWatchlist((prev) => prev.filter((item) => item.code !== code));
      setQuotes((prev) => {
        const next = { ...prev };
        delete next[code];
        return next;
      });
      void message.success(`已从自选移除 ${code}`);
    } catch (error) {
      void message.error(error instanceof Error ? error.message : '删除失败');
    }
  };

  // 统计涨跌分布
  const stats = useMemo(() => {
    const up = rows.filter((r) => r.change > 0).length;
    const down = rows.filter((r) => r.change < 0).length;
    const flat = rows.filter((r) => r.change === 0).length;
    return { up, down, flat, total: rows.length };
  }, [rows]);

  return (
    <Space direction="vertical" size={24} style={{ display: 'flex' }}>
      <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.22), rgba(14,165,233,0.12))' }}>
        <Space direction="vertical" size={10} style={{ display: 'flex' }}>
          <Tag color="blue" style={{ width: 'fit-content', marginInlineEnd: 0 }}>自选股</Tag>
          <Typography.Title level={2} style={{ margin: 0 }}>
            自选股管理
          </Typography.Title>
          <Typography.Text type="secondary">
            管理您的自选股列表，实时查看行情变化，快速定位关注标的。
          </Typography.Text>
        </Space>
      </Card>

      <Card>
        <Space direction="vertical" size={16} style={{ display: 'flex' }}>
          <Space>
            <StockOutlined />
            <Typography.Text strong>添加自选</Typography.Text>
          </Space>
          <StockSearchInput
            limit={10}
            placeholder="输入股票代码、简称或拼音"
            onSelect={(match) => void addStock(match.code, match.name)}
          />
        </Space>
      </Card>

      {loading ? (
        <Skeleton active paragraph={{ rows: 8 }} title={false} />
      ) : rows.length === 0 ? (
        <Empty description="暂无自选股" image={Empty.PRESENTED_IMAGE_SIMPLE}>
          <Typography.Text type="secondary">
            使用上方搜索框添加股票到自选列表
          </Typography.Text>
        </Empty>
      ) : (
        <>
          <Row gutter={[16, 16]}>
            <Col xs={24} sm={8}>
              <Card size="small">
                <Statistic
                  title="上涨"
                  value={stats.up}
                  suffix={`/ ${stats.total}`}
                  valueStyle={{ color: '#ef4444' }}
                />
              </Card>
            </Col>
            <Col xs={24} sm={8}>
              <Card size="small">
                <Statistic
                  title="下跌"
                  value={stats.down}
                  suffix={`/ ${stats.total}`}
                  valueStyle={{ color: '#22c55e' }}
                />
              </Card>
            </Col>
            <Col xs={24} sm={8}>
              <Card size="small">
                <Statistic
                  title="平盘"
                  value={stats.flat}
                  suffix={`/ ${stats.total}`}
                  valueStyle={{ color: '#cbd5e1' }}
                />
              </Card>
            </Col>
          </Row>

          <Card
            title={
              <Space>
                <HeartOutlined />
                <span>自选列表</span>
              </Space>
            }
          >
            <List
              dataSource={rows}
              renderItem={(item) => {
                const color = getValueColor(item.change);
                return (
                  <List.Item
                    actions={[
                      <Button key="open" type="link" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${item.code}`)}>
                        查看
                      </Button>,
                      <Button key="delete" type="link" danger icon={<DeleteOutlined />} onClick={() => void deleteStock(item.code)}>
                        删除
                      </Button>,
                    ]}
                  >
                    <List.Item.Meta
                      avatar={<StockOutlined style={{ fontSize: 18, color: '#1677ff' }} />}
                      title={<Space><span>{item.quote?.Name || item.name || item.code}</span><Typography.Text type="secondary">{item.code}</Typography.Text></Space>}
                    />
                    <Space direction="vertical" size={0} style={{ alignItems: 'flex-end' }}>
                      <Typography.Text>{item.quote?.Price?.toFixed(2) ?? '--'}</Typography.Text>
                      <Typography.Text style={{ color }}>
                        {item.quote ? formatSignedPercent(item.change) : '--'}
                      </Typography.Text>
                    </Space>
                  </List.Item>
                );
              }}
            />
          </Card>
        </>
      )}
    </Space>
  );
}

