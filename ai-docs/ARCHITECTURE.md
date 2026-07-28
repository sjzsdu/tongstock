# TongStock 架构

TongStock 是一个模块化单体。CLI 和 HTTP 只是两个传输适配器；日 K、行情快照和财务快照都通过同一个 `internal/app/stockdata.Service` 获取，不允许传输层直接把 TDX 响应当作最终结果。

## 核心数据流

```text
CLI command ─┐
             ├─> stockdata.Service
HTTP route ──┘       │
                     ├─ 1. 检查 SQLite 中的数据覆盖与同步水位
                     ├─ 2. 按当前时间、交易日历和数据类型判断新鲜度
                     ├─ 3. 新鲜：直接查询数据库
                     └─ 4. 过期/缺失：
                            TDX Provider 拉取缺失范围
                            → 校验
                            → 数据和水位同一事务提交
                            → 重新查询数据库
                            → 返回
```

数据库是股票数据的 read model 和唯一返回来源。同步成功后也必须重新读库，避免 CLI 与 API 对同一请求产生不同语义。相同同步键由 `singleflight` 合并；等待者可以独立取消，不会中断其他调用者共享的刷新。

## 组合与生命周期

`internal/serverapp.App` 是服务进程唯一的 composition root，按以下顺序创建资源：

1. 配置和参数；
2. `storage.Storage` 与版本化迁移；
3. TDX `Executor`/连接池；
4. 旧 TDX 能力适配器与统一 `stockdata.Service`；
5. 各业务 Store、可选 Agent/Chat/Paradigm/Newsfeed 模块；
6. Gin Router、`http.Server` 和监听器。

`Shutdown` 可重复调用，并按 HTTP → 后台任务 → TDX Service → Executor → Storage 的逆序释放。Store 借用 App 持有的数据库连接，不单独关闭它。可选模块失败会标记为 `degraded`，不会伪装成核心模块不可用。

## 模块边界

| 层 | 目录 | 职责 |
|---|---|---|
| 进程组合 | `internal/serverapp` | 构造、运行、诊断、关闭 |
| 应用服务 | `internal/app/stockdata` | DB-first、新鲜度、同步、事务、singleflight |
| 上游适配 | `pkg/tdx` | TDX 协议、连接池和类型化调用 |
| 持久化 | `pkg/storage` | SQLite 连接、迁移和 schema 版本 |
| HTTP 适配 | `pkg/server` | 路由、错误契约、认证、SSE、可观测性 |
| CLI 适配 | `cmd/cli` | Cobra 命令和输出格式 |
| Web 适配 | `web/src/api` | 生成契约和浏览器客户端 |

HTTP 路由和 handler 已按 market、analysis、portfolio、sync、strategy 垂直拆分。CLI 命令也按相同业务域拆分。新增能力应先进入应用服务，再由 CLI/HTTP 适配；不要在 handler 或 Cobra `RunE` 中重新实现新鲜度与同步判断。

## 一致性模式

- `require_fresh`：默认。过期时同步；同步失败则返回稳定错误。
- `allow_stale`：尝试同步，失败且数据库有旧数据时返回旧数据。
- `cache_only`：只读数据库，不访问 TDX；缺失时返回 `cache_miss`。
- `refresh=true` / CLI `--refresh`：强制刷新；不能与 `cache_only` 组合。

## API 与可观测性

所有 JSON 错误使用 `{"error":{"code","message","request_id","details?"}}`。内部连接串、SQL、panic 和上游原始错误不返回给客户端。SSE 在发送响应头前使用同一 JSON 错误；开始流式输出后使用带 `code/message/request_id` 的 `error` 事件。

运行状态：

- `/health/live`：进程存活；
- `/health/ready`：数据库和 TDX 等核心依赖是否可用；
- `/health/diagnostics`：模块状态和 schema 版本；
- `/health/data-sync`：最近一次新鲜度决策、原因、同步范围和 `as_of`。

`api/openapi.json` 是传输契约源，`web/src/api/generated.ts` 由它生成。CI 运行 `pnpm api:check` 防止生成物漂移。
