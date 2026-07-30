import { useState, useCallback } from 'react';
import {
  Card, Input, Select, Button, Space, Tag, Typography, message,
  Form, Steps, Alert, Row, Col, Divider, Progress, Spin, Tooltip
} from 'antd';
import {
  EditOutlined, CheckCircleOutlined, SafetyOutlined, WarningOutlined,
  PlayCircleOutlined, RocketOutlined, DatabaseOutlined, DollarOutlined,
  StockOutlined, ExperimentOutlined, InfoCircleOutlined,
  LoadingOutlined
} from '@ant-design/icons';
import { api } from '../../api/client';
import type { ParadigmItem, HypothesisPreviewResponse } from '../../types/api';

const { Title, Text, Paragraph } = Typography;

const indicatorOptions = [
  { value: 'MA5', label: 'MA5 (5日均线)' },
  { value: 'MA10', label: 'MA10 (10日均线)' },
  { value: 'MA20', label: 'MA20 (20日均线)' },
  { value: 'MA60', label: 'MA60 (60日均线)' },
  { value: 'RSI6', label: 'RSI6 (6日RSI)' },
  { value: 'RSI14', label: 'RSI14 (14日RSI)' },
  { value: 'MACD.DIF', label: 'MACD.DIF' },
  { value: 'MACD.DEA', label: 'MACD.DEA' },
  { value: 'MACD.HIST', label: 'MACD.HIST' },
  { value: 'volume', label: '成交量' },
  { value: 'close', label: '收盘价' },
  { value: 'turnover_rate', label: '换手率' },
  { value: 'boll_upper', label: '布林上轨' },
  { value: 'boll_lower', label: '布林下轨' },
];

const operatorOptions = [
  { value: 'gt', label: '大于 ( > )' },
  { value: 'lt', label: '小于 ( < )' },
  { value: 'gte', label: '大于等于 ( >= )' },
  { value: 'lte', label: '小于等于 ( <= )' },
  { value: 'between', label: '介于 ( A-B )' },
  { value: 'cross_up', label: '上穿' },
  { value: 'cross_down', label: '下穿' },
  { value: 'describe', label: '描述性' },
];

const holdingPeriodOptions = [
  { value: '5日', label: '5 日 (短线)' },
  { value: '10日', label: '10 日 (中短线)' },
  { value: '20日', label: '20 日 (中线)' },
  { value: '60日', label: '60 日 (长线)' },
];

const featureOptions = [
  { value: 'trend', label: '趋势跟踪' },
  { value: 'mean_reversion', label: '均值回归' },
  { value: 'breakout', label: '突破' },
  { value: 'breakdown', label: '跌破' },
  { value: 'momentum', label: '动量' },
  { value: 'reversal', label: '反转' },
  { value: 'range', label: '区间震荡' },
  { value: 'volume', label: '成交量' },
  { value: 'volatility', label: '波动率' },
  { value: 'sector', label: '板块轮动' },
  { value: 'event_driven', label: '事件驱动' },
  { value: 'mean_reversion_short', label: '短线超跌反弹' },
];

const baselineOptions = [
  { value: 'CSI300', label: '沪深300' },
  { value: 'CSI500', label: '中证500' },
  { value: 'CSI1000', label: '中证1000' },
  { value: 'SZ_COMPONENT', label: '深证成指' },
  { value: 'SSE50', label: '上证50' },
  { value: 'SSE180', label: '上证180' },
  { value: 'custom', label: '自定义基准' },
];

interface Condition {
  indicator: string;
  operator: string;
  value: string;
}

interface HypothesisState {
  name: string;
  side: 'buy' | 'sell';
  stockCode: string;
  stockName: string;
  logic: string;
  rationale: string;
  features: string[];
  baseline: string;
  buyConditions: Condition[];
  sellTP: Condition[];
  sellSL: Condition[];
  confirmations: string[];
  invalidations: string[];
  holdingPeriod: string;
  expectedReturn: string;
  riskRewardRatio: string;
  confidence: number;
}

