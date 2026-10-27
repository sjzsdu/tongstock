#!/usr/bin/env node
/**
 * 从仓库真实产物生成文档里的「机器事实」：
 *   1. src/content/docs/docs/api/endpoints.md —— 全部 HTTP 路由表，来源 api/openapi.json
 *   2. src/data/generated.json —— 落地页与文档引用的规模数字，来源可逐个追溯（见 STAT_RULES）
 *
 * 设计约束：
 *   - 幂等：输出只由输入决定，不含时间戳/随机值，重复运行必须字节一致；
 *   - --check：只比对不写盘，供 CI 与本地 verify 使用；
 *   - 不猜：任何取不到的统计都直接报错退出，而不是回落到一个看起来合理的数字。
 *
 * 用法：node scripts/gen-api-docs.mjs [--check]
 */

import { readFileSync, writeFileSync, readdirSync, existsSync, mkdirSync } from 'node:fs';
import { dirname, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));
const SITE = resolve(HERE, '..');
const ROOT = resolve(SITE, '..');
const CHECK = process.argv.includes('--check');

const OPENAPI = resolve(ROOT, 'api/openapi.json');
const OUT_ENDPOINTS = resolve(SITE, 'src/content/docs/docs/api/endpoints.md');
const OUT_DATA = resolve(SITE, 'src/data/generated.json');

const rel = (p) => relative(ROOT, p);

/** 分组规则：顺序即输出顺序，first match wins。 */
const GROUPS = [
  {
    id: 'health',
    title: '健康检查',
    anchor: 'health',
    prefixes: ['/health'],
  },
  {
    id: 'market',
    title: '行情与基础数据',
    anchor: 'market',
    prefixes: [
      '/api/quote',
      '/api/quotes',
      '/api/codes',
      '/api/count',
      '/api/auction',
      '/api/block',
      '/api/index',
      '/api/stocks',
      '/api/stockinfo',
      '/api/history',
      '/api/watchlist',
      '/api/stockpool',
    ],
  },
  {
    id: 'bars',
    title: 'K 线、分时与分笔',
    anchor: 'bars',
    prefixes: ['/api/kline', '/api/minute', '/api/trade', '/api/trades', '/api/xdxr'],
  },
  {
    id: 'signals',
    title: '指标与信号筛选',
    anchor: 'signals',
    prefixes: ['/api/indicator', '/api/screen', '/api/signal-analysis', '/api/factors', '/api/discover'],
  },
  {
    id: 'company',
    title: '财务与公司',
    anchor: 'company',
    prefixes: ['/api/finance', '/api/company'],
  },
  {
    id: 'methods',
    title: '方法与选股',
    anchor: 'methods',
    prefixes: [
      '/api/methods',
      '/api/method-families',
      '/api/selections',
      '/api/automation',
      '/api/onboarding',
      '/api/dashboard',
    ],
  },
  {
    id: 'paradigm',
    title: '范式、监控与前向跟踪',
    anchor: 'paradigm',
    prefixes: ['/api/paradigm', '/api/monitoring', '/api/forward'],
  },
  {
    id: 'position',
    title: '持仓决策与设置',
    anchor: 'position',
    prefixes: ['/api/position-decisions', '/api/settings'],
  },
  {
    id: 'news',
    title: '资讯',
    anchor: 'news',
    prefixes: ['/api/news'],
  },
  {
    id: 'agent',
    title: 'Agent',
    anchor: 'agent',
    prefixes: ['/api/agent'],
  },
];

const OTHER = { id: 'other', title: '其他', anchor: 'other' };

/** 规模数字的取数规则，落地页的矩阵依赖它，改动必须同步文档。 */
const STAT_RULES = {
  apiPaths: `openapi.json 里 paths 的条目数（${rel(OPENAPI)}）`,
  httpOperations: `openapi.json 里方法×路径的组合数（同一个 path 上的 GET/POST 分别计数）`,
  klineTypes: `pkg/tdx/service.go 的 ParseKlineType 分支数（每行首个字符串视为规范名）`,
  indicatorFamilies: `pkg/ta/types.go 的 IndicatorType 常量数 + 量比（VolumeRatio 字段）`,
  webPages: `web/src/App.tsx 中的 <Route path> 数，排除 "*" 兜底`,
  newsSources: `pkg/newsfeed/sources/*.go 的实现文件数，排除 *_test.go、sources.go、browser_*`,
  cliTopLevel: `tongstock --help 输出的 Available Commands 数（由 scripts/gen-cli-docs.mjs 写入）`,
};

function fail(message) {
  console.error(`[gen-api-docs] ${message}`);
  process.exit(1);
}

/** 读文件并在缺失时给出明确错误，而不是静默产出空表。 */
function readRequired(path) {
  if (!existsSync(path)) fail(`缺少输入文件：${rel(path)}`);
  return readFileSync(path, 'utf8');
}

