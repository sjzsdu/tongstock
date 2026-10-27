---
title: 行情与基础数据
description: 行情快照、代码列表、证券数量、集合竞价、板块、指数与自选股相关端点。
---

这一组是最“传统”的部分：给你数据，不替你判断。全部端点见[端点参考](/docs/api/endpoints/#market)。

## 行情

```bash
# 单只
curl "http://localhost:8106/api/quote?code=000001"

# 批量
curl "http://localhost:8106/api/quotes?codes=000001,600519"
```

返回五档买卖盘、昨收价、内外盘、成交量与成交额。数据来自 DB-first 读取路径：本地有新鲜数据直接返回，缺了才去 TDX 同步。

## 代码与数量

```bash
curl "http://localhost:8106/api/codes/list?exchange=sz&category=stock"
curl "http://localhost:8106/api/codes/stats?all=true"
curl "http://localhost:8106/api/count?exchange=sh"
curl "http://localhost:8106/api/stocks/search-index"   # 前端搜索用的索引
```

分类取值为 `all` / `stock` / `gem` / `fund` / `etf` / `bond` / `index`。代码列表与板块属于低频数据，走同库的 TTL 缓存（默认 24 小时）。

## 集合竞价

```bash
curl "http://localhost:8106/api/auction?code=000001"
```

## 板块

```bash
curl "http://localhost:8106/api/block/files"                        # 有哪些板块文件
curl "http://localhost:8106/api/block/list?file=block_zs.dat&type=2&sort=true"
curl "http://localhost:8106/api/block/show?name=沪深300&file=block_zs.dat"
curl "http://localhost:8106/api/block/show?code=600519"             # 反查归属
```

## 指数

```bash
curl "http://localhost:8106/api/index?code=999999&type=day"
```

指数 K 线额外带上涨/下跌家数。

## 自选股与股票池

```bash
curl "http://localhost:8106/api/watchlist"
curl "http://localhost:8106/api/watchlist/groups"
curl "http://localhost:8106/api/stockpool"
```

这两组是**写操作**较多的端点（增删自选、分组、备注、自定义股票池），请求体是 JSON，具体字段见 `api/openapi.json` 的对应 schema。

## 其它

- `/api/history` —— 查询历史记录
- `/api/stockinfo/{code}` —— 本地库里的股票基础信息
- `/api/sync/*` —— 显式触发同步（见[缓存与同步水位](/docs/data/cache-and-sync/)）

## 一致性参数

这一组端点普遍接受：

| 参数 | 作用 |
| --- | --- |
| `consistency=require_fresh` | 默认，必要时同步，失败即报错 |
| `consistency=allow_stale` | 同步失败时返回旧数据 |
| `consistency=cache_only` | 只读数据库 |
| `refresh=true` | 强制刷新后回读 |

语义见[一致性模式](/docs/usage/consistency/)。
