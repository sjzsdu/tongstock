# Agent 配置与扩展

TongStock 内建股票 Agent 的注册和配置，不再要求用户维护 PicoClaw 的 `config.json`。运行引擎仍复用 PicoClaw 库，但模型、密钥环境变量和 Agent 定义均可由 TongStock 自己管理。

## 内建模式

在 `~/.tongstock/config.yaml` 中配置：

```yaml
agent:
  enabled: true
  backend: builtin
  provider: deepseek
  model: deepseek-chat
  api_key_env: DEEPSEEK_API_KEY
  agent: stock-analyst
  stock_agent: stock-analyst
```

启动前设置密钥：

```bash
export DEEPSEEK_API_KEY="your-api-key"
./tongstock server
```

`api_key_env` 推荐显式填写。省略时，TongStock 会对 `openai`、`anthropic`、`deepseek`、`openrouter` 和 `zhipu` 使用各自的标准 API Key 环境变量。远程 provider 必须能从该环境变量读到非空密钥，否则启动诊断会明确报错。`ollama`、`vllm`、`lmstudio`、`gpt4free`、`claude-cli` 和 `codex-cli` 作为本地 provider，可以不配置密钥。OpenAI 兼容的本地或私有服务可额外配置 `api_base`。

## 添加自定义 Agent

`agent_paths` 可以包含 Markdown 文件或目录。目录会被递归扫描，文件格式与 `internal/agents/embedded/*.md` 相同：

```yaml
agent:
  enabled: true
  backend: builtin
  provider: openai
  model: gpt-5.4
  agent_paths:
    - ~/.tongstock/agents
```

Agent 文件示例：

```markdown
---
id: risk-reviewer
name: 风险复核员
description: 复核投资结论中的风险与证据缺口
aliases: [risk, 风险复核]
skills: []
tools: [web_search, web_fetch]
no_history: false
---

# Agent.md

先检查数据来源和时效，再列出结论的成立条件与失效条件。
```

自定义定义与内建 Agent 使用同一个注册表。若 `id` 相同，自定义定义覆盖内建定义，因此既能新增 Agent，也能在不修改源码的情况下调整内建角色。

`aliases` 是可选字符串列表。`/api/agent/chat`、`/api/agent/chat/stream` 和 `/api/agent/debate` 都可使用 Agent 的规范 `id` 或任一别名，并且对大小写不敏感。服务在校验后会统一回填规范 `id`，因此会话记录和 debate participants 不会因别名或大小写变体产生多个 Agent 标识。别名应在注册表中保持唯一，避免同一输入匹配多个定义。

`agent_paths` 按配置中的先后顺序加载，后面的路径覆盖前面的路径。目录内部按相对路径字典序递归加载，因此同一目录中路径字典序更靠后的文件覆盖更早的文件。最终返回列表会按 Agent ID 排序，但该排序不改变覆盖优先级。配置文件中的 `agent_paths` 是全量列表，不与默认值叠加；空字符串会被忽略，符号链接目录会解析到目标目录后加载。

## PicoClaw 兼容迁移

原配置无需立即修改：

```yaml
agent:
  enabled: true
  home: ~/.picoclaw
  config: ~/.picoclaw/config.json
  stock_agent: stock-analyst
```

当 `backend` 省略且存在 `home` 或 `config` 时，TongStock 自动选择 `picoclaw` 后端。也可以显式配置 `backend: picoclaw`。旧 PicoClaw 配置若未显式设置 `agent.model`，只有运行时配置中可用的默认模型才能被自动采用；例如模型缺少成功的可用性状态时，服务可能以 `no model specified` 进入 degraded。遇到该提示时，优先在 TongStock 配置中显式填写 `agent.model`，或先在 PicoClaw 中验证并启用默认模型。迁移到内建模式时，删除 `home`、`config`，并添加 `provider`、`model` 和 `api_key_env` 即可。Agent 对话 API 和内建 Agent ID 保持不变。
