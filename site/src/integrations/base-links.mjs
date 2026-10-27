/**
 * Astro 集成：构建完成后，给 HTML 里的站内绝对链接补上部署 base 前缀。
 *
 * 为什么放到构建后而不是 Markdown 插件：Astro 7 默认 Markdown 处理器已换成 Sätteri，
 * 用 remark 插件要额外安装并切回 unified 处理器，影响面远大于收益。
 * 这里只做一次纯文本级的 href/src 重写，与 Markdown 管道无关，且只在 base 非根时生效。
 *
 * 幂等：已经带 base 前缀的链接会被跳过，重复构建不会叠加。
 *
 * @param {string} rawBase 例如 '/' 或 '/tongstock'
 */
export default function baseLinks(rawBase) {
  const base = rawBase.endsWith('/') ? rawBase.slice(0, -1) : rawBase;

  return {
    name: 'tongstock-base-links',
    hooks: {
      'astro:build:done': async ({ dir, logger }) => {
        if (!base) return; // 部署在域名根，不需要处理

        const { readdir, readFile, writeFile } = await import('node:fs/promises');
        const { join, extname } = await import('node:path');

        const rewrite = (value) => {
          if (!value.startsWith('/')) return value;
          if (value.startsWith('//')) return value; // 协议相对，属于外链
          if (value === base || value.startsWith(`${base}/`)) return value;
          return `${base}${value}`;
        };

        let files = 0;
        let links = 0;

        const walk = async (current) => {
          for (const entry of await readdir(current, { withFileTypes: true })) {
            const full = join(current, entry.name);
            if (entry.isDirectory()) {
              await walk(full);
              continue;
            }
            if (extname(entry.name) !== '.html') continue;

            const html = await readFile(full, 'utf8');
            const next = html.replace(/((?:href|src|action)=")(\/[^"]*)(")/g, (_all, pre, value, post) => {
              const rewritten = rewrite(value);
              if (rewritten !== value) links += 1;
              return `${pre}${rewritten}${post}`;
            });

            if (next !== html) {
              await writeFile(full, next);
              files += 1;
            }
          }
        };

        await walk(dir.pathname);
        logger.info(`base 前缀 ${base}：改写 ${links} 处链接（${files} 个文件）`);
      },
    },
  };
}
