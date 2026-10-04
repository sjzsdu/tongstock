# 可信方法自动化闭环・实施方案

> 交付给编码 agent 的落地文档。目标：把系统升级为自动运转闭环 —— 
>
> **自动研究方法 → 机器验证 → 方法市场（评估指标）→ 用户按指标选方法 → 用选中方法筛选股票 → 监控反馈**
>
> 。
> 本文档含系统现状速览（免再探查）、分阶段改动清单（文件级）、接口契约、验收标准与不可违反的硬约束。



***

## 1. 背景与目标

现状：方法库 `/methods` 中方法全部为 `rejected`，原因是候选未通过机器证据门槛。方法研究、验证、选股三个引擎**均已存在**，但：



* 研究手动触发、无定时自动供给；

* 评估指标分散，用户无法 "按指标挑方法"；

* selection engine 只能全量跑，不能 "选特定方法"；

* 前向健康度未作为用户可见指标。

**目标闭环**：



```
自动研究 → 自动验证(置信度) → 方法市场(评估指标卡) → 用户选方法 → 筛选股票 → 结果信号 → 监控反馈 ──反哺→ 自动研究
```



***

## 2. 系统现状速览（编码 agent 免探查清单）

### 2.1 关键目录 / 文件



| 模块     | 文件                                                                                                                 | 说明                              |
| ------ | ------------------------------------------------------------------------------------------------------------------ | ------------------------------- |
| 方法库    | `internal/methodregistry/registry.go` `policy.go` `types.go`                                                       | 状态机、Cards/Card/Audit、Query      |
| 方法持久化  | `internal/adapter/methodregistryrepo/sqlite.go`                                                                    | SQLite 存储                       |
| 验证     | `internal/validation/critic.go` `types.go`、`internal/ai_critic/`                                                   | 证据评估、置信度、反证                     |
| 规律发现   | `internal/discovery/researcher.go`                                                                                 | 模板扫描、基线比较、候选生成                  |
| 外部来源研究 | `internal/methodresearch/researcher.go`                                                                            | 来源核验→编译→validation handoff      |
| 内置方法   | `internal/methodseed/`                                                                                             | 编译→冻结数据回测→按真实结论登记               |
| 选股引擎   | `internal/selection/engine.go` `types.go`                                                                          | 全量跑 eligible 方法→buy/watch/avoid |
| 自动化编排  | `internal/automation/orchestrator.go`                                                                              | selection→position→前向账本         |
| 前向账本   | `internal/ledger/`                                                                                                 | paper trade / 前向信号              |
| HTTP   | `pkg/server/method_registry_handlers.go` `methodseed_handlers.go` `selection_handlers.go` `automation_handlers.go` | 路由                              |
| 应用装配   | `internal/serverapp/app.go`                                                                                        | 依赖注入                            |
| 前端     | `web/src/pages/Methods.tsx` `Screen.tsx` `Dashboard.tsx`、`web/src/api/client.ts` `generated.ts`                    | UI                              |

### 2.2 现有接口契约（勿破坏）

**方法库路由**（`pkg/server/method_registry_handlers.go`）：



```
GET  /api/methods            查询列表（支持 status/market/universe/holding 过滤）
GET  /api/methods/:id        方法详情
GET  /api/methods/:id/audit  状态审计轨迹
GET  /api/method-families/:id
POST /api/methods/seed       载入内置方法并真实回测登记
```

**选股路由**（`pkg/server/selection_handlers.go`）：



```
GET  /api/selections/today    今日选股（支持 ?method_id= 过滤）
GET  /api/selections/runs     选股记录
GET  /api/selections/runs/:id 单次选股详情（支持 ?method_id=）
```

> ⚠️ 目前
>
> **没有**
>
> "运行选股" 的 POST 端点，
>
> `selection.Engine.Run`
>
>  只在 automation 编排器内被调用。

**methodregistry.Query**（`internal/methodregistry/types.go:124`）：



```
type Query struct {
    Status         []Status
    Market         string
    Universe       string
    HoldingMinDays *int
    HoldingMaxDays *int
    FamilyID       string
    Limit          int
}
```

**selection.Request**（`internal/selection/types.go:25`）：



```
type Request struct {
    MarketSnapshotID  string `json:"market_snapshot_id"`
    FeatureSnapshotID string `json:"feature_snapshot_id,omitempty"`
}
```

**methodregistry.Card**（`internal/methodregistry/types.go:133`）含 `EvidenceSummary` 与 `HealthState`：



```
type EvidenceSummary struct {
    ResultHash string; SnapshotID string; JobHash string
    Confidence string   // insufficient/weak/moderate/strong/rejected
    Passable   bool
    OOSTrades  int; OOSReturn, OOSWinRate, OOSMaxDrawdown float64
}
type HealthState struct {
    Score float64; ForwardSamples int
    Drift, Decay, ExecutionDeviation bool
    CriticalAlerts, ConsecutiveSevere int
    EvidenceHash string; AsOf time.Time
}
```