const defaultState: HypothesisState = {
  name: '',
  side: 'buy',
  stockCode: '',
  stockName: '',
  logic: '',
  rationale: '',
  features: [],
  baseline: '',
  buyConditions: [],
  sellTP: [],
  sellSL: [],
  confirmations: [],
  invalidations: [],
  holdingPeriod: '10日',
  expectedReturn: '5-10%',
  riskRewardRatio: '2:1',
  confidence: 0.6,
};

export default function Hypothesis() {
  const [step, setStep] = useState(0);
  const [state, setState] = useState<HypothesisState>(defaultState);
  const [, setErrors] = useState<string[]>([]);
  const [created, setCreated] = useState<ParadigmItem | null>(null);
  const [creating, setCreating] = useState(false);
  const [previewData, setPreviewData] = useState<HypothesisPreviewResponse | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  const steps = [
    { title: '基础信息', icon: <EditOutlined /> },
    { title: '买入触发', icon: <RocketOutlined /> },
    { title: '卖出/失效', icon: <SafetyOutlined /> },
    { title: '特征/预期', icon: <ExperimentOutlined /> },
    { title: '预览创建', icon: <PlayCircleOutlined /> },
  ];

  // --- Preview fetching ---
  const fetchPreview = useCallback(async (s: HypothesisState) => {
    if (!s.stockCode || !s.name || s.buyConditions.length === 0) {
      setPreviewData(null);
      return;
    }
    setPreviewLoading(true);
    try {
      const data = await api.paradigmPreview({
        name: s.name,
        side: s.side,
        stock_code: s.stockCode,
        stock_name: s.stockName,
        rationale: s.rationale,
        logic: s.logic,
        features: s.features,
        baseline: s.baseline,
        buy_conditions: s.buyConditions.map(c => ({ indicator: c.indicator, operator: c.operator, value: c.value })),
        sell_conditions: {
          take_profit: s.sellTP.map(c => ({ indicator: c.indicator, operator: c.operator, value: c.value })),
          stop_loss: s.sellSL.map(c => ({ indicator: c.indicator, operator: c.operator, value: c.value })),
        },
        confirmations: s.confirmations,
        invalidations: s.invalidations,
        expectation: {
          holding_period: s.holdingPeriod,
          expected_return: s.expectedReturn,
          risk_reward_ratio: s.riskRewardRatio,
          confidence: s.confidence,
        },
      });
      setPreviewData(data);
    } catch {
      setPreviewData(null);
    } finally {
      setPreviewLoading(false);
    }
  }, []);

  // Auto-fetch preview when reaching step 4
  const fetchPreviewDebounced = useCallback((s: HypothesisState) => {
    fetchPreview(s);
  }, [fetchPreview]);

  // --- 步骤 1: 基础信息 ---
  const renderStep1 = () => (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Form layout="vertical">
        <Row gutter={24}>
          <Col xs={24} md={12}>
            <Form.Item label="假设名称" required>
              <Input
                placeholder="如: 60日均线多头排列突破"
                value={state.name}
                onChange={(e) => setState({ ...state, name: e.target.value })}
              />
            </Form.Item>
          </Col>
          <Col xs={24} md={6}>
            <Form.Item label="方向" required>
              <Select
                value={state.side}
                onChange={(v) => setState({ ...state, side: v })}
                options={[
                  { value: 'buy', label: '买入 (做多)' },
                  { value: 'sell', label: '卖出 (做空)' },
                ]}
              />
            </Form.Item>
          </Col>
          <Col xs={24} md={6}>
            <Form.Item label="目标证券代码" required>
              <Input
                placeholder="如: 600519"
                value={state.stockCode}
                onChange={(e) => setState({ ...state, stockCode: e.target.value })}
              />
            </Form.Item>
          </Col>
        </Row>

        <Row gutter={24}>
          <Col xs={24} md={12}>
            <Form.Item label="证券名称 (可选)">
              <Input
                placeholder="如: 贵州茅台"
                value={state.stockName}
                onChange={(e) => setState({ ...state, stockName: e.target.value })}
              />
            </Form.Item>
          </Col>
          <Col xs={24} md={6}>
            <Form.Item label="持有周期">
              <Select
                value={state.holdingPeriod}
                onChange={(v) => setState({ ...state, holdingPeriod: v })}
                options={holdingPeriodOptions}
              />
            </Form.Item>
          </Col>
          <Col xs={24} md={6}>
            <Form.Item label={
              <Space>
                <span>对照基线</span>
                <Tooltip title="用于与策略收益对比的基准指数">
                  <InfoCircleOutlined />
                </Tooltip>
              </Space>
            }>
              <Select
                placeholder="选择对照基线"
                value={state.baseline || undefined}
                onChange={(v) => setState({ ...state, baseline: v })}
                options={baselineOptions}
                allowClear
              />
            </Form.Item>
          </Col>
        </Row>

        <Form.Item label="经济/行为逻辑 (为什么这个策略可能有效)">
          <Input.TextArea
            rows={3}
            placeholder="如: 多头排列代表趋势向上，突破关键阻力意味着新一段行情开始……"
            value={state.logic}
            onChange={(e) => setState({ ...state, logic: e.target.value })}
          />
        </Form.Item>

        <Form.Item label="假设描述/理由 (可选)">
          <Input.TextArea
            rows={2}
            placeholder="补充说明……"
            value={state.rationale}
            onChange={(e) => setState({ ...state, rationale: e.target.value })}
          />
        </Form.Item>
      </Form>
    </Space>
  );

  // --- 步骤 2: 买入触发条件 ---
  const addCondition = (list: 'buy' | 'tp' | 'sl') => {
    const cond: Condition = { indicator: '', operator: '', value: '' };
    if (list === 'buy') setState({ ...state, buyConditions: [...state.buyConditions, cond] });
    else if (list === 'tp') setState({ ...state, sellTP: [...state.sellTP, cond] });
    else setState({ ...state, sellSL: [...state.sellSL, cond] });
  };

  const removeCondition = (list: 'buy' | 'tp' | 'sl', idx: number) => {
    if (list === 'buy') setState({ ...state, buyConditions: state.buyConditions.filter((_, i) => i !== idx) });
    else if (list === 'tp') setState({ ...state, sellTP: state.sellTP.filter((_, i) => i !== idx) });
    else setState({ ...state, sellSL: state.sellSL.filter((_, i) => i !== idx) });
  };

  const updateCondition = (list: 'buy' | 'tp' | 'sl', idx: number, field: keyof Condition, value: string) => {
    const update = (arr: Condition[]) => arr.map((c, i) => i === idx ? { ...c, [field]: value } : c);
    if (list === 'buy') setState({ ...state, buyConditions: update(state.buyConditions) });
    else if (list === 'tp') setState({ ...state, sellTP: update(state.sellTP) });
    else setState({ ...state, sellSL: update(state.sellSL) });
  };

  const renderConditionEditor = (title: string, list: 'buy' | 'tp' | 'sl', conditions: Condition[]) => (
    <div style={{ marginBottom: 16 }}>
      <Space style={{ marginBottom: 8 }}>
        <Text strong>{title}</Text>
        <Button size="small" icon={<EditOutlined />} onClick={() => addCondition(list)}>添加条件</Button>
      </Space>
      {conditions.length === 0 && (
        <Tag>暂无条件</Tag>
      )}
      {conditions.map((c, idx) => (
        <Space key={idx} style={{ display: 'flex', marginBottom: 4 }}>
          <Select
            placeholder="指标"
            value={c.indicator || undefined}
            onChange={(v) => updateCondition(list, idx, 'indicator', v)}
            options={indicatorOptions}
            style={{ width: 160 }}
          />
          <Select
            placeholder="运算符"
            value={c.operator || undefined}
            onChange={(v) => updateCondition(list, idx, 'operator', v)}
            options={operatorOptions}
            style={{ width: 140 }}
          />
          <Input
            placeholder="值 (如 20, 50-80, MA60)"
            value={c.value}
            onChange={(e) => updateCondition(list, idx, 'value', e.target.value)}
            style={{ width: 180 }}
          />
          <Button size="small" danger onClick={() => removeCondition(list, idx)}>删除</Button>
        </Space>
      ))}
    </div>
  );

  const renderStep2 = () => (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      {renderConditionEditor('买入触发条件 (至少 1 个)', 'buy', state.buyConditions)}
      <Alert
        type="info"
        showIcon
        message="提示：买入条件是策略的触发信号。至少需要一个可执行的结构化条件。"
      />
    </Space>
  );

  // --- 步骤 3: 卖出/失效条件 ---
  const renderStep3 = () => (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Divider plain>止盈条件</Divider>
      {renderConditionEditor('止盈', 'tp', state.sellTP)}

      <Divider plain>止损条件</Divider>
      {renderConditionEditor('止损', 'sl', state.sellSL)}

      <Divider plain>否定条件 (可证伪)</Divider>
      <Text type="secondary" style={{ marginBottom: 8, display: 'block' }}>
        ⚠️ 否定条件是假设的可证伪机制——当这些条件发生时，策略被视为失效。<Text strong>必须填写至少 1 项</Text>才能进入实验。
      </Text>
      {state.invalidations.map((inv, idx) => (
        <Space key={idx} style={{ display: 'flex', marginBottom: 4 }}>
          <Tag color="orange">否定 {idx + 1}</Tag>
          <Input
            value={inv}
            onChange={(e) => {
              const arr = [...state.invalidations];
              arr[idx] = e.target.value;
              setState({ ...state, invalidations: arr });
            }}
            style={{ flex: 1 }}
            placeholder="如: 跌破 60 日均线，或成交量萎缩 50%"
          />
          <Button size="small" danger onClick={() =>
            setState({ ...state, invalidations: state.invalidations.filter((_, i) => i !== idx) })
          }>删除</Button>
        </Space>
      ))}
      <Button size="small" icon={<EditOutlined />} onClick={() =>
        setState({ ...state, invalidations: [...state.invalidations, ''] })
      }>添加否定条件</Button>

      <Divider plain>确认条件 (可选)</Divider>
      <Text type="secondary" style={{ marginBottom: 8, display: 'block' }}>
        辅助验证信号，增强策略信心，但不作为失效依据。
      </Text>
      {state.confirmations.map((c, idx) => (
        <Space key={idx} style={{ display: 'flex', marginBottom: 4 }}>
          <Tag color="blue">确认 {idx + 1}</Tag>
          <Input
            value={c}
            onChange={(e) => {
              const arr = [...state.confirmations];
              arr[idx] = e.target.value;
              setState({ ...state, confirmations: arr });
            }}
            style={{ flex: 1 }}
            placeholder="如: MACD 金叉"
          />
          <Button size="small" danger onClick={() =>
            setState({ ...state, confirmations: state.confirmations.filter((_, i) => i !== idx) })
          }>删除</Button>
        </Space>
      ))}
      <Button size="small" icon={<EditOutlined />} onClick={() =>
        setState({ ...state, confirmations: [...state.confirmations, ''] })
      }>添加确认条件</Button>
    </Space>
  );

  // --- 步骤 4: 特征与预期 ---
  const renderStep4 = () => (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card size="small" title={<Space><Tag color="purple">特征标签</Tag><Text type="secondary">标记策略类型，便于后续检索和分析</Text></Space>}>
        <Select
          mode="tags"
          placeholder="选择或添加特征标签"
          value={state.features}
          onChange={(v) => setState({ ...state, features: v })}
          options={featureOptions}
          style={{ minHeight: 80 }}
          tagRender={(props) => {
            const { label, value, closable, onClose } = props;
            const option = featureOptions.find(o => o.value === value);
            return (
              <Tag color="purple" closable={closable} onClose={onClose} style={{ margin: 2 }}>
                {option ? option.label : label}
              </Tag>
            );
          }}
        />
      </Card>

      <Form layout="vertical">
        <Row gutter={24}>
          <Col xs={24} md={8}>
            <Form.Item label="期望年化收益">
              <Input
                placeholder="如: 15%"
                value={state.expectedReturn}
                onChange={(e) => setState({ ...state, expectedReturn: e.target.value })}
              />
            </Form.Item>
          </Col>
          <Col xs={24} md={8}>
            <Form.Item label="风险收益比">
              <Input
                placeholder="如: 2:1"
                value={state.riskRewardRatio}
                onChange={(e) => setState({ ...state, riskRewardRatio: e.target.value })}
              />
            </Form.Item>
          </Col>
          <Col xs={24} md={8}>
            <Form.Item label={`置信度: ${(state.confidence * 100).toFixed(0)}%`}>
              <Input
                type="range"
                min={30}
                max={95}
                step={5}
                value={state.confidence * 100}
                onChange={(e) => setState({ ...state, confidence: Number(e.target.value) / 100 })}
                style={{ width: '100%' }}
              />
              <Progress percent={state.confidence * 100} size="small" />
            </Form.Item>
          </Col>
        </Row>
      </Form>

      <Alert
        type="warning"
        showIcon
        icon={<WarningOutlined />}
        message="进入实验的前提条件"
        description={
          <ul style={{ margin: 0, paddingLeft: 20 }}>
            <li>✅ 至少有 1 个买入触发条件</li>
            <li>✅ <Text strong>至少有 1 个否定条件（可证伪）</Text>——这是进入实验的硬性门槛</li>
            <li>✅ 有清晰的目标证券</li>
            <li>✅ 有明确的持有周期和预期</li>
          </ul>
        }
      />
    </Space>
  );

  // --- 步骤 5: 预览 ---
  const validateHypothesis = (): string[] => {
    const errs: string[] = [];
    if (!state.name.trim()) errs.push('假设名称不能为空');
    if (!state.stockCode.trim()) errs.push('必须指定目标证券代码');
    if (state.buyConditions.length === 0) errs.push('至少需要一个买入触发条件');
    for (const c of state.buyConditions) {
      if (!c.indicator || !c.operator) errs.push(`买入条件 "${c.indicator}" 不完整`);
    }
    if (state.invalidations.filter(x => x.trim()).length === 0) {
      errs.push('⚠️ 缺少可证伪条件 (否定条件)——这是进入实验的硬性门槛');
    }
    return errs;
  };

  const renderPreviewPanel = () => {
    if (previewLoading) {
      return (
        <Card>
          <div style={{ textAlign: 'center', padding: 40 }}>
            <Spin indicator={<LoadingOutlined />} tip="获取预览数据..." />
          </div>
        </Card>
      );
    }

    if (!previewData) {
      return null;
    }

    const { data_info, cost_info, validation } = previewData;

    return (
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        {/* 数据预览 */}
        <Card
          size="small"
          title={
            <Space>
              <DatabaseOutlined />
              <span>数据预览</span>
              {data_info.data_available ? (
                <Tag color="green">数据可用</Tag>
              ) : (
                <Tag color="red">数据缺失</Tag>
              )}
            </Space>
          }
        >
          <Row gutter={[16, 8]}>
            <Col xs={12} md={6}>
              <Text type="secondary">可用天数:</Text> <Text strong>{data_info.data_days} 天</Text>
            </Col>
            <Col xs={12} md={6}>
              <Text type="secondary">最新数据:</Text> <Text>{data_info.last_update || '未知'}</Text>
            </Col>
            <Col xs={12} md={6}>
              <Text type="secondary">最新收盘:</Text> <Text>{data_info.latest_close?.toFixed(2) || '--'}</Text>
            </Col>
            <Col xs={12} md={6}>
              <Text type="secondary">建议切分:</Text> <Text strong>{data_info.suggested_split}</Text>
            </Col>
          </Row>
          {data_info.warning && (
            <Alert
              style={{ marginTop: 8 }}
              type="warning"
              showIcon
              message={data_info.warning}
              description={`训练集: ${data_info.train_days} 天 / 测试集: ${data_info.test_days} 天`}
            />
          )}
        </Card>

        {/* 费用预览 */}
        <Card
          size="small"
          title={
            <Space>
              <DollarOutlined />
              <span>费用预估</span>
            </Space>
          }
        >
          <Row gutter={[16, 8]}>
            <Col xs={12} md={6}>
              <Text type="secondary">交易费率:</Text> <Text>{(cost_info.trading_cost_rate * 100).toFixed(2)}%</Text>
            </Col>
            <Col xs={12} md={6}>
              <Text type="secondary">滑点率:</Text> <Text>{(cost_info.slippage_rate * 100).toFixed(2)}%</Text>
            </Col>
            <Col xs={12} md={6}>
              <Text type="secondary">总成本率:</Text> <Text strong>{(cost_info.total_cost_rate * 100).toFixed(2)}%</Text>
            </Col>
            <Col xs={12} md={6}>
              <Text type="secondary">预期净收益:</Text> <Text strong>{cost_info.net_return_est}</Text>
            </Col>
          </Row>
          <Text type="secondary" style={{ fontSize: 12 }}>
            费用影响: {cost_info.cost_impact}（基于 {cost_info.expected_return} 预期收益估算）
          </Text>
        </Card>

        {/* 验证结果 */}
        <Card
          size="small"
          title={
            <Space>
              <CheckCircleOutlined />
              <span>验证结果</span>
              <Tag color={validation.can_create ? 'green' : 'red'}>
                {validation.can_create ? '可创建' : '不可创建'}
              </Tag>
            </Space>
          }
        >
          <Row gutter={[16, 8]}>
            <Col xs={8}>
              <Text type="secondary">可证伪性:</Text>{' '}
              <Tag color={validation.falsifiability ? 'green' : 'red'}>
                {validation.falsifiability ? '已具备' : '缺失'}
              </Tag>
            </Col>
            <Col xs={8}>
              <Text type="secondary">条件数:</Text> <Text>{validation.total_conditions} 个</Text>
            </Col>
            <Col xs={8}>
              <Text type="secondary">可自动评估:</Text> <Text>{validation.auto_evaluable} 个</Text>
            </Col>
          </Row>
          <div style={{ marginTop: 8 }}>
            <Text type="secondary">完整性:</Text>
            <Progress percent={Math.round(validation.completeness * 100)} size="small" />
          </div>
        </Card>
      </Space>
    );
  };

  const renderStep5 = () => {
    const validation = validateHypothesis();
    const canCreate = validation.length === 0 && previewData?.validation.can_create;

    return (
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <Card title={<Space><CheckCircleOutlined />假设预览</Space>}>
          <DescriptionsPreview state={state} />
        </Card>

        {renderPreviewPanel()}

        {validation.length > 0 ? (
          <Alert
            type="error"
            showIcon
            icon={<WarningOutlined />}
            message="假设验证未通过——请修正以下问题后才能进入实验"
            description={
              <ul style={{ margin: 0, paddingLeft: 20 }}>
                {validation.map((err, idx) => (
                  <li key={idx}>{err}</li>
                ))}
              </ul>
            }
          />
        ) : (
          <Alert
            type="success"
            showIcon
            icon={<CheckCircleOutlined />}
            message="✅ 假设已具备进入实验的条件"
            description={previewData?.data_info.data_available 
              ? '数据就绪，可证伪条件已具备，结构完整。点击下方按钮一键创建可复现实验。'
              : '⚠️ 数据尚未就绪，但假设结构有效。创建后将等待数据同步完成。'}
          />
        )}

        <Space>
          <Button
            type="primary"
            icon={<RocketOutlined />}
            disabled={!canCreate}
            loading={creating}
            onClick={async () => {
              setCreating(true);
              try {
                const resp = await api.paradigmCreate({
                  name: state.name,
                  side: state.side,
                  stock_code: state.stockCode,
                  stock_name: state.stockName,
                  rationale: state.rationale,
                  logic: state.logic,
                  features: state.features,
                  baseline: state.baseline,
                  buy_conditions: state.buyConditions.map(c => ({ indicator: c.indicator, operator: c.operator, value: c.value })),
                  sell_conditions: {
                    take_profit: state.sellTP.map(c => ({ indicator: c.indicator, operator: c.operator, value: c.value })),
                    stop_loss: state.sellSL.map(c => ({ indicator: c.indicator, operator: c.operator, value: c.value })),
                  },
                  confirmations: state.confirmations,
                  invalidations: state.invalidations,
                  expectation: {
                    holding_period: state.holdingPeriod,
                    expected_return: state.expectedReturn,
                    risk_reward_ratio: state.riskRewardRatio,
                    confidence: state.confidence,
                  },
                });
                if (resp.valid) {
                  setCreated(resp.paradigm);
                  message.success('假设已创建为可复现实验');
                } else {
                  message.error(resp.errors?.join('; ') || '创建失败');
                  setErrors(resp.errors || ['创建失败']);
                }
              } catch (err: any) {
                message.error(err?.message || String(err));
              } finally {
                setCreating(false);
              }
            }}
          >
            一键创建可复现实验 →
          </Button>
          <Button onClick={() => { setStep(0); setState(defaultState); setCreated(null); setPreviewData(null); }}>重置</Button>
        </Space>

        {created && (
          <Alert
            type="success"
            showIcon
            message={
              <Space>
                <CheckCircleOutlined />
                <span>实验已创建: <Text strong>{created.name}</Text></span>
                <Tag color={created.side === 'buy' ? 'green' : 'red'}>
                  {created.side === 'buy' ? '买入' : '卖出'}
                </Tag>
                <Tag>ID: {created.id}</Tag>
              </Space>
            }
            description={
              <Space>
                <Button
                  size="small"
                  type="link"
                  icon={<StockOutlined />}
                  onClick={() => { window.location.href = '/research/experiment'; }}
                >进入实验队列 →</Button>
                <Button
                  size="small"
                  type="link"
                  icon={<ExperimentOutlined />}
                  onClick={() => { window.location.href = '/research/candidates'; }}
                >查看候选评估 →</Button>
                <Button
                  size="small"
                  type="link"
                  icon={<DatabaseOutlined />}
                  onClick={async () => {
                    try {
                      const evidence = await api.paradigmEvidence(created.id);
                      message.success('证据卡已生成');
                      console.log('Evidence card:', evidence);
                    } catch (err) {
                      message.info('证据卡将在数据就绪后自动生成');
                    }
                  }}
                >生成证据卡 →</Button>
              </Space>
            }
          />
        )}
      </Space>
    );
  };

  const renderCurrentStep = () => {
    switch (step) {
      case 0: return renderStep1();
      case 1: return renderStep2();
      case 2: return renderStep3();
      case 3: return renderStep4();
      case 4: return renderStep5();
      default: return null;
    }
  };

  const next = () => {
    const nextStep = Math.min(step + 1, steps.length - 1);
    setStep(nextStep);
    if (nextStep === 4) {
      fetchPreviewDebounced(state);
    }
  };
  const prev = () => setStep(Math.max(step - 1, 0));

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>假设编辑器</Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          结构化引导生成假设：经济逻辑 → 触发 → 失效 → 特征 → 预览 → 一键创建可复现实验
        </Paragraph>
      </div>

      <Card>
        <Steps
          current={step}
          items={steps}
          onChange={(s) => { setStep(s); if (s === 4) fetchPreviewDebounced(state); }}
        />
      </Card>

      <Card>
        {renderCurrentStep()}

        <Divider />
        <Space>
          <Button onClick={prev} disabled={step === 0}>← 上一步</Button>
          {step < steps.length - 1 && (
            <Button type="primary" onClick={next}>下一步 →</Button>
          )}
          {step === steps.length - 1 && (
            <Text type="secondary">已到最后一步</Text>
          )}
        </Space>
      </Card>
    </Space>
  );
}

