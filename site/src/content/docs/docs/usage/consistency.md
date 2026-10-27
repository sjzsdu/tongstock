---
title: 一致性模式
description: require_fresh / allow_stale / cache_only / refresh 四档语义，以及在 CLI 和 HTTP 里怎么用。
---

TongStock 的读取路径是 DB-first：先看本地 SQLite 里的数据与同步水位，结合当前日期、交易时段和交易日历判断新鲜度；缺什么就从 TDX 同步那部分，在事务里同时写入业务数据和水位，然后再从数据库读出来返回。

一致性模式就是这条路径上的开关。

## 四档语义

| 模式 | CLI | HTTP 参数 | 行为 |
| --- | --- | --- | --- |
| 要求新鲜 | `--consistency=require_fresh`（默认） | `consistency=require_fresh` | 必要时同步；同步失败返回错误，不拿旧数据糊过去 |
| 允许陈旧 | `--consistency=allow_stale` | `consistency=allow_stale` | 同步失败时允许返回数据库里的旧数据 |
| 只读本地 | `--consistency=cache_only` | `consistency=cache_only` | 完全不连接 TDX，只读数据库；缺数据即报错 |
| 强制刷新 | `--refresh` | `refresh=true` | 先从 TDX 刷新，再回读数据库 |

```bash
./tongstock quote 000001 --consistency=cache_only
./tongstock indicator --code 000001 --type day --refresh
```

```bash
curl "http://localhost:8106/api/quote?code=000001&consistency=allow_stale"
curl "http://localhost:8106/api/kline?code=000001&type=day&refresh=true"
```

## 组合约束

- `--refresh` **不能**与 `--consistency=cache_only` 同时使用：一个要求联网刷新，一个禁止联网，程序会直接报参数冲突。
- `cache_only` 下的部分命令会明确拒绝执行并提示改用本地计算或显式 `sync`，而不是静默降级。
- 非法的一致性取值会在命令启动时被拒绝，不会“忽略并走默认”。

## 何时用哪一档

| 场景 | 建议 |
| --- | --- |
| 盘中要看今天的行情 | `require_fresh`（默认），失败就让它失败，比看到滞后数据更安全 |
| 网络不稳、只想看已同步数据 | `allow_stale`，并注意返回数据的日期 |
| 离线分析、CI、无网的机器 | `cache_only`，前提是已经显式同步过 |
| 已知数据要更新，想一次刷干净 | `refresh=true` |
| 回测与验证 | 走冻结快照（`market freeze`），不用这三档来“凑”新鲜度 |

## 缓存未命中怎么办

`cache_only` 下遇到缺失数据，错误信息会直接告诉你下一步：

```
本地缓存缺少 000001 数据，请显式执行同步
（如 tongstock sync daily --codes 000001 --consistency=require_fresh），
或使用 --consistency=require_fresh --refresh 重试
```

这是刻意设计的：不静默联网，也不返回空结构。

## 与“新鲜度”有关的其它约定

- 判断新鲜度用的是**交易日历 + 当前时间 + 交易时段**，不是简单的“文件修改时间”。
- 日 K、行情快照、财务快照遵循 DB-first；股票代码、板块、F10 等低频数据使用同库中的 TTL 缓存表（股票代码与板块默认 24 小时）。
- 历史验证中没有“点时市值”序列时会 fail-closed，而不是拿今天的市值回填过去。
