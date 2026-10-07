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

***

## 7. 横截面多因子研究（factorlab，已落地 MVP）

### 7.1 方法论背景：形态投票 ≠ 截面排序预测

既有选股链路把「多个方法是否触发形态」加权求和，得分 ≥0.65 才买——这是**形态投票**。而选股本质上是**横截面排序预测**问题：真正要的不是「这只股票符不符合某形态」，而是「这只股票在未来 N 日的收益在全市场里排第几」。两者不等价，也是 23/23 候选全被拒、预测力弱的结构性原因之一。

factorlab 不改选股引擎，先把「预测力」作为独立研究对象建起来：用可解释因子 + 机器可复现的统计证据（RankIC / ICIR / t 统计量）度量「哪组特征真的能预测未来收益」，守住「可解释因子 + 轻量模型 + 因子级证据门槛」的可信定位。

### 7.2 已交付内容

**包 `internal/factorlab/`**（与 methodautomation 同形的数据依赖：冻结快照 + 股票池 + K 线）：

1. **内置因子（`factors.go`，全部可解释、单只股票可计算，共 9 个）**：reversal_1d（1日反转）、momentum_5d / momentum_20d（动量）、up_days_ratio_20d（20日上涨天数占比）、volume_surge（量比激增）、turnover_level（成交额水平）、volatility_20d（波动）、close_position（收盘位置）、price_level（价格水平/低价股效应）。Prior 仅作为待检验假设的文档记录，**实际方向由数据决定**。
   * **数据缺口（已实测确认）**：冻结快照的 BacktestBar 无股本/市值字段，市值规模因子需要先扩展快照冻结 stockinfo（下一步数据管道）；横截面相对强度无需单独因子——组合分内的截面 z 分已扣除截面均值，动量 z 分即相对强度。
2. **统计（`stats.go`）**：逐截面 Spearman RankIC（并列平均秩、截面最少 5 只），输出 MeanIC / ICStd / ICIR / t 统计量。
3. **服务（`service.go`）**：解析冻结快照 → 每股一次 LoadBars 全区间 → 内存逐日截面推进 → 因子原始值与 IC 交集分开存（最后截面日无前向窗口，如实不计 IC 但参与排名）→ 显著性参考线 **|t| ≥ 2 且 |MeanIC| ≥ 0.03，因子方向由数据决定**（Direction = sign(MeanIC)，与先验相反就如实反向——实测 A 股短周期普遍是反转而非动量）→ **仅显著因子按 |IC| 加权**（经典 IC-weighting）合成组合分，输出最后截面日 TopK（默认 30）及逐因子贡献分解。Note 字段是人话结论：显著因子、方向翻转数、不显著就如实说。
   * **重叠窗口修正（实测发现的显著性高估）**：前向窗口长 horizon 时相邻截面日 IC 高度自相关，直接按全截面数算 t 会把 970 个相关截面当成独立样本。现用**步长 = horizon 的去重叠独立子序列**计算 ICStd/TStat（EffectiveSections 透出独立样本量），ICIR = MeanIC/ICStd。
   * **快照新鲜度**：结果携带 SnapshotDateEnd/StaleDays，过时超 14 自然日时 Note 与前端都醒目告警（不静默拒绝）——实测发现快照止于 2026-04-24 时 Top 名单只是历史陈迹；resolveSnapshot 已按 created_at 倒序取最新可用快照，过时的根因是缺少新冻结快照。
4. **红线继承**：前向收益只用于「评估因子」，绝不进入特征计算（前视安全）；一切结论来自真实 K 线，fail-closed（无可用快照/单票快照直接报错）；研究参考线不等于方法库晋级门槛（晋级仍由 validation 工厂决定）。

