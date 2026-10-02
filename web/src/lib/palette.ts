// A股行情色板（唯一语义来源）：涨红 / 跌绿，与国内行情软件通行语义一致。
// 全站价格涨跌相关的颜色必须引用这里，禁止在组件里散落硬编码色值；
// CSS 侧通过 main.tsx 注入的 --price-up / --price-down 变量引用同一来源。
export const PRICE_PALETTE = {
  up: '#ef4444',
  down: '#22c55e',
  // 平盘 / 中性
  flat: '#cbd5e1',
  // 分时均价线（黄）
  avg: '#facc15',
  // 昨收虚线（橙）
  prevClose: '#f59e0b',
} as const;

/** 按涨跌值取价格语义色：>0 红、<0 绿、0 平盘灰 */
export function priceColor(value: number): string {
  if (value > 0) return PRICE_PALETTE.up;
  if (value < 0) return PRICE_PALETTE.down;
  return PRICE_PALETTE.flat;
}

/** hex → rgba，用于量柱等需要透明度的场景 */
export function withAlpha(hex: string, alpha: number): string {
  const v = hex.replace('#', '');
  const r = parseInt(v.slice(0, 2), 16);
  const g = parseInt(v.slice(2, 4), 16);
  const b = parseInt(v.slice(4, 6), 16);
  return `rgba(${r},${g},${b},${alpha})`;
}
