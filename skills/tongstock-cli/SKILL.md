---
name: tongstock-cli
description: "TDX (通达信) CLI/HTTP API for Chinese A-share market data (Shanghai, Shenzhen, Beijing exchanges only). Supports: real-time 5-level bid/ask quotes, K-line (candlestick), intraday minute data, tick-by-tick trades, ex-rights/dividend history, financial statements, index bars, sector/industry classification, and company F10 info. Triggers on: stock quote, K-line, candlestick, A-share, 通达信, TDX, market data, 行情, K线, 除权除息, 财务数据, 板块."
license: MIT
allowed-tools: Bash
---

# TongStock CLI & HTTP API Reference

TDX (通达信) protocol client for Chinese A-share market data (Shanghai, Shenzhen, Beijing exchanges).

## Prerequisites

```bash
# Build from source
git clone https://github.com/sjzsdu/tongstock.git
cd tongstock
go build -o tongstock-cli ./cmd/cli
go build -o tongstock-server ./cmd/server

# Verify
./tongstock-cli --help
```

## CLI Commands

### quote — Real-time Quotes (五档行情)

```bash
tongstock-cli quote <code> [code2 ...]
```

Returns: price, open, high, low, volume, amount, last close, bid/ask 5 levels, inner/outer volume.

```bash
tongstock-cli quote 000001              # 平安银行
tongstock-cli quote 000001 600000       # Multiple stocks
tongstock-cli quote 600519              # 贵州茅台
```

### codes — Stock Code List (股票代码)

```bash
tongstock-cli codes <subcommand> [flags]
```

#### Subcommands

| Subcommand | Description |
|------------|-------------|
| `list` | 列出证券代码 (默认) |
| `stats` | 显示各分类统计信息 |

#### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--exchange`, `-e` | `sz` | Exchange: `sz`, `sh`, `bj` |
| `--category`, `-c` | `all` | Filter by category |
| `--sort`, `-s` | `false` | Sort by count |

#### Categories

| Category | Description |
|----------|-------------|
| `all` | 全部 (默认) |
| `stock` | 股票 |
| `gem` | 创业板 |
| `fund` | 基金 |
| `etf` | ETF |
| `bond` | 债券 |
| `index` | 指数 |

#### Examples

```bash
# 默认列出深圳市场所有证券
tongstock-cli codes list

# 上海市场
tongstock-cli codes list -e sh

# 北京市场
tongstock-cli codes list -e bj

# 按分类过滤 - 只显示股票
tongstock-cli codes list -e sz -c stock

# 按分类过滤 - 只显示ETF
tongstock-cli codes list -e sz -c etf

# 查看统计信息
tongstock-cli codes stats

# 查看所有交易所统计
tongstock-cli codes stats --all

# 按交易所过滤
tongstock-cli codes stats -e sh
```

### kline — K-line (Candlestick) Data

```bash
tongstock-cli kline --code <code> --type <type> [--all]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--code`, `-c` | required | Stock code |
| `--type`, `-t` | `day` | K-line type (see table below) |
| `--all`, `-a` | `false` | Fetch ALL historical data |

**K-line types:**

| Type | Description |
|------|-------------|
| `1m`, `minute` | 1-minute |
| `5m` | 5-minute |
| `15m` | 15-minute |
| `30m` | 30-minute |
| `60m` | 60-minute |
| `day` | Daily |
| `week` | Weekly |
| `month` | Monthly |
| `quarter` | Quarterly |
| `year` | Yearly |

```bash
tongstock-cli kline -c 000001 -t day
tongstock-cli kline -c 600519 -t week
tongstock-cli kline -c 000001 -t 5m
tongstock-cli kline -c 000001 -t day --all   # Full history
```

### index — Index K-line (指数K线)

```bash
tongstock-cli index --code <code> --type <type>
```

Same flags as `kline`. Returns additional fields: `UpCount` (上涨家数), `DownCount` (下跌家数).

Common index codes:
- `999999` — 上证指数
- `399001` — 深证成指
- `399006` — 创业板指
- `399300` — 沪深300

```bash
tongstock-cli index -c 999999 -t day     # 上证指数 daily
tongstock-cli index -c 399300 -t 5m      # 沪深300 5-minute
```

