---
title: FAQ
description: 端口、K 线条数、指标参数、F10 读取、财务单位——实际使用中最常撞到的几件事。
---

## 端口到底是哪个？

内置默认 `8106`（`pkg/config/config.go` 的 `DefaultConfig`）。但**以 `~/.tongstock/config.yaml` 的 `server.port` 为准**：如果你在那里写了 `6565`，服务就在 `http://localhost:6565`。

也可以用 `TONGSTOCK_SERVER_PORT=6565 ./tongstock serve` 临时覆盖。

## K 线一次返回多少根？

取决于是否给窗口参数：

```bash
# 不带 start 与 count：返回库内该代码的全部历史
curl "http://localhost:8106/api/kline?code=000001&type=day"

# 窗口模式：start 与 count 必须同时给
curl "http://localhost:8106/api/kline?code=000001&type=day&start=0&count=500"
```

只给 `count` 不给 `start`（或反过来）会退回全量，不会截断。实测 `000001` 日线：不带参数与只带 `count=5` 都返回约 8400 根，`start=0&count=5` 返回 5 根。

CLI 侧不一样：`./tongstock kline` 的 `--count` 默认 500，`--all` 取全部历史。

## `--days` 和 `--count` 有什么区别？

- `--count`：参与**计算**的 K 线数量（`indicator` 默认 250）
- `--days`：JSON 输出里**返回**多少天（默认 1）

想要“用长历史算、只要最近几天”，两个都给。

## 指标参数改了为什么不生效？

三层覆盖的优先级是 `per-stock override > category override > defaults`。先确认改的那一层没有被更高优先级覆盖；再确认改的是 `~/.tongstock/indicator.yaml`（不是仓库里的 `configs/params.yaml`）。临时用别的参数文件可以 `--config` 指定。

另外：改参数只影响之后新算的指标，不会改写已冻结的快照与已完成的方法验证。

## F10 的 `--block` 取到的内容不对？

历史上观察到多数 `block` 取值会落到同一份内容（常见是“股东研究”）。此时改用字节窗口方式定位：

```bash
curl "http://localhost:8106/api/company?code=000001"                       # 先拿目录
curl "http://localhost:8106/api/company/content?code=000001&filename=000001.txt&start=0&length=10000"
```

`start` / `length` 的单位是 **GBK 字节**，按字符算会切到半个汉字。

## 财务数据的单位是什么？

**千元**。净利润 `11691539` 表示约 116.9 亿元。做估值或增长率计算前先确认量纲，否则会差三个数量级。

## 数据存在哪？怎么备份？

`~/.tongstock/cache/tongstock.db`——一个 SQLite 文件同时装着业务数据、缓存、read model 与同步水位。备份它就是备份了全部数据；配置在 `~/.tongstock/config.yaml` 与 `indicator.yaml`。

## 服务起不来，日志说 access_token？

你把 `bind_address` 改成了非回环地址却没配 `access_token`。这是刻意的：不允许在“以为只有自己能访问”的状态下裸奔。见[远程访问与令牌](/docs/ops/remote-access/)。

## 页面能打开，但功能全是空的？

静态资源是公开的，`/api` 全部需要令牌。远程访问时第一次 401 会弹框输入令牌；如果你在反代后仍拿不到数据，检查 Header 是否被代理丢掉。

## 为什么今天没有推荐？

这是正常结果。没有方法通过门槛时返回空榜单，并给出被排除原因分布。见[决策口径](/docs/data/decision-semantics/)——**空榜是结论，不是故障**。

## 新闻抓不到内容 / 返回 403？

公开源对高频请求会限流（如搜狗、部分财经站点）。拉长间隔，或先用数据库里已有的内容；`/api/news/fetch/browser` 走浏览器渲染路径，能拿到更多但更慢。

## 我装的是旧版本，某个命令不存在？

以二进制的实际输出为准：

```bash
tongstock --help
```

本站的[CLI 参考](/docs/usage/cli-reference/)由当前 `master` 构建的二进制 `--help` 生成，版本不一致时以你本地的为准。
