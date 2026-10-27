#!/usr/bin/env node
/**
 * Pages 语义验收：把「A 方案单站点」的五条关键 URL 跑成脚本，任一不过就退出非零。
 *
 * 为什么不能只跑 `pnpm build`：静态托管的成败在**服务器语义 + 404 分流 + 应用侧还原**
 * 这条链上，构建通过不代表 `/app/stock/000001` 刷新可用。这里用 pages-serve.mjs
 * 起一个模拟 Pages 的服务器（不存在路径 → 根 404.html + 状态 404），再逐条断言，
 * 并把 404.html 里的分流脚本放进 stub 环境执行，验证它真的算出了正确的跳转目标。
 *
 * 用法：node scripts/check-pages.mjs --root=../pages-dist --base=/tongstock/
 */

import { existsSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { startPagesServer } from './pages-serve.mjs';
import { APP_PATH_PARAM, parseRoutes, routeToRegexSource } from './gen-pages-404.mjs';

function parseArgs(argv) {
  const args = {};
  for (const item of argv) {
    const match = item.match(/^--([\w-]+)=(.*)$/);
    if (match) args[match[1]] = match[2];
  }
  return args;
}

const results = [];

function check(name, fn) {
  try {
    const detail = fn();
    results.push({ name, ok: true, detail });
  } catch (error) {
    results.push({ name, ok: false, detail: error instanceof Error ? error.message : String(error) });
  }
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

/** 从 404.html 抽出内联脚本，在只看 window 的 stub 里执行，返回它请求跳转到的地址。 */
function runBootstrap(html, { pathname, search = '', hash = '' }) {
  const script = html.match(/<script>([\s\S]*?)<\/script>/)?.[1];
  assert(script, '404.html 里没有内联分流脚本');
  const replaced = [];
  const window = { location: { pathname, search, hash, replace: (url) => replaced.push(url) } };
  // eslint-disable-next-line no-new-func
  new Function('window', script)(window);
  return replaced;
}

function appPathFromRedirect(url) {
  const queryStart = url.indexOf('?');
  assert(queryStart >= 0, `跳转地址里没有 query：${url}`);
  const params = new URLSearchParams(url.slice(queryStart + 1));
  const raw = params.get(APP_PATH_PARAM);
  assert(raw !== null, `跳转地址里没有 ${APP_PATH_PARAM}：${url}`);
  return { raw, pathname: url.slice(0, queryStart) };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const root = resolve(args.root ?? '../pages-dist');
  const base = args.base ?? '/';
  const normalizedBase = base.endsWith('/') ? base : `${base}/`;
  const appBase = `${normalizedBase}app/`;

  if (!existsSync(join(root, '404.html'))) {
    console.error(`[check-pages] ${root} 缺少根 404.html，先跑 assemble-pages.mjs`);
    process.exit(1);
  }

  const notFoundHtml = readFileSync(join(root, '404.html'), 'utf8');
  const { server, origin } = await startPagesServer({ root, base: normalizedBase });

  const get = async (path, options = {}) => {
    const response = await fetch(`${origin}${path}`, { redirect: 'manual', ...options });
    return { status: response.status, location: response.headers.get('location'), body: await response.text() };
  };

  try {
    // 1. 官网首页
    const home = await get(normalizedBase);
    check('1. /（官网首页）直出', () => {
      assert(home.status === 200, `状态 ${home.status}`);
      assert(home.body.includes('今天买什么'), '正文里没有 hero 标题');
      return '200 · 含 hero「今天买什么」';
    });

    // 2. 文档深链（带尾斜杠 + 不带尾斜杠两种）
    const docSlash = await get(`${normalizedBase}docs/api/endpoints/`);
    const docBare = await get(`${normalizedBase}docs/api/endpoints`);
    check('2a. /docs/api/endpoints/ 直出', () => {
      assert(docSlash.status === 200, `状态 ${docSlash.status}`);
      assert(docSlash.body.includes('端点参考'), '正文里没有「端点参考」');
      return '200 · 含「端点参考」';
    });
    check('2b. /docs/api/endpoints（无尾斜杠）', () => {
      assert(docBare.status === 301, `期望 301，实际 ${docBare.status}`);
      assert(
        docBare.location === `${normalizedBase}docs/api/endpoints/`,
        `Location 应为 ${normalizedBase}docs/api/endpoints/，实际 ${docBare.location}`,
      );
      return `301 → ${docBare.location}`;
    });

    // 3. 应用首页
    const appHome = await get(appBase);
    check('3. /app/ 应用首页', () => {
      assert(appHome.status === 200, `状态 ${appHome.status}`);
      assert(appHome.body.includes('id="root"'), '不是 SPA 外壳（缺 #root）');
      return '200 · SPA 外壳';
    });

    // 4. 应用深链刷新
    const deep = await get(`${appBase}stock/000001`);
    check('4a. /app/stock/000001 落到根 404（Pages 语义）', () => {
      assert(deep.status === 404, `期望 404（Pages 对不存在的静态文件就是 404），实际 ${deep.status}`);
      assert(deep.body === notFoundHtml, '返回的不是根 404.html');
      return '404 · 返回根 404.html';
    });
    check('4b. 404 脚本把应用深链交给 __app_path', () => {
      const targets = runBootstrap(notFoundHtml, { pathname: `${appBase}stock/000001` });
      assert(targets.length === 1, `期望一次跳转，实际 ${targets.length} 次`);
      const { raw, pathname } = appPathFromRedirect(targets[0]);
      assert(pathname === appBase, `跳转应落在 ${appBase}，实际 ${pathname}`);
      assert(raw === 'stock/000001', `__app_path 应为 stock/000001，实际 ${raw}`);
      return `${targets[0]}（应用侧还原为 ${appBase}${raw}）`;
    });

    // 5. 旧 SPA 深链
    const legacy = await get(`${normalizedBase}stock/000001`);
    check('5a. /stock/000001（旧深链）落到根 404', () => {
      assert(legacy.status === 404, `状态 ${legacy.status}`);
      assert(legacy.body === notFoundHtml, '返回的不是根 404.html');
      return '404 · 返回根 404.html';
    });
    check('5b. 404 脚本把旧深链转到应用还原入口', () => {
      const targets = runBootstrap(notFoundHtml, { pathname: `${normalizedBase}stock/000001` });
      assert(targets.length === 1, `期望一次跳转，实际 ${targets.length} 次`);
      const { raw, pathname } = appPathFromRedirect(targets[0]);
      assert(pathname === appBase, `跳转应落在 ${appBase}，实际 ${pathname}`);
      assert(raw === 'stock/000001', `__app_path 应为 stock/000001，实际 ${raw}`);
      return `${targets[0]}（一次跳转，无白屏）`;
    });

    // 6. 真实的 404（不是深链）
    const miss = await get(`${normalizedBase}no-such-page`);
    check('6. /no-such-page 保持 404 页面', () => {
      assert(miss.status === 404, `状态 ${miss.status}`);
      const targets = runBootstrap(notFoundHtml, { pathname: `${normalizedBase}no-such-page` });
      assert(targets.length === 0, '不该跳转');
      return '404 · 不跳转';
    });

    // 7. 两端参数名 / 路由表不许漂移
    check('7a. 参数名与 web 侧一致', () => {
      const webSource = readFileSync(resolve(import.meta.dirname, '../../web/src/lib/appPath.ts'), 'utf8');
      const match = webSource.match(/APP_PATH_PARAM\s*=\s*'([^']+)'/);
      assert(match, '没在 web/src/lib/appPath.ts 里找到 APP_PATH_PARAM');
      assert(match[1] === APP_PATH_PARAM, `web 侧是 ${match[1]}，站点侧是 ${APP_PATH_PARAM}`);
      return `${APP_PATH_PARAM}（两端一致）`;
    });
    check('7b. 404 内嵌的旧路由表与 App.tsx 同步', () => {
      const routes = parseRoutes(readFileSync(resolve(import.meta.dirname, '../../web/src/App.tsx'), 'utf8'));
      for (const route of routes) {
        const regexSource = routeToRegexSource(route);
        assert(
          notFoundHtml.includes(JSON.stringify(regexSource)),
          `404.html 里缺少路由 ${route}（${regexSource}）`,
        );
      }
      return `${routes.length} 条旧路由全部命中`;
    });
  } finally {
    await new Promise((done) => server.close(done));
  }

  const width = Math.max(...results.map((r) => r.name.length));
  console.log(`\nPages 语义验收（root=${root} base=${normalizedBase}）`);
  for (const result of results) {
    console.log(`  ${result.ok ? 'PASS' : 'FAIL'}  ${result.name.padEnd(width)}  ${result.detail}`);
  }
  const failed = results.filter((r) => !r.ok).length;
  console.log(`\n${results.length - failed}/${results.length} 通过${failed ? `，${failed} 条失败` : ''}\n`);
  process.exit(failed ? 1 : 0);
}

main();
