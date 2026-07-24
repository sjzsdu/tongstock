import type { ScreenResult } from '../types/api';
import type { CodesCacheEntry } from '../types/screen';
import { SIGNAL_OPTIONS } from '../types/screen';

export function isBuySignal(type: string): boolean {
  return SIGNAL_OPTIONS.find((signal) => signal.value === type)?.buy ?? false;
}

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

export function formatPercent(value: number): string {
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`;
}

export function getChangePct(result: ScreenResult): number {
  const close = result.last?.Close || 0;
  const open = result.last?.Open || close;
  return open > 0 ? ((close - open) / open) * 100 : 0;
}

export function getPriceColor(value: number): string {
  if (value > 0) return 'var(--ant-color-error)';
  if (value < 0) return 'var(--ant-color-success)';
  return 'var(--ant-color-text-secondary)';
}

export function getMaTrend(result: ScreenResult): { label: string; color: string } {
  const n = result.ma?.['5']?.length || 0;
  const ma5 = result.ma?.['5']?.[n - 1] ?? 0;
  const ma10 = result.ma?.['10']?.[n - 1] ?? 0;
  const ma20 = result.ma?.['20']?.[n - 1] ?? 0;

  if (ma5 > ma10 && ma10 > ma20) {
    return { label: '↗ 多头', color: 'red' };
  }
  if (ma5 < ma10 && ma10 < ma20) {
    return { label: '↘ 空头', color: 'green' };
  }
  return { label: '→ 震荡', color: 'default' };
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