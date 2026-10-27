/**
 * Astro 集成：构建完成后，给 HTML 里的站内绝对 URL 补上部署 base 前缀。
 *
 * 为什么放到构建后而不是 Markdown 插件：Astro 7 默认 Markdown 处理器已换成 Sätteri，
 * 用 remark 插件要额外安装并切回 unified 处理器，影响面远大于收益。
 * 这里只做一次纯文本级的属性重写，与 Markdown 管道无关，且只在 base 非根时生效。
 *
 * 处理范围（四类，缺一个就会漏出畸形 URL）：
 *   href / src / action —— 普通链接与资源
 *   srcset             —— 响应式图片，逐项处理“路径 + 描述符”
 *   content            —— 只处理以 `/` 开头的值（og:image 一类；https 开头的不动）
 *
 * 幂等：已经带 base 前缀的值会跳过，重复构建不会叠加成 `/base/base/...`。
 *
 * @param {string} rawBase 例如 '/' 或 '/tongstock'
 */
export default function baseLinks(rawBase) {
  // 规范化：统一成 `/tongstock/` 形式，避免 `startsWith('/tongstock/')` 判定漏掉无尾斜杠的写法。
  const withSlash = rawBase.endsWith('/') ? rawBase : `${rawBase}/`;
  const prefix = withSlash.slice(0, -1); // `/tongstock`；根路径时为 ''

  return {
    name: 'tongstock-base-links',
    hooks: {
      'astro:build:done': async ({ dir, logger }) => {
        if (!prefix) return; // 部署在域名根，不需要处理

        const { readdir, readFile, writeFile } = await import('node:fs/promises');
        const { join, extname } = await import('node:path');

        /** 单个站内路径加前缀；外链、协议相对、已带前缀的都原样返回。 */
        const rewritePath = (value) => {
          if (!value.startsWith('/')) return value;
          if (value.startsWith('//')) return value; // 协议相对，属于外链
          if (value === prefix || value.startsWith(withSlash)) return value;
          return `${prefix}${value}`;
        };

        /** srcset 形如 `/a.png 1x, /b.png 2x`，逐项重写路径部分。 */
        const rewriteSrcset = (value) =>
          value
            .split(',')
            .map((item) => {
              const trimmed = item.trim();
              if (!trimmed) return trimmed;
              const [path, ...descriptor] = trimmed.split(/\s+/);
              return [rewritePath(path), ...descriptor].join(' ');
            })
            .join(', ');

        let files = 0;
        let rewrites = 0;

        const walk = async (current) => {
          for (const entry of await readdir(current, { withFileTypes: true })) {
            const full = join(current, entry.name);
            if (entry.isDirectory()) {
              await walk(full);
              continue;
            }
            if (extname(entry.name) !== '.html') continue;

            const html = await readFile(full, 'utf8');

            // 先处理 srcset（属性名本身以 src 结尾，避免被 src 规则误伤）。
            let next = html.replace(/(srcset=")([^"]*)(")/g, (_all, pre, value, post) => {
              const rewritten = rewriteSrcset(value);
              if (rewritten !== value) rewrites += 1;
              return `${pre}${rewritten}${post}`;
            });

            next = next.replace(/((?:href|src|action|content)=")([^"]*)(")/g, (_all, pre, value, post) => {
              const rewritten = rewritePath(value);
              if (rewritten !== value) rewrites += 1;
              return `${pre}${rewritten}${post}`;
            });

            if (next !== html) {
              await writeFile(full, next);
              files += 1;
            }
          }
        };

        await walk(dir.pathname);
        logger.info(`base 前缀 ${prefix}：改写 ${rewrites} 处 URL（${files} 个文件）`);
      },
    },
  };
}
