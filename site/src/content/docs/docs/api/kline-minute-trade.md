---
title: K 线、分时与分笔
description: K 线的十种周期、当日与历史分时、分笔成交，以及除权除息数据。
---

端点清单见[端点参考](/docs/api/endpoints/#bars)。

## K 线

```bash
curl "http://localhost:8106/api/kline?code=000001&type=day"
curl "http://localhost:8106/api/kline?code=000001&type=60m&count=200"
```

`type` 取值见[K 线类型](/docs/data/kline-types/)；`count` 与 `start` 控制返回区间。

:::note[默认只返回 100 根]
不带 `count` 时 K 线默认只返回 100 根。要更长历史必须显式传 `count`。
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
| 只拿到 100 根 | 没传 `count` |
| 历史分时为空 | 该日期上游不提供，或本地未同步 |
| 价格序列跳空 | 除权除息未复权，需对照 `/api/xdxr` |
