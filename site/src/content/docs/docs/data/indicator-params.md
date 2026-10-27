---
title: 指标口径
description: 默认参数、分组与单股覆盖的优先级，以及每家指标的字段含义。
---

指标参数放在 `~/.tongstock/indicator.yaml`，首次执行 `indicator` / `screen` 时自动生成。

## 默认结构

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

## 覆盖优先级

```
per-stock override  >  category override  >  defaults
```

`overrides` 按股票代码匹配，`categories` 是自定义分组（分组归属由你在配置里维护，不是系统推断的市值分类）。

## 字段含义

| 指标 | 输出字段 | 说明 |
| --- | --- | --- |
| MA | `ma5` / `ma10` / `ma20` / `ma60` / `ma120` | 周期由配置决定，默认给 5/10/20/60/120 |
| MACD | `dif` / `dea` / `hist` | 快慢线与柱值，参数默认 12/26/9 |
| KDJ | `k` / `d` / `j` | 默认 n=9, m1=3, m2=3 |
| BOLL | `upper` / `middle` / `lower` | 默认 n=20, k=2.0 |
| RSI | `rsi6` / `rsi12` / `rsi24` | 多周期并出（README 的示例里给 6/12/24；配置文件默认是 `[6, 14]`，以你本地配置为准） |
| 量比 | `volume.ratio` | 当前量 / 近 5 日均量 |

除数值外，还会给出 `trend`（如 `bullish`）、各指标自己的 `signal`（如 `golden_cross`、`overbought`）、汇总的 `signals` 列表与 `summary`（趋势、建议动作、强度）。

## 信号定义

| 信号 | 判定 |
| --- | --- |
| `golden_cross` | MACD 的 DIF 上穿 DEA |
| `death_cross` | MACD 的 DIF 下穿 DEA |
| `overbought` | KDJ 等超买区间 |
| `oversold` | KDJ 等超卖区间 |
| 多头/空头排列 | 均线按周期排序关系 |

具体阈值在 `pkg/ta` 中实现；如果你的策略依赖精确阈值，**直接读代码而不是依赖本文档的文字描述**。

## 口径问题：这些参数是你的选择，不是市场事实

同一只股票用 8/17/9 的 MACD 和 12/26/9 的 MACD，结论可能相反。所以：

- 方法（method）里保存的验证口径包含它自己的参数，不共享全局默认；
- 改 `indicator.yaml` 会改变**新跑出来**的指标，但不会改写已冻结的历史快照与已完成的方法验证；
- 汇报结论时把参数一起写出来，否则无法复核。

## 通过 API 修改

```bash
curl "http://localhost:8106/api/settings/indicator"
```

对应 Web UI 的 `/settings` 页面。
