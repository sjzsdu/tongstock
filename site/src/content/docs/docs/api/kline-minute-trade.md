---
title: K 线、分时与分笔
description: K 线的十种周期、当日与历史分时、分笔成交，以及除权除息数据。
---

端点清单见[端点参考](/docs/api/endpoints/#bars)。

## K 线

```bash
# 全量：不带窗口参数时返回库内该代码的全部历史
curl "http://localhost:8106/api/kline?code=000001&type=day"

# 窗口：start 与 count 必须同时给，缺一个就会退回全量
curl "http://localhost:8106/api/kline?code=000001&type=day&start=0&count=200"
```

`type` 取值见[K 线类型](/docs/data/kline-types/)。

:::caution[`start` 与 `count` 必须成对出现]
服务端只在**两个参数都提供**时才走窗口分支；只给 `count`（或只给 `start`）会被忽略，直接返回库内全部历史。

实测（`000001`，日线，库内约 8400 根）：不带参数 → 全部；只带 `count=5` → 仍是全部；带 `start=0&count=5` → 5 根。
:::

:::note[契约缺口]
`api/openapi.json` 目前只声明了 `code` / `type` / `consistency` / `refresh` 四个参数，没有声明 `start` / `count`——这两个参数在实现里有效但未进契约。以契约为准的代码生成器看不到它们，属于已知缺口。
:::

## 分时

```bash
# 当日
curl "http://localhost:8106/api/minute?code=000001"

# 历史某天
curl "http://localhost:8106/api/minute?code=000001&history=true&date=20250314"
```

## 分笔成交

```bash
# 当日
curl "http://localhost:8106/api/trade?code=000001"

# 历史某天
curl "http://localhost:8106/api/trade?code=000001&history=true&date=20240315"
```

`/api/trades` 是批量版本。

分笔与历史分时依赖 TDX 上游的对应数据窗口；当天之外的数据可能不可得，缺失时按一致性模式处理（`require_fresh` 报错，`allow_stale` 返回旧数据）。

## 除权除息

```bash
curl "http://localhost:8106/api/xdxr?code=000001"
```

返回分红、送股、配股、股本变动等历史记录。做任何跨年的价格比较前都应该先看这份数据——否则复权口径会出错。

## 常见坑

| 现象 | 原因 |
| --- | --- |
| 只想取最近 N 根却拿到全量 | 只给了 `count`，没同时给 `start`；窗口分支要求两个参数都在 |
| 历史分时为空 | 该日期上游不提供，或本地未同步 |
| 价格序列跳空 | 除权除息未复权，需对照 `/api/xdxr` |
