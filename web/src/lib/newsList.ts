import type { NewsSummary } from '../types/api';

/** 热度低于该值不展示标签，否则每条都挂一个分数反而全是噪声 */
export const HOT_TAG_THRESHOLD = 50;
/** 关联股票只展示置信度足够的：低置信度多为正文顺带提及，上列表只会误导 */
export const MIN_STOCK_CONFIDENCE = 0.5;
export const MAX_STOCK_CHIPS = 4;

/** 列表里展示的关联股票：按置信度过滤、去重、限量 */
export function stockChips(news: NewsSummary): string[] {
  const codes: string[] = [];
  for (const ref of news.stockRefs ?? []) {
    if (ref.confidence < MIN_STOCK_CONFIDENCE || codes.includes(ref.code)) continue;
    codes.push(ref.code);
    if (codes.length >= MAX_STOCK_CHIPS) break;
  }
  return codes;
}

/**
 * 追加翻页结果时按 id 去重：既去掉与已有列表重叠的条目，也去掉本页内部的重复。
 * 信息流是活的——抓取会往前面插新条目，第 2 页会把上一页尾部再带上来，
 * 不去重就会看到同一条新闻出现两次。
 */
export function mergeNews(prev: NewsSummary[], next: NewsSummary[]): NewsSummary[] {
  const seen = new Set(prev.map((item) => item.id));
  const merged = [...prev];
  for (const item of next) {
    if (seen.has(item.id)) continue;
    seen.add(item.id);
    merged.push(item);
  }
  return merged;
}
