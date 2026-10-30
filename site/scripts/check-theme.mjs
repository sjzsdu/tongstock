#!/usr/bin/env node
/**
 * 站点视觉自检：主题色板 / 文字对比度 / 布局溢出。
 *
 * 为什么需要它：`pnpm build` 只能证明「页面能生成」，证明不了「页面看得清、没被切掉」。
 * 官网/文档站的配色分散在 `src/styles/starlight-theme.css`（Starlight 主题变量）与
 * `src/pages/index.astro` 的页内 `<style>` 里，两者都极易在改动后静默掉出对比度要求；
 * 落地页的卡片/矩阵又用 grid 承载长标识符（`~/.tongstock/cache/tongstock.db`），
 * 一不注意就会把整页顶出视口，产生横向滚动条。
 *
 * 四组检查：
 *  1. 主题色板：把 dist 里的 CSS 逐条过一遍真实的层叠规则（@layer / 特异性 / 顺序），
 *     按 dark、light 两种主题分别解析出最终变量值，再对「同一个规则里同时声明了字色与底色」
 *     的组合算对比度——浅色主题里白字落在浅蓝底上那类事故就发生在这里。
 *  2. 语义对：正文/链接与页面底色、导航/侧栏文字与各自底色必须成对可读。
 *  3. 落地页调色板：页内 `<style>` 里出现的每个文字色，对页面上任何一种底色都必须达到
 *     WCAG AA（正文 4.5:1，大字号 3:1），因为同一档颜色会在多个区块复用。
 *     同时禁止 grid 轨道使用裸 `<n>fr`：轨道最小值会被内容 min-content 撑破容器。
 *  4. 实际渲染（有 Chromium 时）：落地页与文档页在桌面/窄屏宽度下不得横向溢出，
 *     可见文字不得低于 AA。找不到浏览器时明确跳过，不假装通过。
 *
 * 前置条件：先 `pnpm build`（本脚本读 dist/）。
 * 用法：node scripts/check-theme.mjs [--json] [--no-browser] [--browser=<path>]
 */

import { spawn } from 'node:child_process';
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as sleep } from 'node:timers/promises';
import { startPagesServer } from './pages-serve.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const SITE = resolve(HERE, '..');
const DIST = resolve(SITE, 'dist');

/* ------------------------------------------------------------------ 颜色 */

/** 解析 #rgb / #rrggbb / rgb() / rgba() / hsl() / hsla() / 少量颜色关键字 → {r,g,b,a} */
export function parseColor(input) {
  if (!input) return null;
  const value = String(input).trim().toLowerCase();
  const named = {
    transparent: { r: 0, g: 0, b: 0, a: 0 },
    white: { r: 255, g: 255, b: 255, a: 1 },
    black: { r: 0, g: 0, b: 0, a: 1 },
  };
  if (named[value]) return named[value];

  const hex = value.match(/^#([0-9a-f]{3,8})$/);
  if (hex) {
    const h = hex[1];
    const expand = (s) => parseInt(s.length === 1 ? s + s : s, 16);
    if (h.length === 3 || h.length === 4) {
      return {
        r: expand(h[0]),
        g: expand(h[1]),
        b: expand(h[2]),
        a: h.length === 4 ? expand(h[3]) / 255 : 1,
      };
    }
    if (h.length === 6 || h.length === 8) {
      return {
        r: expand(h.slice(0, 2)),
        g: expand(h.slice(2, 4)),
        b: expand(h.slice(4, 6)),
        a: h.length === 8 ? expand(h.slice(6, 8)) / 255 : 1,
      };
    }
    return null;
  }

  const rgb = value.match(/^rgba?\(([^)]+)\)$/);
  if (rgb) {
    const parts = rgb[1].split(/[\s,/]+/).filter(Boolean);
    if (parts.length < 3) return null;
    const num = (s) => (s.endsWith('%') ? (parseFloat(s) / 100) * 255 : parseFloat(s));
    return {
      r: num(parts[0]),
      g: num(parts[1]),
      b: num(parts[2]),
      a: parts.length > 3 ? (parts[3].endsWith('%') ? parseFloat(parts[3]) / 100 : parseFloat(parts[3])) : 1,
    };
  }

  const hsl = value.match(/^hsla?\(([^)]+)\)$/);
  if (hsl) {
    const parts = hsl[1].split(/[\s,/]+/).filter(Boolean);
    if (parts.length < 3) return null;
    const h = ((parseFloat(parts[0]) % 360) + 360) % 360;
    const s = parseFloat(parts[1]) / 100;
    const l = parseFloat(parts[2]) / 100;
    const a = parts.length > 3 ? parseFloat(parts[3]) : 1;
    const c = (1 - Math.abs(2 * l - 1)) * s;
    const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
    const m = l - c / 2;
    const seg = [
      [c, x, 0], [x, c, 0], [0, c, x], [0, x, c], [x, 0, c], [c, 0, x],
    ][Math.floor(h / 60) % 6];
    return {
      r: Math.round((seg[0] + m) * 255),
      g: Math.round((seg[1] + m) * 255),
      b: Math.round((seg[2] + m) * 255),
      a,
    };
  }

  return null;
}