function DescriptionsPreview({ state }: { state: HypothesisState }) {
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Row gutter={[16, 12]}>
        <Col xs={12}><Text type="secondary">名称:</Text> <Text strong>{state.name || '—'}</Text></Col>
        <Col xs={12}><Text type="secondary">方向:</Text> <Tag color={state.side === 'buy' ? 'green' : 'red'}>{state.side}</Tag></Col>
        <Col xs={12}><Text type="secondary">证券:</Text> {state.stockCode} {state.stockName}</Col>
        <Col xs={12}><Text type="secondary">持有期:</Text> {state.holdingPeriod}</Col>
      </Row>

      {state.logic && (
        <div><Text type="secondary">经济/行为逻辑:</Text> <Text>{state.logic}</Text></div>
      )}

      {state.features.length > 0 && (
        <div>
          <Text strong>特征:</Text>
          <div style={{ marginTop: 4 }}>
            {state.features.map((f, i) => (
              <Tag key={i} color="purple">{f}</Tag>
            ))}
          </div>
        </div>
      )}

      {state.baseline && (
        <div><Text type="secondary">对照基线:</Text> <Tag>{state.baseline}</Tag></div>
      )}

      <Divider style={{ margin: '8px 0' }} />

      <div>
        <Text strong>买入触发 ({state.buyConditions.length}):</Text>
        <div style={{ marginTop: 4 }}>
          {state.buyConditions.length === 0 && <Tag>无</Tag>}
          {state.buyConditions.map((c, i) => (
            <Tag key={i} color="green" style={{ margin: 2 }}>
              {c.indicator} {c.operator} {c.value}
            </Tag>
          ))}
        </div>
      </div>

      {state.sellTP.length > 0 && (
        <div>
          <Text strong>止盈:</Text>
          <div style={{ marginTop: 4 }}>
            {state.sellTP.map((c, i) => (
              <Tag key={i} color="lime" style={{ margin: 2 }}>
                {c.indicator} {c.operator} {c.value}
              </Tag>
            ))}
          </div>
        </div>
      )}

      {state.sellSL.length > 0 && (
        <div>
          <Text strong>止损:</Text>
          <div style={{ marginTop: 4 }}>
            {state.sellSL.map((c, i) => (
              <Tag key={i} color="red" style={{ margin: 2 }}>
                {c.indicator} {c.operator} {c.value}
              </Tag>
            ))}
          </div>
        </div>
      )}

      <Divider style={{ margin: '8px 0' }} />

      <div>
        <Text strong>
          否定条件 (可证伪) <Tag color="orange" style={{ margin: 0 }}>{state.invalidations.filter(x => x.trim()).length}</Tag>
        </Text>
        <div style={{ marginTop: 4 }}>
          {state.invalidations.filter(x => x.trim()).length === 0 && (
            <Tag color="error">⚠ 缺失——不能进入实验</Tag>
          )}
          {state.invalidations.filter(x => x.trim()).map((inv, i) => (
            <Tag key={i} color="orange" style={{ margin: 2 }}>{inv}</Tag>
          ))}
        </div>
      </div>

      {state.confirmations.filter(x => x.trim()).length > 0 && (
        <div>
          <Text strong>确认条件:</Text>
          <div style={{ marginTop: 4 }}>
            {state.confirmations.filter(x => x.trim()).map((c, i) => (
              <Tag key={i} color="blue" style={{ margin: 2 }}>{c}</Tag>
            ))}
          </div>
        </div>
      )}

      <Divider style={{ margin: '8px 0' }} />

      <Row gutter={[16, 8]}>
        <Col><Text type="secondary">期望收益:</Text> {state.expectedReturn}</Col>
        <Col><Text type="secondary">风险收益比:</Text> {state.riskRewardRatio}</Col>
        <Col><Text type="secondary">置信度:</Text> {(state.confidence * 100).toFixed(0)}%</Col>
      </Row>
    </Space>
  );
}

// End of Hypothesis component
