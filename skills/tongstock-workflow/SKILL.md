---
name: tongstock-workflow
description: "Chinese A-share stock analysis workflows using TongStock CLI (Shanghai, Shenzhen, Beijing exchanges only). Use when user asks to analyze a stock, screen stocks by fundamentals, check dividend history, compare sector performance, build a stock research report, analyze technical indicators, screen stocks by signals, or pull latest company news. Triggers on: analyze stock, stock screening, 股票分析, 基本面, 选股, research report, sector analysis, dividend analysis, 技术指标, MACD, KDJ, BOLL, RSI, 信号筛选, 指标分析, indicator, screen, signal, golden cross, death cross, overbought, oversold, stock news, 个股新闻, 新闻资讯, 研报, 快讯, 公司新闻."
license: MIT
allowed-tools: Bash
---

# TongStock Analysis Workflows

Pre-built workflows for Chinese A-share analysis using `tongstock-cli`. Each workflow combines multiple data sources into actionable output.

## Workflow 1: Single Stock Deep Analysis (个股深度分析)

Full research report for one stock. Run all steps and synthesize.

```bash
# Step 1: Real-time quote with 5-level bid/ask
tongstock quote <code>

# Step 2: Financial fundamentals
tongstock finance <code>

# Step 3: Ex-rights/dividend history
tongstock xdxr <code>

# Step 4: Recent daily K-lines (price trend)
tongstock kline -c <code> -t day

# Step 5: Company F10 info categories
tongstock company <code>

# Step 6: Get specific F10 content
tongstock company-content <code> --block "公司概况"  # Company profile
tongstock company-content <code> --block "财务分析"  # Financial analysis
tongstock company-content <code> --block "股东研究"  # Shareholder research

# Step 7: Stock basic info (from local DB)
tongstock stockinfo <code>
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
tongstock codes -e sz > /tmp/sz_codes.txt

# Step 2: For each candidate, fetch finance data
for code in 000001 600519 000858 601318; do
  echo "=== $code ==="
  tongstock finance $code
  echo ""
done

# Or use stockinfo for batch listing
tongstock stockinfo --exchange sh  # List all SH stocks
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
tongstock xdxr <code>
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
tongstock block -f block_fg.dat

# Step 2: List concept sectors
tongstock block -f block_gn.dat

# Step 3: For interesting sector stocks, get quotes
tongstock quote <code1> <code2> <code3>

# Step 4: Compare with index
tongstock index -c 999999 -t day
```

**Analysis approach:**
- Identify sector constituents from block data
- Compare individual stock performance vs. sector index
- Look for sector rotation signals (volume surge + price breakout)

## Workflow 5: Technical Quick Check (技术面速查)

Fast technical overview using multiple timeframes.

```bash
# Multi-timeframe K-lines
tongstock kline -c <code> -t day     # Trend
tongstock kline -c <code> -t 60m     # Intraday trend
tongstock kline -c <code> -t 5m      # Short-term momentum

# Today's tick-level activity
tongstock minute <code>              # Minute-by-minute
tongstock trade <code>               # Tick trades (买卖方向)
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
tongstock index -c 999999 -t day     # 上证指数
tongstock index -c 399001 -t day     # 深证成指
tongstock index -c 399006 -t day     # 创业板指
tongstock index -c 399300 -t day     # 沪深300
```

**Key metrics from index bars:**
- UpCount vs. DownCount → market breadth (涨跌家数)
- Volume trend → participation level
- Price vs. moving average crossovers

## Workflow 7: Server Management (服务管理)

Start, stop, check status, or restart the TongStock HTTP server.

```bash
# Start server in foreground (Ctrl+C to stop)
tongstock server

# Start server in background (daemon mode)
tongstock server --daemon

# Or use the dedicated start command
tongstock server start

# Check if server is running
tongstock server status

# Stop the running server
tongstock server stop

# Restart the server
tongstock server restart
```

