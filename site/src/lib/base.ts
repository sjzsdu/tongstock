/**
 * 站点 base 的统一处理。
 *
 * 背景：`import.meta.env.BASE_URL` 的形态不稳定（根路径是 `/`，子路径可能是 `/tongstock`
 * 也可能是 `/tongstock/`），而页面里到处手拼 `${BASE_URL}x.png` 一旦少了斜杠就会得到
 * `/tongstockx.png` 这种畸形 URL。所以**任何站内链接都必须过这里**，不要在页面里裸拼。
 */

/** 规范化后的 base：始终以 `/` 开头、以 `/` 结尾；根路径为 `/`。 */
const raw = import.meta.env.BASE_URL || '/';
export const base = raw.endsWith('/') ? raw : `${raw}/`;

/**
 * 把站内路径拼成带 base 前缀的路径。
 * `withBase('favicon.png')` → `/tongstock/favicon.png`；`withBase('/docs/')` → `/tongstock/docs/`。
 */
export function withBase(path: string): string {
  const clean = path.replace(/^\/+/, '');
  return clean ? `${base}${clean}` : base;
}

/**
 * 站内路径 → 绝对 URL。社交卡片（og:image / twitter:image）与 canonical 必须是绝对地址，
 * 否则抓取方会按自己的域名解析。
 */
export function absoluteUrl(path: string, site?: URL | string): string {
  const origin =
    site instanceof URL
      ? site
      : new URL(typeof site === 'string' && site ? site : 'https://sjzsdu.github.io');
  return new URL(withBase(path), origin).toString();
}
