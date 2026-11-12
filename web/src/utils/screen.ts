import type { ScreenResult } from '../types/api';
import type { CodesCacheEntry } from '../types/screen';
export function stockNamesFromCodesCache(
  codes: string[],
  codesCache: Record<string, CodesCacheEntry>,
): { code: string; name: string }[] {
  const grouped: Record<string, string[]> = { sz: [], sh: [], bj: [] };
  for (const code of codes) {
    if (code.startsWith('6')) grouped.sh.push(code);
    else if (code.startsWith('8') || code.startsWith('9')) grouped.bj.push(code);
    else grouped.sz.push(code);
  }

  const results: { code: string; name: string }[] = [];
  for (const [exchange, codeList] of Object.entries(grouped)) {
    if (codeList.length === 0) continue;
    const cached = codesCache[exchange];
    if (!cached) continue;
    for (const code of codeList) {
      const stockInfo = cached.list.find((item) => item.Code === code);
      if (stockInfo?.Name) {
        results.push({ code, name: stockInfo.Name });
      }
    }
  }
  return results;
}

/** 后端状态日可能是 YYYYMMDD，统一按 YYYY-MM-DD 展示。 */
export function formatStateDate(value: string): string {
  return /^\d{8}$/.test(value) ? `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}` : value;
}

export function formatPercent(value: number): string {
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`;
}

/**
 * 涨跌幅：标准口径相对昨收（PrevClose），与行情软件一致。
 * 筛选使用指定状态日的收盘价与该日之前的最近收盘价。
 */
export function getChangePct(result: ScreenResult): number {
  const last = result.last;
  if (!last) return 0;
  const close = last.Close || 0;
  if (last.PrevClose && last.PrevClose > 0) {
    return ((close - last.PrevClose) / last.PrevClose) * 100;
  }
  return 0;
}

export function getPriceColor(value: number): string {
  if (value > 0) return 'var(--ant-color-error)';
  if (value < 0) return 'var(--ant-color-success)';
  return 'var(--ant-color-text-secondary)';
}

export function exportCsv(filename: string, headers: string[], rows: string[][]): void {
  const csv = [headers, ...rows]
    .map((row) => row.map((cell) => `"${String(cell ?? '').replace(/"/g, '""')}"`).join(','))
    .join('\n');
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}