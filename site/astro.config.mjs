// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import sitemap from '@astrojs/sitemap';
import baseLinks from './src/integrations/base-links.mjs';

/**
 * 部署位置全部来自构建期环境变量，默认值只服务本地开发与“站点在域名根”的场景：
 *   SITE_URL   站点 origin（默认 https://sjzsdu.github.io）
 *   SITE_BASE  部署子路径（默认 /；GitHub Pages 上是 /<repo>/）
 *
 * 变量表见 site/README.md 与 docs/ops/deploy.md。这里只做规范化与防呆，不写死任何地址。
 */
const rawSiteUrl = process.env.SITE_URL?.trim() || 'https://sjzsdu.github.io';
const rawBase = process.env.SITE_BASE?.trim() ?? '';

/** 子路径规范化为 `/` 开头、`/` 结尾；空串归一成根路径。 */
function normalizeBase(input) {
  const trimmed = input.trim();
  if (!trimmed || trimmed === '/') return '/';
  const withLeading = trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
  return withLeading.endsWith('/') ? withLeading : `${withLeading}/`;
}

let parsedSite;
try {
  parsedSite = new URL(rawSiteUrl);
} catch {
  throw new Error(`SITE_URL 不是合法 URL：${rawSiteUrl}（示例：https://sjzsdu.github.io）`);
}

// 防呆：origin 里带路径、SITE_BASE 却没设，是最容易出的一类自相矛盾配置
// （结果会是 base 落在 `/`，而社交卡片/canonical 指向带路径的 origin）。
const sitePath = parsedSite.pathname.replace(/\/+$/, '');
if (sitePath && (rawBase === '' || normalizeBase(rawBase) === '/')) {
  throw new Error(
    `SITE_URL 带了路径（${sitePath}）但 SITE_BASE 未设置。请二选一：` +
      '把路径挪到 SITE_BASE（SITE_URL=https://host SITE_BASE=/repo/），' +
      '或把 SITE_URL 改成纯 origin（https://host）。',
  );
}

const siteUrl = parsedSite.origin;
const base = normalizeBase(rawBase || '/');

export default defineConfig({
  site: siteUrl,
  base,
  trailingSlash: 'ignore',
  integrations: [
    baseLinks(base),
    starlight({
      title: 'TongStock',
      description:
        'TongStock 是一个自托管的 A 股研究与决策工具：AI 提出投资规律，确定性引擎用真实行情数据编译、冻结、回测并审查证据；证据不足时拒绝推荐。',
      defaultLocale: 'root',
      locales: {
        root: { label: '简体中文', lang: 'zh-CN' },
      },
      logo: {
        src: './src/assets/logo.png',
        alt: 'TongStock',
      },
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: process.env.REPO_URL?.trim() || 'https://github.com/sjzsdu/tongstock',
        },
      ],
      customCss: ['./src/styles/starlight-theme.css'],
      editLink: {
        baseUrl: `${(process.env.REPO_URL?.trim() || 'https://github.com/sjzsdu/tongstock').replace(/\/+$/, '')}/edit/master/site/`,
      },
      sidebar: [
        {
          label: '开始使用',
          items: [
            { label: '概览', slug: 'docs/getting-started/overview' },
            { label: '安装', slug: 'docs/getting-started/install' },
            { label: '首次运行与配置', slug: 'docs/getting-started/first-run' },
            { label: '三步上手', slug: 'docs/getting-started/three-steps' },
          ],
        },
        {
          label: '使用方式',
          items: [
            { label: 'Web UI 指南', slug: 'docs/usage/web-ui' },
            { label: 'CLI 指南', slug: 'docs/usage/cli' },
            { label: 'CLI 命令与参数参考', slug: 'docs/usage/cli-reference' },
            { label: '决策流水线', slug: 'docs/usage/decision-pipeline' },
            { label: '一致性模式', slug: 'docs/usage/consistency' },
            { label: 'AI 助手集成（Skills）', slug: 'docs/usage/skills' },
          ],
        },
        {
          label: 'HTTP API',
          items: [
            { label: '认证与安全', slug: 'docs/api/auth' },
            { label: '端点参考', slug: 'docs/api/endpoints' },
            { label: '行情与基础数据', slug: 'docs/api/market-data' },
            { label: 'K 线、分时与分笔', slug: 'docs/api/kline-minute-trade' },
            { label: '指标与信号筛选', slug: 'docs/api/indicator-screen' },
            { label: '财务、公司与板块', slug: 'docs/api/finance-company-block' },
            { label: '方法与选股', slug: 'docs/api/methods-selections' },
            { label: '范式、监控与前向跟踪', slug: 'docs/api/paradigm-monitoring-forward' },
            { label: '持仓决策', slug: 'docs/api/position-decisions' },
            { label: '资讯', slug: 'docs/api/news' },
          ],
        },
        {
          label: '数据说明',
          items: [
            { label: '数据来源', slug: 'docs/data/sources' },
            { label: '缓存与同步水位', slug: 'docs/data/cache-and-sync' },
            { label: 'K 线类型', slug: 'docs/data/kline-types' },
            { label: '指标口径', slug: 'docs/data/indicator-params' },
            { label: '决策口径', slug: 'docs/data/decision-semantics' },
          ],
        },
        {
          label: '运维与部署',
          items: [
            { label: 'Docker 部署', slug: 'docs/ops/deploy' },
            { label: '远程访问与令牌', slug: 'docs/ops/remote-access' },
            { label: '质量门禁', slug: 'docs/ops/quality-gates' },
          ],
        },
        {
          label: '参考',
          items: [
            { label: 'FAQ', slug: 'docs/reference/faq' },
            { label: '限制与免责', slug: 'docs/reference/limitations' },
            { label: '许可证', slug: 'docs/reference/license' },
          ],
        },
      ],
    }),
    sitemap(),
  ],
});
