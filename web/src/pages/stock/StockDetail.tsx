import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  AreaChartOutlined,
  BankOutlined,
  BarChartOutlined,
  CameraOutlined,
  ClockCircleOutlined,
  CompressOutlined,
  DollarOutlined,
  ExpandOutlined,
  FileExcelOutlined,
  FundOutlined,
  GiftOutlined,
  InfoCircleOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Checkbox,
  Col,
  Descriptions,
  Empty,
  Flex,
  List,
  message,
  Radio,
  Progress,
  Row,
  Segmented,
  Skeleton,
  Space,
  Spin,
  Statistic,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { api } from '../../api/client';
import type {
  FinanceTrendsResponse,
  FinanceMetricsResponse,
  KlineSyncState,
  MinuteItem,
  CompanyCategory,
  Signal,
  SignalAnalysis as SignalAnalysisType,
  SignalOutcome,
  StockCompareResponse,
  XdXrItem,
} from '../../types/api';
import CandlestickChart from '../../components/charts/CandlestickChart';
import ChartToolbar from '../../components/charts/ChartToolbar';
import FinanceTrendChart, { type FinanceTrendChartHandle, type FinanceTrendMetric } from '../../components/charts/FinanceTrendChart';
import MinuteChart from '../../components/charts/MinuteChart';
import SignalInterpretationCard from '../../components/SignalInterpretationCard';
import StockCompareView from '../../components/StockCompareView';
import TabContent from '../../components/TabContent';
import TdxContent from '../../components/TdxContent';
import { formatDate, formatShortDate, formatTdxDate, formatTime } from '../../lib/datetime';
import { parseTdxText } from '../../lib/tdx-parser';

type Tab = 'chart' | 'signal' | 'compare' | 'finance' | 'company' | 'dividend' | 'intraday';
type DetailStatus = 'loading' | 'ready' | 'not_found' | 'no_data';
type FinanceCompareMode = 'raw' | 'yoy' | 'qoq';
type FinanceViewMode = 'chart' | 'table';

const TAB_ITEMS: { key: Tab; label: string; icon: React.ReactNode }[] = [
  { key: 'chart', label: 'K线+指标', icon: <AreaChartOutlined /> },
  { key: 'signal', label: '信号', icon: <ThunderboltOutlined /> },
  { key: 'compare', label: '对比', icon: <FundOutlined /> },
  { key: 'finance', label: '财务', icon: <DollarOutlined /> },
  { key: 'company', label: '公司', icon: <BankOutlined /> },
  { key: 'dividend', label: '分红', icon: <GiftOutlined /> },
  { key: 'intraday', label: '分时', icon: <ClockCircleOutlined /> },
];

function getLastTradingDay(): string {
  const today = new Date();
  const dayOfWeek = today.getDay();
  let adjustDays = 1;
  if (dayOfWeek === 0) adjustDays = 2;
  else if (dayOfWeek === 1) adjustDays = 3;

  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - adjustDays);

  const year = yesterday.getFullYear();
  const month = String(yesterday.getMonth() + 1).padStart(2, '0');
  const day = String(yesterday.getDate()).padStart(2, '0');
  return `${year}${month}${day}`;
}

function getDetailErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

function classifyDetailStatus(error: unknown): DetailStatus {
  const message = error instanceof Error ? error.message : '';
  if (message.includes('未找到') || message.includes('多个匹配股票')) return 'not_found';
  return 'no_data';
}

function getValueColor(value: number) {
  if (value > 0) return '#ef4444';
  if (value < 0) return '#22c55e';
  return '#cbd5e1';
}

function formatSigned(value: number, suffix = '') {
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}${suffix}`;
}

function compareDateDesc(a: string | undefined, b: string | undefined): number {
  return String(b ?? '').localeCompare(String(a ?? ''));
}

function amountWanToYi(value: number | undefined): number | undefined {
  return typeof value === 'number' ? value / 10000 : undefined;
}

const FINANCE_METRICS: FinanceTrendMetric[] = [
  { key: 'revenue', label: '营业收入', color: '#3b82f6', axis: 'amount' },
  { key: 'netProfit', label: '净利润', color: '#22c55e', axis: 'amount' },
  { key: 'grossMargin', label: '销售毛利率', color: '#f59e0b', axis: 'percent' },
  { key: 'netMargin', label: '净利润率', color: '#ef4444', axis: 'percent' },
  { key: 'roe', label: '净资产收益率', color: '#8b5cf6', axis: 'percent' },
  { key: 'eps', label: '每股收益', color: '#ec4899', axis: 'perShare' },
  { key: 'operatingCashPerShare', label: '每股经营现金流', color: '#14b8a6', axis: 'perShare' },
];

const FINANCE_CHART_GROUPS = [
  {
    key: 'amount',
    title: '规模趋势',
    description: '收入、利润与每股指标',
    axes: ['amount', 'perShare'] as FinanceTrendMetric['axis'][],
  },
  {
    key: 'margin',
    title: '利润率趋势',
    description: '毛利率、净利率与 ROE',
    axes: ['percent'] as FinanceTrendMetric['axis'][],
  },
] as const;

function formatFinanceMetricValue(value: number | undefined, metric: FinanceTrendMetric) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-';
  if (metric.axis === 'percent') return `${value.toFixed(2)}%`;
  if (metric.axis === 'perShare') return `${value.toFixed(2)}元`;
  if (Math.abs(value) >= 100000000) return `${(value / 100000000).toFixed(2)}亿`;
  if (Math.abs(value) >= 10000) return `${(value / 10000).toFixed(2)}万`;
  return value.toFixed(2);
}

function calcFinanceCompareValue(records: FinanceTrendsResponse['records'], index: number, metricKey: FinanceTrendMetric['key'], mode: FinanceCompareMode) {
  const current = records[index]?.[metricKey];
  if (typeof current !== 'number' || Number.isNaN(current)) return undefined;
  if (mode === 'raw') return current;

  const offset = mode === 'yoy' ? 4 : 1;
  const previous = records[index - offset]?.[metricKey];
  if (typeof previous !== 'number' || Number.isNaN(previous) || previous === 0) return undefined;
  return ((current - previous) / Math.abs(previous)) * 100;
}

function buildFinanceComparisonRecords(
  records: FinanceTrendsResponse['records'],
  metrics: FinanceTrendMetric[],
  mode: FinanceCompareMode,
): FinanceTrendsResponse['records'] {
  if (mode === 'raw') return records;
  return records
    .map((record, index) => {
      const next: FinanceTrendsResponse['records'][number] = { ...record };
      let hasValue = false;
      metrics.forEach((metric) => {
        const value = calcFinanceCompareValue(records, index, metric.key, mode);
        if (typeof value === 'number' && !Number.isNaN(value)) {
          next[metric.key] = value;
          hasValue = true;
        } else {
          delete next[metric.key];
        }
      });
      return hasValue ? next : null;
    })
    .filter((record): record is FinanceTrendsResponse['records'][number] => record !== null);
}

function exportCsv(filename: string, headers: string[], rows: string[][]) {
  const csv = [headers, ...rows]
    .map((row) => row.map((cell) => `"${String(cell ?? '').replace(/"/g, '""')}"`).join(','))
    .join('\n');
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

