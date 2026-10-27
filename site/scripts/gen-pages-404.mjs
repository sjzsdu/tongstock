#!/usr/bin/env node
/**
 * 生成 GitHub Pages 的**根 404.html**（全站只有一个，必须同时承担三件事）：
 *
 *   1. 应用深链（`/app/stock/000001`）：静态托管没有这个文件，把应用内路径塞进
 *      `?__app_path=` 跳到应用根，由应用启动时 `history.replaceState` 还原；
 *   2. 旧 SPA 深链（`/stock/000001`，应用搬到 `/app/` 之前的外部链接）：命中旧路由表就
 *      转到同一个还原入口，一次跳转完成，不出现「先白屏再跳」；
 *   3. 真正的 404：保留本站风格的提示页。
 *
 * 旧路由表从 `web/src/App.tsx` 的 `<Route path="...">` 生成，不手写枚举——改了路由，
 * 重跑一次这条命令即可跟上；`check-pages.mjs` 会比对生成结果与源码是否一致。
 *
 * 用法：
 *   node scripts/gen-pages-404.mjs --base=/tongstock/ --app-path=app/ \
 *     --routes-file=../web/src/App.tsx --out=../pages-dist/404.html
 */

import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

/** 与 web/src/lib/appPath.ts 的 APP_PATH_PARAM 必须一致；check-pages.mjs 会校验这一点。 */
export const APP_PATH_PARAM = '__app_path';

/** 解析 `<Route path="...">`，返回可用于 404 分流的旧路由列表（排除站点根与兜底 `*`）。 */
export function parseRoutes(source) {
  const routes = new Set();
  for (const match of source.matchAll(/<Route\s+path="([^"]+)"/g)) {
    const route = match[1];
    if (route === '*' || route === '/') continue; // 兜底与站点根不需要重定向
    routes.add(route);
  }
  return [...routes].sort();
}

/** `/stock/:code/:tab` → `^/stock/[^/]+/[^/]+/?$` */
export function routeToRegexSource(route) {
  const pattern = route
    .split('/')
    .map((segment) => {
      if (!segment) return '';
      if (segment.startsWith(':')) return '[^/]+';
      return segment.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    })
    .join('/');
  return `^${pattern}/?$`;
}

function escapeHtml(text) {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/** 渲染 404.html：页面内容先正常显示，脚本只在需要分流时才 replace。 */
export function render404({ base, appPath, routes }) {
  const normalizedBase = base.endsWith('/') ? base : `${base}/`;
  const normalizedAppPath = appPath.replace(/^\/+|\/+$/g, '');
  const appBase = `${normalizedBase}${normalizedAppPath}/`;
  // 正则以字符串数组注入，运行时 new RegExp：直接写 `/^\/x\/?$/` 这类字面量时，
  // 路由里的 `/` 会提前闭合字面量，产出 "Invalid regular expression flags"。
  const legacySources = routes.map((route) => JSON.stringify(routeToRegexSource(route)));

  return `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>页面不存在 · TongStock</title>
    <meta name="robots" content="noindex" />
    <link rel="icon" href="${escapeHtml(normalizedBase)}favicon.png" />
    <style>
      :root {
        --bg: #0b1220;
        --panel: #121c2e;
        --line: rgba(255, 255, 255, 0.1);
        --text: #e8edf6;
        --muted: #93a1b8;
      }
      body {
        margin: 0;
        min-height: 100vh;
        display: grid;
        place-items: center;
        background: radial-gradient(60% 120% at 50% 0%, rgba(47, 111, 237, 0.18), transparent 70%), var(--bg);
        color: var(--text);
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
        text-align: center;
        padding: 24px;
      }
      .code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 14px; letter-spacing: 0.16em; color: var(--muted); text-transform: uppercase; }
      h1 { font-size: clamp(26px, 4vw, 40px); margin: 14px 0 10px; }
      p { color: var(--muted); margin: 0 auto 28px; max-width: 34em; line-height: 1.7; }
      .row { display: flex; gap: 12px; justify-content: center; flex-wrap: wrap; }
      a { display: inline-flex; align-items: center; padding: 11px 20px; border-radius: 10px; font-size: 14.5px; font-weight: 600; text-decoration: none; }
      a.primary { background: linear-gradient(180deg, #3d7bf5, #2a63dc); color: #fff; }
      a.ghost { border: 1px solid var(--line); color: var(--text); background: var(--panel); }
      noscript .row { margin-top: 8px; }
    </style>
  </head>
  <body>
    <main>
      <div class="code">404 · not found</div>
      <h1>这个页面不存在</h1>
      <p>如果你是想打开控制台的某个页面，稍后会自动跳转过去；也可以手动进入应用或查看文档。</p>
      <noscript>
        <p>当前浏览器禁用了脚本，深链无法自动还原，请从下面的入口重新进入。</p>
      </noscript>
      <div class="row">
        <a class="primary" href="${escapeHtml(appBase)}">进入控制台</a>
        <a class="ghost" href="${escapeHtml(normalizedBase)}docs/">查看文档</a>
      </div>
    </main>

    <script>
      (function () {
        var base = ${JSON.stringify(normalizedBase)};
        var appBase = ${JSON.stringify(appBase)};
        var param = ${JSON.stringify(APP_PATH_PARAM)};
        var legacySources = [${legacySources.join(', ')}];
        var legacy = legacySources.map(function (pattern) { return new RegExp(pattern); });

        var path = window.location.pathname;
        var search = window.location.search || '';
        var hash = window.location.hash || '';

        // 1) 应用深链：静态托管没有对应文件，交给应用还原。
        if (path.indexOf(appBase) === 0 && path.length > appBase.length) {
          var inner = path.slice(appBase.length) + search + hash;
          window.location.replace(appBase + '?' + param + '=' + encodeURIComponent(inner));
          return;
        }

        // 2) 旧 SPA 深链（应用搬到 /app/ 之前的外部链接）：命中旧路由就转到同一入口。
        if (path.indexOf(base) === 0) {
          var legacyPath = '/' + path.slice(base.length);
          for (var i = 0; i < legacy.length; i += 1) {
            if (legacy[i].test(legacyPath)) {
              var target = path.slice(base.length) + search + hash;
              window.location.replace(appBase + '?' + param + '=' + encodeURIComponent(target));
              return;
            }
          }
        }

        // 3) 其余情况就是 404，保持当前页面。
      })();
    </script>
  </body>
</html>
`;
}

function parseArgs(argv) {
  const args = {};
  for (const item of argv) {
    const match = item.match(/^--([\w-]+)=(.*)$/);
    if (match) args[match[1]] = match[2];
  }
  return args;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const base = args.base ?? '/';
  const appPath = args['app-path'] ?? 'app';
  const routesFile = resolve(args['routes-file'] ?? '../web/src/App.tsx');
  const out = resolve(args.out ?? 'dist/404.html');

  const routes = parseRoutes(readFileSync(routesFile, 'utf8'));
  if (!routes.length) {
    console.error(`[gen-pages-404] 未能从 ${routesFile} 解析到任何旧路由，拒绝生成不含分流表的 404`);
    process.exit(1);
  }

  const html = render404({ base, appPath, routes });
  mkdirSync(dirname(out), { recursive: true });
  writeFileSync(out, html);
  console.log(
    `[gen-pages-404] 写入 ${out}（base=${base} app=${appPath}，旧路由 ${routes.length} 条）`,
  );
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(new URL(import.meta.url).pathname)) {
  main();
}
