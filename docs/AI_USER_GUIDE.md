# TongStock AI 使用指南

## 研究股票或方法

打开首页，在唯一输入框中输入股票代码、方法名称或研究问题。AI 可以提出候选规律，但“已验证”只能来自真实冻结数据、样本外检验和反证审查。若 Agent、数据或验证链路不可用，页面会显示失败，不会生成替代结论。

## 查看今天买什么

“今日候选”来自同一交易日的不可变 `MarketSnapshot` 与 `FeatureSnapshot`。每个候选包含触发方法、证据、数据日期、买入窗口、仓位上限、止损/止盈和失效条件。

- `buy`：方法与证据通过门槛，退出计划完整。
- `watch`：规则命中，但证据、评分或退出计划尚不足以买入。
- `avoid`：失效或风险规则已经触发。
- `insufficient_data`：缺少必要数据。不能把它理解为“暂时可以买”。

空榜单是有效结果，表示没有方法通过门槛，而不是系统应当补一个推荐。

## 查看持仓什么时候卖

持仓页给出 `hold/watch/reduce/exit/insufficient_data`，并显示成本、收益、价格时间、触发事实和最迟处置窗口。停牌、涨跌停或 T+1 会标记 `executable=false` 并说明下一步。

旧交易记录没有 selection/method 血缘时显示 `inferred`。这类判断只使用可见成本和通用风险规则，不会伪造当初的买入理由。

## 命令行运维

```bash
tongstock market list ready
tongstock select run <market_snapshot_id>
tongstock position decide <market_snapshot_id>
tongstock automation run <market_snapshot_id>
tongstock acceptance verify
```

所有命令均 fail-closed。不要冻结 partial/failed 快照，也不要人工把 rejected 方法改成 verified。

## 风险边界

TongStock 是研究和决策辅助工具，不自动向券商下单。历史统计、回测和前向观察都不能保证未来获利；仓位与最终交易决定仍由用户负责。
