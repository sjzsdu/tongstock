import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, Col, Empty, List, Row, Space, Typography } from 'antd';
import StockSearchInput from '../../components/StockSearchInput';
import { api } from '../../api/client';
import type { HistoryStock, WatchlistStock } from '../../types/api';

const HISTORY_LIMIT = 10;

export default function StockChoose() {
  const navigate = useNavigate();
  const [history, setHistory] = useState<HistoryStock[]>([]);
  const [watchlist, setWatchlist] = useState<WatchlistStock[]>([]);
  // 挂载即拉取，初始就是加载中，避免在 effect 里同步 setState。
  const [loadingHistory, setLoadingHistory] = useState(true);
  const [loadingWatchlist, setLoadingWatchlist] = useState(true);

  useEffect(() => {
    let cancelled = false;
    api.history()
      .then((items) => {
        if (cancelled) return;
        // 数据库层不保证顺序，这里按最近分析时间倒序，最新浏览的排最前。
        const sorted = [...items].sort((a, b) => (a.analyzed_at < b.analyzed_at ? 1 : -1));
        setHistory(sorted.slice(0, HISTORY_LIMIT));
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setLoadingHistory(false);
      });

    api.watchlist()
      .then((items) => {
        if (!cancelled) setWatchlist(items);
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setLoadingWatchlist(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const recentItems = useMemo(
    () =>
      history.map((item) => ({
        key: `history-${item.code}`,
        code: item.code,
        name: item.name?.trim() || item.code,
        path: `/stock/${item.code}`,
      })),
    [history],
  );

  const watchlistItems = useMemo(
    () =>
      watchlist.map((item) => ({
        key: `watch-${item.code}`,
        code: item.code,
        name: item.name?.trim() || item.code,
        path: `/stock/${item.code}`,
      })),
    [watchlist],
  );

  const renderList = (items: { key: string; code: string; name: string; path: string }[], loading: boolean) => {
    if (loading) {
      return <List size="small" loading dataSource={[{ key: 'loading' }]} renderItem={() => <List.Item />} />;
    }
    if (items.length === 0) {
      return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无记录" />;
    }
    return (
      <List
        size="small"
        dataSource={items}
        renderItem={(item) => (
          <List.Item
            style={{ cursor: 'pointer' }}
            onClick={() => navigate(item.path)}
          >
            <Space size={8}>
              <Typography.Text code>{item.code}</Typography.Text>
              <Typography.Text ellipsis style={{ maxWidth: 180 }}>{item.name}</Typography.Text>
            </Space>
          </List.Item>
        )}
      />
    );
  };

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 'calc(100vh - 180px)' }}>
      <Card style={{ width: '100%', maxWidth: 720 }}>
        <Space direction="vertical" size={24} style={{ display: 'flex' }}>
          <div style={{ textAlign: 'center' }}>
            <Typography.Title level={2}>个股分析</Typography.Title>
            <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
              输入股票代码、简称、拼音或首字母，快速定位个股并进入分析页面。
            </Typography.Paragraph>
          </div>

          <StockSearchInput
            autoFocus
            limit={12}
            placeholder="输入股票代码、名称或拼音搜索..."
            onSelect={(match) => navigate(`/stock/${match.code}`)}
          />

          <Space direction="vertical" size={4} style={{ textAlign: 'center', display: 'flex' }}>
            <Typography.Text type="secondary">支持股票代码、简称、拼音和首字母搜索</Typography.Text>
            <Typography.Text type="secondary">当存在多个匹配项时，请先从候选列表中选择</Typography.Text>
          </Space>

          <Row gutter={24}>
            <Col xs={24} sm={12}>
              <Card
                size="small"
                title="最近浏览"
                styles={{ body: { maxHeight: 260, overflow: 'auto' } }}
              >
                {renderList(recentItems, loadingHistory)}
              </Card>
            </Col>
            <Col xs={24} sm={12}>
              <Card
                size="small"
                title="自选股"
                styles={{ body: { maxHeight: 260, overflow: 'auto' } }}
              >
                {renderList(watchlistItems, loadingWatchlist)}
              </Card>
            </Col>
          </Row>
        </Space>
      </Card>
    </div>
  );
}
