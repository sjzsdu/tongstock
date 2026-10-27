// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import sitemap from '@astrojs/sitemap';
import baseLinks from './src/integrations/base-links.mjs';

/**
 * 站点部署位置尚未定案（阶段 3 待人类拍板），所以 base 与站点域名都从环境变量取：
 *   SITE_URL=https://example.com SITE_BASE=/tongstock pnpm build
 * 默认按“站点部署在域名根”构建，本地预览与 CI 都不需要额外变量。
 */
const siteUrl = process.env.SITE_URL || 'https://sjzsdu.github.io';
const base = process.env.SITE_BASE || '/';

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
          href: 'https://github.com/sjzsdu/tongstock',
        },
      ],
      customCss: ['./src/styles/starlight-theme.css'],
      editLink: {
        baseUrl: 'https://github.com/sjzsdu/tongstock/edit/master/site/',
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
