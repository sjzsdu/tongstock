import type { APIRoute } from 'astro';
import { withBase } from '../lib/base';

/**
 * robots.txt 由构建时生成，指向真实的 sitemap 地址。
 *
 * sitemap 的路径必须过 `withBase`：`import.meta.env.BASE_URL` 在子路径部署时是 `/tongstock`
 * （不带尾斜杠），直接拼字符串会得到 `/tongstocksitemap-index.xml`。
 */
export const GET: APIRoute = ({ site }) => {
  const origin = site ?? new URL('https://sjzsdu.github.io');
  const sitemap = new URL(withBase('sitemap-index.xml'), origin).toString();

  const body = ['User-agent: *', 'Allow: /', '', `Sitemap: ${sitemap}`, ''].join('\n');

  return new Response(body, {
    headers: { 'Content-Type': 'text/plain; charset=utf-8' },
  });
};