**Daemon mode:**
- `tongstock server --daemon` or `tongstock server start` launches the server in the background
- PID is recorded in `~/.tongstock/server.pid` for process management
- Logs are written to `~/.tongstock/server.log`
- Graceful shutdown: SIGTERM with 8s timeout, then SIGKILL

## Workflow 8: HTTP API Batch Analysis (API 批量分析)

When the server is running, use HTTP API for programmatic access:

```bash
# Start server in background
tongstock server --daemon

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

## Workflow 7: Company F10 Information (公司F10资料)

Access detailed company information from TDX F10 data.

```bash
# List available F10 categories
tongstock company <code>

# Get specific F10 content
tongstock company-content <code> --block "公司概况"     # Company profile
tongstock company-content <code> --block "财务分析"     # Financial analysis
tongstock company-content <code> --block "股东研究"     # Shareholder research
tongstock company-content <code> --block "公司公告"     # Company announcements
tongstock company-content <code> --block "行业分析"     # Industry analysis

# JSON output for parsing
tongstock company <code> --json
tongstock company-content <code> --block "公司概况" --json
```

**Common F10 blocks:**
- 公司概况 - Company overview, business description
- 财务分析 - Detailed financial statements and analysis
- 股东研究 - Shareholder structure and institutional holdings
- 公司公告 - Company announcements and disclosures
- 行业分析 - Industry comparison and market position
- 资本运作 - Capital structure changes
- 分红送转 - Dividend and bonus share history

## Workflow 8: Stock Basic Info (股票基础信息)

Quick access to stock fundamentals from the local database.

```bash
# First, sync data from TDX (required before first use)
tongstock stockinfo sync           # Sync all exchanges
tongstock stockinfo sync --force   # Force full sync (ignore 24h freshness)

# Single stock info
tongstock stockinfo <code>

# List all stocks (top 50)
tongstock stockinfo

# Filter by exchange
tongstock stockinfo --exchange sh  # Shanghai
tongstock stockinfo --exchange sz  # Shenzhen
tongstock stockinfo --exchange bj  # Beijing

# JSON output
tongstock stockinfo <code> --json
```

**Fields available:**
- Price, Open, High, Low, LastClose - Price data
- Volume, Amount, TurnoverRate - Trading activity
- LiuTongGuBen, ZongGuBen - Share structure
- MarketCap, TotalMarketCap - Market valuation
- JingZiChan, JingLiRun, MeiGuJingZiChan - Financial metrics
- StFlag - ST stock indicator

**Note:** Data must be synced from TDX before use. The sync command pulls latest quotes and finance data for all A-share stocks.

## Workflow 9: Technical Indicator Analysis (技术指标分析)

Compute and display technical indicators for a single stock.

```bash
# Single stock with default parameters (table output)
tongstock indicator -c <code> -t day

# JSON format output (single day)
tongstock indicator -c <code> -t day --json

# JSON format with multiple days history
tongstock indicator -c <code> -t day --json --days 5

# All historical data
tongstock indicator -c <code> -t day --all

# Custom parameter config file
tongstock indicator -c <code> -t day --config configs/params.yaml

# Different timeframes
tongstock indicator -c <code> -t 60m    # 60-minute
tongstock indicator -c <code> -t week   # Weekly
```

## Workflow 10: Natural-Language Stock Screening (自然语言选股)

Use this workflow when the user describes conditions in plain Chinese or English, e.g. “找最近20日放量突破且 MACD 金叉的半导体股票”, “筛选自选股里超卖反弹的票”, or “找概念板块里多头排列的股票”. Translate the request into a concrete stock universe, timeframe, signal filters, and optional post-filters.

```bash
# 1. Resolve the universe
# Optional: inspect block files and constituents when the user mentions industry/concept/theme.
tongstock block -f block_fg.dat
tongstock block -f block_gn.dat

# 2. Run signal screening for candidate codes through the HTTP API when server is available.
# codes should be comma-separated, type can be day/week/60m/30m/15m.
curl -s "http://localhost:8080/api/screen?codes=<codes>&type=day" | jq .

