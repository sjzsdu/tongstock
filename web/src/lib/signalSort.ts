// SignalTabContent 信号日期排序比较器。
//
// antd Table 在 defaultSortOrder='descend' 下会把比较器结果再反转一次，
// 因此比较器本身必须写成升序（旧→新）：antd 反转后即「最新在前」，
// 且表头的 ascend/descend 标示与实际内容方向一致。
export function signalDateAscComparator(
  a: { Date?: string } | null | undefined,
  b: { Date?: string } | null | undefined,
): number {
  return String(a?.Date ?? '').localeCompare(String(b?.Date ?? ''));
}

// 复现 antd Table 的排序行为：先按比较器排序，descend 时整体反转。
export function antdSortedDates<T extends { Date?: string }>(rows: T[], defaultSortOrder: 'ascend' | 'descend'): T[] {
  const sorted = [...rows].sort(signalDateAscComparator);
  return defaultSortOrder === 'descend' ? sorted.reverse() : sorted;
}