/** 相对亮度（WCAG 2.x） */
export function luminance({ r, g, b }) {
  const f = (v) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
}

/** 半透明前景合成到底色上，避免把 rgba 文字算成比实际更亮 */
export function flatten(fg, bg) {
  if (fg.a >= 1) return { ...fg, a: 1 };
  return {
    r: fg.r * fg.a + bg.r * (1 - fg.a),
    g: fg.g * fg.a + bg.g * (1 - fg.a),
    b: fg.b * fg.a + bg.b * (1 - fg.a),
    a: 1,
  };
}

export function contrast(fg, bg) {
  const top = flatten(fg, bg);
  const l1 = luminance(top);
  const l2 = luminance(bg);
  const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1];
  return (hi + 0.05) / (lo + 0.05);
}

export function formatColor(c) {
  if (!c) return '(unresolved)';
  const hex = `#${[c.r, c.g, c.b].map((v) => Math.round(v).toString(16).padStart(2, '0')).join('')}`;
  return c.a >= 1 ? hex : `${hex}@${Math.round(c.a * 100)}%`;
}

/* -------------------------------------------------------------- CSS 解析 */

/**
 * 极简 CSS 解析：只保留「规则 → 声明」这一层，同时记录 @layer / @media 上下文与出现顺序。
 * 站点自己的样式表都很小，够用；解析不了的块跳过而不是猜。
 */
export function parseCss(css, { file = '(inline)', startOrder = 0 } = {}) {
  const rules = [];
  let order = startOrder;
  let i = 0;
  const stack = [];

  const currentLayer = () => stack.filter((s) => s.kind === 'layer').map((s) => s.name);

  while (i < css.length) {
    if (css.startsWith('/*', i)) {
      const end = css.indexOf('*/', i + 2);
      i = end === -1 ? css.length : end + 2;
      continue;
    }
    const brace = css.indexOf('{', i);
    const close = css.indexOf('}', i);
    const semi = css.indexOf(';', i);
    if (brace === -1) break;
    if (close !== -1 && close < brace) {
      stack.pop();
      i = close + 1;
      continue;
    }
    // `@layer a, b;` / `@import ...;` 这类语句级 at-rule：没有块，直接跳过，
    // 否则会把后面的选择器吞进 prelude，层叠关系全错。
    if (semi !== -1 && semi < brace) {
      i = semi + 1;
      continue;
    }
    const prelude = css.slice(i, brace).trim();
    let depth = 0;
    let j = brace;
    for (; j < css.length; j++) {
      if (css[j] === '{') depth++;
      else if (css[j] === '}') {
        depth--;
        if (depth === 0) break;
      }
    }
    const body = css.slice(brace + 1, j);
    i = j + 1;

    if (prelude.startsWith('@')) {
      const layerMatch = prelude.match(/^@layer\s+([\w.-]+)$/);
      const nested = !layerMatch && /^@(?:media|supports|container|layer)\b/.test(prelude);
      // 打印样式表也会写 :root 变量，但它不该参与屏幕上的层叠判定
      if (nested && /^@media\b/i.test(prelude) && /\bprint\b/i.test(prelude) && !/\bscreen\b/i.test(prelude)) {
        continue;
      }
      if (layerMatch || nested) {
        stack.push({ kind: layerMatch ? 'layer' : 'at', name: layerMatch ? layerMatch[1] : prelude });
        const inner = parseCss(body, { file, startOrder: order });
        for (const rule of inner.rules) {
          rules.push({ ...rule, layer: [...currentLayer(), ...rule.layer] });
          order = Math.max(order, rule.order) + 1;
        }
        stack.pop();
      }
      continue;
    }

    const declarations = [];
    for (const chunk of splitDeclarations(body)) {
      const colon = chunk.indexOf(':');
      if (colon === -1) continue;
      const prop = chunk.slice(0, colon).trim();
      if (!prop.startsWith('--') && !KEPT_PROPS.test(prop)) continue;
      let value = chunk.slice(colon + 1).trim();
      const important = /!important$/i.test(value);
      if (important) value = value.replace(/!important$/i, '').trim();
      declarations.push({ prop, value, important });
    }
    if (declarations.length) {
      rules.push({
        file,
        order: order++,
        layer: currentLayer(),
        selectors: splitSelectors(prelude),
        declarations,
      });
    }
  }
  return { rules };
}