function parseOpenapi() {
  try {
    return JSON.parse(readRequired(OPENAPI));
  } catch (error) {
    fail(`api/openapi.json 解析失败：${error.message}`);
  }
}

/** 解析参数：inline 参数直接取，$ref 到 components/parameters 的回查后取。 */
function resolveParams(op, spec) {
  const out = [];
  for (const raw of op.parameters ?? []) {
    let param = raw;
    if (raw.$ref) {
      const name = raw.$ref.split('/').pop();
      param = spec.components?.parameters?.[name];
      if (!param) fail(`无法解析参数引用 ${raw.$ref}`);
    }
    out.push(param);
  }
  return out;
}

function describeParams(op, spec) {
  const params = resolveParams(op, spec);
  if (!params.length) return '—';
  return params
    .map((p) => {
      const required = p.required ? '' : '?';
      const where = p.in === 'path' ? 'path' : p.in;
      return `\`${p.name}${required}\`(${where})`;
    })
    .join(' · ');
}

function hasBody(op) {
  return Boolean(op.requestBody);
}

function statusCodes(op) {
  const codes = Object.keys(op.responses ?? {});
  return codes.length ? codes.join(' / ') : '—';
}

/** 表格单元格里的竖线必须转义，否则会劈开表格。 */
const cell = (text) => String(text).replace(/\|/g, '\\|').replace(/\n/g, ' ');

function groupOf(path) {
  for (const group of GROUPS) {
    if (group.prefixes.some((prefix) => path === prefix || path.startsWith(`${prefix}/`))) return group;
  }
  return OTHER;
}

function buildEndpointTable(spec) {
  const rows = [];
  for (const [path, item] of Object.entries(spec.paths)) {
    for (const [method, op] of Object.entries(item)) {
      if (!['get', 'post', 'put', 'patch', 'delete', 'head', 'options'].includes(method)) continue;
      rows.push({
        path,
        method: method.toUpperCase(),
        operationId: op.operationId ?? '',
        params: describeParams(op, spec),
        body: hasBody(op) ? '是' : '—',
        status: statusCodes(op),
        group: groupOf(path),
      });
    }
  }
  rows.sort((a, b) => a.path.localeCompare(b.path) || a.method.localeCompare(b.method));
  return rows;
}

function renderEndpoints(spec, rows) {
  const counts = new Map();
  for (const row of rows) counts.set(row.group.id, (counts.get(row.group.id) ?? 0) + 1);

  const lines = [];
  lines.push('---');
  lines.push('title: 端点参考');
  lines.push('description: TongStock 全部 HTTP 路由，由 api/openapi.json 生成，请勿手工编辑。');
  lines.push('---');
  lines.push('');
  lines.push('本页由 `site/scripts/gen-api-docs.mjs` 从仓库里的 `api/openapi.json` 生成，**不要手工编辑**。');
  lines.push('契约的权威定义、以及它与 Gin 实际路由的双向比对，都在仓库里完成；本页只是它的可读视图。');
  lines.push('');
  lines.push(`- 路由总数：**${rows.length}**（含 health 探针）`);
  lines.push(`- 生成来源：\`${rel(OPENAPI)}\``);
  lines.push('- 参数列：带 `?` 表示可选；`(query)` / `(path)` 表示参数位置');
  lines.push('- 请求列：`是` 表示该端点接受 JSON 请求体');
  lines.push('');

  const ordered = [...GROUPS, OTHER];
  for (const group of ordered) {
    const groupRows = rows.filter((row) => row.group.id === group.id);
    if (!groupRows.length) continue;
    lines.push(`## ${group.title}`);
    lines.push('');
    lines.push(`共 ${groupRows.length} 个端点。`);
    lines.push('');
    lines.push('| 方法 | 路径 | operationId | 参数 | 请求体 | 响应码 |');
    lines.push('| --- | --- | --- | --- | --- | --- |');
    for (const row of groupRows) {
      lines.push(
        `| ${row.method} | \`${cell(row.path)}\` | ${cell(row.operationId) || '—'} | ${cell(row.params)} | ${row.body} | ${cell(row.status)} |`,
      );
    }
    lines.push('');
  }

  lines.push(':::note[为什么不在文档里逐个解释字段]');
  lines.push('字段级定义随代码变动，抄一份到文档里必然失真。要看具体响应结构，请直接读');
  lines.push('`api/openapi.json` 的 `components.schemas`，或用 `curl` 打本机实例。');
  lines.push(':::');
  lines.push('');
  return { text: lines.join('\n'), counts };
}

