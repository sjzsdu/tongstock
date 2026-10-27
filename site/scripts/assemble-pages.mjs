#!/usr/bin/env node
/**
 * 组装 GitHub Pages 的发布目录（A 方案：官网占根，应用在 /app/ 下）。
 *
 *   <out>/            ← site/dist（官网 + 文档 + favicon/og）
 *   <out>/app/        ← web/dist（现有 Web UI，构建时 VITE_BASE_PATH=/<repo>/app/）
 *   <out>/404.html    ← 根 404 分流页（见 gen-pages-404.mjs）
 *
 * Pages 每个站点只有一个根 404.html，所以这一步是整条链路的收口点：
 * 少了它，应用深链刷新就是白屏。
 *
 * 用法：
 *   node scripts/assemble-pages.mjs --site-dist=dist --web-dist=../web/dist \
 *     --out=../pages-dist --base=/tongstock/ --app-path=app --routes-file=../web/src/App.tsx
 */

import { cpSync, existsSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync, mkdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { parseRoutes, render404 } from './gen-pages-404.mjs';

function parseArgs(argv) {
  const args = {};
  for (const item of argv) {
    const match = item.match(/^--([\w-]+)=(.*)$/);
    if (match) args[match[1]] = match[2];
  }
  return args;
}

function countFiles(dir) {
  let total = 0;
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    total += statSync(full).isDirectory() ? countFiles(full) : 1;
  }
  return total;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const siteDist = resolve(args['site-dist'] ?? 'dist');
  const webDist = resolve(args['web-dist'] ?? '../web/dist');
  const out = resolve(args.out ?? '../pages-dist');
  const base = args.base ?? '/';
  const appPath = (args['app-path'] ?? 'app').replace(/^\/+|\/+$/g, '');
  const routesFile = resolve(args['routes-file'] ?? '../web/src/App.tsx');

  for (const [label, path] of [
    ['site/dist', siteDist],
    ['web/dist', webDist],
  ]) {
    if (!existsSync(path)) {
      console.error(`[assemble-pages] 缺少 ${label}（${path}），先构建对应产物`);
      process.exit(1);
    }
  }

  rmSync(out, { recursive: true, force: true });
  mkdirSync(out, { recursive: true });
  cpSync(siteDist, out, { recursive: true });
  cpSync(webDist, join(out, appPath), { recursive: true });

  const routes = parseRoutes(readFileSync(routesFile, 'utf8'));
  if (!routes.length) {
    console.error(`[assemble-pages] 未能从 ${routesFile} 解析到旧路由，拒绝生成没有分流表的 404`);
    process.exit(1);
  }
  writeFileSync(join(out, '404.html'), render404({ base, appPath, routes }));

  console.log(
    [
      `[assemble-pages] 输出目录 ${out}`,
      `  ${base}          ← 官网与文档（${countFiles(siteDist)} 个文件）`,
      `  ${base}${appPath}/        ← Web UI（${countFiles(webDist)} 个文件）`,
      `  ${base}404.html    ← 根 404 分流（旧路由 ${routes.length} 条）`,
    ].join('\n'),
  );
}

main();