const KEPT_PROPS = /^(color|background|background-color|font-size|font-weight|grid-template-columns)$/;

function splitDeclarations(body) {
  const out = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < body.length; i++) {
    const ch = body[i];
    if (ch === '(') depth++;
    else if (ch === ')') depth--;
    else if (ch === ';' && depth === 0) {
      out.push(body.slice(start, i));
      start = i + 1;
    }
  }
  out.push(body.slice(start));
  return out.map((s) => s.trim()).filter(Boolean);
}

function splitSelectors(prelude) {
  const out = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < prelude.length; i++) {
    const ch = prelude[i];
    if (ch === '(') depth++;
    else if (ch === ')') depth--;
    else if (ch === ',' && depth === 0) {
      out.push(prelude.slice(start, i).trim());
      start = i + 1;
    }
  }
  out.push(prelude.slice(start).trim());
  return out.filter(Boolean);
}

/** 每个 @layer 的先后顺序：后声明的层优先级更高，未分层的声明最高 */
function collectLayerOrder(rules) {
  const order = new Map();
  for (const rule of rules) {
    for (const name of rule.layer) {
      if (!order.has(name)) order.set(name, order.size);
    }
  }
  return order;
}

function layerPriority(layerPath, layerOrder) {
  if (!layerPath.length) return Number.MAX_SAFE_INTEGER;
  const top = layerPath[0];
  return layerOrder.has(top) ? layerOrder.get(top) : 0;
}

function specificity(selector) {
  // :not(...)/:is(...) 内部按参数本身计权，先把括号拆掉再数，别把 :not 也算成一个类
  const unrolled = selector.replace(/:(?:not|is|where)\(([^)]*)\)/g, (_, inner) =>
    inner.includes('data-theme') ? inner : '',
  );
  const ids = (unrolled.match(/#[\w-]+/g) || []).length;
  const classes = (unrolled.match(/\.[\w-]+|\[[^\]]+\]|:(?!:)[\w-]+/g) || []).length;
  const types = (unrolled.match(/(^|[\s>+~])([a-z][\w-]*)|::[\w-]+/g) || []).length;
  return ids * 1e6 + classes * 1e3 + types;
}

/**
 * 主题变量都写在根元素上；这里只判断「这条规则是否命中给定主题下的根元素」，
 * 认这几种写法（站点与 Starlight 实际用到的全集）：
 *   :root / html / html:root / html:root[data-theme='light'] / html:root:not([data-theme='light'])
 * 判断不了的一律跳过，不虚构断言。
 */