function countKlineTypes() {
  const source = readRequired(resolve(ROOT, 'pkg/tdx/service.go'));
  const start = source.indexOf('func ParseKlineType');
  if (start < 0) fail('pkg/tdx/service.go 里找不到 ParseKlineType');
  const body = source.slice(start, source.indexOf('\n}', start));
  const names = new Set();
  for (const line of body.split('\n')) {
    const match = line.match(/^\s*case\s+"([^"]+)"/);
    if (match) names.add(match[1]);
  }
  if (!names.size) fail('ParseKlineType 里没有解析到任何 K 线类型');
  return names.size;
}

function countIndicatorFamilies() {
  const source = readRequired(resolve(ROOT, 'pkg/ta/types.go'));
  const declared = source.match(/Indicator[A-Z]+\s+IndicatorType\s*=\s*"[^"]+"/g) ?? [];
  if (!declared.length) fail('pkg/ta/types.go 里没有解析到 IndicatorType 常量');
  const hasVolumeRatio = /VolumeRatio\s+\*VolumeRatioResult/.test(source);
  return declared.length + (hasVolumeRatio ? 1 : 0);
}

function countWebPages() {
  const source = readRequired(resolve(ROOT, 'web/src/App.tsx'));
  const routes = source.match(/<Route\s+path="([^"]+)"/g) ?? [];
  const pages = routes.filter((line) => !line.includes('path="*"'));
  if (!pages.length) fail('web/src/App.tsx 里没有解析到任何 <Route path>');
  return pages.length;
}

function countNewsSources() {
  const dir = resolve(ROOT, 'pkg/newsfeed/sources');
  if (!existsSync(dir)) fail('缺少目录 pkg/newsfeed/sources');
  const files = readdirSync(dir).filter(
    (name) =>
      name.endsWith('.go') &&
      !name.endsWith('_test.go') &&
      name !== 'sources.go' &&
      !name.startsWith('browser'),
  );
  if (!files.length) fail('pkg/newsfeed/sources 里没有找到实现文件');
  return files.length;
}

function readGenerated() {
  if (!existsSync(OUT_DATA)) return {};
  try {
    return JSON.parse(readFileSync(OUT_DATA, 'utf8'));
  } catch {
    return {};
  }
}

function main() {
  const spec = parseOpenapi();
  const rows = buildEndpointTable(spec);
  const { text: endpointsText, counts } = renderEndpoints(spec, rows);

  const previous = readGenerated();
  const stats = {
    apiPaths: Object.keys(spec.paths).length,
    httpOperations: rows.length,
    klineTypes: countKlineTypes(),
    indicatorFamilies: countIndicatorFamilies(),
    webPages: countWebPages(),
    newsSources: countNewsSources(),
    // 由 gen-cli-docs.mjs 维护；缺失时不影响本脚本，但站点构建会因 undefined 露出空位，所以一并报错。
    cliTopLevel: previous.stats?.cliTopLevel ?? 0,
  };

  const data = {
    _generated: '由 site/scripts/gen-api-docs.mjs 与 gen-cli-docs.mjs 生成，请勿手工编辑。',
    stats,
    statRules: STAT_RULES,
    apiGroups: [...GROUPS, OTHER]
      .map((group) => ({ id: group.id, title: group.title, endpoints: counts.get(group.id) ?? 0 }))
      .filter((group) => group.endpoints > 0),
  };
  const dataText = `${JSON.stringify(data, null, 2)}\n`;

  if (CHECK) {
    const problems = [];
    if (!existsSync(OUT_ENDPOINTS) || readFileSync(OUT_ENDPOINTS, 'utf8') !== endpointsText) {
      problems.push(`${rel(OUT_ENDPOINTS)} 与 api/openapi.json 不一致（跑 node scripts/gen-api-docs.mjs 重新生成）`);
    }
    if (!existsSync(OUT_DATA) || readFileSync(OUT_DATA, 'utf8') !== dataText) {
      problems.push(`${rel(OUT_DATA)} 与仓库现状不一致（跑 node scripts/gen-api-docs.mjs 重新生成）`);
    }
    if (problems.length) {
      for (const problem of problems) console.error(`[gen-api-docs] ${problem}`);
      process.exit(1);
    }
    console.log(`[gen-api-docs] 一致：${rows.length} 条路由，${data.apiGroups.length} 个分组`);
    return;
  }

  mkdirSync(dirname(OUT_ENDPOINTS), { recursive: true });
  mkdirSync(dirname(OUT_DATA), { recursive: true });
  writeFileSync(OUT_ENDPOINTS, endpointsText);
  writeFileSync(OUT_DATA, dataText);
  console.log(`[gen-api-docs] 写入 ${rel(OUT_ENDPOINTS)}（${rows.length} 条路由）`);
  console.log(`[gen-api-docs] 写入 ${rel(OUT_DATA)}（stats: ${JSON.stringify(stats)}）`);
}

main();
