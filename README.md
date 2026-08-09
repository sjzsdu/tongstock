# TongStock AI 投资研究与决策工具

TongStock 围绕两个问题组织产品：**今天买什么，以及持仓什么时候卖**。AI 负责提出和解释投资规律；确定性引擎使用真实 A 股数据完成编译、冻结、回测、证据审查、每日选股和卖出判断。没有足够数据或证据时，系统会拒绝推荐。

## 三步使用

1. 在首页输入股票代码、已知方法名称或自然语言研究问题，例如“研究 000063 最近上涨前的共同特征”。
2. 查看“今日决策”。候选会同时展示真实数据日期、方法证据、买入窗口、仓位上限和退出计划；空榜单表示当前没有方法通过门槛。
3. 在“持仓卖出”查看 `hold/watch/reduce/exit`。旧持仓没有原始方法血缘时会明确标为 `inferred`，停牌或 T+1 时不会假装已经卖出。

可信方法库位于 `/methods`。旧信号筛选、范式和实验工具仍保留在“高级工具”，用于审计而不是普通用户主流程。完整说明见 [用户指南](docs/AI_USER_GUIDE.md)，真实数据验收见 [验收报告](docs/REAL_DATA_ACCEPTANCE.md)。

CLI 和 HTTP API 共享同一个 DB-first 股票数据服务：先从 SQLite 检查数据与同步水位，再结合当前日期、交易时段和交易日历判断新鲜度；数据缺失或过期时只从 TDX 同步缺失范围，在事务中写入业务数据和水位，最后重新读取数据库返回。详细设计见 [架构说明](ai-docs/ARCHITECTURE.md)。

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
- **股票代码** - 获取沪深北交易所所有股票代码，支持分类过滤
- **技术指标** - MACD/KDJ/MA(5/10/20/60/120)/BOLL/RSI(6/12/24)/量比，支持参数化计算
- **信号检测** - 金叉/死叉/超买/超卖/突破，自动检测并标记
- **批量筛选** - 按板块或代码列表批量筛选信号，支持并发
- **双模式** - CLI 命令行工具 + HTTP REST API
- **数据缓存** - 股票代码和板块数据 24 小时缓存

## 安装

```bash
# 克隆项目
git clone https://github.com/sjzsdu/tongstock.git
cd tongstock

# 安装依赖并构建（需要 Go 1.25+、Node.js 22+ 和 pnpm 10）
cd web && pnpm install --frozen-lockfile && cd ..
make cli

# 启动 HTTP 服务
./tongstock server

# macOS 启动菜单栏
./tongstock menubar
```

## Skill 使用（推荐）

本项目已发布为 Skills，可通过以下命令直接安装使用：

```bash
npx skills add sjzsdu/tongstock
```

安装后即可通过 Codebuff 与 AI 对话的方式使用 TongStock 的所有功能：
- 查询股票行情、K线、分时、财务等数据
- 技术指标分析与信号检测
- 批量筛选股票信号
- 板块分类与成分股查询
- 股票代码批量操作

**提示**：首次使用需确保 TongStock 服务已启动（`./tongstock server`），默认服务地址 `http://localhost:8080`

## Web UI

启动 server 后访问 `http://localhost:8080` 即可使用 Web 界面。

### 功能页面

| 页面 | 路径 | 功能 |
|------|------|------|
| 今日决策 | `/` | AI 研究输入、真实选股和持仓风险摘要 |
| 可信方法 | `/methods` | 方法生命周期、适用范围与样本外证据 |
| 自选股 | `/watchlist` | 分组自选股、备注和快速行情入口 |
| 股票选择 | `/stock/choose` | 搜索并进入个股详情 |
| 个股详情 | `/stock/:code` | 行情、K 线、指标、财务和资讯 |
| 信号筛选 | `/screen` | 批量筛选金叉/死叉/超买/超卖 |
| 持仓卖出 | `/portfolio` | hold/watch/reduce/exit 队列与执行约束 |
| 板块 | `/blocks` | 板块列表和成分股 |
| 指数详情 | `/index/:code` | 指数行情与 K 线 |
| Agent | `/agent` | 股票 Agent 对话与诊断 |
| 投资范式 | `/paradigms` | 范式分析、复盘与告警 |
| 隔夜策略 | `/strategy/overnight` | 隔夜套利策略分析 |
| 新闻 | `/news` | 新闻流、热点事件和情绪 |
| 设置 | `/settings` | 指标参数等本地设置 |

### 开发模式

```bash
cd web
pnpm install --frozen-lockfile
pnpm dev           # 启动开发服务器，默认代理到 localhost:8080
```

## CLI 使用方法

### 查询行情

```bash
./tongstock quote 000001
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
# 默认列出深圳市场所有证券
./tongstock codes list

# 上海市场
./tongstock codes list -e sh

# 北京市场
./tongstock codes list -e bj

# 按分类过滤 - 只显示股票
./tongstock codes list -e sz -c stock

# 按分类过滤 - 只显示ETF
./tongstock codes list -e sz -c etf

# 查看各分类统计信息
./tongstock codes stats

# 查看所有交易所统计
./tongstock codes stats --all
```

