import { useState } from 'react';
import { Card, Input, Select, Button, Space, List, Tag, Typography, message, Divider, Row } from 'antd';
import { RobotOutlined, ExperimentOutlined } from '@ant-design/icons';
import { api } from '../../api/client';
import type { ParadigmItem } from '../../types/api';

const { Title, Text, Paragraph } = Typography;

const hypothesisTemplates = [
  { value: 'mean_reversion', label: '均值回归：超跌反弹' },
  { value: 'momentum', label: '趋势延续：强者恒强' },
  { value: 'breakout', label: '突破策略：放量突破' },
  { value: 'volume_divergence', label: '量价背离：缩量下跌' },
  { value: 'rsi_reversal', label: 'RSI 反转：超卖反弹' },
  { value: 'custom', label: '自定义假设' },
];

export default function Hypothesis() {
  const [stockCode, setStockCode] = useState('');
  const [selectedTemplate, setSelectedTemplate] = useState<string | undefined>();
  const [loading, setLoading] = useState(false);
  const [hypothesis, setHypothesis] = useState<ParadigmItem | null>(null);

  const generate = async () => {
    if (!stockCode) {
      message.warning('请先输入股票代码');
      return;
    }
    setLoading(true);
    try {
      const resp = await api.paradigmAnalyze(stockCode);
      if (resp.paradigm) {
        setHypothesis(resp.paradigm);
        message.success('假设生成成功');
      } else {
        message.warning(resp.message || '未能生成假设');
      }
    } catch (err) {
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>假设生成</Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          基于 AI 和内部数据生成可证伪的研究假设，进入实验验证阶段
        </Paragraph>
      </div>

      <Card title="生成假设">
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Space style={{ width: '100%' }}>
            <Input
              placeholder="输入股票代码 (如 600519)"
              value={stockCode}
              onChange={(e) => setStockCode(e.target.value)}
              style={{ flex: 1 }}
              onPressEnter={generate}
            />
            <Select
              placeholder="选择假设模板"
              value={selectedTemplate}
              onChange={setSelectedTemplate}
              options={hypothesisTemplates}
              style={{ width: 180 }}
              allowClear
            />
            <Button
              type="primary"
              icon={<RobotOutlined />}
              loading={loading}
              onClick={generate}
            >
              AI 生成假设
            </Button>
          </Space>
          <Text type="secondary" style={{ fontSize: 12 }}>
            提示：选择模板引导 AI 生成特定类型的假设，或留空让 AI 自由探索
          </Text>
        </Space>
      </Card>

      {hypothesis && (
        <Card
          title={
            <Space>
              <ExperimentOutlined />
              <span>生成结果：{hypothesis.name}</span>
              <Tag color={hypothesis.side === 'buy' ? 'green' : 'red'}>
                {hypothesis.side === 'buy' ? '买入' : '卖出'}
              </Tag>
            </Space>
          }
          extra={<Tag color={hypothesis.validation?.valid ? 'green' : 'orange'}>
            {hypothesis.validation?.valid ? '结构有效' : '需要审查'}
          </Tag>}
        >
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            {/* 买入条件 */}
            {hypothesis.buy_conditions.length > 0 && (
              <div>
                <Text strong>买入条件：</Text>
                <List
                  size="small"
                  bordered
                  dataSource={hypothesis.buy_conditions}
                  renderItem={(item) => (
                    <List.Item>
                      <Tag color="blue">{item.indicator}</Tag>
                      <Text>{item.operator} {item.value}</Text>
                    </List.Item>
                  )}
                />
              </div>
            )}

            {/* 卖出条件 */}
            {hypothesis.sell_conditions?.take_profit && hypothesis.sell_conditions.take_profit.length > 0 && (
              <div>
                <Text strong>止盈条件：</Text>
                <List
                  size="small"
                  bordered
                  dataSource={hypothesis.sell_conditions.take_profit}
                  renderItem={(item) => (
                    <List.Item>
                      <Tag color="green">{item.indicator}</Tag>
                      <Text>{item.operator} {item.value}</Text>
                    </List.Item>
                  )}
                />
              </div>
            )}

            {hypothesis.sell_conditions?.stop_loss && hypothesis.sell_conditions.stop_loss.length > 0 && (
              <div>
                <Text strong>止损条件：</Text>
                <List
                  size="small"
                  bordered
                  dataSource={hypothesis.sell_conditions.stop_loss}
                  renderItem={(item) => (
                    <List.Item>
                      <Tag color="red">{item.indicator}</Tag>
                      <Text>{item.operator} {item.value}</Text>
                    </List.Item>
                  )}
                />
              </div>
            )}

            {/* 期望收益 */}
            {hypothesis.expectation && (
              <div>
                <Divider style={{ margin: '12px 0' }} />
                <Row gutter={[16, 8]}>
                  <Space>
                    <Text type="secondary">持有期:</Text>
                    <Text>{hypothesis.expectation.holding_period}</Text>
                  </Space>
                  <Space>
                    <Text type="secondary">期望收益:</Text>
                    <Text strong>{hypothesis.expectation.expected_return}</Text>
                  </Space>
                  <Space>
                    <Text type="secondary">风险收益比:</Text>
                    <Text>{hypothesis.expectation.risk_reward_ratio}</Text>
                  </Space>
                  <Space>
                    <Text type="secondary">置信度:</Text>
                    <Text>{(hypothesis.expectation.confidence * 100).toFixed(0)}%</Text>
                  </Space>
                </Row>
              </div>
            )}

            {/* 确认条件 */}
            {hypothesis.confirmations && hypothesis.confirmations.length > 0 && (
              <div>
                <Text strong>确认条件：</Text>
                <Space wrap>
                  {hypothesis.confirmations.map((c, i) => (
                    <Tag key={i} color="blue">{c}</Tag>
                  ))}
                </Space>
              </div>
            )}

            {/* 否定条件 */}
            {hypothesis.invalidations && hypothesis.invalidations.length > 0 && (
              <div>
                <Text strong>否定条件：</Text>
                <Space wrap>
                  {hypothesis.invalidations.map((c, i) => (
                    <Tag key={i} color="orange">{c}</Tag>
                  ))}
                </Space>
              </div>
            )}

            {/* 操作按钮 */}
            <Divider style={{ margin: '12px 0' }} />
            <Space>
              <Button type="primary" onClick={() => {
                window.location.href = '/research/experiment';
              }}>
                进入实验验证 →
              </Button>
              <Button onClick={generate}>重新生成</Button>
            </Space>
          </Space>
        </Card>
      )}

      {!hypothesis && (
        <Card>
          <Space direction="vertical" size="middle">
            <ExperimentOutlined style={{ fontSize: 48, color: '#bfbfbf' }} />
            <Text type="secondary">
              输入股票代码和假设模板，AI 将生成可证伪的研究假设
            </Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
              假设包含：买入/卖出条件、期望收益、确认条件和否定条件
            </Text>
          </Space>
        </Card>
      )}
    </Space>
  );
}
