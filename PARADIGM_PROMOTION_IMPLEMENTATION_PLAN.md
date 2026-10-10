# 范式驱动研究闭环・实施方案（情况 B）

> 交付给编码 agent 的落地文档。选型结论：**范式驱动研究 → 晋级为可信方法 → 选股 → 监控反馈** 的范式驱动闭环（情况 B）。
>
> 核心判断：范式库（`internal/paradigms`）与可信方法库（`internal/methodregistry`）当前是**两条平行、未打通的领域**——选股引擎只消费 methodregistry，范式库的 `ReviewStatus=promoted` 不会自动变成方法。本文档第一步就是补上这条**缺失的"范式→方法晋级链路"**，之后才是范式管理、可信方法、范式监控的完善。

---

## 1. 背景与目标

**现状**：范式矿工 agent（`internal/agents/embedded/stock-paradigm-miner.md`）产出结构化、可证伪的"待验证假设"（范式），存入范式库；但**没有任何代码路径能把范式编译、验证、注册进可信方法库**。范式做得多好都是孤岛，进不了选股。

**目标闭环**（范式驱动）：

```
范式矿工挖假设 → 范式管理(评审/回测/晋级) → 范式→方法晋级链路
   → 可信方法库(过机器证据门槛) → 用户选方法 → 筛选股票
   → 范式监控(前向健康/退化告警) ──反哺→ 新一轮范式研究
```

**不可违反的定位**：范式是"研究假设"，能否晋级为"可信方法"由**机器证据门槛**（真实回测 + 置信度）决定，绝不硬编码晋级，未过门槛如实标 rejected。

---

## 2. 系统现状速览（编码 agent 免再探查清单）

### 2.1 范式库 `internal/paradigms/`

| 文件 | 说明 |
| --- | --- |
| `paradigm.go` | `Paradigm` 聚合根：`Side`(buy/sell)、`Context`(MarketCap/ShareholderDominant/Activity/Trend)、`BuyConds/SellConds`、`Expectation`、`Rationale`、`ReviewStatus`、`Evidence`、`Transitions` |
| `condition` | `Condition{Indicator, Operator, Value}`，Operator ∈ `cross_above / cross_below / gt / lt / between / near` |
| `evidence.go` | `EvidenceCard`：`PromotionEligible` / `PromotionBlockers` / `InSample` / `OutOfSample` / `CounterEvidence` 等 |
| `store.go` | `Store` + `Repository` 接口：`LoadAll()` / `Save(p)` / `Delete(id)` |
| `state_machine.go` | 状态枚举 `pending→reviewed→verified→promoted→degraded/suspended/rejected`；**注释明确：Transition 校验路径无生产调用者，已在架构整顿中删除**（即晋级状态流转目前是"壳"，无落地） |
| `validation.go` | 范式自校验（条件可自动求值比例、数据完整度、可靠性标签） |

### 2.2 可信方法 `internal/methodregistry/`

| 文件 | 说明 |
| --- | --- |
| `registry.go` | `New(repo)` / `Register(ctx, Registration)` / `Cards(ctx, Query)` |
| `types.go` | `Method`（状态机 draft→candidate→verified→observing/degraded→retired/rejected）、`EvidenceSummary`(Confidence/Passable/OOSTrades/OOSReturn/OOSWinRate/OOSMaxDrawdown/SharpeRatio)、`HealthState`、`Registration`、`Evidence` 接口 |
| `policy.go` | 状态机：`Initial`(evidence 完整且 passable 且置信度 moderate/strong→verified)、`Health`(连续3次severe→retired) |

**关键**：`methodregistry.ValidationEvidence{Bundle}` 实现了 `Evidence` 接口，是现有晋级注册的标准证据载体。

### 2.3 方法编译与验证