> ⚠️ 
>
> **缺口**
>
> ：
>
> `EvidenceSummary`
>
>  没有 SharpeRatio / SortinoRatio 字段。夏普在 
>
> `internal/validation`
>
>  中已计算但未持久化到方法卡。阶段 B 需要补充。

**置信度规则**（`internal/validation/critic.go:ComputeConfidence`）：

hard blocker→rejected；OOS 交易 <8 或总交易 = 0→insufficient；多重检验不显著→rejected；OOS 夏普 < 0.5 或回撤> 40%→weak；有 soft blocker→moderate；否则 strong。

**方法状态机**（`internal/methodregistry/policy.go`）：

`Initial`：evidence 完整且 passable 且置信度 moderate/strong → `verified`；否则 `candidate`/`rejected`。

`Health`：连续 3 次 severe→retired；告警 / 漂移 / 衰减 / 评分 < 60→degraded；前向样本≥20 且评分≥75→observing。

**选股引擎输出**（`selection.Run`）：



```
type Run struct {
    ID, RunHash, SnapshotID, FeatureSnapshotID, SnapshotDate, Status string
    EligibleMethods, ScannedStocks, CandidateCount, BuyCount int
    ActionCounts map[string]int   // buy/watch/avoid/insufficient_data
    Candidates   []Candidate      // 每只股票：Code, Action, Score, Triggers(Facts, Evidence, Score)
    Exclusions   []Exclusion
    CreatedAt    time.Time
}
```

**automation 编排**（`internal/automation/orchestrator.go`）：

`Run(ctx, snapshotID)` 已串联 selection→positiondecision→`ledger.SignalLedger`（前向账本 /paper trade）。阶段 A/D 复用此模式。

### 2.3 前端 API client（`web/src/api/client.ts`）

已有：`api.methodCards()`, `api.methodCard(id)`, `api.methodAudit(id)`, `api.seedMethods()`。

方法类型见 `web/src/api/generated.ts`。



***

## 3. 分阶段实施

### 阶段 A・自动方法研究（供给）

**目标**：方法库能自动持续产出 `verified` 方法；reject 原因可统计、可反哺。

**改动清单**：



1. **新增调度编排** `internal/methodautomation/`（仿 `internal/automation` 模式）：

* `type Orchestrator struct`，`func New(...) (*Orchestrator, error)`。

* `Run(ctx, req)`：按批次执行 discovery（多持有期 × 多股票池 × 多市场）→ 每个候选自动进入 validation → 通过门槛自动晋级 `verified` → 未过门槛记录 `rejected` 及原因。

* 复用现有组件：`discovery.NewResearcher`、`methodseed.NewService`、validation 工厂、`methodregistry`。

* 周期运行：后台 goroutine ticker（接入 `internal/serverapp/app.go` 生命周期）+ 手动触发 POST 端点。

* `DiscoveryTrials` 全局累计并传入 validation 的 `MultipleTesting`（不只 seed 手动传）。

1. **reject 原因分类与持久化**：

* 在 validation 已产出的 reason（`ComputeConfidence` 各分支）基础上，把原因写入方法的 `AuditEvent.Reason`（已有字段）并新增聚合统计视图。

* 新增查询：`GET /api/methods/reject-stats` → 按 reason 分布统计（交易数不足 / 夏普不足 / 多重检验 /critic 硬伤）。

1. **反哺（P2）**：按 reject 分布淘汰长期无戏模板、扩产有戏方向（改 `discovery/candidateTemplates`）。

**验收**：



* 定时任务运行后，方法库出现 `verified` 方法（非全 rejected）。

* 方法详情能看到 reject 原因；`/methods/reject-stats` 返回分布。

* 手动端点可触发一轮完整研究→验证→晋级。



***

### 阶段 B・方法市场（评估指标）

**目标**：用户能按评估指标排序、筛选、对比方法，10 秒内完成选择。

**改动清单**：



1. **后端补充夏普字段**：

* `internal/validation` 把已计算的 `SharpeRatio`/`SortinoRatio` 持久化；`methodregistry` 的 `EvidenceSummary`（或 `Card`）新增 `sharpe_ratio`、`sortino_ratio`（omitempty，旧数据为缺省）。

* `internal/adapter/methodregistryrepo/sqlite.go` 同步列迁移。

1. **前端改造&#x20;**`Methods.tsx`**&#x20;为 "方法市场"**：

* 每个方法卡片 / 表格增加指标列：置信度、OOS 交易数、OOS 收益、胜率、最大回撤、**健康分（HealthState.Score）**、触发频率。

* 排序：按置信度 / 收益 / 胜率 / 回撤 / 交易数 / 健康分（前端排序即可，可选后端 `sort_by`）。

* 多条件筛选（已有部分，补健康分、置信度）。

