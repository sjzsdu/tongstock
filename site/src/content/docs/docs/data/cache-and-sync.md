---
title: 缓存与同步水位
description: 一份 SQLite 数据库如何同时承载业务数据、缓存与同步水位，以及手动同步该怎么做。
---

## 一份数据库，四个角色

`~/.tongstock/cache/tongstock.db` 这一个文件里同时装着：

1. **业务数据** —— 日 K、行情快照、财务快照等
2. **TTL 缓存** —— 股票代码、板块、F10 等低频数据（股票代码与板块默认 24 小时）
3. **read model** —— 给 Web UI 与搜索用的读取模型（例如股票搜索索引）
4. **同步水位** —— 每个代码、每类数据已经同步到哪一天

放在一起的好处是：备份、迁移、排查都只处理一个文件，且事务可以同时更新业务数据与水位，不会出现“数据写进去了但水位没更新”的半截状态。

## 读取路径（DB-first）

```
请求 → 查本地数据 + 水位
      → 用交易日历 + 当前时间 + 交易时段判断新鲜度
      → 新鲜：直接返回
      → 不新鲜/缺失：只向 TDX 同步缺失的那部分
      → 在事务中写入业务数据 + 水位
      → 重新从数据库读出并返回
```

“只同步缺失部分”是重点：不会为了查一只股票的今天数据把三年历史重拉一遍。

## 手动同步

正常情况下不需要手动同步，读取路径会自己补。需要显式同步的场景是：离线准备数据、或者想在 `cache_only` 模式下工作。

```bash
# 同步指定代码的日 K
./tongstock sync daily --codes 000001,600519

# 同步模式
./tongstock sync daily --codes 000001 --mode incremental   # 只补增量
./tongstock sync daily --codes 000001 --mode full          # 全量重拉
./tongstock sync daily --codes 000001 --mode auto          # 默认，按水位判断
```

HTTP 侧的同步入口在 `/api/sync/*`，健康与诊断在 `/health/data-sync`。

## 与一致性模式的配合

| 你的目标 | 做法 |
| --- | --- |
| 正常使用 | 什么都不用做（`require_fresh` 默认） |
| 离线分析 | 先 `sync daily`，再用 `--consistency=cache_only` |
| 数据可疑 | `--refresh` 强制刷新后回读 |
| 上游不稳 | `--consistency=allow_stale`，并注意返回数据日期 |

语义细节见[一致性模式](/docs/usage/consistency/)。

## 时间口径

新鲜度判断用的不是“文件修改时间”，而是**交易日历 + 当前时间 + 交易时段**：非交易日、收盘后、开盘前对“今天的数据是否应该存在”的判断都不同。

历史验证里如果缺少某个时点的市值序列，系统会 fail-closed（明确失败），而不是拿今天的市值回填过去。

## 备份与迁移

```bash
# 备份（服务停机时复制即可；运行中也请用 sqlite 的在线备份）
cp ~/.tongstock/cache/tongstock.db ~/tongstock-backup-$(date +%F).db
```

换机器时把 `~/.tongstock/` 整个目录带过去即可：配置、指标参数、数据库都在里面。
