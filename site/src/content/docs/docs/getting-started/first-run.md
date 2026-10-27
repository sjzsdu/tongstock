---
title: 首次运行与配置
description: 启动后的访问入口（官网 / 文档 / Web UI / API）、默认端口、~/.tongstock/ 目录结构、两份配置文件各自的职责，以及环境变量覆盖规则。
---

第一次运行任何命令（包括 `./tongstock serve`）时，程序会自动创建 `~/.tongstock/` 并写入默认配置，不需要手动初始化。

## 启动后先打开哪里

`./tongstock serve` 一个进程同时提供官网、文档、Web UI 与 API，运行时不需要 Node（二者产物都 `//go:embed` 进了二进制）。

| 地址 | 内容 |
| --- | --- |
| `http://localhost:8106/` | **官网**（产品介绍、特性、文档入口） |
| `http://localhost:8106/docs/` | 使用文档（本站） |
| `http://localhost:8106/app/` | **Web UI**（今日决策、选股方法、持仓卖出等） |
| `http://localhost:8106/api/…` | REST 接口 |
| `http://localhost:8106/health` | 健康检查 |

:::caution[根路径已经从 Web UI 变成官网]
以前 `http://localhost:8106/` 打开就是 Web UI，现在根路径是官网，Web UI 在 `/app/` 下。
旧深链（如 `/stock/000519`、`/news`）会被服务端 **302 到 `/app/<原路径>`**，书签与外链不会失效；
但如果你在脚本里硬编码过 `http://host:8106/` + 页面路径，需要补上 `/app` 前缀。
:::

官网与静态资源保持公开；`/api` 在非本机访问时仍要求 Bearer 令牌（见[认证与安全](/docs/api/auth/)）。

## 目录结构

```
~/.tongstock/
├── config.yaml          # 应用主配置：服务、TDX 上游、缓存、数据库、可选的 Agent
├── indicator.yaml       # 指标参数：首次运行 indicator/screen 时生成
└── cache/
    └── tongstock.db     # SQLite 单库：业务数据 + 缓存 + read model + 同步水位
```

`cache/tongstock.db` 是一份统一的 SQLite 数据库，业务数据、TTL 缓存、股票 read model 和各类同步水位都在同一个文件里——这决定了备份、迁移、排查都只需要处理一个文件。

## 主配置 `config.yaml`

首次生成的内容包含（节选，注释保留在文件里）：

```yaml
server:
  port: 8106
  bind_address: 127.0.0.1
  # access_token: "please-change-me"

tdx:
  # hosts: 留空使用内置默认地址
  #   - "124.71.187.122:7709"

cache:
  backend: sqlite
  # dir: ~/.tongstock/cache

database:
  driver: sqlite3
  # dsn: ~/.tongstock/cache/tongstock.db
```

三条容易踩的规则：

1. **`bind_address` 保持 `127.0.0.1` 时不需要令牌**，服务只对本机可用。
2. **改成 `0.0.0.0` 或任何非回环地址时，`access_token` 必填**——没有配置令牌，服务会直接拒绝启动，而不是“先跑起来再说”。
3. **`driver` 只支持 `sqlite3`**，其余值不构成受支持的组合。

### 环境变量覆盖

三个服务端变量会覆盖配置文件里的对应项（供容器通过 compose 的 `environment`/secret 注入，密钥不落盘）：

| 变量 | 覆盖项 |
| --- | --- |
| `TONGSTOCK_SERVER_PORT` | `server.port`（非 1–65535 的整数会被忽略） |
| `TONGSTOCK_SERVER_BIND_ADDRESS` | `server.bind_address` |
| `TONGSTOCK_SERVER_ACCESS_TOKEN` | `server.access_token` |

优先级是：**环境变量 > `config.yaml` > 内置默认值**。

### 可选：Agent 配置

`config.yaml` 里还有一段被注释掉的 `agent` 配置，用于 `/agent` 页面与 Agent 相关接口使用的模型与角色。不配置也能运行，默认走内建 backend；模型、`api_key_env`、以及按场景声明的默认角色都写在段配置里，无需另外维护 PicoClaw 配置文件。历史 PicoClaw 的 `home/config` 仍然可用，会自动进入兼容模式。

## 指标参数 `indicator.yaml`

只在 `indicator` / `screen` 相关命令或页面首次计算时生成：

```yaml
defaults:
  ma: [5, 10, 20, 60]
  macd: { fast: 12, slow: 26, signal: 9 }
  kdj:  { n: 9, m1: 3, m2: 3 }
  boll: { n: 20, k: 2.0 }
  rsi:  [6, 14]
```

支持 `categories` 分组与 `overrides` 单股覆盖，优先级为 **per-stock override > category override > defaults**。完整说明见[指标口径](/docs/data/indicator-params/)。

## 换端口、换配置文件

```bash
# 临时指定另一份配置文件
./tongstock indicator --code 000001 --config configs/params.yaml

# 改端口：编辑 config.yaml 的 server.port，或用环境变量
TONGSTOCK_SERVER_PORT=6565 ./tongstock serve
```

端口以生效配置为准：内置默认是 `8106`，但如果你在 `config.yaml` 里写了别的值，一切地址都以那个值为准。

## 下一步

- [三步上手](/docs/getting-started/three-steps/) 走一遍主流程
- [远程访问与令牌](/docs/ops/remote-access/) 把服务暴露到其他机器
