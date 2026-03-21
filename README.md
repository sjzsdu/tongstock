# TongStock 通达信股票数据查询工具

基于 Go 语言实现的 TDX (通达信) 行情数据客户端，支持 CLI 和 HTTP API 两种方式获取股票数据。

## 功能特性

- **实时行情** - 五档买卖盘、昨收价、内外盘、成交量/额
- **K线数据** - 支持 1分钟/5分钟/15分钟/30分钟/60分钟/日/周/月/季/年 K线
- **指数K线** - 指数专用K线，包含上涨/下跌家数
- **分时数据** - 当日及历史分时走势数据
- **分笔成交** - 当日及历史分笔成交数据
- **除权除息** - 分红、送股、配股、股本变动等历史记录
- **财务数据** - 总股本、流通股、净资产、净利润等核心财务指标
- **公司信息** - F10资料（最新提示、公司概况、财务分析等）
- **板块分类** - 行业、概念、地域、风格等板块分类数据
- **集合竞价** - 开盘前竞价阶段的匹配量、未匹配量等数据
- **证券数量** - 查询各交易所证券总数
- **股票代码** - 获取沪深北交易所所有股票代码
- **技术指标** - MACD/KDJ/MA/BOLL/RSI，支持参数化计算
- **信号检测** - 金叉/死叉/超买/超卖/突破，自动检测并标记
- **批量筛选** - 按板块或代码列表批量筛选信号，支持并发
- **双模式** - CLI 命令行工具 + HTTP REST API

## 安装

```bash
# 克隆项目
git clone https://github.com/sjzsdu/tongstock.git
cd tongstock

# 一键安装（需要 Go 1.24+ 和 pnpm）
bash setup.sh

# 或手动构建
pnpm install
make server
make cli
```

## Web UI

启动 server 后访问 `http://localhost:8080` 即可使用 Web 界面。

### 功能页面

| 页面 | 路径 | 功能 |
|------|------|------|
| 市场总览 | `/` | 主要指数行情 + 快速分析入口 |
| 指标分析 | `/stock` | 单股 MACD/KDJ/MA/BOLL 图表 + 信号标记 |
| 信号筛选 | `/screen` | 批量筛选金叉/死叉/超买/超卖 |

### 开发模式

```bash
cd web
npm install
npm run dev        # 启动开发服务器，默认代理到 localhost:8080
```

## CLI 使用方法

### 查询行情

```bash
./tongstock-cli quote 000001
```

输出示例：
```
000001 平安银行
  最新价: 12.350
  开盘: 12.200 最高: 12.400 最低: 12.150
  成交量: 1234.56 手
  成交额: 15234.56 万
```

### 获取股票代码列表

```bash
# 深圳市场 (默认)
./tongstock-cli codes

# 上海市场
./tongstock-cli codes --exchange sh

# 北京市场
./tongstock-cli codes --exchange bj
```

### 查询K线数据

```bash
# 日K
./tongstock-cli kline --code 000001 --type day

# 周K
./tongstock-cli kline --code 000001 --type week

# 月K
./tongstock-cli kline --code 000001 --type month

# 1分钟K
./tongstock-cli kline --code 000001 --type 1m

# 5分钟K
./tongstock-cli kline --code 000001 --type 5m

# 季K
./tongstock-cli kline --code 000001 --type quarter

# 年K
./tongstock-cli kline --code 000001 --type year

# 获取全部历史K线
./tongstock-cli kline --code 000001 --type day --all
```

### 查询分时数据

```bash
# 查询当日分时数据
./tongstock-cli minute 000001

# 查询历史分时数据 (需要指定日期)
./tongstock-cli minute 000001 --history --date 20250314
```

### 查询证券数量

```bash
# 深圳市场 (默认)
./tongstock-cli count

# 上海市场
./tongstock-cli count --exchange sh

# 北京市场
./tongstock-cli count --exchange bj
```

### 查询集合竞价

```bash
# 查询集合竞价数据
./tongstock-cli auction 000001
```

### 查询分笔成交

```bash
# 查询当日分笔成交
./tongstock-cli trade 000001

# 查询历史分笔成交 (需要指定日期)
./tongstock-cli trade 000001 --history --date 20240315
```

### 查询除权除息

```bash
./tongstock-cli xdxr 000001
```

### 查询财务数据

```bash
./tongstock-cli finance 000001
```

### 查询指数K线

```bash
# 上证指数日K
./tongstock-cli index --code 999999 --type day

# 沪深300 5分钟K
./tongstock-cli index --code 399300 --type 5m
```

### 查询公司信息(F10)

```bash
# 查询公司信息目录
./tongstock-cli company 000001

# 查询公司信息具体内容
./tongstock-cli company-content 000001

# 通过块名称查询特定内容
./tongstock-cli company-content 000001 --block "公司概况"

# 指定起始位置和长度
./tongstock-cli company-content 000001 --start 30744 --length 9560
```

### 查询板块分类

