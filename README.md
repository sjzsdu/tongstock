# TongStock 通达信股票数据查询工具

基于 Go 语言实现的 TDX (通达信) 行情数据客户端，支持 CLI 和 HTTP API 两种方式获取股票数据。

## 功能特性

- **实时行情** - 五档买卖盘、涨跌幅、成交量等
- **K线数据** - 支持 1分钟/5分钟/15分钟/30分钟/60分钟/日/周/月 K线
- **股票代码** - 获取沪深北交易所所有股票代码
- **双模式** - CLI 命令行工具 + HTTP REST API

## 安装

```bash
# 克隆项目
git clone https://github.com/your-repo/TongStock.git
cd TongStock

# 编译
go build -o tongstock-cli ./cmd/cli
go build -o tongstock-server ./cmd/server
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
# 深圳市场
./tongstock-cli codes

# 上海市场
./tongstock-cli codes --exchange sh
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
| `/api/kline` | GET | `code`, `type` | K线数据 |
| `/api/codes` | GET | `exchange` | 股票代码 |

### 示例

```bash
# 查询行情
curl "http://localhost:8080/api/quote?code=000001"

# 查询K线
curl "http://localhost:8080/api/kline?code=000001&type=day"

# 获取股票列表
curl "http://localhost:8080/api/codes?exchange=sz"
```

## 配置

暂无配置文件，服务器地址使用内置默认值（通达信公网服务器）。

如需自定义服务器，可在代码中修改 `pkg/tdx/client.go` 中的 `DefaultHosts` 变量。

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

## 项目结构

```
TongStock/
├── cmd/
│   ├── cli/           # CLI 工具
│   └── server/        # HTTP API 服务
├── pkg/
│   ├── tdx/           # TDX 协议实现
│   │   ├── client.go  # 客户端
│   │   └── protocol/  # 协议解析
│   └── utils/         # 工具函数
├── configs/           # 配置文件
└── README.md
```

## 技术栈

- **Go 1.25** - 开发语言
- **urfave/cli** - CLI 框架
- **Gin** - HTTP 框架
- **TDX 协议** - 通达信私有二进制协议

## 数据来源

数据来源于通达信官方行情服务器（端口 7709），仅供学习交流使用，请勿用于商业用途。

## 许可证

MIT License

## 注意事项

1. 本项目仅供学习研究使用
2. 请遵守通达信的服务条款
3. 行情数据可能有延迟，不建议用于实盘交易
