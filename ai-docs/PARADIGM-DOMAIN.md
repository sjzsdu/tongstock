# 研究领域模型与范式生命周期

本文档定义 TongStock 范式研究系统的领域模型、状态机和追溯性要求。

## 1. 领域对象

### 1.1 核心对象

| 对象 | 说明 | 生命周期 |
|------|------|----------|
| **Hypothesis** | 可测试的研究假设 | draft → tested → accepted / rejected |
| **DatasetSnapshot** | 不可变的数据快照 | 创建后不可变 |
| **FeatureSet** | 特征/指标集合定义 | 版本化，创建后不可变 |
| **Experiment** | 假设在数据上的运行 | running → completed / failed |
| **Candidate** | 通过初步筛选的候选范式 | 由 Experiment 产生 |
| **ParadigmVersion** | 范式的版本快照 | 每次状态变更产生新版本 |
| **ValidationReport** | 准入验证报告 | 关联 ParadigmVersion + DatasetSnapshot |
| **Signal** | 范式发出的交易信号 | 时间戳 + 范式版本追溯 |
| **ForwardRun** | 前向模拟运行 | active → completed / stopped |
| **Review** | 人工评审记录 | approve / reject / request_changes |

### 1.2 对象关系

```
Hypothesis (1) ──→ (N) Experiment
                      │
                      ├── DatasetSnapshot (1) ──→ (N) Experiment
                      ├── FeatureSet (1) ──→ (N) Experiment
                      │
                      └── (1) ──→ (N) Candidate
                                    │
                                    └── (1) ──→ (N) ParadigmVersion
                                                  │
                                                  ├── (1) ──→ (N) ValidationReport
                                                  ├── (1) ──→ (N) Signal
                                                  ├── (1) ──→ (N) ForwardRun
                                                  └── (1) ──→ (N) Review
```

### 1.3 不可混用规则

- **Candidate ≠ Paradigm**: Candidate 是实验产物，ParadigmVersion 是经过验证的范式快照
- **Experiment ≠ Validation**: Experiment 是假设测试，Validation 是准入评估
- **Signal ≠ Paradigm**: Signal 是瞬时事件，Paradigm 是持久规则集
- **DatasetSnapshot ≠ FeatureSet**: 数据快照是数据，特征集是计算规则

---

## 2. 生命周期状态机

### 2.1 状态定义

| 状态 | 中文 | 说明 |
|------|------|------|
| `hypothesis` | 研究假设 | 提出可测试的交易模式假设 |
| `experiment` | 实验进行中 | 在历史数据上运行规则集 |
| `candidate` | 候选范式 | 通过初步筛选，等待验证 |
| `validation` | 验证中 | 进行正式证据评估 |
| `observation` | 前向观察 | 模拟前向运行，监控漂移 |
| `promoted` | 已晋级 | 通过全部验证，可用于有限实盘 |
| `relegated` | 降级 | 性能下降，退回观察 |
| `retired` | 已淘汰 | 不再使用（终态） |

### 2.2 状态转换图

```
                    ┌──────────────────────────────────────────┐
                    │                                          │
                    ▼                                          │
hypothesis → experiment → candidate → validation → observation │
                    │          │           │           │       │
                    ▼          ▼           ▼           ▼       │
                 retired    retired    candidate   promoted ───┘
                                              │       │
                                              ▼       ▼
                                          relegated  retired
                                              │
                                              ▼
                                          observation (重试)
```

### 2.3 合法转换

| 源状态 | 允许的目标状态 |
|--------|----------------|
| hypothesis | experiment |
| experiment | candidate, retired |
| candidate | validation, retired |
| validation | observation, candidate, retired |
| observation | promoted, relegated, retired |
| promoted | relegated, retired |
| relegated | observation, retired |
| retired | _(终态，无转换)_ |

### 2.4 状态属性

| 状态 | 是否终态 | 是否活跃(可发信号) |
|------|----------|---------------------|
| hypothesis | 否 | 否 |
| experiment | 否 | 否 |
| candidate | 否 | 否 |
| validation | 否 | 否 |
| observation | 否 | **是** |
| promoted | 否 | **是** |
| relegated | 否 | 否 |
| retired | **是** | 否 |

---

## 3. 版本化与追溯

### 3.1 版本规则

- 每次状态转换必须创建新的 `ParadigmVersion`
- 版本号单调递增
- 每个版本记录：规则集快照、指标快照、父版本 ID、变更原因
- 所有派生结果（实验、候选、验证报告、前向运行）必须引用具体的版本

### 3.2 追溯链

每个范式版本必须能追溯到：

```
ParadigmVersion
  ├── ValidationReport (准入评估)
  ├── Experiment (产生候选的实验)
  ├── Hypothesis (原始假设)
  ├── DatasetSnapshot (使用的数据)
  └── FeatureSet (使用的特征)
```

### 3.3 追溯完整性

| 状态 | 必须有的追溯 |
|------|-------------|
| experiment | paradigm_version_id + hypothesis_id |
| candidate | paradigm_version_id + experiment_id |
| validation | paradigm_version_id + dataset_snapshot_id |
| promoted | paradigm_version_id + validation_report_id |

---

## 4. 代码实现

### 4.1 Go 包结构

```
internal/paradigm/
├── domain.go       # 领域模型类型定义
├── domain_test.go  # 状态机和追溯性测试
└── (future) store.go  # 持久化层
```

### 4.2 关键类型

- `State` - 生命周期状态枚举
- `Lifecycle` - 状态机实例，跟踪当前状态和转换历史
- `Transition` - 单次状态转换事件
- `TraceabilityChain` - 追溯链，验证各状态的追溯完整性

### 4.3 核心方法

```go
// 状态机
func NewLifecycle(paradigmID string) *Lifecycle
func (l *Lifecycle) TransitionTo(to State, reason, by string) error
func (l *Lifecycle) IsTerminal() bool
func (l *Lifecycle) IsActive() bool
func CanTransition(from, to State) bool

// 追溯性
func (t *TraceabilityChain) Validate(state State) error
```

---

## 5. 与证据标准的关系

生命周期与 `ai-docs/PARADIGM-EVIDENCE.md` 定义的准入标准配合使用：

```
validation 状态 → 使用 evidence.ComputeMetrics() 计算指标
                  → 使用 evidence.CheckAdmission() 判定准入
                  → 生成 ValidationReport 记录结果
```

准入等级与生命周期的对应：

| 准入等级 | 允许的下一状态 |
|----------|----------------|
| platinum | observation (直接进入) |
| gold | observation |
| silver | observation (需额外监控) |
| bronze | candidate (退回，需改进) |

---

## 6. 后续工作

- [ ] 实现 `store.go`：ParadigmVersion 的持久化（SQLite/Postgres）
- [ ] 实现 `repository.go`：Hypothesis、Experiment、Candidate 的 CRUD
- [ ] 实现 `lifecycle_service.go`：状态转换的业务逻辑和事件发布
- [ ] 集成到 HTTP API：`/api/paradigms/:id/lifecycle` 状态转换端点
- [ ] 集成到 CLI：`tongstock paradigm transition` 命令