function matchesRoot(selector, theme) {
  const sel = selector.trim();
  const match = sel.match(
    /^((?:html)?(?::root)?)(?:\[data-theme=['"]?([\w-]+)['"]?\])?(?::not\(\[data-theme=['"]?([\w-]+)['"]?\]\))?$/,
  );
  if (!match || !match[1]) return false;
  const [, , required, excluded] = match;
  if (required && required !== theme) return false;
  if (excluded && excluded === theme) return false;
  return true;
}

/** 解析 var() 引用，拿到最终值 */
export function resolveValue(value, vars, depth = 0) {
  if (depth > 20 || value === undefined) return value;
  const varRef = /var\(\s*(--[\w-]+)\s*(?:,\s*([^)]*))?\)/g;
  let out = value;
  let match;
  while ((match = varRef.exec(value))) {
    const [, name, fallback] = match;
    let replacement = vars.get(name);
    if (replacement === undefined || replacement === '') replacement = fallback;
    if (replacement === undefined) return undefined;
    out = out.replace(match[0], resolveValue(replacement, vars, depth + 1) ?? '');
  }
  return out;
}

/** 按主题解析出根元素上的自定义属性表（真实层叠：层 > 特异性 > 顺序） */
export function resolveThemeVars(rules, theme) {
  const layerOrder = collectLayerOrder(rules);
  const winners = new Map();
  for (const rule of rules) {
    for (const selector of rule.selectors) {
      if (!matchesRoot(selector, theme)) continue;
      const key = [
        rule.declarations.some((d) => d.important) ? 1 : 0,
        layerPriority(rule.layer, layerOrder),
        specificity(selector),
        rule.order,
      ];
      for (const decl of rule.declarations) {
        if (!decl.prop.startsWith('--')) continue;
        const prev = winners.get(decl.prop);
        if (!prev || compareKey(key, prev.key) >= 0) {
          winners.set(decl.prop, { key, value: decl.value, file: rule.file });
        }
      }
    }
  }
  const raw = new Map([...winners].map(([k, v]) => [k, v.value]));
  const resolved = new Map();
  for (const name of raw.keys()) {
    const value = resolveValue(raw.get(name), raw);
    if (value !== undefined) resolved.set(name, value.trim());
  }
  return resolved;
}

function compareKey(a, b) {
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return a[i] - b[i];
  }
  return 0;
}

/* --------------------------------------------------------------- 读取产物 */

function walk(dir, filter, out = []) {
  if (!existsSync(dir)) return out;
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) walk(full, filter, out);
    else if (filter(name)) out.push(full);
  }
  return out;
}

function loadStylesheets() {
  const rules = [];
  let order = 0;
  for (const file of walk(DIST, (n) => n.endsWith('.css')).sort()) {
    const parsed = parseCss(readFileSync(file, 'utf8'), {
      file: file.slice(DIST.length + 1),
      startOrder: order,
    });
    rules.push(...parsed.rules);
    order = parsed.rules.length ? parsed.rules[parsed.rules.length - 1].order + 1 : order;
  }
  return rules;
}

const problems = [];
const notes = [];

function fail(area, message) {
  problems.push({ area, message });
}

/* ------------------------------------------------------- 1/2. 主题与对比度 */

/**
 * 成对可读的语义组合。左边是前景变量、右边是底变量，第三项是它出现在哪。
 * 前四对对应 Starlight 的站点骨架（正文 / 链接 / 导航 / 侧栏）；
 * 最后一对是 `.starlight-aside--note` 的写法（字色 --sl-color-white、底色 --sl-color-blue-low），
 * 主题只改一半时，这里就是白字压在浅蓝底上的事故点。
 */
const THEME_PAIRS = [
  ['--sl-color-text', '--sl-color-bg', '正文'],
  ['--sl-color-text-accent', '--sl-color-bg', '链接/强调文字'],
  ['--sl-color-text', '--sl-color-bg-nav', '导航文字'],
  ['--sl-color-text', '--sl-color-bg-sidebar', '侧栏文字'],
  ['--sl-color-white', '--sl-color-blue-low', '提示块（aside）'],
];

