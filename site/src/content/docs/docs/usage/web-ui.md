---
title: Web UI 指南
description: 20 个页面各自解决什么问题，以及主流程与高级研究入口的分工。
---

`./tongstock serve` 后打开 `http://127.0.0.1:8106/app/` 即用（根路径 `/` 是官网，`/docs/` 是文档）。页面由 React + TypeScript + Vite 构建，构建产物被嵌进 Go 二进制，所以运行时不依赖 Node。

![TongStock 个股详情页：行情概览与 K 线、技术指标](/shots/stock-detail.jpg)

*上图取自本地实例的真实截图：个股详情页（`/stock/:code`）的上半屏，含行情概览与 K 线、技术指标区。*

## 页面清单

下表是**应用内**路径；浏览器里实际地址是 `http://localhost:8106/app` + 该路径（例如 `/stock/:code` → `/app/stock/000001`）。旧地址（不带 `/app` 前缀）会被服务端 302 到新位置。

| 页面 | 路径 | 解决什么 |
| --- | --- | --- |
| 今日决策 | `/` | AI 研究输入、真实选股和持仓风险摘要 |
| 我的选股方法 | `/methods` | 一句话规则、历史验证口径、证据等级，并按方法筛选今日股票 |
| 高级研究与管理 | `/methods/advanced` | 自动研究、因子研究、方法比较、反馈与前向健康管理 |
| 自选股 | `/watchlist` | 分组自选股、备注与快速行情入口 |
| 股票选择 | `/stock/choose` | 搜索并进入个股详情 |
| 个股详情 | `/stock/:code` | 行情、K 线、指标、财务与资讯 |
| 信号筛选 | `/screen` | 批量筛选金叉/死叉/超买/超卖 |
| 持仓卖出 | `/portfolio` | `hold` / `watch` / `reduce` / `exit` 队列与执行约束 |
| 板块 | `/blocks` | 板块列表与成分股 |
| 指数详情 | `/index/:code` | 指数行情与 K 线（含上涨/下跌家数） |
| Agent | `/agent` | 股票 Agent 对话与诊断 |
| 投资范式（高级） | `/paradigms` | 范式分析、复盘与告警 |
| 范式监控（高级） | `/monitoring` | 范式前向表现：漂移/衰减/集中度评分、输入源诊断与预警确认 |
| 隔夜策略 | `/strategy/overnight` | 隔夜套利策略分析 |
| 新闻 | `/news` | 新闻流、热点事件与情绪 |
| 设置 | `/settings` | 指标参数等本地设置 |

另有方法选股结果页 `/methods/selection/:runId`、新闻事件详情 `/news/event/:id`、个股与指数详情下钻 `/:code/:tab`，以及 404 兜底。

## 主流程与高级入口的分工

导航按“普通用户主流程”和“高级研究与管理”划分，不是把所有页面平铺：

- **主流程**：今日决策 → 我的选股方法 → 持仓卖出，外加自选股、个股详情、行情工具。
- **高级研究**：自动研究、因子研究、方法比较、范式库与范式监控都从 `/methods/advanced` 进入。`/paradigms` 与 `/monitoring` 的路由仍然保留（可以直接访问、也可以被外部链接引用），只是不再占用主导航。
- **审计用途**：信号筛选等工具留在“行情工具”菜单里，用来事后核对，而不是日常选股入口。

## 页面和接口的对应关系

Web UI 只是 HTTP API 的一层视图，页面上看到的数据都能通过同名接口拿到：

| 页面 | 主要接口 |
| --- | --- |
| `/` | `/api/dashboard/today`、`/api/selections/today`、`/health*` |
| `/methods` | `/api/methods`、`/api/selections/*` |
| `/portfolio` | `/api/position-decisions` |
| `/screen` | `/api/screen` |
| `/stock/:code` | `/api/quote`、`/api/kline`、`/api/indicator`、`/api/finance`、`/api/news` |
| `/paradigms`、`/monitoring` | `/api/paradigm/*`、`/api/monitoring/*`、`/api/forward/*` |

完整路由表见[端点参考](/docs/api/endpoints/)。

## 前端开发的注意事项

如果你要改 Web UI，注意契约校验是强制门禁：

```bash
cd web
pnpm install --frozen-lockfile
pnpm dev        # 开发服务器，/api 代理到 vite.config.ts 里配置的目标（默认 localhost:8106）
pnpm api:check  # 校验生成的 DTO/操作表没有漂移
```

`pnpm build` 会执行 `tsc -b`，类型错误直接阻断构建；接口 DTO 由脚本从 `api/openapi.json` 生成，手改会在 `api:check` 时被发现。
