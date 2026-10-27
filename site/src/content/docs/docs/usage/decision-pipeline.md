---
title: 决策流水线
description: 从发现候选规律到每日选股、持仓判断与质量门，每一步的产物是什么、为什么不可变。
---

这条流水线是 TongStock 与“行情工具”的分界线。核心原则只有一条：**每一步的产物都可追溯到真实数据，并且一旦冻结就不再变**。

## 全景

```
discover ──► method compile ──► validate run ──► market build/freeze ──► select run ──► position decide
   挖掘          编译成结构          回测+检验+反证        冻结快照            每日候选          卖出判断
                 （哈希版本化）      （EvidenceBundle）   （不可修改）        （不可变运行）
```

## 1. 发现候选：`discover`

```bash
./tongstock discover --codes 000063
```

只需给出股票代码。系统自动冻结真实数据，在发现区间搜索候选规律，**预留未触碰的样本**，输出可执行的方法与验证交接信息。它不会直接生成买入推荐——这是刻意的：发现阶段的产物必须再经过验证。

## 2. 编译成结构：`method compile`

```bash
./tongstock method compile candidate.json      # 输出稳定 CompiledMethod JSON
./tongstock method explain compiled.json       # 诊断、歧义与修复建议
./tongstock method entry compiled.json kline.json   # 在历史上执行入场规则，打印匹配日期与 trace
```

`compile` 把结构化候选（观察窗口、目标幅度、价格基准、成功条件、股票池范围等）编译成带哈希的稳定结构。哈希是后面所有引用（验证制品、证据、方法库条目）的主键。

`explain` 用来看“为什么这条规则编译出了歧义”，`entry` 则是最小可信验证：先把入场规则在真实 K 线上跑一遍，看匹配到哪些日子。

:::note[验证口径属于方法本身]
观察窗口、目标幅度、价格基准、成功条件都是方法的一部分，不是全局默认值。系统不再把“20 个交易日”“收盘价达到”当成所有方法的默认口径；兼容模板会被明确标记为确定性 fallback。
:::

## 3. 验证：`validate`

```bash
./tongstock validate run                 # 真实数据自动回测 + 统计检验 + critic 反证
./tongstock validate list                # 按方法哈希列出可引用的验证制品
./tongstock validate evidence <hash>     # 按结果哈希查已持久化的 EvidenceBundle
```

产物是 `EvidenceBundle`：回测结果、统计检验、以及 critic 的反证意见。这份 bundle 的哈希就是文档和 UI 里“证据等级”的出处——不是一个人为打分。

## 4. 冻结快照：`market`

```bash
./tongstock market build 2026-03-27        # 构建当日 MarketSnapshot（可选 FeatureSnapshot）
./tongstock market freeze <id>             # 冻结，之后任何修改都会失败
./tongstock market list                    # 列出快照，按 status 过滤
./tongstock market show <id>               # 查看快照内容
./tongstock market report <id>             # 导出 JSON 报告（含各代码水位与特征统计）
./tongstock market features <snapshot_id>  # 在 ready 快照上构建 FeatureSnapshot
```

冻结是“可审计”的前提：事后复盘时看到的候选，与当天跑出来的完全一致。

## 5. 每日选股：`select`

```bash
./tongstock select run              # 省略快照 id 时用最新 ready 快照
./tongstock select show <run_id>    # 查看一次不可变运行
./tongstock select list [date]      # 历史运行列表
```

`select run` 会在全部**可信方法**上执行，输出带证据的买入候选。没有方法通过门槛时，结果就是空的——这是结论，不是失败。

## 6. 卖出判断：`position`

```bash
./tongstock position decide [market_snapshot_id]
```

基于真实持仓与冻结快照生成**不可变**的持仓决策，输出 `hold` / `watch` / `reduce` / `exit`。旧持仓缺少原始方法血缘时标为 `inferred`；停牌或 T+1 条件下不会假装已经卖出。

## 7. 串成一天：`automation`

```bash
./tongstock automation run [snapshot_id]   # 幂等跑完选股 + 持仓判断 + 提醒闭环
./tongstock automation unlock <snapshot_id> # 释放卡死的任务锁（标记 failed，下次 run 重试）
```

日常运维只需要这一个命令；`unlock` 是异常恢复手段。

## 8. 质量门：`quality` 与 `acceptance`

```bash
./tongstock quality check      # 全量质量检查
./tongstock quality status     # 查看质量门配置与状态
./tongstock quality report [id]
./tongstock quality verify     # 快速验证：数据质量 + 回测黄金集

./tongstock acceptance verify  # 验证正式数据库全链路血缘；缺失即失败
```

`quality` 与单元测试不同：它需要网络与本地数据库状态，因此不在 `make check` 里，而是发布前的独立门禁（`make quality-check`）。

## 9. 方法库与横向比较

```bash
./tongstock method library sync-discovery [research-id]   # 把发现与验证制品同步进方法库
./tongstock method stratify                              # 按历史日均成交额分层，逐层重验样本外表现
./tongstock method report [id]
./tongstock method pool / seed ...                       # 方法股票池与种子候选
```

`stratify` 是防止“结论只在大盘股上成立”的手段：分层之后逐层重验，任一层崩掉都会暴露。

## 通过 HTTP 触发同样的流程

每一步都有对应 REST 入口，例如 `/api/methods/research/run`、`/api/selections/run`、`/api/position-decisions`、`/api/forward/runs`。完整清单见[端点参考](/docs/api/endpoints/)。