* 对比视图：勾选多个方法 → 指标并排对比（新 Drawer / Modal）。

* 指标缺失时明确显示 "待验证"，不假装。

**验收**：



* 方法市场可按任一指标排序、多条件筛选、多方法横向对比。

* 健康分、夏普可见；缺失指标标 "待验证"。



***

### 阶段 C・选方法 → 筛选闭环

**目标**：用户从方法市场选中方法 → 一键用该 (些) 方法筛选股票 → 看到结果。

**改动清单**：



1. **后端**：

* `internal/selection/types.go`：`Request` 增加 `MethodIDs []string`（`json:"method_ids,omitempty"`；nil = 全部）。

* `internal/selection/engine.go`：`Run` 中若 `len(MethodIDs)>0`，先过滤 `allMethods` 到这些 ID（不存在的 ID → 返回错误或记录 exclusion），再走既有 eligible 逻辑。

* `internal/methodregistry/types.go`：`Query` 增加 `IDs []string`（供按 ID 批量查询）。

* 新增 `POST /api/selections/run`：body `{market_snapshot_id, feature_snapshot_id?, method_ids}` → 返回 `selection.Run`（复用 `Engine.Run`）。

* （P2）策略持久化：`POST/GET /api/strategies`（name + method\_ids + 绑定的方法版本）。

1. **前端**：

* 方法市场增加 "用选中方法筛选" 按钮：勾选 1..N 个方法 → 调 `POST /selections/run` → 跳转结果页。

* 结果页展示 `Run.Candidates`：buy/watch/avoid 分类 + 每只股票的 `Trigger.Facts`（触发路径 / 规则 / 是否通过）+ `Score`。复用 `SignalInterpretationCard` / `VirtualResultTable` 风格。

* 结果与所选方法清晰关联（显示用了哪些方法、各方法触发哪些股票）。

**验收**：



* 勾选方法 → 一键筛选 → 展示该方法的选股结果，只跑选中方法。

* 结果中每只股票能看到触发事实与评分。



***

### 阶段 D・监控与反馈

**目标**：已选方法每日前向表现可见；退化告警；用户反馈反哺研究。

**改动清单**：



1. **后端**：

* 聚合 "已选方法前向表现"：命中率、`HealthState.Score`、是否 degraded/retired（复用 automation 已写入的 forwardLedger + `Policy.Health`）。

* 退化事件 → 告警（接入现有 monitoring 或新端点）。

1. **前端**：

* 方法市场 / 筛选结果旁展示所用方法的前向健康度与退化提示（"建议换方法"）。

* 方法卡片 "好用 / 不好用" 反馈 → 写 `AuditEvent`/ 新 feedback 字段 → 反哺阶段 A。

**验收**：



* 用户能看到所用方法的前向表现与健康度；退化时收到提示。

* 用户反馈可写回并影响后续研究。



***

## 4. 跨阶段硬约束（红线，不可违反）



1. **不硬编码 verified**：任何方法状态由机器证据（真实回测 + 置信度）决定，延续 `methodseed` 既有原则；未过门槛如实标 rejected/candidate。

2. **前视安全**：discovery 用未触碰尾部（`reserveUntouchedTail`），validation 用样本外；不得用未来数据参与特征 / 判定。

3. **多重检验**：候选空间增大必须计入显著性校正（`DiscoveryTrials`），否则挖出的全是假阳性。

4. **数据可复核**：所有指标来自真实回测；缺失标 "待验证"，不编默认值。

5. **向后兼容**：新增字段 omitempty、新端点不破坏既有 `GET /methods`、`GET /selections/*`。

6. **测试**：每个阶段后端补测试（对照 `methodregistry/policy_test.go`、`engine_integration_test.go` 既有风格）。



***

## 5. 实施顺序与里程碑

依赖关系：阶段 B/C 需要库里存在 `verified` 方法才有意义，因此 **A（基础版）先行**；B 可用真实数据 mock 与 A 并行开发。



| 里程碑 | 内容       | 完成定义                                        |
| --- | -------- | ------------------------------------------- |
| M1  | 阶段 A 基础版 | 定时自动研究→验证→晋级跑通，库中出现 verified 方法，reject 原因可看 |
| M2  | 阶段 B     | 方法市场可按指标排序 / 筛选 / 对比，健康分、夏普可见               |
| M3  | 阶段 C     | 选方法→筛选→结果闭环走通                               |
| M4  | 阶段 D     | 前向表现 / 退化告警 / 用户反馈闭环                        |



***

## 6. 完成定义（Definition of Done）



* 闭环全链路走通：自动研究 → 验证 → 方法市场 → 用户选方法 → 筛选 → 监控反馈。

* 方法库不再恒为 "全部 rejected"；有真实通过机器证据门槛的 verified 方法。

* 用户能凭评估指标自主选择方法，并看到该方法的选股结果与前向表现。

* 既有接口与功能不回归；新增功能均有后端测试与前端可用性验证。