---
title: 财务、公司与板块
description: 财务指标、F10 公司信息按块读取，以及板块分类的查询方式。
---

端点清单见[端点参考](/docs/api/endpoints/#company)。

## 财务

```bash
curl "http://localhost:8106/api/finance?code=000001"
```

返回总股本、流通股、净资产、净利润等核心指标。

:::caution[单位是千元]
财务数值单位是**千元**。例如净利润 `11691539` 表示约 116.9 亿元。任何二次计算（估值、增长率）之前先确认量纲，否则会差三个数量级。
:::

## 公司信息（F10）

F10 资料按“块”组织，两步读取：

```bash
# 第一步：拿目录（有哪些块）
curl "http://localhost:8106/api/company?code=000001"

# 第二步：按块名取内容（推荐）
curl "http://localhost:8106/api/company/content?code=000001&block=公司概况"
```

`block` 参数取 `/api/company` 返回的 `Name` 字段。

旧式调用仍兼容，用字节窗口读取：

```bash
curl "http://localhost:8106/api/company/content?code=000001&filename=000001.txt&start=0&length=10000"
```

`start` / `length` 的单位是 **GBK 字节**，不是字符——按字符算会读到半个汉字。

:::note[按块名读取的已知限制]
部分块的 `block` 参数会返回同一份内容（历史上观察到多数取值落到“股东研究”）。如果发现取到的块和预期不符，改用 `filename` + `start` 的字节窗口方式定位，偏移量从 `/api/company` 的目录信息推算。
:::

## 板块

板块查询同时出现在“行情与基础数据”和这里，因为它既是行情维度也是分类维度：

```bash
curl "http://localhost:8106/api/block/files"
curl "http://localhost:8106/api/block/list?file=block_fg.dat&sort=true"
curl "http://localhost:8106/api/block/show?name=银行&file=block_fg.dat"
```

四个板块文件的分工见[数据来源](/docs/data/sources/)。
