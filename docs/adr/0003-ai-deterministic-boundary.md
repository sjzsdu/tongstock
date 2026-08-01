# ADR 0003：AI 与确定性内核边界

- 状态：Accepted
- 日期：2026-08-01
- 依赖：ADR 0002

## 背景

当前研究流程完全由人工驱动：用户手填假设参数（20+ 字段）、手工点按钮推进范式状态机、手工注入前向信号、手工提交监控收益数组。这违背 TongStock 的核心目标——让 AI 自动发现规律并验证。同时，如果让 AI 直接写入 verified 状态或编造统计结果，会破坏真实性保证。

## 决策

### AI 可以做的事

1. **提出候选**：AI agent 通过显式 Tool/Use Case 提出范式候选（Paradigm candidate），状态为 `candidate`，不是 `verified`。
2. **解释证据**：AI 可以读取已生成的 Evidence 和 Metrics，用自然语言向用户解释。
3. **建议参数**：AI 可以建议买入/卖出条件、持仓周期、预期收益区间，但这些只是 candidate 字段，不直接成为交易信号。
4. **生成研究文本**：AI 可以生成研究分析文本（agent_text），作为辅助说明。

### AI 不能做的事（硬性禁止）

1. **不能直接写入 `verified` 状态**：范式状态机从 `candidate` → `experimented` → `reviewed` → `verified` 的推进，必须经过确定性 Critic + Evidence 准入规则，AI 无权跳过。
2. **不能编造统计结果**：回测收益、胜率、置信度、p-value 等统计量必须由确定性 backtest engine 计算，AI 不能生成或修改这些值。
3. **不能直接创建前向信号**：Forward Signal 只能由确定性执行引擎根据 verified 范式和市场数据自动生成，AI 不能手注。
4. **不能直接生成监控报告**：Monitoring Report 必须由确定性计算引擎消费 Forward Ledger 产出，AI 不能手填收益数组。
5. **不能直接写入交易信号**：买什么/何时卖的最终决策由确定性 use case 根据已验证范式产出，AI 只能提供候选和解释。

### 确定性内核边界

以下能力是确定性的，AI 不参与计算只参与消费：

| 能力 | Owner | 输入 | 输出 |
|---|---|---|---|
| Backtest | `domain/experiment` + `application/usecases/backtest` | 范式 + 数据集快照 | Experiment Run + Metrics |
| Evidence 准入 | `domain/paradigm` + `internal/ai_critic` | Experiment Run + Metrics | AdmissionResult (admit/reject) |
| Forward Signal | `domain/forward` + `application/usecases/forward_execute` | verified 范式 + 市场数据 | SignalEntry (持久化) |
| Monitoring | `domain/monitoring` + `application/usecases/monitor_run` | Forward Ledger (持久化) | MonitorReport (持久化) |
| Quality Gate | `domain/quality` + `application/usecases/quality_check` | 数据集 + 范式 | QualityReport |

### AI 交互入口

AI 只通过以下入口与系统交互：

1. **Tool Registry**（`internal/ai_tools`）：AI agent 调用注册的 tool 提出候选或查询证据，tool 内部调用 use case。
2. **Agent Chat**（`internal/agents` + `internal/picoclaw`）：对话式交互，agent 可以读取公开数据但写入必须经过 use case。
3. **不允许** AI 直接调用 Store、DB 或 Skip use case 层。

## 验证方式

1.4 完成后：
- AI 写入路径只有 `application/usecases` 层的 Tool 入口
- `verified` 状态推进路径不包含任何 AI/LLM 包的写入
- Forward Signal 写入路径不包含人工注入入口
- Monitoring Report 产出路径不包含人工收益数组提交

## 迁移

1.3 阶段删除所有人工手填入口（Hypothesis 页面、forward signal append、monitoring run）。
1.4 阶段将 AI tool 入口收敛为唯一候选创建路径。
