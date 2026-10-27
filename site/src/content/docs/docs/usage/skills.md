---
title: AI 助手集成（Skills）
description: 用 npx skills add 把 TongStock 的能力装进支持 Skills 的 AI 助手，以及两个 skill 各自负责什么。
---

TongStock 的两份 Skills 定义随仓库发布在 `skills/` 目录下，可以直接装进支持 Skills 的编码助手，不需要自己写提示词。

## 安装

```bash
npx skills add sjzsdu/tongstock
```

装好之后，助手会在合适的时候自动触发对应 skill。前提是 TongStock 服务或 CLI 可用：

- 只用 CLI 也能工作（`tongstock` 在 `PATH` 里即可）；
- 要用 HTTP 方式，需要先 `./tongstock serve`，默认端口 `8106`。

## 两个 skill 的分工

| Skill | 定位 | 典型触发 |
| --- | --- | --- |
| `tongstock-cli` | CLI 与 HTTP API 的**参考手册**：命令、参数、字段含义 | “查一下 000001 的 K 线”“这个接口怎么传参” |
| `tongstock-workflow` | **多步分析流程**：把若干数据源组合成一份可交付的结论 | “分析一下 600519 的基本面和技术面”“筛出今天出现金叉的股票” |

### `tongstock-cli`

覆盖：行情、代码列表、K 线、分时、分笔、除权除息、财务、板块、F10、技术指标、信号筛选、证券数量、集合竞价、个股新闻，以及对应的 HTTP 端点。

当你不确定某个字段是什么意思（比如财务数据的单位）、某个参数怎么传时，助手会查这个 skill 而不是猜。

:::caution[以二进制为准]
`skills/` 里的命令表是随仓库维护的文档。如果你的本地二进制版本与此不一致，**以 `tongstock <cmd> --help` 的输出为准**——本站的 [CLI 命令与参数参考](/docs/usage/cli-reference/)也是这么生成的。
:::

### `tongstock-workflow`

预置了多套分析流程，例如：单股深度分析、基本面选股、分红分析、板块分析、技术面速查、大盘概览、批量信号筛选、个股新闻、热门股票等。

这些流程的价值在于**组合**：单股深度分析会依次取行情、财务、除权除息、日 K、F10、本地股票信息，再综合成报告；而不是只回答一个字段。

## 一个可复现的对话例子

```text
你：分析 600519 最近的技术面和基本面
助手：（调用 tongstock quote / indicator / finance / xdxr / company 等多步）
     → 输出价格位置、均线与 MACD 状态、财务指标、分红历史，并注明数据日期
```

关键是每一条结论都带数据日期和来源字段——这和 Web UI 里“候选必须带证据”是同一条原则。

## 常见问题

**装完没反应？**
先确认 `tongstock` 在 `PATH` 里：`tongstock --help`。没有二进制的话，助手无法执行命令。

**想让助手走 HTTP 而不是 CLI？**
先起服务并确认 `curl http://127.0.0.1:8106/health` 返回 200。远程实例还要带令牌，见[远程访问与令牌](/docs/ops/remote-access/)。

**财务数据的单位是什么？**
千元。例如净利润 `11691539` 表示约 116.9 亿元。这类量纲问题在 `tongstock-cli` skill 里有说明，也值得在任何二次计算前先确认。
