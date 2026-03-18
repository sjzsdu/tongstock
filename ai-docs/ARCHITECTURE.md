# 系统架构

## 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI / HTTP Server                        │
│                        (cmd/cli / cmd/server)                    │
└─────────────────────────────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                      pkg/tdx/service.go                         │
│                    Service 层 (统一入口)                         │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐    │
│  │ Fetch   │ │ Fetch   │ │ Fetch   │ │ Fetch   │ │ Fetch   │    │
│  │ Codes   │ │ Kline   │ │ XdXr    │ │Finance  │ │ Block   │    │
│  └────┬────┘ └────┬────┘ └────┬────┘ └────┬────┘ └────┬────┘    │
│       │           │           │           │           │          │
│       ▼           ▼           │           │           │          │
│  ┌─────────────────────┐     │           │           │          │
│  │   Client (TDX协议)   │◄────┴───────────┴───────────┘          │
│  │   实时数据访问        │                                        │
│  └──────────┬──────────┘                                        │
└─────────────┼───────────────────────────────────────────────────┘
              │
    ┌─────────┼────────────────────────────────────┐
    │         │              │                    │
    ▼         ▼              ▼                    ▼
┌────────┐ ┌────────┐ ┌────────┐ ┌────────────────────────────┐
│Codes    │ │ Kline   │ │ Workday│ │  XdXr / Finance / Company │
│Cache    │ │ DB      │ │ DB     │ │  Block (Cache)             │
│Store    │ │ Store   │ │ Store  │ │  Stores                   │
└────────┘ └────────┘ └────────┘ └────────────────────────────┘
     │                      │
     ▼                      ▼
┌─────────────┐    ┌─────────────────────────┐
│ Cache 接口   │    │  DB 接口 (sql.DB)      │
│ (SQLite/    │    │  (SQLite/Postgres/    │
│  File)       │    │   MySQL)              │
└─────────────┘    └─────────────────────────┘
```

## 模块职责

### pkg/cache - 通用缓存

**职责**: 提供 KV 缓存抽象，支持 TTL、bucket 分组

**接口**:
```go
type Cache interface {
    Get(bucket, key string) ([]byte, error)
    Set(bucket, key string, value []byte, opts ...Option) error
    Delete(bucket, key string) error
    Has(bucket, key string) bool
    List(bucket string) ([]string, error)
    Clear(bucket string) error
    Close() error
}
```

**后端实现**:
- `SQLiteCache` - 基于 SQLite，键值存 BLOB
- `FileCache` - 基于文件系统，.dat + .meta 文件

### pkg/db - 数据库抽象

**职责**: 提供数据库连接工厂，支持多驱动

**接口**:
```go
func Open(driver, dsn string) (*sql.DB, error)
func OpenSQLite(dsn string) (*sql.DB, error)
func OpenPostgres(dsn string) (*sql.DB, error)
func OpenMySQL(dsn string) (*sql.DB, error)
func OpenFromConfig(driver, dsn string) (*sql.DB, error)
```

### pkg/config - 配置管理

**职责**: 集中管理配置，支持 YAML 文件 + 默认值

**配置项**:
```yaml
server:
  port: 8080

tdx:
  hosts:  # 可选，留空使用内置默认

cache:
  backend: sqlite  # sqlite 或 file
  dir: ~/.tongstock/cache

database:
  driver: sqlite3
  dsn: ~/.tongstock/cache/tongstock.db
```

**目录结构**:
```
~/.tongstock/
├── config.yaml      # 配置文件
└── cache/
    ├── tongstock.db  # SQLite 数据库 (K线、交易日历)
    └── (缓存数据)
```

### pkg/tdx/service.go - Service 层

**职责**: 编排 Client + 本地存储，提供统一的数据访问入口

**核心功能**:
- 缓存命中时直接返回本地数据
- 缓存未命中时拉取远程数据并落库
- 智能判断是否需要刷新（如交易时间内刷新日K）
- 生命周期管理（统一 Close）

## 数据流

### 缓存命中场景

```
用户请求 → Service.FetchXxx()
         → 检查本地 Store
         → 有缓存且未过期
         → 直接返回
```

### 缓存未命中场景

```
用户请求 → Service.FetchXxx()
         → 检查本地 Store
         → 无缓存/已过期
         → Client 拉取 TDX
         → 存入本地 Store
         → 返回数据
```

## K线数据特殊处理

日K线使用 DB 存储，采用智能增量更新：

```
FetchKlineAll(code, ktype=day):
  ① 查本地最新日期
  ② 判断是否需要更新:
     - 昨日收盘后至今 → 跳过
     - 盘中且有今日数据 → 只刷新今日
     - 有新数据 → 增量拉取
  ③ 存库，返回全量
```

## 扩展性

### 新增数据类型的缓存

1. 在 `metadata.go` 创建新的 Store 结构
2. 实现 Get/Save 方法（复用 CodeStore 的 cache 实例）
3. 在 Service 中添加对应字段和 Fetch 方法
4. CLI/Server 路由到新的 Fetch 方法

### 新增数据库表

1. 在对应的 Store 中添加新的表创建逻辑
2. 使用 `KlineStore` 模式：init() 中执行 CREATE TABLE
