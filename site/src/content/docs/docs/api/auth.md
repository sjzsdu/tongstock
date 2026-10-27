---
title: 认证与安全
description: 默认只监听回环地址、非本机访问必须配令牌，以及浏览器端 401 的处理流程。
---

HTTP API 的安全模型只有两条规则，但都是硬约束。

## 规则一：默认只对本机开放

`server.bind_address` 默认是 `127.0.0.1`，服务只监听回环地址。此时不需要令牌。

```yaml
server:
  port: 8106
  bind_address: 127.0.0.1
```

## 规则二：非回环地址必须配令牌，否则拒绝启动

把监听地址改成 `0.0.0.0` 或任何非回环地址时，**必须同时配置 `access_token`**：

```yaml
server:
  port: 8106
  bind_address: 0.0.0.0
  access_token: "替换为足够长的随机令牌"
```

没有配置令牌时，服务不会“先跑起来再警告”，而是直接退出。这是为了避免一个“以为只有自己能访问”的裸奔实例。

容器部署可以用环境变量注入，密钥不落盘：

```bash
TONGSTOCK_SERVER_BIND_ADDRESS=0.0.0.0 \
TONGSTOCK_SERVER_ACCESS_TOKEN=... \
./tongstock serve
```

## 请求怎么带令牌

通过 `Authorization` Header 传 Bearer 令牌，**不要**放进 URL：

```bash
curl -H "Authorization: Bearer $TONGSTOCK_ACCESS_TOKEN" \
  "http://server-host:8106/api/quote?code=000001"
```

放在 URL 里会被代理日志、浏览器历史、Referer 记录下来。

## 哪些路径需要令牌

| 路径 | 是否需要令牌 |
| --- | --- |
| `/api/**` | 是（全部） |
| `/health`、`/health/*` | 否 |
| SPA 静态资源 | 否 |

`/health` 保持公开是为了让反向代理和容器探针能工作；它不返回业务数据。

## 浏览器端：第一次 401 之后

远程打开 Web UI 时，页面第一次收到 401 会弹输入框让你填 Access Token：

- 令牌只保存在浏览器 `localStorage`；
- 之后所有 API 请求和 Agent 的 SSE 请求都通过 `Authorization` Header 携带；
- 不会写进 URL，也不会存到服务端。

换机器或清空浏览器数据后需要重新输入。

## 反向代理场景

TLS 终止、域名、以及把令牌从外部 Header 透传进来的做法见[远程访问与令牌](/docs/ops/remote-access/)。

## 常见误判

| 现象 | 原因 |
| --- | --- |
| 服务启动即退出，日志提到 access_token | `bind_address` 不是回环地址但没配令牌 |
| `curl` 返回 401 | 没带 `Authorization`，或令牌与服务端配置不一致 |
| 浏览器能打开页面但功能全空 | 页面拿到了静态资源（公开），但 `/api` 请求被 401 拦下 |
| 从另一台机器访问超时 | 服务仍监听 `127.0.0.1`，需要改 `bind_address` |
