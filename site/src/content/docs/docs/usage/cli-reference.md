---
title: CLI 命令与参数参考
description: 由 tongstock --help 实跑输出生成的命令与参数表，请勿手工编辑。
---

本页由 `site/scripts/gen-cli-docs.mjs` 调用二进制自身的 `--help` 生成，**不要手工编辑**。
参数的事实源永远是二进制：任何一处不一致，都以 `tongstock <cmd> --help` 的输出为准。

- 顶层命令：**27** 个（已省略 cobra 自动生成的 `completion` 与 `help`）
- 每个命令都有的 `-h, --help` 已省略，不重复列出
- 想给 AI 助手用，见 [AI 助手集成](/docs/usage/skills/)；想看决策流水线怎么串，见 [决策流水线](/docs/usage/decision-pipeline/)。

## 全局参数

下列参数对所有子命令生效，含义见[一致性模式](/docs/usage/consistency/)。

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--consistency` | string | "require_fresh" | 数据一致性: require_fresh, allow_stale, cache_only |
| `--refresh` | bool | — | 强制从 TDX 刷新后再从数据库读取 |

## 顶层命令一览

| 命令 | 作用 |
| --- | --- |
| [`tongstock acceptance`](#tongstock-acceptance) | 真实数据端到端验收 |
| [`tongstock arch`](#tongstock-arch) | 架构质量门: 依赖方向、循环引用、死代码、重复领域类型、Mock/合成结果扫描 |
| [`tongstock auction`](#tongstock-auction) | 查询集合竞价数据 |
| [`tongstock automation`](#tongstock-automation) | 每日真实数据选股、持仓判断和提醒闭环 |
| [`tongstock block`](#tongstock-block) | 查询板块分类信息 |
| [`tongstock codes`](#tongstock-codes) | 获取证券代码列表 |
| [`tongstock company`](#tongstock-company) | 获取指定股票的 F10 公司信息分类目录，包括公司概况、财务分析、股东研究等。 |
| [`tongstock count`](#tongstock-count) | 查询证券数量 |
| [`tongstock discover`](#tongstock-discover) | 用户只需提供股票代码。系统自动冻结真实数据，在发现区间搜索候选，预留未触碰样本，输出可执行方法和验证交接信息，不直接生成买入推荐。 |
| [`tongstock finance`](#tongstock-finance) | 获取指定股票的财务数据，包括总股本、流通股本、净资产、净利润、每股净资产等。 |
| [`tongstock indicator`](#tongstock-indicator) | 查询技术指标 |
| [`tongstock kline`](#tongstock-kline) | 查询K线数据 |
| [`tongstock market`](#tongstock-market) | 封装 全市场真实数据就绪与每日特征快照。 |
| [`tongstock method`](#tongstock-method) | 统一投资方法模型 (Unified Investment Method, UIM) 是所有策略的唯一入口。 |
| [`tongstock news`](#tongstock-news) | 查询个股新闻资讯 |
| [`tongstock position`](#tongstock-position) | 基于真实持仓和冻结快照判断何时卖 |
| [`tongstock quality`](#tongstock-quality) | 执行统一质量门检查, 覆盖数据质量、回测黄金集、范式阶段门、AI 评测、前向监控和恢复就绪 |
| [`tongstock quote`](#tongstock-quote) | 查询股票行情 |
| [`tongstock screen`](#tongstock-screen) | 批量筛选股票信号 |
| [`tongstock select`](#tongstock-select) | 在冻结真实数据上生成可审计的每日买入候选 |
| [`tongstock serve`](#tongstock-serve) | 启动、停止、查看状态或重启 TongStock HTTP 服务。不带子命令时默认以前台模式运行。旧命令名 server 仍可作为别名使用。 |
| [`tongstock stockinfo`](#tongstock-stockinfo) | 获取指定股票的基础信息，包括价格、市值、财务指标等。 |
| [`tongstock sync`](#tongstock-sync) | 同步本地行情数据 |
| [`tongstock topic`](#tongstock-topic) | 从各大新闻站点抓取指定日期的全局资讯，聚合出当日被新闻提及最多的热门股票。 |
| [`tongstock validate`](#tongstock-validate) | 验证工厂 (Validation Factory) 接收已编译方法 + 股票代码 + 日期范围， |
| [`tongstock watchlist`](#tongstock-watchlist) | 查看和管理自选股列表，数据与 web 端自选股页面完全同步。 |
| [`tongstock xdxr`](#tongstock-xdxr) | 获取指定股票的除权除息历史记录。 |

## `tongstock acceptance`

真实数据端到端验收

```bash
tongstock acceptance [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock acceptance verify`](#tongstock-acceptance-verify) | 验证正式数据库全链路血缘；缺失即失败 |

## `tongstock acceptance verify`

验证正式数据库全链路血缘；缺失即失败

```bash
tongstock acceptance verify [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock arch`

架构质量门: 依赖方向、循环引用、死代码、重复领域类型、Mock/合成结果扫描

```bash
tongstock arch [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock arch check`](#tongstock-arch-check) | 执行架构质量门检查；任何门失败都返回非零退出码 |

## `tongstock arch check`

执行架构质量门检查；任何门失败都返回非零退出码

```bash
tongstock arch check [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--json` | bool | — | 输出 JSON 格式 |
| `--root` | string | "." | Go 仓库根目录 |

## `tongstock auction`

查询集合竞价数据

```bash
tongstock auction [code] [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock automation`

每日真实数据选股、持仓判断和提醒闭环

```bash
tongstock automation [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock automation run`](#tongstock-automation-run) | 幂等运行一个冻结快照的每日闭环 |
| [`tongstock automation unlock`](#tongstock-automation-unlock) | 强制释放卡死的自动化任务锁（标记为 failed，下次 run 自动重试） |

## `tongstock automation run`

幂等运行一个冻结快照的每日闭环

```bash
tongstock automation run [snapshot_id] [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock automation unlock`

强制释放卡死的自动化任务锁（标记为 failed，下次 run 自动重试）

```bash
tongstock automation unlock <snapshot_id> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock block`

查询板块分类信息

```bash
tongstock block [command]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-f, --file` | string | "block_zs.dat" | 板块文件 |

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock block files`](#tongstock-block-files) | 列出所有可用的板块文件 |
| [`tongstock block list`](#tongstock-block-list) | 列出所有板块 [type, 板块编码, 板块名称, 成分股数量] |
| [`tongstock block show`](#tongstock-block-show) | 显示指定板块的成分股 (支持模糊搜索) |

## `tongstock block files`

列出所有可用的板块文件

```bash
tongstock block files [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock block list`

列出所有板块 [type, 板块编码, 板块名称, 成分股数量]

```bash
tongstock block list [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-f, --file` | string | "block_zs.dat" | 板块文件: block.dat/block_zs.dat/block_fg.dat/block_gn.dat |
| `-s, --sort` | bool | — | 按成分股数量排序 |
| `-t, --type` | string | — | 按Type过滤 (如: 2) |

## `tongstock block show`

显示指定板块的成分股 (支持模糊搜索)

```bash
tongstock block show [板块名称] [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-c, --code` | string | — | 根据股票代码查询所属板块 |
| `-f, --file` | string | "block_zs.dat" | 板块文件 |

## `tongstock codes`

获取证券代码列表

```bash
tongstock codes [command]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-e, --exchange` | string | "sz" | 交易所: sz/sh/bj |

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock codes list`](#tongstock-codes-list) | 列出证券代码 (支持分类过滤) |
| [`tongstock codes stats`](#tongstock-codes-stats) | 显示证券分类统计 |

## `tongstock codes list`

列出证券代码 (支持分类过滤)

```bash
tongstock codes list [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-c, --category` | string | — | 分类过滤: stock/fund/etf/bond/index/gem/all |

## `tongstock codes stats`

显示证券分类统计

```bash
tongstock codes stats [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-a, --all` | bool | — | 显示所有交易所统计 |

## `tongstock company`

获取指定股票的 F10 公司信息分类目录，包括公司概况、财务分析、股东研究等。
示例: tongstock company 000001 tongstock company 600519 --json

```bash
tongstock company <code> [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--json` | bool | — | 以 JSON 输出 |

## `tongstock count`

查询证券数量

```bash
tongstock count [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-e, --exchange` | string | "sz" | 交易所: sz/sh/bj |

## `tongstock discover`

用户只需提供股票代码。系统自动冻结真实数据，在发现区间搜索候选，预留未触碰样本，输出可执行方法和验证交接信息，不直接生成买入推荐。

```bash
tongstock discover [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock discover run`](#tongstock-discover-run) | 只输入股票代码启动真实数据规律发现 |
| [`tongstock discover show`](#tongstock-discover-show) | 查询并重新校验持久化的发现轨迹 |

## `tongstock discover run`

只输入股票代码启动真实数据规律发现

```bash
tongstock discover run [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--code` | strings | — | 股票代码，可重复或逗号分隔 |
| `--hold-days` | int | — | 候选持有交易日 (0=自动 5) |
| `--pool` | string | — | 股票池 ID：解析其过滤条件得到股票代码 |
| `--question` | string | — | 可选研究问题 |
| `--search-budget` | int | — | 搜索预算 (0=自动 24) |
| `--snapshot` | string | — | 已冻结数据快照 ID (空=自动创建) |

## `tongstock discover show`

查询并重新校验持久化的发现轨迹

```bash
tongstock discover show <research-id> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock finance`

获取指定股票的财务数据，包括总股本、流通股本、净资产、净利润、每股净资产等。
示例: tongstock finance 000001 tongstock finance 600519 --json

```bash
tongstock finance <code> [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--json` | bool | — | 以 JSON 输出 |

## `tongstock indicator`

查询技术指标

```bash
tongstock indicator [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-a, --all` | bool | — | 获取全部历史K线 |
| `-c, --code` | string | — | 股票代码 |
| `--config` | string | — | 参数配置文件路径 |
| `-n, --count` | int | 250 | K线数量 |
| `-d, --days` | int | 1 | JSON输出时返回的历史天数 |
| `-j, --json` | bool | — | JSON格式输出 |
| `-t, --type` | string | "day" | K线类型 |

## `tongstock kline`

查询K线数据

```bash
tongstock kline [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-a, --all` | bool | — | 获取全部历史K线 |
| `-c, --code` | string | — | 股票代码 |
| `-n, --count` | int | 500 | K线数量 |
| `-j, --json` | bool | — | JSON格式输出 |
| `-t, --type` | string | "day" | K线类型: 1m/minute, 5m, 15m, 30m, 60m, day, week, month, quarter, year |

## `tongstock market`

封装 全市场真实数据就绪与每日特征快照。
下游 (选股 / AI 挖掘 / 回测) 只能引用 status=ready 且 frozen=1 的 snapshot_id。
Fail closed: 如果 coverage / gap 未达阈值，扫描阶段不使用该快照。

```bash
tongstock market [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock market build`](#tongstock-market-build) | 构建指定交易日的 MarketSnapshot（可选 FeatureSnapshot） |
| [`tongstock market features`](#tongstock-market-features) | 在一个已 ready 的 MarketSnapshot 上构建 FeatureSnapshot |
| [`tongstock market freeze`](#tongstock-market-freeze) | 冻结一个 MarketSnapshot（之后任何修改都会失败） |
| [`tongstock market list`](#tongstock-market-list) | 列所有 MarketSnapshot（按 status 可选过滤） |
| [`tongstock market report`](#tongstock-market-report) | 导出 snapshot 的 JSON 报告（含所有代码水位 + feature 统计） |
| [`tongstock market show`](#tongstock-market-show) | 查看 snapshot；传 <id> 或 (date, universe) |

## `tongstock market build`

构建指定交易日的 MarketSnapshot（可选 FeatureSnapshot）

```bash
tongstock market build <YYYY-MM-DD> [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--adj` | string | "forward" | 价格口径: raw/forward/backward |
| `--codes` | string | — | 指定代码列表，用逗号分隔；会覆盖 universe 过滤 |
| `--coverage` | float | 0.99 | ready 覆盖阈值 (0,1] |
| `--max-gapped` | int | 50 | 最多允许多少只股票有 gap_days > 0 |
| `--universe` | string | "universe_usable" | 股票池: universe_usable / universe_csi800 / universe_all_a |

## `tongstock market features`

在一个已 ready 的 MarketSnapshot 上构建 FeatureSnapshot

```bash
tongstock market features <snapshot_id> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock market freeze`

冻结一个 MarketSnapshot（之后任何修改都会失败）

```bash
tongstock market freeze <id> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock market list`

列所有 MarketSnapshot（按 status 可选过滤）

```bash
tongstock market list [status] [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock market report`

导出 snapshot 的 JSON 报告（含所有代码水位 + feature 统计）

```bash
tongstock market report <id> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock market show`

查看 snapshot；传 <id> 或 (date, universe)

```bash
tongstock market show ([date] [universe] | <id>) [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--adj` | string | "forward" | 价格口径 |

## `tongstock method`

统一投资方法模型 (Unified Investment Method, UIM) 是所有策略的唯一入口。
任何自然语言、结构化规则或代码策略最终都被编译为 methods.CompiledMethod,
并通过确定性 executor 执行, 确保同一版本在任何环境下结果一致。

```bash
tongstock method [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock method compile`](#tongstock-method-compile) | 将结构化候选编译为稳定 CompiledMethod JSON |
| [`tongstock method entry`](#tongstock-method-entry) | 在 K 线历史上执行入场规则, 打印匹配日期和 trace |
| [`tongstock method explain`](#tongstock-method-explain) | 解释 CompiledMethod 的诊断、歧义与修复建议 |
| [`tongstock method library`](#tongstock-method-library) | 可信投资方法库与机器生命周期 |
| [`tongstock method research`](#tongstock-method-research) | 由 AI 查找真实来源、拆分冲突规则并编译可验证方法 |

## `tongstock method compile`

将结构化候选编译为稳定 CompiledMethod JSON

```bash
tongstock method compile [candidate.json] [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-o, --output` | string | — | 写入编译 JSON 制品路径 |

## `tongstock method entry`

在 K 线历史上执行入场规则, 打印匹配日期和 trace

```bash
tongstock method entry <compiled.json> <kline.json> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock method explain`

解释 CompiledMethod 的诊断、歧义与修复建议

```bash
tongstock method explain [compiled.json] [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock method library`

可信投资方法库与机器生命周期

```bash
tongstock method library [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock method library pool`](#tongstock-method-library-pool) | 按最新冻结快照逐只解析方法的 scope，输出池内全部股票与关键属性 |
| [`tongstock method library seed`](#tongstock-method-library-seed) | 把内置示例方法（均线多头排列 / MACD 多头 / 量价突破）逐个编译， |
| [`tongstock method library stratify`](#tongstock-method-library-stratify) | 分层稳健性分析：同一个方法在不同结构的股票上表现不同。 |

## `tongstock method library pool`

按最新冻结快照逐只解析方法的 scope，输出池内全部股票与关键属性
（流通市值、户均持股、收盘价），并标注当日入场规则是否触发。
这是只读诊断视图，不修改方法库。默认只打印池内股票，--show all 包含被排除的。
用法示例： tongstock method library pool --method method-9626afeb68e7b3c3 tongstock method library pool --method method-9626afeb68e7b3c3 --export pool.csv

```bash
tongstock method library pool [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--export` | string | — | 全量导出 CSV 路径（可选） |
| `--method` | string | — | 方法 ID（必填） |
| `--show` | string | "in" | 显示范围: in（池内）/ triggered（当日触发）/ all（含排除） |

## `tongstock method library seed`

把内置示例方法（均线多头排列 / MACD 多头 / 量价突破）逐个编译，
在已冻结的真实日线快照上做全股票池回测，按验证工厂的真实结论注册到方法库。
未过门槛的方法会被如实标注为 rejected/candidate，绝不硬编码为 verified。
用法示例： tongstock method library seed --snapshot discovery-1786158672652829000 --max-codes 300

```bash
tongstock method library seed [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--candidate` | string | — | 自定义候选方法 JSON 文件（与内置种子同管线验证并注册） |
| `--end` | string | — | 新建快照的截止日 YYYY-MM-DD |
| `--keys` | strings | — | 只载入指定 key 的种子方法（可重复）；空=全部 |
| `--max-codes` | int | — | 从快照解析股票池时的上限（<=0 表示不设上限） |
| `--snapshot` | string | — | 已冻结的真实数据快照 ID（空=用 --universe/--start/--end 新建） |
| `--split` | string | — | 切分类型: fixed / walk_forward（空=walk_forward） |
| `--start` | string | — | 新建快照的起始日 YYYY-MM-DD |
| `--trials` | int | 1 | 发现阶段尝试次数（多重检验惩罚） |
| `--universe` | string | — | 股票代码列表（逗号分隔）；空=从快照解析 |

## `tongstock method library stratify`

分层稳健性分析：同一个方法在不同结构的股票上表现不同。
本命令把验证股票池按「全窗口历史日均成交额」升序等分为 N 层（默认 3 层），
逐层用与正式验证完全相同的回测引擎、切分方式和股票池重算样本外统计，
用于回答「该方法在哪类股票上有效、在哪类股票上失效」。
这是研究分析工具：只读活表数据与注册表，不修改方法库，不产出新的晋级证据。
用法示例： tongstock method library stratify --method method-9626afeb68e7b3c3

```bash
tongstock method library stratify [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--by` | string | "market_cap" | 分层维度: market_cap / holders / cap_holders / amount / turnover（换手率）/ volatility（波动率）/ amplitude（振幅）/ momentum（60日动量） |
| `--emit-scope` | bool | — | 把最优层固化为方法 scope 条件并输出建议 JSON |
| `--groups` | int | 3 | 分层组数 |
| `--max-codes` | int | — | 股票池解析上限（<=0 表示不设上限，使用全部合格股票） |
| `--method` | string | — | 方法 ID（必填） |
| `--regime` | bool | true | 输出市场环境标注（池内等权指数 vs 60日均线 的多头/空头分段统计） |
| `--snapshot` | string | — | （已废弃，兼容保留并被忽略）数据固定读活表全量 |
| `--split` | string | — | 切分类型: fixed / walk_forward（空=walk_forward） |

## `tongstock method research`

由 AI 查找真实来源、拆分冲突规则并编译可验证方法

```bash
tongstock method research [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--code` | string | — | 可选真实股票代码；存在时自动创建验证交接 |
| `--model` | string | — | 可选 AI 模型覆盖 |
| `--name` | string | — | 方法名称，例如：杨永兴隔夜套利法 |
| `--text` | string | — | 方法自然语言描述（AI 会查找来源） |
| `--url` | string | — | 方法资料 URL（AI 会交叉核验） |

## `tongstock news`

查询个股新闻资讯

```bash
tongstock news [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock news fetch`](#tongstock-news-fetch) | 手动抓取资讯入库 |
| [`tongstock news query`](#tongstock-news-query) | 查询某只股票的最新资讯，数据来自东方财富（新闻/研报）与财联社（快讯）。 |
| [`tongstock news reindex`](#tongstock-news-reindex) | 识别规则升级后，存量关联不会被重新抓取覆盖（数据源只返回最新一批新闻）， |
| [`tongstock news summarize`](#tongstock-news-summarize) | 把某只股票在指定日期（或最近 N 天）内、来自各渠道（财联社/东方财富/雪球/巨潮资讯等）的 |

## `tongstock news fetch`

手动抓取资讯入库

```bash
tongstock news fetch [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--global` | bool | — | 抓取全局快讯流而非指定个股 |

## `tongstock news query`

查询某只股票的最新资讯，数据来自东方财富（新闻/研报）与财联社（快讯）。
默认只显示「标题命中股票」或「数据源原生关联」的强相关资讯；
加 --all-mentions 可包含仅在正文中被提及的内容。
一致性遵循与其他命令相同的语义： require_fresh（默认） 必要时抓取，源全部失败则报错 allow_stale 源失败时返回库内已有数据 cache_only 只读库，不访问网络

```bash
tongstock news query [code] [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--all-mentions` | bool | — | 包含仅在正文中被提及的弱关联资讯 |
| `-d, --days` | int | — | 只返回最近 N 天（0 表示不限） |
| `--json` | bool | — | 以 JSON 输出 |
| `-n, --limit` | int | 20 | 返回条数 |
| `--min-confidence` | float | — | 关联置信度下限 [0,1]，0 表示用默认值 0.5 |
| `-t, --type` | string | — | 资讯类型: 新闻/研报/快讯/公告/其他 |

## `tongstock news reindex`

识别规则升级后，存量关联不会被重新抓取覆盖（数据源只返回最新一批新闻），
需要显式重建。重建会按当前规则重算本地识别（代码/简称命中），
数据源原生关联与数据源给定的置信度原样保留。
纯本地操作，不访问网络。典型场景：升级后修正历史数据里的泛词误命中
（如 *ST发展 被「服务业发展」这类标题关联）。

```bash
tongstock news reindex [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock news summarize`

把某只股票在指定日期（或最近 N 天）内、来自各渠道（财联社/东方财富/雪球/巨潮资讯等）的
资讯交给 AI 简报员（news-summarize agent），输出一份简报，而不是原始资讯列表。
示例： tongstock news summarize 001216 # 今天 tongstock news summarize 001216 --date 2026-09-21 # 指定日期 tongstock news summarize 001216 --days 7 # 最近 7 天 tongstock news summarize 001216 --raw # 不调 AI，输出原始聚合（调试用）
默认路径依赖 agent.enabled=true；一致性语义与其他命令相同：
require_fresh / allow_stale / cache_only。

```bash
tongstock news summarize [code] [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--all-mentions` | bool | true | 包含仅在正文中被提及的弱关联资讯 |
| `--date` | string | — | 指定日期 YYYY-MM-DD（默认今天，与 --days 互斥） |
| `-d, --days` | int | 1 | 回溯最近 N 天（默认 1 即当天） |
| `--json` | bool | — | 以 JSON 输出 |
| `-n, --limit` | int | 200 | 最多汇总的条数 |
| `--model` | string | — | 可选 AI 模型覆盖 |
| `--raw` | bool | — | 跳过 AI，输出原始聚合摘要（调试用） |

## `tongstock position`

基于真实持仓和冻结快照判断何时卖

```bash
tongstock position [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock position decide`](#tongstock-position-decide) | 生成不可变持仓决策 |

## `tongstock position decide`

生成不可变持仓决策

```bash
tongstock position decide [market_snapshot_id] [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--feature-snapshot` | string | — | 指定特征快照 |

## `tongstock quality`

执行统一质量门检查, 覆盖数据质量、回测黄金集、范式阶段门、AI 评测、前向监控和恢复就绪

```bash
tongstock quality [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock quality check`](#tongstock-quality-check) | 执行全量质量检查 |
| [`tongstock quality report`](#tongstock-quality-report) | 查看质量报告 |
| [`tongstock quality status`](#tongstock-quality-status) | 查看质量门配置与状态 |
| [`tongstock quality verify`](#tongstock-quality-verify) | 快速验证: 数据质量 + 回测黄金集 |

## `tongstock quality check`

执行全量质量检查

```bash
tongstock quality check [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--backup-file` | string | — | 实际备份文件路径（用于恢复就绪检查） |
| `--block` | bool | — | 退出码: 有 block 时返回非零 |
| `--codes` | string | — | 股票代码列表 (逗号分隔, 如 sh000001,sz399001) |
| `--degrade-mode` | string | — | 已配置的降级模式: safe_mode/readonly/no_forward |
| `--end` | string | — | 结束日期 (YYYYMMDD) |
| `--golden` | bool | — | 运行黄金回测测试集 |
| `--json` | bool | — | 输出 JSON 格式 |
| `--kline-type` | string | "day" | K 线类型: day/week/month/5min/15min/30min/60min |
| `--skip-ai` | bool | — | 跳过 AI 评测检查 |
| `--skip-backtest` | bool | — | 跳过回测黄金集检查 |
| `--skip-data` | bool | — | 跳过数据质量检查 |
| `--skip-forward` | bool | — | 跳过前向监控检查 |
| `--skip-paradigm` | bool | — | 跳过范式阶段检查 |
| `--skip-recovery` | bool | — | 跳过恢复就绪检查 |
| `--source-id` | string | — | 来源 ID (如 paradigm 版本 ID) |
| `--source-type` | string | "system" | 来源类型: paradigm/run/system |
| `--start` | string | — | 开始日期 (YYYYMMDD) |

## `tongstock quality report`

查看质量报告

```bash
tongstock quality report [id] [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock quality status`

查看质量门配置与状态

```bash
tongstock quality status [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock quality verify`

快速验证: 数据质量 + 回测黄金集

```bash
tongstock quality verify [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock quote`

查询股票行情

```bash
tongstock quote [codes...] [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock screen`

批量筛选股票信号

```bash
tongstock screen [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-c, --codes` | string | — | 逗号分隔的股票代码列表 |
| `-f, --file` | string | — | 股票代码文件路径（每行一个代码） |
| `-p, --pool` | int | 10 | 并发池大小 |
| `-s, --signal` | string | — | 筛选信号类型: golden_cross/death_cross/overbought/oversold |
| `-t, --type` | string | "day" | K线类型 |

## `tongstock select`

在冻结真实数据上生成可审计的每日买入候选

```bash
tongstock select [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock select list`](#tongstock-select-list) | 列出历史选股运行 |
| [`tongstock select run`](#tongstock-select-run) | 运行全部可信方法；省略 snapshot 时使用最新 ready 快照 |
| [`tongstock select show`](#tongstock-select-show) | 查看一次不可变选股运行 |

## `tongstock select list`

列出历史选股运行

```bash
tongstock select list [date] [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock select run`

运行全部可信方法；省略 snapshot 时使用最新 ready 快照

```bash
tongstock select run [market_snapshot_id] [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--feature-snapshot` | string | — | 指定 FeatureSnapshot，默认取绑定快照的最新 ready 版本 |

## `tongstock select show`

查看一次不可变选股运行

```bash
tongstock select show <run_id> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock serve`

启动、停止、查看状态或重启 TongStock HTTP 服务。不带子命令时默认以前台模式运行。旧命令名 server 仍可作为别名使用。

```bash
tongstock serve [flags]
tongstock serve [command]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--daemon` | bool | — | 以后台模式运行 |

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock serve restart`](#tongstock-serve-restart) | 停止并重新启动 TongStock 服务。 |
| [`tongstock serve start`](#tongstock-serve-start) | 在后台启动 TongStock HTTP 服务，自动写入 PID 文件以便后续管理。 |
| [`tongstock serve status`](#tongstock-serve-status) | 显示 TongStock 服务的运行状态、PID 和端口信息。 |
| [`tongstock serve stop`](#tongstock-serve-stop) | 优雅停止正在运行的 TongStock 服务（先 SIGTERM，超时后 SIGKILL）。 |

## `tongstock serve restart`

停止并重新启动 TongStock 服务。

```bash
tongstock serve restart [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock serve start`

在后台启动 TongStock HTTP 服务，自动写入 PID 文件以便后续管理。

```bash
tongstock serve start [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock serve status`

显示 TongStock 服务的运行状态、PID 和端口信息。

```bash
tongstock serve status [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock serve stop`

优雅停止正在运行的 TongStock 服务（先 SIGTERM，超时后 SIGKILL）。

```bash
tongstock serve stop [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock stockinfo`

获取指定股票的基础信息，包括价格、市值、财务指标等。
示例: tongstock stockinfo 000001 tongstock stockinfo 600519 --json tongstock stockinfo --exchange sh # 列出上交所所有股票 tongstock stockinfo sync # 从 TDX 同步数据

```bash
tongstock stockinfo [code] [flags]
tongstock stockinfo [command]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--exchange` | string | — | 按交易所筛选 (sh/sz/bj) |
| `--json` | bool | — | 以 JSON 输出 |

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock stockinfo sync`](#tongstock-stockinfo-sync) | 从通达信服务器拉取最新的股票行情和财务数据，写入本地 stockinfo 库。 |

## `tongstock stockinfo sync`

从通达信服务器拉取最新的股票行情和财务数据，写入本地 stockinfo 库。
示例: tongstock stockinfo sync # 同步所有交易所 tongstock stockinfo sync --force # 强制全量同步（忽略24小时新鲜窗口）

```bash
tongstock stockinfo sync [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--force` | bool | — | 强制全量同步 |

## `tongstock sync`

同步本地行情数据

```bash
tongstock sync [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock sync daily`](#tongstock-sync-daily) | 同步指定代码的日K数据 |

## `tongstock sync daily`

同步指定代码的日K数据

```bash
tongstock sync daily [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--codes` | string | — | 股票代码列表，逗号/空格分隔 |
| `--concurrency` | int | 3 | 并发数 |
| `--mode` | string | "auto" | 同步模式: auto/full/incremental |

## `tongstock topic`

从各大新闻站点抓取指定日期的全局资讯，聚合出当日被新闻提及最多的热门股票。
日期参数可省略（默认今天），支持 2006-01-02 与 20060102 两种写法。
topic 是从媒体新闻中聚合热点，与是否交易日无关。
数据来自东方财富、财联社、证券时报、21世纪经济报道、第一财经、新浪财经、腾讯财经，
关联通过实体识别与数据源原生标注完成；
只统计标题命中或数据源原生关联的强相关内容，正文偶然提及不会计入榜单。
一致性遵循与其他命令相同的语义： require_fresh（默认） 必要时抓取，源全部失败则报错 allow_stale 源失败时返回库内已有数据 cache_only 只读库，不访问网络

```bash
tongstock topic [date] [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--json` | bool | — | 以 JSON 输出 |
| `--min-confidence` | float | — | 关联置信度下限 [0,1]，0 表示用默认值 0.5 |
| `-n, --top` | int | 10 | 返回榜单条数 |
| `--wide` | bool | — | 输出完整明细（关联新闻标题与来源） |

## `tongstock validate`

验证工厂 (Validation Factory) 接收已编译方法 + 股票代码 + 日期范围，
在真实 K 线上执行确定性回测、walk-forward 切分、多重检验惩罚和 critic 反证，
输出 EvidenceBundle (置信等级 + promotion blockers + 结果哈希)。
流程：method compile > compiled.json && tongstock validate run --method compiled.json --code 600519
系统会根据真实库内最新交易日自动选取最近两年，并在回测前冻结不可变快照。

```bash
tongstock validate [command]
```

**Flags**

该命令没有自己的 flag。

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock validate evidence`](#tongstock-validate-evidence) | 按结果哈希查询已持久化 EvidenceBundle |
| [`tongstock validate list`](#tongstock-validate-list) | 按方法哈希列出可引用的验证制品 |
| [`tongstock validate run`](#tongstock-validate-run) | 执行验证流水线并输出 EvidenceBundle JSON |

## `tongstock validate evidence`

按结果哈希查询已持久化 EvidenceBundle

```bash
tongstock validate evidence <result-hash> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock validate list`

按方法哈希列出可引用的验证制品

```bash
tongstock validate list [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--limit` | int | 20 | 最多返回制品数 (1-100) |
| `--method-hash` | string | — | 已编译方法内容哈希 |

## `tongstock validate run`

执行验证流水线并输出 EvidenceBundle JSON

```bash
tongstock validate run [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--benchmark` | string | — | 基准代码 (空=同标的买入持有) |
| `--cash` | float | 1e+06 | 初始资金 |
| `--code` | string | — | 股票代码 (如 600519) |
| `--end` | string | — | 截止日期 YYYY-MM-DD |
| `--method` | string | — | 已编译方法 JSON 文件路径 |
| `--output` | string | — | EvidenceBundle JSON 制品路径 (可选) |
| `--snapshot` | string | — | 已冻结数据快照 ID (空=自动从真实行情创建) |
| `--split` | string | — | 切分类型: fixed / walk_forward (空=walk_forward) |
| `--start` | string | — | 起始日期 YYYY-MM-DD (空=快照/最近2年) |
| `--trials` | int | 1 | 发现阶段尝试次数 (用于 Bonferroni 多重检验惩罚) |

## `tongstock watchlist`

查看和管理自选股列表，数据与 web 端自选股页面完全同步。
不带子命令时等同于 list。
示例: tongstock watchlist # 列出全部自选股 tongstock watchlist add 600519 002714 # 添加自选（自动补全名称） tongstock watchlist add 300750 --group 行业 --note "长线观察" tongstock watchlist remove 600519 # 移除自选 tongstock watchlist group 600519 custom # 修改分组 tongstock watchlist note 600519 "止损位 15.00" # 修改备注

```bash
tongstock watchlist [flags]
tongstock watchlist [command]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-g, --group` | string | — | 只列出指定分组的自选股 |
| `--json` | bool | — | 以 JSON 输出 |

**子命令**

| 子命令 | 作用 |
| --- | --- |
| [`tongstock watchlist add`](#tongstock-watchlist-add) | 添加一只或多只股票到自选列表。 |
| [`tongstock watchlist group`](#tongstock-watchlist-group) | 修改自选股分组 |
| [`tongstock watchlist note`](#tongstock-watchlist-note) | 修改自选股备注 |
| [`tongstock watchlist remove`](#tongstock-watchlist-remove) | 移除自选股 |

## `tongstock watchlist add`

添加一只或多只股票到自选列表。
代码支持 600519、sh600519、600519.SH 等写法；
名称优先用 --name，否则从本地 stockinfo 库自动补全，缺失时回退为代码本身。

```bash
tongstock watchlist add <code> [code...] [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--group` | string | "default" | 分组 |
| `--name` | string | — | 股票名称（默认从本地 stockinfo 库补全） |
| `--note` | string | — | 备注 |

## `tongstock watchlist group`

修改自选股分组

```bash
tongstock watchlist group <code> <group> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock watchlist note`

修改自选股备注

```bash
tongstock watchlist note <code> <note> [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock watchlist remove`

移除自选股

```bash
tongstock watchlist remove <code> [code...] [flags]
```

**Flags**

该命令没有自己的 flag。

## `tongstock xdxr`

获取指定股票的除权除息历史记录。
示例: tongstock xdxr 000001 tongstock xdxr 600519 --json

```bash
tongstock xdxr <code> [flags]
```

**Flags**

| Flag | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--json` | bool | — | 以 JSON 输出 |

:::note[怎么读这张表]
`类型` 列里的 `bool` 表示开关型 flag，出现即生效；其余类型需要跟一个值。
`默认值` 列显示 `—` 表示该 flag 没有默认值（通常是空字符串或零值）。
:::
