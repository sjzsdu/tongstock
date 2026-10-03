const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];

const BEIJING_OFFSET_MINUTES = 8 * 60;

export interface BeijingTimeParts {
  hour: number;
  minute: number;
  text: string;
}

// 与后端 strategy.BeijingNow() 对齐：固定东八区（A 股交易时区，无夏令时），
// 不随浏览器/部署环境的本地时区漂移。用于策略结果区等必须以北京时间展示的场合。
export function beijingNowParts(date: Date = new Date()): BeijingTimeParts {
  const shifted = new Date(date.getTime() + (date.getTimezoneOffset() + BEIJING_OFFSET_MINUTES) * 60_000);
  const hour = shifted.getHours();
  const minute = shifted.getMinutes();
  const pad = (value: number) => String(value).padStart(2, '0');
  return { hour, minute, text: `${pad(hour)}:${pad(minute)}` };
}

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

// 历史分时/逐笔回退探测用的候选日：按本地日历从 from 往前数 count 个工作日
// （默认 12 个，覆盖国庆/春节等最长 8-10 天的连续休市）。不识别法定节假日——
// 假期当天查不到数据属预期，调用方逐个探测直到取到数据或候选用尽。
// 全部用本地日期分量构造，避免 toISOString 的 UTC 偏移把日期错移一天。
export function recentWeekdayDates(count = 12, from: Date = new Date()): Date[] {
  const result: Date[] = [];
  const cursor = new Date(from.getFullYear(), from.getMonth(), from.getDate());
  // 每周只有 5 个工作日，上限给 3 倍冗余即可扫够 count 个
  for (let i = 0; i < count * 3 && result.length < count; i++) {
    cursor.setDate(cursor.getDate() - 1);
    const day = cursor.getDay();
    if (day === 0 || day === 6) continue;
    result.push(new Date(cursor));
  }
  return result;
}

// 后端历史接口的日期参数格式：YYYYMMDD（本地日期分量）
export function formatAPIDate(date: Date): string {
  return `${date.getFullYear()}${pad2(date.getMonth() + 1)}${pad2(date.getDate())}`;
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
