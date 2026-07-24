import { useRef, useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  SearchOutlined,
  DownloadOutlined,
  SaveOutlined,
  EditOutlined,
  InfoCircleOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Divider,
  Empty,
  Flex,
  List,
  Segmented,
  Select,
  Tooltip,
  Space,
  Tag,
  Typography,
  message,
} from 'antd';
import { api } from '../api/client';
import type { ScreenResult, ScreenCodeStatus } from '../types/api';
import { KTYPE_OPTIONS, SIGNAL_OPTIONS } from '../types/screen';
import { useCodesCache } from '../hooks/useCodesCache';
import { useWatchlist } from '../hooks/useWatchlist';
import { useTrade } from '../hooks/useTrade';
import { useScreen } from '../hooks/useScreen';
import { VirtualResultTable } from '../components/VirtualResultTable';
import { TradeModal } from '../components/screen/TradeModal';
import { SourceSelectorModal } from '../components/screen/SourceSelectorModal';
import { SignalHelpModal } from '../components/screen/SignalHelpModal';
import { SyncResultModal } from '../components/screen/SyncResultModal';
import { BlockStocksModal } from '../components/screen/BlockStocksModal';
import { exportCsv, stockNamesFromCodesCache, formatPercent, getMaTrend } from '../utils/screen';

const { Paragraph, Text, Title } = Typography;

