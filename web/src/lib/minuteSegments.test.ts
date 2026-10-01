import { describe, expect, it } from 'vitest';
import { buildPriceSegments } from './minuteSegments';

const t = (minute: number) => 9 * 3600 + minute * 60;

describe('buildPriceSegments', () => {
  const lastClose = 10;

  it('全程在昨收上方时只有一段红色', () => {
    const segments = buildPriceSegments(
      [{ time: t(0), value: 10.5 }, { time: t(1), value: 10.8 }, { time: t(2), value: 10.2 }],
      lastClose,
    );
    expect(segments).toHaveLength(1);
    expect(segments[0].dir).toBe(1);
    expect(segments[0].points).toHaveLength(3);
  });

  it('穿越昨收时拆成红绿两段，并在昨收价处插值衔接', () => {
    const segments = buildPriceSegments(
      [
        { time: t(0), value: 10.4 }, // 上方 → 红
        { time: t(1), value: 9.6 },  // 下方 → 绿
        { time: t(2), value: 9.4 },
      ],
      lastClose,
    );
    expect(segments).toHaveLength(2);
    expect(segments[0].dir).toBe(1);
    expect(segments[1].dir).toBe(-1);
    // 前段尾部与后段头部都是昨收价的衔接点
    const tail = segments[0].points[segments[0].points.length - 1];
    const head = segments[1].points[0];
    expect(tail.value).toBe(lastClose);
    expect(head.value).toBe(lastClose);
    expect(head.time).toBe(tail.time);
    // 插值点时间介于两点之间
    expect(tail.time).toBeGreaterThan(t(0));
    expect(tail.time).toBeLessThan(t(1));
  });

  it('价格恰等于昨收并入上方段（dir=1）', () => {
    const segments = buildPriceSegments(
      [{ time: t(0), value: lastClose }, { time: t(1), value: 9.9 }],
      lastClose,
    );
    expect(segments).toHaveLength(2);
    expect(segments[0].dir).toBe(1);
    expect(segments[1].dir).toBe(-1);
  });

  it('多次穿越产生多段且段间衔接', () => {
    const segments = buildPriceSegments(
      [
        { time: t(0), value: 10.2 },
        { time: t(1), value: 9.8 },
        { time: t(2), value: 10.3 },
        { time: t(3), value: 9.7 },
      ],
      lastClose,
    );
    expect(segments.map(s => s.dir)).toEqual([1, -1, 1, -1]);
    for (let i = 1; i < segments.length; i++) {
      expect(segments[i].points[0].value).toBe(lastClose);
    }
  });
});