| 文件 | 说明 |
| --- | --- |
| `internal/methods/compiler.go` | `Compile(c *Candidate) (*CompiledMethod, []Diagnostic, error)`。`Candidate` 的 `SourceKind` 枚举**已含 `"existing_paradigm"`**（为范式晋级预留的信号） |
| `internal/methods/seed.go` | `SeedCandidate`：编译器输入模板（只含单日可判定状态条件、显式 `FeatureDeps`、股票池 universe_usable）——**晋级范式的编译必须遵循同一约束** |
| `internal/validation/` | `NewFactory` → `ValidationJob{MethodHash, SnapshotID, Universe, DateStart, DateEnd, SplitType, DiscoveryTrials}` → `factory.Run` → `bundle`(Confidence/Passable/OosStats/ResultHash) |

### 2.4 现成的"研究→验证→注册"模板（可直接复用）

- **`internal/methodautomation/orchestrator.go`**：`runHoldDays` 已实现完整流程 `编译 → NewFactory → factory.Run → evidence.Save → Registry.Register`（见该文件 300–370 行）。**晋级链路直接复用此模板**。
- **`internal/methodseed/service.go`** `loadOne`：同样流程（编译→验证→注册→status）。

### 2.5 服务端 `pkg/server/`

| 文件 | 说明 |
| --- | --- |
| `paradigm_handlers.go` | `/api/paradigm/*`：`list / stats / analyze / evaluate / review`；`Group("/paradigm")` 已建立 |
| `paradigm_experiment_handlers.go` | `handleParadigmBacktest` → `executeParadigmExperiment(p, req)`（真实冻结快照回测范式）；`latestParadigmExperimentEvidence`（按范式+实验取证据卡） |
| `paradigm_evidence_builder.go` | 构建 `EvidenceCard`、判定 `PromotionEligible`（blockers 为空才可晋级） |
| `method_market_handlers.go` | `/api/methods/*`：`research/run`、`reject-stats`、`forward-health`、`:id/feedback` |

### 2.6 装配 `internal/serverapp/app.go`

已有：`methodRegistry`(247)、`methodAutomation`(358, Deps.Registry=methodRegistry)、`factorLab`(380)、`paradigmStore`(466, `NewStoreWithRepository`)、`a.api.SetParadigmStore(paradigmStore)`(471)。

**缺口**：`methodAutomation.Deps` 与 `paradigmStore` 之间**没有接线**；也没有"范式晋级"专用的编排器或 handler。

---

## 3. 分阶段实施

### 阶段 A（P0・核心）・范式→方法晋级链路

**目标**：范式经机器验证后能真正注册进可信方法库，`ReviewStatus=promoted` 落地为方法，选股可消费。

**改动清单**：

1. **新增范式→候选编译映射** `internal/paradigms/promote.go`：
   - `func ToCandidate(p *Paradigm) (*methods.Candidate, []string, error)`：
     - `Side=buy` 才可晋级；`Side=sell` 返回 blocker。
     - `BuyConds`（and 组合）→ `Entry` 规则树；`SellConds.TakeProfit` → `TakeProfitPct`/`Exit`；`StopLoss` → `StopLossPct`。
     - `Context.MarketCap` → 映射 `Universe`/`BoardFilter`（small/mid/large 分桶）；`Expectation.HoldingPeriod` → `HoldingMaxDays/HoldingMinDays`。
     - `SourceKind: "existing_paradigm"`。
   - **Operator 映射必须 fail-closed**：
     - `gt / lt / between`（→ and(gt,lt)）→ 可映射；
     - `cross_above / cross_below / near` → **返回 blocker**（选股引擎只持单日冻结特征，`internal/selection/engine.go:108` 已对 cross/in_window 产出 `historical_features_unavailable` exclusion——晋级进方法的范式不能含此类条件，否则"验证通过但永不触发"）。
   - 缺 `Universe`/`Position`/`FeatureDeps` 等编译必需字段 → 返回诊断，不静默猜测。

