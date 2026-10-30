import { renderToStaticMarkup } from 'react-dom/server';
import { ConfigProvider } from 'antd';
import { describe, expect, it } from 'vitest';
import MethodMechanismDiagram from './MethodMechanismDiagram';
import { APP_THEME, CARD_SURFACE } from '../../theme/appTheme';

/**
 * 首页首屏的「这套工具怎么思考」图解：颜色必须来自主题色板，并且真的读得清。
 *
 * 这层断言抓的是一类曾经真实发生过的缺陷：深色站点里出现写死的浅色块
 * （`background: #fafafa` + 默认浅色文字 = 白底白字 1.05:1），以及把浅色主题的
 * 预设主色当成深色底上的文字色（紫 #722ed1 在 #111827 上只有 2.56:1）。
 * 所以这里不比对样式快照，而是渲染后按 WCAG 相对亮度算对比度。
 */

function parseColor(value: string) {
  const hex = value.trim().match(/^#([0-9a-f]{3,8})$/i);
  if (hex) {
    const h = hex[1];
    const pair = (i: number) => parseInt(h.length <= 4 ? h[i] + h[i] : h.slice(i * 2, i * 2 + 2), 16);
    return { r: pair(0), g: pair(1), b: pair(2), a: 1 };
  }
  const rgb = value.trim().match(/^rgba?\(([^)]+)\)$/i);
  if (rgb) {
    const parts = rgb[1].split(',').map((p) => Number.parseFloat(p.trim()));
    return { r: parts[0], g: parts[1], b: parts[2], a: parts.length > 3 ? parts[3] : 1 };
  }
  return null;
}

function relativeLuminance({ r, g, b }: { r: number; g: number; b: number }) {
  const channel = (v: number) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

function contrastAgainst(fg: { r: number; g: number; b: number; a: number }, surface: string) {
  const bg = parseColor(surface);
  if (!bg) throw new Error(`测试配置的卡片底色无法解析：${surface}`);
  const blended = {
    r: fg.r * fg.a + bg.r * (1 - fg.a),
    g: fg.g * fg.a + bg.g * (1 - fg.a),
    b: fg.b * fg.a + bg.b * (1 - fg.a),
  };
  const l1 = relativeLuminance(blended);
  const l2 = relativeLuminance(bg);
  const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1];
  return (hi + 0.05) / (lo + 0.05);
}

/** 把渲染结果里的内联样式拆成 { prop, color } 列表 */
function inlineColors(html: string) {
  const out: { prop: string; value: string }[] = [];
  for (const style of html.matchAll(/style="([^"]*)"/g)) {
    for (const declaration of style[1].split(';')) {
      const colon = declaration.indexOf(':');
      if (colon === -1) continue;
      const prop = declaration.slice(0, colon).trim().toLowerCase();
      const value = declaration.slice(colon + 1).trim();
      const colorMatch = value.match(/#[0-9a-f]{3,8}|rgba?\([^)]+\)/i);
      if (colorMatch) out.push({ prop, value: colorMatch[0] });
    }
  }
  return out;
}

function renderDiagram() {
  return renderToStaticMarkup(
    <ConfigProvider theme={APP_THEME}>
      <MethodMechanismDiagram />
    </ConfigProvider>,
  );
}

describe('MethodMechanismDiagram 深色主题适配', () => {
  it('不在深色底上画浅色块', () => {
    const colors = inlineColors(renderDiagram());
    const surfaces = colors.filter((c) => c.prop === 'background' || c.prop === 'background-color');
    // 防呆：扫不到样式说明断言会变成空转，宁可红
    expect(surfaces.length).toBeGreaterThan(0);
    for (const surface of surfaces) {
      const color = parseColor(surface.value);
      expect(color, `无法解析底色 ${surface.value}`).not.toBeNull();
      const luminance = relativeLuminance(color!);
      expect(
        luminance,
        `深色主题里出现了浅色块（${surface.prop}: ${surface.value}，亮度 ${luminance.toFixed(2)}）`,
      ).toBeLessThan(0.5);
    }
  });

  it('所有文字色在深色卡片底上达到 AA（4.5:1）', () => {
    const colors = inlineColors(renderDiagram());
    const texts = colors.filter((c) => c.prop === 'color');
    expect(texts.length).toBeGreaterThan(0);
    for (const text of texts) {
      const color = parseColor(text.value);
      expect(color, `无法解析文字色 ${text.value}`).not.toBeNull();
      const ratio = contrastAgainst(color!, CARD_SURFACE);
      expect(
        ratio,
        `文字色 ${text.value} 在卡片底色 ${CARD_SURFACE} 上只有 ${ratio.toFixed(2)}:1`,
      ).toBeGreaterThanOrEqual(4.5);
    }
  });
});
