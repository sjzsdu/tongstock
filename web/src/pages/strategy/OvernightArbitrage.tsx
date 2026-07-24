import { useState, useEffect, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { SearchOutlined, EditOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Collapse, Empty, Flex, List, Space, Statistic, Tag, Typography, message } from 'antd';
import { api } from '../../api/client';
import type { OvernightCandidate } from '../../api/client';
import type { BlockInfo } from '../../types/screen';
import { useCodesCache } from '../../hooks/useCodesCache';
import { useWatchlist } from '../../hooks/useWatchlist';
import { useOvernightStrategy } from '../../hooks/useOvernightStrategy';
import { useOvernightTrade } from '../../hooks/useOvernightTrade';
import { OvernightResultTable } from '../../components/OvernightResultTable';
import { OvernightSourceModal } from '../../components/strategy/OvernightSourceModal';
import { OvernightTradeModal } from '../../components/strategy/OvernightTradeModal';
import { BlockStocksModal } from '../../components/screen/BlockStocksModal';
import { stockNamesFromCodesCache } from '../../utils/screen';
import { OVERNIGHT_CRITERIA } from '../../types/strategy';

const { Paragraph, Text, Title } = Typography;

type BlockListItem = { name: string; type: number; count: number };

export default function OvernightArbitrage() {
  const navigate = useNavigate();
  const [messageApi, contextHolder] = message.useMessage();

  // Hooks
  const { preloadCodesCache } = useCodesCache();
  const { stockList, addCodes, removeCode } = useWatchlist();
  const {
    sourceTab,
    setSourceTab,
    customPools,
    currentCustomPoolId,
    setCurrentCustomPoolId,
    addCustomPool,
    deleteCustomPool,
    updateCustomPool,
    marketCodes,
    marketLoading,
    results,
    failedCodes,
    total,
    hasLoaded,
    isOvernightTime,
    currentTime,
    loading,
    sortKey,
    sortAsc,
    sortedResults,
    doScreen,
    handleSortChange,
    loadMarketCodes,
  } = useOvernightStrategy();
  const { trades, tradingLoading, loadTrades, handleBuy, handleSell, confirmTrade } = useOvernightTrade();

  // Local state
  const [inputCode, setInputCode] = useState('');
  const [inputLoading, setInputLoading] = useState(false);
  const [showSourceModal, setShowSourceModal] = useState(false);
  const [showBlockModal, setShowBlockModal] = useState(false);
  const [showTradeModal, setShowTradeModal] = useState(false);
  const [currentTradeAction, setCurrentTradeAction] = useState<'buy' | 'sell'>('buy');
  const [currentTradeStock, setCurrentTradeStock] = useState<OvernightCandidate | null>(null);
  const [tradeReason, setTradeReason] = useState('');
  const [customPoolName, setCustomPoolName] = useState('');

  // Block related state
  const [blockFile, setBlockFile] = useState('block_zs.dat');
  const [blockData, setBlockData] = useState<BlockListItem[]>([]);
  const [selectedBlock, setSelectedBlock] = useState<BlockInfo | null>(null);
  const [blockLoading, setBlockLoading] = useState(false);
  const [blockStocksLoading, setBlockStocksLoading] = useState(false);
  const [blockSearch, setBlockSearch] = useState('');
  const [blockStocksWithNames, setBlockStocksWithNames] = useState<{ code: string; name: string }[]>([]);
  const [blockStocksLoadingNames, setBlockStocksLoadingNames] = useState(false);

  // Load trades after screen completes
  useEffect(() => {
    if (hasLoaded && sortedResults.length > 0) {
      void loadTrades(sortedResults.map((item) => item.code));
    }
  }, [hasLoaded, sortedResults, loadTrades]);

  // Block operations
  const loadBlocks = useCallback(async (file: string) => {
    setBlockLoading(true);
    try {
      const response = await api.blockList(file, undefined, true);
      setBlockData(response.blocks || []);
      setSelectedBlock(null);
    } catch {
      setBlockData([]);
    } finally {
      setBlockLoading(false);
    }
  }, []);

  useEffect(() => {
    if (sourceTab === 'block') {
      void loadBlocks(blockFile);
    }
  }, [sourceTab, blockFile, loadBlocks]);

  const loadBlockStocks = useCallback(async (block: BlockListItem) => {
    setBlockStocksLoading(true);
    try {
      const response = await api.blockShow(block.name, undefined, blockFile);
      if (response.stocks && response.stocks.length > 0) {
        const stocksWithNames = response.stocks.map((stock) => ({
          code: stock.code,
          name: stock.name?.trim() ? stock.name : stock.code,
        }));
        setSelectedBlock({
          name: block.name,
          type: block.type,
          count: block.count,
          stocks: response.stocks.map((stock) => stock.code),
          stocksWithNames,
        });
      } else {
        setSelectedBlock({ name: block.name, type: block.type, count: block.count });
      }
    } catch {
      setSelectedBlock({ name: block.name, type: block.type, count: block.count });
    } finally {
      setBlockStocksLoading(false);
    }
  }, [blockFile]);

  const handleSelectBlock = useCallback((block: BlockInfo) => {
    if (selectedBlock?.name === block.name) {
      setSelectedBlock(null);
      return;
    }
    void loadBlockStocks(block);
  }, [loadBlockStocks, selectedBlock]);

  // Resolved codes
  const resolvedCodes = useMemo(() => {
    if (sourceTab === 'block' && selectedBlock?.stocks) {
      return selectedBlock.stocks.join(',');
    }
    if (sourceTab === 'market' || sourceTab === 'custom') {
      return marketCodes.map((item) => item.code).join(',');
    }
    return stockList.map((stock) => stock.code).join(',');
  }, [selectedBlock, sourceTab, stockList, marketCodes]);

  // Add codes from input
  const onAddCodes = useCallback(async () => {
    const codes = inputCode
      .split(/[, \n]+/)
      .map((value: string) => value.trim().toUpperCase())
      .filter(Boolean);

    if (codes.length === 0) return;

    const invalidCodes = codes.filter((value: string) => !/^\d{6}$/.test(value));
    if (invalidCodes.length > 0) {
      messageApi.error(`无效的股票代码: ${invalidCodes.join(', ')}`);
      return;
    }

    const existingCodes = codes.filter((value: string) => stockList.some((stock) => stock.code === value));
    if (existingCodes.length > 0) {
      messageApi.warning(`股票已存在: ${existingCodes.join(', ')}`);
    }

    const newCodes = codes.filter((value: string) => !stockList.some((stock) => stock.code === value));
    if (newCodes.length === 0) {
      setInputCode('');
      return;
    }

    setInputLoading(true);
    try {
      const cache = await preloadCodesCache();
      await addCodes(newCodes, cache);
      const resolved = stockNamesFromCodesCache(newCodes, cache);
      messageApi.success(resolved.length === 1 ? `已添加 ${resolved[0].name}` : `已添加 ${resolved.length} 只股票`);
    } catch {
      messageApi.error('获取股票信息失败');
    } finally {
      setInputLoading(false);
      setInputCode('');
    }
  }, [inputCode, stockList, preloadCodesCache, addCodes]);

  // Open block modal
  const openBlockModal = useCallback(async () => {
    if (!selectedBlock?.stocks?.length) return;
    setShowBlockModal(true);

    if (selectedBlock.stocksWithNames?.length) {
      setBlockStocksWithNames(selectedBlock.stocksWithNames);
      return;
    }

    setBlockStocksLoadingNames(true);
    try {
      const cache = await preloadCodesCache();
      const rows = stockNamesFromCodesCache(selectedBlock.stocks, cache);
      const byCode = new Map(rows.map((row) => [row.code, row.name]));
      const filled = selectedBlock.stocks.map((code: string) => ({
        code,
        name: byCode.get(code) ?? code,
      }));
      setBlockStocksWithNames(filled);
    } finally {
      setBlockStocksLoadingNames(false);
    }
  }, [selectedBlock, preloadCodesCache]);

  // Add all block stocks to watchlist
  const addAllBlockStocksToWatchlist = useCallback(() => {
    const newStocks = blockStocksWithNames
      .filter((stock) => !stockList.some((watch) => watch.code === stock.code))
      .map((stock) => ({ code: stock.code, name: stock.name }));

    if (newStocks.length === 0) {
      messageApi.warning('所有股票已存在');
      return;
    }

    newStocks.forEach((stock) => api.watchlistAdd(stock.code, stock.name).catch(() => {}));
    messageApi.success(`已添加 ${newStocks.length} 只股票`);
  }, [blockStocksWithNames, stockList]);

  // Trade handlers
  const onHandleBuy = useCallback((result: OvernightCandidate) => {
    const { canBuy, reason } = handleBuy(result);
    if (!canBuy && reason) {
      messageApi.warning(reason);
      return;
    }
    setCurrentTradeAction('buy');
    setCurrentTradeStock(result);
    setTradeReason('');
    setShowTradeModal(true);
  }, [handleBuy]);

  const onHandleSell = useCallback((result: OvernightCandidate) => {
    const { canSell, reason } = handleSell(result);
    if (!canSell && reason) {
      messageApi.warning(reason);
      return;
    }
    setCurrentTradeAction('sell');
    setCurrentTradeStock(result);
    setTradeReason('');
    setShowTradeModal(true);
  }, [handleSell]);

  const onConfirmTrade = useCallback(async () => {
    if (!currentTradeStock) return;
    const { success, message: msg } = await confirmTrade(currentTradeStock, currentTradeAction, tradeReason);
    if (success) {
      messageApi.success(msg);
      setShowTradeModal(false);
    } else {
      messageApi.error(msg);
    }
  }, [currentTradeStock, currentTradeAction, tradeReason, confirmTrade]);

  // Custom pool handlers
  const onAddCustomPool = useCallback(() => {
    addCustomPool(customPoolName);
    setCustomPoolName('');
  }, [customPoolName, addCustomPool]);

  const currentCustomPool = customPools.find(p => p.id === currentCustomPoolId);
  const customMarketCapMin = currentCustomPool?.minMarketCap || 50;
  const customMarketCapMax = currentCustomPool?.maxMarketCap || 200;

  return (
    <>
      {contextHolder}
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Flex justify="space-between" align="center" wrap="wrap" gap={12}>
          <div>
            <Title level={3} style={{ margin: 0 }}>杨永兴隔夜套利法</Title>
            <Paragraph type="secondary" style={{ marginBottom: 0 }}>
              尾盘14:30后选股，次日早盘卖出。筛选标准：涨幅3%-5%、量比&gt;1、换手率5%-10%、流通市值50-200亿、近20日涨停、MA多头排列、股价站在均价线上方。
            </Paragraph>
          </div>
        </Flex>

        {!isOvernightTime && (
          <Alert
            type="warning"
            showIcon
            message="当前时间不是最佳筛选时间"
            description="杨永兴策略建议在14:30之后进行选股，此时全天走势基本定型，能更好判断主力意图。"
          />
        )}

        <Card size="small" hoverable style={{ cursor: 'pointer' }} onClick={() => setShowSourceModal(true)}>
          <Flex justify="space-between" align="center">
            <Space>
              <Text type="secondary">股票池：</Text>
              {sourceTab === 'watchlist' ? (
                <Text strong>{stockList.length} 只自选股</Text>
              ) : sourceTab === 'block' && selectedBlock ? (
                <Text strong>{selectedBlock.name}（{selectedBlock.stocks?.length || selectedBlock.count} 只）</Text>
              ) : sourceTab === 'market' ? (
                <Text strong>全市场（{marketLoading ? '加载中...' : `${marketCodes.length}只`}）</Text>
              ) : sourceTab === 'custom' ? (
                <Text strong>自定义（{customMarketCapMin}-{customMarketCapMax}亿，{marketLoading ? '加载中...' : `${marketCodes.length}只`}）</Text>
              ) : (
                <Text type="secondary">未选择股票来源</Text>
              )}
            </Space>
            <Button size="small" icon={<EditOutlined />}>更换</Button>
          </Flex>
        </Card>

        <Card title="选股标准说明" size="small">
          <Space direction="vertical" size={12} style={{ width: '100%' }}>
            <Flex gap={16} wrap>
              {OVERNIGHT_CRITERIA.map((item) => (
                <div key={item.key} style={{ display: 'flex', gap: 8 }}>
                  <Tag color="green" style={{ width: 20, textAlign: 'center' }}>✓</Tag>
                  <Text>{item.label}</Text>
                </div>
              ))}
            </Flex>
            <Alert
              type="info"
              showIcon
              message="卖出铁律"
              description="次日早盘10:00前必须全部清仓，无论盈亏，除非一字/缩量涨停可留。追求1%-3%溢价即可兑现。"
            />
          </Space>
        </Card>

        <Button
          type="primary"
          icon={<SearchOutlined />}
          loading={loading}
          onClick={() => void doScreen(resolvedCodes)}
          disabled={!resolvedCodes.trim()}
          size="large"
        >
          {loading ? '筛选中...' : `开始筛选（${resolvedCodes.split(',').filter(Boolean).length}只股票）`}
        </Button>

        {hasLoaded && (
          <Card size="small" style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.08), rgba(14,165,233,0.06))' }}>
            <Flex justify="space-between" align="center" wrap="wrap" gap={16}>
              <Space size={24}>
                <Statistic title="扫描总数" value={total} suffix="只" style={{ fontSize: 13 }} />
                <Statistic title="最终候选" value={results.length} suffix="只" style={{ fontSize: 13 }} />
              </Space>
              <Space size={[6, 6]} wrap>
                <Tag color="blue">当前时间: {currentTime}</Tag>
                <Tag color={isOvernightTime ? 'green' : 'orange'}>{isOvernightTime ? '✓ 最佳筛选时间' : '建议14:30后筛选'}</Tag>
              </Space>
            </Flex>
          </Card>
        )}

        {hasLoaded && failedCodes.length > 0 && (
          <Collapse
            items={[{
              key: 'failed',
              label: <Space><Tag color="error">失败/淘汰 {failedCodes.length}</Tag><Text type="secondary">点击查看详情</Text></Space>,
              children: (
                <Space direction="vertical" size={8} style={{ width: '100%' }}>
                  <List
                    size="small"
                    dataSource={failedCodes.slice(0, 50)}
                    renderItem={(item) => (
                      <List.Item>
                        <Flex justify="space-between" align="center" style={{ width: '100%' }}>
                          <Space>
                            <Text code>{item.code}</Text>
                          </Space>
                          <Text type="danger" style={{ fontSize: 12 }}>{item.reason}</Text>
                        </Flex>
                      </List.Item>
                    )}
                  />
                  {failedCodes.length > 50 && (
                    <Text type="secondary" style={{ fontSize: 12, textAlign: 'center', display: 'block' }}>
                      仅显示前50条，共{failedCodes.length}条
                    </Text>
                  )}
                </Space>
              ),
            }]}
          />
        )}

        {sortedResults.length > 0 ? (
          <OvernightResultTable
            results={sortedResults}
            sortKey={sortKey}
            sortAsc={sortAsc}
            onSortChange={handleSortChange}
            navigate={navigate}
            trades={trades}
            tradingLoading={tradingLoading}
            handleBuy={onHandleBuy}
            handleSell={onHandleSell}
          />
        ) : !loading ? (
          <Card>
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={hasLoaded ? '当前筛选条件下没有命中结果' : '选择股票来源后点击"开始筛选"'}
            />
          </Card>
        ) : null}
      </Space>

      <BlockStocksModal
        visible={showBlockModal}
        onCancel={() => setShowBlockModal(false)}
        selectedBlock={selectedBlock}
        blockStocksWithNames={blockStocksWithNames}
        blockStocksLoadingNames={blockStocksLoadingNames}
        stockList={stockList}
        onAddStock={(stock) => api.watchlistAdd(stock.code, stock.name).then(() => messageApi.success(`已添加 ${stock.name}`)).catch(() => {})}
        onRemoveStock={removeCode}
        onAddAll={addAllBlockStocksToWatchlist}
        navigate={navigate}
      />

      <OvernightSourceModal
        visible={showSourceModal}
        onCancel={() => setShowSourceModal(false)}
        sourceTab={sourceTab}
        onSourceTabChange={setSourceTab}
        stockList={stockList}
        onRemoveStock={removeCode}
        inputCode={inputCode}
        onInputCodeChange={setInputCode}
        onAddCodes={onAddCodes}
        inputLoading={inputLoading}
        blockFile={blockFile}
        onBlockFileChange={setBlockFile}
        blockSearch={blockSearch}
        onBlockSearchChange={setBlockSearch}
        blockData={blockData}
        blockLoading={blockLoading}
        selectedBlock={selectedBlock}
        onSelectBlock={handleSelectBlock}
        blockStocksLoading={blockStocksLoading}
        onOpenBlockModal={openBlockModal}
        marketCodes={marketCodes}
        marketLoading={marketLoading}
        customPools={customPools}
        currentCustomPoolId={currentCustomPoolId}
        onCurrentCustomPoolIdChange={setCurrentCustomPoolId}
        customPoolName={customPoolName}
        onCustomPoolNameChange={setCustomPoolName}
        onAddCustomPool={onAddCustomPool}
        onDeleteCustomPool={deleteCustomPool}
        onUpdateCustomPool={updateCustomPool}
        onLoadMarketCodes={loadMarketCodes}
        navigate={navigate}
      />

      <OvernightTradeModal
        visible={showTradeModal}
        onCancel={() => setShowTradeModal(false)}
        onConfirm={onConfirmTrade}
        currentTradeAction={currentTradeAction}
        currentTradeStock={currentTradeStock}
        tradeReason={tradeReason}
        onTradeReasonChange={setTradeReason}
        tradingLoading={tradingLoading}
        trades={trades}
      />
    </>
  );
}