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

## 部署（尚未接线）

部署方案（单 Pages 站点 / 独立托管 / 子路径）留给人类拍板，本分支**不改任何 workflow**。构建时可用两个环境变量适配任意位置：

```bash
SITE_URL=https://example.com SITE_BASE=/tongstock pnpm build
```

## 自检脚本做了什么

- `scripts/check-links.mjs`：扫 `dist/` 里的 HTML 找死链；再扫 Markdown 里出现的 `/api/...`，确认它在生成的端点表里真实存在（防止文档提到一个仓库里没有的接口）。
- `scripts/gen-api-docs.mjs`：解析 openapi 的路径、参数（含 `$ref`）、请求体与响应码，按 10 个业务分组出表；同时统计落地页用的规模数字，取数规则写在脚本的 `STAT_RULES` 里。
- `scripts/gen-cli-docs.mjs`：递归遍历 `--help`，解析命令树与 flag（含默认值），生成 27 个顶层命令的参考页。找不到二进制会直接失败，不产出半份文档。

## 维护约定

1. **参数与端点禁止手写**，一律由脚本生成。
2. 手写文档只讲“为什么 / 怎么用 / 有什么坑”，字段级定义指向 `api/openapi.json`。
3. 不写收益率、胜率、准确率、用户量、客户 logo，也不做“实时”承诺；免责口径与仓库 README 一致，由 `src/config/site.ts` 单点定义。
