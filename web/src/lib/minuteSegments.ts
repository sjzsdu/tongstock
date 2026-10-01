// 分时价格线按「相对昨收」拆成多段：昨收之上红、之下绿（A股通行语义，
// 与 K 线 upColor/downColor 同一套色板）。穿越昨收的相邻两点在昨收价处
// 线性插值出衔接点，保证段与段首尾相连。
// lightweight-charts 单序列不支持逐点换色，拆段是改动最小、结果可测的方案。
export interface MinutePoint { time: number; value: number }
export interface PriceSegment { dir: 1 | -1; points: MinutePoint[] }

export function buildPriceSegments(priceData: MinutePoint[], lastClose: number): PriceSegment[] {
  const segments: PriceSegment[] = [];
  const dirOf = (v: number): 1 | -1 => (v >= lastClose ? 1 : -1);
  for (let i = 0; i < priceData.length; i++) {
    const point = priceData[i];
    const dir = dirOf(point.value);
    const prev = segments[segments.length - 1];
    if (!prev) {
      segments.push({ dir, points: [point] });
      continue;
    }
    if (dir === prev.dir) {
      prev.points.push(point);
      continue;
    }
    // 方向翻转：在昨收价处插值出衔接点，分别接到前段尾部与后段头部
    const prevPoint = priceData[i - 1];
    const dv = point.value - prevPoint.value;
    const ratio = dv !== 0 ? (lastClose - prevPoint.value) / dv : 0.5;
    const clamped = Math.max(0, Math.min(1, ratio));
    const crossTime = prevPoint.time + (point.time - prevPoint.time) * clamped;
    prev.points.push({ time: crossTime, value: lastClose });
    segments.push({ dir, points: [{ time: crossTime, value: lastClose }, point] });
  }
  return segments;
}