**支持的分类：**
- `all` - 全部
- `stock` - 股票
- `gem` - 创业板
- `fund` - 基金
- `etf` - ETF
- `bond` - 债券
- `index` - 指数

### 查询证券数量

```bash
# 深圳市场 (默认)
./tongstock count

# 上海市场
./tongstock count --exchange sh

# 北京市场
./tongstock count --exchange bj
```

### 查询集合竞价

```bash
# 查询集合竞价数据
./tongstock auction 000001
```

### 行情数据查询入口

K线、分时、分笔、除权除息、财务、指数 K 线、F10 公司信息等数据已统一走 HTTP API，
请通过 Web 界面（`/stock/:code` 个股详情、`/index/:code` 指数详情）或 `./tongstock server`
提供的 REST 接口查询；CLI 保留 `quote/codes/indicator/screen/block/count/auction` 等
轻量查询命令。

### 查询板块分类

```bash
# 列出所有板块文件
./tongstock block files

# 列出指数板块
./tongstock block list -f block_zs.dat

# 按Type过滤（2=标准板块）
./tongstock block list -f block.dat -t 2

# 按成分股数量排序
./tongstock block list -f block_fg.dat -s

# 显示板块成分股
./tongstock block show "沪深300" -f block_zs.dat

# 模糊搜索板块
./tongstock block show "银行" -f block_fg.dat

# 按股票代码查询所属板块
./tongstock block show --code 600519
```

**板块文件：**
| 文件 | 名称 | 说明 |
|------|------|------|
| `block.dat` | 综合板块 | 综合分类 |
| `block_zs.dat` | 指数板块 | 主要指数成分股 |
| `block_fg.dat` | 行业板块 | 行业分类 |
| `block_gn.dat` | 概念板块 | 概念主题 |

### 技术指标分析

```bash
# 单股指标分析（默认参数，表格输出）
./tongstock indicator --code 000001 --type day

# JSON格式输出（默认返回最新一天）
./tongstock indicator --code 000001 --type day --json

# JSON格式输出，返回最近5天数据
./tongstock indicator --code 000001 --type day --json --days 5

# 获取全部历史K线计算指标
./tongstock indicator --code 000001 --type day --all

# 指定K线数量
./tongstock indicator --code 000001 --type day --count 500

# 使用自定义参数配置文件
./tongstock indicator --code 000001 --type day --config configs/params.yaml
```

**输出包含：**
- 最近 20 天 K 线 + MA(5/10/20/60/120) + MACD(DIF/DEA/HIST) + KDJ(K/D/J) + BOLL(UPPER/MID/LOWER) + RSI(6/12/24) + 量比
- 最新信号（金叉/死叉/超买/超卖/多头排列/空头排列等）

**JSON 输出格式（单日）：**
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

**JSON 输出格式（多日，--days > 1）：**
```json
{
  "code": "000001",
  "name": "平安银行",
  "days": 5,
  "count": 5,
  "history": [
    { "timestamp": "2026-03-25", "price": {...}, "ma": {...}, ... },
    { "timestamp": "2026-03-26", "price": {...}, "ma": {...}, ... },
    { "timestamp": "2026-03-27", "price": {...}, "ma": {...}, ... },
    { "timestamp": "2026-03-28", "price": {...}, "ma": {...}, ... },
    { "timestamp": "2026-03-29", "price": {...}, "ma": {...}, ... }
  ],
  "summary": { "trend": "上升趋势", "signal": "持有", "strength": 72 }
}
```

### 批量信号筛选

```bash
# 指定股票列表筛选
./tongstock screen --codes "000001,600519,000858" --type day --signal golden_cross

# 从文件读取股票代码（每行一个）
./tongstock screen --file codes.txt --type day --signal oversold

# 设置并发池大小（默认10）
./tongstock screen --codes "000001,600519" --pool 5

# 可用信号类型: golden_cross, death_cross, overbought, oversold
```

## HTTP API 使用方法

### 启动服务

```bash
./tongstock server
```

服务默认只监听 `http://127.0.0.1:8080`。

如需从其他设备访问，必须同时配置非本机监听地址和访问令牌：

```yaml
server:
  port: 8080
  bind_address: 0.0.0.0
  access_token: "替换为足够长的随机令牌"
```

远程 API 客户端通过 Header 传递令牌，不要把令牌放进 URL：

```bash
curl -H "Authorization: Bearer $TONGSTOCK_ACCESS_TOKEN" \
  "http://server-host:8080/api/quote?code=000001"
```

远程打开 Web UI 时，页面第一次收到 401 会提示输入 Access Token；令牌仅保存在浏览器
localStorage 中，后续 API 和 Agent SSE 请求通过 `Authorization` Header 发送。`/health`
和 SPA 静态资源保持公开，所有 `/api` 路由均受令牌保护。非 loopback 地址没有配置
`access_token` 时，服务会拒绝启动。

### API 接口

