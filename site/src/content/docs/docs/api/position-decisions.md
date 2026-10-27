---
title: 持仓决策
description: 基于真实持仓与冻结快照生成 hold / watch / reduce / exit 判断的端点。
---

端点清单见[端点参考](/docs/api/endpoints/#position)。

## 端点

```bash
curl -X POST "http://localhost:8106/api/position-decisions/run"
curl "http://localhost:8106/api/position-decisions/today"
curl "http://localhost:8106/api/position-decisions/runs/{id}"
```

`run` 生成一次**不可变**的决策；`today` 是当天最新视图；`runs/{id}` 用于事后核对。

## 输出语义

| 动作 | 含义 |
| --- | --- |
| `hold` | 继续持有 |
| `watch` | 观察，条件临近但未触发 |
| `reduce` | 减仓 |
| `exit` | 退出 |

每个决策都带执行约束。两类必须如实暴露的情况：

1. **血缘缺失** —— 持仓没有原始方法记录时标为 `inferred`（推断），而不是假装它来自某个方法。
2. **无法执行** —— 停牌、T+1 限制下不会报告“已卖出”，而是把约束单独列出。

这两条是这套系统可信度的核心：宁可显示“无法判断/无法执行”，也不给一个看起来干净的假答案。

## 与选股的关系

选股（`/api/selections/*`）回答“买什么”，持仓决策回答“什么时候卖”，两者共用同一个冻结快照与同一批方法。所以：

```
同一个 snapshot → 候选来自 select，退出条件来自方法自带的退出计划
```

这也是为什么持仓决策需要快照 id：没有冻结数据就没有可复核的判断。

## 设置

指标参数可以通过 `/api/settings/indicator` 读取与更新，对应 Web UI 的 `/settings` 页面。
