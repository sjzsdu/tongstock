# 按状态筛选（第 3 期）

信号筛选读取 `signal_state_daily`，不拉取行情、不运行事件检测。界面与 CSV 显示所选交易日的四维状态，点击命中行打开该股图表并定位到同一日。

## 请求与语义

`GET /api/screen?codes=000001,600000&type=day&date=2026-11-10&state_filter=<URL 编码的 JSON>`

- `date` 接受 `YYYY-MM-DD` / `YYYYMMDD`；省略时统一使用快照库最新交易日，不能逐股回退到不同日期。
- 仅支持日线、未复权快照；股票池去重，不设原来的 500 只截断。
- `signals=` 已移除，传入时返回 400（包括空参数）。旧 OR 查询须迁移。
- `conditions` 全部同时满足。未指定的维度不加约束；中性必须显式指定。
- 四维中任一维数据不足或缺失，该股日均不参与匹配，计入 `insufficientCount`。完全没有当日快照的代码单独计入 `missingCount`，不伪造中性。
- 迁移条件与当日条件同样取 AND。迁移要求同股、同维、相邻观测从 `from` 中某态变成 `to`；前一观测或本次观测不足都不构成迁移。
- `within_days` 为 1–365 个**自然日**，窗口为 `[date - within_days + 1, date]`，包含所选日。前一观测允许在窗口之前；不会跨过数据不足的观测，也不会查看所选日之后的数据。
- 迁移后的态无需一直保持到所选日。如果需要“近 5 日转买且当日仍买”，额外指定该维 `state=buy`。

当日交叉=买 AND 均线=买：

```json
{"conditions":[{"dim":"cross","state":"buy"},{"dim":"ma","state":"buy"}]}
```

增加“超买超卖=中性”：

```json
{"conditions":[{"dim":"cross","state":"buy"},{"dim":"ma","state":"buy"},{"dim":"osc","state":"neutral"}]}
```

近 5 日交叉从死叉迁移为金叉（非买转买时 `from` 可填 `["sell","neutral"]`）：

```json
{"conditions":[],"transition":{"dim":"cross","from":["sell"],"to":"buy","within_days":5}}
```

非法维度、非三态、重复维度、同态迁移、空迁移起点和未知 JSON 字段返回 400。省略过滤对象或空条件列表表示不限定状态，但仍排除数据不足。

## 响应与界面

响应包含 `date`、`total`（去重股票池数）、`successCount`、`insufficientCount`、`missingCount`、`results` 与 `dimensions`。每条结果包含 `code/name/date/states` 和所选日 `last.Close/PrevClose`；本地没有价格时为 `null`。价格仅从本地日线读取，不回退为最新报价。

`GET /api/screen/options` 无需行情服务或快照初始化即可返回后端 `signalstate.Metadata()`。维度名、状态标签、方向标签 `direction_label`、颜色和说明均由此提供。前端选择器、说明弹窗、结果列和 CSV 直接消费元数据，不保留旧二态映射。数据不足态仅用于说明，不出现在筛选选项内。

界面不再复用旧事件筛选缓存或执行第二遍 OR 过滤。修改条件后提示结果仍来自上次执行；刷新使用上次的完整股票池，不会只重筛上次命中的代码。

结果链接为 `/stock/<code>/chart?date=YYYY-MM-DD`。价格主图和副图窗口定位到该日，状态带读数默认显示同一天；鼠标悬停时可继续查看其他日期。

## 验证

- `go test ./pkg/statesnapshot ./pkg/server ./pkg/signalstate`：AND 与 OR 的差异、显式中性、数据不足、缺失日期、迁移边界、无未来数据、数据不足不能桥接迁移、HTTP 参数验证与无行情服务读取。
- 5,252 只合成股票的数据库回归：命中超过 500 只，与独立 SQL 直查逐行一致。
- 可选真实库验收只执行读取（使用已回填数据库的只读 URI）：

```sh
TONGSTOCK_SNAPSHOT_PARITY_DSN='file:/path/to/tongstock.db?mode=ro' \
  go test ./pkg/statesnapshot -run TestScreenRealSnapshotParity -v
```

2026-11-11 对最新快照日 2026-11-10、5,189 只股票执行验收：交叉买 AND 均线买命中 391 只；再加超买超卖中性命中 347 只；近 5 自然日死叉转金叉命中 101 只。三项与独立 SQL 一致，均报告数据不足排除 27 个股日。该次筛选查询耗时分别约 2.65 s、68 ms、345 ms；独立 SQL 对照耗时另计。

浏览器验收使用真实库的隔离副本、80 只测试股票池：三条件命中 3 只，验证中性列、说明弹窗、条件变更提示及对应日期下钻。测试环境的股票池和报价入口为夹具，筛选、指标和状态序列使用生产处理器。
