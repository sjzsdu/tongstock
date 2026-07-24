import { Modal, Button, Card, Flex, Space, Typography, List, Spin } from 'antd';
import { PlusOutlined, CloseOutlined } from '@ant-design/icons';
import type { BlockInfo, StockItem } from '../../types/screen';

const { Text } = Typography;

interface BlockStocksModalProps {
  visible: boolean;
  onCancel: () => void;
  selectedBlock: BlockInfo | null;
  blockStocksWithNames: { code: string; name: string }[];
  blockStocksLoadingNames: boolean;
  stockList: StockItem[];
  onAddStock: (stock: { code: string; name: string }) => void;
  onRemoveStock: (code: string) => void;
  onAddAll: () => void;
  navigate: (path: string) => void;
}

export function BlockStocksModal({
  visible,
  onCancel,
  selectedBlock,
  blockStocksWithNames,
  blockStocksLoadingNames,
  stockList,
  onAddStock,
  onRemoveStock,
  onAddAll,
  navigate,
}: BlockStocksModalProps) {
  return (
    <Modal
      open={visible}
      onCancel={onCancel}
      footer={[
        <Button key="close" onClick={onCancel}>关闭</Button>,
        <Button key="add-all" type="primary" icon={<PlusOutlined />} onClick={onAddAll}>
          全部加入自选
        </Button>,
      ]}
      width={760}
      title={selectedBlock ? `${selectedBlock.name} 成分股` : '成分股'}
    >
      {blockStocksLoadingNames ? (
        <Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin /></Flex>
      ) : (
        <List
          grid={{ gutter: 12, column: 2 }}
          dataSource={blockStocksWithNames}
          renderItem={(stock) => {
            const inWatchlist = stockList.some((item) => item.code === stock.code);
            return (
              <List.Item>
                <Card size="small" hoverable onClick={() => {
                  onCancel();
                  navigate(`/stock/${stock.code}/chart`);
                }}>
                  <Flex justify="space-between" align="center" gap={12}>
                    <Space direction="vertical" size={2}>
                      <Text code>{stock.code}</Text>
                      <Text>{stock.name}</Text>
                    </Space>
                    <Button
                      size="small"
                      type={inWatchlist ? 'default' : 'primary'}
                      icon={inWatchlist ? <CloseOutlined /> : <PlusOutlined />}
                      onClick={(event) => {
                        event.stopPropagation();
                        if (inWatchlist) {
                          onRemoveStock(stock.code);
                        } else {
                          onAddStock({ code: stock.code, name: stock.name });
                        }
                      }}
                    >
                      {inWatchlist ? '移除' : '加入自选'}
                    </Button>
                  </Flex>
                </Card>
              </List.Item>
            );
          }}
        />
      )}
    </Modal>
  );
}