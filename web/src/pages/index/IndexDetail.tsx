import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  AreaChartOutlined,
  BarChartOutlined,
  ClockCircleOutlined,
} from '@ant-design/icons';
import {
  Card,
  Col,
  Empty,
  Flex,
  Progress,
  Row,
  Space,
  Statistic,
  Tabs,
  Tag,
  Typography,
  Spin,
} from 'antd';
import { api } from '../../api/client';
import type { IndexBar, MinuteItem } from '../../types/api';
import CandlestickChart from '../../components/charts/CandlestickChart';
import ChartToolbar from '../../components/charts/ChartToolbar';
import MinuteChart from '../../components/charts/MinuteChart';
import { formatShortDate } from '../../lib/datetime';

type Tab = 'chart' | 'intraday' | 'stats';
type DetailStatus = 'loading' | 'ready' | 'not_found';

const INDICES: Record<string, string> = {
  '999999': '上证指数',
  '399001': '深证成指',
  '399006': '创业板指',
  '399300': '沪深300',
};

const TAB_ITEMS: { key: Tab; label: string; icon: React.ReactNode }[] = [
  { key: 'chart', label: 'K线+指标', icon: <AreaChartOutlined /> },
  { key: 'intraday', label: '分时', icon: <ClockCircleOutlined /> },
  { key: 'stats', label: '涨跌统计', icon: <BarChartOutlined /> },
];

function getValueColor(value: number) {
  if (value > 0) return '#ef4444';
  if (value < 0) return '#22c55e';
  return '#cbd5e1';
}

