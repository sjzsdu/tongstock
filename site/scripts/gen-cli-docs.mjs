#!/usr/bin/env node
/**
 * 从二进制自身的 --help 输出生成 CLI 参考，写入
 *   src/content/docs/docs/usage/cli-reference.md
 * 并把顶层命令数写进 src/data/generated.json 的 stats.cliTopLevel。
 *
 * 为什么必须实跑而不是手写：
 * * flag 名、默认值、是否 required 只有 cobra 自己知道；
 * * 手抄一份参数表，下一次改代码就失真，而失真后的文档比没有文档更糟。
 *
 * 二进制来源优先级：环境变量 TONGSTOCK_BIN → 仓库根 ./tongstock → PATH 里的 tongstock。
 * 找不到就直接失败退出（宁可让 CI 红，也不产出不完整的「参考」）。
 *
 * 用法：node scripts/gen-cli-docs.mjs [--check]
 */

import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs';
import { dirname, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));
const SITE = resolve(HERE, '..');
const ROOT = resolve(SITE, '..');
const CHECK = process.argv.includes('--check');

const OUT_DOC = resolve(SITE, 'src/content/docs/docs/usage/cli-reference.md');
const OUT_DATA = resolve(SITE, 'src/data/generated.json');

/** cobra 自带的命令：真实存在但对使用者没有参考价值，从文档里排除并注明。 */
const EXCLUDED = new Set(['completion', 'help']);

const MAX_DEPTH = 3;

const rel = (p) => relative(ROOT, p);

function fail(message) {
  console.error(`[gen-cli-docs] ${message}`);
  process.exit(1);
}

function resolveBinary() {
  const candidates = [];
  if (process.env.TONGSTOCK_BIN) candidates.push(process.env.TONGSTOCK_BIN);
  candidates.push(resolve(ROOT, 'tongstock'));
  candidates.push('tongstock');

  for (const candidate of candidates) {
    if (candidate === 'tongstock') {
      try {
        execFileSync('tongstock', ['--help'], { stdio: 'ignore' });
        return 'tongstock';
      } catch {
        continue;
      }
    }
    if (existsSync(candidate)) return candidate;
  }

  fail(
    [
      '找不到 tongstock 二进制，无法生成 CLI 参考。',
      '先构建：make cli（会连带构建 Web 并嵌入 pkg/web/dist），',
      '或用 TONGSTOCK_BIN=/path/to/tongstock 指定已有二进制。',
    ].join('\n'),
  );
}

