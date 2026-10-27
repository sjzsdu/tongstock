/**
 * 站点配置的唯一入口：所有对外 URL 都可由**构建期环境变量**覆盖，这里只提供默认值。
 *
 * 为什么这样做：换域名、临时指到内网实例、把 CTA 指向别处，都不应该改代码。
 * 变量表（名称 / 默认值 / 作用 / 改哪里）见 `site/README.md` 与 `site/src/content/docs/docs/ops/deploy.md`。
 *
 * 取值顺序：`PUBLIC_<NAME>`（Vite 会注入到 import.meta.env，可用于将来的客户端脚本）
 *        → `<NAME>`（构建期 process.env）。
 * 两者都只在构建期求值，**不会**把值塞进客户端 bundle；确实需要在浏览器里读的，
 * 必须用 `PUBLIC_` 前缀，按 Vite 规则来。
 */

function buildEnv(name: string): string | undefined {
  const injected = (import.meta.env as unknown as Record<string, unknown>)[`PUBLIC_${name}`];
  if (typeof injected === 'string' && injected.trim()) return injected.trim();

  if (typeof process !== 'undefined' && process.env) {
    const raw = process.env[name];
    if (raw && raw.trim()) return raw.trim();
  }
  return undefined;
}

/** origin 只保留协议+主机+端口，路径部分交给 SITE_BASE，避免两处各写一半。 */
function normalizeOrigin(input: string): string {
  const url = new URL(input);
  return url.origin;
}

/** 子路径规范化为 `/` 开头、`/` 结尾；空串与 `''` 都归一成根路径 `/`。 */
function normalizeBase(input: string): string {
  const trimmed = input.trim();
  if (!trimmed || trimmed === '/') return '/';
  const withLeading = trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
  return withLeading.endsWith('/') ? withLeading : `${withLeading}/`;
}

/**
 * 拼接部署 base 与站内相对路径，得到**同站相对 URL**（如 `/app/`、`/tongstock/app/`）。
 * 相对 URL 让自托管与子路径部署共用同一份代码，不需要任何环境变量。
 */
function joinPath(base: string, path: string): string {
  const cleanBase = base.endsWith('/') ? base : `${base}/`;
  const cleanPath = path.replace(/^\/+/, '');
  if (!cleanPath) return cleanBase;
  return cleanPath.endsWith('/') ? `${cleanBase}${cleanPath}` : `${cleanBase}${cleanPath}/`;
}

const siteUrl = normalizeOrigin(buildEnv('SITE_URL') ?? 'https://sjzsdu.github.io');
const basePath = normalizeBase(buildEnv('SITE_BASE') ?? '/');
const repoUrl = (buildEnv('REPO_URL') ?? 'https://github.com/sjzsdu/tongstock').replace(/\/+$/, '');
const dockerImage = buildEnv('DOCKER_IMAGE') ?? 'sjzsdu/tongstock';

export const site = {
  name: 'TongStock',
  tagline: '今天买什么，什么时候卖',
  description:
    'TongStock 是一个自托管的 A 股研究与决策工具：AI 提出投资规律，确定性引擎用真实行情数据编译、冻结、回测并审查证据；证据不足时拒绝推荐。',

  /** 站点 origin 与部署子路径（与 astro.config.mjs 的 site/base 同源）。 */
  siteUrl,
  basePath,

  /**
   * 主 CTA「进入控制台」的落点。
   * 默认是**同站相对路径** `<base>app/`：自托管（base=/）下就是 `/app/`，GitHub Pages
   * （base=/<repo>/）下是 `/<repo>/app/`，两种部署都不需要额外配置。
   * 用 APP_URL 可以指向任何实例（含内网）。
   */
  appUrl: buildEnv('APP_URL') ?? joinPath(basePath, 'app/'),
  appUrlNote: '静态托管只有界面，功能需要本机运行 ./tongstock serve；公网实例请自备 access_token',

  /** 本机启动命令：次 CTA 复制用，也是文档里的默认运行方式。 */
  localCommand: buildEnv('LOCAL_COMMAND') ?? './tongstock serve',
  localUrl: buildEnv('LOCAL_URL') ?? 'http://localhost:8106/',

  repoUrl,
  releasesUrl: buildEnv('RELEASES_URL') ?? `${repoUrl}/releases`,
  dockerHubUrl: buildEnv('DOCKER_HUB_URL') ?? `https://hub.docker.com/r/${dockerImage}`,
  apiContractUrl: buildEnv('API_CONTRACT_URL') ?? `${repoUrl}/blob/master/api/openapi.json`,
  /** 仓库目前没有独立 LICENSE 文件，许可声明在 README 的“许可证”小节。 */
  licenseUrl: buildEnv('LICENSE_URL') ?? `${repoUrl}#许可证`,

  /** 免责口径：与仓库 README「注意事项」保持一致，落地页页脚显式写出。 */
  disclaimer:
    '数据来源于通达信官方行情服务器，仅供学习研究使用，请勿用于商业用途；行情数据可能有延迟，不建议用于实盘交易。',

  /** 文档首页相对路径（Starlight 的 docs 集合挂在 /docs 下）。 */
  docsPath: '/docs/',
} as const;

export type SiteConfig = typeof site;
