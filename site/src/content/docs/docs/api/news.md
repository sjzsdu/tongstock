---
title: 资讯
description: 新闻流、热点事件、个股资讯与情绪相关端点。
---

端点清单见[端点参考](/docs/api/endpoints/#news)。资讯来自九个公开源（见[数据来源](/docs/data/sources/)），聚合后按个股做实体识别。

## 新闻流

```bash
curl "http://localhost:8106/api/news/feed"
curl "http://localhost:8106/api/news/feed/sources"    # 有哪些源
curl "http://localhost:8106/api/news/feed/facets"     # 分面统计
curl "http://localhost:8106/api/news/search?q=关键词"
```

## 个股资讯

```bash
curl "http://localhost:8106/api/news/stock/000001"
curl "http://localhost:8106/api/news/item/{id}"
```

## 热点事件

```bash
curl "http://localhost:8106/api/news/events"
curl "http://localhost:8106/api/news/events/{id}"
curl "http://localhost:8106/api/news/topics"           # 当日热门股票
curl -X POST "http://localhost:8106/api/news/events/refresh"
```

## 情绪

```bash
curl "http://localhost:8106/api/news/sentiment/market"
curl "http://localhost:8106/api/news/sentiment/stock/000001"
curl "http://localhost:8106/api/news/sentiment/trend"
curl "http://localhost:8106/api/news/sentiment/heatmap"
```

情绪是**对文本的聚合打分**，不是价格预测。用它做筛选可以，用它做结论要小心。

## 提醒规则

资讯模块自带一套提醒规则与未读管理：

```bash
curl "http://localhost:8106/api/news/alerts"
curl "http://localhost:8106/api/news/alerts/rules"
curl "http://localhost:8106/api/news/alerts/unread"
curl "http://localhost:8106/api/news/alerts/count"
curl -X POST "http://localhost:8106/api/news/alerts/read-all"
```

## 抓取控制

```bash
curl -X POST "http://localhost:8106/api/news/fetch"
curl -X POST "http://localhost:8106/api/news/fetch/browser"
```

`fetch` 是常规抓取；`fetch/browser` 走浏览器渲染路径，用于常规抓取拿不到内容的情况——代价是更慢，不建议随时调用。

:::caution[抓取频率]
公开源对高频请求会限流（常见表现是 403 或返回空）。需要批量收集时请拉长间隔，或先用本地数据库里已有的内容。
:::

## 在 Web UI 里

`/news` 是新闻流与热点；`/news/event/:id` 是事件详情。CLI 侧对应 `news` 与 `topic` 两个命令。
