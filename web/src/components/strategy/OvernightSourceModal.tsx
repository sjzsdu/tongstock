import { useState } from 'react';
import { SearchOutlined, PlusOutlined, InfoCircleOutlined, CloseOutlined } from '@ant-design/icons';
import { Modal, Button, Segmented, Input, Flex, Space, Typography, List, Empty, Tag, Alert, Select, Pagination, Spin } from 'antd';
import type { StockItem, BlockInfo } from '../../types/screen';
import type { CustomPool, OvernightSourceTab } from '../../types/strategy';

const { Text } = Typography;

interface OvernightSourceModalProps {
  visible: boolean;
  onCancel: () => void;
  sourceTab: OvernightSourceTab;
  onSourceTabChange: (value: OvernightSourceTab) => void;
  stockList: StockItem[];
  onRemoveStock: (code: string) => void;
  inputCode: string;
  onInputCodeChange: (value: string) => void;
  onAddCodes: () => void;
  inputLoading: boolean;
  blockFile: string;
  onBlockFileChange: (value: string) => void;
  blockSearch: string;
  onBlockSearchChange: (value: string) => void;
  blockData: { name: string; type: number; count: number }[];
  blockLoading: boolean;
  selectedBlock: BlockInfo | null;
  onSelectBlock: (block: BlockInfo) => void;
  blockStocksLoading: boolean;
  onOpenBlockModal: () => void;
  marketCodes: { code: string; name: string }[];
  marketLoading: boolean;
  customPools: CustomPool[];
  currentCustomPoolId: string;
  onCurrentCustomPoolIdChange: (value: string) => void;
  customPoolName: string;
  onCustomPoolNameChange: (value: string) => void;
  onAddCustomPool: () => void;
  onDeleteCustomPool: (poolId: string) => void;
  onUpdateCustomPool: (poolId: string, updates: Partial<CustomPool>) => void;
  onLoadMarketCodes: () => void;
  navigate: (path: string) => void;
}