```bash
# 指数板块
./tongstock-cli block --file block_zs.dat

# 行业板块
./tongstock-cli block --file block_fg.dat

# 概念板块
./tongstock-cli block --file block_gn.dat
```

### 技术指标分析

```bash
# 单股指标分析（默认参数）
./tongstock-cli indicator --code 000001 --type day

# 获取全部历史K线计算指标
./tongstock-cli indicator --code 000001 --type day --all

# 指定K线数量
./tongstock-cli indicator --code 000001 --type day --count 500

# 使用自定义参数配置文件
./tongstock-cli indicator --code 000001 --type day --config configs/params.yaml
```

输出包含：
- 最近 20 天 K 线 + MA(5/10/20) + MACD(DIF/DEA/HIST) + KDJ(K/D/J) + BOLL(UPPER/MID/LOWER)
- 最新信号（金叉/死叉/超买/超卖/多头排列/空头排列等）

### 批量信号筛选

```bash
# 指定股票列表筛选
./tongstock-cli screen --codes "000001,600519,000858" --type day --signal golden_cross

# 从文件读取股票代码（每行一个）
./tongstock-cli screen --file codes.txt --type day --signal oversold

# 设置并发池大小（默认10）
./tongstock-cli screen --codes "000001,600519" --pool 5

# 可用信号类型: golden_cross, death_cross, overbought, oversold
```

## HTTP API 使用方法

### 启动服务

```bash
./tongstock-server
```

服务默认监听 `http://localhost:8080`

### API 接口

| 接口 | 方法 | 参数 | 说明 |
|------|------|------|------|
| `/health` | GET | - | 健康检查 |
| `/api/quote` | GET | `code` | 实时行情 |
| `/api/kline` | GET | `code`, `type`, `start`, `count` | K线数据 |
| `/api/codes` | GET | `exchange` | 股票代码 |
| `/api/minute` | GET | `code`, `date`, `history` | 分时数据（当日/历史） |
| `/api/count` | GET | `exchange` | 证券数量 |
| `/api/auction` | GET | `code` | 集合竞价数据 |
| `/api/trade` | GET | `code`, `start`, `count`, `date`, `history` | 分笔成交数据 |
| `/api/xdxr` | GET | `code` | 除权除息信息 |
| `/api/finance` | GET | `code` | 财务数据 |
| `/api/index` | GET | `code`, `type` | 指数K线 |
| `/api/company` | GET | `code` | 公司信息目录(F10) |
| `/api/company/content` | GET | `code`, `filename` | 公司信息内容 |
| `/api/block` | GET | `file` | 板块分类数据 |
| `/api/indicator` | GET | `code`, `type` | 技术指标（MACD/KDJ/MA/BOLL/RSI + 信号） |
| `/api/screen` | GET | `codes`, `type`, `signal` | 批量信号筛选 |

### 示例

```bash
# 查询行情
curl "http://localhost:8080/api/quote?code=000001"

# 查询K线
curl "http://localhost:8080/api/kline?code=000001&type=day"

# 获取股票列表
curl "http://localhost:8080/api/codes?exchange=sz"

# 查询当日分时数据
curl "http://localhost:8080/api/minute?code=000001"

# 查询历史分时数据
curl "http://localhost:8080/api/minute?code=000001&history=true&date=20250314"

# 查询证券数量
curl "http://localhost:8080/api/count?exchange=sh"

# 查询集合竞价
curl "http://localhost:8080/api/auction?code=000001"

# 查询分笔成交
curl "http://localhost:8080/api/trade?code=000001"

# 查询历史分笔成交
curl "http://localhost:8080/api/trade?code=000001&history=true&date=20240315"

# 查询除权除息
curl "http://localhost:8080/api/xdxr?code=000001"

# 查询财务数据
curl "http://localhost:8080/api/finance?code=000001"

# 查询指数K线
curl "http://localhost:8080/api/index?code=999999&type=day"

# 查询公司信息目录
curl "http://localhost:8080/api/company?code=000001"

# 查询公司信息内容
curl "http://localhost:8080/api/company/content?code=000001&filename=000001.txt"

# 查询板块分类
curl "http://localhost:8080/api/block?file=block_zs.dat"
```

## 配置

### 应用主配置

`~/.tongstock/config.yaml` — 首次运行自动生成，可自行编辑：

```yaml
server:
  port: 8080

tdx:
  # hosts:
  #   - "124.71.187.122:7709"

cache:
  backend: sqlite
  dir: ~/.tongstock/cache

database:
  driver: sqlite3
  dsn: ~/.tongstock/cache/tongstock.db
```

### 指标参数配置

`~/.tongstock/indicator.yaml` — 首次运行 indicator/screen 命令时自动生成，可自行编辑：

