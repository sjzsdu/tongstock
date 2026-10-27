---
title: 远程访问与令牌
description: 把服务暴露到本机之外时必须做的事：监听地址、令牌、反向代理与探针。
---

## 最短路径

三步，缺一不可：

1. 监听地址改成非回环：`bind_address: 0.0.0.0`（或具体网卡地址）
2. 配置 `access_token`（足够长的随机串）——否则**服务拒绝启动**
3. 客户端通过 `Authorization: Bearer <token>` 访问

```yaml
server:
  port: 8106
  bind_address: 0.0.0.0
  access_token: "用 openssl rand -hex 32 生成"
```

```bash
TOKEN="<你的令牌>"
curl -H "Authorization: Bearer $TOKEN" "http://server-host:8106/api/quote?code=000001"
curl "http://server-host:8106/health"     # 健康检查无需令牌
```

## 为什么令牌不能放 URL

`?access_token=...` 这类写法会留在代理访问日志、浏览器历史、`Referer` 里。TongStock 只接受 Header 携带，且浏览器端令牌只存在 `localStorage`。

## 反向代理示例（通用写法）

下面是形状示例，不是可直接复制的生产配置——按你自己的域名、证书与路径调整：

```nginx
server {
  listen 443 ssl;
  server_name tongstock.example.com;

  location / {
    proxy_pass http://127.0.0.1:8106;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # Agent 的 SSE 流式响应需要关闭缓冲
    proxy_buffering off;
    proxy_read_timeout 3600s;
  }
}
```

两个关键点：

- **SSE 必须关闭代理缓冲**，否则 Agent 对话会“一次性吐出”或者干脆超时；
- 令牌由客户端 Header 直接透传，代理不需要（也不应该）改写它。

## 健康检查

| 端点 | 用途 |
| --- | --- |
| `/health` | 基础健康检查，公开 |
| `/health/live` | 存活探针 |
| `/health/ready` | 就绪探针 |
| `/health/data-sync` | 数据同步状态 |
| `/health/diagnostics` | 诊断信息 |

容器编排里用 `/health/live` 与 `/health/ready` 更准确：前者只判断进程活着，后者判断依赖就绪。

## 浏览器访问

远程打开 Web UI 时，第一次 401 会弹框让你输入令牌：

- 令牌只保存在该浏览器的 `localStorage`；
- 之后 `/api` 请求与 Agent 的 SSE 请求都自动带 `Authorization`；
- 换浏览器或清空站点数据后需要重新输入。

## 排查表

| 现象 | 检查项 |
| --- | --- |
| 服务启动即退出 | 是否配了 `access_token`；日志里会有明确提示 |
| 502 / 连接被拒 | 容器或服务是否监听 `0.0.0.0`；代理 upstream 端口对不对 |
| 401 | Header 是否带上；令牌是否与配置一致 |
| Agent 对话卡住 | 代理是否关闭了缓冲、`read_timeout` 是否太短 |
| 页面能开但功能全空 | 静态资源公开、`/api` 被拦——检查浏览器控制台的 401 |
