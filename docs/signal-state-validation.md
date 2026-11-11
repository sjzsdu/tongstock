# WAN-60 实库验收记录（2026-11-10）

本地日线快照已完成全市场回填。计算复用第 1 期纯内核，未读取旧事件流水。

| 项目 | 实测结果 |
| --- | --- |
| 本地代码数 | 5,252 |
| 实际股票日 | 16,553,584（19901219—20261110） |
| 四维状态行 | 66,214,336 |
| 每维行数 | cross / osc / boll / ma 各 16,553,584 |
| 有 bar 的状态覆盖率 | 100%，缺行 0 |
| 全市场逐行参考校验 | 66,214,336 条维度行一致 |
| 全市场增量重跑 | changed=0，复核 66,214,336 条维度行 |
| 口径 | 全表 raw / state-v1；每股参数 JSON 与输入种子可追溯 |
| 版本 / hash 格式错误 | 0 |
| 数据不足维度行 | 714,724，独立标记，技术态一致性错误 0 |
| 索引 | `(trade_date)` 与 `(code, trade_date)` 均已建成并核验 |

200 只分层抽样股票的全历史试跑：634,438 个股票日、2,537,752 条维度行。
通过原现算接口相同的本地读数路径（`KlineStore.GetKline` + `FilterValidKlines`）
逐行比较状态和 `calc_hash`，零差异；同一完整回填命令重跑 changed=0。
全量按股事务提交，中断后可由增量模式补齐；最终再对全市场逐行复核，保证完整覆盖。

真实行情连续五交易日重放（600519，2026-11-04/05/06/09/10）：每天只新增四条
维度行，每日 hash 与完整历史参考计算一致，无缺口；重跑 changed=0。
原有截断对拍测试及新增后缀种子、修订重算、缺维修复、撤回行情、502 股无上限、
SQLite 双连接写锁、同步失败传播、仅读库接口等测试通过。

`make check` 全部通过（Go 测试、race、架构、迁移、前端 lint/typecheck/API
生成检查、100 个前端测试及构建）。新增快照与锁测试另经 race 检查。

可复算命令：

```sh
tongstock backfill states --from 1990-01-01 --mode full --verify
tongstock backfill states --from 1990-01-01 --mode incremental --verify
go test ./pkg/signalstate ./pkg/statesnapshot ./pkg/tdx ./pkg/server ./cmd/cli
```

读取入口：`GET /api/states?date=20261110&codes=000001,600519` 与
`GET /api/states/series?code=600519&from=20261104&to=20261110`。
第 1 期现算接口保留用于对照。操作策略见 [signal-state-backfill.md](signal-state-backfill.md)。
