# 现状差距清单：审计现有数据、信号、范式与回测实现

本文档记录对 TongStock 现有模块的审计结果，识别时序泄漏、样本偏差、成交假设、不可复现和指标口径问题。

## 审计范围

- TDX 数据链路 (`pkg/tdx/service.go`)
- 指标计算 (`pkg/ta/`)
- 信号检测 (`pkg/signal/`)
- 范式评估 (`pkg/server/paradigm_evaluate.go`, `internal/paradigms/`)
- 新闻与财务数据 (`pkg/newsfeed/`)

---

## 问题清单（按严重级排序）

### 🔴 P0 - 时序泄漏（未来数据）

| # | 问题 | 代码位置 | 影响 | 后续任务 |
|---|------|----------|------|----------|
| P0-1 | `fetchCurrentIndicator` 使用最新 K 线计算指标时未排除当日数据（盘中时使用未来数据） | `pkg/server/paradigm_evaluate.go:25` | 评估使用当日未结算数据，信号失真 | 修复为使用 T-1 日数据 |
| P0-2 | 趋势检测使用最近 N 日 MA 判断，未明确排除当日 | `pkg/signal/signal.go:226` | 趋势判断可能包含当日数据 | 确认是否排除当日 |
| P0-3 | 回测周期检测使用全量历史数据包含当前日期 | `pkg/signal/cycle.go:127` | 周期统计可能包含未完成的当前周期 | 排除当前未完成周期 |

### 🟠 P1 - 样本偏差

| # | 问题 | 代码位置 | 影响 | 后续任务 |
|---|------|----------|------|----------|
| P1-1 | K线获取仅保留 `FilterValidKlines` 过滤后的数据，未包含退市股 | `pkg/tdx/service.go:474` | 存在幸存者偏差 | 审计 TDX 数据源是否包含退市股 |
| P1-2 | `GetKlineAll` 默认拉取全量历史，未区分 training/testing 窗口 | `pkg/tdx/service.go:361` | 样本外验证不干净 | 需支持滚动窗口数据分割 |
| P1-3 | 信号检测使用 `EnableMA/KDJ/MACD/RSI/BOLL` 全量开启，无分层验证 | `pkg/signal/detector.go:24-83` | 多重检验问题，信号过密 | 添加信号组合的统计显著性检验 |

### 🟡 P2 - 成交假设

| # | 问题 | 代码位置 | 影响 | 后续任务 |
|---|------|----------|------|----------|
| P2-1 | 周期收益计算假设以收盘价成交 | `pkg/signal/cycle.go:78-80` | 未考虑滑点，实际收益更低 | 添加滑点成本参数 |
| P2-2 | 未处理涨跌停无法成交情况 | `pkg/signal/cycle.go` | 回测假设 100% 成交 | 添加涨跌停过滤或成交率估计 |
| P2-3 | 卖出条件检测使用最新价格，未模拟实际信号触发后的价格变化 | `pkg/server/paradigm_evaluate.go:63-192` | 信号触发后价格可能已变 | 需使用信号触发时刻的价格 |

### 🔵 P3 - 不可复现

| # | 问题 | 代码位置 | 影响 | 后续任务 |
|---|------|----------|------|----------|
| P3-1 | 指标计算参数硬编码，未版本化 | `pkg/ta/indicator.go` | 不同运行结果可能不同 | 迁移到 `configs/params.yaml` |
| P3-2 | 缓存 TTL 分散在代码中（`financeTTL`, `xdxrTTL`, `companyTTL`, `blockTTL`） | `pkg/tdx/service.go:136-139` | 缓存策略不可追溯 | 统一配置管理 |
| P3-3 | `marketNow()` 使用系统时间，未模拟时间 | `pkg/tdx/service.go:382` | 回测无法精确重现 | 需支持注入时间 |

### 🟢 P4 - 指标口径

| # | 问题 | 代码位置 | 影响 | 后续任务 |
|---|------|----------|------|----------|
| P4-1 | RSI 指标使用 `rsi14` 硬编码，未支持多周期 | `pkg/server/paradigm_evaluate.go:108` | 与 `DefaultConfig` 中的 RSI6/12/24 不一致 | 统一指标配置 |
| P4-2 | MA 指标命名不一致：代码用 `ma20`，文档用 `MA20` | `pkg/signal/` vs `ai-docs/SIGNAL.md` | 指标口径不统一 | 标准化命名 |
| P4-3 | 置信区间计算使用 Normal 近似，未使用 Bootstrap | `internal/evidence/types.go` | 小样本时 CI 不准确 | 考虑添加 Bootstrap 方法 |

---

## 基线结果

当前版本（commit `b351f57`）测试状态：

```bash
$ go test ./... -count=1
ok  	github.com/sjzsdu/tongstock/pkg/signal	0.011s
ok  	github.com/sjzsdu/tongstock/pkg/ta	0.014s
ok  	github.com/sjzsdu/tongstock/pkg/tdx	0.092s
ok  	github.com/sjzsdu/tongstock/internal/evidence	0.008s
ok  	github.com/sjzsdu/tongstock/internal/paradigms	0.015s
```

所有现有测试通过，但审计发现的问题尚未修复。

---

## 后续任务

| 任务 | 依赖 | 优先级 |
|------|------|--------|
| 修复 P0-1: 评估使用 T-1 日数据 | P0 | 必须 |
| 修复 P1-3: 添加信号统计显著性检验 | P1 | 高 |
| 添加 P2: 滑点成本和涨跌停处理 | P1 | 高 |
| 修复 P3: 配置外部化与时间注入 | P2 | 中 |
| 统一 P4: 指标口径标准化 | P3 | 低 |

---

*审计时间: 2026-07-29*
*审计人: tongstock-qhe.1.2*
