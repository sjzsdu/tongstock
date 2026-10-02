import { describe, expect, it } from 'vitest';
import { antdSortedDates, signalDateAscComparator } from './signalSort';

interface Row { Date?: string }

const rows: Row[] = [
  { Date: '2015-01-27' },
  { Date: '2024-06-18' },
  { Date: '2015-03-16' },
  { Date: '2020-11-02' },
  { Date: undefined },
];

describe('signalDateAscComparator', () => {
  it('按日期升序比较（旧→新）', () => {
    expect(signalDateAscComparator({ Date: '2015-01-27' }, { Date: '2024-06-18' })).toBeLessThan(0);
    expect(signalDateAscComparator({ Date: '2024-06-18' }, { Date: '2015-01-27' })).toBeGreaterThan(0);
    expect(signalDateAscComparator({ Date: '2020-11-02' }, { Date: '2020-11-02' })).toBe(0);
  });

  it('空日期排在最前（升序）', () => {
    expect(signalDateAscComparator({ Date: undefined }, { Date: '2020-11-02' })).toBeLessThan(0);
  });
});

describe('antd descend 语义下的信号列表首行', () => {
  // antd Table 在 defaultSortOrder='descend' 时反转比较器结果（等价于升序排序后整体反转），
  // 因此组件里升序比较器 + descend 必须让首行=最新日期。
  it('descend 时首行是最新日期', () => {
    const sorted = antdSortedDates(rows, 'descend');
    expect(sorted[0].Date).toBe('2024-06-18');
    expect(sorted[sorted.length - 1].Date).toBeUndefined();
  });

  it('ascend 时首行是最旧日期', () => {
    const sorted = antdSortedDates(rows, 'ascend');
    expect(sorted[0].Date).toBeUndefined();
    expect(sorted[sorted.length - 1].Date).toBe('2024-06-18');
  });
});