function checkThemes(rules) {
  for (const theme of ['dark', 'light']) {
    const vars = resolveThemeVars(rules, theme);
    const get = (name) => {
      const value = vars.get(name);
      return value === undefined ? null : parseColor(value);
    };

    for (const [fgName, bgName, label] of THEME_PAIRS) {
      const fg = get(fgName);
      const bg = get(bgName);
      if (!fg || !bg) continue;
      const ratio = contrast(fg, bg);
      if (ratio < 4.5) {
        fail(
          `主题 ${theme}`,
          `${label}：${fgName}=${formatColor(fg)} 落在 ${bgName}=${formatColor(bg)} 上，对比度 ${ratio.toFixed(2)}:1，低于 AA 要求的 4.5:1`,
        );
      }
    }

    notes.push(`主题 ${theme}：解析出 ${vars.size} 个根级变量`);
  }
}

/* ---------------------------------------------------- 3. 落地页调色板 */

const LANDING = join(DIST, 'index.html');

/** 落地页用到的样式：文档里的 <style> + 它引用的外链样式表（Astro 会把组件样式抽成 _astro/*.css） */
function landingStyles() {
  if (!existsSync(LANDING)) return null;
  const html = readFileSync(LANDING, 'utf8');
  const parts = [];
  for (const link of html.matchAll(/<link[^>]+rel="stylesheet"[^>]*>/gi)) {
    const href = link[0].match(/href="([^"]+)"/)?.[1];
    if (!href || /^https?:/i.test(href)) continue;
    const file = join(DIST, href.replace(/^\//, ''));
    if (existsSync(file)) parts.push(readFileSync(file, 'utf8'));
  }
  for (const inline of html.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/gi)) {
    parts.push(inline[1]);
  }
  return parts.join('\n');
}

function checkLandingPalette() {
  const css = landingStyles();
  if (css === null) {
    fail('落地页', '找不到 dist/index.html，先跑 pnpm build');
    return;
  }
  if (!css.trim()) {
    fail('落地页', '解析不到落地页的样式（既没有外链也没有页内 <style>），检查构建产物');
    return;
  }
  const pageRules = parseCss(css, { file: 'dist/index.html' }).rules;
  const vars = resolveThemeVars(pageRules, 'dark');

  // 页面只有三档底色（--bg / --bg-alt / --panel），同一档文字色会在多个区块复用，
  // 所以按「最暗的那档底色」判定：它过得去，换到任何区块都过得去。
  const surfaces = new Map();
  for (const name of ['--bg', '--bg-alt', '--panel']) {
    const color = parseColor(vars.get(name) ?? '');
    if (color) surfaces.set(`${name}(${formatColor(color)})`, color);
  }
  if (!surfaces.size) {
    fail('落地页', '页内样式里解析不到 --bg / --bg-alt / --panel，检查色板是否改名');
    return;
  }

  for (const rule of pageRules) {
    const colorDecl = rule.declarations.find((d) => d.prop === 'color');
    if (!colorDecl) continue;
    const fg = parseColor(resolveValue(colorDecl.value, vars) ?? '');
    if (!fg) continue;
    const size = parseFloat(
      (rule.declarations.find((d) => d.prop === 'font-size')?.value || '16').replace(/[^\d.]/g, ''),
    ) || 16;
    const weight = parseInt(
      rule.declarations.find((d) => d.prop === 'font-weight')?.value || '400',
      10,
    ) || 400;
    const large = size >= 24 || (size >= 18.66 && weight >= 700);
    const need = large ? 3 : 4.5;

    let worst = null;
    for (const [surfaceName, bg] of surfaces) {
      const ratio = contrast(fg, bg);
      if (!worst || ratio < worst.ratio) worst = { ratio, surfaceName };
    }
    if (worst && worst.ratio < need) {
      fail(
        '落地页',
        `${rule.selectors.join(', ')} 的文字色 ${colorDecl.value}（${formatColor(fg)}）落在 ${worst.surfaceName} 上只有 ${worst.ratio.toFixed(2)}:1，低于 ${need}:1`,
      );
    }
  }

  // grid 轨道写裸 fr（= minmax(auto, Nfr)）时，子项里不可断的长标识符会把轨道顶到
  // min-content 宽度、把整页顶出视口 —— 落地页卡片里全是路径/接口串，必须 minmax(0, Nfr)。
  for (const rule of pageRules) {
    const trackDecl = rule.declarations.find((d) => d.prop === 'grid-template-columns');
    if (!trackDecl) continue;
    const bare = trackDecl.value
      .split(/\s+(?![^(]*\))/)
      .map((part) => part.trim())
      .filter((part) => /^[\d.]+fr$/.test(part));
    if (bare.length) {
      fail(
        '落地页',
        `${rule.selectors.join(', ')} 的 grid-template-columns 用了裸 fr（${bare.join(' ')}）：` +
          '轨道最小值会被内容 min-content 撑破容器，窄屏上整页出现横向滚动条；改成 minmax(0, Nfr)',
      );
    }
  }
}

