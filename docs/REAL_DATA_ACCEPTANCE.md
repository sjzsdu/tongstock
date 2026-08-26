# 真实数据端到端验收报告

验收时间：2026-08-02（Asia/Shanghai）

## 数据与性能

- 正式 SQLite 数据库：`~/.tongstock/cache/tongstock.db`。
- 验收交易日：2026-04-24。
- MarketSnapshot：`1cea6d87fe08f0d4483a`，真实股票 583/583 ready，内容哈希 `a5d81bb7935d5c48a3ecfc6371e8eb8df0ba0c5d0d504df1f267f1a83b759b05`。
- FeatureSnapshot：`fs_6ae506474c2117707db8`，583 只股票、8716 个真实特征值，内容哈希前缀 `2d06e8571384`。
- 选股运行：`selection-a5bdcfaf34c16ad5`。扫描 583 只股票，CLI 进程总耗时约 0.234 秒。
- 持仓运行：`position-ff59ec4805ef1e99`。
- 自动作业：`job-18c80222e4c24450`，绑定上述 selection/position，业务运行约 54ms；重跑返回同一 Job。
- 另使用 2026-07-15、2026-07-16 的真实日线完成独立快照回放。

## 三条旅程

1. 个股规律发现：真实 discovery research trace 已持久化；候选必须进入验证工厂，不因 AI 文本直接晋级。
2. 命名方法研究：真实 source evidence、research artifact、validation evidence 和 critic 结论可按哈希追溯。
3. 每日决策：583 股 selection、position decision、automation Job 均绑定不可变快照。当前两个登记方法均为 `rejected`，因此真实结果是 eligible=0、buy=0。正式库当前没有持仓，因此 position decisions 为空；系统没有插入演示持仓。

命名方法研究制品 `method-research-1785680258346550000` 使用两份实际下载并计算 SHA-256 的公开二手资料：[新浪新闻](https://www.sina.cn/news/detail/5290792584480386.html) 和 [淘股吧](https://m.tgb.cn/a/2it5fHCsXNT)。两者只能支持“尾盘买、次日早盘卖”的共同描述，无法消除日内阈值差异，因此研究状态为 `source_complete`，但编译版本保持 ambiguous、不可执行，也没有进入 verified 方法库。

## 持续质量门

`make check` 覆盖 Go 全量测试、race、架构依赖、生产无随机/无 Mock/无伪造结果扫描、迁移幂等、OpenAPI、前端 lint/typecheck/test/build。生产扫描同时覆盖 Go 和 `web/src`。

本机可额外执行：

```bash
tongstock acceptance verify
```

该命令复核快照与特征内容哈希、leak gate，以及 research → evidence → method → selection → position → automation 血缘，缺任何一环直接失败。

## 明确限制

- 本次验收证明软件链路在现有真实数据上可复现工作，不证明任何方法可以稳定获利。
- 当前真实方法均未通过门槛，所以不能验收真实 buy 信号或后续成交收益；系统选择拒绝推荐。
- 当前正式库没有真实持仓，所以停牌、止损、止盈、T+1 场景由真实 SQLite 适配器集成测试覆盖，但不能冒充用户真实持仓结果。
- 数据库在该交易日可组成的完整真实股票池为 583 只，不等于当日全部 A 股。扩大覆盖前必须先补齐行情和证券状态数据。
