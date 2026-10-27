---
title: 概览
description: TongStock 解决什么问题、由哪几层组成、以及为什么“空榜单”是正常结果。
---

TongStock 围绕两个问题组织产品：**今天买什么**，以及**持仓什么时候卖**。

它不是一个“把行情数据摆出来让你自己看”的工具。数据只是原料，产品的输出是**带证据的决策**：候选股、买入窗口、仓位上限、退出计划，以及每条结论背后的方法、样本与验证口径。反过来，当验证通不过门槛时，它会明确地什么都不推荐。

## 三层结构

| 层 | 职责 | 代码位置 |
| --- | --- | --- |
| AI 层 | 提出和解释投资规律：把自然语言研究问题转成结构化候选 | `internal/agents`、`internal/agentservice` |
| 确定性引擎 | 编译、哈希版本化、冻结快照、回测、证据审查、每日选股、卖出判断 | `internal/methods`、`internal/selection`、`internal/backtest`、`internal/paradigms` |
| 数据层 | TDX 协议接入、本地 SQLite、DB-first 读取与同步水位 | `pkg/tdx`、`pkg/storage`、`internal/app/stockdata` |

关键分工是：**AI 不直接产生买卖建议**。它产出的是可被确定性代码编译和验证的候选；能不能进入“今日决策”，取决于验证结果。

## 四种使用方式

1. **Web UI** —— `./tongstock serve` 后打开浏览器，覆盖今日决策、选股方法、持仓卖出、个股详情、板块、资讯等 20 个页面。
2. **CLI** —— `quote`、`indicator`、`screen`、`block` 等轻量查询命令，加上 `method`、`select`、`position`、`automation` 完整的决策流水线。
3. **HTTP API** —— 上述能力全部有 REST 入口，契约见仓库 `api/openapi.json`，与 Gin 实际路由做双向比对。
4. **AI 助手（Skills）** —— `npx skills add sjzsdu/tongstock` 后，在支持 Skills 的助手里直接用自然语言调用上述能力。

四种方式共用同一个 DB-first 数据服务：先查本地 SQLite 与同步水位，数据缺失或过期时只从 TDX 同步缺失部分，再回读数据库。

## 为什么会出现空榜单

这是设计意图，不是故障。每次选股都要求方法在**真实、已冻结**的数据上通过验证；不满足就返回空列表，并说明被排除的原因（数据不足、证据等级不够、样本没通过门槛等）。系统宁可今天不推荐，也不会用今天的市值回填历史、不会把停牌当成没成交、不会把推断出来的持仓血缘伪装成原始方法。

## 下一步

- [安装](/docs/getting-started/install/) 并跑起来
- [首次运行与配置](/docs/getting-started/first-run/) 了解 `~/.tongstock/` 里都有什么
- [三步上手](/docs/getting-started/three-steps/) 走一遍主流程
