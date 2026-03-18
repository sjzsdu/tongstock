# Service 层 API

## 概述

`Service` 是 TongStock 的统一数据访问入口，封装了 TDX Client 和本地存储（Cache/DB），提供智能缓存逻辑。

## 创建与销毁

### NewService

```go
func NewService(client *Client) (*Service, error)
```

创建一个 Service 实例，同时初始化所有存储。

**参数**:
- `client`: 已连接的 TDX Client

**返回值**:
- `*Service`: Service 实例
- `error`: 初始化错误

**示例**:
```go
client, err := tdx.DialHosts(config.Get().TDX.Hosts)
if err != nil {
    return err
}
svc, err := tdx.NewService(client)
if err != nil {
    return err
}
defer svc.Close()
```

### Close

```go
func (s *Service) Close() error
```

关闭 Service 及所有内部资源（Client、Cache、DB）。

---

## 数据访问方法

### FetchCodes

```go
func (s *Service) FetchCodes(exchange protocol.Exchange) ([]*protocol.CodeItem, error)
```

获取股票代码列表。

**缓存策略**: Cache，TTL=24小时

**参数**:
- `exchange`: 交易所 (protocol.ExchangeSZ / ExchangeSH / ExchangeBJ)

**返回值**:
- `[]*protocol.CodeItem`: 股票代码列表
- `error`: 错误

---

### FetchKlineAll

```go
func (s *Service) FetchKlineAll(code string, ktype uint8) ([]*protocol.Kline, error)
```

获取完整 K 线数据（带智能缓存）。

**缓存策略**: 
- 日K (ktype=9): DB 存储，智能增量更新
- 其他类型: 穿透，每次直连 TDX

**参数**:
- `code`: 股票代码 (如 "000001")
- `ktype`: K线类型

| ktype | 类型 |
|-------|------|
| 0 | 5分钟 |
| 1 | 15分钟 |
| 2 | 30分钟 |
| 3 | 60分钟 |
| 5 | 周 |
| 6 | 月 |
| 7 | 1分钟 |
| 9 | 日 ✅ 存库 |
| 10 | 季 |
| 11 | 年 |

**返回值**:
- `[]*protocol.Kline`: K线数据
- `error`: 错误

---

### FetchKline

```go
func (s *Service) FetchKline(code string, ktype uint8, start, count uint16) ([]*protocol.Kline, error)
```

获取指定范围的 K 线数据（非全量）。

**缓存策略**: 穿透，每次直连 TDX（用于获取历史片段）

**参数**:
- `code`: 股票代码
- `ktype`: K线类型
- `start`: 起始位置
- `count`: 数量

---

### FetchXdXr

```go
func (s *Service) FetchXdXr(code string) ([]*protocol.XdXrItem, error)
```

获取除权除息信息。

**缓存策略**: Cache，TTL=7天

---

### FetchFinance

```go
func (s *Service) FetchFinance(code string) (*protocol.FinanceInfo, error)
```

获取财务数据。

**缓存策略**: Cache，TTL=7天

---

### FetchCompanyCategory

```go
func (s *Service) FetchCompanyCategory(code string) ([]*protocol.CompanyCategoryItem, error)
```

获取 F10 公司信息目录。

**缓存策略**: Cache，TTL=30天

---

### FetchCompanyContent

```go
func (s *Service) FetchCompanyContent(code, filename string, start, length uint32) (string, error)
```

获取 F10 公司信息内容。

**缓存策略**: Cache，TTL=30天

---

### FetchBlock

```go
func (s *Service) FetchBlock(blockFile string) ([]*protocol.BlockItem, error)
```

获取板块分类信息。

**缓存策略**: Cache，TTL=1天

**参数**:
- `blockFile`: 板块文件 (如 "block_zs.dat", "block_fg.dat", "block_gn.dat")

---

### EnsureWorkday

```go
func (s *Service) EnsureWorkday() error
```

确保交易日历数据存在。

**缓存策略**: DB，首次调用时自动从 K线数据中提取

---

## 直接访问 Client

对于实时数据（如行情、分时），可绕过缓存直接访问 Client：

```go
// 实时行情
quotes, err := svc.Client.GetQuote("000001")

// 分时数据
minute, err := svc.Client.GetMinute("000001")

// 分笔成交
trade, err := svc.Client.GetMinuteTrade("000001", 0, 100)
```

---

## 辅助函数

### ParseKlineType

```go
func ParseKlineType(s string) uint8
```

将人类友好的 K线类型字符串转换为协议常量。

**参数**:
- `s`: 类型字符串 ("1m", "5m", "15m", "30m", "60m", "day", "week", "month", "quarter", "year")

**返回值**:
- `uint8`: 协议常量

**示例**:
```go
ktype := tdx.ParseKlineType("day")  // 返回 9
ktype := tdx.ParseKlineType("week") // 返回 5
```

---

## 完整使用示例

### CLI 命令

```go
func runKline(cmd *cobra.Command, args []string) error {
    svc, err := dialService()
    if err != nil {
        return err
    }
    defer svc.Close()

    ktype := tdx.ParseKlineType(klineType)
    var klines []*protocol.Kline
    
    if klineAll {
        klines, err = svc.FetchKlineAll(klineCode, ktype)
    } else {
        klines, err = svc.FetchKline(klineCode, ktype, 0, 100)
    }
    
    // 打印结果
    for _, k := range klines {
        fmt.Printf("%s O:%.2f H:%.2f L:%.2f C:%.2f\n",
            k.Time.Format("2006-01-02"), k.Open, k.High, k.Low, k.Close)
    }
    return nil
}
```

### HTTP Handler

```go
func handleKline(c *gin.Context) {
    code := c.Query("code")
    ktype := tdx.ParseKlineType(c.Query("type"))

    svc, err := getService()
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    klines, err := svc.FetchKlineAll(code, ktype)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, klines)
}
```
