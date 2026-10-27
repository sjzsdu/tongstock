---
title: 范式、监控与前向跟踪
description: 范式库、前向运行与监控告警三组端点，以及它们和“方法”的区别。
---

端点清单见[端点参考](/docs/api/endpoints/#paradigm)。

## 三个概念先分清

| 概念 | 是什么 | 主要端点 |
| --- | --- | --- |
| 范式（paradigm） | 一类可复用的市场行为模式，可以复盘、评估、晋升为方法 | `/api/paradigm/*` |
| 前向运行（forward） | 方法或信号在被冻结之后的**真实前向表现**，含执行与结算 | `/api/forward/*` |
| 监控（monitoring） | 对前向表现的漂移、衰减、集中度做评分与告警 | `/api/monitoring/*` |

关系是：范式经过评估与晋升成为方法；方法上线后有前向运行；前向数据反过来喂给监控，触发衰减告警。

## 范式

```bash
curl "http://localhost:8106/api/paradigm/list"
curl "http://localhost:8106/api/paradigm/stats"
curl "http://localhost:8106/api/paradigm/stock/{code}"      # 个股相关的范式

curl -X POST "http://localhost:8106/api/paradigm/analyze"
curl -X POST "http://localhost:8106/api/paradigm/backtest"
curl -X POST "http://localhost:8106/api/paradigm/evaluate"

curl "http://localhost:8106/api/paradigm/{id}"
curl "http://localhost:8106/api/paradigm/{id}/evidence"
curl "http://localhost:8106/api/paradigm/{id}/review"
```

晋升为方法：

```bash
curl -X POST "http://localhost:8106/api/paradigm/{id}/promote"
curl "http://localhost:8106/api/paradigm/{id}/promotion/status"
```

## 前向运行

```bash
curl "http://localhost:8106/api/forward/runs"
curl "http://localhost:8106/api/forward/runs/{id}"
curl "http://localhost:8106/api/forward/runs/{id}/signals"
curl "http://localhost:8106/api/forward/runs/{id}/equity"
curl "http://localhost:8106/api/forward/runs/{id}/compare"
curl -X POST "http://localhost:8106/api/forward/runs/{id}/execute"
curl -X POST "http://localhost:8106/api/forward/runs/{id}/finalize"
```

`execute` 与 `finalize` 是状态推进：先执行（登记实际成交约束），再结算。这是“不假装已经成交”的落点——执行约束没满足时不会被静默跳过。

## 监控与告警

```bash
curl "http://localhost:8106/api/monitoring/health"
curl "http://localhost:8106/api/monitoring/report"
curl -X POST "http://localhost:8106/api/monitoring/report/refresh"
curl "http://localhost:8106/api/monitoring/config"
curl "http://localhost:8106/api/monitoring/alerts"
curl -X POST "http://localhost:8106/api/monitoring/alerts/{id}/ack"
curl -X POST "http://localhost:8106/api/monitoring/alerts/{id}/resolve"
```

告警需要显式 `ack` 与 `resolve`，避免“刷新一下就自动消失”的假修复。

## 在 Web UI 里的位置

这三组端点对应 `/paradigms` 与 `/monitoring` 两个页面，都从「高级研究与管理」（`/methods/advanced`）进入，不再占用主导航。