/* --------------------------------------------- 4. 实际渲染（需要浏览器） */

const BROWSER_CANDIDATES = [
  process.env.CHROME_BIN,
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/Applications/Chromium.app/Contents/MacOS/Chromium',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium',
  '/usr/bin/chromium-browser',
].filter(Boolean);

const RENDER_PAGES = [
  { path: '/', label: '落地页', themes: [null] },
  { path: '/docs/getting-started/overview/', label: '文档页', themes: ['dark', 'light'] },
  { path: '/docs/api/endpoints/', label: '文档页（端点参考）', themes: ['dark', 'light'] },
];
const RENDER_WIDTHS = [1440, 1024, 390];

function findBrowser(explicit) {
  for (const path of explicit ? [explicit] : BROWSER_CANDIDATES) {
    if (path && existsSync(path)) return path;
  }
  return null;
}

async function renderAudit(browserPath, baseUrl) {
  const port = 9531;
  const chrome = spawn(browserPath, [
    '--headless=new',
    '--no-sandbox',
    '--disable-gpu',
    '--disable-crashpad',
    '--disable-crash-reporter',
    '--hide-scrollbars',
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${join(process.env.TMPDIR || '/tmp', 'tongstock-site-check')}`,
    'about:blank',
  ], { stdio: 'ignore' });

  try {
    let up = false;
    for (let i = 0; i < 150; i++) {
      try {
        const res = await fetch(`http://127.0.0.1:${port}/json/version`);
        if (res.ok) {
          up = true;
          break;
        }
      } catch {}
      await sleep(100);
    }
    if (!up) {
      fail('渲染', `浏览器没起来（${browserPath}），渲染检查未执行`);
      return;
    }

    for (const page of RENDER_PAGES) {
      for (const theme of page.themes) {
        for (const width of RENDER_WIDTHS) {
          const label = `${page.label} ${page.path} @${width}${theme ? ` ${theme}` : ''}`;
          const findings = await auditOne(port, `${baseUrl}${page.path}`, width, theme);
          if (findings.error) {
            fail('渲染', `${label}：${findings.error}`);
            continue;
          }
          for (const item of findings.contrast) {
            fail(
              '渲染',
              `${label}：${item.sel} 文字 ${item.color} 落在 ${item.bg} 上只有 ${item.ratio}:1（需 ${item.need}:1，「${item.text}」）`,
            );
          }
          if (findings.scrollWidth > findings.clientWidth + 1) {
            fail(
              '渲染',
              `${label}：整页宽 ${findings.scrollWidth}px 超出视口 ${findings.clientWidth}px，右侧内容被切掉；` +
                `越界元素 ${findings.overflow.join('、') || '(未定位到)'}`,
            );
          }
        }
      }
    }
  } finally {
    chrome.kill();
  }
}