```yaml
defaults:
  ma: [5, 10, 20, 60]
  macd:
    fast: 12
    slow: 26
    signal: 9
  kdj:
    n: 9
    m1: 3
    m2: 3
  boll:
    n: 20
    k: 2.0
  rsi: [6, 14]

categories:
  large_cap:
    ma: [5, 10, 20, 60, 120]
  small_cap:
    ma: [5, 10, 20]
    macd:
      fast: 8
      slow: 17

overrides:
  "000001":
    kdj:
      n: 5
```

**参数覆盖优先级**：per-stock override > category override > defaults

### 用户目录结构

```
~/.tongstock/
├── config.yaml          # 应用主配置
├── indicator.yaml       # 指标参数配置
├── cache/
│   └── tongstock.db     # SQLite 缓存数据库
```

如需自定义服务器地址，可在 `config.yaml` 中设置 `tdx.hosts`。如需自定义指标参数，编辑 `indicator.yaml`。如需临时指定配置文件，可使用 `--config` 参数覆盖。

## K线类型参数说明

| type 参数 | 说明 |
|-----------|------|
| `1m`, `minute` | 1分钟K |
| `5m` | 5分钟K |
| `15m` | 15分钟K |
| `30m` | 30分钟K |
| `60m` | 60分钟K |
| `day` | 日K |
| `week` | 周K |
| `month` | 月K |
| `quarter` | 季K |
| `year` | 年K |

## 项目结构

```
tongstock/
├── cmd/
│   ├── cli/              # CLI 工具
│   │   └── main.go       # 命令行入口
│   └── server/           # HTTP API 服务
│       └── main.go       # 服务入口（嵌入 Web UI）
├── web/                  # React + TypeScript Web UI
│   ├── src/
│   │   ├── api/          # API 客户端
│   │   ├── components/   # 组件（图表等）
│   │   ├── pages/        # 页面（Dashboard/Stock/Screen）
│   │   └── types/        # TypeScript 类型
│   ├── package.json
│   └── vite.config.ts
├── pkg/
│   ├── tdx/              # TDX 协议实现
│   │   ├── client.go     # 客户端
│   │   ├── hosts.go      # 服务器地址
│   │   ├── codes.go      # 股票代码
│   │   ├── pull.go       # 行情拉取 + KlineStore
│   │   ├── service.go    # 业务逻辑层
│   │   ├── workday.go    # 交易日判断
│   │   ├── bj_codes.go   # 北京交易所代码
│   │   └── protocol/     # 协议解析
│   │       ├── quote.go   # 行情解析(含五档盘口)
│   │       ├── kline.go   # K线解析
│   │       ├── index.go   # 指数K线解析
│   │       ├── minute.go  # 分时解析
│   │       ├── trade.go   # 分笔解析
│   │       ├── xdxr.go    # 除权除息解析
│   │       ├── finance.go # 财务数据解析
│   │       ├── company.go # 公司信息解析
│   │       ├── block.go   # 板块信息解析
│   │       ├── code.go    # 代码解析
│   │       └── ...
│   ├── ta/               # 技术指标计算（无状态）
│   │   ├── types.go      # 核心类型（KlineInput, IndicatorResult）
│   │   ├── ma.go         # SMA, EMA
│   │   ├── macd.go       # MACD
│   │   ├── kdj.go        # KDJ
│   │   ├── boll.go       # BOLL
│   │   ├── rsi.go        # RSI
│   │   └── indicator.go  # 统一计算入口（并发）
│   ├── signal/           # 信号检测
│   │   ├── signal.go     # Signal 类型定义
│   │   ├── detector.go   # 统一检测入口（并发）
│   │   ├── cross.go      # 金叉/死叉检测
│   │   ├── macd.go       # MACD 信号
│   │   ├── kdj.go        # KDJ 信号
│   │   ├── boll.go       # BOLL 信号
│   │   ├── ma.go         # MA 信号
│   │   └── rsi.go        # RSI 信号
│   ├── param/            # 参数管理
│   │   ├── types.go      # CategoryParams, ParamConfig
│   │   ├── params.go     # Init, Resolve（三层参数覆盖）
│   │   └── category.go   # 按代码判断市值分类
│   └── utils/            # 工具函数
├── configs/
│   └── params.yaml       # 指标参数配置（含大盘/小盘分类）
├── Makefile              # 构建脚本
└── README.md
```

## 技术栈

- **Go 1.24+** - 后端开发语言
- **spf13/cobra** - CLI 框架
- **Gin** - HTTP 框架
- **TDX 协议** - 通达信私有二进制协议
- **gopkg.in/yaml.v3** - 参数配置解析
- **React 19** - Web UI 前端框架
- **TypeScript** - 前端类型安全
- **Vite** - 前端构建工具
- **Tailwind CSS** - 样式框架
- **Recharts** - 图表组件库

## 数据来源

数据来源于通达信官方行情服务器（端口 7709），仅供学习交流使用，请勿用于商业用途。

## 许可证

MIT License

## 注意事项

1. 本项目仅供学习研究使用
2. 请遵守通达信的服务条款
3. 行情数据可能有延迟，不建议用于实盘交易
