---
title: TongStock 文档
description: TongStock 的安装、使用方式、HTTP API 契约、数据口径与部署运维文档。
template: splash
hero:
  tagline: 从安装到 HTTP 端点，从指标口径到部署运维。全部内容对应仓库里的真实实现。
  actions:
    - text: 安装并跑起来
      link: getting-started/install/
      icon: right-arrow
      variant: primary
    - text: CLI 命令参考
      link: usage/cli-reference/
      icon: document
---

TongStock 围绕两个问题组织产品：**今天买什么**，以及**持仓什么时候卖**。AI 负责提出和解释投资规律，确定性引擎使用真实 A 股数据完成编译、冻结、回测、证据审查、每日选股和卖出判断；没有足够数据或证据时，系统会拒绝推荐。

## 从这里开始

- [安装](/docs/getting-started/install/) —— Release 产物、源码编译、Docker 三条路径。
- [首次运行与配置](/docs/getting-started/first-run/) —— 默认端口、`~/.tongstock/` 目录、两份配置文件的职责。
- [三步上手](/docs/getting-started/three-steps/) —— 首页输入 → 今日决策 → 持仓卖出。

## 按使用方式

- [Web UI 指南](/docs/usage/web-ui/) —— 20 个页面各自负责什么。
- [CLI 指南](/docs/usage/cli/) —— 行情、指标、板块、公司信息的命令行用法。
- [CLI 命令与参数参考](/docs/usage/cli-reference/) —— 由 `--help` 实跑输出生成，不手写。
- [决策流水线](/docs/usage/decision-pipeline/) —— 方法编译、每日选股、持仓判断的完整链路。
- [一致性模式](/docs/usage/consistency/) —— `require_fresh` / `allow_stale` / `cache_only` / `refresh`。

## 给集成方

- [HTTP API 认证与安全](/docs/api/auth/) —— 令牌、监听地址与非本机访问的硬约束。
- [端点参考](/docs/api/endpoints/) —— 全部路由，由 `api/openapi.json` 生成。
- [AI 助手集成（Skills）](/docs/usage/skills/) —— `npx skills add sjzsdu/tongstock`。

## 数据与运维

- [数据来源](/docs/data/sources/)、[缓存与同步水位](/docs/data/cache-and-sync/)、[决策口径](/docs/data/decision-semantics/)。
- [Docker 部署](/docs/ops/deploy/)、[远程访问与令牌](/docs/ops/remote-access/)、[质量门禁](/docs/ops/quality-gates/)。

:::note[文档的边界]
本站描述行为时以仓库现状（`master` 分支）为准。HTTP 契约的唯一事实源是仓库里的 `api/openapi.json`，CLI 参数的事实源是二进制自身的 `--help` 输出——两者都由 `site/scripts/` 下的脚本同步到本站；若本站与它们不符，以它们为准。
:::
