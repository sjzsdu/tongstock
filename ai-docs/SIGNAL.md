# 信号检测模块设计与实现

本文档详细说明 TongStock 项目中技术指标信号检测的算法设计和实现逻辑。

## 1. 模块架构

```
pkg/signal/
├── signal.go      # 核心类型定义、趋势判断
├── detector.go    # 信号检测入口
├── cross.go      # 交叉检测算法
├── macd.go      # MACD 信号
├── kdj.go       # KDJ 信号
├── ma.go        # 均线信号
├── boll.go      # 布林带信号
├── rsi.go       # RSI 信号
└── cycle.go    # 交易周期分析
```

## 2. 核心类型

### 2.1 Signal 定义

```go
type Signal struct {
    Code      string      // 股票代码
    Date      time.Time  // 信号日期
    Type      SignalType // 信号类型
    Indicator string    // 指标来源
    Details   string    // 详细参数
    Strength  float64   // 信号强度 [0, 1]
}
```

### 2.2 SignalType 类型

| 类型 | 含义 | 说明 |
|-----|------|------|
| `金叉` | Golden Cross | 短期线上穿长期线 |
| `死叉` | Death Cross | 短期线下穿长期线 |
| `超买` | Overbought | 指标处于高位 |
| `超卖` | Oversold | 指标处于低位 |
| `突破上轨` | Break Upper | 价格突破布林上轨 |
| `跌破下轨` | Break Lower | 价格跌破布林下轨 |
| `多头排列` | Bull Align | 均线呈多头排列 |
| `空头排列` | Bear Align | 均线呈空头排列 |

## 3. 指标参数配置

默认参数定义于 `pkg/ta/indicator.go`:

```go
func DefaultConfig() *IndicatorConfig {
    return &IndicatorConfig{
        MA:   []int{5, 10, 20, 60, 120},  // 均线周期
        MACD: &MACDConfig{Fast: 12, Slow: 26, Signal: 9},  // MACD 参数
        KDJ:  &KDJConfig{N: 9, M1: 3, M2: 3},  // KDJ 参数
        BOLL: &BOLLConfig{N: 20, K: 2.0},    // 布林带参数
        RSI:  []int{6, 12, 24},              // RSI 周期
    }
}
```

## 4. 信号检测算法

### 4.1 交叉检测算法

所有金叉死叉检测基于统一的 `detectLineCross` 函数:

```go
// cross.go
func detectLineCross(line1, line2 []float64) []int {
    n := min(len(line1), len(line2))
    result := make([]int, n)
    for i := 1; i < n; i++ {
        prev := line1[i-1] - line2[i-1]
        curr := line1[i] - line2[i]
        result[i] = detectCross(curr, prev)
    }
    return result
}

func detectCross(current, prev float64) int {
    if prev <= 0 && current > 0 { return 1 }   // 金叉
    if prev >= 0 && current < 0 { return -1 }  // 死叉
    return 0
}
```

逻辑: 比较相邻两天两条线的差值符号变化。

### 4.2 MACD 信号

基于 DIF 和 DEA 的交叉:

```go
// macd.go
func detectMACDSignals(code string, klines []ta.KlineInput, macd *ta.MACDResult) []Signal {
    crosses := detectLineCross(macd.DIF, macd.DEA)
    for i, c := range crosses {
        if c == 0 { continue }
        signals = append(signals, Signal{
            Type:     SignalGoldenCross,  // c == 1
            Strength: math.Abs(macd.Hist[i]),
        })
    }
}
```

触发条件:
- **金叉**: DIF 从下方穿越 DEA (DIF - DEA 由负转正)
- **死叉**: DIF 从上方穿越 DEA (DIF - DEA 由正转负)

### 4.3 KDJ 信号

基于 K线和 D线的交叉 + J线的超买超卖:

```go
// kdj.go
func detectKDJSignals(...) []Signal {
    // 1. K线与D线交叉
    crosses := detectLineCross(kdj.K, kdj.D)

    // 2. J线超买 (J > 100)
    for i, jVal := range kdj.J {
        if jVal > 100 {
            signals = append(signals, Signal{
                Type:     SignalOverbought,
                Strength: (jVal - 100) / 100,
            })
        }
        // 3. J线超卖 (J < 0)
        if jVal < 0 {
            signals = append(signals, Signal{
                Type:     SignalOversold,
                Strength: (-jVal) / 100,
            })
        }
    }
}
```

触发条件:
- **金叉/死叉**: K线与D线交叉
- **超买**: J > 100
- **超卖**: J < 0

### 4.4 均线信号

检测 MA5/MA10/MA20/MA60 的交叉和排列:

```go
// ma.go
// 1. 相邻均线交叉
crosses := detectLineCross(ma["5"], ma["10"])
crosses := detectLineCross(ma["10"], ma["20"])

// 2. 多头排列: MA5 > MA10 > MA20 > 0
if ma5[i] > ma10[i] && ma10[i] > ma20[i] {
    Type: SignalBullAlign
}

// 3. 空头排列: MA5 < MA10 < MA20
if ma5[i] < ma10[i] && ma10[i] < ma20[i] {
    Type: SignalBearAlign
}
```

### 4.5 布林带信号

检测价格突破上下轨:

