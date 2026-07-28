# 运行维护

## 启动与关闭

```bash
./tongstock server
```

服务默认监听 `127.0.0.1:8080`。非 loopback 监听必须配置 `server.access_token`，否则启动失败。进程响应 SIGINT/SIGTERM，停止接收请求，等待 HTTP 和后台任务退出，再关闭 TDX 连接池与 SQLite。

## 健康检查

- `GET /health/live` 只表示进程存活；
- `GET /health/ready` 在数据库或 TDX 执行器不可用时返回 503；
- 可选 Agent/Newsfeed 模块降级时 readiness 仍为 200，状态为 `degraded`；
- `GET /health/diagnostics` 返回模块和 schema 版本；
- `GET /health/data-sync` 返回最近的数据新鲜度决策。

## 数据一致性

HTTP 的 quote、daily kline、finance 接口支持 `consistency=require_fresh|allow_stale|cache_only` 和 `refresh=true`。CLI 对应 `--consistency`、`--refresh`。响应头 `X-Data-Freshness`、`X-Data-Sync-Status`、`X-Data-As-Of` 可用于判断结果来源。

## 质量门禁

安装 Go、Node.js 22+、pnpm 10 以及前端依赖后，在仓库根目录运行：

```bash
make check
```

该命令检查 gofmt 回归、全部 Go 测试、关键并发路径 race test、前端 lint 基线、TypeScript、OpenAPI 生成漂移和前端单测。CI 使用同一命令。

## 故障排查

- `upstream_unavailable`：数据库可用但 TDX 同步失败；可用 `allow_stale` 临时读取旧数据。
- `cache_miss`：`cache_only` 下没有本地数据，改用默认模式完成首次同步。
- readiness 503：检查 `/health/diagnostics` 的 `database` 和 `tdx`。
- schema 启动失败：查看日志中的 migration 版本，不要手工推进 `schema_migrations`。
- SSE 中断：客户端应保留收到的消息，并读取最后一个 `error` 事件的 code 和 request_id。
