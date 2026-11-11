import { HistogramSeries, type IChartApi, type Time } from 'lightweight-charts';
import type { DimensionMeta, KlineItem, StateDim, StateSeriesItem, StateSeriesResponse, StateValue } from '../../types/api';

/**
 * 状态带图层（图形部分）：把「每只股票每天的 4 个状态量」以状态带叠加在 K 线图上。
 *
 * 图上画法：在主图底部为每个维度挂一条隐形刻度泳道，逐日画一个 value=1 的色块；
 * 连续同色的色块连成一条「状态带」，与 K 线共用同一时间轴，缩放/平移天然对齐。
 *
 * 配色与文案一律取自后端下发的 series.dimensions（pkg/signalstate 元数据），
 * 这里不写「哪个状态算买入、哪个状态算卖出」之类的映射：口径只存在于后端枚举，
 * 新增维度或改口径时前端零改动。图例与当日读数见同目录的 StateBandOverlay.tsx。
 */

/** 4 条泳道在价格面板里的纵向布局（底部 0.872~1.0）。 */
const LANE_TOP = 0.872;
const LANE_STEP = 0.031;
const LANE_HEIGHT = 0.027;

/** 每条泳道用的独立价格刻度前缀（不可见刻度，仅用来占位）。 */
const LANE_SCALE_PREFIX = 'state-lane-';

interface LaneContext {
  chart: IChartApi;
  klines: KlineItem[];
  series: StateSeriesResponse;
}

/** 从后端元数据里取出某维度的「状态 → 颜色 / 文案」查找表。 */
export function stateMetaMap(meta: DimensionMeta): Map<StateValue, { color: string; label: string }> {
  const map = new Map<StateValue, { color: string; label: string }>();
  for (const state of meta.states) {
    map.set(state.state, { color: state.color, label: state.label });
  }
  return map;
}

export function dateKeyOf(kline: KlineItem): string {
  return (kline.Time ?? '').slice(0, 10);
}

/** 把 bar 时间归一成可比较的 key：日线 YYYY-MM-DD，分钟线 YYYY-MM-DD HH:MM。 */
export function barTimeKey(value: string | undefined): string {
  const normalized = (value ?? '').trim().replace('T', ' ');
  return normalized.length > 10 ? normalized.slice(0, 16) : normalized.slice(0, 10);
}

/**
 * 状态序列索引。
 * 日线按 date 索引；60 分一天有多根 bar，必须按完整 time 索引，否则后写的会覆盖先写的。
 */
export function indexStateSeries(
  series: StateSeriesResponse | null | undefined,
  intraday = false,
): Map<string, StateSeriesItem> {
  const map = new Map<string, StateSeriesItem>();
  for (const item of series?.items ?? []) {
    const key = intraday ? barTimeKey(item.time ?? item.date) : item.date.slice(0, 10);
    map.set(key, item);
  }
  return map;
}

function toTime(dateStr: string | undefined, intraday: boolean): Time | null {
  if (!dateStr) return null;
  if (intraday) {
    const normalized = dateStr.includes(' ') ? dateStr.replace(' ', 'T') : dateStr;
    const ts = Math.floor(new Date(normalized).getTime() / 1000);
    return Number.isFinite(ts) ? (ts as Time) : null;
  }
  const day = dateStr.slice(0, 10);
  return /^\d{4}-\d{2}-\d{2}$/.test(day) ? (day as Time) : null;
}

/**
 * 在已创建的价格图上挂 4 条状态带。中性也有颜色（灰），所以「满色带」不是空白，
 * 而是明确的「带内中间区」；热身不足的维度用后端给的琥珀色，与中性一眼可分。
 */
export function addStateBands({ chart, klines, series }: LaneContext): void {
  if (klines.length === 0 || series.dimensions.length === 0) return;
  const intraday = new Set(klines.map((k) => dateKeyOf(k))).size < klines.length;
  const byDate = indexStateSeries(series, intraday);

  series.dimensions.forEach((meta, laneIndex) => {
    const scaleId = `${LANE_SCALE_PREFIX}${meta.dim}`;
    const colors = stateMetaMap(meta);
    const lane = chart.addSeries(HistogramSeries, {
      priceScaleId: scaleId,
      priceFormat: { type: 'volume' },
      priceLineVisible: false,
      lastValueVisible: false,
    });
    chart.priceScale(scaleId).applyOptions({
      visible: false,
      scaleMargins: {
        top: LANE_TOP + laneIndex * LANE_STEP,
        bottom: 1 - (LANE_TOP + laneIndex * LANE_STEP + LANE_HEIGHT),
      },
    });

    const data: { time: Time; value: number; color: string }[] = [];
    for (const kline of klines) {
      const time = toTime(kline.Time, intraday);
      if (!time) continue;
      const key = intraday ? barTimeKey(kline.Time) : dateKeyOf(kline);
      const item = byDate.get(key);
      if (!item) continue;
      const stateMeta = colors.get(item.states[meta.dim as StateDim]);
      if (!stateMeta) continue;
      data.push({ time, value: 1, color: stateMeta.color });
    }
    lane.setData(data);
  });
}
