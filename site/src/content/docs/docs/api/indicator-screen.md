---
title: 指标与信号筛选
description: 技术指标端点、批量信号筛选，以及因子与发现相关的实验性入口。
---

端点清单见[端点参考](/docs/api/endpoints/#signals)。

## 技术指标

```bash
# 默认只返回最新一天
curl "http://localhost:8106/api/indicator?code=000001&type=day"

# 返回最近 5 天
curl "http://localhost:8106/api/indicator?code=000001&type=day&days=5"
```

返回结构包含价格、MA(5/10/20/60/120)、MACD(DIF/DEA/HIST)、KDJ(K/D/J)、BOLL(UPPER/MID/LOWER)、RSI(6/12/24)、量比，以及 `signals` 与 `summary`。

指标参数（周期、快慢线、覆盖优先级）来自 `~/.tongstock/indicator.yaml`，口径见[指标口径](/docs/data/indicator-params/)。

## 批量信号筛选

```bash
curl "http://localhost:8106/api/screen?codes=000001,600519&type=day&signal=golden_cross"
```

可用信号：`golden_cross`、`death_cross`、`overbought`、`oversold`。服务端会并发处理，逐个代码计算指标并匹配信号。

## 单股信号分析

```bash
curl "http://localhost:8106/api/signal-analysis?code=000001"
```

## 因子与发现（研究用）

这几个端点属于高级研究路径，产物会进入验证流水线，不适合当作即时结论：

| 端点 | 用途 |
| --- | --- |
| `/api/factors/*` | 因子相关计算与查询 |
| `/api/discover/*` | 从冻结历史中挖掘候选规律 |

它们的输出需要经过编译与验证才算“结论”，见[决策流水线](/docs/usage/decision-pipeline/)。

## 一处容易误解的地方

`/api/indicator` 的 `days` 控制**返回**多少天，不控制**计算窗口**。想让指标用更长历史计算，要在服务端配置或对应请求里调整参与计算的 K 线数量——这和 CLI 的 `--count` / `--days` 区别是同一件事。