# 3. For each shortlisted stock, collect evidence.
curl -s "http://localhost:8080/api/indicator?code=<code>&type=day&days=60" | jq .
curl -s "http://localhost:8080/api/signal-analysis?code=<code>&type=day" | jq .
curl -s "http://localhost:8080/api/finance?code=<code>" | jq .
```

**Translation rules:**
- “金叉 / golden cross” → signal contains `金叉`.
- “死叉 / death cross” → signal contains `死叉`.
- “超卖 / oversold” → signal contains `超卖`.
- “超买 / overbought” → signal contains `超买`.
- “多头排列 / uptrend alignment” → signal contains `多头排列`.
- “突破 / breakout” → combine latest close, MA trend, BOLL upper/lower, and volume expansion if available.
- “最近 N 日” → request indicator with `days=N` and only keep signals whose dates fall inside the window.
- “半导体/新能源/银行等板块” → use `block_gn.dat` for concepts first, then `block_fg.dat` for industries.

**Answer format:**
1. State the interpreted conditions and stock universe.
2. Return a ranked table: code, name, latest price/change, matched signals, signal date, strength, 5/10/20-day historical outcome if available.
3. Explain why each stock matched and list missing/uncertain data.
4. Include risk notes: data source is TDX, signals are not investment advice.

**Supported Indicators:**
- **MA**: 5, 10, 20, 60, 120 day moving averages
- **MACD**: DIF, DEA, Histogram (default: 12/26/9)
- **KDJ**: K, D, J values (default: 9/3/3)
- **BOLL**: Upper, Middle, Lower bands (default: 20/2.0)
- **RSI**: RSI6, RSI12, RSI24 (relative strength)
- **Volume Ratio**: Current volume / 5-day average volume

**Table Output includes:**
- Last 20 days: Date, Close, MA5/10/20/60/120, DIF/DEA/HIST, K/D/J, RSI6/12/24, UPPER/MID/LOWER, Volume Ratio
- Latest signals: 金叉 (golden cross), 死叉 (death cross), 超买 (overbought), 超卖 (oversold), 多头排列 (bull alignment), 空头排列 (bear alignment), 突破上轨 (break upper band), 跌破下轨 (break lower band)

**JSON Output (single day):**
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

**Parameter resolution:**
- Per-stock override > Category override (large_cap/small_cap) > Default
- Categories auto-detected by code prefix (600xxx = large_cap, 002xxx = small_cap)

## Workflow 11: Batch Signal Screening (批量信号筛选)

Screen a list of stocks for specific signals using parallel computation.

```bash
# Screen specific stocks for golden cross
tongstock screen -c "000001,600519,000858,601318" -t day -s golden_cross

# Screen from file (one code per line)
tongstock screen -f codes.txt -t day -s oversold

# Screen with concurrency control
tongstock screen -c "000001,600519" -p 5 -s death_cross
```

**Available signal filters (-s):**
| Signal | Description |
|--------|-------------|
| `golden_cross` | DIF crosses above DEA (MACD), or K crosses above D (KDJ) |
| `death_cross` | DIF crosses below DEA (MACD), or K crosses below D (KDJ) |
| `overbought` | J > 100 (KDJ) or RSI > 80 |
| `oversold` | J < 0 (KDJ) or RSI < 20 |

**Combination with sector analysis:**
```bash
# Step 1: Get sector stocks
tongstock block -f block_fg.dat | grep "银行" > banking.txt

# Step 2: Screen for signals
tongstock screen -f banking.txt -t day -s golden_cross -p 8
```

**Output table columns:**
- Code, Date, Close, MA5/10/20, DIF, K, J, Latest Signals

## Workflow 12: 个股新闻资讯 (Stock News)

Combine latest company news with the existing fundamentals/technicals to produce a more complete read on a stock. News data comes from 东方财富 (新闻/研报) and 财联社 (快讯), associated to the stock via entity recognition.

```bash
# 1. Latest strong-related news for the stock (titles + native/title hits)
tongstock news query <code>

# 2. Recent news only (e.g. last 7 days), JSON for easy parsing
tongstock news query <code> --days 7 --json

