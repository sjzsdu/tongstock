# 股票数据迁移清单

统一 `stockdata.Service` 已覆盖三类新鲜度差异最大的核心数据；其余现有能力仍通过类型化 `pkg/tdx.Service`，入口层没有裸 `tdx.Client` 调用。后续迁移保持 URL/CLI 输出兼容。

| 数据类型 | 当前持久化/返回方式 | 新鲜度语义 | 状态/后续 |
|---|---|---|---|
| 日 K | `kline` + `data_sync_state`，提交后重读 | 已完成交易日的覆盖缺口 | 已迁移 |
| 行情快照 | `quote_snapshot` + 水位 | 盘中 15 秒，盘后最近完成交易日 | 已迁移 |
| 财务 | `finance_snapshot` + 水位 | 7 天财报 TTL | 已迁移 |
| 股票代码/证券元数据 | `cache` TTL；`stockinfo` 业务表 | 代码低频变更、行情字段更短 TTL | 下一阶段拆成 metadata read model |
| 分钟 K / 周月季年 K | TDX 类型化调用；部分由日线可派生 | 周期完成时间和请求覆盖 | 为每粒度增加独立覆盖键后迁移 |
| 分时/逐笔/集合竞价 | TDX 类型化调用 | 交易时段短 TTL，盘后不可变 | 增加按交易日分区的快照/明细表 |
| 除权除息 | `cache` 7 天 | 公司行动日期与修订时间 | 迁入 corporate_action 表 |
| 公司资料/F10 | `cache` 30 天 | 文件版本/更新时间 | 迁入文档元数据与内容表 |
| 板块 | `cache` 1 天 | 交易日版本 | 迁入 block + membership 版本表 |
| 新闻 | 共享 SQLite 的 `news_items`/`hot_events` | 来源发布时间与抓取水位 | 已统一连接，后续接统一 freshness 接口 |

每新增一种 read model 必须同时提供：主键/覆盖范围、`source_updated_at`、事务水位、业务校验、`RequireFresh`/`AllowStale`/`CacheOnly` 测试、CLI/API 适配，以及 diagnostics。
