import { describe, expect, it, vi } from 'vitest';
import { APP_PATH_PARAM, applyRestoredAppPath, restoreAppPath } from './appPath';

const BASE = '/tongstock/app/';

describe('restoreAppPath', () => {
  it('没有约定参数时返回 null（正常进入）', () => {
    expect(restoreAppPath('', BASE)).toBeNull();
    expect(restoreAppPath('?tab=kline', BASE)).toBeNull();
  });

  it('还原应用内深链', () => {
    const search = `?${APP_PATH_PARAM}=${encodeURIComponent('stock/000001')}`;
    expect(restoreAppPath(search, BASE)).toBe('/tongstock/app/stock/000001');
  });

  it('保留深链自带的 search 与 hash', () => {
    const search = `?${APP_PATH_PARAM}=${encodeURIComponent('stock/000001?tab=kline#f10')}`;
    expect(restoreAppPath(search, BASE)).toBe('/tongstock/app/stock/000001?tab=kline#f10');
  });

  it('容错前导斜杠，不产生双斜杠', () => {
    const search = `?${APP_PATH_PARAM}=${encodeURIComponent('/stock/000001')}`;
    expect(restoreAppPath(search, BASE)).toBe('/tongstock/app/stock/000001');
  });

  it('base 缺尾斜杠时自动补齐', () => {
    const search = `?${APP_PATH_PARAM}=${encodeURIComponent('methods')}`;
    expect(restoreAppPath(search, '/tongstock/app')).toBe('/tongstock/app/methods');
  });

  it('空值与纯斜杠视为无需还原', () => {
    expect(restoreAppPath(`?${APP_PATH_PARAM}=`, BASE)).toBeNull();
    expect(restoreAppPath(`?${APP_PATH_PARAM}=${encodeURIComponent('/')}`, BASE)).toBeNull();
  });
});

describe('applyRestoredAppPath', () => {
  it('有约定参数时 replaceState 一次并返回新地址', () => {
    const replaceState = vi.fn();
    const url = applyRestoredAppPath({
      location: { search: `?${APP_PATH_PARAM}=${encodeURIComponent('stock/000001')}` },
      history: { replaceState },
    });

    expect(url).toBe('/stock/000001');
    expect(replaceState).toHaveBeenCalledTimes(1);
    expect(replaceState).toHaveBeenCalledWith(null, '', '/stock/000001');
  });

  it('无约定参数时不触碰历史记录', () => {
    const replaceState = vi.fn();
    const url = applyRestoredAppPath({ location: { search: '' }, history: { replaceState } });

    expect(url).toBeNull();
    expect(replaceState).not.toHaveBeenCalled();
  });
});
