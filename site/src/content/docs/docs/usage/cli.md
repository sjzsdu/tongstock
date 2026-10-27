---
title: CLI 指南
description: 行情、指标、板块、公司信息的命令行用法，以及哪些数据查询走 CLI、哪些走 HTTP。
---

CLI 保留轻量查询命令；K 线、分时、分笔、除权除息、财务、指数 K 线、F10 等**重数据查询统一走 HTTP API**（也就是 Web UI 背后的接口）。这样拆分是为了避免在命令行里堆出几十个输出格式。

完整的命令与参数表见[CLI 命令与参数参考](/docs/usage/cli-reference/)，本页讲用法。

## 行情查询

```bash
./tongstock quote 000001
```

输出包含名称、最新价、开高低、成交量与成交额。多代码可以一起传。

## 股票代码列表

```bash
./tongstock codes list                 # 默认深圳市场
./tongstock codes list -e sh           # 上海
./tongstock codes list -e bj           # 北京
./tongstock codes list -e sz -c stock  # 只列股票
./tongstock codes list -e sz -c etf    # 只列 ETF

./tongstock codes stats                # 当前市场各分类统计
./tongstock codes stats --all          # 所有交易所
```

支持的分类：`all`、`stock`、`gem`（创业板）、`fund`、`etf`、`bond`、`index`。

## 证券数量与集合竞价

```bash
./tongstock count                       # 深圳（默认）
./tongstock count --exchange sh
./tongstock count --exchange bj

./tongstock auction 000001              # 集合竞价数据
```

## 板块

```bash
./tongstock block files                          # 列出所有板块文件
./tongstock block list -f block_zs.dat           # 指数板块
./tongstock block list -f block.dat -t 2         # 按 Type 过滤
./tongstock block list -f block_fg.dat -s        # 按成分股数量排序
./tongstock block show "沪深300" -f block_zs.dat # 板块成分股
./tongstock block show "银行" -f block_fg.dat    # 模糊搜索
./tongstock block show --code 600519             # 反查个股所属板块
```

| 文件 | 内容 |
| --- | --- |
| `block.dat` | 综合板块 |
| `block_zs.dat` | 指数板块（主要指数成分股） |
| `block_fg.dat` | 行业板块 |
| `block_gn.dat` | 概念板块 |

## 技术指标

```bash
# 单股，表格输出
./tongstock indicator --code 000001 --type day

# JSON 输出，默认只返回最新一天
./tongstock indicator --code 000001 --type day --json

# JSON 输出，返回最近 5 天
./tongstock indicator --code 000001 --type day --json --days 5

# 用全部历史 K 线计算
./tongstock indicator --code 000001 --type day --all

# 指定参与计算的 K 线数量
./tongstock indicator --code 000001 --type day --count 500

# 自定义参数文件
./tongstock indicator --code 000001 --type day --config configs/params.yaml
```

输出包含 K 线 + MA(5/10/20/60/120) + MACD(DIF/DEA/HIST) + KDJ(K/D/J) + BOLL(UPPER/MID/LOWER) + RSI(6/12/24) + 量比，以及最新信号（金叉/死叉/超买/超卖/多头排列/空头排列等）。

:::note[`--days` 和 `--count` 不是一回事]
`--count` 决定**参与计算**的 K 线数量，`--days` 决定**返回**多少天。要“多给我几天结果”用 `--days`；要“用更长历史算指标”用 `--count`。
:::

## 批量信号筛选

```bash
# 指定代码列表
./tongstock screen --codes "000001,600519,000858" --type day --signal golden_cross

# 从文件读代码（每行一个）
./tongstock screen --file codes.txt --type day --signal oversold

# 并发池大小（默认 10）
./tongstock screen --codes "000001,600519" --pool 5
```

可用信号：`golden_cross`、`death_cross`、`overbought`、`oversold`。

## 公司信息与财务

```bash
./tongstock company 000001                         # F10 目录
./tongstock company-content 000001 --block "公司概况"  # 按块名取内容
./tongstock finance 000001                         # 财务指标
./tongstock xdxr 000001                            # 除权除息
./tongstock stockinfo 000001                       # 本地库里的股票基础信息
```

## 资讯

```bash
./tongstock news                       # 个股相关新闻
./tongstock topic                      # 当日热点
```

资讯来源见[数据来源](/docs/data/sources/)。

## 全局参数：一致性模式

所有数据命令都接受同一组全局参数：

```bash
./tongstock quote 000001 --consistency=cache_only
./tongstock indicator --code 000001 --type day --refresh
```

- `--consistency`：`require_fresh`（默认）、`allow_stale`、`cache_only`
- `--refresh`：强制从 TDX 刷新后再读库（不能与 `cache_only` 同时使用）

语义与错误处理见[一致性模式](/docs/usage/consistency/)。

## 环境差异：装好的版本 vs 仓库构建

`make install` 装到 `~/.local/bin` 的二进制与仓库里 `./tongstock` 是同一个程序。如果你发现某个命令不存在，先确认版本：

```bash
tongstock --help | head -30
```

以 `--help` 的实际输出为准——本页描述的是仓库 `master` 上的命令集。
