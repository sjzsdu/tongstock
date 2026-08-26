# 稳定获利范式证据与准入标准

本文档定义 TongStock 范式研究系统中，范式晋级的证据标准与准入规范。**"稳定获利"是严格的研究目标，不代表承诺或保证未来收益**。

## 1. 核心原则

### 1.1 禁止事项（红区）

| 禁止项 | 说明 | 违反后果 |
|--------|------|----------|
| **未来数据** | 回测、验证和特征计算中禁止使用任何未来时点的数据 | 直接拒绝 |
| **幸存者偏差** | 必须包含已退市/破产股票；禁止仅用当前存活股票回测 | 直接拒绝 |
| **只看胜率** | 胜率不是唯一标准；必须配合收益、风险和样本量综合评估 | 警告+需补充证据 |
| **过拟合美化** | 禁止通过参数调优刻意拟合历史数据 | 直接拒绝 |
| **选择性报告** | 必须报告所有交易，不得只报告盈利交易 | 直接拒绝 |

### 1.2 "稳定"的定义

"稳定"指**在满足以下条件时，历史证据表明该范式具有可重复的性能特征**：

- 样本量充足
- 跨多个独立时间窗口表现一致
- 跨不同市场状态（牛/熊/震荡/高波动）表现一致
- 参数敏感性在可接受范围内
- 经过成本后（手续费、滑点）验证

**稳定 ≠ 保证未来收益。历史表现不代表未来结果。**

---

## 2. 必需指标与口径

### 2.1 成本后期望收益 (Post-Cost Expected Return)

**定义**：扣除交易成本后的期望收益率。

**计算方法**：
```
期望收益 = Σ(单次收益 × 概率) - 总成本
单次收益 = (卖出价 - 买入价) / 买入价 × 100%
总成本 = 买入佣金 + 卖出佣金 + 印花税 + 滑点成本
```

**最低证据要求**：
- 假设：佣金万2.5（双向）、印花税千1（卖出）、滑点 0.1%
- 必须使用实际交易费率计算
- 单边成本低于 0.3% 的声称需额外验证

**拒绝条件**：成本后期望收益 ≤ 0%

---

### 2.2 样本量 (Sample Size)

**定义**：回测期间完整交易周期的总数。

**计算方法**：从买入信号触发到卖出信号触发的完整闭环次数。

**最低证据要求**：

| 持有周期 | 最小样本量 |
|----------|------------|
| 日内 (T+0) | 100 |
| 1-5 天 | 50 |
| 5-20 天 | 30 |
| 20+ 天 | 20 |

**拒绝条件**：样本量低于对应持有周期的最小要求

---

### 2.3 置信区间 (Confidence Interval)

**定义**：使用 Bootstrap 或统计方法计算的收益分布区间。

**计算方法**：
1. 对所有交易收益进行重采样（有放回），重复 N 次（如 10000 次）
2. 每次计算平均收益
3. 取 2.5% 和 97.5% 分位数作为 95% 置信区间

**最低证据要求**：
- 95% 置信区间下界 > 0
- 置信区间宽度不超过均值绝对值的 200%

**拒绝条件**：95% 置信区间下界 ≤ 0

---

### 2.4 最大回撤 (Maximum Drawdown)

**定义**：从历史最高点到最低点的最大跌幅。

**计算方法**：
```
MDD = max(Peak - Trough) / Peak × 100%
其中 Peak = 最高累计收益，Trough = 最低累计收益
```

**最低证据要求**：

| 策略类型 | 最大回撤上限 |
|----------|--------------|
| 日内 | 5% |
| 短线 (1-5天) | 10% |
| 中线 (5-20天) | 15% |
| 长线 (20+天) | 20% |

**拒绝条件**：最大回撤超过对应类型上限

---

### 2.5 跨滚动窗口表现 (Cross-Rolling-Window Performance)

**定义**：将历史数据划分为多个独立时间窗口，验证范式在每个窗口的表现。

**计算方法**：
1. 将数据划分为多个不重叠的滚动窗口（如 1 年一个窗口）
2. 在每个窗口内独立回测
3. 计算各窗口的：收益、胜率、最大回撤

**最低证据要求**：
- 至少 3 个滚动窗口
- 至少 70% 的窗口期望收益 > 0
- 窗口间收益标准差不超过均值的 150%

**拒绝条件**：少于 3 个窗口 或 超过 50% 的窗口负收益

---

### 2.6 跨市场状态表现 (Cross-Market-Regime Performance)

**定义**：验证范式在不同市场环境下的一致性表现。

**市场状态分类**：

| 状态 | 定义 | 判定指标 |
|------|------|----------|
| 牛市 | 20日均线向上，沪深300涨幅 > 10% | MA20 ↑, Index ↑ |
| 熊市 | 20日均线向下，沪深300跌幅 > 10% | MA20 ↓, Index ↓ |
| 震荡 | 20日均线走平，20日内涨跌幅 < 5% | MA20 →, |Index| < 5% |
| 高波动 | ATR 处于过去 60 日 80% 分位以上 | ATR > P80 |