# 3. Include weak mentions (only referenced in body text)
tongstock news query <code> --all-mentions

# 4. Merge with fundamentals + technicals for full context
tongstock finance <code>
tongstock indicator -c <code> -t day --json

# 5. Get company F10 info for deeper context
tongstock company-content <code> --block "公司概况"
```

**How to interpret the news output:**
- `关联=原生` — data source natively tagged the stock (highest confidence, e.g. Cailianpress `stock_list`, Eastmoney research `stockName`).
- `关联=标题命中` — stock short name matched in the headline (high confidence ≥ 0.9).
- `关联=正文提及` — only mentioned in the body (weak; filtered by default threshold 0.5, use `--all-mentions`).
- `weakCount` — number of body-only mentions hidden by the confidence threshold.
- `status`:
  - `ok` — fresh data fetched/served.
  - `stale` — data sources failed, serving cached data (see `degraded`).
  - `insufficient_data` — nothing in cache and `cache_only` was requested (or service not enabled).

**Recommended analysis flow:**
1. Lead with `news query <code>` to surface what the market is currently talking about (events, ratings, announcements).
2. Cross-check against `finance` (profit/dividend trends) and `indicator` (trend/signals) from Workflow 8.
3. Pay attention to `研报` (research reports) for analyst rating changes and `快讯` (flash news) for real-time catalysts.
4. Note risk: news is aggregated from third-party sources, not investment advice.

**Offline / deterministic mode:** pass `--consistency cache_only` (or `?consistency=cache_only` on the API) to read only locally cached news without network access. Use `--refresh` to force a re-fetch when you suspect stale data.

## Workflow 13: 热门股票 (Daily Hot Stocks)

Aggregate what the market is talking about on a given date (default: today). Pulls the global flash/article feeds from 东方财富 and 财联社, then ranks stocks by how many strongly-related news items mention them that day.

```bash
# 1. Today's hot stocks (auto-falls back to the most recent trading day on weekends/holidays)
tongstock topic

# 2. A specific date, JSON for parsing
tongstock topic 2026-09-17 --json
tongstock topic 20260917 --json

# 3. Full detail: representative headlines and sources per stock
tongstock topic --wide

# 4. Bigger board / strict date (no trading-day fallback)
tongstock topic --top 20
tongstock topic --strict
```

**How to interpret the output:**
- `提及 N 条` — number of strongly-related news items that day (title hits or source-native tags only; body-only mentions never count).
- `标题/原生` vs `原生` — how many were title/code matches vs natively tagged by the source.
- `热度` — composite ranking score (mentions + title/native hits + avg hot score + source diversity). Display only.
- `fallback` / 回溯提示 — the requested date was a non-trading day and the board actually covers the previous trading day; pass `--strict` to disable.
- `status`: `ok` / `stale` (sources failed, serving cached data) / `insufficient_data` (nothing for that date yet — try again later or run `tongstock news fetch --global`).

**Recommended flow:** pair `topic` with `news query <code>` for the stocks that make the board to drill into each stock's own news, then cross-check with `finance`/`indicator`/`company-content`.

**How to interpret the output:**
- `提及 N 条` — number of strongly-related news items that day (title hits or source-native tags only; body-only mentions never count).
- `标题/原生` vs `原生` — how many were title/code matches vs natively tagged by the source.
- `热度` — composite ranking score (mentions + title/native hits + avg hot score + source diversity). Display only.
- `fallback` / 回溯提示 — the requested date was a non-trading day and the board actually covers the previous trading day; pass `--strict` to disable.
- `status`: `ok` / `stale` (sources failed, serving cached data) / `insufficient_data` (nothing for that date yet — try again later or run `tongstock news fetch --global`).

**Recommended flow:** pair `topic` with `news query <code>` for the stocks that make the board to drill into each stock's own news, then cross-check with `finance`/`indicator`.

> Installation / upgrade: see the `tongstock-cli` skill — it auto-installs the latest GitHub Release binary (with `gh` draft fallback and source-build fallback).
