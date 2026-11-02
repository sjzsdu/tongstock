import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import PageLoading from './PageLoading';

/**
 * 首页 loading 曾经直接渲染裸 `<Spin />`：它是 inline-block，会贴在内容区左上角，
 * 深色背景下看起来像页面塌了。这里锁住"页面级 loading 必须整体居中"这条契约，
 * 而不是比对整段样式快照。
 */
describe('PageLoading', () => {
  it('把 spinner 与提示文字一起居中', () => {
    const html = renderToStaticMarkup(<PageLoading tip="正在加载今日决策…" />);

    expect(html).toContain('ant-spin');
    expect(html).toContain('正在加载今日决策…');
    // antd v6 的 Flex 用类名表达对齐，实际样式由 cssinjs 注入
    expect(html).toContain('ant-flex-justify-center');
    expect(html).toContain('ant-flex-align-center');
    expect(html).toContain('ant-flex-vertical');
    // 横向撑满内容区，居中才有参照宽度
    expect(html).toMatch(/width:100%/);
    // 默认高度随视口收缩但不塌成一条线
    expect(html).toContain('min-height:max(220px, calc(100vh - 380px))');
    expect(html).toContain('role="status"');
  });

  it('支持自定义高度与尺寸，且不传 tip 时不渲染文字', () => {
    const html = renderToStaticMarkup(<PageLoading minHeight={400} size="small" />);

    expect(html).toContain('min-height:400px');
    expect(html).toContain('ant-spin-sm');
    expect(html).not.toContain('ant-typography');
  });
});