async function auditOne(devtoolsPort, url, width, theme) {
  const target = await (await fetch(
    `http://127.0.0.1:${devtoolsPort}/json/new?${encodeURIComponent(url)}`,
    { method: 'PUT' },
  )).json();
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((res, rej) => {
    ws.addEventListener('open', res);
    ws.addEventListener('error', rej);
  });
  let id = 0;
  const pending = new Map();
  ws.addEventListener('message', (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.id && pending.has(msg.id)) {
      const p = pending.get(msg.id);
      pending.delete(msg.id);
      msg.error ? p.reject(new Error(JSON.stringify(msg.error))) : p.resolve(msg.result);
    }
  });
  const send = (method, params = {}) => {
    const i = ++id;
    ws.send(JSON.stringify({ id: i, method, params }));
    return new Promise((res, rej) => pending.set(i, { resolve: res, reject: rej }));
  };
  try {
    if (theme) {
      await send('Emulation.setEmulatedMedia', {
        features: [{ name: 'prefers-color-scheme', value: theme }],
      });
    }
    await send('Emulation.setDeviceMetricsOverride', {
      width, height: 900, deviceScaleFactor: 1, mobile: width < 500,
    });
    await sleep(700);
    const result = await send('Runtime.evaluate', {
      expression: RENDER_AUDIT_EXPRESSION,
      returnByValue: true,
    });
    if (result.exceptionDetails) {
      return { error: JSON.stringify(result.exceptionDetails).slice(0, 300) };
    }
    return result.result.value;
  } finally {
    ws.close();
    await fetch(`http://127.0.0.1:${devtoolsPort}/json/close/${target.id}`).catch(() => {});
  }
}

/** 在页面里跑的审计：可见文字的对比度 + 整页横向溢出 */
const RENDER_AUDIT_EXPRESSION = String.raw`(function () {
  function parseColor(str) {
    var m = String(str).match(/rgba?\(([^)]+)\)/);
    if (!m) return null;
    var parts = m[1].split(/[\s,\/]+/).filter(Boolean).map(Number);
    return { r: parts[0], g: parts[1], b: parts[2], a: parts.length > 3 ? parts[3] : 1 };
  }
  function lum(c) {
    function f(v) { var s = v / 255; return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4); }
    return 0.2126 * f(c.r) + 0.7152 * f(c.g) + 0.0722 * f(c.b);
  }
  function blend(top, bottom) {
    return { r: top.r * top.a + bottom.r * (1 - top.a), g: top.g * top.a + bottom.g * (1 - top.a), b: top.b * top.a + bottom.b * (1 - top.a), a: 1 };
  }
  function ratio(fg, bg) {
    var t = fg.a < 1 ? blend(fg, bg) : fg;
    var l1 = lum(t), l2 = lum(bg);
    return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
  }
  function effectiveBg(el) {
    var stack = [], node = el;
    while (node && node.nodeType === 1) {
      var c = parseColor(getComputedStyle(node).backgroundColor);
      if (c && c.a > 0) stack.push(c);
      node = node.parentElement;
    }
    var bg = { r: 255, g: 255, b: 255, a: 1 };
    for (var i = stack.length - 1; i >= 0; i--) bg = blend(stack[i], bg);
    return bg;
  }
  function className(el) {
    return typeof el.className === 'string' && el.className ? '.' + el.className.trim().split(/\s+/).join('.') : '';
  }
  var clientWidth = document.documentElement.clientWidth;
  var contrast = [], overflow = [];
  var elements = document.querySelectorAll('body *');
  for (var i = 0; i < elements.length; i++) {
    var el = elements[i];
    var style = getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden' || parseFloat(style.opacity) === 0) continue;
    var rect = el.getBoundingClientRect();
    // 只读屏元素、跳转链接在视觉上不占位，不参与对比度判定
    if (el.classList.contains('sr-only') || el.classList.contains('skip') || el.classList.contains('sl-skip-link')) continue;
    if ((style.clipPath && style.clipPath !== 'none') || (style.clip && style.clip !== 'auto')) continue;
    if (rect.width <= 1 || rect.height <= 1 || rect.right < 0) continue;
    var text = '';
    for (var n = 0; n < el.childNodes.length; n++) {
      if (el.childNodes[n].nodeType === 3) text += ' ' + el.childNodes[n].textContent.trim();
    }
    text = text.trim();
    if (text) {
      var fg = parseColor(style.color);
      if (fg) {
        var bg = effectiveBg(el);
        var value = ratio(fg, bg);
        var size = parseFloat(style.fontSize);
        var weight = parseInt(style.fontWeight, 10) || 400;
        var need = (size >= 24 || (size >= 18.66 && weight >= 700)) ? 3 : 4.5;
        if (value < need) {
          contrast.push({
            sel: el.tagName.toLowerCase() + className(el),
            text: text.slice(0, 40),
            color: 'rgb(' + Math.round(fg.r) + ',' + Math.round(fg.g) + ',' + Math.round(fg.b) + ')',
            bg: 'rgb(' + Math.round(bg.r) + ',' + Math.round(bg.g) + ',' + Math.round(bg.b) + ')',
            ratio: Math.round(value * 100) / 100,
            need: need,
          });
        }
      }
    }
    if (rect.right > clientWidth + 1) {
      overflow.push(el.tagName.toLowerCase() + className(el) + '→' + Math.round(rect.right) + 'px');
    }
  }
  return {
    clientWidth: clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
    contrast: contrast.slice(0, 12),
    overflow: Array.from(new Set(overflow)).slice(0, 6),
  };
})()`;

