/**
 * 静态托管下 browser router 深链的最小补救。
 *
 * 背景：页面托管在 GitHub Pages 这类纯静态环境，`/app/stock/000001` 这种路径没有对应文件，
 * 服务器会返回根 `404.html`。根 404 的分流脚本会把「应用内相对路径」放进
 * `?__app_path=...` 跳到应用根；应用启动时用这里把 URL 还原成用户真正想看的地址，
 * 否则 router 会停在首页。
 *
 * 约束：只动 URL（`history.replaceState`），不换路由方案、不引入新概念。
 */

/** 404 兜底跳转约定的 query 参数名；与 site/scripts/gen-pages-404.mjs 保持一致。 */
export const APP_PATH_PARAM = '__app_path';

/**
 * 从 404 带回的 query 里算出应还原成什么 URL。
 *
 * @param search 形如 `?__app_path=stock%2F000001%3Ftab%3Dkline`
 * @param baseUrl 应用的部署 base（Vite 的 `import.meta.env.BASE_URL`，如 `/tongstock/app/`）
 * @returns 还原后的完整路径（含 base 与原有 search/hash）；无需还原时返回 `null`
 */
export function restoreAppPath(search: string, baseUrl: string): string | null {
  const normalizedSearch = search.startsWith('?') ? search : `?${search}`;
  const params = new URLSearchParams(normalizedSearch);
  const raw = params.get(APP_PATH_PARAM);
  if (!raw) return null;

  // 404 脚本可能带上前导斜杠，统一去掉，避免拼成 `//stock/000001`。
  const inner = raw.replace(/^\/+/, '');
  if (!inner) return null;

  // 除 __app_path 之外的参数不属于还原流程，若存在则原样保留在末尾。
  params.delete(APP_PATH_PARAM);
  const rest = params.toString();

  const base = baseUrl.endsWith('/') ? baseUrl : `${baseUrl}/`;
  if (!rest) return `${base}${inner}`;
  const separator = inner.includes('?') ? '&' : '?';
  return `${base}${inner}${separator}${rest}`;
}

/**
 * 在浏览器里执行还原；返回还原后的 URL，未做任何事时返回 `null`。
 * 传参形式的 window 依赖是为了让这段逻辑可在 node 里测试。
 */
export function applyRestoredAppPath(
  target: { location: Pick<Location, 'search'>; history: Pick<History, 'replaceState'> } = window,
): string | null {
  const restored = restoreAppPath(target.location.search, import.meta.env.BASE_URL);
  if (!restored) return null;
  target.history.replaceState(null, '', restored);
  return restored;
}
