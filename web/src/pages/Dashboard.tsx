import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ArrowRightOutlined,
  ClockCircleOutlined,
  DeleteOutlined,
  RiseOutlined,
  SearchOutlined,
  StockOutlined,
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
import type { HistoryStock, Quote } from '../types/api';
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
  const [history, setHistory] = useState<HistoryStock[]>([]);
  const [historyQuotes, setHistoryQuotes] = useState<Record<string, Quote>>({});
  const [loadingIndices, setLoadingIndices] = useState(true);
  const [loadingHistory, setLoadingHistory] = useState(true);

  useEffect(() => {
    void loadDashboardData();
  }, []);

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
    setLoadingHistory(true);

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

    try {
      const saved = await api.history();
      setHistory(saved);
      await Promise.all(saved.map(async (stock) => {
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
      <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.22), rgba(14,165,233,0.12))' }}>
        <Row gutter={[24, 24]} align="middle">
          <Col xs={24} xl={15}>
            <Space direction="vertical" size={10} style={{ display: 'flex' }}>
              <Tag color="blue" style={{ width: 'fit-content', marginInlineEnd: 0 }}>TongStock 工作台</Tag>
              <Typography.Title level={2} style={{ margin: 0 }}>
                市场总览
              </Typography.Title>
              <Typography.Text type="secondary">
                查看主要指数表现、最近分析记录与快速入口，作为日常盯盘与个股分析的起点。
              </Typography.Text>
            </Space>
          </Col>
          <Col xs={24} xl={9}>
            <Card size="small" style={{ background: 'rgba(15, 23, 42, 0.45)', borderColor: 'rgba(148, 163, 184, 0.18)' }}>
              <Space direction="vertical" size={16} style={{ display: 'flex' }}>
                <Space>
                  <SearchOutlined />
                  <Typography.Text strong>快速分析</Typography.Text>
                </Space>
                <StockSearchInput
                  limit={10}
                  placeholder="输入股票代码、简称或拼音"
                  onSelect={(match) => navigate(`/stock/${match.code}`)}
                />
                <Typography.Text type="secondary">
                  输入股票代码、简称或拼音，直接进入指标分析页面。
                </Typography.Text>
              </Space>
            </Card>
          </Col>
        </Row>
      </Card>

      <Row gutter={[16, 16]}>
        {indices.map((idx) => {
          const color = getValueColor(idx.change);
          return (
            <Col xs={24} sm={12} lg={6} key={idx.code}>
              <Card>
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

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={15}>
          <Card
            title={<Space><ClockCircleOutlined /><span>历史个股</span></Space>}
            extra={<Button type="link" onClick={() => navigate('/stock/choose')}>新增分析</Button>}
          >
            {loadingHistory ? (
              <Skeleton active paragraph={{ rows: 6 }} title={false} />
            ) : historyRows.length === 0 ? (
              <Empty description="暂无历史个股" />
            ) : (
              <List
                dataSource={historyRows}
                renderItem={(item) => {
                  const color = getValueColor(item.change);
                  return (
                    <List.Item
									  actions={[
										<Button key="open" type="link" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${item.code}`)}>
										  查看
										</Button>,
										<Button key="delete" type="link" danger icon={<DeleteOutlined />} onClick={() => void deleteHistoryStock(item.code)}>
										  删除
										</Button>,
									  ]}
                    >
                      <List.Item.Meta
                        avatar={<StockOutlined style={{ fontSize: 18, color: '#1677ff' }} />}
                        title={<Space><span>{item.quote?.Name || item.name || item.code}</span><Typography.Text type="secondary">{item.code}</Typography.Text></Space>}
                        description={item.analyzed_at ? `最近分析：${formatDateTime(item.analyzed_at)}` : '已加入历史记录'}
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
            )}
          </Card>
        </Col>
        <Col xs={24} xl={9}>
          <Card>
            <Space direction="vertical" size={18} style={{ display: 'flex' }}>
              <Space>
                <StockOutlined />
                <Typography.Text strong>使用提示</Typography.Text>
              </Space>
              <Row gutter={[12, 12]}>
                <Col span={24}>
                  <Card size="small">
                    <Typography.Text strong>1. 搜索个股</Typography.Text>
                    <div><Typography.Text type="secondary">支持代码、名称、拼音和首字母模糊匹配。</Typography.Text></div>
                  </Card>
                </Col>
                <Col span={24}>
                  <Card size="small">
                    <Typography.Text strong>2. 查看指标</Typography.Text>
                    <div><Typography.Text type="secondary">进入个股详情页后可切换 K 线、财务、公司、分红和分时视图。</Typography.Text></div>
                  </Card>
                </Col>
                <Col span={24}>
                  <Card size="small">
                    <Typography.Text strong>3. 跟踪历史</Typography.Text>
                    <div><Typography.Text type="secondary">分析过的个股会自动加入历史记录，方便快速回看。</Typography.Text></div>
                  </Card>
                </Col>
              </Row>
            </Space>
          </Card>
        </Col>
      </Row>
    </Space>
  );
}