**最低证据要求**：
- 至少覆盖 2 种市场状态
- 每种状态样本量 ≥ 10
- 至少 50% 的市场状态期望收益 > 0

**拒绝条件**：所有市场状态均为负收益

---

### 2.7 参数敏感性 (Parameter Sensitivity)

**定义**：关键参数变化对策略收益的影响程度。

**计算方法**：
1. 识别策略的关键参数（如 MACD 的快线/慢线周期）
2. 将参数在 ±20% 范围内变化
3. 计算收益变化率

**参数敏感性系数**：
```
Sensitivity = |ΔReturn| / |ΔParam| 
其中 ΔParam 为参数变化百分比
```

**最低证据要求**：

| 敏感性系数 | 评级 | 处理方式 |
|------------|------|----------|
| < 0.5 | 低 | 通过 |
| 0.5 - 1.0 | 中 | 警告但可接受 |
| > 1.0 | 高 | 拒绝或需简化参数 |

**拒绝条件**：敏感性系数 > 1.0

---

### 2.8 收益集中度 (Return Concentration)

**定义**：收益分布的集中程度，用于检测是否存在少数交易贡献大部分收益的情况。

**计算方法**：
1. 计算所有交易的收益
2. 按收益降序排列
3. 计算 Top 20% 交易贡献的收益占比

**最低证据要求**：
- Top 20% 交易贡献收益 ≤ 60%
- Top 10% 交易贡献收益 ≤ 40%

**拒绝条件**：Top 20% 交易贡献 > 80% 收益（存在过拟合风险）

---

### 2.9 收益风险比 (Risk-Reward Ratio)

**定义**：平均收益与最大回撤的比值。

**计算方法**：
```
RRR = 平均收益 / |最大回撤|
```

**最低证据要求**：

| 策略类型 | 最低收益风险比 |
|----------|----------------|
| 日内 | 0.5 |
| 短线 | 0.8 |
| 中线 | 1.0 |
| 长线 | 1.2 |

**拒绝条件**：收益风险比低于对应类型要求

---

### 2.10 夏普比率 (Sharpe Ratio)（可选增强）

**定义**：经风险调整后的收益指标。

**计算方法**：
```
Sharpe = (平均收益 - 无风险利率) / 收益标准差
```

**参考标准**：Sharpe > 1.0 为优秀，0.5-1.0 为良好，< 0.5 为一般

---

## 3. 机器可读结构

### 3.1 证据数据模型

```go
// Evidence 范式晋级的完整证据报告
type Evidence struct {
    ParadigmID    string           // 范式 ID
    GeneratedAt   time.Time        // 生成时间
    DataVersion   string           // 数据版本（用于追溯）
    
    // 核心指标
    Metrics       Metrics          // 必填指标集合
    Windows       []WindowResult   // 滚动窗口结果
    Regimes       []RegimeResult   // 市场状态结果
    
    // 校验结果
    Validation    ValidationResult // 综合校验结果
    Warnings      []string         // 警告信息
    Blockers      []string         // 阻塞项（违反禁止项）
}

// Metrics 必填指标集合
type Metrics struct {
    SampleSize           int       `json:"sample_size"`            // 样本量
    HoldingPeriod        string    `json:"holding_period"`         // 持有周期类型: intraday/short/medium/long
    
    // 收益指标
    GrossReturn          float64   `json:"gross_return"`           // 总收益率(%)
    NetReturn            float64   `json:"net_return"`             // 成本后期望收益(%)
    AvgReturn            float64   `json:"avg_return"`             // 平均单次收益(%)
    MedianReturn         float64   `json:"median_return"`          // 中位数收益(%)
    
    // 风险指标
    MaxDrawdown          float64   `json:"max_drawdown"`           // 最大回撤(%)
    StdDev               float64   `json:"std_dev"`                // 收益标准差
    
    // 统计指标
    WinRate              float64   `json:"win_rate"`               // 胜率(%)
    ConfidenceInterval   [2]float64 `json:"confidence_interval"`   // 95% 置信区间 [下限, 上限]
    ConfidenceLevel      float64   `json:"confidence_level"`       // 置信度
    
    // 集中度指标
    Top20Concentration   float64   `json:"top20_concentration"`    // Top20% 收益占比(%)
    Top10Concentration   float64   `json:"top10_concentration"`    // Top10% 收益占比(%)
    
    // 敏感性指标
    ParamSensitivity     float64   `json:"param_sensitivity"`      // 参数敏感性系数
    
    // 收益风险比
    RiskRewardRatio      float64   `json:"risk_reward_ratio"`      // 收益风险比
    SharpeRatio          float64   `json:"sharpe_ratio,omitempty"` // 夏普比率(可选)
}

// WindowResult 滚动窗口结果
type WindowResult struct {
    WindowID    string    `json:"window_id"`     // 窗口标识
    StartDate   string    `json:"start_date"`    // 起始日期
    EndDate     string    `json:"end_date"`      // 结束日期
    SampleSize  int       `json:"sample_size"`   // 窗口内样本量
    NetReturn   float64   `json:"net_return"`    // 成本后收益(%)
    WinRate     float64   `json:"win_rate"`      // 胜率(%)
    MaxDrawdown float64   `json:"max_drawdown"`  // 最大回撤(%)
}

// RegimeResult 市场状态结果
type RegimeResult struct {
    Regime        string  `json:"regime"`          // 市场状态: bull/bear/range/volatile
    SampleSize    int     `json:"sample_size"`     // 样本量
    NetReturn     float64 `json:"net_return"`      // 成本后收益(%)
    WinRate       float64 `json:"win_rate"`        // 胜率(%)
    Count         int     `json:"count"`           // 交易次数
}

// ValidationResult 综合校验结果
type ValidationResult struct {
    Pass          bool              `json:"pass"`            // 是否通过
    Score         float64           `json:"score"`           // 综合评分 0-100
    RejectedRules []string          `json:"rejected_rules"`  // 违反的规则
    Warnings      []string          `json:"warnings"`        // 警告信息
    Blockers      []string          `json:"blockers"`        // 阻塞项（必须修复）
}
```

