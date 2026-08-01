# ADR 0004：领域模型所有权收敛与迁移兼容

- 状态：Accepted
- 日期：2026-08-01
- 依赖：ADR 0002, ADR 0003

## 背景

审计发现 4 组重复领域模型：Paradigm 双包、Quality 双包、Experiment 三处、Evidence 三处。当前每个重复模型都有不同的字段集和存储路径，导致上层 handler 需要同时引用两份，数据所有权模糊。

## 决策

### 1. Paradigm 收敛

**权威 owner：`domain/paradigm`**（合并后）

| 来源 | 处置 | 依据 |
|---|---|---|
| `internal/paradigms/`（复数，16 处引用） | **MERGE INTO** `domain/paradigm` | 功能更全：StateMachine/Validator/Compiler/Scoring/Dedup/Lineage/Schema |
| `internal/paradigm/`（单数，7 处引用） | **MERGE INTO** `domain/paradigm` | 保留 FeaturePipeline/FeatureStore/FeatureSpec/FeatureRegistry/DatasetSnapshot/SnapshotStore |
| `internal/evidence/types.go` | **MERGE INTO** `domain/paradigm` 的 Evidence 子包 | Evidence/Metrics/WindowResult/RegimeResult/AdmissionResult 统一归属 |

合并后 `domain/paradigm` 包含：
- `Paradigm`（唯一结构体，合并字段）
- `Version`, `Candidate`, `DecisionCard`
- `Evidence`, `EvidenceCard`, `Metrics`, `AdmissionResult`
- `StateMachine`, `Validator`, `Compiler`, `Scoring`, `Dedup`, `Lineage`
- `Schema`, `Store`（port）, `SchemaStore`
- `FeatureSpec`, `FeatureStore`, `FeaturePipeline`, `FeatureRegistry`
- `DatasetSnapshot`, `SnapshotStore`
- `ReviewRecord`

### 2. Quality 收敛

**权威 owner：`domain/quality`**

| 来源 | 处置 |
|---|---|
| `internal/quality/`（确定性 + migrations v7） | **KEEP-AS-CORE** → 提纯为 `domain/quality` |
| `internal/ai_quality/` | **DELETE**（零生产调用） |

### 3. Experiment 收敛

**权威 owner：`domain/experiment`**

| 来源 | 处置 |
|---|---|
| `internal/experiment/`（migrations v9） | **KEEP-AS-CORE** → 提纯为 `domain/experiment` |
| `internal/backtest/paradigm_experiment.go` | **MERGE** 状态管理部分到 `domain/experiment`；backtest 仅保留执行器 `paradigm_executor.go` |
| `internal/paradigms/dedup.go:BatchExperiment` | **DELETE** 重复类型，引用 `domain/experiment` |

### 4. Monitoring 收敛

**权威 owner：`domain/monitoring`**

| 来源 | 处置 |
|---|---|
| `internal/monitoring/`（纯计算） | **KEEP-AS-CORE** → 提纯为 `domain/monitoring` |
| `pkg/server/monitoring_handlers.go:monitoringRunRequest` | **DELETE** 手填收益入口 |
| `s.monitoringReport`（内存） | **REPLACE** 为 SQLite 持久化 adapter（1.4 新增 migration v11） |

### 5. Review 处置

**无权威 owner**

| 来源 | 处置 |
|---|---|
| `internal/review/` + `/api/review/*` | **DELETE** 全部，未来复盘用例出现后重建 |

## 数据迁移与兼容期

### 迁移原则

1. 每次合并产生一个新 SQLite migration（版本递增）
2. migration 必须幂等（可重复执行不报错）
3. 必须有回滚 SQL（注释形式）
4. 旧数据原地迁移，不创建影子表长期并存
5. 迁移完成后旧 schema 列在同一 migration 中清理

### 具体迁移计划

| 版本 | 名称 | 内容 | 兼容期 |
|---|---|---|---|
| v11 | `paradigm_convergence` | 将 v1 `paradigms` 表 JSON 拆分为结构化列；保留 `id` 和 `stock_code` 关联 | 迁移后立即删除旧 JSON 列，无双轨 |
| v12 | `monitoring_persistence` | 新建 `monitoring_report` 表存储持久化监控报告 | 无旧表，无兼容期 |
| v13 | `forward_signal_source_gate` | 在 `forward_signal` 表加 `source` 字段 CHECK 约束，仅允许 `deterministic` / `manual_legacy` | 手工历史数据标记为 `manual_legacy`，未来写入仅 `deterministic` |

### 兼容期终止条件

- **v11**: 迁移完成后立即删除旧 `paradigms.data_json` 列，无读取旧 JSON 的代码路径
- **v12**: 无旧实现，立即生效
- **v13**: `manual_legacy` 仅用于历史展示，不参与新计算；不提供任何写入 `manual_legacy` 的入口

### 迁移验证

每个 migration 必须通过：
1. 全新数据库迁移（从 v1 到最新）成功
2. 旧数据库（有真实数据）升级成功
3. 重复执行不报错（幂等）
4. 已有实验制品（experiment_run_artifact）仍可读取
5. 回滚 SQL 可执行（手动验证）

## 收敛顺序

1.3 阶段：先删除无调用者的 `ai_quality`、`review`，不涉及数据迁移
1.4 阶段：按 v11→v12→v13 顺序执行模型合并和迁移
1.5 阶段：质量门固化禁止旧路径重新出现
