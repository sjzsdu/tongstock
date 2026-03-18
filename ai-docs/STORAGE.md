# 数据存储策略

## 概述

TongStock 根据数据的特性（更新频率、数据量、查询模式），采用三种存储策略：

| 策略 | 适用场景 | 存储位置 |
|------|----------|----------|
| **Cache** | 数据变化慢、需要 TTL 过期 | `~/.tongstock/cache/tongstock.db` (SQLite) |
| **DB** | 大量数据、需要复杂查询 | `~/.tongstock/cache/tongstock.db` (SQLite) |
| **穿透** | 实时数据、变化频繁 | 不存储，每次直连 TDX |

---

## 详细策略

### 1. Cache 策略

**适用**: 变化慢的数据，使用 TTL 自动过期

| 数据类型 | Bucket | TTL | 说明 |
|----------|--------|-----|------|
| Codes (股票列表) | `codes` | 24 小时 | 股票代码变化极慢 |
| XdXr (除权除息) | `xdxr` | 7 天 | 几年才变一次 |
| Finance (财务数据) | `finance` | 7 天 | 季度更新 |
| Company (F10信息) | `company_cat` / `company_content` | 30 天 | 几乎不变 |
| Block (板块信息) | `block` | 1 天 | 变化较慢 |

**实现**:
- 使用统一的 `Cache` 接口（SQLite 后端）
- 数据序列化为 JSON 存储
- TTL 过期后自动失效，触发重新拉取

**代码示例**:
```go
// 从缓存获取
items, err := xdxrStore.Get(code)
if err == nil && items != nil {
    return items, nil  // 命中缓存
}

// 缓存未命中，从远程拉取
items, err = client.GetXdXrInfo(code)
_ = xdxrStore.Save(code, items)  // 存入缓存
return items, nil
```

---

### 2. DB 策略

**适用**: 大量数据、需要 SQL 查询

| 数据类型 | 表名 | 数据量估算 | 说明 |
|----------|------|-----------|------|
| Kline 日线 | `kline` | 1250 万条 (5000股票×10年) | 唯一持久化存储的K线类型 |
| Workday | `workday` | ~5000 条 | 交易日历 |

**Kline 表结构**:
```sql
CREATE TABLE kline (
    code TEXT,
    ktype INTEGER,
    date TEXT,
    open REAL,
    high REAL,
    low REAL,
    close REAL,
    volume REAL,
    amount REAL,
    PRIMARY KEY (code, ktype, date)
);
CREATE INDEX idx_code_ktype ON kline(code, ktype);
CREATE INDEX idx_date ON kline(date);
```

**K线智能更新逻辑**:
```
情况                              │ 操作
─────────────────────────────────┼──────────────────────
本地无数据                        │ 全量拉取
本地 ≥ 昨收盘 且 非交易时间        │ 跳过（直接返回本地）
本地 = 今日 且 交易时间中          │ 只刷新今日1条
本地 < 今日                       │ 增量拉取
```

**为什么只存日K**:
- 分钟/周/月/季/年数据量巨大（见下表）
- 盘中实时性要求高，缓存价值低
- 后续如需扩展，用独立表实现

| K线类型 | 5000股票×1年数据量 |
|---------|-------------------|
| 1分钟   | 1.2 亿条（不存）  |
| 5分钟   | 2400 万条（不存） |
| 日线    | 125 万条 ✅       |

---

### 3. 穿透策略

**适用**: 实时数据，变化频繁，缓存无意义

| 数据类型 | 说明 |
|----------|------|
| Quote (实时行情) | 每秒都在变 |
| Minute (分时数据) | 盘中实时 |
| Trade (分笔成交) | 盘中实时 |

**实现**: 直接调用 `Client.GetXxx()`，不经过任何本地存储

---

## 存储位置

所有数据默认存储在 `~/.tongstock/` 目录：

```
~/.tongstock/
├── config.yaml          # 配置文件
└── cache/
    └── tongstock.db    # SQLite 数据库
        ├── cache 表    # Cache 策略数据 (Codes/XdXr/Finance/Company/Block)
        ├── kline 表    # DB 策略数据 (日K)
        └── workday 表  # DB 策略数据 (交易日历)
```

**配置说明** (`~/.tongstock/config.yaml`):

```yaml
# 缓存后端: sqlite 或 file
cache:
  backend: sqlite
  dir: ~/.tongstock/cache

# 数据库配置
database:
  driver: sqlite3
  dsn: ~/.tongstock/cache/tongstock.db
```

---

## 切换后端

### Cache 后端切换

```yaml
# 使用文件缓存
cache:
  backend: file
  dir: /path/to/cache_dir
```

### Database 驱动切换

```yaml
# PostgreSQL
database:
  driver: postgres
  dsn: user=xxx dbname=xxx sslmode=disable

# MySQL
database:
  driver: mysql
  dsn: user:password@tcp(localhost:3306)/dbname
```

---

## 手动清理

### 清理所有缓存
```bash
# 方式1: 删除数据库
rm ~/.tongstock/cache/tongstock.db

# 方式2: 通过代码
cache.Clear("codes")
cache.Clear("xdxr")
# ...
```

### 清理特定类型
```go
// 只清理 K线数据（保留缓存）
// 需要用 db.Exec("DELETE FROM kline")

// 只清理缓存
cache.Clear("codes")
cache.Clear("xdxr")
```
