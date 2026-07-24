import { Modal, Button, Segmented, Input, Flex, Space, Typography, List, Empty, Tag, Alert } from 'antd';
import { SearchOutlined, SyncOutlined, EyeOutlined, CloseOutlined } from '@ant-design/icons';
import type { SourceTab, StockItem, BlockInfo } from '../../types/screen';
import { ALL_BLOCK_FILES } from '../../types/screen';

const { Text } = Typography;

interface SourceSelectorModalProps {
  visible: boolean;
  onCancel: () => void;
  sourceTab: SourceTab;
  onSourceTabChange: (value: SourceTab) => void;
  stockList: StockItem[];
  onStockListChange: React.Dispatch<React.SetStateAction<StockItem[]>>;
  inputCode: string;
  onInputCodeChange: (value: string) => void;
  onAddCodes: () => void;
  inputLoading: boolean;
  syncLoading: boolean;
  onSync: () => void;
  blockFile: string;
  onBlockFileChange: (value: string) => void;
  blockSearch: string;
  onBlockSearchChange: (value: string) => void;
  blockData: BlockInfo[];
  blockLoading: boolean;
  selectedBlock: BlockInfo | null;
  onSelectBlock: (block: BlockInfo) => void;
  blockStocksLoading: boolean;
  onOpenBlockModal: () => void;
  navigate: (path: string) => void;
  onRemoveStock: (code: string) => void;
}

export function SourceSelectorModal({
  visible,
  onCancel,
  sourceTab,
  onSourceTabChange,
  stockList,
  inputCode,
  onInputCodeChange,
  onAddCodes,
  inputLoading,
  onSync,
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
  navigate,
  onRemoveStock,
}: SourceSelectorModalProps) {
  const sortedBlocks = [...blockData].sort((a, b) => b.count - a.count);
  const filteredBlocks = !blockSearch
    ? sortedBlocks
    : sortedBlocks.filter((block) => block.name.toLowerCase().includes(blockSearch.toLowerCase()));

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
      <Segmented<SourceTab>
        value={sourceTab}
        onChange={(value) => onSourceTabChange(value)}
        options={[
          { label: '自选股', value: 'watchlist' },
          { label: '板块', value: 'block' },
        ]}
        style={{ marginBottom: 16 }}
      />

      {sourceTab === 'watchlist' ? (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Input
            prefix={<SearchOutlined />}
            value={inputCode}
            onChange={(event) => onInputCodeChange(event.target.value)}
            onPressEnter={() => void onAddCodes()}
            placeholder="输入股票代码，支持逗号/空格分隔"
            suffix={inputLoading ? <SyncOutlined spin /> : null}
          />
          <Flex justify="space-between" align="center" gap={8}>
            <Text type="secondary">共 {stockList.length} 只股票</Text>
            <Button
              size="small"
              icon={<SyncOutlined />}
              disabled={stockList.length === 0}
              onClick={() => void onSync()}
            >
              同步日K
            </Button>
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
      ) : (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Segmented
            block
            value={blockFile}
            onChange={(value) => {
              onBlockFileChange(String(value));
            }}
            options={ALL_BLOCK_FILES.map((item) => ({ label: item.label, value: item.file }))}
          />
          <Input
            prefix={<SearchOutlined />}
            value={blockSearch}
            onChange={(event) => onBlockSearchChange(event.target.value)}
            placeholder="搜索板块..."
          />
          <div style={{ maxHeight: 280, overflow: 'auto' }}>
            {blockLoading ? (
              <Flex justify="center" align="center" style={{ minHeight: 240 }}><SyncOutlined spin /></Flex>
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
                  <Button size="small" icon={<EyeOutlined />} onClick={() => void onOpenBlockModal()} disabled={!selectedBlock.stocks?.length}>
                    查看成分股
                  </Button>
                </Space>
              }
            />
          )}
        </Space>
      )}
    </Modal>
  );
}