export default function Screen() {
  const navigate = useNavigate();
  const tableContainerRef = useRef<HTMLDivElement>(null);
  const [messageApi, contextHolder] = message.useMessage();

  // Hooks
  const { preloadCodesCache } = useCodesCache();
  const { stockList, setStockList, addCodes, removeCode, syncWatchlistDaily } = useWatchlist();
  const { trades, tradingLoading, loadTrades, handleBuy, handleSell, confirmTrade } = useTrade();
  const {
    sourceTab,
    setSourceTab,
    ktype,
    setKtype,
    selectedSignals,
    setSelectedSignals,
    results,
    failedCodes,
    cappedInfo,
    hasScreenLoaded,
    total,
    loading,
    sortKey,
    sortAsc,
    filteredResults,
    sortedResults,
    signalCounts,
    blockFile,
    setBlockFile,
    blockData,
    selectedBlock,
    blockLoading,
    blockStocksLoading,
    blockSearch,
    setBlockSearch,
    handleSelectBlock,
    syncResult,
    setSyncResult,
    doScreen,
    retryFailed,
    handleSortChange,
  } = useScreen();

  // Local state
  const [inputCode, setInputCode] = useState('');
  const [inputLoading, setInputLoading] = useState(false);
  const [showHelpModal, setShowHelpModal] = useState(false);
  const [showSourceModal, setShowSourceModal] = useState(false);
  const [showBlockModal, setShowBlockModal] = useState(false);
  const [showTradeModal, setShowTradeModal] = useState(false);
  const [currentTradeAction, setCurrentTradeAction] = useState<'buy' | 'sell'>('buy');
  const [currentTradeStock, setCurrentTradeStock] = useState<ScreenResult | null>(null);
  const [tradeReason, setTradeReason] = useState('');
  const [blockStocksWithNames, setBlockStocksWithNames] = useState<{ code: string; name: string }[]>([]);
  const [blockStocksLoadingNames, setBlockStocksLoadingNames] = useState(false);

  // Resolved codes
  const resolvedCodes = useCallback(() => {
    if (sourceTab === 'block' && selectedBlock?.stocks) {
      return selectedBlock.stocks.join(',');
    }
    return stockList.map((stock) => stock.code).join(',');
  }, [selectedBlock, sourceTab, stockList]);

  // Auto screen on mount
  useEffect(() => {
    const codes = resolvedCodes().trim();
    if (codes && !hasScreenLoaded && !loading) {
      void doScreen(codes);
    }
  }, [resolvedCodes, hasScreenLoaded, loading, doScreen]);

  // Load trades after screen completes
  useEffect(() => {
    if (hasScreenLoaded && sortedResults.length > 0) {
      void loadTrades(sortedResults.map((item) => item.code));
    }
  }, [hasScreenLoaded, sortedResults, loadTrades]);

  // Handle buy action
  const onHandleBuy = (result: ScreenResult) => {
    const { canBuy, reason } = handleBuy(result);
    if (!canBuy && reason) {
      messageApi.warning(reason);
      return;
    }
    setCurrentTradeAction('buy');
    setCurrentTradeStock(result);
    setTradeReason('');
    setShowTradeModal(true);
  };

  // Handle sell action
  const onHandleSell = (result: ScreenResult) => {
    const { canSell, reason } = handleSell(result);
    if (!canSell && reason) {
      messageApi.warning(reason);
      return;
    }
    setCurrentTradeAction('sell');
    setCurrentTradeStock(result);
    setTradeReason('');
    setShowTradeModal(true);
  };

  // Confirm trade
  const onConfirmTrade = async () => {
    if (!currentTradeStock) return;
    const { success, message: msg } = await confirmTrade(currentTradeStock, currentTradeAction, ktype, tradeReason);
    if (success) {
      messageApi.success(msg);
      setShowTradeModal(false);
    } else {
      messageApi.error(msg);
    }
  };

  // Add codes from input
  const onAddCodes = async () => {
    const codes = inputCode
      .split(/[, \n]+/)
      .map((value) => value.trim().toUpperCase())
      .filter(Boolean);

    if (codes.length === 0) return;

    const invalidCodes = codes.filter((value) => !/^\d{6}$/.test(value));
    if (invalidCodes.length > 0) {
      messageApi.error(`无效的股票代码: ${invalidCodes.join(', ')}`);
      return;
    }

    const existingCodes = codes.filter((value) => stockList.some((stock) => stock.code === value));
    if (existingCodes.length > 0) {
      messageApi.warning(`股票已存在: ${existingCodes.join(', ')}`);
    }

    const newCodes = codes.filter((value) => !stockList.some((stock) => stock.code === value));
    if (newCodes.length === 0) {
      setInputCode('');
      return;
    }

    setInputLoading(true);
    try {
      const cache = await preloadCodesCache();
      const resolved = stockNamesFromCodesCache(newCodes, cache);
      if (resolved.length === 0) {
        messageApi.error('股票代码不存在');
      } else {
        await addCodes(newCodes, cache);
        messageApi.success(resolved.length === 1 ? `已添加 ${resolved[0].name}` : `已添加 ${resolved.length} 只股票`);
      }
    } catch {
      messageApi.error('获取股票信息失败');
    } finally {
      setInputLoading(false);
      setInputCode('');
    }
  };

  // Open block modal
  const openBlockModal = async () => {
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
      const filled = selectedBlock.stocks.map((code) => ({
        code,
        name: byCode.get(code) ?? code,
      }));
      setBlockStocksWithNames(filled);
    } finally {
      setBlockStocksLoadingNames(false);
    }
  };

  // Add all block stocks to watchlist
  const addAllBlockStocksToWatchlist = () => {
    const newStocks = blockStocksWithNames
      .filter((stock) => !stockList.some((watch) => watch.code === stock.code))
      .map((stock) => ({ code: stock.code, name: stock.name }));

    if (newStocks.length === 0) {
      messageApi.warning('所有股票已存在');
      return;
    }

    setStockList((previous) => [...previous, ...newStocks]);
    newStocks.forEach((stock) => api.watchlistAdd(stock.code, stock.name).catch(() => {}));
    messageApi.success(`已添加 ${newStocks.length} 只股票`);
  };

  // Export screen results
  const exportScreenResults = () => {
    exportCsv(
      `tongstock-screen-${new Date().toISOString().slice(0, 10)}.csv`,
      ['代码', '名称', '收盘', '涨跌幅', 'MA趋势', '信号'],
      sortedResults.map((result) => {
        const maTrend = getMaTrend(result);
        const close = result.last?.Close || 0;
        const open = result.last?.Open || close;
        const changePct = open > 0 ? ((close - open) / open) * 100 : 0;
        return [
          result.code,
          result.name || '',
          String(result.last?.Close ?? ''),
          formatPercent(changePct),
          maTrend.label,
          (result.signals || []).map((signal) => `${signal.Indicator}${signal.Type}`).join(';'),
        ];
      }),
    );
  };

  // Save screen results
  const saveScreenResults = async () => {
    if (sortedResults.length === 0) return;
    try {
      await api.saveScreenResults(sortedResults.map((item) => ({ code: item.code, name: item.name })));
      setStockList((previous) => {
        const merged = [...previous];
        for (const item of sortedResults) {
          if (!merged.some((stock) => stock.code === item.code)) merged.push({ code: item.code, name: item.name });
        }
        return merged;
      });
      messageApi.success(`已保存 ${sortedResults.length} 只命中股票到自选股`);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '保存失败');
    }
  };

  // Sync watchlist daily
  const onSyncWatchlistDaily = async () => {
    try {
      const result = await syncWatchlistDaily();
      setSyncResult(result || null);
      if (!result) {
        messageApi.warning('没有需要同步的股票');
        return;
      }
      if (result.failed > 0) {
        messageApi.warning(`同步完成：成功 ${result.success} 只，失败 ${result.failed} 只`);
      } else {
        messageApi.success(`同步完成：${result.success} 只自选股日K已更新`);
      }
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '同步失败');
    }
  };

  // Add single stock
  const onAddStock = (stock: { code: string; name: string }) => {
    api.watchlistAdd(stock.code, stock.name).catch(() => {});
    setStockList((previous) => [...previous, { code: stock.code, name: stock.name }]);
    messageApi.success(`已添加 ${stock.name}`);
  };

  return (
    <>
      {contextHolder}
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Flex justify="space-between" align="center" wrap="wrap" gap={12}>
          <div>
            <Title level={3} style={{ margin: 0 }}>信号筛选</Title>
            <Paragraph type="secondary" style={{ marginBottom: 0 }}>
              从自选股或板块成分股中批量计算指标信号，并快速跳转到个股详情。
            </Paragraph>
          </div>
        </Flex>

        <Card size="small" hoverable style={{ cursor: 'pointer' }} onClick={() => setShowSourceModal(true)}>
          <Flex justify="space-between" align="center">
            <Space>
              <Text type="secondary">股票池：</Text>
              {sourceTab === 'watchlist' ? (
                <Text strong>{stockList.length} 只自选股</Text>
              ) : selectedBlock ? (
                <Text strong>{selectedBlock.name}（{selectedBlock.stocks?.length || selectedBlock.count} 只）</Text>
              ) : (
                <Text type="secondary">未选择板块</Text>
              )}
            </Space>
            <Button size="small" icon={<EditOutlined />}>更换</Button>
          </Flex>
        </Card>

        <Card title="筛选设置" size="small">
          <Flex wrap="wrap" gap={12} align="center">
            <Flex gap={8} align="center">
              <Text type="secondary">周期</Text>
              <Segmented
                value={ktype}
                onChange={(value) => setKtype(value as string)}
                options={KTYPE_OPTIONS}
                size="small"
              />
            </Flex>

            <Divider type="vertical" />

            <Flex gap={8} align="center" style={{ flex: 1, minWidth: 240 }}>
              <Text type="secondary">信号过滤</Text>
              <Tooltip title="查看信号含义说明">
                <Button icon={<InfoCircleOutlined />} size="small" type="text" onClick={() => setShowHelpModal(true)} />
              </Tooltip>
              <Select
                mode="multiple"
                value={selectedSignals}
                onChange={(value) => setSelectedSignals(value as string[])}
                placeholder="选择信号类型"
                style={{ flex: 1 }}
                size="small"
                options={[
                  {
                    label: '买入信号',
                    options: SIGNAL_OPTIONS.filter((opt) => opt.buy).map((opt) => ({
                      value: opt.value,
                      label: opt.label,
                    })),
                  },
                  {
                    label: '卖出信号',
                    options: SIGNAL_OPTIONS.filter((opt) => !opt.buy).map((opt) => ({
                      value: opt.value,
                      label: opt.label,
                    })),
                  },
                ]}
              />
            </Flex>

            <Button
              type="primary"
              icon={<SearchOutlined />}
              loading={loading}
              onClick={() => void doScreen(resolvedCodes())}
              disabled={!resolvedCodes().trim()}
            >
              开始筛选
            </Button>
          </Flex>
        </Card>

        {cappedInfo && (
          <Alert type="warning" showIcon message="批量已截断" description={cappedInfo.reason} />
        )}

        {hasScreenLoaded && (
          <Card size="small" style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.08), rgba(14,165,233,0.06))' }}>
            <Flex justify="space-between" align="center" wrap="wrap" gap={16}>
              <Space size={24}>
                <Text>扫描总数: <strong>{total}</strong></Text>
                <Text>命中结果: <strong>{filteredResults.length}</strong></Text>
                <Text>活跃信号: <strong>{Object.keys(signalCounts).length}</strong></Text>
                {failedCodes.length > 0 && (
                  <Text style={{ color: '#cf1322' }}>失败: <strong>{failedCodes.length}</strong></Text>
                )}
              </Space>
              {results.length > 0 && (
                <Space size={[6, 6]} wrap>
                  {Object.entries(signalCounts).map(([type, count]) => (
                    <Tag key={type} color={type === '金叉' || type === '超卖' || type === '跌破下轨' || type === '多头排列' ? 'red' : 'green'}>
                      {type} {count}
                    </Tag>
                  ))}
                </Space>
              )}
            </Flex>
          </Card>
        )}

        {hasScreenLoaded && failedCodes.length > 0 && (
          <Collapse
            items={[{
              key: 'failed',
              label: <Space><Tag color="error">失败 {failedCodes.length}</Tag><Text type="secondary">点击查看详情</Text></Space>,
              children: (
                <Space direction="vertical" size={8} style={{ width: '100%' }}>
                  <Flex justify="flex-end">
                    <Button size="small" icon={<SyncOutlined />} loading={loading} onClick={() => void retryFailed()}>
                      重试失败项
                    </Button>
                  </Flex>
                  <List
                    size="small"
                    dataSource={failedCodes}
                    renderItem={(item: ScreenCodeStatus) => (
                      <List.Item>
                        <Flex justify="space-between" align="center" style={{ width: '100%' }}>
                          <Space>
                            <Text code>{item.code}</Text>
                            {item.name && <Text type="secondary">{item.name}</Text>}
                          </Space>
                          <Text type="danger" style={{ fontSize: 12 }}>{item.reason}</Text>
                        </Flex>
                      </List.Item>
                    )}
                  />
                </Space>
              ),
            }]}
          />
        )}

        {sortedResults.length > 0 ? (
          <VirtualResultTable
            results={sortedResults}
            tableContainerRef={tableContainerRef}
            sortKey={sortKey}
            sortAsc={sortAsc}
            onSortChange={handleSortChange}
            navigate={navigate}
            trades={trades}
            tradingLoading={tradingLoading}
            handleBuy={onHandleBuy}
            handleSell={onHandleSell}
            extra={
              <Space wrap>
                <Button icon={<DownloadOutlined />} onClick={exportScreenResults} size="small">
                  导出CSV
                </Button>
                <Button icon={<SaveOutlined />} onClick={() => void saveScreenResults()} size="small">
                  保存结果
                </Button>
              </Space>
            }
          />
        ) : !loading ? (
          <Card>
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={hasScreenLoaded ? '当前筛选条件下没有命中结果' : '选择股票来源后点击“开始筛选”'}
            />
          </Card>
        ) : null}
      </Space>

      <TradeModal
        visible={showTradeModal}
        onCancel={() => setShowTradeModal(false)}
        onConfirm={onConfirmTrade}
        currentTradeAction={currentTradeAction}
        currentTradeStock={currentTradeStock}
        tradeReason={tradeReason}
        onTradeReasonChange={(value) => setTradeReason(value)}
        tradingLoading={tradingLoading}
        trades={trades}
      />

      <SourceSelectorModal
        visible={showSourceModal}
        onCancel={() => setShowSourceModal(false)}
        sourceTab={sourceTab}
        onSourceTabChange={(value) => setSourceTab(value)}
        stockList={stockList}
        onStockListChange={setStockList}
        inputCode={inputCode}
        onInputCodeChange={(value) => setInputCode(value)}
        onAddCodes={onAddCodes}
        inputLoading={inputLoading}
        syncLoading={false}
        onSync={onSyncWatchlistDaily}
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
        navigate={navigate}
        onRemoveStock={removeCode}
      />

      <SignalHelpModal
        visible={showHelpModal}
        onCancel={() => setShowHelpModal(false)}
      />

      <SyncResultModal
        result={syncResult}
        onCancel={() => setSyncResult(null)}
      />

      <BlockStocksModal
        visible={showBlockModal}
        onCancel={() => setShowBlockModal(false)}
        selectedBlock={selectedBlock}
        blockStocksWithNames={blockStocksWithNames}
        blockStocksLoadingNames={blockStocksLoadingNames}
        stockList={stockList}
        onAddStock={onAddStock}
        onRemoveStock={removeCode}
        onAddAll={addAllBlockStocksToWatchlist}
        navigate={navigate}
      />
    </>
  );
}