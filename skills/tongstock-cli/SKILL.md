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

### codes — Stock Code List

```bash
tongstock-cli codes [--exchange <sz|sh|bj>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--exchange`, `-e` | `sz` | Exchange: `sz` (Shenzhen), `sh` (Shanghai), `bj` (Beijing) |

```bash
tongstock-cli codes                     # Shenzhen (default)
tongstock-cli codes -e sh               # Shanghai
tongstock-cli codes -e bj               # Beijing
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
tongstock-cli company-content 000001                          # Basic usage (auto-detect filename)
tongstock-cli company-content 000001 --block "公司概况"       # By block name
tongstock-cli company-content 000001 --block "财务分析"       # Financial analysis
tongstock-cli company-content 000001 --block "股东研究"       # Shareholder research
tongstock-cli company-content 000001 --start 30744 --length 9560  # By range
tongstock-cli company-content 000001 000001.txt               # Specify filename
```

### block — Sector Classification (板块分类)

```bash
tongstock-cli block [--file <filename>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--file`, `-f` | `block_zs.dat` | Block file |

Available block files:

| File | Description |
|------|-------------|
| `block_zs.dat` | Index sectors (指数板块) |
| `block_fg.dat` | Industry sectors (行业板块) |
| `block_gn.dat` | Concept sectors (概念板块) |
| `block.dat` | General classification |

```bash
tongstock-cli block -f block_zs.dat     # Index sectors
tongstock-cli block -f block_fg.dat     # Industry sectors
tongstock-cli block -f block_gn.dat     # Concept sectors
```

## HTTP API

Start server: `tongstock-server` (listens on `:8080`)

| Endpoint | Params | Description |
|----------|--------|-------------|
| `GET /health` | - | Health check |
| `GET /api/quote` | `code` | Real-time quote |
| `GET /api/codes` | `exchange` | Stock code list |
| `GET /api/kline` | `code`, `type` | K-line data |
| `GET /api/index` | `code`, `type` | Index K-line |
| `GET /api/minute` | `code` | Intraday minute |
| `GET /api/trade` | `code`, `start`, `count`, `date`, `history` | Tick trades |
| `GET /api/xdxr` | `code` | Ex-rights/dividends |
| `GET /api/finance` | `code` | Financial data |
| `GET /api/company` | `code` | F10 category list |
| `GET /api/company/content` | `code`, `filename` | F10 content |
| `GET /api/block` | `file` | Sector classification |

```bash
curl "http://localhost:8080/api/quote?code=000001"
curl "http://localhost:8080/api/kline?code=000001&type=day"
curl "http://localhost:8080/api/finance?code=600519"
curl "http://localhost:8080/api/xdxr?code=000001"
curl "http://localhost:8080/api/index?code=999999&type=day"
curl "http://localhost:8080/api/company?code=000001"
curl "http://localhost:8080/api/company/content?code=000001&filename=000001.txt"
curl "http://localhost:8080/api/block?file=block_fg.dat"
```

## Stock Code Conventions

| Prefix | Exchange | Examples |
|--------|----------|----------|
| `6xxxxx` | Shanghai (SH) | 600000, 601318, 688xxx (科创板) |
| `0xxxxx` | Shenzhen (SZ) | 000001, 002xxx |
| `3xxxxx` | Shenzhen (SZ) | 300xxx (创业板), 399xxx (深证指数) |
| `8xxxxx` | Beijing (BJ) | 830xxx, 831xxx |
| `9xxxxx` | Shanghai (SH) | 999999 (上证指数) |

Codes can be passed as 6-digit (auto-detected) or 8-digit with prefix (`sh600000`, `sz000001`, `bj830001`).