function formatSigned(value: number, suffix = '') {
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}${suffix}`;
}

function formatSignedPercent(value: number) {
  return formatSigned(value, '%');
}

function amountWanToYi(value: number | undefined): number | undefined {
  return typeof value === 'number' ? value / 10000 : undefined;
}

export default function IndexDetail() {
  const { code: paramCode, tab: paramTab } = useParams();
  const navigate = useNavigate();
  const [code, setCode] = useState(paramCode || '');
  const [tab, setTab] = useState<Tab>((paramTab as Tab) || 'chart');
  const [name, setName] = useState('');
  const [klines, setKlines] = useState<IndexBar[]>([]);
  const [indicator, setIndicator] = useState<any>(null);
  const [ktype, setKtype] = useState('day');
  const [mainOverlay, setMainOverlay] = useState('MA');
  const [subPanel, setSubPanel] = useState('MACD');
  const [minuteData, setMinuteData] = useState<MinuteItem[]>([]);
  const [minuteDate, setMinuteDate] = useState<string>('');
  const [minuteLoading, setMinuteLoading] = useState(false);
  const [detailStatus, setDetailStatus] = useState<DetailStatus>('loading');
  const [chartLoading, setChartLoading] = useState(false);

  useEffect(() => {
    if (!paramCode) {
      navigate('/');
      return;
    }
    setCode(paramCode);
    setName(INDICES[paramCode] || paramCode);
    if (paramTab) setTab(paramTab as Tab);
  }, [paramCode, paramTab, navigate]);

  const switchTab = (nextTab: Tab) => {
    setTab(nextTab);
    navigate(`/index/${code}/${nextTab}`, { replace: true });
  };

  const calculateIndicators = (bars: IndexBar[]) => {
    const closes = bars.map(b => b.Close);
    const highs = bars.map(b => b.High);
    const lows = bars.map(b => b.Low);

    const maPeriods = [5, 10, 20, 60];
    const ma: Record<string, number[]> = {};
    maPeriods.forEach(period => {
      ma[period.toString()] = closes.map((_, i) => {
        if (i < period - 1) return 0;
        let sum = 0;
        for (let j = i - period + 1; j <= i; j++) sum += closes[j];
        return sum / period;
      });
    });

    const macdFast = 12, macdSlow = 26, macdSignal = 9;
    const ema = (data: number[], period: number): number[] => {
      const result: number[] = [];
      const multiplier = 2 / (period + 1);
      let prevEma = data[0];
      result.push(prevEma);
      for (let i = 1; i < data.length; i++) {
        prevEma = data[i] * multiplier + prevEma * (1 - multiplier);
        result.push(prevEma);
      }
      return result;
    };
    const fastEma = ema(closes, macdFast);
    const slowEma = ema(closes, macdSlow);
    const dif = fastEma.map((f, i) => f - slowEma[i]);
    const dea = ema(dif, macdSignal);
    const hist = dif.map((d, i) => d - dea[i]);

    const kdjN = 9, kdjM1 = 3, kdjM2 = 3;
    const kdj = (high: number[], low: number[], close: number[], n: number, m1: number, m2: number) => {
      const kResult: number[] = [];
      const dResult: number[] = [];
      const jResult: number[] = [];
      for (let i = 0; i < close.length; i++) {
        if (i < n - 1) {
          kResult.push(50);
          dResult.push(50);
          jResult.push(50);
          continue;
        }
        const minLow = Math.min(...low.slice(i - n + 1, i + 1));
        const maxHigh = Math.max(...high.slice(i - n + 1, i + 1));
        const rsv = maxHigh > minLow ? ((close[i] - minLow) / (maxHigh - minLow)) * 100 : 50;
        const prevK = kResult[i - 1] || 50;
        const k = (2 / m1) * prevK + (1 / m1) * rsv;
        const prevD = dResult[i - 1] || 50;
        const d = (2 / m2) * prevD + (1 / m2) * k;
        const j = 3 * k - 2 * d;
        kResult.push(k);
        dResult.push(d);
        jResult.push(j);
      }
      return { K: kResult, D: dResult, J: jResult };
    };
    const kdjResult = kdj(highs, lows, closes, kdjN, kdjM1, kdjM2);

    const bollN = 20, bollK = 2;
    const bollMiddle = ma['20'];
    const bollUpper: number[] = [];
    const bollLower: number[] = [];
    for (let i = 0; i < closes.length; i++) {
      if (i < bollN - 1) {
        bollUpper.push(0);
        bollLower.push(0);
        continue;
      }
      const sum = closes.slice(i - bollN + 1, i + 1).reduce((s, c) => s + Math.pow(c - bollMiddle[i], 2), 0);
      const std = Math.sqrt(sum / bollN);
      bollUpper.push(bollMiddle[i] + bollK * std);
      bollLower.push(bollMiddle[i] - bollK * std);
    }

    const rsiPeriods = [6, 12, 24];
    const rsi: Record<string, number[]> = {};
    rsiPeriods.forEach(period => {
      const result: number[] = [];
      for (let i = 0; i < closes.length; i++) {
        if (i < period) {
          result.push(50);
          continue;
        }
        let upSum = 0, downSum = 0;
        for (let j = i - period + 1; j <= i; j++) {
          const diff = closes[j] - closes[j - 1];
          if (diff > 0) upSum += diff;
          else downSum += Math.abs(diff);
        }
        const avgUp = upSum / period;
        const avgDown = downSum / period;
        result.push(avgDown === 0 ? 100 : avgUp / (avgUp + avgDown) * 100);
      }
      rsi[period.toString()] = result;
    });

    return {
      ma,
      macd: { DIF: dif, DEA: dea, Hist: hist, HIST: hist },
      kdj: kdjResult,
      boll: { Upper: bollUpper, Middle: bollMiddle, Lower: bollLower },
      rsi,
      signals: [],
    };
  };

  useEffect(() => {
    if (!code) return;
    let cancelled = false;
    setChartLoading(true);
    setKlines([]);
    setIndicator(null);

    const loadIndex = async () => {
      try {
        const bars = await api.index(code, ktype);
        if (cancelled) return;
        if (!bars || bars.length === 0) {
          setDetailStatus('not_found');
          setChartLoading(false);
          return;
        }
        setKlines(bars);
        const indicators = calculateIndicators(bars);
        setIndicator(indicators);
        setDetailStatus('ready');
        setChartLoading(false);
      } catch {
        if (cancelled) return;
        setDetailStatus('not_found');
        setChartLoading(false);
      }
    };

    loadIndex();

    return () => { cancelled = true; };
  }, [code, ktype]);

  useEffect(() => {
    if (!code || detailStatus !== 'ready') return;
    if (tab === 'intraday') {
      const fetchMinute = async () => {
        setMinuteLoading(true);
        let loaded = false;
        try {
          const r = await api.minute(code);
          if (r.List && r.List.length > 0) {
            setMinuteData(r.List);
            const today = new Date();
            setMinuteDate(formatShortDate(today));
            loaded = true;
          }
        } catch {
        }

        if (!loaded) {
          const yesterday = new Date();
          yesterday.setDate(yesterday.getDate() - (yesterday.getDay() === 0 ? 2 : yesterday.getDay() === 1 ? 3 : 1));
          const dateStr = yesterday.toISOString().slice(0, 10).replace(/-/g, '');
          try {
            const histR = await api.minuteHistory(code, dateStr);
            if (histR.List && histR.List.length > 0) {
              setMinuteData(histR.List);
              setMinuteDate(formatShortDate(yesterday));
              loaded = true;
            }
          } catch {
          }
        }

        if (!loaded) {
          setMinuteData([]);
          setMinuteDate('');
        }
        setMinuteLoading(false);
      };
      void fetchMinute();
    }
  }, [code, tab, detailStatus]);

  const pct = useMemo(() => {
    if (klines.length >= 2) {
      const last = klines[klines.length - 1];
      const prev = klines[klines.length - 2];
      return prev.Close > 0 ? ((last.Close - prev.Close) / prev.Close) * 100 : 0;
    }
    return 0;
  }, [klines]);

  const valueColor = getValueColor(pct);
  const up = pct >= 0;
  const showTabs = detailStatus === 'ready';

  const lastBar = klines.length > 0 ? klines[klines.length - 1] : null;
  const prevBar = klines.length > 1 ? klines[klines.length - 2] : null;

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(30,41,59,0.95), rgba(15,23,42,0.92))' }}>
        <Flex justify="space-between" align="flex-start" gap={16} wrap>
          <Space direction="vertical" size={14} style={{ flex: 1, minWidth: 280 }}>
            {showTabs && (
              <Space direction="vertical" size={12} style={{ display: 'flex' }}>
                <Space wrap size={[8, 8]}>
                  <Tag color="blue">{code}</Tag>
                  <Tag color={ktype === 'day' ? 'geekblue' : 'purple'}>{ktype.toUpperCase()}</Tag>
                </Space>
                <Space wrap size={[8, 8]} align="center">
                  <Typography.Title level={2} style={{ margin: 0 }}>{name}</Typography.Title>
                  <Typography.Text style={{ fontSize: 32, fontWeight: 700, color: valueColor }}>
                    {lastBar?.Close?.toFixed(2) ?? '--'}
                  </Typography.Text>
                  <Tag color={up ? 'red' : 'green'}>{formatSignedPercent(pct)}</Tag>
                </Space>
                <Typography.Text type="secondary">
                  指数详情：K线走势、分时图、涨跌统计与成分股分析。
                </Typography.Text>
              </Space>
            )}
          </Space>
        </Flex>
      </Card>

      {showTabs && (
        <Card>
          <Row gutter={[16, 16]}>
            <Col xs={12} md={8} xl={4}>
              <Statistic title="现价" value={lastBar?.Close} precision={2} valueStyle={{ color: valueColor }} />
            </Col>
            <Col xs={12} md={8} xl={4}>
              <Statistic title="涨跌幅" value={pct} suffix="%" precision={2} valueStyle={{ color: valueColor }} />
            </Col>
            <Col xs={12} md={8} xl={4}>
              <Statistic title="开盘" value={lastBar?.Open} precision={2} />
            </Col>
            <Col xs={12} md={8} xl={4}>
              <Statistic title="最高" value={lastBar?.High} precision={2} valueStyle={{ color: '#ef4444' }} />
            </Col>
            <Col xs={12} md={8} xl={4}>
              <Statistic title="最低" value={lastBar?.Low} precision={2} valueStyle={{ color: '#22c55e' }} />
            </Col>
            <Col xs={12} md={8} xl={4}>
              <Statistic title="昨收" value={prevBar?.Close} precision={2} />
            </Col>
            <Col xs={12} md={8} xl={4}>
              <Statistic title="成交量" value={(lastBar?.Volume ?? 0) / 10000} suffix="万" precision={0} />
            </Col>
            <Col xs={12} md={8} xl={4}>
              <Statistic title="成交额" value={amountWanToYi(lastBar?.Amount)} suffix="亿" precision={2} />
            </Col>
          </Row>
        </Card>
      )}

      {detailStatus === 'loading' && (
        <Card>
          <Flex justify="center" align="center" style={{ minHeight: 240 }}>
            <Spin size="large" />
          </Flex>
        </Card>
      )}

      {detailStatus === 'not_found' && (
        <Card>
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <Space direction="vertical" size={4}>
                <Typography.Text strong>未找到该指数</Typography.Text>
                <Typography.Text type="secondary">请确认指数代码是否正确。</Typography.Text>
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
          <Card>
            <ChartToolbar
              ktype={ktype}
              onKtypeChange={setKtype}
              mainOverlay={mainOverlay}
              onMainOverlayChange={setMainOverlay}
              subPanel={subPanel}
              onSubPanelChange={setSubPanel}
            />
          </Card>
          {chartLoading ? (
            <Card>
              <Flex justify="center" align="center" style={{ minHeight: 470 }}>
                <Spin size="large" />
              </Flex>
            </Card>
          ) : (
            <Card bodyStyle={{ padding: 0 }}>
              <CandlestickChart klines={klines} indicator={indicator} mainOverlay={mainOverlay} subPanel={subPanel} />
            </Card>
          )}
        </Space>
      )}

      {showTabs && tab === 'intraday' && minuteLoading && (
        <Card>
          <Flex justify="center" align="center" style={{ minHeight: 240 }}>
            <Spin size="large" />
          </Flex>
        </Card>
      )}

      {showTabs && tab === 'intraday' && !minuteLoading && minuteData.length === 0 && (
        <Card>
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无可展示的分时数据" />
        </Card>
      )}

      {showTabs && tab === 'intraday' && !minuteLoading && minuteData.length > 0 && (() => {
        const lastClose = prevBar?.Close || 0;
        let totalAmount = 0;
        let totalVolume = 0;
        minuteData.forEach((m) => {
          const vol = Math.abs(m.Number);
          totalAmount += m.Price * vol;
          totalVolume += vol;
        });
        const vwap = totalVolume > 0 ? totalAmount / totalVolume : lastClose;

        return (
          <Space direction="vertical" size={16} style={{ display: 'flex' }}>
            <Card>
              <Space wrap size={[16, 12]}>
                <Typography.Text type="secondary">{minuteDate || formatShortDate(new Date())}</Typography.Text>
                <Typography.Text>昨收 {lastClose.toFixed(2)}</Typography.Text>
                <Typography.Text>均价 <span className={vwap >= lastClose ? 'price-up' : 'price-down'}>{vwap.toFixed(2)}</span></Typography.Text>
              </Space>
            </Card>
            <Card bodyStyle={{ padding: 0 }}>
              <MinuteChart data={minuteData} lastClose={lastClose} />
            </Card>
          </Space>
        );
      })()}

      {showTabs && tab === 'stats' && klines.length > 0 && (
        <Space direction="vertical" size={16} style={{ display: 'flex' }}>
          <Card title={<Space><BarChartOutlined />涨跌统计</Space>}>
            <Row gutter={[16, 16]}>
              <Col xs={24} md={8}>
                <Card size="small">
                  <Space direction="vertical" size={8} style={{ display: 'flex' }}>
                    <Typography.Text type="secondary">上涨家数</Typography.Text>
                    <Statistic
                      value={lastBar?.UpCount}
                      precision={0}
                      valueStyle={{ color: '#ef4444', fontSize: 28 }}
                    />
                  </Space>
                </Card>
              </Col>
              <Col xs={24} md={8}>
                <Card size="small">
                  <Space direction="vertical" size={8} style={{ display: 'flex' }}>
                    <Typography.Text type="secondary">下跌家数</Typography.Text>
                    <Statistic
                      value={lastBar?.DownCount}
                      precision={0}
                      valueStyle={{ color: '#22c55e', fontSize: 28 }}
                    />
                  </Space>
                </Card>
              </Col>
              <Col xs={24} md={8}>
                <Card size="small">
                  <Space direction="vertical" size={8} style={{ display: 'flex' }}>
                    <Typography.Text type="secondary">平盘家数</Typography.Text>
                    <Statistic
                      value={(lastBar?.UpCount || 0) + (lastBar?.DownCount || 0) > 0 ? 0 : '-'}
                      precision={0}
                      valueStyle={{ fontSize: 28 }}
                    />
                  </Space>
                </Card>
              </Col>
            </Row>

            <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
              <Col xs={24}>
                <Card size="small" title="涨跌比例">
                  <Space direction="vertical" size={8} style={{ display: 'flex', width: '100%' }}>
                    <Progress
                      percent={lastBar && (lastBar.UpCount + lastBar.DownCount) > 0
                        ? (lastBar.UpCount / (lastBar.UpCount + lastBar.DownCount)) * 100
                        : 50}
                      strokeColor="#ef4444"
                      showInfo={false}
                    />
                    <Flex justify="space-between">
                      <Typography.Text style={{ color: '#22c55e' }}>
                        下跌 {(lastBar?.DownCount || 0)} 家
                      </Typography.Text>
                      <Typography.Text style={{ color: '#ef4444' }}>
                        上涨 {(lastBar?.UpCount || 0)} 家
                      </Typography.Text>
                    </Flex>
                  </Space>
                </Card>
              </Col>
            </Row>

            <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
              <Col xs={24}>
                <Card size="small" title="近5日涨跌家数趋势">
                  <div style={{ display: 'flex', alignItems: 'flex-end', gap: 8, height: 120 }}>
                    {klines.slice(-5).map((bar, i) => {
                      const maxCount = Math.max(...klines.slice(-5).map(b => Math.max(b.UpCount, b.DownCount)));
                      return (
                        <div key={i} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
                          <div style={{ display: 'flex', alignItems: 'flex-end', height: 80, gap: 4, width: '100%', justifyContent: 'center' }}>
                            <div
                              style={{
                                width: '40%',
                                backgroundColor: '#22c55e',
                                height: `${((bar.DownCount || 0) / maxCount) * 100}%`,
                                minHeight: '4px',
                              }}
                            />
                            <div
                              style={{
                                width: '40%',
                                backgroundColor: '#ef4444',
                                height: `${((bar.UpCount || 0) / maxCount) * 100}%`,
                                minHeight: '4px',
                              }}
                            />
                          </div>
                          <Typography.Text style={{ fontSize: 12, marginTop: 4 }}>{bar.Time.slice(5)}</Typography.Text>
                        </div>
                      );
                    })}
                  </div>
                </Card>
              </Col>
            </Row>
          </Card>
        </Space>
      )}
    </Space>
  );
}