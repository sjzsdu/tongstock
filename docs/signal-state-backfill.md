每日状态快照只读取本地 `kline_latest` raw 日线，不读取旧事件流水。每根实际 bar
产生四维状态行；停牌不产行，预热不足保留 `insufficient_data` 与独立布尔标记。

```sh
# 先通过 --codes 选择 200 只全历史试跑，对拍通过后再放全量。
tongstock backfill states --codes 000001,600519 --mode full --verify
# 全市场，无 500 只上限；from 之前的完整行情仍作为指标种子。
tongstock backfill states --from 1990-01-01 --mode full --verify
# 相同命令重跑应 changed=0，现有行不 UPDATE。
tongstock backfill states --from 1990-01-01 --mode full --verify
# 日线批量同步完成后，对同步成功代码自动执行此逻辑。
tongstock backfill states --mode incremental --verify
```

按代码排序，每股一个事务，四维状态及输入种子同事务提交。中断回滚当前股，
已提交股票保留，重跑同命令即可恢复。内存仅保留一股历史，指标通过第 1 期
`signalstate.ComputeSeries` / `ComputeSeriesFrom` 计算一次，不逐日前缀重复计算指标。
`--from` 只限制写入区间，不截断输入；首次局部回填后增量补齐历史缺口。
已有种子对应的参数或行情前缀变更时，自动重算整股，撤回 bar 同步删除快照。

`calc_version` 沿用内核版本，`adjust_version=raw`。`calc_hash` 为 SHA-256，
覆盖代码、日期、K 线类型、算法版本、归一化参数、该日前缀 OHLCVA 二进制值及
四维状态。追加未来行情不影响历史 hash，四维共享当天向量 hash，现算接口也
返回该 hash。`signal_state_seed.definition` 保留参数 JSON 及定义指纹。
参数或算法发布变更后应全量回填，避免不同股票混用口径。

增量检查输入指纹及每个实际交易日的四维行数。无变化时不重新分类；新增日期
或缺行时，以完整历史预热同一内核，仅分类缺失日期起的后缀、只写缺失日；历史修订重算整股。
不以固定回看窗口近似 MACD/KDJ 递归指标。`--verify` 逐维核对输出状态、hash、
insufficient、版本及覆盖行数；失败立即回滚当前股。汇总 JSON 写 stdout，
逐股进度写 stderr。失败返回非零状态，可直接重跑。

```text
GET /api/states?date=2026-11-10&codes=000001,600519
GET /api/states?date=20261110
GET /api/states/series?code=000001&from=20260101&to=20261110
```

读取接口仅访问快照库，无网络请求、无现场补算。`items` 是维度行数组，包含
code/trade_date/ktype/dim/state/insufficient/adjust_version/calc_version/calc_hash。
trade_date 为 YYYYMMDD，没有快照返回空数组。`dimensions` 来自唯一后端枚举。
第 1 期 `/api/stock/:code/state-series?adjust=raw` 保留，供现算对拍。