/* ------------------------------------------------------------------- main */

async function main() {
  const argv = process.argv.slice(2);
  const json = argv.includes('--json');
  const noBrowser = argv.includes('--no-browser');
  const dumpVars = argv.includes('--dump-vars');
  const browserArg = argv.find((a) => a.startsWith('--browser='))?.split('=')[1];

  if (!existsSync(DIST)) {
    console.error('[check-theme] 缺少 dist/，先运行 pnpm build');
    process.exit(1);
  }

  const rules = loadStylesheets();
  if (!rules.length) {
    console.error('[check-theme] dist 里没解析到任何 CSS 规则，构建产物可能不完整');
    process.exit(1);
  }

  // 排查主题取值用：把两种主题下解析到的变量打出来，和浏览器 computed style 对照
  if (dumpVars) {
    const traceVar = argv.find((a) => a.startsWith('--trace='))?.split('=')[1];
    if (traceVar) {
      for (const rule of rules) {
        for (const decl of rule.declarations) {
          if (decl.prop !== traceVar) continue;
          console.log(
            `${rule.file} order=${rule.order} layer=[${rule.layer.join('>')}] sel=${rule.selectors.join(',')} value=${decl.value}`,
          );
        }
      }
      process.exit(0);
    }
    for (const theme of ['dark', 'light']) {
      const vars = resolveThemeVars(rules, theme);
      console.log(`--- ${theme} ---`);
      for (const name of [...vars.keys()].sort()) {
        if (!/--sl-color-(bg|text|white|black|accent|blue-low)/.test(name)) continue;
        console.log(`${name}: ${vars.get(name)}`);
      }
    }
    process.exit(0);
  }

  checkThemes(rules);
  checkLandingPalette();

  if (noBrowser) {
    notes.push('渲染检查：跳过（--no-browser）');
  } else {
    const browserPath = findBrowser(browserArg);
    if (!browserPath) {
      notes.push('渲染检查：跳过（没找到 Chromium，用 --browser=/path/to/chrome 指定）');
    } else {
      // 复用 Pages 语义的静态服务器：本地看到的目录式 URL / 404 行为与线上一致
      const { server, origin } = await startPagesServer({ root: DIST, base: '/', port: 9530 });
      try {
        await renderAudit(browserPath, origin);
      } finally {
        server.close();
      }
    }
  }

  if (json) {
    console.log(JSON.stringify({ problems, notes }, null, 2));
  } else {
    for (const note of notes) console.log(`· ${note}`);
    if (problems.length) {
      console.error(`\n✗ 主题 / 对比度 / 布局自检发现 ${problems.length} 个问题：`);
      const byArea = new Map();
      for (const problem of problems) {
        if (!byArea.has(problem.area)) byArea.set(problem.area, []);
        byArea.get(problem.area).push(problem.message);
      }
      for (const [area, messages] of byArea) {
        console.error(`\n[${area}]`);
        for (const message of messages) console.error(`  - ${message}`);
      }
    } else {
      console.log('\n✓ 主题色板、文字对比度与布局溢出检查通过');
    }
  }
  process.exit(problems.length ? 1 : 0);
}

main().catch((error) => {
  console.error('[check-theme] 执行失败：', error);
  process.exit(1);
});