```go
// boll.go
for i := range klines {
    close := klines[i].Close
    // 突破上轨
    if close > boll.Upper[i] && boll.Upper[i] > 0 {
        signals = append(signals, Signal{
            Type:     SignalBreakUpper,
            Strength: (close - boll.Upper[i]) / boll.Upper[i],
        })
    }
    // 跌破下轨
    if close < boll.Lower[i] && boll.Lower[i] > 0 {
        signals = append(signals, Signal{
            Type:     SignalBreakLower,
            Strength: (boll.Lower[i] - close) / boll.Lower[i],
        })
    }
}
```

### 4.6 RSI 信号

检测 RSI 超过阈值:

```go
// rsi.go
for period, values := range rsi {
    for i, val := range values {
        // 超买区
        if val > 80 {
            signals = append(signals, Signal{
                Type:     SignalOverbought,
                Strength: (val - 80) / 20,
            })
        }
        // 超卖区
        if val < 20 {
            signals = append(signals, Signal{
                Type:     SignalOversold,
                Strength: (20 - val) / 20,
            })
        }
    }
}
```

## 5. 趋势过滤

信号检测时会先判断当前趋势，根据趋势过滤部分信号:

```go
// signal.go
func detectTrend(klines []ta.KlineInput, ma map[string][]float64) TrendDirection {
    // 多头排列天数统计
    bullDays := 0
    bearDays := 0
    for i := range last5Days {
        if ma5[i] > ma10[i] && ma10[i] > ma20[i] { bullDays++ }
        if ma5[i] < ma10[i] && ma10[i] < ma20[i] { bearDays++ }
    }
    // 趋势判断
}
```

趋势过滤规则:

| 趋势 | 保留信号 | 过滤信号 |
|-----|---------|---------|
| 上涨 | 金叉、死叉、超买、超卖 | - |
| 下跌 | 死叉、金叉、超买、超卖 | - |
| 横盘 | 超买、超卖、突破上/下轨 | 金叉、死叉 |

```go
func shouldGenerateSignal(signalType SignalType, trend TrendDirection) bool {
    // 超买超卖和突破信号不受趋势限制
    case SignalOverbought, SignalOversold, SignalBreakUpper, SignalBreakLower:
        return true
    // 金叉只在上涨趋势有效
    case SignalGoldenCross:
        return trend == TrendUptrend
    // 死叉只在下跌趋势有效
    case SignalDeathCross:
        return trend == TrendDowntrend
}
```

## 6. 交易周期分析

基于全量历史数据，检测完整的买卖周期:

```go
// cycle.go
func DetectAllCycles(code string, klines []ta.KlineInput, result *ta.IndicatorResult) []TradeCycle {
    // 1. MACD 金叉死叉周期
    crosses := detectLineCross(result.MACD.DIF, result.MACD.DEA)
    cycles := detectCyclesFromCrosses(code, klines, crosses, "MACD")

    // 2. KDJ 周期
    cycles := detectCyclesFromCrosses(code, klines, crosses, "KDJ")

    // 3. MA 周期
    cycles := detectCyclesFromCrosses(code, klines, crosses, "MA(5,10)")
}
```

周期检测逻辑:

1. 遇到**金叉** → 记录买入点 (pendingBuy)
2. 遇到**死叉** → 完成周期，计算收益率和最��涨跌幅
3. 周期属性:
   - `BuyDate` / `SellDate`: 买卖日期
   - `BuyPrice` / `SellPrice`: 买卖价格
   - `HoldDays`: 持有天数
   - `ReturnPct`: 收益率 (%)
   - `MaxProfit` / `MaxLoss`: 期间最大涨跌幅

## 7. 信号强度计算

信号强度 `Strength` 为 [0, 1] 的浮点数:

| 指标 | 计算公式 |
|-----|--------|
| MACD 金叉/死叉 | `|Hist| / 某基准` |
| KDJ 超买/超卖 | `(J - 100) / 100` 或 `-J / 100` |
| BOLL 突破 | `|Close - Upper| / Upper` |
| RSI 超买/超卖 | `(RSI - 80) / 20` 或 `(20 - RSI) / 20` |
| 多头排列 | `(MA5 - MA20) / MA20` |

## 8. 数据流程图

```
TDX Server
    │
    ▼
FetchKlineAll() ── 全量历史K线
    │
    ▼
ta.Calculate() ── 计算技术指标
    │
    ├─ MA (5,10,20,60,120)
    ├─ MACD (DIF,DEA,Hist)
    ├─ KDJ (K,D,J)
    ├─ BOLL (Upper,Middle,Lower)
    └─ RSI (6,12,24)
    │
    ▼
signal.Detect() ── 检测信号
    │
    ├─ DetectMACDSignals()
    ├─ DetectKDJSignals()
    ├─ DetectMASignals()
    ├─ DetectBOLLSignals()
    └─ DetectRSISignals()
    │
    ▼
[Signal] ── 返回信号列表
    │
    ▼
signal.DetectAllCycles() ── 检测交易周期
    │
    ▼
[TradeCycle] ── 返回周期列表
```

## 9. API 接口

| 接口 | 说明 |
|-----|------|
| `/api/screen` | 批量筛选信号 |
| `/api/indicator` | 单股技术指标 + 信号 |
| `/api/signal-analysis` | 信号历史表现分析 |

## 10. 参考资料

- 指标计算: `pkg/ta/`
- 信号检测: `pkg/signal/`
- 服务器处理: `internal/serverapp/app.go`（入口：`tongstock server`）
