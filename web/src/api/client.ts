import type { IndicatorData, ScreenResponse } from '../types/api';

const BASE = '';

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || '请求失败');
  }
  return res.json();
}

export async function getIndicator(code: string, type = 'day'): Promise<IndicatorData> {
  return fetchJSON(`/api/indicator?code=${code}&type=${type}`);
}

export async function getScreen(codes: string, type = 'day', signal?: string): Promise<ScreenResponse> {
  const params = new URLSearchParams({ codes, type });
  if (signal) params.set('signal', signal);
  return fetchJSON(`/api/screen?${params}`);
}

export async function getQuote(code: string): Promise<any> {
  return fetchJSON(`/api/quote?code=${code}`);
}

export async function getKline(code: string, type = 'day'): Promise<any> {
  return fetchJSON(`/api/kline?code=${code}&type=${type}`);
}

export async function getFinance(code: string): Promise<any> {
  return fetchJSON(`/api/finance?code=${code}`);
}

export async function getIndex(code: string, type = 'day'): Promise<any> {
  return fetchJSON(`/api/index?code=${code}&type=${type}`);
}
