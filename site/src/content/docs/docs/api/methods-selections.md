---
title: 方法与选股
description: 方法库、研究运行、每日选股与自动化闭环的端点。
---

端点清单见[端点参考](/docs/api/endpoints/#methods)。这一组是整个产品最核心的部分，语义比行情端点复杂，建议先读[决策流水线](/docs/usage/decision-pipeline/)。

## 方法库

```bash
curl "http://localhost:8106/api/methods"                 # 列表
curl "http://localhost:8106/api/methods/{id}"            # 单个方法详情
curl "http://localhost:8106/api/methods/{id}/audit"      # 审计信息
curl "http://localhost:8106/api/methods/{id}/feedback"   # 反馈记录
curl "http://localhost:8106/api/methods/{id}/pool"       # 该方法的结构化股票池
```

每个方法是**编译后的稳定结构**，用哈希标识。方法自带验证口径（观察窗口、目标幅度、价格基准、成功条件），不共享全局默认值。

其它与「方法整体状态」相关的端点：

| 端点 | 用途 |
| --- | --- |
| `/api/methods/forward-health` | 方法的前向健康度 |
| `/api/methods/reject-stats` | 被拒绝的候选统计（为什么没进榜） |
| `/api/method-families/{id}` | 方法族视图 |

## 自动研究

```bash
curl -X POST "http://localhost:8106/api/methods/research/run"
curl "http://localhost:8106/api/methods/research/status"
curl "http://localhost:8106/api/methods/research/last"
curl "http://localhost:8106/api/methods/research/traces/{id}/summary"
```

这条链路对应 CLI 的 `method research` 与 `discover`：从真实历史里提出候选，产物进入编译与验证，不直接变成推荐。

## 每日选股

```bash
curl -X POST "http://localhost:8106/api/selections/run"
curl "http://localhost:8106/api/selections/today"
curl "http://localhost:8106/api/selections/runs"
curl "http://localhost:8106/api/selections/runs/{id}"
```

一次 `run` 是**不可变**的运行：事后查看 `runs/{id}` 得到的内容与当天完全一致。

## 首页聚合

```bash
curl "http://localhost:8106/api/dashboard/today"
```

Web UI 首页（`/`）就是它的视图：今日候选、持仓风险摘要、系统健康与工作日志都从这一个端点来。

## 自动化闭环

```bash
curl -X POST "http://localhost:8106/api/automation/run"
curl "http://localhost:8106/api/automation/jobs"
curl "http://localhost:8106/api/automation/outbox"
```

对应 CLI 的 `automation run`：幂等地把选股、持仓判断、提醒串成一天的一次执行。`outbox` 是待发出的提醒队列。

## 引导流程

```bash
curl -X POST "http://localhost:8106/api/onboarding/run"
```

首次配置的引导任务，对应 Web UI 的引导向导。

## 一个阅读顺序建议

如果把这一组端点当 REST 随便调，很容易误以为“调 `selections/run` 就该有结果”。实际顺序是：

```
market 快照就绪 → 方法已通过验证 → selections/run → dashboard/today 才有内容
```

没有候选不是接口坏了，是门槛没过。