### minute — Intraday Minute Data (分时数据)

```bash
tongstock-cli minute <code>
```

```bash
tongstock-cli minute 000001
```

### trade — Tick-by-tick Trades (分笔成交)

```bash
tongstock-cli trade <code> [--history] [--date <YYYYMMDD>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--date`, `-d` | - | Date for history mode (YYYYMMDD) |
| `--history`, `-H` | `false` | Query historical trades |
| `--start`, `-s` | `0` | Start offset |
| `--count`, `-c` | `100` | Number of records |

```bash
tongstock-cli trade 000001                              # Today
tongstock-cli trade 000001 --history --date 20250314    # Historical
```

### xdxr — Ex-rights & Dividends (除权除息)

```bash
tongstock-cli xdxr <code>
```

Returns history of: dividends (分红), bonus shares (送股), rights issue (配股), share capital changes (股本变动).

```bash
tongstock-cli xdxr 000001
tongstock-cli xdxr 600519
```

### finance — Financial Data (财务数据)

```bash
tongstock-cli finance <code>
```

Returns: total shares, float shares, total assets, net assets, revenue, net profit, NAV per share, shareholder count, IPO date, etc.

```bash
tongstock-cli finance 000001
tongstock-cli finance 600519
```

### company — Company Info / F10 (公司信息)

```bash
tongstock-cli company <code>
```

Lists available F10 document categories (latest tips, company profile, financial analysis, shareholder research, etc).

```bash
tongstock-cli company 000001
```

### company-content — Company Info Content / F10 (公司信息内容)

```bash
tongstock-cli company-content <code> [filename] [--block <name>] [--start <offset>] [--length <length>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--block`, `-b` | - | Block name (e.g., "公司概况") |
| `--start`, `-s` | `0` | Start offset |
| `--length`, `-l` | `10000` | Content length |

**Available F10 blocks:**

| Block Name | Description |
|------------|-------------|
| 最新提示 | 公司最新动态、公告、报道等 |
| 公司概况 | 公司基本信息、主营业务等 |
| 财务分析 | 财务指标、报表分析等 |
| 股本结构 | 股本构成、股东持股情况等 |
| 股东研究 | 主要股东、股东变化等 |
| 机构持股 | 机构投资者持股情况 |
| 分红融资 | 分红历史、融资情况等 |
| 高管治理 | 公司管理层信息 |
| 资金动向 | 资金流入流出情况 |
| 资本运作 | 并购、重组等资本活动 |
| 热点题材 | 公司涉及的热点概念 |
| 公司公告 | 公司发布的正式公告 |
| 公司报道 | 媒体对公司的报道 |
| 经营分析 | 业务经营情况分析 |
| 行业分析 | 所属行业情况分析 |
| 研报评级 | 分析师研究报告和评级 |

```bash
tongstock-cli company-content 000001                          # Basic usage
tongstock-cli company-content 000001 --block "公司概况"       # By block name
tongstock-cli company-content 000001 --block "财务分析"       # Financial analysis
tongstock-cli company-content 000001 --start 30744 --length 9560  # By range
```

### block — Sector Classification (板块分类)

```bash
tongstock-cli block <subcommand> [flags]
```

#### Subcommands

| Subcommand | Description |
|------------|-------------|
| `files` | 列出所有可用的板块文件 |
| `list` | 列出所有板块 [type, 编码, 名称, 成分股数] |
| `show` | 显示指定板块的成分股 |

#### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--file`, `-f` | `block_zs.dat` | Block file |
| `--type`, `-t` | - | Filter by type (e.g., 2) |
| `--sort`, `-s` | `false` | Sort by stock count |
| `--code`, `-c` | - | Query blocks containing this stock |

#### Available Block Files

| File | Name | Description |
|------|------|-------------|
| `block.dat` | 综合板块 | 综合分类 |
| `block_zs.dat` | 指数板块 | 主要指数成分股 |
| `block_fg.dat` | 行业板块 | 行业分类 |
| `block_gn.dat` | 概念板块 | 概念主题 |

#### Examples