2. **新增晋级服务** `internal/paradigmspromote/service.go`（仿 `methodautomation`）：
   - `func Promote(ctx, paradigmID, opts) (PromoteOutcome, error)`：
     1. `paradigmStore.Get(paradigmID)`；
     2. `ToCandidate` → 有 blocker 则记入 `PromotionBlockers` 并返回（不写状态）；
     3. `methods.Compile` → `IsExecutable()` 检查；
     4. `validation.NewFactory` + `ValidationJob`（`DiscoveryTrials` 沿用全局累计）→ `factory.Run`；
     5. `evidence.Save`；
     6. `methodRegistry.Register(Registration{FamilyID:"paradigm-"+p.ID, VariantID:compiled.ContentHash, Name:"[范式] "+p.Name, SourceResearchID:p.ID, ValidationJobID:jobHash, Method:compiled, Evidence:ValidationEvidence{Bundle}})`；
     7. **回写范式**：`ReviewStatus="promoted"`、`Evidence.PromotionEligible`、记录 `MethodID` 与 `StateTransition{From:"reviewed",To:"promoted",Action:"promote",EvidenceHash}` → `paradigmStore.Save(p)`。

3. **HTTP 端点** `pkg/server/paradigm_promote.go`：
   - `POST /api/paradigm/:id/promote` → body 可选 `{snapshot_id}`，异步启动（复用 factorlab 的 `started:true + status_url` 模式，避免超时）+ `GET /api/paradigm/:id/promotion/status`。
   - `GET /api/paradigm/:id/evidence`（若无则复用 `latestParadigmExperimentEvidence` 语义）。

4. **装配** `internal/serverapp/app.go`：`paradigmPromote` 依赖注入（`paradigmStore` + `methodRegistry` + validation 工厂 + evidence + bars + benchmark + universe），`a.api.SetParadigmPromote(...)`。

**验收**：
- 对一个 `ReviewStatus=reviewed` 的 buy 范式调用 promote，若真实回测通过门槛 → 方法库出现 `verified` 方法，范式 `ReviewStatus=promoted` 且带 `MethodID`；若未过门槛 → 范式如实 `rejected` + 原因，方法库不变。
- 含 `cross/near` 条件的范式晋级被 blocker 拦截，不产生方法。
- 前端范式卡片能看到"晋级为方法 → 方法ID/链接"。

---

### 阶段 B・范式管理（好用性）

**目标**：范式从挖出到评审到晋级是一条顺畅、可解释、可复盘的工作流；评审结论有依据而非拍脑袋。

**改动清单**：

1. **评审闭环落地**：补齐 `ReviewStatus` 流转为生产路径（重新引入校验，替代 `state_machine.go` 里被删的壳）：
   - `PUT /api/paradigm/:id/review` 已有雏形，扩展为：`pending→reviewed`（填 `ReviewNote/ReviewRating`）、`reviewed→rejected`（人工淘汰 + 原因）、`reviewed→(调 promote)`。
   - 每次流转写 `Transitions` 审计。

2. **范式列表增强**（`web/src/pages/Paradigms.tsx` 已有 `reliabilityColor`/review 筛选）：补充列 `自动可评估比例 / 数据完整度 / 是否可晋级（是否有 PromotionBlocker） / 关联方法`。

3. **回测前置可视化**：`handleParadigmBacktest` 已存在，前端把"回测 → 证据卡 → 是否 PromotionEligible + blockers"串成单个"晋级体检"流程（复用 `paradigm_evidence_builder`）。

4. **评审反馈反哺**：晋级成功的方法 + 用户好评 → 提高该范式族/方向的下一轮挖矿预算；淘汰率高 → 降预算（对齐 `methodautomation.negativeFeedback` 既有逻辑）。

**验收**：
- 范式库每一条都能看到"评审状态 + 晋级资格（可/不可 + 原因）"。
- 人工评审动作全部留痕；晋级链路可一键触达。

---

### 阶段 C・可信方法（方法市场）

