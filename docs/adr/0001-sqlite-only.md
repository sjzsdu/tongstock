# ADR 0001：正式存储后端限定为 SQLite

- 状态：Accepted
- 日期：2026-07-28

## 背景

旧代码暴露 SQLite、PostgreSQL、MySQL 方言分支，但建表、迁移、占位符和连接生命周期分散在各 Store 中，没有任何非 SQLite 集成测试。这会制造“配置可选但运行不可靠”的假支持。

TongStock 当前是单机工具和模块化单体：CLI、HTTP、缓存、新闻和股票 read model 位于同一进程，数据规模与写并发适合 SQLite。

## 决策

当前正式只支持 SQLite。`pkg/storage.New` 对其他 driver fail-fast。所有模块共享一个 App 持有的连接，所有 schema 变化进入版本化 SQLite migration。

## 结果

- 获得可重复的事务、迁移、备份和旧库升级语义；
- 删除 `pkg/db` 重叠工厂和 MySQL/PostgreSQL 驱动依赖；
- 原有 Store 中残留的方言查询分支仅是迁移期兼容代码，不代表支持承诺；
- 配置文档不再展示不可用的 MySQL/PostgreSQL 示例。

## 重新引入其他数据库的条件

必须先提供独立 storage adapter、完整版本化 migration、每个受支持方言的容器化契约测试、备份/恢复文档，以及 CI 质量门禁。不得仅添加驱动或零散 SQL 分支就宣称支持。