```bash
# 列出所有板块文件
tongstock-cli block files

# 列出指数板块（默认）
tongstock-cli block list -f block_zs.dat

# 按Type过滤（2=标准板块）
tongstock-cli block list -f block.dat -t 2

# 按成分股数量排序
tongstock-cli block list -f block_fg.dat -s

# 显示板块成分股
tongstock-cli block show "沪深300" -f block_zs.dat

# 模糊搜索板块
tongstock-cli block show "银行" -f block_fg.dat

# 按股票代码查询所属板块
tongstock-cli block show --code 600519
```

### indicator — Technical Indicators (技术指标)

```bash
tongstock-cli indicator --code <code> --type <type> [--all] [--count <n>] [--json]
```

Computes MACD, KDJ, MA(5/10/20/60/120), BOLL, RSI(6/12/24), Volume Ratio indicators with signal detection.

| Flag | Default | Description |
|------|---------|-------------|
| `--code`, `-c` | required | Stock code |
| `--type`, `-t` | `day` | K-line type |
| `--all`, `-a` | `false` | Fetch ALL historical data |
| `--count`, `-n` | `250` | Number of K-lines |
| `--json`, `-j` | `false` | JSON format output |
| `--days`, `-d` | `1` | Number of days to return in JSON output |
| `--config` | - | Custom parameter config file |

**Supported Indicators:**
- **MA**: 5, 10, 20, 60, 120 day moving averages
- **MACD**: DIF, DEA, Histogram (default: 12/26/9)
- **KDJ**: K, D, J values (default: 9/3/3)
- **BOLL**: Upper, Middle, Lower bands (default: 20/2.0)
- **RSI**: RSI6, RSI12, RSI24 (relative strength)
- **Volume Ratio**: Current volume / 5-day average volume

```bash
# Basic usage (table output)
tongstock-cli indicator -c 000001 -t day

# Full history
tongstock-cli indicator -c 000001 -t day --all

# Custom K-line count
tongstock-cli indicator -c 000001 -t day --count 500

# JSON output (for program parsing)
tongstock-cli indicator -c 000001 -t day --json
```

**JSON Output Format:**
```json
{
  "code": "000001",
  "name": "平安银行",
  "timestamp": "2026-03-29",
  "price": { "current": 12.58, "change": 0.45, "change_pct": 3.71 },
  "ma": { "ma5": 12.32, "ma10": 12.18, "ma20": 11.95, "ma60": 11.50, "ma120": 11.20, "trend": "bullish" },
  "macd": { "dif": 0.35, "dea": 0.22, "hist": 0.26, "signal": "golden_cross" },
  "kdj": { "k": 72.5, "d": 68.2, "j": 81.1, "signal": "overbought" },
  "rsi": { "rsi6": 65.2, "rsi12": 62.8, "rsi24": 58.4, "signal": "neutral" },
  "boll": { "upper": 13.20, "middle": 12.50, "lower": 11.80, "position": 0.65, "signal": "normal" },
  "volume": { "current": 1250000, "avg5": 980000, "ratio": 1.28, "signal": "active" },
  "signals": ["golden_cross", "overbought", "多头排列"],
  "summary": { "trend": "上升趋势", "signal": "持有", "strength": 72 }
}
```

### screen — Signal Screening (信号筛选)

```bash
tongstock-cli screen --codes <codes> --type <type> --signal <signal>
```

| Flag | Default | Description |
|------|---------|-------------|
| `--codes`, `-c` | - | Comma-separated stock codes |
| `--file`, `-f` | - | File with one code per line |
| `--type`, `-t` | `day` | K-line type |
| `--signal`, `-s` | required | Signal type |
| `--pool`, `-p` | `10` | Concurrency pool size |

**Available Signals:**
- `golden_cross` - 金叉 (DIF crosses above DEA, or K crosses above D)
- `death_cross` - 死叉 (DIF crosses below DEA, or K crosses below D)
- `overbought` - 超买 (J > 100 or RSI > 80)
- `oversold` - 超卖 (J < 0 or RSI < 20)

```bash
tongstock-cli screen -c "000001,600519" -t day -s golden_cross
tongstock-cli screen -f codes.txt -t day -s oversold
```