function helpText(binary, args) {
  try {
    return execFileSync(binary, [...args, '--help'], { encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
  } catch (error) {
    fail(`执行 ${args.join(' ') || '(root)'} --help 失败：${error.message}`);
  }
}

/**
 * 生成的文本要做一层清理：某些命令的简介里带内部任务编号（形如 `tongstock-ai.7:`），
 * 那是开发期的追溯标记，不该出现在对外文档里。只去掉编号，不改写句子。
 */
function sanitize(text) {
  return text.replace(/tongstock-ai\.\d+[:：]\s*/g, '').replace(/\s{2,}/g, ' ').trim();
}

/** 从 help 输出里截取某一节的正文行（到下一个空行 + 顶格标题为止）。 */
function section(text, heading) {
  const lines = text.split('\n');
  const start = lines.findIndex((line) => line.trim() === heading);
  if (start < 0) return [];
  const out = [];
  for (let i = start + 1; i < lines.length; i += 1) {
    const line = lines[i];
    if (line.trim() === '') {
      // 空行后若紧跟一个新的顶格标题，本节结束。
      const next = lines.slice(i + 1).find((l) => l.trim() !== '');
      if (next && /^[A-Z][A-Za-z ]*:$/.test(next.trim())) break;
      if (!next) break;
      out.push('');
      continue;
    }
    out.push(line);
  }
  return out;
}

/** Usage 段的命令行，用于展示「怎么调用」。 */
function usageLine(text) {
  const lines = section(text, 'Usage:')
    .map((line) => line.trim())
    .filter(Boolean);
  return lines.length ? lines.join('\n') : '';
}

/** Available Commands 段：返回 [{name, short}]，保持 cobra 的输出顺序。 */
function subcommands(text) {
  const out = [];
  for (const line of section(text, 'Available Commands:')) {
    const match = line.match(/^\s{2}([a-z0-9][\w-]*)\s{2,}(.+)$/i);
    if (match) out.push({ name: match[1], short: match[2].trim() });
  }
  return out;
}

/**
 * Flags 段解析。cobra 的两种写法：
 *       --json                 说明
 *   -c, --code string          说明
 * 续行是更深的缩进且不以 '-' 开头。
 */
function parseFlags(lines) {
  const flags = [];
  let current = null;
  for (const line of lines) {
    if (!line.trim()) continue;
    const match = line.match(/^\s+(?:(-\w), )?(--[\w-]+)(?:\s+(\w+))?\s{2,}(.*)$/);
    if (match) {
      current = {
        short: match[1] ?? '',
        name: match[2],
        type: match[3] ?? 'bool',
        desc: match[4].trim(),
      };
      flags.push(current);
      continue;
    }
    if (current && /^\s{6,}/.test(line)) {
      current.desc = `${current.desc} ${line.trim()}`.trim();
    }
  }
  return flags;
}

/** 把 "... (default "x")" 从说明里拆出来，单独成列，表格更好读。 */
function splitDefault(desc) {
  const match = desc.match(/\s*\(default (.*)\)\s*$/);
  if (!match) return { desc: desc.trim(), def: '—' };
  return { desc: desc.slice(0, match.index).trim(), def: match[1].trim() };
}

const cell = (text) => String(text).replace(/\|/g, '\\|').replace(/\n+/g, ' ').trim();

function flagTable(flags) {
  if (!flags.length) return ['该命令没有自己的 flag。', ''];
  const lines = ['| Flag | 类型 | 默认值 | 说明 |', '| --- | --- | --- | --- |'];
  for (const flag of flags) {
    const { desc, def } = splitDefault(flag.desc);
    const name = flag.short ? `${flag.short}, ${flag.name}` : flag.name;
    lines.push(`| \`${cell(name)}\` | ${flag.type} | ${cell(def)} | ${cell(desc) || '—'} |`);
  }
  lines.push('');
  return lines;
}

/**
 * cobra 的普通 --help 输出没有 "Short:" 标签：命令的简介就是 "Usage:" 之前的第一段文字。
 * 因此 short 取该段首行，intro 取整段（多行时保留换行）。
 */
function headText(text) {
  const lines = text.split('\n');
  const usageIdx = lines.findIndex((line) => line.trim() === 'Usage:');
  const head = (usageIdx < 0 ? lines : lines.slice(0, usageIdx)).filter((line) => line.trim() !== '');
  return {
    short: head[0]?.trim() ?? '',
    intro: head.join('\n').trim(),
  };
}

function walk(binary, path, depth, collected) {
  const args = path.filter(Boolean);
  const text = helpText(binary, args);
  const label = ['tongstock', ...args].join(' ');
  const { short, intro } = headText(text);

  const entry = {
    label,
    depth,
    short: sanitize(short),
    intro: sanitize(intro),
    usage: usageLine(text),
    ownFlags: parseFlags(section(text, 'Flags:')).filter((flag) => flag.name !== '--help'),
    globalFlags: args.length ? parseFlags(section(text, 'Global Flags:')) : [],
    children: [],
  };

  if (depth >= MAX_DEPTH) return entry;

  for (const child of subcommands(text)) {
    if (EXCLUDED.has(child.name)) continue;
    entry.children.push(walk(binary, [...args, child.name], depth + 1, collected));
  }
  return entry;
}

function normalizeNodes(node) {
  // 深层节点的 Global Flags 与顶层重复，只在顶层展示一次。
  if (node.depth > 0) node.globalFlags = [];
  node.children = node.children.map(normalizeNodes);
  return node;
}

function render(nodes, rootGlobalFlags) {
  const topLevel = nodes;
  const lines = [];
  lines.push('---');
  lines.push('title: CLI 命令与参数参考');
  lines.push('description: 由 tongstock --help 实跑输出生成的命令与参数表，请勿手工编辑。');
  lines.push('---');
  lines.push('');
  lines.push('本页由 `site/scripts/gen-cli-docs.mjs` 调用二进制自身的 `--help` 生成，**不要手工编辑**。');
  lines.push('参数的事实源永远是二进制：任何一处不一致，都以 `tongstock <cmd> --help` 的输出为准。');
  lines.push('');
  lines.push(`- 顶层命令：**${topLevel.length}** 个（已省略 cobra 自动生成的 \`completion\` 与 \`help\`）`);
  lines.push('- 每个命令都有的 `-h, --help` 已省略，不重复列出');
  lines.push('- 想给 AI 助手用，见 [AI 助手集成](/docs/usage/skills/)；想看决策流水线怎么串，见 [决策流水线](/docs/usage/decision-pipeline/)。');
  lines.push('');

  if (rootGlobalFlags.length) {
    lines.push('## 全局参数');
    lines.push('');
    lines.push('下列参数对所有子命令生效，含义见[一致性模式](/docs/usage/consistency/)。');
    lines.push('');
    lines.push(...flagTable(rootGlobalFlags));
  }

  lines.push('## 顶层命令一览');
  lines.push('');
  lines.push('| 命令 | 作用 |');
  lines.push('| --- | --- |');
  for (const node of topLevel) {
    lines.push(`| [\`${node.label}\`](#${anchorOf(node.label)}) | ${cell(node.short) || '—'} |`);
  }
  lines.push('');

  const emit = (node) => {
    lines.push(`## \`${node.label}\``);
    lines.push('');
    if (node.intro) {
      lines.push(node.intro);
      lines.push('');
    }
    if (node.usage) {
      lines.push('```bash');
      lines.push(node.usage);
      lines.push('```');
      lines.push('');
    }
    lines.push('**Flags**');
    lines.push('');
    lines.push(...flagTable(node.ownFlags));

    if (node.children.length) {
      lines.push('**子命令**');
      lines.push('');
      lines.push('| 子命令 | 作用 |');
      lines.push('| --- | --- |');
      for (const child of node.children) {
        lines.push(`| [\`${child.label}\`](#${anchorOf(child.label)}) | ${cell(child.short) || '—'} |`);
      }
      lines.push('');
    }

    for (const child of node.children) emit(child);
  };

  for (const node of topLevel) emit(node);

  lines.push(':::note[怎么读这张表]');
  lines.push('`类型` 列里的 `bool` 表示开关型 flag，出现即生效；其余类型需要跟一个值。');
  lines.push('`默认值` 列显示 `—` 表示该 flag 没有默认值（通常是空字符串或零值）。');
  lines.push(':::');
  lines.push('');
  return lines.join('\n');
}

/** 标题锚点：把 `tongstock method compile` 转成 `tongstock-method-compile`。 */
function anchorOf(label) {
  return label
    .toLowerCase()
    .replace(/[^\w\u4e00-\u9fa5 -]/g, '')
    .replace(/ +/g, '-');
}

function readGenerated() {
  if (!existsSync(OUT_DATA)) {
    fail('缺少 site/src/data/generated.json，先跑 node scripts/gen-api-docs.mjs');
  }
  try {
    return JSON.parse(readFileSync(OUT_DATA, 'utf8'));
  } catch (error) {
    fail(`src/data/generated.json 解析失败：${error.message}`);
  }
}

function main() {
  const binary = resolveBinary();
  const rootText = helpText(binary, []);
  const rootNode = walk(binary, [], 0, []);
  const topLevel = rootNode.children.map(normalizeNodes);

  if (!topLevel.length) fail('二进制 --help 里没有解析到任何顶层命令');

  // root 没有父命令，它的持久 flag 落在 "Flags:" 段里；各子命令看到的是同名 "Global Flags:"。
  const rootGlobalFlags = parseFlags(section(rootText, 'Flags:')).filter((flag) => flag.name !== '--help');
  if (!rootGlobalFlags.length) fail('没有解析到全局参数（--consistency / --refresh），--help 输出格式可能变了');

  const docText = render(topLevel, rootGlobalFlags);

  const generated = readGenerated();
  generated.stats = { ...generated.stats, cliTopLevel: topLevel.length };
  const dataText = `${JSON.stringify(generated, null, 2)}\n`;

  if (CHECK) {
    const problems = [];
    if (!existsSync(OUT_DOC) || readFileSync(OUT_DOC, 'utf8') !== docText) {
      problems.push(`${rel(OUT_DOC)} 与二进制 --help 不一致（跑 node scripts/gen-cli-docs.mjs 重新生成）`);
    }
    if (readFileSync(OUT_DATA, 'utf8') !== dataText) {
      problems.push(`${rel(OUT_DATA)} 的 stats.cliTopLevel 与二进制不一致`);
    }
    if (problems.length) {
      for (const problem of problems) console.error(`[gen-cli-docs] ${problem}`);
      process.exit(1);
    }
    console.log(`[gen-cli-docs] 一致：${topLevel.length} 个顶层命令（${binary}）`);
    return;
  }

  mkdirSync(dirname(OUT_DOC), { recursive: true });
  writeFileSync(OUT_DOC, docText);
  writeFileSync(OUT_DATA, dataText);
  console.log(`[gen-cli-docs] 写入 ${rel(OUT_DOC)}（${topLevel.length} 个顶层命令，二进制 ${binary}）`);
  if (!rootText.trim()) console.warn('[gen-cli-docs] 顶层 --help 输出为空，可能有问题');
}

main();