**HTTP 与装配**：`POST /api/factors/research/run` **异步启动**（全市场快照上一轮可达分钟级，同步等待必然超时；仿自动研究批次：HTTP 立即返回 `started:true` + `status_url`，后台 goroutine 用 Background ctx + 10 分钟超时防泄漏）+ `GET /api/factors/research/last`（无完成结果返回 200 `no_completed_run`，运行中带 `running:true`，前端轮询）+ `GET /api/factors/picks/last`（落库名单，见 7.5）；`internal/serverapp/app.go` 与 methodautomation 共用同一组 Deps 装配，模块名 `factor_lab`。

**前端（Methods 页「因子研究 · 截面排序预测」卡片）**：答案优先——先给 Note 结论 + Top 名单（含逐因子贡献分解 Tag，保持「为何选它」可解释），因子级 IC/t 证据折叠在下层供审计；不显著因子显式标注参考线。

### 7.3 与既有闭环的关系（明确边界）

* factorlab 当前是**独立研究入口**，不替换 `selection/engine.go` 的形态投票选股；选股方法库、证据门槛、前向健康均不受影响。
* 因子评估是**研究参考线**，产出的是「这组因子在历史截面上的预测力证据」，不是方法库的 verified 方法。

### 7.5 因子通道落库（最小生产闭环，已落地）

研究证据要可追溯才有生产价值：显著因子的 TopN 观察名单持久化为**因子通道**（区别于仅存在内存的 research/last 结果，这里是可重访的落库产出）。

* **方案决策（含一次试错）**：先尝试把因子 TopN 写进 `internal/selection` 的 selection 表——发现 selection 硬架构 FK 强制指向 market_snapshot / feature_snapshot，与 factorlab 用 paradigm dataset_snapshot 不兼容；且形态投票选股语义（触发/黑名单/风险触发器）不能承载因子名单。**已回滚**，改用独立通道表。
* **存储（`internal/factorlab/channel.go` + 迁移 v22 `factor_pick_run_channel`）**：表 `factor_pick_run`，`run_id = "pick-<AsOf>"`（AsOf = 最后截面日期）为主键，同一截面日重跑幂等更新（INSERT ON CONFLICT DO UPDATE），`idx(as_of DESC, updated_at_ns DESC)` 支撑 GetLatestPickRun。
* **写入路径**：`POST /factors/research/run` 带 `write_to_selection:true` 时，批次完成后调 `RunResult.ToPickRun` 转 PickRun 落库；**无显著因子（无 picks）时如实报错并仅落日志，不产出空名单占位**。整行 JSON 存档 factors_snapshot 因子评估、逐股票贡献分解，保证任何一天都能回答「那天为什么是这些股票」。
* **读取路径**：`GET /api/factors/picks/last` 返回最近一次 PickRun（无产出返回 200 `no_pick_run`），前端「因子名单」卡片只读展示：run_id / 截面日 / 过时天数 / 落库时因子评估快照 / TopN + 贡献分解。不进方法库、不经 validation 晋级门槛、不接前向监控——观察价值先行，升级为可验证方法走 AI 挖因子里程碑（7.6-2）。
* **测试**：pkg/server 夹具用相位式截面数据（趋势-反转排布，与 internal/factorlab 同构）锁死「run 产出真实结果 + TopK 可控 + write_to_selection 落数 + 同截面日幂等」四条链。

### 7.6 下一里程碑（未做）

1. **接入选股**：把显著因子 TopN 产出一个 selection 候选通道（与形态投票并列、可对比 OOS 表现），或把「因子组合」注册为一种可验证方法进方法库过门槛。
2. **AI 挖因子**：AI 从截面数据挖掘新可解释因子候选（带先验方向），进入同一套 IC 证据评估，而非发明机械买卖规则模板。
3. **数据补齐**：板块/行业归属、市场宽度、涨停梯队 → 相对板块强度、情绪类因子；资金流数据拉高上限。
4. **分层回测**：Top/Bottom 分层单调性、TopK 前向超额，作为比 IC 更直观的预测力呈现。