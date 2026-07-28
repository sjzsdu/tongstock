const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];

function pad2(value: number): string {
  return String(value).padStart(2, '0');
}

function parseDateLike(input: string | number | Date | null | undefined): Date | null {
  if (input === null || input === undefined || input === '') return null;
  if (input instanceof Date) return Number.isNaN(input.getTime()) ? null : input;

  if (typeof input === 'number') {
    const raw = String(input);
    if (/^\d{8}$/.test(raw)) {
      return new Date(Number(raw.slice(0, 4)), Number(raw.slice(4, 6)) - 1, Number(raw.slice(6, 8)));
    }
    const date = new Date(input);
    return Number.isNaN(date.getTime()) ? null : date;
  }

  const trimmed = input.trim();
  if (!trimmed) return null;

  // TDX compact datetime: YYYYMMDD HH:MM[:SS]
  const tdxMatch = trimmed.match(/^(\d{4})(\d{2})(\d{2})\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/);
  if (tdxMatch) {
    return new Date(
      Number(tdxMatch[1]), Number(tdxMatch[2]) - 1, Number(tdxMatch[3]),
      Number(tdxMatch[4]), Number(tdxMatch[5]), tdxMatch[6] ? Number(tdxMatch[6]) : 0
    );
  }

  if (/^\d{8}$/.test(trimmed)) {
    return new Date(Number(trimmed.slice(0, 4)), Number(trimmed.slice(4, 6)) - 1, Number(trimmed.slice(6, 8)));
  }
  const dateOnly = trimmed.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (dateOnly) {
    return new Date(Number(dateOnly[1]), Number(dateOnly[2]) - 1, Number(dateOnly[3]));
  }
  const normalized = trimmed.includes('T') ? trimmed : trimmed.replace(' ', 'T');
  const date = new Date(normalized);
  return Number.isNaN(date.getTime()) ? null : date;
}

export function formatDate(input: string | number | Date | null | undefined, fallback = '-'): string {
  const date = parseDateLike(input);
  if (!date) return fallback;
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`;
}

export function formatShortDate(input: string | number | Date | null | undefined, fallback = '-'): string {
  const date = parseDateLike(input);
  if (!date) return fallback;
  return `${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${WEEKDAYS[date.getDay()]}`;
}

export function formatTime(input: string | Date | null | undefined, fallback = '-'): string {
  if (!input) return fallback;
  if (typeof input === 'string' && /^\d{1,2}:\d{2}(:\d{2})?$/.test(input.trim())) {
    const [hour, minute] = input.trim().split(':');
    return `${pad2(Number(hour))}:${minute}`;
  }
  const date = parseDateLike(input);
  if (!date) return fallback;
  return `${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
}

export function formatDateTime(input: string | number | Date | null | undefined, fallback = '-'): string {
  const date = parseDateLike(input);
  if (!date) return fallback;
  return `${formatDate(date)} ${formatTime(date)}`;
}

export function formatTdxDate(input: string | null | undefined, fallback = '-'): string {
  if (!input) return fallback;
  return formatDate(input.slice(0, 10), fallback);
}
