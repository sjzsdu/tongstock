import { describe, expect, it } from 'vitest';
import { MAX_STOCK_CHIPS, MIN_STOCK_CONFIDENCE, mergeNews, stockChips } from './newsList';
import type { NewsSummary } from '../types/api';

function news(id: string, overrides: Partial<NewsSummary> = {}): NewsSummary {
  return {
    id,
    source: '财联社',
    newsType: '快讯',
    title: `标题${id}`,
    summary: '',
    publishTime: '2026-09-29T10:00:00+08:00',
    hotScore: 0,
    tags: [],
    relatedStocks: [],
    ...overrides,
  };
}

describe('mergeNews', () => {
  it('追加第二页', () => {
    const merged = mergeNews([news('a'), news('b')], [news('c'), news('d')]);
    expect(merged.map((n) => n.id)).toEqual(['a', 'b', 'c', 'd']);
  });

  it('翻页重叠的条目只保留一次', () => {
    const merged = mergeNews([news('a'), news('b')], [news('b'), news('c')]);
    expect(merged.map((n) => n.id)).toEqual(['a', 'b', 'c']);
  });

  it('同一页内的重复条目也一并去掉', () => {
    const merged = mergeNews([], [news('a'), news('a'), news('b')]);
    expect(merged.map((n) => n.id)).toEqual(['a', 'b']);
  });

  it('不修改入参', () => {
    const prev = [news('a')];
    const next = [news('a'), news('b')];
    mergeNews(prev, next);
    expect(prev).toHaveLength(1);
    expect(next).toHaveLength(2);
  });
});

describe('stockChips', () => {
  it('过滤低置信度关联', () => {
    const item = news('a', {
      stockRefs: [
        { code: '600519', match_type: 'name_hit', confidence: 0.9 },
        { code: '000001', match_type: 'name_hit', confidence: 0.2 },
      ],
    });
    expect(stockChips(item)).toEqual(['600519']);
    expect(MIN_STOCK_CONFIDENCE).toBeGreaterThan(0.2);
  });

  it('重复代码只出现一次且有上限', () => {
    const refs = [
      { code: '600519' as const, match_type: 'name_hit' as const, confidence: 0.9 },
      { code: '600519' as const, match_type: 'code_hit' as const, confidence: 0.8 },
      ...Array.from({ length: 6 }, (_, i) => ({
        code: `60000${i}`,
        match_type: 'name_hit' as const,
        confidence: 0.7,
      })),
    ];
    const chips = stockChips(news('a', { stockRefs: refs }));
    expect(new Set(chips).size).toBe(chips.length);
    expect(chips.length).toBeLessThanOrEqual(MAX_STOCK_CHIPS);
    expect(chips[0]).toBe('600519');
  });

  it('没有关联时不展示股票标签', () => {
    expect(stockChips(news('a'))).toEqual([]);
    expect(stockChips(news('b', { stockRefs: [] }))).toEqual([]);
  });
});
