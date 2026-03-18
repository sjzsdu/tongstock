# TongStock 文档

本目录包含 TongStock 项目的技术架构和设计文档。

## 文档列表

### 架构设计
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - 系统架构概览，包括模块划分、数据流图

### 数据存储
- **[STORAGE.md](STORAGE.md)** - 各类数据的存储策略（Cache/DB/穿透）

### 接口文档
- **[SERVICE.md](SERVICE.md)** - Service 层 API 参考

---

## 快速导航

### 数据存储决策

| 数据类型 | 存储方式 | TTL/特性 |
|----------|----------|----------|
| Codes (股票列表) | Cache | 24 小时 |
| Kline 日线 | DB | 永久，增量更新 |
| Kline 其他 | 穿透 | 不存储 |
| Workday | DB | 永久 |
| Quote (实时行情) | 穿透 | 实时 |
| Minute (分时) | 穿透 | 实时 |
| Trade (分笔) | 穿透 | 实时 |
| XdXr (除权除息) | Cache | 7 天 |
| Finance (财务) | Cache | 7 天 |
| Company (F10) | Cache | 30 天 |
| Block (板块) | Cache | 1 天 |

### 核心模块

- `pkg/cache/` - 通用缓存接口 + SQLite/File 后端
- `pkg/db/` - 数据库抽象层（支持 SQLite/PostgreSQL/MySQL）
- `pkg/config/` - 配置系统（YAML + 默认值）
- `pkg/tdx/service.go` - Service 层，统一的数据访问入口
