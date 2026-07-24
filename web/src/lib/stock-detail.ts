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
