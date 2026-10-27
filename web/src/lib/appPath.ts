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
 * 安全约束：`__app_path` 是**外部可控输入**（任何人构造一个链接就能喂进来）。
 * 所以这里不接受「归一化后跑出 base」的值——`../x`、`%2e%2e/%2e%2e/etc` 一律当作
 * 没有可还原的目标返回 `null`，避免把任意路径写进 SPA 路由甚至跨源。
 *
 * @param search 形如 `?__app_path=stock%2F000001%3Ftab%3Dkline`
 * @param baseUrl 应用的部署 base（Vite 的 `import.meta.env.BASE_URL`，如 `/tongstock/app/`）
 * @returns 还原后的完整路径（含 base 与原有 search/hash）；无需还原或越界时返回 `null`
 */
export function restoreAppPath(search: string, baseUrl: string): string | null {
  const normalizedSearch = search.startsWith('?') ? search : `?${search}`;
  const params = new URLSearchParams(normalizedSearch);
  const raw = params.get(APP_PATH_PARAM);
  if (!raw) return null;

  // 除 __app_path 之外的参数不属于还原流程，若存在则原样保留在末尾。
  params.delete(APP_PATH_PARAM);
  const rest = params.toString();

  const base = baseUrl.endsWith('/') ? baseUrl : `${baseUrl}/`;
  const resolved = resolveWithinBase(raw, base);
  if (!resolved) return null;

  if (!rest) return resolved;
  const separator = resolved.includes('?') ? '&' : '?';
  return `${resolved}${separator}${rest}`;
}

/**
 * 把 `__app_path` 归一化成 base 内的绝对路径；越界或非法时返回 `null`。
 *
 * 借用 WHATWG URL 的归一化能力（消解 `.` / `..`、识别 `%2e` 编码），
 * 占位 origin 只用于解析，不会产生网络访问。
 */
function resolveWithinBase(raw: string, base: string): string | null {
  // 去掉前导斜杠：否则 `//evil.com/x` 会被解析成协议相对 URL（跨源）。
  const trimmed = raw.replace(/^\/+/, '');
  if (!trimmed) return null;

  // 同时按「原样」和「百分号解码后」两种解读各校验一次：`..%2f..%2fetc` 这类值
  // 原样看只是一个普通段，但任何下游一旦解码就会变成路径分隔符——两种解读里
  // 只要有一种会跑出 base，就拒绝还原。
  const candidates = [trimmed];
  try {
    const decoded = decodeURIComponent(trimmed);
    if (decoded !== trimmed) candidates.push(decoded);
  } catch {
    // 非法百分号编码：保留原样解读，交给下面的解析去判
  }
  for (const candidate of candidates) {
    if (!normalizeOnce(candidate, base)) return null;
  }

  return normalizeOnce(trimmed, base);
}

/** 单次解读的归一化；越界返回 null。 */
function normalizeOnce(value: string, base: string): string | null {
  const placeholder = 'http://placeholder.invalid';
  let parsed: URL;
  try {
    parsed = new URL(`${base}${value}`, placeholder);
  } catch {
    return null;
  }

  if (parsed.origin !== new URL(placeholder).origin) return null; // 被解析成了绝对地址
  if (!parsed.pathname.startsWith(base)) return null; // 归一化后跑出了应用 base

  return `${parsed.pathname}${parsed.search}${parsed.hash}`;
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
