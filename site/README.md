# site/ —— TongStock 官网站点

落地页 + 使用文档的静态站。**与仓库其余部分硬隔离**：自带 `package.json` 与 lockfile，不参与 Go 构建，也不进入根 `Makefile` 的 `check` 门禁。

## 这是什么

- `/` 落地页：`src/pages/index.astro`，手写布局（不走文档主题），9 个区块的 startup 叙事。
- `/docs/**` 文档站：Starlight（Astro）+ Markdown 内容集合，自带侧栏、全文搜索（Pagefind）、sitemap。
- 唯一的事实源：`src/config/site.ts`（对外链接、产品名、免责口径、主 CTA 落点 `appUrl`）。

## 构建

```bash
# 在仓库根执行（会先构建 CLI，因为文档里的参数表要实跑 --help）
make site

# 或者只在本目录里手动来一遍
cd site
pnpm install --frozen-lockfile
TONGSTOCK_BIN=/path/to/tongstock pnpm gen    # 生成端点表与 CLI 参考
pnpm build                                   # 产物在 dist/
node scripts/check-links.mjs                 # 死链 + 文档引用的 API 路径自检
```

::: 提示：本机若未关闭 Astro 遥测，首次 `pnpm build` 可能因写入 `~/Library/Preferences/astro` 失败；加 `ASTRO_TELEMETRY_DISABLED=1` 即可（`make site` 已带上）。
:::

## 哪些内容是生成的（不要手改）

| 文件 | 来源 | 生成器 |
| --- | --- | --- |
| `src/content/docs/docs/api/endpoints.md` | `api/openapi.json` | `scripts/gen-api-docs.mjs` |
| `src/data/generated.json` | openapi + 源码统计 + `--help` | 上面两个脚本 |
| `src/content/docs/docs/usage/cli-reference.md` | `tongstock <cmd> --help` 实跑输出 | `scripts/gen-cli-docs.mjs` |
| `public/og.png` | 内联 SVG 模板 | `scripts/gen-og.mjs`（可选，产物已提交） |

`pnpm gen:check` 只比对不写盘，用来在 CI 里抓“代码变了但文档没重新生成”。

## 文档里的链接怎么写

用**站内绝对路径**（`/docs/usage/cli/`），不要用相对路径：

Markdown 的相对链接按“源文件路径”解析，而 Astro 输出的是目录式 URL（`/docs/usage/cli/`），浏览器按 URL 解析会差一层目录。

部署到子路径时，`src/integrations/base-links.mjs` 会在构建后统一给站内绝对链接补前缀，不需要改 Markdown。

## 部署（A 方案：单 Pages 站点，官网占根）

`https://<owner>.github.io/<repo>/` = 官网 + 文档，`/<repo>/app/` = 现有 Web UI，根 `/<repo>/404.html` 负责深链分流。`.github/workflows/pages.yml` 是唯一相关的 workflow。

部署位置全部来自构建期变量（`site.ts` 只提供默认值），GitHub 侧用 repo variables 覆盖，完整的变量表（名称/默认值/作用/改哪里）见文档站 `docs/ops/deploy/` 与源码 `src/config/site.ts` 的注释：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `SITE_URL` | `https://<owner>.github.io` | 站点 origin（canonical / og:image / sitemap） |
| `SITE_BASE` | `/<repo>/` | 部署子路径，同时决定 Astro `base` 与 `VITE_BASE_PATH` |
| `APP_URL` | `<SITE_URL><SITE_BASE>app/` | 主 CTA 落点，可指向任意实例（含内网） |
| `REPO_URL` | `https://github.com/sjzsdu/tongstock` | 仓库及衍生链接的基址 |
| `ASTRO_TELEMETRY_DISABLED` | `1` | 关闭 Astro 遥测（CI 必需） |

换位置只改变量、不改代码：

```bash
SITE_URL=https://example.com SITE_BASE=/repo/ APP_URL=https://inner.local:8106/ pnpm build
```

`astro.config.mjs` 会拒绝「`SITE_URL` 带路径但 `SITE_BASE` 留空」这种自相矛盾配置。

### 本地复现 Pages 布局并验收深链

```bash
pnpm build                                                   # 需要 SITE_BASE=/tongstock/
cd ../web && VITE_BASE_PATH=/tongstock/app/ pnpm build && cd ../site
node scripts/assemble-pages.mjs --site-dist=dist --web-dist=../web/dist \
  --out=../pages-dist --base=/tongstock/ --app-path=app
node scripts/check-pages.mjs --root=../pages-dist --base=/tongstock/
```

`assemble-pages.mjs` 把 `site/dist` 放到根、`web/dist` 放进 `app/`、并生成根 `404.html`；`check-pages.mjs` 起一个**模拟 Pages 语义**的服务器（不存在路径 → 根 404.html + 状态 404），逐条断言官网首页、文档深链（带/不带尾斜杠）、`/app/`、`/app/stock/000001` 刷新、旧深链 `/stock/000001` 重定向，以及两端参数名/旧路由表不漂移。

## 自检脚本做了什么

- `scripts/check-links.mjs`：扫 `dist/` 里的 HTML 找死链；再扫 Markdown 里出现的 `/api/...`，确认它在生成的端点表里真实存在（防止文档提到一个仓库里没有的接口）。
- `scripts/check-theme.mjs`：主题与可读性自检（`pnpm check:theme`）。把 `dist/` 里的 CSS 过一遍真实的层叠规则（`@layer` / 特异性 / 顺序），分别解析出 dark 与 light 两套主题的变量，再核对「正文 / 链接 / 导航 / 侧栏 / 提示块」这几对色是否达到 WCAG AA；落地页另外检查页内调色板对每种底色的对比度，以及 grid 轨道有没有用裸 `fr`（会被长标识符撑破容器，窄屏整页横向滚动）。有 Chromium 时会再用它实际渲染落地页与两个文档页（1440 / 1024 / 390 三个宽度 × 两套主题），逐个可见文字算对比度、逐个元素查是否越出视口；没装浏览器就明确打印「跳过」，不假装通过。改动主题色板后先跑它，比肉眼看截图可靠。
- `scripts/gen-api-docs.mjs`：解析 openapi 的路径、参数（含 `$ref`）、请求体与响应码，按 10 个业务分组出表；同时统计落地页用的规模数字，取数规则写在脚本的 `STAT_RULES` 里。
- `scripts/gen-cli-docs.mjs`：递归遍历 `--help`，解析命令树与 flag（含默认值），生成 27 个顶层命令的参考页。找不到二进制会直接失败，不产出半份文档。

## 维护约定

1. **参数与端点禁止手写**，一律由脚本生成。
2. 手写文档只讲“为什么 / 怎么用 / 有什么坑”，字段级定义指向 `api/openapi.json`。
3. 不写收益率、胜率、准确率、用户量、客户 logo，也不做“实时”承诺；免责口径与仓库 README 一致，由 `src/config/site.ts` 单点定义。
