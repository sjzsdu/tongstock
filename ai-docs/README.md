# TongStock 技术文档

- [ARCHITECTURE.md](ARCHITECTURE.md)：模块化单体、DB-first 数据流和生命周期
- [STORAGE.md](STORAGE.md)：SQLite 支持范围、迁移、备份和恢复
- [SERVICE.md](SERVICE.md)：统一股票数据服务和稳定错误
- [OPERATIONS.md](OPERATIONS.md)：部署、健康检查、质量门禁和排障
- [DATA-MIGRATION.md](DATA-MIGRATION.md)：已迁移数据和其余能力的 read model 计划
- [DIAGRAMS.md](DIAGRAMS.md)：关键流程图
- [SIGNAL.md](SIGNAL.md)：技术信号说明

核心原则：

1. CLI 与 HTTP 是适配器，共用 `internal/app/stockdata.Service`。
2. 日 K、行情和财务以 SQLite read model 为准；过期时同步 TDX，提交后重新读库。
3. App 统一持有数据库、TDX Executor、HTTP 和后台任务生命周期。
4. 当前正式只支持 SQLite；所有 schema 变化进入版本化 migration。
5. `api/openapi.json` 是 API 契约源，生成的 TypeScript 由 CI 检查。
