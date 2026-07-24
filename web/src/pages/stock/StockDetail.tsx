import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { AreaChartOutlined, BankOutlined, BarChartOutlined, ClockCircleOutlined, DollarOutlined, GiftOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { Card, Empty, Flex, Space, Spin, Tabs, Typography } from 'antd';
import CandlestickChart from '../../components/charts/CandlestickChart';
import ChartToolbar from '../../components/charts/ChartToolbar';
import StockCompareView from '../../components/StockCompareView';
import AgentChatPanel from '../../components/AgentChatPanel';
import ParadigmResultDrawer from '../../components/ParadigmResultDrawer';
import { StockInfoHeader } from '../../components/stock/StockInfoHeader';
import { StockStatistics } from '../../components/stock/StockStatistics';
import { SignalTabContent } from '../../components/stock/SignalTabContent';
import { FinanceTabContent } from '../../components/stock/FinanceTabContent';
import { CompanyTabContent } from '../../components/stock/CompanyTabContent';
import { DividendTabContent } from '../../components/stock/DividendTabContent';
import { IntradayTabContent } from '../../components/stock/IntradayTabContent';
import { useStockDetail } from '../../hooks/useStockDetail';
import { useStockChart } from '../../hooks/useStockChart';
import { useStockFinance } from '../../hooks/useStockFinance';
import { useStockCompany } from '../../hooks/useStockCompany';
import { useStockMinute } from '../../hooks/useStockMinute';
import { useStockCompare } from '../../hooks/useStockCompare';
import { useParadigmAnalysis } from '../../hooks/useParadigmAnalysis';
import { getValueColor } from '../../lib/stock-detail';
import { api } from '../../api/client';
import type { XdXrItem } from '../../types/api';

type Tab = 'chart' | 'signal' | 'compare' | 'finance' | 'company' | 'dividend' | 'intraday';

const TAB_ITEMS: { key: Tab; label: string; icon: React.ReactNode }[] = [
  { key: 'chart', label: 'K线+指标', icon: <AreaChartOutlined /> },
  { key: 'signal', label: '信号', icon: <ThunderboltOutlined /> },
  { key: 'compare', label: '对比', icon: <DollarOutlined /> },
  { key: 'finance', label: '财务', icon: <BarChartOutlined /> },
  { key: 'company', label: '公司', icon: <BankOutlined /> },
  { key: 'dividend', label: '分红', icon: <GiftOutlined /> },
  { key: 'intraday', label: '分时', icon: <ClockCircleOutlined /> },
];

export default function StockDetail() {
  const { tab: paramTab } = useParams();
  const navigate = useNavigate();
  const [tab, setTab] = useState<Tab>((paramTab as Tab) || 'chart');
  const [dividends, setDividends] = useState<XdXrItem[]>([]);
  const [fullscreen, setFullscreen] = useState(false);
  const [agentPanelOpen, setAgentPanelOpen] = useState(false);

  const { code, quote, loading, detailStatus, detailError, syncState, ktype, setKtype } = useStockDetail();
  const { klines, indicator, chartLoading, analysis, sortedSignals, sortedSignalOutcomes, latestClose, mainOverlay, setMainOverlay, subPanel, setSubPanel } = useStockChart(code, ktype, detailStatus);
  const { finance, financeTrends, financeMetrics, financeTrendMode, setFinanceTrendMode, financeCompareMode, setFinanceCompareMode, financeViewMode, setFinanceViewMode, selectedFinanceMetrics, setSelectedFinanceMetrics, financeTrendLoading, availableFinanceMetrics, financeChartGroups, financeDisplayRecords, activeFinanceMetrics, latestFinanceRecord, formatFinanceMetricValue, financeItems } = useStockFinance(code, detailStatus);
  const { companyCats, companyContent, selectedCat, loadCompanyContent } = useStockCompany(code, detailStatus);
  const { minuteData, minuteDate, minuteLoading, minuteError, highlightedIdx, setHighlightedIdx } = useStockMinute(code, detailStatus);
  const { compareData, compareLoading } = useStockCompare(code, detailStatus);
  const { paradigmResult, paradigmLoading, paradigmAgentText, paradigmEvalConfirm, paradigmEvalInvalid, paradigmDrawerOpen, setParadigmDrawerOpen, analyzeParadigm } = useParadigmAnalysis();

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setFullscreen(false);
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    if (tab === 'dividend') api.xdxr(code).then((d) => setDividends([...d].reverse())).catch(() => {});
    if (tab === 'intraday') api.finance(code).then(() => {}).catch(() => {});
  }, [code, tab, detailStatus]);

  const switchTab = (nextTab: Tab) => {
    setTab(nextTab);
    navigate(`/stock/${code}/${nextTab}`, { replace: true });
  };

  const pct = quote ? ((quote.Price - quote.LastClose) / quote.LastClose) * 100 : 0;
  const up = pct >= 0;
  const showTabs = detailStatus === 'ready';
  const showInitialLoading = !showTabs && (loading || chartLoading || detailStatus === 'loading');
  const valueColor = getValueColor(pct);

  return (
    <div style={fullscreen ? { position: 'fixed', inset: 0, zIndex: 1000, background: '#0b1220', padding: 24, overflow: 'auto' } : undefined}>
      <Space direction="vertical" size={16} style={{ display: 'flex' }}>
        {showTabs && quote && (
          <StockInfoHeader
            code={code}
            quote={quote}
            ktype={ktype}
            pct={pct}
            valueColor={valueColor}
            up={up}
            syncState={syncState}
            fullscreen={fullscreen}
            onFullscreenChange={setFullscreen}
            onAgentClick={() => setAgentPanelOpen(true)}
            onParadigmClick={() => {
              setParadigmDrawerOpen(true);
              void analyzeParadigm(code, quote.Name);
            }}
            onParadigmRefresh={() => {
              setParadigmDrawerOpen(true);
              void analyzeParadigm(code, quote.Name, true);
            }}
          />
        )}

        {showTabs && quote && (
          <StockStatistics quote={quote} latestClose={latestClose} valueColor={valueColor} />
        )}

        {showInitialLoading && (
          <Card><Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin size="large" /></Flex></Card>
        )}

        {!showInitialLoading && !showTabs && (
          <Card>
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={
                <Space direction="vertical" size={4}>
                  <Typography.Text strong>
                    {detailStatus === 'not_found' ? '未找到该股票' : '该股票暂无可展示的数据'}
                  </Typography.Text>
                  <Typography.Text type="secondary">{detailError || '请重新搜索并选择一个有效的股票。'}</Typography.Text>
                </Space>
              }
            />
          </Card>
        )}

        {showTabs && (
          <Tabs
            activeKey={tab}
            onChange={(key) => switchTab(key as Tab)}
            items={TAB_ITEMS.map((item) => ({ key: item.key, label: <Space>{item.icon}{item.label}</Space> }))}
          />
        )}

        {showTabs && tab === 'chart' && (klines.length > 0 || chartLoading) && (
          <Space direction="vertical" size={16} style={{ display: 'flex' }}>
            <Card><ChartToolbar ktype={ktype} onKtypeChange={setKtype} mainOverlay={mainOverlay} onMainOverlayChange={setMainOverlay} subPanel={subPanel} onSubPanelChange={setSubPanel} /></Card>
            {chartLoading ? (
              <Card><Flex justify="center" align="center" style={{ minHeight: 470 }}><Spin size="large" /></Flex></Card>
            ) : (
              <Card bodyStyle={{ padding: 0 }}><CandlestickChart klines={klines} indicator={indicator} mainOverlay={mainOverlay} subPanel={subPanel} /></Card>
            )}
          </Space>
        )}

        {showTabs && tab === 'signal' && (
          <SignalTabContent
            chartLoading={chartLoading}
            analysis={analysis}
            sortedSignals={sortedSignals}
            sortedSignalOutcomes={sortedSignalOutcomes}
            pct={pct}
            up={up}
          />
        )}

        {showTabs && tab === 'compare' && (
          compareLoading ? (
            <Card><Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin size="large" /></Flex></Card>
          ) : compareData ? (
            <StockCompareView
              code={compareData.code}
              stockName={compareData.stock_name}
              stockChange={compareData.stock_change}
              comparisons={compareData.comparisons}
              loading={compareLoading}
            />
          ) : (
            <Card><Empty description="暂无对比数据" /></Card>
          )
        )}

        {showTabs && tab === 'finance' && financeTrends && (
          <FinanceTabContent
            financeTrends={financeTrends}
            financeMetrics={financeMetrics}
            financeTrendMode={financeTrendMode}
            setFinanceTrendMode={setFinanceTrendMode}
            financeCompareMode={financeCompareMode}
            setFinanceCompareMode={setFinanceCompareMode}
            financeViewMode={financeViewMode}
            setFinanceViewMode={setFinanceViewMode}
            selectedFinanceMetrics={selectedFinanceMetrics}
            setSelectedFinanceMetrics={setSelectedFinanceMetrics}
            financeTrendLoading={financeTrendLoading}
            availableFinanceMetrics={availableFinanceMetrics}
            financeChartGroups={financeChartGroups}
            financeDisplayRecords={financeDisplayRecords}
            activeFinanceMetrics={activeFinanceMetrics}
            latestFinanceRecord={latestFinanceRecord}
            formatFinanceMetricValue={formatFinanceMetricValue}
            financeItems={financeItems}
            code={code}
          />
        )}

        {showTabs && tab === 'company' && (
          <CompanyTabContent
            companyCats={companyCats}
            companyContent={companyContent}
            selectedCat={selectedCat}
            loadCompanyContent={loadCompanyContent}
          />
        )}

        {showTabs && tab === 'dividend' && (
          <DividendTabContent dividends={dividends} />
        )}

        {showTabs && tab === 'intraday' && (
          <IntradayTabContent
            minuteData={minuteData}
            minuteDate={minuteDate}
            minuteLoading={minuteLoading}
            minuteError={minuteError}
            quote={quote}
            finance={finance}
            highlightedIdx={highlightedIdx}
            setHighlightedIdx={setHighlightedIdx}
          />
        )}

        <AgentChatPanel
          stockCode={code}
          stockName={quote?.Name}
          open={agentPanelOpen}
          onClose={() => setAgentPanelOpen(false)}
        />

        <ParadigmResultDrawer
          stockCode={code}
          stockName={quote?.Name}
          open={paradigmDrawerOpen}
          onClose={() => setParadigmDrawerOpen(false)}
          loading={paradigmLoading}
          paradigm={paradigmResult}
          evaluatedConfirm={paradigmEvalConfirm}
          evaluatedInvalid={paradigmEvalInvalid}
          agentText={paradigmAgentText}
        />
      </Space>
    </div>
  );
}
