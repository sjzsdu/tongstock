---
title: K 线类型
description: 十种 K 线周期参数的对照表，以及默认返回条数与计算窗口的区别。
---

`type` 参数取值如下（CLI 与 HTTP 通用）：

| `type` | 周期 | 备注 |
| --- | --- | --- |
| `1m` / `minute` | 1 分钟 | 两个写法等价 |
| `5m` | 5 分钟 | |
| `15m` | 15 分钟 | |
| `30m` | 30 分钟 | |
| `60m` | 60 分钟 | |
| `day` | 日 | **默认值** |
| `week` | 周 | |
| `month` | 月 | |
| `quarter` | 季 | |
| `year` | 年 | |

取值定义在 `pkg/tdx/service.go` 的 `ParseKlineType`：无法识别的字符串会退回 `day`，所以**拼错类型不会报错，只会静默变成日线**——写脚本时留意这一点。

## 默认返回条数

```bash
curl "http://localhost:8106/api/kline?code=000001&type=day"          # 只返回 100 根
curl "http://localhost:8106/api/kline?code=000001&type=day&count=500" # 显式要 500 根
```

不带 `count` 时默认 100 根。做长期统计（例如年均线、多年回撤）时必须显式指定，否则结论会建立在一个很短的窗口上。

## 计算窗口 ≠ 返回窗口

在指标场景里这两个概念容易混：

| 参数 | 控制什么 |
| --- | --- |
| `count`（CLI `--count`） | 参与**计算**的 K 线数量 |
| `days`（CLI `--days`） | **返回**多少天的结果 |

想要「用 500 根日线算出指标，但只给我最近 5 天」，两个都要给。

## 指数 K 线

指数用单独端点 `/api/index?code=...&type=...`，返回值额外带上涨/下跌家数。CLI 侧没有独立的指数命令，走 HTTP 或用 Web UI 的 `/index/:code` 页面。
