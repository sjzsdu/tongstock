/**
 * 浏览器标签页标题。
 *
 * 分两层：
 * 1. 路由级：AppLayout 按路径用 `titleForPath` 写入，切路由即生效；
 * 2. 详情级：个股/事件页在数据到达后用 `useDocumentTitle` 覆盖成更具体的名字
 *    （如「华泰证券(601688) · TongStock」），只在拿得到名字时才写，
 *    避免把路由级标题覆盖成空值。
 */

export const BRAND = 'TongStock';

/** 首页用品牌句，其余页面用「页面 · 品牌」 */
export const HOME_TITLE = `${BRAND} · AI 投资决策`;

/** 指数代码 -> 名称，行情页与标签页标题共用 */
export const INDEX_NAMES: Record<string, string> = {
  '999999': '上证指数',
  '399001': '深证成指',
  '399006': '创业板指',
  '399300': '沪深300',
};

export function withBrand(page: string): string {
  return `${page} · ${BRAND}`;
}

/**
 * 截断超长标题。热点事件标题可能上百字，标签页宽度有限，
 * 只留开头一段，避免被浏览器截成半句。
 */
export function clampTitle(text: string, max = 30): string {
  const clean = text.trim().replace(/\s+/g, ' ');
  if (clean.length <= max) return clean;
  return `${clean.slice(0, max)}…`;
}

export function stockTitle(name: string, code: string): string {
  return withBrand(code ? `${name}(${code})` : name);
}

export function indexTitle(code: string): string {
  return withBrand(INDEX_NAMES[code] || `指数 ${code}`);
}

/** 路由 -> 页面名；首页返回空串 */
const ROUTE_PAGES: Array<[RegExp, string]> = [
  [/^\/stock\/choose(\/|$)/, '个股分析'],
  [/^\/watchlist(\/|$)/, '自选股'],
  [/^\/screen(\/|$)/, '信号筛选'],
  [/^\/portfolio(\/|$)/, '持仓卖出'],
  [/^\/blocks(\/|$)/, '股票池'],
  [/^\/methods\/selection\//, '今日候选股票'],
  [/^\/methods\/advanced(\/|$)/, '高级研究与管理'],
  [/^\/methods(\/|$)/, '我的选股方法'],
  [/^\/paradigms(\/|$)/, '范式库'],
  [/^\/monitoring(\/|$)/, '范式监控'],
  [/^\/agent(\/|$)/, 'AI 助手'],
  [/^\/strategy\/overnight(\/|$)/, '隔夜套利'],
  [/^\/news\/event\/[^/]+/, '热点事件'],
  [/^\/news(\/|$)/, '财经资讯'],
  [/^\/settings(\/|$)/, '配置'],
];

/**
 * 路由 -> 标签页标题。`/stock/:code`、`/index/:code` 先用代码占位，
 * 详情页拿到名称后再覆盖。
 */
export function titleForPath(pathname: string): string {
  if (pathname === '/' || pathname === '') return HOME_TITLE;

  const segments = pathname.split('/').filter(Boolean);
  if (segments[0] === 'stock') {
    if (segments[1] === 'choose' || !segments[1]) return withBrand('个股分析');
    return withBrand(`个股 ${segments[1]}`);
  }
  if (segments[0] === 'index') {
    return segments[1] ? indexTitle(segments[1]) : withBrand('指数详情');
  }

  for (const [pattern, page] of ROUTE_PAGES) {
    if (pattern.test(pathname)) return withBrand(page);
  }
  return withBrand('页面不存在');
}
