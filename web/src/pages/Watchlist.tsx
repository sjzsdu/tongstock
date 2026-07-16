import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ArrowRightOutlined,
  DeleteOutlined,
  EditOutlined,
  HeartOutlined,
  StockOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Col,
  Empty,
  Input,
  List,
  Popover,
  Row,
  Segmented,
  Select,
  Skeleton,
  Space,
  Statistic,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import { api } from '../api/client';
import type { Quote, WatchlistStock } from '../types/api';
import StockSearchInput from '../components/StockSearchInput';

const { Text } = Typography;

const GROUP_COLORS: Record<string, string> = {
  default: 'default',
  industry: 'blue',
  concept: 'green',
  custom: 'purple',
};

function getGroupColor(group: string): string {
  return GROUP_COLORS[group] ?? 'cyan';
}

function getGroupLabel(group: string): string {
  const labels: Record<string, string> = {
    default: '默认',
    industry: '行业',
    concept: '概念',
  };
  return labels[group] ?? group;
}

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
  const [groups, setGroups] = useState<{ name: string; count: number }[]>([]);
  const [activeGroup, setActiveGroup] = useState<string>('__all__');
  const [noteEditing, setNoteEditing] = useState<string | null>(null);
  const [noteDraft, setNoteDraft] = useState('');
  const [groupEditing, setGroupEditing] = useState<string | null>(null);
  const [groupDraft, setGroupDraft] = useState('');

  const loadGroups = useCallback(async () => {
    try {
      const result = await api.watchlistGroups();
      setGroups(result.groups ?? []);
    } catch {
      // ignore group load failure
    }
  }, []);

  const loadWatchlist = useCallback(async () => {
    setLoading(true);
    try {
      const groupParam = activeGroup !== '__all__' ? activeGroup : undefined;
      const saved = await api.watchlist(groupParam);
      const list = saved ?? [];
      setWatchlist(list);
      await Promise.all(list.map(async (stock) => {
        try {
          const quote = await api.quote(stock.code);
          setQuotes((prev) => ({ ...prev, [stock.code]: quote }));
        } catch {
          // ignore single quote failure
        }
      }));
      await loadGroups();
    } finally {
      setLoading(false);
    }
  }, [activeGroup, loadGroups]);

  useEffect(() => {
    void loadWatchlist();
  }, [loadWatchlist]);

  const rows = useMemo(() => (watchlist ?? []).map((stock) => {
    const quote = quotes[stock.code];
    const change = quote ? ((quote.Price - quote.LastClose) / quote.LastClose) * 100 : 0;
    return {
      ...stock,
      quote,
      change,
    };
  }), [watchlist, quotes]);

  const addStock = async (code: string, name?: string) => {
    const group = activeGroup !== '__all__' ? activeGroup : undefined;
    try {
      await api.watchlistAdd(code, name, group);
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
      void loadGroups();
    } catch (error) {
      void message.error(error instanceof Error ? error.message : '删除失败');
    }
  };

  const saveNote = async (code: string) => {
    try {
      await api.watchlistUpdateNote(code, noteDraft);
      setWatchlist((prev) => prev.map((item) =>
        item.code === code ? { ...item, note: noteDraft } : item,
      ));
      setNoteEditing(null);
      void message.success('备注已更新');
    } catch (error) {
      void message.error(error instanceof Error ? error.message : '更新失败');
    }
  };

  const saveGroup = async (code: string) => {
    const newGroup = groupDraft.trim() || 'default';
    try {
      await api.watchlistUpdateGroup(code, newGroup);
      setWatchlist((prev) => prev.map((item) =>
        item.code === code ? { ...item, group: newGroup } : item,
      ));
      setGroupEditing(null);
      void message.success('分组已更新');
      void loadGroups();
      if (activeGroup !== '__all__' && activeGroup !== newGroup) {
        void loadWatchlist();
      }
    } catch (error) {
      void message.error(error instanceof Error ? error.message : '更新失败');
    }
  };

  const stats = useMemo(() => {
    const up = rows.filter((r) => r.change > 0).length;
    const down = rows.filter((r) => r.change < 0).length;
    const flat = rows.filter((r) => r.change === 0).length;
    return { up, down, flat, total: rows.length };
  }, [rows]);

  const groupOptions = useMemo(() => {
    const existing = (groups ?? []).map((g) => g.name);
    const presets = ['default', 'industry', 'concept', 'custom'];
    const all = Array.from(new Set([...presets, ...existing]));
    return all.map((name) => ({ label: getGroupLabel(name), value: name }));
  }, [groups]);

  return (
    <Space direction="vertical" size={24} style={{ display: 'flex' }}>
      <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.22), rgba(14,165,233,0.12))' }}>
        <Space direction="vertical" size={10} style={{ display: 'flex' }}>
          <Tag color="blue" style={{ width: 'fit-content', marginInlineEnd: 0 }}>自选股</Tag>
          <Typography.Title level={2} style={{ margin: 0 }}>
            自选股管理
          </Typography.Title>
          <Typography.Text type="secondary">
            管理您的自选股列表，实时查看行情变化，按分组组织关注标的。
          </Typography.Text>
        </Space>
      </Card>

      <Card>
        <Space direction="vertical" size={16} style={{ display: 'flex' }}>
          <Space>
            <StockOutlined />
            <Typography.Text strong>添加自选{activeGroup !== '__all__' && <Tag color={getGroupColor(activeGroup)} style={{ marginInlineStart: 8 }}>{getGroupLabel(activeGroup)}</Tag>}</Typography.Text>
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
      ) : rows.length === 0 && groups.length === 0 ? (
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
            extra={
              <Segmented
                size="small"
                value={activeGroup}
                onChange={(value) => setActiveGroup(value as string)}
                options={[
                  { label: `全部 (${(groups ?? []).reduce((sum, g) => sum + g.count, 0)})`, value: '__all__' },
                  ...(groups ?? []).map((g) => ({
                    label: `${getGroupLabel(g.name)} (${g.count})`,
                    value: g.name,
                  })),
                ]}
              />
            }
          >
            {rows.length === 0 ? (
              <Empty description={activeGroup === '__all__' ? '暂无自选股' : `「${getGroupLabel(activeGroup)}」分组为空`} image={Empty.PRESENTED_IMAGE_SIMPLE} />
            ) : (
              <List
                dataSource={rows}
                renderItem={(item) => {
                  const color = getValueColor(item.change);
                  const groupName = item.group || 'default';
                  return (
                    <List.Item
                      actions={[
                        <Button key="open" type="link" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${item.code}`)}>
                          查看
                        </Button>,
                        <Popover
                          key="group"
                          trigger="click"
                          open={groupEditing === item.code}
                          onOpenChange={(open) => {
                            if (open) {
                              setGroupEditing(item.code);
                              setGroupDraft(groupName);
                            } else {
                              setGroupEditing(null);
                            }
                          }}
                          content={
                            <Space direction="vertical" size={8} style={{ width: 220 }}>
                              <Text strong>选择分组</Text>
                              <Select
                                style={{ width: '100%' }}
                                value={groupDraft}
                                onChange={setGroupDraft}
                                options={groupOptions}
                                showSearch
                              />
                              <Space>
                                <Button size="small" type="primary" onClick={() => void saveGroup(item.code)}>确定</Button>
                                <Button size="small" onClick={() => setGroupEditing(null)}>取消</Button>
                              </Space>
                            </Space>
                          }
                        >
                          <Tooltip title="修改分组">
                            <Button type="text" size="small" icon={<Tag color={getGroupColor(groupName)}>{getGroupLabel(groupName)}</Tag>} />
                          </Tooltip>
                        </Popover>,
                        <Popover
                          key="note"
                          trigger="click"
                          open={noteEditing === item.code}
                          onOpenChange={(open) => {
                            if (open) {
                              setNoteEditing(item.code);
                              setNoteDraft(item.note || '');
                            } else {
                              setNoteEditing(null);
                            }
                          }}
                          content={
                            <Space direction="vertical" size={8} style={{ width: 280 }}>
                              <Text strong>编辑备注</Text>
                              <Input.TextArea
                                value={noteDraft}
                                onChange={(e) => setNoteDraft(e.target.value)}
                                placeholder="添加备注，如：长线观察、止损位 15.00"
                                autoSize={{ minRows: 2, maxRows: 4 }}
                                onPressEnter={(e) => {
                                  if (!e.shiftKey) {
                                    e.preventDefault();
                                    void saveNote(item.code);
                                  }
                                }}
                              />
                              <Space>
                                <Button size="small" type="primary" onClick={() => void saveNote(item.code)}>保存</Button>
                                <Button size="small" onClick={() => setNoteEditing(null)}>取消</Button>
                              </Space>
                            </Space>
                          }
                        >
                          <Tooltip title={item.note || '添加备注'}>
                            <Button type="text" size="small" icon={<EditOutlined />} />
                          </Tooltip>
                        </Popover>,
                        <Button key="delete" type="link" danger icon={<DeleteOutlined />} onClick={() => void deleteStock(item.code)}>
                          删除
                        </Button>,
                      ]}
                    >
                      <List.Item.Meta
                        avatar={<StockOutlined style={{ fontSize: 18, color: '#1677ff' }} />}
                        title={
                          <Space wrap>
                            <span>{item.quote?.Name || item.name || item.code}</span>
                            <Text type="secondary">{item.code}</Text>
                            <Tag color={getGroupColor(groupName)} style={{ marginInlineEnd: 0 }}>{getGroupLabel(groupName)}</Tag>
                          </Space>
                        }
                        description={
                          item.note ? (
                            <Text type="secondary" style={{ fontSize: 13 }}>{item.note}</Text>
                          ) : null
                        }
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
        </>
      )}
    </Space>
  );
}
