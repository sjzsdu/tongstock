export function getValueColor(value: number): string {
  if (value > 0) return '#ef4444';
  if (value < 0) return '#22c55e';
  return '#cbd5e1';
}

export function formatSigned(value: number, suffix = ''): string {
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}${suffix}`;
}

export function amountWanToYi(value: number | undefined): number | undefined {
  return typeof value === 'number' ? value / 10000 : undefined;
}

export function getSyncStatusPresentation(status: string): { label: string; color: string } {
  if (status === 'ok') return { label: '正常', color: 'green' };
  if (status === 'failed') return { label: '失败', color: 'red' };
  return { label: '未记录', color: 'orange' };
}

export function getSyncAgeDays(lastSyncAt: string | undefined, now = Date.now()): number | null {
  if (!lastSyncAt) return null;
  const timestamp = Date.parse(lastSyncAt);
  if (!Number.isFinite(timestamp) || timestamp <= 0 || timestamp > now) return null;
  return Math.floor((now - timestamp) / (1000 * 60 * 60 * 24));
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

export function downloadDataUrl(filename: string, dataUrl: string): void {
  const link = document.createElement('a');
  link.href = dataUrl;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}
