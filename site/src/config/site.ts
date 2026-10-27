/**
 * 站点唯一的事实源：所有对外链接、产品名与免责口径都在这里定义。
 *
 * 换部署位置、换公网实例、换仓库地址时**只改这一个文件**，落地页与文档都读它，
 * 不允许在页面里硬编码 URL。
 */

export const site = {
  name: 'TongStock',
  tagline: '今天买什么，什么时候卖',
  description:
    'TongStock 是一个自托管的 A 股研究与决策工具：AI 提出投资规律，确定性引擎用真实行情数据编译、冻结、回测并审查证据；证据不足时拒绝推荐。',

  /**
   * 主 CTA「进入控制台」的落点。
   * 当前默认指向 GitHub Pages 上的 Web UI 静态外壳；换成公网实例时只改这一行。
   */
  appUrl: 'https://sjzsdu.github.io/tongstock/',
  appUrlNote: '静态托管只有界面，功能需要本机运行 ./tongstock serve；公网实例请自备 access_token',

  /** 本机启动命令：次 CTA 复制用，也是文档里的默认运行方式。 */
  localCommand: './tongstock serve',
  localUrl: 'http://localhost:8106/',

  repoUrl: 'https://github.com/sjzsdu/tongstock',
  releasesUrl: 'https://github.com/sjzsdu/tongstock/releases',
  dockerHubUrl: 'https://hub.docker.com/r/sjzsdu/tongstock',
  apiContractUrl: 'https://github.com/sjzsdu/tongstock/blob/master/api/openapi.json',
  /** 仓库目前没有独立 LICENSE 文件，许可声明在 README 的“许可证”小节。 */
  licenseUrl: 'https://github.com/sjzsdu/tongstock#许可证',

  /** 免责口径：与仓库 README「注意事项」保持一致，落地页页脚显式写出。 */
  disclaimer:
    '数据来源于通达信官方行情服务器，仅供学习研究使用，请勿用于商业用途；行情数据可能有延迟，不建议用于实盘交易。',

  /** 文档首页相对路径（Starlight 的 docs 集合挂在 /docs 下）。 */
  docsPath: '/docs/',
} as const;

export type SiteConfig = typeof site;
