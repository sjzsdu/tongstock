import type { APIRoute } from 'astro';

/**
 * robots.txt 由构建时生成，指向真实的 sitemap 地址。
 * 站点域名与 base 都取 astro.config.mjs 里的 site/base，避免写死两份。
 */
export const GET: APIRoute = ({ site }) => {
  const base = import.meta.env.BASE_URL;
  const origin = site ?? new URL('https://sjzsdu.github.io');
  const sitemap = new URL(`${base}sitemap-index.xml`, origin).toString();

  const body = ['User-agent: *', 'Allow: /', '', `Sitemap: ${sitemap}`, ''].join('\n');

  return new Response(body, {
    headers: { 'Content-Type': 'text/plain; charset=utf-8' },
  });
};