function downloadDataUrl(filename: string, dataUrl: string) {
  const link = document.createElement('a');
  link.href = dataUrl;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}

export default function StockDetail() {
  const { code: paramCode, tab: paramTab } = useParams();
  const navigate = useNavigate();
  const [code, setCode] = useState(paramCode || '');
  const [tab, setTab] = useState<Tab>((paramTab as Tab) || 'chart');
  const [quote, setQuote] = useState<any>(null);
  const [klines, setKlines] = useState<any[]>([]);
  const [indicator, setIndicator] = useState<any>(null);
  const [ktype, setKtype] = useState('day');
  const [mainOverlay, setMainOverlay] = useState('MA');
  const [subPanel, setSubPanel] = useState('MACD');
  const [finance, setFinance] = useState<any>(null);
  const [financeTrends, setFinanceTrends] = useState<FinanceTrendsResponse | null>(null);
  const [financeMetrics, setFinanceMetrics] = useState<FinanceMetricsResponse | null>(null);
  const [financeTrendMode, setFinanceTrendMode] = useState<'quarter' | 'year'>('quarter');
  const [financeCompareMode, setFinanceCompareMode] = useState<FinanceCompareMode>('raw');
  const [financeViewMode, setFinanceViewMode] = useState<FinanceViewMode>('chart');
  const [selectedFinanceMetrics, setSelectedFinanceMetrics] = useState<string[]>(['revenue', 'netProfit', 'roe']);
  const [financeTrendLoading, setFinanceTrendLoading] = useState(false);
  const [companyCats, setCompanyCats] = useState<any[]>([]);
  const [companyContent, setCompanyContent] = useState('');
  const [selectedCat, setSelectedCat] = useState('');
  const [dividends, setDividends] = useState<any[]>([]);
  const [minuteData, setMinuteData] = useState<any[]>([]);
  const [minuteDate, setMinuteDate] = useState<string>('');
  const [minuteLoading, setMinuteLoading] = useState(false);
  const [minuteError, setMinuteError] = useState('');
  const [analysis, setAnalysis] = useState<SignalAnalysisType | null>(null);
  const [compareData, setCompareData] = useState<StockCompareResponse | null>(null);
  const [compareLoading, setCompareLoading] = useState(false);
  const [highlightedIdx, setHighlightedIdx] = useState(-1);
	const [fullscreen, setFullscreen] = useState(false);
	const tradeRowRefs = useRef<Record<number, HTMLDivElement | null>>({});
	const amountChartRef = useRef<FinanceTrendChartHandle | null>(null);
	const marginChartRef = useRef<FinanceTrendChartHandle | null>(null);
	const [loading, setLoading] = useState(false);
	const [chartLoading, setChartLoading] = useState(false);
	const [detailStatus, setDetailStatus] = useState<DetailStatus>('loading');
	const [detailError, setDetailError] = useState('');
	const [syncState, setSyncState] = useState<KlineSyncState | null>(null);

  useEffect(() => {
    if (!paramCode) {
      navigate('/stock/choose');
      return;
    }
    setCode(paramCode);
    if (paramTab) setTab(paramTab as Tab);
  }, [paramCode, paramTab, navigate]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setFullscreen(false);
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  const switchTab = (nextTab: Tab) => {
    setTab(nextTab);
    navigate(`/stock/${code}/${nextTab}`, { replace: true });
  };

  const loadCompanyContent = async (cat: string | CompanyCategory) => {
    const catName = typeof cat === 'string' ? cat : cat.Name;
    setSelectedCat(catName);
    setCompanyContent('');
    try {
      const r = await api.companyContent(code, cat);
      setCompanyContent((r.content || '').replace(/\r/g, ''));
    } catch {
      setCompanyContent('加载失败');
    }
  };

	useEffect(() => {
		if (!code) return;
		let cancelled = false;
		setLoading(true);
		setDetailStatus('loading');
		setDetailError('');
		setQuote(null);
		setFinance(null);
		setFinanceTrends(null);
		setCompanyCats([]);
    setCompanyContent('');
    setSelectedCat('');
    setDividends([]);
    setMinuteData([]);
    setMinuteDate('');
    setMinuteError('');
    setSyncState(null);

    // Load sync state (non-blocking, best-effort)
    api.getSyncState(code, ktype).then((state) => {
      if (!cancelled) setSyncState(state);
    }).catch(() => {});

		const loadQuote = async () => {
			try {
				const quoteResult = await api.quote(code);
				if (cancelled) return;
				setQuote(quoteResult);
				api.historyAdd(code, quoteResult.Name).catch(() => {});
				setLoading(false);
			} catch (error) {
				if (cancelled) return;
				setDetailStatus('not_found');
				setDetailError(getDetailErrorMessage(error, '未找到匹配股票或行情数据'));
				setLoading(false);
				return;
			}
		};

		loadQuote().catch((error) => {
			if (cancelled) return;
			setDetailStatus(classifyDetailStatus(error));
			setDetailError(getDetailErrorMessage(error, '加载股票详情失败'));
			setLoading(false);
		});

		return () => {
			cancelled = true;
		};
	}, [code]);

	useEffect(() => {
		if (!code) return;
		let cancelled = false;
		setChartLoading(true);
		setIndicator(null);
		setKlines([]);
		setAnalysis(null);

    // Refresh sync state when ktype changes
    api.getSyncState(code, ktype).then((state) => {
      if (!cancelled) setSyncState(state);
    }).catch(() => {});

		const loadChart = async () => {
			const [indicatorResult, analysisResult] = await Promise.allSettled([
				api.indicator(code, ktype),
				api.signalAnalysis(code, ktype),
			]);

			if (cancelled) return;

			if (indicatorResult.status === 'rejected') {
				setDetailStatus(classifyDetailStatus(indicatorResult.reason));
				setDetailError(getDetailErrorMessage(indicatorResult.reason, '该股票暂无可展示的数据'));
				setChartLoading(false);
				return;
			}

      const indicatorData = indicatorResult.value;
      const nextKlines = indicatorData?.klines || [];
      setIndicator(indicatorData);
      setKlines(nextKlines);

			if (nextKlines.length === 0) {
				setDetailStatus('no_data');
				setDetailError('该股票暂无可展示的K线数据');
				setChartLoading(false);
				return;
			}

			if (analysisResult.status === 'fulfilled') setAnalysis(analysisResult.value);
			setDetailStatus('ready');
			setDetailError('');
			setChartLoading(false);
		};

		loadChart().catch((error) => {
			if (cancelled) return;
			setDetailStatus(classifyDetailStatus(error));
			setDetailError(getDetailErrorMessage(error, '加载K线失败'));
			setChartLoading(false);
		});

		return () => {
			cancelled = true;
		};
	}, [code, ktype]);

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    if (tab === 'finance') {
      api.finance(code).then(setFinance).catch(() => {});
      api.financeMetrics(code).then(setFinanceMetrics).catch(() => setFinanceMetrics(null));
    }
    if (tab === 'company') api.company(code).then((cats) => {
      setCompanyCats(cats);
      if (cats.length > 0 && !selectedCat) void loadCompanyContent(cats[0]);
    }).catch(() => {});
    if (tab === 'dividend') api.xdxr(code).then((d) => setDividends([...d].reverse())).catch(() => {});
    if (tab === 'compare') {
      setCompareLoading(true);
      api.stockCompare(code).then((d) => {
        setCompareData(d);
        setCompareLoading(false);
      }).catch(() => {
        setCompareLoading(false);
      });
    }
    if (tab === 'intraday') {
      const fetchMinute = async () => {
        setMinuteLoading(true);
        setMinuteError('');
        let loaded = false;
        try {
          const r = await api.minute(code);
          if (r.List && r.List.length > 0) {
            setMinuteData(r.List);
            const today = new Date();
            setMinuteDate(formatShortDate(today));
            loaded = true;
          } else {
            setMinuteData([]);
          }
        } catch {
          // fall back to last trading day's minute data below
        }

        if (!loaded) {
          const yesterday = getLastTradingDay();
          try {
            const histR = await api.minuteHistory(code, yesterday);
            if (histR.List && histR.List.length > 0) {
              setMinuteData(histR.List);
              setMinuteDate(formatShortDate(yesterday));
              loaded = true;
            }
          } catch {
            // ignore historical minute errors
          }
        }

        if (!loaded) {
          setMinuteData([]);
          setMinuteDate('');
          setMinuteError('暂无可展示的分时数据');
        }
        api.quote(code).then(setQuote).catch(() => {});
        setMinuteLoading(false);
      };
      void fetchMinute();
      api.finance(code).then(setFinance).catch(() => {});
      const timer = setInterval(fetchMinute, 30000);
      return () => clearInterval(timer);
    }
  }, [code, tab, detailStatus]);

  useEffect(() => {
    if (!code || detailStatus !== 'ready' || tab !== 'finance') return;
    let cancelled = false;
    setFinanceTrendLoading(true);
    api.financeTrends(code, financeTrendMode)
      .then((data) => {
        if (cancelled) return;
        setFinanceTrends(data);
      })
      .catch(() => {
        if (cancelled) return;
        setFinanceTrends(null);
      })
      .finally(() => {
        if (!cancelled) setFinanceTrendLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [code, detailStatus, tab, financeTrendMode]);

  const pct = quote ? ((quote.Price - quote.LastClose) / quote.LastClose) * 100 : 0;
  const up = pct >= 0;
  const showTabs = detailStatus === 'ready';
  const showInitialLoading = !showTabs && (loading || chartLoading || detailStatus === 'loading');
  const valueColor = getValueColor(pct);
  const sortedSignals = useMemo(
    () => [...(indicator?.signals ?? [])].sort((a, b) => compareDateDesc(a.Date, b.Date)),
    [indicator?.signals],
  );
  const sortedSignalOutcomes = useMemo<SignalOutcome[]>(
    () => [...(analysis?.outcomes ?? [])].sort((a, b) => compareDateDesc(a.date, b.date)),
    [analysis?.outcomes],
  );
  const latestClose = klines.length > 0 ? klines[klines.length - 1].Close : undefined;

  const financeItems = finance ? [
    ['总股本', finance.ZongGuBen, '万股'],
    ['流通股本', finance.LiuTongGuBen, '万股'],
    ['总资产', finance.ZongZiChan, '万元'],
    ['净资产', finance.JingZiChan, '万元'],
    ['主营收入', finance.ZhuYingShouRu, '万元'],
    ['净利润', finance.JingLiRun, '万元'],
    ['每股净资产', finance.MeiGuJingZiChan, '元'],
    ['股东人数', finance.GuDongRenShu, '人'],
  ] : [];

  const availableFinanceMetrics = useMemo(() => {
    const trendMetrics = new Set(financeTrends?.metrics ?? []);
    return FINANCE_METRICS.filter((metric) => trendMetrics.has(metric.key));
  }, [financeTrends]);

  const financeChartGroups = useMemo(() => {
    return FINANCE_CHART_GROUPS.map((group) => ({
      ...group,
      metrics: availableFinanceMetrics.filter((metric) => group.axes.includes(metric.axis)),
    })).filter((group) => group.metrics.length > 0);
  }, [availableFinanceMetrics]);

  const financeDisplayRecords = useMemo(() => {
    if (!financeTrends) return [];
    return buildFinanceComparisonRecords(financeTrends.records, availableFinanceMetrics, financeCompareMode);
  }, [availableFinanceMetrics, financeCompareMode, financeTrends]);

  const activeFinanceMetrics = useMemo(() => {
    const selected = availableFinanceMetrics.filter((metric) => selectedFinanceMetrics.includes(metric.key));
    return selected.length > 0 ? selected : availableFinanceMetrics.slice(0, 3);
  }, [availableFinanceMetrics, selectedFinanceMetrics]);

  const latestFinanceRecord = financeDisplayRecords[financeDisplayRecords.length - 1];

  const financeTableColumns: ColumnsType<FinanceTrendsResponse['records'][number]> = useMemo(() => [
    {
      title: financeTrendMode === 'year' ? '年度' : '期间',
      dataIndex: 'label',
      fixed: 'left',
      width: 120,
    },
    ...activeFinanceMetrics.map((metric) => ({
      title: metric.label,
      key: metric.key,
      align: 'right' as const,
      render: (_: unknown, row: FinanceTrendsResponse['records'][number]) => formatFinanceMetricValue(row[metric.key], {
        ...metric,
        axis: financeCompareMode === 'raw' ? metric.axis : 'percent',
      }),
    })),
  ], [activeFinanceMetrics, financeCompareMode, financeTrendMode]);

  const dividendColumns: ColumnsType<XdXrItem> = [
    { title: '日期', dataIndex: 'Date', render: (value) => formatTdxDate(value) },
    { title: '类型', dataIndex: 'Category' },
    { title: '分红(元)', dataIndex: 'FenHong', align: 'right', render: (value) => value > 0 ? value.toFixed(4) : '-' },
    { title: '送转(股)', dataIndex: 'SongZhuanGu', align: 'right', render: (value) => value > 0 ? value.toFixed(2) : '-' },
    { title: '配股价', dataIndex: 'PeiGuJia', align: 'right', render: (value) => value > 0 ? value.toFixed(2) : '-' },
    { title: '流通盘', dataIndex: 'PanHouLiuTong', align: 'right', render: (value) => value > 0 ? `${(value / 10000).toFixed(1)}万` : '-' },
    { title: '总股本', dataIndex: 'HouZongGuBen', align: 'right', render: (value) => value > 0 ? `${(value / 10000).toFixed(1)}万` : '-' },
  ];

  const minuteColumns: ColumnsType<MinuteItem> = useMemo(() => {
    const lastClose = quote?.LastClose || 0;
    return [
      { title: '时间', dataIndex: 'Time', width: 90, render: (value: string) => formatTime(value) },
      {
        title: '价格',
        dataIndex: 'Price',
        align: 'right',
        render: (value: number) => <span className={value >= lastClose ? 'price-up' : 'price-down'}>{value.toFixed(2)}</span>,
      },
      {
        title: '涨跌',
        align: 'right',
        render: (_, row) => {
          const chg = row.Price - lastClose;
          return <span className={chg >= 0 ? 'price-up' : 'price-down'}>{chg > 0 ? '+' : ''}{chg.toFixed(2)}</span>;
        },
      },
      {
        title: '涨幅',
        align: 'right',
        render: (_, row) => {
          const chgPct = lastClose > 0 ? ((row.Price - lastClose) / lastClose) * 100 : 0;
          return <span className={chgPct >= 0 ? 'price-up' : 'price-down'}>{chgPct > 0 ? '+' : ''}{chgPct.toFixed(2)}%</span>;
        },
      },
      { title: '成交量', dataIndex: 'Number', align: 'right', render: (value: number) => `${Math.abs(value).toLocaleString()}手` },
    ];
  }, [quote]);

  const handleExportFinanceCsv = () => {
    if (financeDisplayRecords.length === 0 || activeFinanceMetrics.length === 0) {
      message.warning('暂无可导出的财务趋势数据');
      return;
    }
    exportCsv(
      `${code}-finance-${financeTrendMode}-${financeCompareMode}.csv`,
      ['期间', ...activeFinanceMetrics.map((metric) => metric.label)],
      financeDisplayRecords.map((record) => [
        record.label,
        ...activeFinanceMetrics.map((metric) => {
          const axis = financeCompareMode === 'raw' ? metric.axis : 'percent';
          return formatFinanceMetricValue(record[metric.key], { ...metric, axis });
        }),
      ]),
    );
    void message.success('已导出财务趋势数据 CSV');
  };

  const handleExportFinanceChart = (groupKey: 'amount' | 'margin') => {
    const chartRef = groupKey === 'amount' ? amountChartRef.current : marginChartRef.current;
    const image = chartRef?.exportImage();
    if (!image) {
      message.warning('当前图表尚未准备好，稍后再试');
      return;
    }
    downloadDataUrl(`${code}-finance-${groupKey}-${financeTrendMode}-${financeCompareMode}.png`, image);
    void message.success('已导出财务趋势图');
  };

  const selectedMinute = highlightedIdx >= 0 && highlightedIdx < minuteData.length ? minuteData[highlightedIdx] : null;

  return (
    <div style={fullscreen ? { position: 'fixed', inset: 0, zIndex: 1000, background: '#0b1220', padding: 24, overflow: 'auto' } : undefined}>
      <Space direction="vertical" size={16} style={{ display: 'flex' }}>
        <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(30,41,59,0.95), rgba(15,23,42,0.92))' }}>
          <Flex justify="space-between" align="flex-start" gap={16} wrap>
            <Space direction="vertical" size={14} style={{ flex: 1, minWidth: 280 }}>
              {showTabs && quote && (
                <Space direction="vertical" size={12} style={{ display: 'flex' }}>
                  <Space wrap size={[8, 8]}>
                    <Tag color="blue">{quote.Code || code}</Tag>
                    <Tag color={ktype === 'day' ? 'geekblue' : 'purple'}>{ktype.toUpperCase()}</Tag>
                    <Tag color={detailStatus === 'ready' ? 'success' : 'default'}>实时分析</Tag>
                    {syncState?.last_date && (
                      <Tooltip title={syncState.last_sync_at ? `最后同步: ${formatDate(new Date(syncState.last_sync_at))}` : '同步时间未知'}>
                        <Tag color={syncState.status === 'ok' ? 'cyan' : 'orange'}>
                          数据截至 {syncState.last_date}
                        </Tag>
                      </Tooltip>
                    )}
                    {syncState?.status === 'failed' && syncState.error && (
                      <Tooltip title={syncState.error}>
                        <Tag color="red">同步异常</Tag>
                      </Tooltip>
                    )}
                  </Space>
                  <Space wrap size={[8, 8]} align="center">
                    <Typography.Title level={2} style={{ margin: 0 }}>{quote.Name}</Typography.Title>
                    <Typography.Text style={{ fontSize: 32, fontWeight: 700, color: valueColor }}>
                      {quote.Price?.toFixed(2)}
                    </Typography.Text>
                    <Tag color={up ? 'red' : 'green'}>{formatSigned(pct, '%')}</Tag>
                    <Typography.Text style={{ color: valueColor }}>
                      {formatSigned(quote.Price - quote.LastClose)}
                    </Typography.Text>
                  </Space>
                  <Typography.Text type="secondary">
                    结合行情、技术指标、分时与 F10 数据，适合做单只股票的快速研判。
                  </Typography.Text>
                </Space>
              )}
            </Space>
            <Button
              icon={fullscreen ? <CompressOutlined /> : <ExpandOutlined />}
              onClick={() => setFullscreen(!fullscreen)}
            >
              {fullscreen ? '退出全屏' : '全屏'}
            </Button>
          </Flex>
        </Card>

        {showTabs && quote && (
          <Card>
            <Row gutter={[16, 16]}>
              <Col xs={12} md={8} xl={4}><Statistic title="现价" value={quote.Price} precision={2} valueStyle={{ color: valueColor }} /></Col>
              <Col xs={12} md={8} xl={4}><Statistic title="涨跌幅" value={pct} suffix="%" precision={2} valueStyle={{ color: valueColor }} /></Col>
              <Col xs={12} md={8} xl={4}><Statistic title="开盘" value={quote.Open} precision={2} /></Col>
              <Col xs={12} md={8} xl={4}><Statistic title="收盘" value={latestClose} precision={2} valueStyle={{ color: latestClose ? valueColor : undefined }} /></Col>
              <Col xs={12} md={8} xl={4}><Statistic title="昨收" value={quote.LastClose} precision={2} /></Col>
              <Col xs={12} md={8} xl={4}><Statistic title="最高" value={quote.High} precision={2} valueStyle={{ color: '#ef4444' }} /></Col>
              <Col xs={12} md={8} xl={4}><Statistic title="最低" value={quote.Low} precision={2} valueStyle={{ color: '#22c55e' }} /></Col>
              <Col xs={12} md={8} xl={4}><Statistic title="成交量" value={quote.Volume / 10000} suffix="万" precision={0} /></Col>
              <Col xs={12} md={8} xl={4}><Statistic title="成交额" value={amountWanToYi(quote.Amount)} suffix="亿" precision={2} /></Col>
            </Row>
          </Card>
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

        {showTabs && tab === 'signal' && chartLoading && (
          <Card><Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin size="large" /></Flex></Card>
        )}

        {showTabs && tab === 'signal' && !chartLoading && (
          <Space direction="vertical" size={16} style={{ display: 'flex' }}>
            {/* 信号解读卡片 */}
            {analysis && (
              <SignalInterpretationCard
                interpretations={analysis.interpretations ?? []}
                overallSummary={analysis.overall_summary ?? '当前无明显技术信号'}
                trend={analysis.trend ?? '趋势不明'}
              />
            )}

            <Row gutter={[16, 16]}>
              <Col xs={24} xl={12}>
                <Card>
                  <Statistic
                    title="信号回测样本"
                    value={analysis?.signals ?? 0}
                    suffix={`/ ${analysis?.count ?? 0}`}
                  />
                </Card>
              </Col>
              <Col xs={24} xl={12}>
                <Card>
                  <Space direction="vertical" size={8} style={{ display: 'flex' }}>
                    <Typography.Text type="secondary">上涨/下跌强度</Typography.Text>
                    <Progress
                      percent={Math.min(100, Math.max(0, 50 + pct * 5))}
                      strokeColor={up ? '#ef4444' : '#22c55e'}
                      showInfo={false}
                    />
                    <Typography.Text>{formatSigned(pct, '%')}</Typography.Text>
                  </Space>
                </Card>
              </Col>
            </Row>

            <Card title={<Space><ThunderboltOutlined />信号列表</Space>}>
              <Table<Signal>
                size="small"
                pagination={{ pageSize: 20, showSizeChanger: false, showTotal: (total) => `共 ${total} 条` }}
                rowKey={(row, index) => `${row.Date}-${row.Type}-${row.Indicator}-${index}`}
                dataSource={sortedSignals}
                locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无信号" /> }}
                columns={[
                  {
                    title: '日期',
                    dataIndex: 'Date',
                    defaultSortOrder: 'ascend',
                    sorter: (a, b) => compareDateDesc(a.Date, b.Date),
                    render: (value) => formatTdxDate(value),
                  },
                  { title: '指标', dataIndex: 'Indicator' },
                  { title: '信号', dataIndex: 'Type' },
                  { title: '强度', dataIndex: 'Strength', align: 'right', render: (value) => typeof value === 'number' ? value.toFixed(3) : '-' },
                  { title: '详情', dataIndex: 'Details', render: (value) => <Tag>{value || '触发'}</Tag> },
                ]}
              />
            </Card>

            {!analysis ? (
              <Card><Flex justify="center" align="center" style={{ minHeight: 160 }}><Spin /></Flex></Card>
            ) : analysis.summary.length > 0 ? (
              <Card title="信号回测" extra={<Typography.Text type="secondary">基于历史 {analysis.count} 根K线中的 {analysis.signals} 个信号</Typography.Text>}>
                <Table
                  pagination={false}
                  rowKey={(row) => row.type}
                  size="small"
                  dataSource={analysis.summary}
                  columns={[
                    { title: '信号', dataIndex: 'type' },
                    { title: '建议', dataIndex: 'action', render: (value) => <Tag color={value === '买入参考' ? 'red' : 'green'}>{value}</Tag> },
                    { title: '触发次数', dataIndex: 'count', align: 'right' },
                    { title: '次日上涨率', dataIndex: 'win1', align: 'right', render: (_, row) => row.valid1 > 0 ? `${row.win1.toFixed(0)}% (${row.valid1})` : '-' },
                    { title: '5日上涨率', dataIndex: 'win5', align: 'right', render: (_, row) => row.valid5 > 0 ? `${row.win5.toFixed(0)}% (${row.valid5})` : '-' },
                    { title: '10日上涨率', dataIndex: 'win10', align: 'right', render: (_, row) => row.valid10 > 0 ? `${row.win10.toFixed(0)}% (${row.valid10})` : '-' },
                    { title: '20日上涨率', dataIndex: 'win20', align: 'right', render: (_, row) => row.valid20 > 0 ? `${row.win20.toFixed(0)}% (${row.valid20})` : '-' },
                    { title: '次日均涨幅', dataIndex: 'avg1', align: 'right', render: (value, row) => row.valid1 > 0 ? `${value > 0 ? '+' : ''}${value.toFixed(2)}%` : '-' },
                    { title: '5日均涨幅', dataIndex: 'avg5', align: 'right', render: (value, row) => row.valid5 > 0 ? `${value > 0 ? '+' : ''}${value.toFixed(2)}%` : '-' },
                  ]}
                />
              </Card>
            ) : (
              <Card><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无信号回测数据" /></Card>
            )}

            {sortedSignalOutcomes.length > 0 && (
              <Card title="信号明细">
                <Table
                  size="small"
                  pagination={{ pageSize: 20, showSizeChanger: false, showTotal: (total) => `共 ${total} 条` }}
                  rowKey={(row) => `${row.date}-${row.type}-${row.indicator}`}
                  dataSource={sortedSignalOutcomes}
                  columns={[
                    { title: '日期', dataIndex: 'date', render: (value) => formatDate(value) },
                    { title: '指标', dataIndex: 'indicator' },
                    { title: '信号', dataIndex: 'type' },
                    { title: '建议', dataIndex: 'action', render: (value) => <Tag color={value === '买入参考' ? 'red' : 'green'}>{value}</Tag> },
                    { title: '触发价', dataIndex: 'price', align: 'right', render: (value) => value.toFixed(2) },
                    { title: '次日涨跌', dataIndex: 'chg1', align: 'right', render: (value) => formatChange(value) },
                    { title: '5日涨跌', dataIndex: 'chg5', align: 'right', render: (value) => formatChange(value) },
                    { title: '10日涨跌', dataIndex: 'chg10', align: 'right', render: (value) => formatChange(value) },
                    { title: '20日涨跌', dataIndex: 'chg20', align: 'right', render: (value) => formatChange(value) },
                  ]}
                />
              </Card>
            )}
          </Space>
        )}

        {showTabs && tab === 'compare' && (
          compareLoading ? (
            <Skeleton active paragraph={{ rows: 8 }} title={false} />
          ) : compareData ? (
            <StockCompareView
              code={compareData.code}
              stockName={compareData.stock_name}
              stockChange={compareData.stock_change}
              comparisons={compareData.comparisons}
              loading={compareLoading}
            />
          ) : (
            <Empty description="暂无对比数据" />
          )
        )}

        {showTabs && tab === 'finance' && finance && (
          <Space direction="vertical" size={16} style={{ display: 'flex' }}>
            <Card title="财务概览">
              <Descriptions bordered column={{ xs: 1, md: 2, xl: 4 }}>
                {financeItems.map(([label, value, unit]) => (
                  <Descriptions.Item key={String(label)} label={label}>
                    {typeof value === 'number' ? value.toLocaleString() : value} {unit}
                  </Descriptions.Item>
                ))}
              </Descriptions>
            </Card>

            {financeMetrics && financeMetrics.tables.length > 0 && (
              <Card title="主要财务指标" extra={<Typography.Text type="secondary">来自 F10「财务分析」文本解析</Typography.Text>}>
                <Tabs
                  items={financeMetrics.tables.map((table) => ({
                    key: table.title,
                    label: table.title,
                    children: (
                      <Table
                        size="small"
                        rowKey={(row) => row.name}
                        dataSource={table.rows}
                        pagination={false}
                        scroll={{ x: 'max-content' }}
                        columns={[
                          { title: '指标', dataIndex: 'name', fixed: 'left', width: 180 },
                          ...table.periods.map((period, index) => ({
                            title: formatDate(period),
                            key: period,
                            align: 'right' as const,
                            render: (_: unknown, row: { values: string[] }) => row.values[index] || '-',
                          })),
                        ]}
                      />
                    ),
                  }))}
                />
              </Card>
            )}

            <Card
              title={<Space><BarChartOutlined />财务趋势</Space>}
              extra={
                <Space wrap>
                  <Radio.Group
                    value={financeTrendMode}
                    onChange={(event) => setFinanceTrendMode(event.target.value as 'quarter' | 'year')}
                    optionType="button"
                    buttonStyle="solid"
                    options={[
                      { label: '按季度', value: 'quarter' },
                      { label: '按年度', value: 'year' },
                    ]}
                  />
                  <Segmented<FinanceCompareMode>
                    value={financeCompareMode}
                    onChange={(value) => setFinanceCompareMode(value)}
                    options={[
                      { label: '原值', value: 'raw' },
                      { label: '同比', value: 'yoy' },
                      { label: '环比', value: 'qoq' },
                    ]}
                  />
                  <Segmented<FinanceViewMode>
                    value={financeViewMode}
                    onChange={(value) => setFinanceViewMode(value)}
                    options={[
                      { label: '图表', value: 'chart' },
                      { label: '表格', value: 'table' },
                    ]}
                  />
                  <Button icon={<FileExcelOutlined />} onClick={handleExportFinanceCsv}>导出数据</Button>
                  <Checkbox.Group
                    value={selectedFinanceMetrics.filter((metric) => availableFinanceMetrics.some((item) => item.key === metric))}
                    options={availableFinanceMetrics.map((metric) => ({ label: metric.label, value: metric.key }))}
                    onChange={(values) => setSelectedFinanceMetrics(values as string[])}
                  />
                </Space>
              }
            >
              {financeTrendLoading ? (
                <Flex justify="center" align="center" style={{ minHeight: 320 }}><Spin size="large" /></Flex>
              ) : financeTrends && activeFinanceMetrics.length > 0 ? (
                <Space direction="vertical" size={16} style={{ display: 'flex' }}>
                  <Row gutter={[16, 16]}>
                    {activeFinanceMetrics.slice(0, 4).map((metric) => (
                      <Col key={metric.key} xs={24} md={12} xl={6}>
                        <Card size="small">
                          <Statistic
                            title={metric.label}
                            value={typeof latestFinanceRecord?.[metric.key] === 'number' ? latestFinanceRecord[metric.key] : undefined}
                            formatter={(value) => formatFinanceMetricValue(
                              typeof value === 'number' ? value : Number(value),
                              { ...metric, axis: financeCompareMode === 'raw' ? metric.axis : 'percent' },
                            )}
                            valueStyle={{ color: metric.color, fontSize: 20 }}
                            suffix={latestFinanceRecord?.label}
                          />
                        </Card>
                      </Col>
                    ))}
                  </Row>
                  {financeViewMode === 'chart' ? (
                    <Row gutter={[16, 16]}>
                      {financeChartGroups.map((group) => {
                        const selectedGroupMetrics = group.metrics.filter((metric) => selectedFinanceMetrics.includes(metric.key));
                        const chartMetrics = selectedGroupMetrics.map((metric) => ({
                          ...metric,
                          axis: financeCompareMode === 'raw' ? metric.axis : 'percent',
                        }));
                        const chartRef = group.key === 'amount' ? amountChartRef : marginChartRef;
                        return (
                          <Col key={group.key} xs={24}>
                            <Card
                              size="small"
                              title={group.title}
                              extra={
                                <Space>
                                  <Typography.Text type="secondary">{group.description}</Typography.Text>
                                  <Button size="small" icon={<CameraOutlined />} onClick={() => handleExportFinanceChart(group.key as 'amount' | 'margin')}>
                                    导出图片
                                  </Button>
                                </Space>
                              }
                            >
                              {chartMetrics.length > 0 ? (
                                <FinanceTrendChart
                                  ref={chartRef}
                                  records={financeDisplayRecords}
                                  metrics={chartMetrics}
                                  mode={financeTrendMode}
                                />
                              ) : (
                                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请至少选择一个该分组下的指标" />
                              )}
                            </Card>
                          </Col>
                        );
                      })}
                    </Row>
                  ) : (
                    <Card size="small" title="财务趋势表格">
                      <Table
                        size="small"
                        rowKey={(row) => row.period}
                        columns={financeTableColumns}
                        dataSource={[...financeDisplayRecords].reverse()}
                        pagination={{ pageSize: financeTrendMode === 'year' ? 10 : 12 }}
                        scroll={{ x: 'max-content' }}
                      />
                    </Card>
                  )}
                  <Typography.Text type="secondary">
                    数据来源于通达信 F10「财务分析」栏目，支持原值、同比、环比视角，以及图表/表格切换和导出。
                  </Typography.Text>
                </Space>
              ) : (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无可绘制的财务趋势数据" />
              )}
            </Card>
          </Space>
        )}

        {showTabs && tab === 'company' && (
          <TabContent>
            <Row gutter={[16, 16]}>
              <Col xs={24} lg={6}>
                <Card title="F10目录">
                  <List
                    size="small"
                    dataSource={companyCats}
                    renderItem={(cat) => (
                      <List.Item>
                        <Button type={selectedCat === cat.Name ? 'primary' : 'text'} block onClick={() => void loadCompanyContent(cat)}>
                          {cat.Name}
                        </Button>
                      </List.Item>
                    )}
                  />
                </Card>
              </Col>
              <Col xs={24} lg={18}>
                <Card title={<Space><InfoCircleOutlined />内容</Space>}>
                  {companyContent ? (
                    <TdxContent sections={parseTdxText(companyContent)} />
                  ) : (
                    <Empty description="点击左侧目录查看内容" />
                  )}
                </Card>
              </Col>
            </Row>
          </TabContent>
        )}

        {showTabs && tab === 'dividend' && (
          <Card title="分红与除权除息">
            <Table rowKey={(row) => `${row.Date}-${row.Category}`} columns={dividendColumns} dataSource={dividends} size="small" pagination={{ pageSize: 12 }} />
          </Card>
        )}

        {showTabs && tab === 'intraday' && minuteLoading && (
          <Card><Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin size="large" /></Flex></Card>
        )}

        {showTabs && tab === 'intraday' && !minuteLoading && minuteData.length === 0 && (
          <Card><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={minuteError || '暂无可展示的分时数据'} /></Card>
        )}

        {showTabs && tab === 'intraday' && !minuteLoading && minuteData.length > 0 && (() => {
          const lastClose = quote?.LastClose || 0;
          let totalAmount = 0;
          let totalVolume = 0;
          minuteData.forEach((m) => {
            const vol = Math.abs(m.Number);
            totalAmount += m.Price * vol;
            totalVolume += vol;
          });
          const vwap = totalVolume > 0 ? totalAmount / totalVolume : lastClose;
          const sVol = quote?.SVol || 0;
          const bVol = quote?.BVol || 0;
          const innerVol = Math.abs(sVol);
          const outerVol = Math.abs(bVol);
          const totalVol = innerVol + outerVol;
          const innerPct = totalVol > 0 ? (innerVol / totalVol) * 100 : 50;
          const outerPct = totalVol > 0 ? (outerVol / totalVol) * 100 : 50;
          const turnover = finance?.LiuTongGuBen > 0 ? ((quote?.Volume || 0) / finance.LiuTongGuBen) * 100 : 0;

          return (
            <Space direction="vertical" size={16} style={{ display: 'flex' }}>
              <Card>
                <Space wrap size={[16, 12]}>
                  <Typography.Text type="secondary">{minuteDate || formatShortDate(new Date())}</Typography.Text>
                  <Typography.Text>昨收 {lastClose.toFixed(2)}</Typography.Text>
                  <Typography.Text>均价 <span className={vwap >= lastClose ? 'price-up' : 'price-down'}>{vwap.toFixed(2)}</span></Typography.Text>
                  <Typography.Text>内盘 {(innerVol / 10000).toFixed(0)}万 ({innerPct.toFixed(1)}%)</Typography.Text>
                  <Typography.Text>外盘 {(outerVol / 10000).toFixed(0)}万 ({outerPct.toFixed(1)}%)</Typography.Text>
                  {turnover > 0 && <Typography.Text>换手 {turnover.toFixed(2)}%</Typography.Text>}
                </Space>
              </Card>
              <Row gutter={[16, 16]}>
                <Col xs={24} xl={15}>
                  <Card bodyStyle={{ padding: 0 }}>
                    <MinuteChart
                      data={minuteData}
                      lastClose={lastClose}
                      onClickIndex={(idx) => {
                        setHighlightedIdx(idx);
                        const tableIdx = minuteData.length - 1 - idx;
                        const el = tradeRowRefs.current[tableIdx];
                        if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
                      }}
                    />
                  </Card>
                  {selectedMinute && (
                    <Card size="small" style={{ marginTop: 12 }}>
                      <Space wrap>
                        <Typography.Text>选中: {formatTime(selectedMinute.Time)}</Typography.Text>
                        <Typography.Text>价格: {selectedMinute.Price.toFixed(2)}</Typography.Text>
                        <Typography.Text>成交量: {Math.abs(selectedMinute.Number).toLocaleString()}手</Typography.Text>
                        <Button size="small" onClick={() => setHighlightedIdx(-1)}>清除选择</Button>
                      </Space>
                    </Card>
                  )}
                </Col>
                <Col xs={24} xl={9}>
                  <Card title="成交明细">
                    <Table
                      size="small"
                      pagination={{ pageSize: 12 }}
                      rowKey={(row, index) => `${row.Time}-${index}`}
                      dataSource={[...minuteData].reverse()}
                      columns={minuteColumns}
                      onRow={(_, rowIndex) => ({
                        onClick: () => {
                          if (typeof rowIndex === 'number') setHighlightedIdx(minuteData.length - 1 - rowIndex);
                        },
                      })}
                    />
                  </Card>
                </Col>
              </Row>
            </Space>
          );
        })()}
      </Space>
    </div>
  );
}

function formatChange(value: number | null | undefined) {
  if (value === null || value === undefined) return '-';
  return <span className={value >= 0 ? 'price-up' : 'price-down'}>{value > 0 ? '+' : ''}{value.toFixed(2)}%</span>;
}
