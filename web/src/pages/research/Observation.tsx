import { useEffect, useState } from 'react';
import {
  Card, Row, Col, Tag, Typography, Space, Alert, Table, Progress, Button, message,
  Modal, Form, Input, InputNumber, DatePicker, Select, Statistic, Divider, Descriptions,
  Drawer, Empty, Tooltip, Badge, Segmented,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  EyeOutlined, WarningOutlined, CheckCircleFilled, PlusOutlined,
  PlayCircleOutlined, StopOutlined, FundOutlined, ThunderboltOutlined,
  ExperimentOutlined, BarChartOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { api } from '../../api/client';
import type {
  ParadigmAlertItem, ParadigmItem,
  ForwardRun, ForwardRunCreateRequest, SignalEntry, ComparisonReport,
  ForwardRunExecuteResponse,
} from '../../types/api';

const { Title, Text, Paragraph } = Typography;

export default function Observation() {
  const [loading, setLoading] = useState(false);
  const [alerts, setAlerts] = useState<ParadigmAlertItem[]>([]);
  const [, setTotal] = useState(0);
  const [paradigms, setParadigms] = useState<ParadigmItem[]>([]);

  // Forward Run state
  const [runs, setRuns] = useState<ForwardRun[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [executeModalOpen, setExecuteModalOpen] = useState(false);
  const [compareModalOpen, setCompareModalOpen] = useState(false);
  const [detailDrawerOpen, setDetailDrawerOpen] = useState(false);
  const [currentRun, setCurrentRun] = useState<ForwardRun | null>(null);
  const [currentSignals, setCurrentSignals] = useState<SignalEntry[]>([]);
  const [compareResult, setCompareResult] = useState<{
    report: ComparisonReport; pass: boolean; warnings: string[];
  } | null>(null);
  const [activeTab, setActiveTab] = useState<'alerts' | 'runs' | 'signals'>('alerts');

  const load = async () => {
    setLoading(true);
    try {
      const [alertResp, paradigmResp] = await Promise.all([
        api.paradigmAlerts(),
        api.paradigmList(undefined, undefined, { review_status: 'promoted', limit: 50 }),
      ]);
      setAlerts(alertResp.alerts || []);
      setTotal(alertResp.total || 0);
      setParadigms(paradigmResp.paradigms || []);
    } catch (err) {
      message.error(String(err));
    } finally {
      setLoading(false);
    }
  };

  const loadRuns = async () => {
    setRunsLoading(true);
    try {
      const resp = await api.forwardRuns(20);
      setRuns(resp.runs || []);
    } catch (err) {
      message.error(String(err));
    } finally {
      setRunsLoading(false);
    }
  };

  useEffect(() => {
    load();
    loadRuns();
  }, []);

  // ========================================================================
  // Forward Run Actions
  // ========================================================================

  const [createForm] = Form.useForm();
  const [compareForm] = Form.useForm();

  const handleCreateRun = async (values: any) => {
    try {
      const payload: ForwardRunCreateRequest = {
        paradigm_version_id: values.paradigm_version_id,
        start_date: values.start_date.format('YYYY-MM-DD'),
        initial_cash: values.initial_cash || 1000000,
        enable_t_1: values.enable_t_1 ?? true,
        enable_price_limit: values.enable_price_limit ?? true,
        enable_suspension: values.enable_suspension ?? true,
        board: values.board || 'main',
        commission_rate: values.commission_rate,
        slippage_bps: values.slippage_bps,
        stamp_duty_rate: values.stamp_duty_rate,
      };
      const resp = await api.forwardRunCreate(payload);
      message.success(`前向运行 ${resp.run.id} 已创建`);
      setCreateModalOpen(false);
      createForm.resetFields();
      loadRuns();
    } catch (err) {
      message.error(String(err));
    }
  };

  const handleExecuteRun = async (runId: string, payload?: { from_date?: string; to_date?: string; signal_id?: string }) => {
    try {
      const resp: ForwardRunExecuteResponse = await api.forwardRunExecute(runId, payload);
      message.success(`执行完成: ${resp.executed} 成交, ${resp.rejected} 拒绝`);
      loadRuns();
      // Refresh current run details if open
      if (currentRun?.id === runId) {
        loadRunDetails(runId);
      }
    } catch (err) {
      message.error(String(err));
    }
  };

  const handleFinalizeRun = async (runId: string) => {
    Modal.confirm({
      title: '结束前向运行',
      content: '确定要结束这个前向运行吗? 结束后将计算最终统计数据。',
      onOk: async () => {
        try {
          const resp = await api.forwardRunFinalize(runId);
          message.success(`运行 ${resp.run.id} 已结束`);
          loadRuns();
        } catch (err) {
          message.error(String(err));
        }
      },
    });
  };

  const loadRunDetails = async (runId: string) => {
    try {
      const [runResp, signalsResp] = await Promise.all([
        api.forwardRunGet(runId),
        api.forwardRunSignals(runId),
      ]);
      setCurrentRun(runResp.run);
      setCurrentSignals(signalsResp.signals || []);
    } catch (err) {
      message.error(String(err));
    }
  };

  const handleOpenRunDetail = async (run: ForwardRun) => {
    setCurrentRun(run);
    setDetailDrawerOpen(true);
    await loadRunDetails(run.id);
  };

  const handleCompare = async (values: any) => {
    if (!currentRun) return;
    try {
      const resp = await api.forwardRunCompare(currentRun.id, {
        theoretical_return: values.theoretical_return || 0,
        theoretical_max_drawdown: values.theoretical_max_drawdown || 0,
        theoretical_sharpe: values.theoretical_sharpe || 0,
        theoretical_win_rate: values.theoretical_win_rate || 0,
        theoretical_signals: values.theoretical_signals || 0,
        theoretical_annualized_return: values.theoretical_annualized_return || 0,
      });
      setCompareResult({
        report: resp.report,
        pass: resp.pass,
        warnings: resp.warnings,
      });
    } catch (err) {
      message.error(String(err));
    }
  };

  // ========================================================================
  // Derived data
  // ========================================================================

  const criticalCount = alerts.filter(a => a.severity === 'critical').length;

  const activeRuns = runs.filter(r => r.status === 'active');
  const completedRuns = runs.filter(r => r.status === 'completed');

  // ========================================================================
  // Table columns
  // ========================================================================

  const alertColumns: ColumnsType<ParadigmAlertItem> = [
    {
      title: '严重度', dataIndex: 'severity', key: 'severity', width: 90,
      render: (s: string) => {
        const colors: Record<string, string> = { critical: 'red', warning: 'orange', info: 'blue' };
        return <Tag color={colors[s] || 'default'}>{s}</Tag>;
      },
    },
    { title: '范式', dataIndex: 'paradigm_id', key: 'pid', width: 160, ellipsis: true },
    { title: '股票', dataIndex: 'stock_code', key: 'code', width: 80 },
    { title: '类型', dataIndex: 'type', key: 'type', width: 100 },
    { title: '条件', dataIndex: 'condition', key: 'condition', width: 180, ellipsis: true },
    { title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (s: string) => <Tag>{s}</Tag>,
    },
  ];

  const runColumns: ColumnsType<ForwardRun> = [
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 100,
      render: (s: string) => {
        const colors: Record<string, string> = { active: 'processing', completed: 'success', stopped: 'default' };
        return <Badge status={colors[s] as any} text={s} />;
      },
    },
    { title: '运行 ID', dataIndex: 'id', key: 'id', width: 220, ellipsis: true,
      render: (id: string) => <Text copyable style={{ fontSize: 12 }}>{id}</Text>,
    },
    { title: '范式版本', dataIndex: 'paradigm_version_id', key: 'pvid', width: 140, ellipsis: true },
    { title: '开始日期', dataIndex: 'start_date', key: 'start', width: 110,
      render: (d: string) => dayjs(d).format('YYYY-MM-DD'),
    },
    {
      title: '收益', dataIndex: 'total_return', key: 'return', width: 110,
      render: (r: number) => (
        <Text style={{ color: r >= 0 ? '#52c41a' : '#ff4d4f', fontWeight: 600 }}>
          {(r * 100).toFixed(2)}%
        </Text>
      ),
    },
    { title: '信号', dataIndex: 'signal_count', key: 'signals', width: 70 },
    { title: '成交', dataIndex: 'filled_count', key: 'filled', width: 70 },
    { title: '拒绝', dataIndex: 'rejected_count', key: 'rejected', width: 70,
      render: (r: number) => r > 0 ? <Text type="danger">{r}</Text> : r,
    },
    { title: '胜率', dataIndex: 'win_rate', key: 'winrate', width: 80,
      render: (w: number) => w > 0 ? `${(w * 100).toFixed(1)}%` : '-',
    },
    {
      title: '操作', key: 'actions', width: 180, fixed: 'right',
      render: (_: any, record: ForwardRun) => (
        <Space size={4}>
          <Tooltip title="查看详情">
            <Button size="small" icon={<EyeOutlined />} onClick={() => handleOpenRunDetail(record)} />
          </Tooltip>
          {record.status === 'active' && (
            <>
              <Tooltip title="执行信号">
                <Button size="small" type="primary" icon={<PlayCircleOutlined />}
                  onClick={() => { setCurrentRun(record); setExecuteModalOpen(true); }} />
              </Tooltip>
              <Tooltip title="结束运行">
                <Button size="small" danger icon={<StopOutlined />}
                  onClick={() => handleFinalizeRun(record.id)} />
              </Tooltip>
            </>
          )}
          {record.status === 'completed' && (
            <Tooltip title="对比分析">
              <Button size="small" icon={<BarChartOutlined />}
                onClick={() => { setCurrentRun(record); setCompareModalOpen(true); setCompareResult(null); }} />
            </Tooltip>
          )}
        </Space>
      ),
    },
  ];

  const signalColumns: ColumnsType<SignalEntry> = [
    { title: '日期', dataIndex: 'signal_date', key: 'date', width: 110,
      render: (d: string) => dayjs(d).format('YYYY-MM-DD'),
    },
    { title: '股票', dataIndex: 'stock_code', key: 'code', width: 90 },
    { title: '方向', dataIndex: 'direction', key: 'dir', width: 70,
      render: (d: string) => <Tag color={d === 'buy' ? 'green' : 'red'}>{d === 'buy' ? '买入' : '卖出'}</Tag>,
    },
    { title: '信号价', dataIndex: 'price', key: 'price', width: 90,
      render: (p: number) => p.toFixed(2),
    },
    { title: '置信度', dataIndex: 'confidence', key: 'conf', width: 100,
      render: (c: number) => <Progress percent={Math.round(c * 100)} size="small" />,
    },
    { title: '执行状态', key: 'exec_status', width: 100,
      render: (_: any, r: SignalEntry) => {
        if (!r.execution) return <Tag>待执行</Tag>;
        const color = r.execution.status === 'filled' ? 'green' : r.execution.status === 'rejected' ? 'red' : 'orange';
        return <Tag color={color}>{r.execution.status}</Tag>;
      },
    },
    { title: '成交价', key: 'exec_price', width: 90,
      render: (_: any, r: SignalEntry) => r.execution ? r.execution.exec_price.toFixed(2) : '-',
    },
    { title: '盈亏', key: 'pnl', width: 100,
      render: (_: any, r: SignalEntry) => {
        if (!r.execution || r.execution.pnl === 0) return '-';
        return <Text style={{ color: r.execution.pnl > 0 ? '#52c41a' : '#ff4d4f' }}>{r.execution.pnl.toFixed(2)}</Text>;
      },
    },
    { title: '原因', key: 'reason', width: 150, ellipsis: true,
      render: (_: any, r: SignalEntry) => r.execution?.reject_reason || r.source?.triggered_by || '-',
    },
  ];

  // ========================================================================
  // Render
  // ========================================================================

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={3} style={{ marginBottom: 4 }}>前向观察</Title>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          不可回填的实时信号监测与 Paper Trading 前向模拟，追踪已验证范式的实时表现与风险状态
        </Paragraph>
      </div>

      {/* 统计卡片 */}
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="监测中范式"
              value={paradigms.length}
              prefix={<EyeOutlined style={{ color: '#1677ff' }} />}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="活跃前向运行"
              value={activeRuns.length}
              valueStyle={{ color: activeRuns.length > 0 ? '#1677ff' : undefined }}
              prefix={<FundOutlined />}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="已完成运行"
              value={completedRuns.length}
              valueStyle={{ color: completedRuns.length > 0 ? '#52c41a' : undefined }}
              prefix={<CheckCircleFilled />}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="严重警告"
              value={criticalCount}
              valueStyle={{ color: criticalCount > 0 ? '#ff4d4f' : undefined }}
              prefix={<WarningOutlined />}
            />
          </Card>
        </Col>
      </Row>

      {/* Tab Switcher */}
      <Segmented
        value={activeTab}
        onChange={(v) => setActiveTab(v as any)}
        options={[
          { label: <Space><WarningOutlined />信号告警</Space>, value: 'alerts' },
          { label: <Space><FundOutlined />前向运行</Space>, value: 'runs' },
          { label: <Space><ThunderboltOutlined />信号账本</Space>, value: 'signals' },
        ]}
      />

      {activeTab === 'alerts' && (
        <>
          {criticalCount > 0 && (
            <Alert
              type="error"
              showIcon
              icon={<WarningOutlined />}
              message={`发现 ${criticalCount} 条严重警告`}
              description="多个已验证范式出现严重异常，建议立即检查相关持仓"
              action={<Button danger size="small" onClick={() => {
                window.location.href = '/portfolio';
              }}>查看持仓</Button>}
            />
          )}

          <Card title={<Space><EyeOutlined />实时信号监控</Space>}
            extra={<Button onClick={load} loading={loading}>刷新</Button>}
          >
            <Table
              rowKey={(r) => `${r.paradigm_id}-${r.stock_code}-${r.type}`}
              columns={alertColumns}
              dataSource={alerts}
              loading={loading}
              pagination={{ pageSize: 10, showSizeChanger: true }}
              size="middle"
            />
          </Card>

          <Card title="已晋级范式">
            {paradigms.length === 0 ? (
              <Empty description="暂无已晋级范式" />
            ) : (
              <Row gutter={[16, 16]}>
                {paradigms.map((p) => (
                  <Col xs={24} sm={12} lg={8} key={p.id}>
                    <Card size="small">
                      <Space direction="vertical" size={4} style={{ width: '100%' }}>
                        <Text strong ellipsis style={{ maxWidth: 200 }}>{p.name}</Text>
                        <Space>
                          <Tag color={p.side === 'buy' ? 'green' : 'red'}>{p.side === 'buy' ? '买' : '卖'}</Tag>
                          <Tag color={p.validation?.reliability_label === 'high' ? 'green' : 'orange'}>
                            {p.validation?.reliability_label || '未知'}
                          </Tag>
                        </Space>
                        {p.actual_return !== undefined && (
                          <div>
                            <Text type="secondary" style={{ fontSize: 12 }}>实际收益</Text>
                            <div>
                              <Text style={{
                                color: p.actual_return >= 0 ? '#52c41a' : '#ff4d4f',
                                fontSize: 18, fontWeight: 600,
                              }}>
                                {(p.actual_return * 100).toFixed(2)}%
                              </Text>
                            </div>
                          </div>
                        )}
                        {p.expectation?.win_rate !== undefined && (
                          <Progress
                            percent={Math.round((p.expectation?.win_rate ?? 0) * 100)}
                            size="small"
                            format={() => `胜率 ${((p.expectation?.win_rate ?? 0) * 100).toFixed(0)}%`}
                          />
                        )}
                      </Space>
                    </Card>
                  </Col>
                ))}
              </Row>
            )}
          </Card>
        </>
      )}

      {activeTab === 'runs' && (
        <Card
          title={<Space><FundOutlined />前向运行管理</Space>}
          extra={
            <Space>
              <Button onClick={loadRuns} loading={runsLoading}>刷新</Button>
              <Button type="primary" icon={<PlusOutlined />}
                onClick={() => setCreateModalOpen(true)}>新建运行</Button>
            </Space>
          }
        >
          <Table
            rowKey="id"
            columns={runColumns}
            dataSource={runs}
            loading={runsLoading}
            pagination={{ pageSize: 10, showSizeChanger: true }}
            scroll={{ x: 1400 }}
            size="middle"
          />
        </Card>
      )}

      {activeTab === 'signals' && (
        <Card title={<Space><ThunderboltOutlined />信号账本 (全部运行)</Space>}>
          {runs.length === 0 ? (
            <Empty description="暂无前向运行，请先创建一个运行" />
          ) : (
            <Empty description="选择一个前向运行查看信号" />
          )}
        </Card>
      )}

      {/* ================================================================== */}
      {/* Modals & Drawers                                                    */}
      {/* ================================================================== */}

      {/* Create Run Modal */}
      <Modal
        title={<Space><PlusOutlined />新建前向运行</Space>}
        open={createModalOpen}
        onCancel={() => { setCreateModalOpen(false); createForm.resetFields(); }}
        onOk={() => createForm.submit()}
        width={600}
      >
        <Form form={createForm} layout="vertical" onFinish={handleCreateRun}
          initialValues={{
            initial_cash: 1000000,
            enable_t_1: true,
            enable_price_limit: true,
            enable_suspension: true,
            board: 'main',
            start_date: dayjs(),
          }}
        >
          <Form.Item name="paradigm_version_id" label="范式版本 ID" rules={[{ required: true }]}>
            <Input placeholder="pv-xxx" />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="start_date" label="开始日期" rules={[{ required: true }]}>
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="initial_cash" label="初始资金 (元)">
                <InputNumber style={{ width: '100%' }} min={10000} step={100000} />
              </Form.Item>
            </Col>
          </Row>
          <Divider plain>交易约束</Divider>
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item name="enable_t_1" label="T+1 约束" valuePropName="checked">
                <Select options={[{ value: true, label: '启用' }, { value: false, label: '禁用' }]} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="enable_price_limit" label="涨跌停" valuePropName="checked">
                <Select options={[{ value: true, label: '启用' }, { value: false, label: '禁用' }]} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="enable_suspension" label="停牌检查" valuePropName="checked">
                <Select options={[{ value: true, label: '启用' }, { value: false, label: '禁用' }]} />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="board" label="板块">
            <Select options={[
              { value: 'main', label: '主板 (±10%)' },
              { value: 'chinext', label: '创业板 (±20%)' },
              { value: 'star', label: '科创板 (±20%)' },
              { value: 'bj', label: '北交所 (±30%)' },
            ]} />
          </Form.Item>
        </Form>
      </Modal>

      {/* Execute Modal */}
      <Modal
        title={<Space><PlayCircleOutlined />执行信号 - {currentRun?.id}</Space>}
        open={executeModalOpen}
        onCancel={() => setExecuteModalOpen(false)}
        onOk={async () => {
          if (currentRun) {
            await handleExecuteRun(currentRun.id);
          }
          setExecuteModalOpen(false);
        }}
      >
        <Alert type="info" showIcon
          message="将执行该前向运行中所有未执行的信号"
          description="执行将遵守 A 股交易约束 (T+1 / 涨跌停 / 停牌 / 最小交易单位)"
          style={{ marginBottom: 16 }}
        />
        <Descriptions column={1} size="small" bordered>
          <Descriptions.Item label="运行 ID">{currentRun?.id}</Descriptions.Item>
          <Descriptions.Item label="状态">{currentRun?.status}</Descriptions.Item>
          <Descriptions.Item label="信号总数">{currentRun?.signal_count}</Descriptions.Item>
          <Descriptions.Item label="已成交">{currentRun?.filled_count}</Descriptions.Item>
          <Descriptions.Item label="已执行">{currentRun?.executed_count}</Descriptions.Item>
        </Descriptions>
      </Modal>

      {/* Compare Modal */}
      <Modal
        title={<Space><BarChartOutlined />对比分析 - {currentRun?.id}</Space>}
        open={compareModalOpen}
        onCancel={() => { setCompareModalOpen(false); setCompareResult(null); }}
        footer={null}
        width={720}
      >
        {!compareResult ? (
          <Form form={compareForm} layout="vertical" onFinish={handleCompare}
            initialValues={{
              theoretical_return: 0.15,
              theoretical_max_drawdown: 0.05,
              theoretical_sharpe: 2.0,
              theoretical_win_rate: 0.65,
              theoretical_signals: currentRun?.signal_count || 0,
              theoretical_annualized_return: 0.30,
            }}
          >
            <Divider plain>理论回测指标</Divider>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item name="theoretical_return" label="理论收益率" rules={[{ required: true }]}>
                  <InputNumber style={{ width: '100%' }} step={0.01} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name="theoretical_annualized_return" label="年化收益率">
                  <InputNumber style={{ width: '100%' }} step={0.01} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name="theoretical_max_drawdown" label="理论最大回撤">
                  <InputNumber style={{ width: '100%' }} step={0.01} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name="theoretical_sharpe" label="理论夏普比率">
                  <InputNumber style={{ width: '100%' }} step={0.1} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name="theoretical_win_rate" label="理论胜率">
                  <InputNumber style={{ width: '100%' }} step={0.05} min={0} max={1} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name="theoretical_signals" label="理论信号数">
                  <InputNumber style={{ width: '100%' }} min={0} />
                </Form.Item>
              </Col>
            </Row>
            <Button type="primary" htmlType="submit" block>执行对比分析</Button>
          </Form>
        ) : (
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            {/* Pass/Fail Banner */}
            <Alert
              type={compareResult.pass ? 'success' : 'error'}
              showIcon
              icon={compareResult.pass ? <CheckCircleFilled /> : <WarningOutlined />}
              message={compareResult.pass ? '✅ 前向表现验证通过' : '❌ 前向表现验证未通过'}
              description={compareResult.pass
                ? '前向表现与理论回测一致, 范式有效性得到验证'
                : '前向表现与理论回测存在显著差距, 需进一步分析'}
            />
            {compareResult.warnings.length > 0 && (
              <Alert
                type="warning"
                showIcon
                message="警告"
                description={
                  <ul style={{ margin: 0, paddingLeft: 20 }}>
                    {compareResult.warnings.map((w, i) => <li key={i}>{w}</li>)}
                  </ul>
                }
              />
            )}

            {/* Metrics Comparison */}
            <Row gutter={[16, 16]}>
              <Col span={12}>
                <Card size="small" title={<Text type="secondary">理论收益</Text>}>
                  <Text style={{ fontSize: 24, fontWeight: 600 }}>
                    {(compareResult.report.theoretical.total_return * 100).toFixed(2)}%
                  </Text>
                </Card>
              </Col>
              <Col span={12}>
                <Card size="small" title={<Text type="secondary">实际收益</Text>}>
                  <Text style={{
                    fontSize: 24, fontWeight: 600,
                    color: compareResult.report.actual.total_return >= 0 ? '#52c41a' : '#ff4d4f',
                  }}>
                    {(compareResult.report.actual.total_return * 100).toFixed(2)}%
                  </Text>
                </Card>
              </Col>
            </Row>

            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="收益差距">
                <Text style={{ color: compareResult.report.gap.return_gap > 0 ? '#ff4d4f' : '#52c41a' }}>
                  {(compareResult.report.gap.return_gap * 100).toFixed(2)}%
                  <Text type="secondary" style={{ marginLeft: 8 }}>
                    ({(compareResult.report.gap.return_gap_pct * 100).toFixed(1)}%)
                  </Text>
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label="回撤差距">
                {(compareResult.report.gap.drawdown_gap * 100).toFixed(2)}%
              </Descriptions.Item>
              <Descriptions.Item label="夏普差距">
                {compareResult.report.gap.sharpe_gap.toFixed(2)}
              </Descriptions.Item>
              <Descriptions.Item label="胜率差距">
                {(compareResult.report.gap.win_rate_gap * 100).toFixed(1)}%
              </Descriptions.Item>
              <Descriptions.Item label="约束影响">
                {compareResult.report.gap.constraint_impact > 0
                  ? `${(compareResult.report.gap.constraint_impact * 100).toFixed(1)}% 的信号被约束拒绝`
                  : '无'}
              </Descriptions.Item>
              <Descriptions.Item label="执行损失">
                {compareResult.report.gap.exec_loss > 0
                  ? `${(compareResult.report.gap.exec_loss * 100).toFixed(1)}%`
                  : '无'}
              </Descriptions.Item>
            </Descriptions>

            <Card size="small" title="关键洞察">
              <ul style={{ margin: 0 }}>
                {compareResult.report.gap.key_insights.map((insight, i) => (
                  <li key={i} style={{ marginBottom: 4 }}>
                    <ExperimentOutlined style={{ marginRight: 6, color: '#1677ff' }} />
                    {insight}
                  </li>
                ))}
              </ul>
            </Card>

            <Button onClick={() => setCompareResult(null)}>重新对比</Button>
          </Space>
        )}
      </Modal>

      {/* Run Detail Drawer */}
      <Drawer
        title={<Space><EyeOutlined />前向运行详情</Space>}
        open={detailDrawerOpen}
        onClose={() => setDetailDrawerOpen(false)}
        width={720}
      >
        {currentRun && (
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            {/* Run Info */}
            <Card size="small">
              <Descriptions column={2} size="small">
                <Descriptions.Item label="运行 ID" span={2}>
                  <Text copyable>{currentRun.id}</Text>
                </Descriptions.Item>
                <Descriptions.Item label="状态">
                  <Badge status={currentRun.status === 'active' ? 'processing' : 'success'} text={currentRun.status} />
                </Descriptions.Item>
                <Descriptions.Item label="开始日期">
                  {dayjs(currentRun.start_date).format('YYYY-MM-DD')}
                </Descriptions.Item>
                {currentRun.end_date && (
                  <Descriptions.Item label="结束日期">
                    {dayjs(currentRun.end_date).format('YYYY-MM-DD')}
                  </Descriptions.Item>
                )}
                <Descriptions.Item label="初始资金">
                  ¥{currentRun.initial_cash.toLocaleString()}
                </Descriptions.Item>
                <Descriptions.Item label="最终资金">
                  ¥{currentRun.final_cash.toLocaleString(undefined, { maximumFractionDigits: 2 })}
                </Descriptions.Item>
                <Descriptions.Item label="总收益" span={2}>
                  <Text style={{
                    color: currentRun.total_return >= 0 ? '#52c41a' : '#ff4d4f',
                    fontWeight: 600, fontSize: 18,
                  }}>
                    ¥{currentRun.total_pnl.toFixed(2)} ({(currentRun.total_return * 100).toFixed(2)}%)
                  </Text>
                </Descriptions.Item>
                <Descriptions.Item label="最大回撤">
                  {(currentRun.max_drawdown * 100).toFixed(2)}%
                </Descriptions.Item>
                <Descriptions.Item label="胜率">
                  {(currentRun.win_rate * 100).toFixed(1)}%
                </Descriptions.Item>
              </Descriptions>
            </Card>

            {/* Signal list */}
            <Card size="small" title={`信号列表 (${currentSignals.length})`}>
              <Table
                rowKey="id"
                columns={signalColumns}
                dataSource={currentSignals}
                pagination={{ pageSize: 10 }}
                size="small"
                scroll={{ x: 1200 }}
              />
            </Card>

            {/* Actions */}
            {currentRun.status === 'active' && (
              <Space>
                <Button type="primary" icon={<PlayCircleOutlined />}
                  onClick={() => handleExecuteRun(currentRun.id)}>
                  执行所有待执行信号
                </Button>
                <Button danger icon={<StopOutlined />}
                  onClick={() => handleFinalizeRun(currentRun.id)}>
                  结束运行
                </Button>
              </Space>
            )}
            {currentRun.status === 'completed' && (
              <Button icon={<BarChartOutlined />}
                onClick={() => { setCompareModalOpen(true); setCompareResult(null); }}>
                对比分析
              </Button>
            )}
          </Space>
        )}
      </Drawer>
    </Space>
  );
}
