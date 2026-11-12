import { useRef, useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import dayjs from 'dayjs';
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
  Empty,
  Flex,
  Tooltip,
  Space,
  Tag,
  Typography,
  message,
} from 'antd';
import { api } from '../api/client';
import type { ScreenResult, SyncFreshnessResult } from '../types/api';
import { StateFilterControls } from '../components/screen/StateFilterControls';
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
import { exportCsv, stockNamesFromCodesCache, formatPercent, formatStateDate, getChangePct } from '../utils/screen';

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
    stateFilter, setStateFilter, date, setDate, dimensions, snapshotDate, error, conditionsChanged,
    hasScreenLoaded,
    total,
    loading,
    screenedAt,
    forceRefresh,
    sortKey,
    sortAsc,
    filteredResults,
    sortedResults,
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
  const [freshnessData, setFreshnessData] = useState<SyncFreshnessResult[]>([]);

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

  // Load freshness data after screen completes
  useEffect(() => {
    if (hasScreenLoaded && total > 0) {
      const codes = sourceTab === 'block' && selectedBlock?.stocks
        ? selectedBlock.stocks
        : stockList.map((stock) => stock.code);
      if (codes.length > 0) {
        api.getSyncFreshness(codes)
          .then((result) => {
            setFreshnessData(result.results);
          })
          .catch(() => {
            // ignore freshness load failure
          });
      }
    }
  }, [hasScreenLoaded, total, sourceTab, selectedBlock, stockList]);

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
      `tongstock-screen-${dayjs().format('YYYY-MM-DD')}.csv`,
      ['代码', '名称', '日期', '收盘', '涨跌幅', '迁移日期', ...dimensions.map((d) => d.label)],
      sortedResults.map((result) => {
        return [
          result.code,
          result.name || '',
          result.date,
          result.last?.Close == null ? '' : result.last.Close.toFixed(2),
          result.last?.PrevClose ? formatPercent(getChangePct(result)) : '-',
          result.transition_date || '',
          ...dimensions.map((d) => {
            const state = d.states.find((s) => s.state === result.states[d.dim]);
            if (!state) return '';
            // 状态标签本身是「金叉/超买/带内」等形态词；补上后端下发的方向，避免 CSV 里看不出买卖。
            return state.direction_label ? `${state.label}（${state.direction_label}）` : state.label;
          }),
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

  const stateDateLabel = snapshotDate ? formatStateDate(snapshotDate) : '';

  return (
    <>
      {contextHolder}
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Flex justify="space-between" align="center" wrap="wrap" gap={12}>
          <div>
            <Title level={3} style={{ margin: 0 }}>信号筛选</Title>
            <Paragraph type="secondary" style={{ marginBottom: 0 }}>
              选一个交易日，用四个指标的状态同时筛股票；状态在收盘后算好，点击结果可以查看该股逐日状态变化。
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
            <StateFilterControls
              dimensions={dimensions}
              value={stateFilter}
              onChange={setStateFilter}
              date={date}
              onDateChange={setDate}
              onOpenHelp={() => setShowHelpModal(true)}
            />
            <Tooltip title="查看三态与迁移条件说明">
              <Button icon={<InfoCircleOutlined />} onClick={() => setShowHelpModal(true)}>状态说明</Button>
            </Tooltip>

            <Tooltip title="用当前条件重新筛选">
              <Button
                icon={<SyncOutlined />}
                loading={loading}
                onClick={() => void forceRefresh()}
                disabled={!hasScreenLoaded}
              >
                强制刷新
              </Button>
            </Tooltip>

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

        {conditionsChanged && <Alert type="info" showIcon title="条件已修改，请点击开始筛选；下方仍为上次执行结果。" />}
        {loading && <Alert type="info" showIcon title="正在筛选…首次筛选可能需要 30–60 秒；结果返回前请勿重复点击。" />}
        {error && (
          <Alert
            type="error"
            showIcon
            title="本次筛选没有完成，请稍后重试"
            description={error}
            action={<Button size="small" onClick={() => void forceRefresh()} disabled={!hasScreenLoaded}>重试</Button>}
          />
        )}

        {hasScreenLoaded && (
          <Card size="small" style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.08), rgba(14,165,233,0.06))' }}>
            <Flex justify="space-between" align="center" wrap="wrap" gap={16}>
              <Space size={24} wrap>
                <Text>扫描 <strong>{total}</strong> 只</Text>
                <Text>命中 <strong>{filteredResults.length}</strong> 只</Text>
                <Text>状态日期 <strong>{stateDateLabel || '—'}</strong></Text>
              </Space>
              <Space size={16} wrap>
                {screenedAt && (
                  <Tooltip title="本次筛选执行时间">
                    <Text type="secondary">
                      执行时间: <strong>{new Date(screenedAt).toLocaleTimeString('zh-CN', { hour12: false })}</strong>
                    </Text>
                  </Tooltip>
                )}
              </Space>
            </Flex>
          </Card>
        )}

        {/* Data freshness diagnostics */}
        {hasScreenLoaded && freshnessData.length > 0 && (
          <Card size="small" title="数据新鲜度诊断">
            <Flex wrap="wrap" gap={16} align="center">
              <Space size={[6, 6]} wrap>
                {(() => {
                  const fresh = freshnessData.filter((r) => r.freshness === 'fresh').length;
                  const stale = freshnessData.filter((r) => r.freshness === 'stale').length;
                  const outdated = freshnessData.filter((r) => r.freshness === 'outdated').length;
                  const failed = freshnessData.filter((r) => r.freshness === 'failed').length;
                  const empty = freshnessData.filter((r) => r.freshness === 'empty').length;
                  const unknown = freshnessData.filter((r) => r.freshness === 'unknown').length;
                  return (
                    <>
                      <Tag color="green">新鲜 {fresh}</Tag>
                      <Tag color="orange">滞后 {stale}</Tag>
                      <Tag color="red">过期 {outdated}</Tag>
                      <Tag color="red">失败 {failed}</Tag>
                      <Tag color="default">无数据 {empty}</Tag>
                      <Tag color="default">未知 {unknown}</Tag>
                    </>
                  );
                })()}
              </Space>
              {freshnessData.filter((r) => r.freshness === 'outdated' || r.freshness === 'failed').length > 0 && (
                <Button size="small" icon={<SyncOutlined />} onClick={onSyncWatchlistDaily}>
                  同步更新数据
                </Button>
              )}
            </Flex>
          </Card>
        )}

        {sortedResults.length > 0 ? (
          <VirtualResultTable
            dimensions={dimensions}
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

      <SignalHelpModal dimensions={dimensions}
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