**目标**：方法市场按指标可排序、可筛选、可对比，选方法 → 筛选闭环走通（实施计划已定义，此处确认状态）。

**改动清单**（多为确认/补齐，主体已在 `METHOD_AUTOMATION_IMPLEMENTATION_PLAN.md` 阶段 B/C 定义）：
1. `EvidenceSummary` 补 `SharpeRatio/SortinoRatio`（validation 已计算未持久化），`methodregistryrepo/sqlite.go` 同步迁移。
2. `Methods.tsx` 完善指标列（置信度/OOS交易数/OOS收益/胜率/回撤/健康分/触发频率）+ 排序 + 对比。
3. `selection.Request` 增加 `MethodIDs` + `POST /api/selections/run`（按选中方法跑）+ 结果页展示触发事实与评分。

**验收**：勾选方法 → 一键筛选 → 只跑选中方法；每只股票可见触发事实与评分。

---

### 阶段 D・范式监控

**目标**：晋级方法的前向表现可见，退化告警，反馈反哺研究。

**改动清单**：
1. 聚合"晋级方法前向表现"：命中率 / `HealthState.Score` / 是否 degraded/retired（复用 automation 已写入的 forwardLedger + `Policy.Health`）。
2. 退化事件 → 告警；前端方法卡片/范式卡片旁展示前向健康度与"建议换方法"提示。
3. 用户"好用/不好用"反馈 → 写 `AuditEvent`/feedback 字段 → 反哺阶段 B 挖矿预算。

**验收**：用户能看到所用方法的前向表现与健康度；退化有提示；反馈写回并影响后续研究。

---

## 4. 跨阶段硬约束（红线）

1. **不硬编码晋级**：范式 `promoted` 或方法 `verified` 一律由机器证据（真实回测 + 置信度门槛）决定；未过门槛如实 `rejected`。
2. **前视安全**：范式晋级复用 `validation` 的样本外分割；不得用未来数据参与特征/判定。
3. **多重检验**：范式候选空间扩大必须计入 `DiscoveryTrials` 显著性校正。
4. **数据可复核**：所有指标来自真实回测；缺失标"待验证"，不编默认值。
5. **编译器 fail-closed**：无法可靠映射的范式条件（cross/near 等）拒绝晋级，不静默降级成近似规则。
6. **向后兼容**：新增字段 omitempty、新端点不破坏既有 `/api/paradigm/*`、`/api/methods/*`、`/api/selections/*`。
7. **测试**：每阶段补后端测试（对照 `paradigm_experiment_handlers_test.go`、`methodregistry/policy_test.go`、`engine_integration_test.go` 风格）。

---

## 5. 实施顺序与里程碑

依赖：阶段 B/C 的"晋级体检/方法市场"只有在 A 打通后才真正有意义，故 **A 先行**；B/C/D 可按依赖推进。

| 里程碑 | 内容 | 完成定义 |
| --- | --- | --- |
| M1 | 阶段 A | 范式可一键晋级为方法；过门槛出现 `verified` 方法，范式状态与 MethodID 回写；cross/near 被拦截 |
| M2 | 阶段 B | 范式评审闭环落地、晋级资格可见、回测体检串联 |
| M3 | 阶段 C | 方法市场排序/筛选/对比 + 选方法→筛选闭环走通 |
| M4 | 阶段 D | 前向表现/退化告警/用户反馈闭环 |

---

## 6. 完成定义（Definition of Done）

- 范式 → 方法晋级链路全链路走通：挖假设 → 评审 → 机器验证 → 注册进方法库 → 选股 → 监控 → 反哺。
- 范式库不再是与选股无关的孤岛；`promoted` 范式对应方法库中真实存在且可被选股消费的方法。
- 方法库有真实通过机器证据门槛的 `verified` 方法（非恒为全 rejected）。
- 用户能凭评估指标自主选方法，看到该方法的选股结果与前向表现。
- 既有接口与功能不回归；新增功能均有后端测试与前端可用性验证。
