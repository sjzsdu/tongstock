import { useEffect, useRef } from 'react';
import { createChart, LineSeries, HistogramSeries, type Time } from 'lightweight-charts';
import type { MinuteItem } from '../../types/api';

interface Props {
  data: MinuteItem[];
  lastClose: number;
  onClickIndex?: (index: number) => void;
}

function timeToTimestamp(timeStr: string): number {
  const [h, m] = timeStr.split(':').map(Number);
  const now = new Date();
  now.setHours(h, m, 0, 0);
  return Math.floor(now.getTime() / 1000);
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

    const allTimestamps = generateAllTradingMinutes();
    const dataMap = new Map(data.map(m => [timeToTimestamp(m.Time), m]));

    const priceData = allTimestamps
      .filter(ts => dataMap.has(ts))
      .map(ts => ({ time: ts as Time, value: dataMap.get(ts)!.Price }));

    const priceSeries = chart.addSeries(LineSeries, {
      color: '#3b82f6',
      lineWidth: 2,
      priceLineVisible: false,
      lastValueVisible: true,
    });
    priceSeries.setData(priceData);

    priceSeries.createPriceLine({
      price: lastClose,
      color: '#f59e0b',
      lineWidth: 1,
      lineStyle: 3,
      axisLabelVisible: true,
      title: '昨收',
    });

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

    const vwapSeries = chart.addSeries(LineSeries, {
      color: '#f97316',
      lineWidth: 1,
      lineStyle: 2,
      priceLineVisible: false,
      lastValueVisible: false,
      title: '均价',
    });
    vwapSeries.setData(vwapData);

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
        color: m && m.Price >= lastClose ? 'rgba(239,68,68,0.35)' : 'rgba(34,197,94,0.35)',
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