| 接口 | 方法 | 参数 | 说明 |
|------|------|------|------|
| `/health` | GET | - | 健康检查 |
| `/api/quote` | GET | `code` | 实时行情 |
| `/api/codes` | GET | `exchange` | 股票代码(传统) |
| `/api/codes/list` | GET | `exchange`, `category` | 股票代码列表(支持过滤) |
| `/api/codes/stats` | GET | `exchange`, `all` | 代码统计 |
| `/api/kline` | GET | `code`, `type`, `start`, `count` | K线数据 |
| `/api/count` | GET | `exchange` | 证券数量 |
| `/api/auction` | GET | `code` | 集合竞价数据 |
| `/api/minute` | GET | `code`, `date`, `history` | 分时数据（当日/历史） |
| `/api/trade` | GET | `code`, `start`, `count`, `date`, `history` | 分笔成交数据 |
| `/api/xdxr` | GET | `code` | 除权除息信息 |
| `/api/finance` | GET | `code` | 财务数据 |
| `/api/index` | GET | `code`, `type` | 指数K线 |
| `/api/company` | GET | `code` | 公司信息目录(F10) |
| `/api/company/content` | GET | `code`, `filename` | 公司信息内容 |
| `/api/block` | GET | `file` | 板块分类(传统) |
| `/api/block/files` | GET | - | 板块文件列表 |
| `/api/block/list` | GET | `file`, `type`, `sort` | 结构化板块列表 |
| `/api/block/show` | GET | `name`, `code`, `file` | 板块成分股/按股票查板块 |
| `/api/indicator` | GET | `code`, `type`, `days` | 技术指标（MACD/KDJ/MA/BOLL/RSI/量比 + 信号），days参数可限制返回的K线数量 |
| `/api/screen` | GET | `codes`, `type`, `signal` | 批量信号筛选 |

### 示例

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

# 板块文件列表
curl "http://localhost:8080/api/block/files"

# 板块列表（过滤+排序）
curl "http://localhost:8080/api/block/list?file=block_zs.dat&type=2&sort=true"

# 板块成分股
curl "http://localhost:8080/api/block/show?name=沪深300&file=block_zs.dat"

# 按股票代码查询所属板块
curl "http://localhost:8080/api/block/show?code=600519"
```

### 缓存说明

应用使用 `~/.tongstock/cache/tongstock.db` 这一份 SQLite 数据库统一保存业务数据、缓存、股票 read model 和同步水位。日 K、行情快照和财务快照遵循数据库优先策略；股票代码、板块、F10 等低频数据使用同库中的 TTL cache 表。

核心股票接口支持：

- `consistency=require_fresh`（默认）：必要时同步，失败返回错误；
- `consistency=allow_stale`：同步失败时允许返回数据库旧数据；
- `consistency=cache_only`：只读数据库，不连接 TDX；
- `refresh=true`：强制刷新后重新读库。

CLI 使用同名全局参数，例如：

```bash
./tongstock quote 000001 --consistency=cache_only
./tongstock indicator --code 000001 --type day --refresh
```

## 配置

### 应用主配置

`~/.tongstock/config.yaml` — 首次运行自动生成，可自行编辑：

```yaml
server:
  port: 8080
  bind_address: 127.0.0.1
  # 非本机监听时必填
  # access_token: "替换为足够长的随机令牌"

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
│   ├── cli/              # 按业务域拆分的 CLI 传输适配器
│   └── server/           # HTTP 进程入口
├── internal/
│   ├── app/stockdata/    # DB-first 应用服务、新鲜度、同步与事务
│   └── serverapp/        # App composition root 和生命周期
├── pkg/
│   ├── server/           # HTTP 路由、垂直 handlers、错误契约、可观测性
│   ├── storage/          # SQLite 连接和版本化迁移
│   └── tdx/              # TDX 协议、连接池和上游适配
├── api/openapi.json      # API 契约源
├── web/                  # React + TypeScript Web UI
├── docs/adr/             # 架构决策记录
└── ai-docs/              # 架构、存储、服务和运维文档
```

本地和 CI 使用同一个质量入口：

```bash
make check
```

`api/openapi.json` 覆盖所有公开 HTTP 路由。Go 契约测试双向比对实际 Gin
路由与 OpenAPI，前端 `api:check` 则验证生成的 DTO/操作表没有漂移。

## 技术栈

- **Go 1.25+** - 后端开发语言
- **spf13/cobra** - CLI 框架
- **Gin** - HTTP 框架
- **TDX 协议** - 通达信私有二进制协议
- **gopkg.in/yaml.v3** - 参数配置解析
- **React 19** - Web UI 前端框架
- **TypeScript** - 前端类型安全
- **Vite** - 前端构建工具
- **Tailwind CSS** - 样式框架
- **lightweight-charts** - 图表组件库

## 数据来源

数据来源于通达信官方行情服务器（端口 7709），仅供学习交流使用，请勿用于商业用途。

## 许可证

MIT License

## 注意事项

1. 本项目仅供学习研究使用
2. 请遵守通达信的服务条款
3. 行情数据可能有延迟，不建议用于实盘交易
