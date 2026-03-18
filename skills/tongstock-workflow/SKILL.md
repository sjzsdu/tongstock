---
name: tongstock-workflow
description: "Chinese A-share stock analysis workflows using TongStock CLI (Shanghai, Shenzhen, Beijing exchanges only). Use when user asks to analyze a stock, screen stocks by fundamentals, check dividend history, compare sector performance, or build a stock research report. Triggers on: analyze stock, stock screening, 股票分析, 基本面, 选股, research report, sector analysis, dividend analysis."
license: MIT
allowed-tools: Bash
---

# TongStock Analysis Workflows

Pre-built workflows for Chinese A-share analysis using `tongstock-cli`. Each workflow combines multiple data sources into actionable output.

## Workflow 1: Single Stock Deep Analysis (个股深度分析)

Full research report for one stock. Run all steps and synthesize.

```bash
# Step 1: Real-time quote with 5-level bid/ask
tongstock-cli quote <code>

# Step 2: Financial fundamentals
tongstock-cli finance <code>

# Step 3: Ex-rights/dividend history
tongstock-cli xdxr <code>

# Step 4: Recent daily K-lines (price trend)
tongstock-cli kline -c <code> -t day

# Step 5: Company F10 info categories
tongstock-cli company <code>
```

**Analysis checklist:**
- Current price vs. NAV per share → P/B ratio
- Net profit trend (from finance data)
- Dividend history frequency and amount (from xdxr)
- Recent price trend and volume pattern (from kline)
- Key support/resistance levels from K-line data

## Workflow 2: Stock Screening by Fundamentals (基本面选股)

Screen stocks by retrieving financial data for a batch of codes.

```bash
# Step 1: Get all stock codes for a market
tongstock-cli codes -e sz > /tmp/sz_codes.txt

# Step 2: For each candidate, fetch finance data
for code in 000001 600519 000858 601318; do
  echo "=== $code ==="
  tongstock-cli finance $code
  echo ""
done
```

**Screening criteria to evaluate:**
- Total shares & float shares → liquidity
- Net profit > 0 → profitable
- NAV per share → valuation floor
- Shareholder count trend → institutional interest
- Revenue scale → company size

## Workflow 3: Dividend Analysis (分红分析)

Find stocks with consistent dividend history.

```bash
# Get ex-rights/dividend records
tongstock-cli xdxr <code>
```

**What to look for in output:**
- Category = "除权除息" entries → actual dividend events
- `FenHong` field → cash dividend per share (元)
- `SongZhuanGu` field → bonus/transfer shares per 10 shares
- Frequency: annual dividends = positive signal
- Calculate dividend yield: FenHong / current_price × 100%

## Workflow 4: Sector/Industry Analysis (板块分析)

Find which stocks belong to a sector, then analyze the sector.

```bash
# Step 1: List industry sectors
tongstock-cli block -f block_fg.dat

# Step 2: List concept sectors
tongstock-cli block -f block_gn.dat

# Step 3: For interesting sector stocks, get quotes
tongstock-cli quote <code1> <code2> <code3>

# Step 4: Compare with index
tongstock-cli index -c 999999 -t day
```

**Analysis approach:**
- Identify sector constituents from block data
- Compare individual stock performance vs. sector index
- Look for sector rotation signals (volume surge + price breakout)

## Workflow 5: Technical Quick Check (技术面速查)

Fast technical overview using multiple timeframes.

```bash
# Multi-timeframe K-lines
tongstock-cli kline -c <code> -t day     # Trend
tongstock-cli kline -c <code> -t 60m     # Intraday trend
tongstock-cli kline -c <code> -t 5m      # Short-term momentum

# Today's tick-level activity
tongstock-cli minute <code>              # Minute-by-minute
tongstock-cli trade <code>               # Tick trades (买卖方向)
```

**What to evaluate:**
- Daily K: overall trend direction (uptrend/downtrend/sideways)
- 60m K: medium-term momentum
- 5m K: entry/exit timing
- Minute data: intraday price pattern
- Trade data: buy vs. sell pressure (Status field: 0=buy, 1=sell)

## Workflow 6: Market Overview (大盘概览)

Quick pulse of the overall market.

```bash
# Major indices
tongstock-cli index -c 999999 -t day     # 上证指数
tongstock-cli index -c 399001 -t day     # 深证成指
tongstock-cli index -c 399006 -t day     # 创业板指
tongstock-cli index -c 399300 -t day     # 沪深300
```

**Key metrics from index bars:**
- UpCount vs. DownCount → market breadth (涨跌家数)
- Volume trend → participation level
- Price vs. moving average crossovers

## Workflow 7: HTTP API Batch Analysis (API 批量分析)

When the server is running, use HTTP API for programmatic access:

```bash
# Start server in background
tongstock-server &

# Batch fetch via API (JSON output, easy to parse)
curl -s "http://localhost:8080/api/quote?code=000001" | jq .
curl -s "http://localhost:8080/api/finance?code=000001" | jq .
curl -s "http://localhost:8080/api/xdxr?code=000001" | jq .
curl -s "http://localhost:8080/api/kline?code=000001&type=day" | jq .

# Compare multiple stocks
for code in 000001 600519 000858; do
  echo "=== $code ==="
  curl -s "http://localhost:8080/api/finance?code=$code" | jq '{code: .code, net_profit: .JingLiRun, nav: .MeiGuJingZiChan, shareholders: .GuDongRenShu}'
done
```

## Output Interpretation Guide

### Quote Fields
| Field | Meaning |
|-------|---------|
| Price | Latest trade price |
| LastClose | Previous close (for calculating % change) |
| SVol | Inner volume 内盘 (seller-initiated) |
| BVol | Outer volume 外盘 (buyer-initiated) |
| BidAsk[0-4] | 5-level bid/ask depth |

### Finance Fields
| Field | Meaning | Unit |
|-------|---------|------|
| LiuTongGuBen | Float shares | 万股 |
| ZongGuBen | Total shares | 万股 |
| JingLiRun | Net profit | 万元 |
| MeiGuJingZiChan | NAV per share | 元 |
| GuDongRenShu | Shareholder count | 人 |
| ZhuYingShouRu | Revenue | 万元 |

### XdXr Categories
| Category | Meaning |
|----------|---------|
| 1 | 除权除息 (ex-dividend) |
| 2-10 | Share capital changes |
| 11-12 | Share consolidation |
| 13-14 | Warrant issuance |