### 3.2 准入判定规则

```go
// AdmissionResult 准入判定结果
type AdmissionResult struct {
    Eligible       bool              `json:"eligible"`        // 是否符合晋级条件
    Level          string            `json:"level"`           // 准入等级: bronze/silver/gold/platinum
    Reasons        []string          `json:"reasons"`         // 判定理由
    MustFix        []string          `json:"must_fix"`        // 必须修复的问题
    Suggestions    []string          `json:"suggestions"`     // 改进建议
}

// 判定逻辑
func (e *Evidence) CheckAdmission() *AdmissionResult {
    var blockers, mustFix, suggestions []string
    var warnings []string
    score := 100.0
    
    // 1. 禁止项检查（任何一项直接拒绝）
    if e.hasFutureData() {
        blockers = append(blockers, "使用未来数据")
    }
    if e.hasSurvivorshipBias() {
        blockers = append(blockers, "存在幸存者偏差")
    }
    
    // 2. 核心指标检查
    if e.Metrics.NetReturn <= 0 {
        mustFix = append(mustFix, "成本后期望收益必须大于0")
        score -= 30
    }
    if !e.meetsSampleSizeRequirement() {
        mustFix = append(mustFix, "样本量不足")
        score -= 20
    }
    if e.Metrics.ConfidenceInterval[0] <= 0 {
        mustFix = append(mustFix, "95%置信区间下界必须大于0")
        score -= 25
    }
    if e.Metrics.MaxDrawdown > e.maxDrawdownLimit() {
        mustFix = append(mustFix, "最大回撤超过限制")
        score -= 15
    }
    
    // 3. 窗口检查
    if !e.meetsWindowRequirement() {
        mustFix = append(mustFix, "滚动窗口表现不符合要求")
        score -= 20
    }
    
    // 4. 集中度检查
    if e.Metrics.Top20Concentration > 60 {
        warnings = append(warnings, "收益集中度过高")
        score -= 10
    }
    
    // 5. 敏感性检查
    if e.Metrics.ParamSensitivity > 1.0 {
        mustFix = append(mustFix, "参数敏感性过高")
        score -= 15
    }
    
    // 6. 评级
    level := "bronze"
    if score >= 90 && len(blockers) == 0 && len(mustFix) == 0 {
        level = "platinum"
    } else if score >= 75 && len(blockers) == 0 && len(mustFix) == 0 {
        level = "gold"
    } else if score >= 60 && len(blockers) == 0 {
        level = "silver"
    }
    
    return &AdmissionResult{
        Eligible:    len(blockers) == 0 && len(mustFix) == 0,
        Level:       level,
        Reasons:     warnings,
        MustFix:     mustFix,
        Suggestions: suggestions,
    }
}
```

---

## 4. 准入等级

| 等级 | 要求 | 说明 |
|------|------|------|
| **Platinum** | 90分以上，无阻塞项 | 优秀，可进入产品级验证 |
| **Gold** | 75分以上，无阻塞项 | 良好，可进入模拟前向 |
| **Silver** | 60分以上，无阻塞项 | 一般，需改进后重新验证 |
| **Bronze** | 60分以下或有阻塞项 | 不通过，需修复后重新验证 |

---

## 5. 生命周期

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   候选      │───▶│   验证      │───▶│   观察      │───▶│   晋级      │───▶│   淘汰      │
│  (Candidate)│    │ (Validation)│    │ (Observation)│   │ (Promoted) │    │ (Retired)   │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
      │                  │                  │                  │                  │
  初始信号生成      历史回测验证      模拟前向运行      有限实盘         降级或退出
  + 初步筛选       + 证据报告        + 漂移监控        + 逐步加仓
```

---

## 6. 版本与追溯

- 每份证据报告必须记录数据版本、特征版本、规则版本
- 范式生命周期中所有变更必须版本化
- 任何晋级/降级/淘汰决策必须可追溯到具体的证据报告
