#!/usr/bin/env node
/**
 * 一个**模拟 GitHub Pages 语义**的最小静态服务器，专供本地验收使用。
 *
 * 语义要点（与 Pages 对齐，否则本地测过线上照样坏）：
 *   - `/x/`            → `x/index.html`
 *   - `/x`（目录存在）  → 301 到 `/x/`
 *   - `/x`（文件存在）  → 直接返回
 *   - 其它             → 返回**根** `404.html`，HTTP 状态 404（不是站点某处的 404）
 *
 * 用法（独立起服务）：
 *   node scripts/pages-serve.mjs --root=../pages-dist --base=/tongstock/ --port=8788
 * 也可被 check-pages.mjs 以函数形式复用。
 */

import { createServer } from 'node:http';
import { existsSync, readFileSync, statSync } from 'node:fs';
import { extname, join, normalize, resolve } from 'node:path';

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.webp': 'image/webp',
  '.ico': 'image/x-icon',
  '.txt': 'text/plain; charset=utf-8',
  '.xml': 'application/xml; charset=utf-8',
  '.woff2': 'font/woff2',
  '.map': 'application/json; charset=utf-8',
  '.wasm': 'application/wasm',
};

/** 把 URL 路径安全地映射到 root 内的文件；越界返回 null。 */
function resolveInside(root, urlPath) {
  const decoded = decodeURIComponent(urlPath.split('?')[0].split('#')[0]);
  const normalized = normalize(decoded).replace(/^(\.\.[/\\])+/, '');
  const full = join(root, normalized);
  return full.startsWith(root) ? full : null;
}

/**
 * @param {{ root: string, base: string }} options base 形如 `/tongstock/`
 */
export function createPagesHandler({ root, base }) {
  const rootAbs = resolve(root);
  const normalizedBase = base.endsWith('/') ? base : `${base}/`;
  const notFoundFile = join(rootAbs, '404.html');

  return (req, res) => {
    const url = req.url || '/';
    const [pathname] = url.split('?');

    if (!pathname.startsWith(normalizedBase)) {
      res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end('outside base');
      return;
    }

    const rel = pathname.slice(normalizedBase.length);
    const target = resolveInside(rootAbs, rel);
    if (!target) {
      res.writeHead(400, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end('bad path');
      return;
    }

    const send = (file, status = 200) => {
      const type = MIME[extname(file)] ?? 'application/octet-stream';
      res.writeHead(status, { 'Content-Type': type });
      res.end(readFileSync(file));
    };

    // `/x/` → x/index.html
    if (rel === '' || rel.endsWith('/')) {
      const index = join(target, 'index.html');
      if (existsSync(index)) return send(index);
      return send(notFoundFile, 404);
    }

    if (existsSync(target) && statSync(target).isFile()) return send(target);

    // `/x` 且 x/ 目录存在 → 301 到 `/x/`（Pages 对目录就是这么处理的）
    if (existsSync(target) && statSync(target).isDirectory()) {
      const location = `${pathname}/`.replace(/\/{2,}/g, '/');
      res.writeHead(301, { Location: location });
      res.end();
      return;
    }

    // `/x.html`
    if (existsSync(`${target}.html`)) return send(`${target}.html`);

    // 其它一律落到根 404.html，状态 404
    return send(notFoundFile, 404);
  };
}

/** 起一个监听随机（或指定）端口的服务器，返回 { server, port, url }。 */
export function startPagesServer({ root, base, port = 0 }) {
  const handler = createPagesHandler({ root, base });
  const server = createServer(handler);
  return new Promise((resolvePromise, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => {
      const address = server.address();
      const actualPort = typeof address === 'object' && address ? address.port : port;
      resolvePromise({ server, port: actualPort, origin: `http://127.0.0.1:${actualPort}` });
    });
  });
}

function parseArgs(argv) {
  const args = {};
  for (const item of argv) {
    const match = item.match(/^--([\w-]+)=(.*)$/);
    if (match) args[match[1]] = match[2];
  }
  return args;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const root = resolve(args.root ?? '../pages-dist');
  const base = args.base ?? '/';
  const port = Number(args.port ?? 8788);

  if (!existsSync(join(root, 'index.html'))) {
    console.error(`[pages-serve] ${root} 不是组装好的 Pages 目录（缺 index.html）`);
    process.exit(1);
  }

  const { origin } = await startPagesServer({ root, base, port });
  console.log(`[pages-serve] ${origin}${base} → ${root}（不存在路径返回根 404.html，状态 404）`);
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(new URL(import.meta.url).pathname)) {
  main();
}
