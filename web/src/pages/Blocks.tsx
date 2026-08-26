import { useState } from 'react';
import { FundOutlined, StockOutlined, SearchOutlined, FilterOutlined, SaveOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Col, Empty, Input, InputNumber, List, Modal, Row, Select, Skeleton, Space, Switch, Table, Tag, Tabs, Typography, message } from 'antd';
import { useStockPool, usePoolStocks } from '../hooks/useStockPool';
import { useStockInfo, type StockInfoFilters } from '../hooks/useStockInfo';
import { useTdxBlocks } from '../hooks/useTdxBlocks';
import { StockPoolList } from '../components/StockPoolList';
import { StockPoolFilterPanel } from '../components/StockPoolFilterPanel';
import { StockPoolModal } from '../components/StockPoolModal';
import { StockListTable } from '../components/StockListTable';
import DiscoverResearchPanel from '../components/DiscoverResearchPanel';
import type { CustomStockPool, StockPoolFilter, FilterField } from '../types/api';

const exchangeOptions = [
  { value: '', label: '全部' },
  { value: 'sh', label: '上海(SH)' },
  { value: 'sz', label: '深圳(SZ)' },
  { value: 'bj', label: '北京(BJ)' },
];

export default function Blocks() {
  // === TDX Blocks ===
  const {
    files,
    selectedFile,
    setSelectedFile,
    blocks,
    selectedBlock,
    setSelectedBlock,
    blockStocks,
    loadingFiles,
    loadingBlocks,
    loadingStocks,
  } = useTdxBlocks();

  // === Custom Stock Pool ===
  const {
    pools,
    currentPool,
    currentPoolId,
    setCurrentPoolId,
    addPool,
    updatePool,
    deletePool,
  } = useStockPool();

  const { filteredStocks, loading: loadingPoolStocks } = usePoolStocks(currentPool);

  // === Modal States ===
  const [showAddModal, setShowAddModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [editingPool, setEditingPool] = useState<CustomStockPool | null>(null);
  const [showSaveAsPoolModal, setShowSaveAsPoolModal] = useState(false);
  const [saveAsPoolName, setSaveAsPoolName] = useState('');

  // === All Stocks ===
  const {
    stocks: allStocks,
    total: allStocksTotal,
    loading: loadingAllStocks,
    filters: stockInfoFilters,
    setFilters: setStockInfoFilters,
    resetFilters: resetStockInfoFilters,
  } = useStockInfo();

  // === Event Handlers ===
  const handleAddPool = () => {
    setEditingPool(null);
    setShowAddModal(true);
  };

  const handleEditPool = (pool: CustomStockPool) => {
    setEditingPool(pool);
    setShowEditModal(true);
  };

  const handleDeletePool = (id: string) => {
    if (pools.length <= 1) {
      Modal.warning({ title: '提示', content: '至少保留一个股票池' });
      return;
    }
    Modal.confirm({
      title: '确认删除',
      content: '确定要删除这个股票池吗？',
      okText: '删除',
      okType: 'danger',
      onOk: async () => {
        try {
          await deletePool(id);
        } catch {
          message.error('删除失败');
        }
      },
    });
  };

  const handleSavePool = (pool: CustomStockPool) => {
    if (editingPool) {
      updatePool(pool);
    } else {
      addPool({
        name: pool.name,
        description: pool.description,
        filters: pool.filters,
      });
    }
  };

  const handleSaveAsPool = async () => {
    if (!saveAsPoolName.trim()) {
      message.error('请输入股票池名称');
      return;
    }
    
    const filters: StockPoolFilter[] = [];
    if (stockInfoFilters.minMarketCap != null || stockInfoFilters.maxMarketCap != null) {
      filters.push({
        field: 'marketCap' as FilterField,
        operator: 'between',
        value: [stockInfoFilters.minMarketCap || 0, stockInfoFilters.maxMarketCap || 999999],
      });
    }
    if (stockInfoFilters.exchange) {
      filters.push({
        field: 'exchange' as FilterField,
        operator: 'in',
        value: [stockInfoFilters.exchange],
      });
    }
    if (stockInfoFilters.excludeST) {
      filters.push({
        field: 'excludeST' as FilterField,
        operator: 'between',
        value: [true],
      });
    }
    
    await addPool({
      name: saveAsPoolName.trim(),
      description: `筛选条件: ${filters.length > 0 ? filters.map(f => f.field).join(', ') : '无'}`,
      filters,
    });
    
    setShowSaveAsPoolModal(false);
    setSaveAsPoolName('');
    message.success('股票池保存成功');
  };

  const handleFilterChange = (key: keyof StockInfoFilters, value: number | string | boolean | undefined) => {
    setStockInfoFilters({ [key]: value });
  };

  return (
    <Space direction="vertical" size={24} style={{ display: 'flex' }}>
      <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.22), rgba(14,165,233,0.12))' }}>
        <Space direction="vertical" size={10} style={{ display: 'flex' }}>
          <Tag color="blue" style={{ width: 'fit-content', marginInlineEnd: 0 }}>股票池管理</Tag>
          <Typography.Title level={2} style={{ margin: 0 }}>
            板块与股票池
          </Typography.Title>
          <Typography.Text type="secondary">
            管理TDX板块和自定义股票池，支持多种筛选条件组合。
          </Typography.Text>
        </Space>
      </Card>

      <Tabs defaultActiveKey="blocks">
        {/* TDX Blocks Tab */}
        <Tabs.TabPane
          key="blocks"
          tab={
            <Space>
              <FundOutlined />
              <span>TDX 板块</span>
            </Space>
          }
        >
          <Row gutter={[16, 16]}>
            <Col xs={24} lg={8}>
              <Card title={<span>板块文件</span>}>
                {loadingFiles ? (
                  <Skeleton active paragraph={{ rows: 2 }} title={false} />
                ) : files.length === 0 ? (
                  <Empty description="暂无板块数据" />
                ) : (
                  <Select
                    value={selectedFile}
                    onChange={setSelectedFile}
                    options={files.map((f) => ({ value: f.file, label: `${f.name} (${f.desc})` }))}
                    style={{ width: '100%' }}
                  />
                )}
              </Card>
            </Col>

            <Col xs={24} lg={8}>
              <Card title={<span>板块列表</span>}>
                {loadingBlocks ? (
                  <Skeleton active paragraph={{ rows: 6 }} title={false} />
                ) : blocks.length === 0 ? (
                  <Empty description="暂无板块" />
                ) : (
                  <List
                    dataSource={blocks}
                    renderItem={(item) => (
                      <List.Item
                        onClick={() => setSelectedBlock(item.name)}
                        style={{ cursor: 'pointer', background: selectedBlock === item.name ? 'rgba(22,119,255,0.1)' : undefined }}
                      >
                        <List.Item.Meta
                          title={item.name}
                          description={`${item.count} 只股票`}
                        />
                      </List.Item>
                    )}
                    style={{ maxHeight: 400, overflow: 'auto' }}
                  />
                )}
              </Card>
            </Col>

            <Col xs={24} lg={8}>
              <Card title={<span>成分股</span>}>
                {loadingStocks ? (
                  <Skeleton active paragraph={{ rows: 6 }} title={false} />
                ) : !selectedBlock ? (
                  <Empty description="请先选择板块" />
                ) : blockStocks.length === 0 ? (
                  <Empty description="该板块暂无成分股" />
                ) : (
                  <Table
                    columns={[
                      { title: '代码', dataIndex: 'code', key: 'code', width: 100, render: (code: string) => <Button type="link" size="small">{code}</Button> },
                      { title: '名称', dataIndex: 'name', key: 'name', width: 120 },
                      { title: '交易所', dataIndex: 'exchange', key: 'exchange', width: 80, render: (ex: string) => <Tag color={ex === 'sh' ? 'blue' : ex === 'sz' ? 'green' : 'orange'}>{ex.toUpperCase()}</Tag> },
                    ]}
                    dataSource={blockStocks}
                    rowKey="code"
                    size="small"
                    pagination={{ pageSize: 10, size: 'small' }}
                  />
                )}
              </Card>
            </Col>
          </Row>
        </Tabs.TabPane>

        {/* Custom Stock Pool Tab */}
        <Tabs.TabPane
          key="custom"
          tab={
            <Space>
              <StockOutlined />
              <span>自定义股票池</span>
            </Space>
          }
        >
          <Row gutter={[16, 16]}>
            <Col xs={24} lg={5}>
              <StockPoolList
                pools={pools}
                currentPoolId={currentPoolId}
                onSelect={setCurrentPoolId}
                onAdd={handleAddPool}
                onEdit={handleEditPool}
                onDelete={handleDeletePool}
              />
            </Col>

            <Col xs={24} lg={19}>
              {currentPool ? (
                <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
                  <StockPoolFilterPanel
                    pool={currentPool}
                    onEdit={() => handleEditPool(currentPool)}
                  />
                  <DiscoverResearchPanel poolId={currentPool.id} poolName={currentPool.name} />
                  <StockListTable
                    title="股票列表"
                    dataSource={filteredStocks.map(s => ({
                      code: s.code,
                      name: s.name,
                      exchange: s.exchange,
                    }))}
                    total={filteredStocks.length}
                    loading={loadingPoolStocks}
                    columns={[
                      { title: '代码', dataIndex: 'code', key: 'code', width: 100, render: (code: string) => <Button type="link" size="small">{code}</Button> },
                      { title: '名称', dataIndex: 'name', key: 'name', width: 120 },
                      { title: '交易所', dataIndex: 'exchange', key: 'exchange', width: 80, render: (ex: string) => <Tag color={ex === 'sh' ? 'blue' : ex === 'sz' ? 'green' : 'orange'}>{ex.toUpperCase()}</Tag> },
                    ]}
                  />
                </Space>
              ) : (
                <Card>
                  <Empty description="请选择一个股票池" />
                </Card>
              )}
            </Col>
          </Row>
        </Tabs.TabPane>

        {/* All Stocks Tab */}
        <Tabs.TabPane
          key="allstocks"
          tab={
            <Space>
              <SearchOutlined />
              <span>全部股票</span>
            </Space>
          }
        >
          <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
            <Card
              title={
                <Space>
                  <FilterOutlined />
                  <span>筛选条件</span>
                  <Button size="small" type="primary" icon={<SaveOutlined />} onClick={() => setShowSaveAsPoolModal(true)}>
                    保存为股票池
                  </Button>
                  <Button size="small" onClick={resetStockInfoFilters}>重置</Button>
                </Space>
              }
              bodyStyle={{ padding: '16px' }}
            >
              <Row gutter={[16, 12]}>
                <Col xs={24} sm={12} lg={8}>
                  <Space direction="vertical" size={4}>
                    <Typography.Text type="secondary">流通市值(亿)</Typography.Text>
                    <Space>
                      <InputNumber
                        min={0}
                        style={{ width: 100 }}
                        placeholder="最小值"
                        value={stockInfoFilters.minMarketCap}
                        onChange={(v) => handleFilterChange('minMarketCap', v || undefined)}
                      />
                      <span>~</span>
                      <InputNumber
                        min={0}
                        style={{ width: 100 }}
                        placeholder="最大值"
                        value={stockInfoFilters.maxMarketCap}
                        onChange={(v) => handleFilterChange('maxMarketCap', v || undefined)}
                      />
                    </Space>
                  </Space>
                </Col>
                <Col xs={24} sm={12} lg={8}>
                  <Space direction="vertical" size={4}>
                    <Typography.Text type="secondary">股价(元)</Typography.Text>
                    <Space>
                      <InputNumber
                        min={0}
                        style={{ width: 100 }}
                        placeholder="最小值"
                        value={stockInfoFilters.minPrice}
                        onChange={(v) => handleFilterChange('minPrice', v || undefined)}
                      />
                      <span>~</span>
                      <InputNumber
                        min={0}
                        style={{ width: 100 }}
                        placeholder="最大值"
                        value={stockInfoFilters.maxPrice}
                        onChange={(v) => handleFilterChange('maxPrice', v || undefined)}
                      />
                    </Space>
                  </Space>
                </Col>
                <Col xs={24} sm={12} lg={8}>
                  <Space direction="vertical" size={4}>
                    <Typography.Text type="secondary">换手率(%)</Typography.Text>
                    <Space>
                      <InputNumber
                        min={0}
                        style={{ width: 100 }}
                        placeholder="最小值"
                        value={stockInfoFilters.minTurnoverRate}
                        onChange={(v) => handleFilterChange('minTurnoverRate', v || undefined)}
                      />
                      <span>~</span>
                      <InputNumber
                        min={0}
                        style={{ width: 100 }}
                        placeholder="最大值"
                        value={stockInfoFilters.maxTurnoverRate}
                        onChange={(v) => handleFilterChange('maxTurnoverRate', v || undefined)}
                      />
                    </Space>
                  </Space>
                </Col>
                <Col xs={24} sm={12} lg={8}>
                  <Space direction="vertical" size={4}>
                    <Typography.Text type="secondary">交易所</Typography.Text>
                    <Select
                      value={stockInfoFilters.exchange}
                      onChange={(v) => handleFilterChange('exchange', v)}
                      options={exchangeOptions}
                      style={{ width: 150 }}
                      size="small"
                    />
                  </Space>
                </Col>
                <Col xs={24} sm={12} lg={8}>
                  <Space direction="vertical" size={4}>
                    <Typography.Text type="secondary">排除ST</Typography.Text>
                    <Switch
                      checked={stockInfoFilters.excludeST}
                      onChange={(v) => handleFilterChange('excludeST', v)}
                    />
                  </Space>
                </Col>
              </Row>
            </Card>

            <StockListTable
              title="股票列表"
              dataSource={allStocks}
              total={allStocksTotal}
              loading={loadingAllStocks}
            />
          </Space>
        </Tabs.TabPane>
      </Tabs>

      {/* Add/Edit Pool Modal */}
      <StockPoolModal
        visible={showAddModal || showEditModal}
        onCancel={() => { setShowAddModal(false); setShowEditModal(false); }}
        onSave={handleSavePool}
        editingPool={editingPool}
      />

      {/* Save As Pool Modal */}
      <Modal
        title="保存为股票池"
        open={showSaveAsPoolModal}
        onCancel={() => { setShowSaveAsPoolModal(false); setSaveAsPoolName(''); }}
        footer={[
          <Button key="cancel" onClick={() => { setShowSaveAsPoolModal(false); setSaveAsPoolName(''); }}>取消</Button>,
          <Button key="save" type="primary" onClick={handleSaveAsPool} icon={<SaveOutlined />}>保存</Button>,
        ]}
        width={400}
      >
        <Input
          value={saveAsPoolName}
          onChange={(e) => setSaveAsPoolName(e.target.value)}
          placeholder="输入股票池名称"
          autoFocus
        />
        <Alert
          type="info"
          showIcon
          message="提示"
          description="当前筛选条件将作为股票池的条件保存"
          style={{ marginTop: 12, fontSize: '12px' }}
        />
      </Modal>
    </Space>
  );
}