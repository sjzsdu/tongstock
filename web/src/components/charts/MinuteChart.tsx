import { useEffect, useRef } from 'react';
import { createChart, LineSeries, HistogramSeries, type ISeriesApi, type Time } from 'lightweight-charts';
import type { MinuteItem } from '../../types/api';
import { PRICE_PALETTE, priceColor, withAlpha } from '../../lib/palette';
import { buildPriceSegments } from '../../lib/minuteSegments';

interface Props {
  data: MinuteItem[];
  lastClose: number;
  onClickIndex?: (index: number) => void;
}

function timeToTimestamp(timeStr: string): number | null {
  const timePart = timeStr.includes(' ') ? timeStr.split(' ').pop()! : timeStr;
  const [h, m] = timePart.split(':').map(Number);
  if (!Number.isFinite(h) || !Number.isFinite(m)) return null;
  const now = new Date();
  now.setHours(h, m, 0, 0);
  const ts = Math.floor(now.getTime() / 1000);
  return Number.isFinite(ts) ? ts : null;
}

function generateAllTradingMinutes(): number[] {
  const timestamps: number[] = [];
  const make = (h: number, m: number) => {
    const d = new Date();
    d.setHours(h, m, 0, 0);
    return Math.floor(d.getTime() / 1000);
  };
  for (let m = 30; m <= 150; m++) {
    timestamps.push(make(9 + Math.floor(m / 60), m % 60));
  }
  for (let m = 0; m <= 120; m++) {
    timestamps.push(make(13 + Math.floor(m / 60), m % 60));
  }
  return timestamps;
}

export default function MinuteChart({ data, lastClose, onClickIndex }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current || data.length === 0 || lastClose <= 0) return;

    const dataMap = new Map<number, MinuteItem>();
    data.forEach((m) => {
      const ts = timeToTimestamp(m.Time);
      if (ts !== null && typeof m.Price === 'number' && Number.isFinite(m.Price)) dataMap.set(ts, m);
    });

    const allTimestamps = generateAllTradingMinutes();
    const priceData = allTimestamps
      .filter(ts => dataMap.has(ts))
      .map(ts => ({ time: ts, value: dataMap.get(ts)!.Price }));

    if (priceData.length === 0) return;

    const chart = createChart(containerRef.current, {
      width: containerRef.current.clientWidth,
      height: 320,
      layout: {
        background: { color: '#0f172a' },
        textColor: '#64748b',
        fontFamily: 'system-ui, sans-serif',
      },
      grid: {
        vertLines: { color: '#1e293b' },
        horzLines: { color: '#1e293b' },
      },
      crosshair: {
        mode: 1,
        vertLine: { color: '#3b82f6', width: 1, style: 2, labelBackgroundColor: '#3b82f6' },
        horzLine: { color: '#3b82f6', width: 1, style: 2, labelBackgroundColor: '#3b82f6' },
      },
      localization: {
        timeFormatter: (time: number) => {
          const d = new Date(time * 1000);
          return `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`;
        },
      },
      rightPriceScale: { borderColor: '#334155', scaleMargins: { top: 0.08, bottom: 0.18 } },
      timeScale: {
        borderColor: '#334155',
        timeVisible: true,
        secondsVisible: false,
        tickMarkFormatter: (time: Time) => {
          const ts = time as number;
          const d = new Date(ts * 1000);
          const h = d.getHours();
          const m = d.getMinutes();
          if (m === 0 || m === 30) {
            return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}`;
          }
          return '';
        },
      },
    });

    // 分时线：相对昨收动态着色（上红下绿、穿越分段变色，A股通行语义，
    // 与 K 线 upColor/downColor 同一套色板）。
    // 当前价标签挂在含最后一点的段上，颜色跟随价格所在段的方向。
    const segments = buildPriceSegments(priceData, lastClose);
    const lastPointTime = priceData[priceData.length - 1].time;
    let firstSeries: ISeriesApi<'Line'> | null = null;
    for (const seg of segments) {
      const hasLastPoint = seg.points.some(p => p.time === lastPointTime);
      const series = chart.addSeries(LineSeries, {
        color: priceColor(seg.dir),
        lineWidth: 2,
        priceLineVisible: false,
        lastValueVisible: hasLastPoint,
      });
      series.setData(seg.points.map(p => ({ time: p.time as Time, value: p.value })));
      if (!firstSeries) firstSeries = series;
    }

    // 昨收虚线（橙）保留：priceLine 挂在任一价格序列上，位置为昨收价
    if (firstSeries) {
      firstSeries.createPriceLine({
        price: lastClose,
        color: PRICE_PALETTE.prevClose,
        lineWidth: 1,
        lineStyle: 3,
        axisLabelVisible: true,
        title: '昨收',
      });
    }

    let cumAmount = 0;
    let cumVolume = 0;
    const vwapData = allTimestamps.map(ts => {
      const m = dataMap.get(ts);
      if (m) {
        const vol = Math.abs(m.Number);
        cumAmount += m.Price * vol;
        cumVolume += vol;
      }
      return {
        time: ts as Time,
        value: cumVolume > 0 ? cumAmount / cumVolume : lastClose,
      };
    });

    // 均价线（黄）
    const vwapSeries = chart.addSeries(LineSeries, {
      color: PRICE_PALETTE.avg,
      lineWidth: 1,
      lineStyle: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      title: '均价',
    });
    vwapSeries.setData(vwapData);

    // 量柱：相对昨收红/绿（35% 透明度）
    const volSeries = chart.addSeries(HistogramSeries, {
      priceFormat: { type: 'volume' },
      priceScaleId: '',
    });
    volSeries.priceScale().applyOptions({ scaleMargins: { top: 0.85, bottom: 0 } });
    volSeries.setData(allTimestamps.map(ts => {
      const m = dataMap.get(ts);
      return {
        time: ts as Time,
        value: m ? (Math.abs(m.Number) || 0) : 0,
        color: m && m.Price >= lastClose ? withAlpha(PRICE_PALETTE.up, 0.35) : withAlpha(PRICE_PALETTE.down, 0.35),
      };
    }));

    chart.timeScale().fitContent();
    const remainingMinutes = allTimestamps.length - priceData.length;
    if (remainingMinutes > 0) {
      chart.timeScale().applyOptions({ rightOffset: remainingMinutes });
    }

    chart.subscribeClick((param) => {
      if (param.time && onClickIndex) {
        const ts = param.time as number;
        const idx = data.findIndex(m => timeToTimestamp(m.Time) === ts);
        if (idx >= 0) onClickIndex(idx);
      }
    });

    const handleResize = () => {
      if (containerRef.current) chart.applyOptions({ width: containerRef.current.clientWidth });
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      chart.remove();
    };
  }, [data, lastClose, onClickIndex]);

  return <div ref={containerRef} className="w-full rounded-lg overflow-hidden" />;
}