### count — Securities Count (证券数量)

```bash
tongstock-cli count [--exchange <sz|sh|bj>]
```

```bash
tongstock-cli count
tongstock-cli count -e sh
tongstock-cli count -e bj
```

### auction — Call Auction (集合竞价)

```bash
tongstock-cli auction <code>
```

```bash
tongstock-cli auction 000001
```

## HTTP API

Start server: `tongstock-server` (listens on `:8080`)

### Core APIs

| Endpoint | Params | Description |
|----------|--------|-------------|
| `GET /health` | - | Health check |
| `GET /api/quote` | `code` | Real-time quote |
| `GET /api/codes` | `exchange` | Stock code list (legacy) |
| `GET /api/codes/list` | `exchange`, `category` | Stock code list with filter |
| `GET /api/codes/stats` | `exchange`, `all` | Code statistics |
| `GET /api/kline` | `code`, `type`, `start`, `count` | K-line data |
| `GET /api/index` | `code`, `type` | Index K-line |
| `GET /api/minute` | `code`, `date`, `history` | Intraday minute |
| `GET /api/trade` | `code`, `start`, `count`, `date`, `history` | Tick trades |
| `GET /api/xdxr` | `code` | Ex-rights/dividends |
| `GET /api/finance` | `code` | Financial data |
| `GET /api/company` | `code` | F10 category list |
| `GET /api/company/content` | `code`, `filename` | F10 content |
| `GET /api/block` | `file` | Sector classification (legacy) |
| `GET /api/block/files` | - | List available block files |
| `GET /api/block/list` | `file`, `type`, `sort` | Structured block list |
| `GET /api/block/show` | `name`, `code`, `file` | Block stocks or query by stock |
| `GET /api/indicator` | `code`, `type`, `days` | Technical indicators (MA/MACD/KDJ/BOLL/RSI/VolumeRatio), days param limits K-line count |
| `GET /api/screen` | `codes`, `type`, `signal` | Signal screening |
| `GET /api/count` | `exchange` | Securities count |
| `GET /api/auction` | `code` | Call auction |

### API Examples

```bash
# 查询行情
curl "http://localhost:8080/api/quote?code=000001"

# 股票代码列表（带分类）
curl "http://localhost:8080/api/codes/list?exchange=sz&category=stock"
curl "http://localhost:8080/api/codes/list?exchange=sz&category=etf"

# 股票代码统计
curl "http://localhost:8080/api/codes/stats?exchange=sz"
curl "http://localhost:8080/api/codes/stats?all=true"

# 查询K线
curl "http://localhost:8080/api/kline?code=000001&type=day"

# 板块文件列表
curl "http://localhost:8080/api/block/files"

# 板块列表（过滤+排序）
curl "http://localhost:8080/api/block/list?file=block_zs.dat&type=2&sort=true"

# 板块成分股
curl "http://localhost:8080/api/block/show?name=沪深300&file=block_zs.dat"

# 按股票代码查询所属板块
curl "http://localhost:8080/api/block/show?code=600519"

# 查询财务数据
curl "http://localhost:8080/api/finance?code=600519"

# 查询除权除息
curl "http://localhost:8080/api/xdxr?code=000001"

# 技术指标
curl "http://localhost:8080/api/indicator?code=000001&type=day"
```

### Caching

Codes and Block APIs are cached in SQLite for 24 hours:
- `codes.db` - codes cache
- `blocks.db` - blocks cache

## Stock Code Conventions

| Prefix | Exchange | Examples |
|--------|----------|----------|
| `6xxxxx` | Shanghai (SH) | 600000, 601318, 688xxx (科创板) |
| `0xxxxx` | Shenzhen (SZ) | 000001, 002xxx |
| `3xxxxx` | Shenzhen (SZ) | 300xxx (创业板), 399xxx (深证指数) |
| `8xxxxx` | Beijing (BJ) | 830xxx, 831xxx |
| `9xxxxx` | Shanghai (SH) | 999999 (上证指数) |

Codes can be passed as 6-digit (auto-detected) or 8-digit with prefix (`sh600000`, `sz000001`, `bj830001`).