# ADR 0002：模块化单体目标架构与依赖方向

- 状态：Accepted
- 日期：2026-08-01
- 依赖：ADR 0001（SQLite-only）

## 背景

当前 `pkg/server/server.go` 的 `*Server` 结构体直接持有 20+ 个 Store、Service 和引擎字段，所有 handler 挂在同一结构体上，业务流程内联在 HTTP handler 中。`internal/paradigm` 与 `internal/paradigms` 双包并存，领域模型直接持有 `*sql.DB`，无 ports/adapters 分界。这导致依赖方向混乱、无法独立测试领域逻辑、新增功能只能让 Server 继续膨胀。

## 决策

### 架构形态

保持模块化单体（modular monolith），不引入微服务、事件总线或通用插件框架。

### 依赖方向（强制）

```
transport (HTTP / CLI)
         ↓
application use cases
         ↓
domain
         ↓
ports (interface)
         ↓
adapters (SQLite / TDX / LLM / Browser / Notification)
```

每一层只能依赖下层，禁止反向依赖和同层循环依赖。

### 层职责

| 层 | 职责 | 禁止 |
|---|---|---|
| `transport/http` | 鉴权、参数绑定、状态码、DTO 映射、调用 use case | 业务逻辑、直接访问 Store/DB |
| `transport/cli` | 参数解析、调用 use case | 业务逻辑、直接访问 Store/DB |
| `application/usecases` | 编排领域对象和端口完成一个用例 | HTTP/Gin 概念、直接持有 *sql.DB |
| `domain/*` | 纯领域值对象、领域服务、状态机 | import Gin/SQLite/TDX/LLM SDK |
| `ports` | 跨模块和基础设施边界的 interface | 具体实现 |
| `adapters/*` | 实现 ports interface，持有外部资源 | 被 domain import |

### 目标模块（owner 表）

| 模块路径 | 拥有的领域对象 | 持久化表 | 来源合并 |
|---|---|---|---|
| `domain/market` | StockCode, Kline, XDXR, Workday, MarketSnapshot, PriceAdjustment | kline, kline_sync_state, workday, quote_snapshot, finance_snapshot, xdxr_event, adjustment_factor, security_status_history, data_sync_state | pkg/tdx/protocol 类型提纯 + internal/app/stockdata/types |
| `domain/portfolio` | Watchlist, StockPool, History, Trade, Position | watchlist, stockpool, history_stocks, trades | pkg/watchlist, pkg/stockpool, pkg/history, pkg/trading |
| `domain/paradigm` | Paradigm, Version, Evidence, StateMachine, DecisionCard, Candidate, Lineage, Schema, Compiler, Validator, Scoring, Dedup | paradigms (v1), feature_spec/set/value (v6) | internal/paradigms + internal/paradigm + internal/evidence 合并 |
| `domain/experiment` | Experiment, Run, Artifact, Registry | experiment_registry, experiment_run, experiment_run_artifact (v9), dataset_snapshot/snapshot_data_source/experiment_snapshot_binding (v5) | internal/experiment 为权威 |
| `domain/forward` | ForwardRun, SignalEntry, EquityCurve | forward_run, forward_signal (v10) | internal/ledger 类型提纯 |
| `domain/quality` | QualityIssue, QualityReport, Gate, GateResult | quality_report, quality_issue, quality_gate_config (v7) | internal/quality |
| `domain/monitoring` | DriftResult, DecayResult, ConcentrationResult, Alert, MonitorReport | （新增持久化表，1.4 阶段定义） | internal/monitoring 类型提纯 |
| `domain/agent` | AgentSession, ToolCall, Transcript | chat_sessions | internal/agents, internal/picoclaw, internal/ai_critic 类型提纯 |
| `domain/news` | NewsItem, HotEvent, Alert | news_items, hot_events, alert_records | pkg/newsfeed/types |

### Composition Root

`internal/serverapp/app.go` 保持为唯一 Composition Root：
- 装配所有 adapter 和 store
- 向 use case 注入 ports
- 向 transport 注入 use case
- 拥有所有长生命周期资源的 Close 顺序

### *Server 拆分策略

当前 `*Server` 不再无限增长。新功能按模块注册 handler group，每个 group 接收自己模块的 use case 依赖，不共享全局 struct。1.4 阶段逐步拆分。

## 验证方式

1.4 完成后，以下测试必须通过：
- `go test ./domain/...` 不依赖任何 adapter 包
- import graph 无环（`go list -deps` 或 `depguard` linter）
- domain 包不 import `gin`、`database/sql`、`github.com/sjzsdu/tongstock/pkg/tdx`、`github.com/sjzsdu/tongstock/pkg/storage`

## 不做的事

- 不引入微服务
- 不引入事件总线或通用插件框架
- 不为假想未来需求提前抽象 interface
- 不创建额外的 ORM 或 query builder 层
