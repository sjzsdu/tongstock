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

`api_key_env` 推荐显式填写。省略时，TongStock 会对 `openai`、`anthropic`、`deepseek`、`openrouter` 和 `zhipu` 使用各自的标准 API Key 环境变量。OpenAI 兼容的本地或私有服务可额外配置 `api_base`。

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
skills: []
tools: [web_search, web_fetch]
no_history: false
---

# Agent.md

先检查数据来源和时效，再列出结论的成立条件与失效条件。
```

自定义定义与内建 Agent 使用同一个注册表。若 `id` 相同，自定义定义覆盖内建定义，因此既能新增 Agent，也能在不修改源码的情况下调整内建角色。

## PicoClaw 兼容迁移

原配置无需立即修改：

```yaml
agent:
  enabled: true
  home: ~/.picoclaw
  config: ~/.picoclaw/config.json
  stock_agent: stock-analyst
```

当 `backend` 省略且存在 `home` 或 `config` 时，TongStock 自动选择 `picoclaw` 后端。也可以显式配置 `backend: picoclaw`。迁移到内建模式时，删除 `home`、`config`，并添加 `provider`、`model` 和 `api_key_env` 即可。Agent 对话 API 和内建 Agent ID 保持不变。
