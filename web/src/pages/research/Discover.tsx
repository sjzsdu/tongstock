import { useEffect, useMemo, useState } from 'react';
import { Card, Empty, Row, Col, Select, Space, Tag, Typography, message, Spin } from 'antd';
import { api } from '../../api/client';
import type { ParadigmItem, ParadigmDecisionCard, ParadigmTransitionRequest } from '../../types/api';
import DecisionCard from '../../components/research/DecisionCard';
import { Link } from 'react-router-dom';

const { Title, Text, Paragraph } = Typography;

const RELIABILITY_OPTIONS = [
  { label: '高', value: 'high' },
  { label: '中', value: 'medium' },
  { label: '低', value: 'low' },
];

const SIDE_OPTIONS = [
  { label: '全部方向', value: '' },
  { label: '多头', value: 'buy' },
  { label: '空头', value: 'sell' },
];

export default function Discover() {
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<ParadigmItem[]>([]);
  void items;
  const [cards, setCards] = useState<ParadigmDecisionCard[]>([]);
  const [side, setSide] = useState<string | undefined>(undefined);
  const [reliability, setReliability] = useState<string | undefined>(undefined);

  const filters = useMemo(
    () => ({
      review_status: 'verified',
      side: side || undefined,
      reliability: reliability || undefined,
    }),
    [side, reliability],
  );

  const load = async () => {
    setLoading(true);
    try {
      const [discover, cardsResp] = await Promise.all([
        api.paradigmDiscover(filters),
        api.paradigmDecisionCards(),
      ]);
      setItems(discover.paradigms || []);
      setCards(cardsResp.cards || []);
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [side, reliability]);

  const handleSave = async (card: ParadigmDecisionCard) => {
    try {
      await api.watchlistAdd(card.stock_code, card.stock_name, 'paradigm');
      message.success(`已将 ${card.stock_code} 加入观察`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
    }
  };

  const handleTransition = async (id: string, req: ParadigmTransitionRequest) => {
    try {
      const res = await api.paradigmTransition(id, req);
      message.success(`已变更为 ${res.transition.to}`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
      throw err;
    }
  };

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card
        bordered={false}
        style={{ background: 'linear-gradient(135deg, rgba(82,196,26,0.22), rgba(22,119,255,0.12))' }}
      >
        <Space direction="vertical" size={10} style={{ display: 'flex' }}>
          <Tag color="green" style={{ width: 'fit-content', marginInlineEnd: 0 }}>发现</Tag>
          <Title level={3} style={{ margin: 0 }}>已晋级范式</Title>
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>
            只有通过阶段门验证的范式才会出现在此, 可供产品侧决策引用。未晋级候选请前往
            <Link to="/research/candidates" style={{ marginInlineStart: 4 }}>候选评估</Link>。
          </Paragraph>
        </Space>
      </Card>

      <Card
        title={
          <Space>
            <span>决策卡</span>
            <Text type="secondary" style={{ fontSize: 12 }}>
              共 {cards.length} 张
            </Text>
          </Space>
        }
        extra={
          <Space>
            <Select
              placeholder="方向"
              allowClear
              options={SIDE_OPTIONS.slice(1)}
              value={side}
              onChange={setSide}
              style={{ width: 120 }}
            />
            <Select
              placeholder="可靠性"
              allowClear
              options={RELIABILITY_OPTIONS}
              value={reliability}
              onChange={setReliability}
              style={{ width: 120 }}
            />
          </Space>
        }
      >
        <Spin spinning={loading}>
          {cards.length === 0 ? (
            <Empty
              description="暂无已晋级范式"
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            >
              <Link to="/research/hypothesis">去生成研究假设</Link>
            </Empty>
          ) : (
            <Row gutter={[16, 16]}>
              {cards
                .filter((c) => (side ? c.side === side : true))
                .filter((c) => (reliability ? c.reliability === reliability : true))
                .map((card) => (
                  <Col xs={24} sm={12} xl={12} key={card.paradigm_id}>
                    <DecisionCard
                      card={card}
                      onViewEvidence={() => {
                        message.info(`查看 ${card.paradigm_name} 的证据卡`);
                      }}
                      onViewLineage={() => {
                        message.info(`查看 ${card.paradigm_name} 的研究血缘`);
                      }}
                      onSave={() => void handleSave(card)}
                      onTransition={(id, req) => handleTransition(id, req)}
                    />
                  </Col>
                ))}
            </Row>
          )}
        </Spin>
      </Card>
    </Space>
  );
}