export function OvernightSourceModal({
  visible,
  onCancel,
  sourceTab,
  onSourceTabChange,
  stockList,
  onRemoveStock,
  inputCode,
  onInputCodeChange,
  onAddCodes,
  inputLoading,
  blockFile,
  onBlockFileChange,
  blockSearch,
  onBlockSearchChange,
  blockData,
  blockLoading,
  selectedBlock,
  onSelectBlock,
  blockStocksLoading,
  onOpenBlockModal,
  marketCodes,
  marketLoading,
  customPools,
  currentCustomPoolId,
  onCurrentCustomPoolIdChange,
  customPoolName,
  onCustomPoolNameChange,
  onAddCustomPool,
  onDeleteCustomPool,
  onUpdateCustomPool,
  onLoadMarketCodes,
  navigate,
}: OvernightSourceModalProps) {
  const [customPage, setCustomPage] = useState(1);
  const customPageSize = 20;

  const sortedBlocks = [...blockData].sort((a, b) => b.count - a.count);
  const filteredBlocks = !blockSearch
    ? sortedBlocks
    : sortedBlocks.filter((block) => block.name.toLowerCase().includes(blockSearch.toLowerCase()));

  const currentCustomPool = customPools.find(p => p.id === currentCustomPoolId);

  return (
    <Modal
      title="选择股票来源"
      open={visible}
      onCancel={onCancel}
      footer={[
        <Button key="cancel" onClick={onCancel}>关闭</Button>,
        <Button key="confirm" type="primary" onClick={onCancel}>确定</Button>,
      ]}
      width={760}
      style={{ maxHeight: '70vh' }}
    >
      <Segmented<OvernightSourceTab>
        value={sourceTab}
        onChange={(value) => onSourceTabChange(value)}
        options={[
          { label: '自选股', value: 'watchlist' },
          { label: '板块', value: 'block' },
          { label: '全市场', value: 'market' },
          { label: '自定义', value: 'custom' },
        ]}
        style={{ marginBottom: 16 }}
      />

      {sourceTab === 'watchlist' && (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Input
            prefix={<SearchOutlined />}
            value={inputCode}
            onChange={(event) => onInputCodeChange(event.target.value)}
            onPressEnter={() => void onAddCodes()}
            placeholder="输入股票代码，支持逗号/空格分隔"
            suffix={inputLoading ? <Spin size="small" /> : null}
          />
          <Flex justify="space-between" align="center" gap={8}>
            <Text type="secondary">共 {stockList.length} 只股票</Text>
          </Flex>
          <div style={{ maxHeight: 280, overflow: 'auto' }}>
            {stockList.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="输入股票代码后回车添加" />
            ) : (
              <List
                size="small"
                dataSource={stockList}
                renderItem={(stock) => (
                  <List.Item
                    style={{ cursor: 'pointer' }}
                    onClick={() => {
                      onCancel();
                      navigate(`/stock/${stock.code}/chart`);
                    }}
                    actions={[
                      <Button
                        key="remove"
                        type="text"
                        danger
                        icon={<CloseOutlined />}
                        onClick={(event) => {
                          event.stopPropagation();
                          onRemoveStock(stock.code);
                        }}
                      />,
                    ]}
                  >
                    <List.Item.Meta
                      title={<Space><Text code>{stock.code}</Text><Text>{stock.name || '-'}</Text></Space>}
                    />
                  </List.Item>
                )}
              />
            )}
          </div>
        </Space>
      )}

      {sourceTab === 'block' && (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Segmented
            block
            value={blockFile}
            onChange={(value) => {
              onBlockFileChange(String(value));
            }}
            options={[
              { label: '综合板块', value: 'block_zs.dat' },
              { label: '概念板块', value: 'block_gn.dat' },
              { label: '风格板块', value: 'block_fg.dat' },
            ]}
          />
          <Input
            prefix={<SearchOutlined />}
            value={blockSearch}
            onChange={(event) => onBlockSearchChange(event.target.value)}
            placeholder="搜索板块..."
          />
          <div style={{ maxHeight: 280, overflow: 'auto' }}>
            {blockLoading ? (
              <Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin /></Flex>
            ) : (
              <List
                size="small"
                dataSource={filteredBlocks}
                renderItem={(block) => (
                  <List.Item
                    style={{
                      cursor: 'pointer',
                      borderRadius: 8,
                      paddingInline: 12,
                      background: selectedBlock?.name === block.name ? 'var(--ant-color-primary-bg)' : undefined,
                    }}
                    onClick={() => onSelectBlock(block)}
                  >
                    <Flex justify="space-between" align="center" style={{ width: '100%' }}>
                      <Text ellipsis style={{ maxWidth: 180 }}>{block.name}</Text>
                      <Tag>{block.count}只</Tag>
                    </Flex>
                  </List.Item>
                )}
              />
            )}
          </div>
          {selectedBlock && (
            <Alert
              type="info"
              showIcon
              message={`已选 ${selectedBlock.name}`}
              description={
                <Space wrap>
                  <Text>{blockStocksLoading ? '加载成分股中...' : `${selectedBlock.stocks?.length || selectedBlock.count} 只股票`}</Text>
                  <Button size="small" icon={<InfoCircleOutlined />} onClick={() => void onOpenBlockModal()} disabled={!selectedBlock.stocks?.length}>
                    查看成分股
                  </Button>
                </Space>
              }
            />
          )}
        </Space>
      )}

      {sourceTab === 'market' && (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message="全市场股票池"
            description="包含上海、深圳、北京三大交易所的所有股票，数据量大，筛选时间较长。"
          />
          {marketLoading ? (
            <Flex justify="center" align="center" style={{ minHeight: 200 }}><Spin tip="正在获取全市场股票列表..." /></Flex>
          ) : (
            <List
              size="small"
              dataSource={marketCodes.slice(0, 50)}
              renderItem={(item) => (
                <List.Item>
                  <Space><Text code>{item.code}</Text><Text>{item.name}</Text></Space>
                </List.Item>
              )}
            />
          )}
          {marketCodes.length > 50 && (
            <Text type="secondary" style={{ fontSize: 12, textAlign: 'center', display: 'block' }}>
              仅显示前50只，共{marketCodes.length}只
            </Text>
          )}
        </Space>
      )}

      {sourceTab === 'custom' && (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message="自定义股票池"
            description="根据流通市值范围筛选股票，适合杨永兴策略的50-200亿市值要求。"
          />
          <Flex justify="space-between" align="center" style={{ marginBottom: 12 }}>
            <Select
              value={currentCustomPoolId}
              onChange={(value) => { onCurrentCustomPoolIdChange(value); setCustomPage(1); }}
              style={{ width: 200 }}
              options={customPools.map(p => ({ value: p.id, label: p.name }))}
            />
            <Space>
              <Input
                style={{ width: 150 }}
                placeholder="新股票池名称"
                value={customPoolName}
                onChange={(e) => onCustomPoolNameChange(e.target.value)}
                prefix={<PlusOutlined />}
                onPressEnter={onAddCustomPool}
              />
              <Button type="primary" size="small" onClick={onAddCustomPool}>添加</Button>
              <Button
                size="small"
                danger
                onClick={() => currentCustomPoolId && onDeleteCustomPool(currentCustomPoolId)}
                disabled={customPools.length <= 1}
              >删除</Button>
            </Space>
          </Flex>
          <Flex gap={16}>
            <Space direction="vertical" style={{ flex: 1 }}>
              <Text type="secondary">最小流通市值（亿）</Text>
              <Input
                type="number"
                value={currentCustomPool?.minMarketCap || 0}
                onChange={(e) => {
                  const val = Number(e.target.value) || 0;
                  onUpdateCustomPool(currentCustomPoolId, { minMarketCap: val });
                }}
                min={0}
                max={10000}
              />
            </Space>
            <Space direction="vertical" style={{ flex: 1 }}>
              <Text type="secondary">最大流通市值（亿）</Text>
              <Input
                type="number"
                value={currentCustomPool?.maxMarketCap || 0}
                onChange={(e) => {
                  const val = Number(e.target.value) || 0;
                  onUpdateCustomPool(currentCustomPoolId, { maxMarketCap: val });
                }}
                min={0}
                max={10000}
              />
            </Space>
          </Flex>
          <Button type="primary" size="small" onClick={() => void onLoadMarketCodes()} loading={marketLoading}>
            {marketLoading ? '刷新中...' : '刷新股票列表'}
          </Button>
          {marketLoading ? (
            <Flex justify="center" align="center" style={{ minHeight: 150 }}><Spin tip="正在获取股票列表..." /></Flex>
          ) : (
            <>
              <List
                size="small"
                dataSource={marketCodes.slice((customPage - 1) * customPageSize, customPage * customPageSize)}
                renderItem={(item) => (
                  <List.Item>
                    <Space><Text code>{item.code}</Text><Text>{item.name}</Text></Space>
                  </List.Item>
                )}
              />
              {marketCodes.length > customPageSize && (
                <Pagination
                  current={customPage}
                  pageSize={customPageSize}
                  total={marketCodes.length}
                  onChange={(page) => setCustomPage(page)}
                  style={{ marginTop: 12, textAlign: 'center' }}
                />
              )}
            </>
          )}
        </Space>
      )}
    </Modal>
  );
}