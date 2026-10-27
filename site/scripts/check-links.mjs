#!/usr/bin/env node
/**
 * 站点自检：两件事必须在 CI 里跑，否则文档会慢慢烂掉。
 *
 * 1. 死链：构建产物里的内部链接，逐个落到真实文件。
 * 2. 不存在的 API 路径：手写文档里出现的 `/api/...` 必须在生成的端点表里存在，
 *    这样“文档提到一个接口，但仓库里没有”这类失真会在 CI 红掉，而不是等读者发现。
 *
 * 前置条件：先 `pnpm build`（脚本读 dist/）。
 * 用法：node scripts/check-links.mjs
 */

import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs';
import { dirname, join, resolve, relative, posix } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));
const SITE = resolve(HERE, '..');
const DIST = resolve(SITE, 'dist');
const DOCS = resolve(SITE, 'src/content/docs');
const PAGES = resolve(SITE, 'src/pages');
const ENDPOINTS_MD = resolve(DOCS, 'docs/api/endpoints.md');

const problems = [];

function walk(dir, filter, out = []) {
  if (!existsSync(dir)) return out;
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) walk(full, filter, out);
    else if (filter(name)) out.push(full);
  }
  return out;
}

function checkDistExists() {
  if (!existsSync(DIST)) {
    console.error('[check-links] 缺少 dist/，先运行 pnpm build');
    process.exit(1);
  }
}

/* ---------------- 1. 死链 ---------------- */

/** 只关心站内链接：外链、协议链接、锚点、mailto 一律跳过。 */
function isInternal(href) {
  if (!href) return false;
  if (/^(https?:|mailto:|tel:|javascript:|data:)/i.test(href)) return false;
  if (href.startsWith('#')) return false;
  return true;
}

function resolveTarget(fromFile, href) {
  const clean = href.split('#')[0].split('?')[0];
  if (!clean) return null; // 纯锚点
  if (clean.startsWith('/')) return join(DIST, clean);
  return resolve(dirname(fromFile), clean);
}

function targetExists(target) {
  if (!target) return true;
  if (existsSync(target) && statSync(target).isFile()) return true;
  // 目录形式：/docs/ → /docs/index.html
  const joined = join(target, 'index.html');
  if (existsSync(joined)) return true;
  // 无扩展名的页面：/docs/foo → /docs/foo.html 或 /docs/foo/index.html
  if (!target.endsWith('.html') && existsSync(`${target}.html`)) return true;
  return false;
}

function checkDeadLinks() {
  const htmlFiles = walk(DIST, (name) => name.endsWith('.html'));
  if (!htmlFiles.length) {
    problems.push('dist/ 里没有任何 HTML，构建可能失败了');
    return;
  }

  const attr = /(?:href|src)="([^"]+)"/g;
  for (const file of htmlFiles) {
    const html = readFileSync(file, 'utf8');
    for (const match of html.matchAll(attr)) {
      const href = match[1];
      if (!isInternal(href)) continue;
      const target = resolveTarget(file, href);
      if (!targetExists(target)) {
        problems.push(
          `死链：${relative(DIST, file)} → ${href}（解析到 ${relative(DIST, target)}）`,
        );
      }
    }
  }
}

/* ---------------- 2. 不存在的 API 路径 ---------------- */

/** 从端点表里取全部路径（含 `{param}` 模板）。 */
function knownApiPaths() {
  if (!existsSync(ENDPOINTS_MD)) {
    problems.push('缺少 endpoints.md，先跑 node scripts/gen-api-docs.mjs');
    return new Set();
  }
  const text = readFileSync(ENDPOINTS_MD, 'utf8');
  const paths = new Set();
  for (const match of text.matchAll(/\|\s*(?:GET|POST|PUT|PATCH|DELETE)\s*\|\s*`([^`]+)`/g)) {
    paths.add(match[1]);
  }
  if (!paths.size) problems.push('endpoints.md 里没有解析到任何路径，生成逻辑可能坏了');
  return paths;
}

/**
 * 段级匹配，外加“模块前缀”放行：
 * 文档里会用 `/api`、`/api/forward/`、`/api/position-decisions` 指代一组端点，
 * 这类写法只要在该前缀下存在任何真实端点就算通过。
 */
function matchesKnown(path, known) {
  if (known.has(path)) return true;

  // 模块根：/api、/health 这类没有具体端点的写法
  if (path === '/api' || path === '/health') {
    return [...known].some((candidate) => candidate.startsWith(`${path}/`));
  }

  // 前缀写法：/api/forward/ 或 /api/position-decisions
  const prefix = path.endsWith('/') ? path : `${path}/`;
  if ([...known].some((candidate) => candidate.startsWith(prefix))) return true;

  const segments = path.replace(/\/$/, '').split('/');
  for (const candidate of known) {
    const parts = candidate.split('/');
    if (parts.length !== segments.length) continue;
    let ok = true;
    for (let i = 0; i < parts.length; i += 1) {
      const want = parts[i];
      const got = segments[i];
      if (want === got) continue;
      const templated = /^\{.+\}$/.test(want);
      // 具体值（数字代码、中文名、示例 id）允许命中模板段。
      const concrete = /^(?:\d+|[0-9A-Za-z-]*\d[0-9A-Za-z-]*|[^A-Za-z0-9_{}]+)$/.test(got);
      if (templated && (concrete || got === '')) continue;
      ok = false;
      break;
    }
    if (ok) return true;
  }
  return false;
}

function checkApiPaths() {
  const known = knownApiPaths();
  if (!known.size) return;

  const sources = [
    ...walk(DOCS, (name) => name.endsWith('.md')).filter((file) => file !== ENDPOINTS_MD),
    ...walk(PAGES, (name) => name.endsWith('.astro')),
  ];

  for (const file of sources) {
    const text = readFileSync(file, 'utf8');
    for (const match of text.matchAll(/`(\/(?:api|health)[A-Za-z0-9_\-/{}.:]*)/g)) {
      const raw = match[1].split('?')[0].split('#')[0];
      const path = raw.endsWith('/') ? raw.slice(0, -1) : raw;
      if (!matchesKnown(path, known)) {
        problems.push(`不存在的 API 路径：${relative(SITE, file)} → ${raw}`);
      }
    }
  }
}

/* ---------------- 执行 ---------------- */

checkDistExists();
checkDeadLinks();
checkApiPaths();

if (problems.length) {
  for (const problem of problems) console.error(`[check-links] ${problem}`);
  console.error(`[check-links] ${problems.length} 个问题`);
  process.exit(1);
}

console.log('[check-links] 通过：无死链，文档引用的 API 路径都存在');
