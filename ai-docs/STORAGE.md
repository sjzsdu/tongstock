# 存储与迁移

## 支持范围

TongStock 当前正式只支持 SQLite (`sqlite3`)。配置为 MySQL、PostgreSQL 或其他驱动时，进程会在启动阶段明确失败，不会进入“部分语句可用”的不确定状态。选择依据和后续扩展条件见 [`docs/adr/0001-sqlite-only.md`](../docs/adr/0001-sqlite-only.md)。

默认数据库：

```yaml
database:
  driver: sqlite3
  dsn: ~/.tongstock/cache/tongstock.db
```

SQLite 使用 busy timeout、外键检查和单写连接。App 内的 Cache、Kline、Newsfeed、Chat、Paradigm 及业务 Store 共享同一个 `storage.Storage`/`sql.DB`。

## Schema 所有权

所有 `CREATE TABLE`、索引和兼容性 `ALTER TABLE` 只存在于 `pkg/storage/migrations.go`。Store 构造函数只验证依赖并使用既有 schema，不能自行建表或迁移。

`schema_migrations` 记录已应用版本。每个版本在事务中执行，失败会回滚且不会推进版本号。当前版本：

| 版本 | 内容 |
|---|---|
| 1 | 原有业务表、Kline、缓存、Newsfeed、Chat、Paradigm，以及旧 history/watchlist 列升级 |
| 2 | `quote_snapshot`、`finance_snapshot`、统一 `data_sync_state` |

升级旧数据库时直接启动新版本即可。迁移测试会构造旧 schema 和旧数据，验证新增列、数据保留与事务回滚。

## DB-first 股票数据

| 数据类型 | 业务表 | 新鲜度依据 |
|---|---|---|
| 日 K | `kline` | 请求范围内已完成交易日的覆盖缺口 |
| 行情 | `quote_snapshot` | 盘中短 TTL；盘后最近完成交易日 |
| 财务 | `finance_snapshot` | 财报更新 TTL |

`data_sync_state` 保存同步键、请求/覆盖范围、源更新时间、最后同步时间、质量和写入行数。业务数据和水位在同一事务提交；任何一方失败都不推进水位。

## 备份、恢复与排障

停服后备份：

```bash
cp -f ~/.tongstock/cache/tongstock.db /safe/path/tongstock.db
```

恢复时先停服，再用备份覆盖原文件并启动；启动迁移会自动把旧版本升级到当前版本。不要手工删除 `schema_migrations` 或只复制 WAL 文件。

检查运行状态：

```bash
curl -fsS http://127.0.0.1:8080/health/ready
curl -fsS http://127.0.0.1:8080/health/diagnostics
curl -fsS http://127.0.0.1:8080/health/data-sync
```

`database is locked` 通常表示另一个进程直接打开并长时间持有写事务；确认只运行一个服务实例。迁移失败时服务会拒绝启动，日志会包含迁移版本，数据库仍停留在上一个完整版本。
