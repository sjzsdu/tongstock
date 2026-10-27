# web/ —— TongStock Web UI

React 19 + TypeScript + Vite 构建的单页应用。构建产物会被复制到 `pkg/web/dist`，由 Go 侧的 `//go:embed` 嵌进二进制，所以**运行时不依赖 Node**。

完整的页面清单、每页职责与对应接口见官网站点的 [Web UI 指南](../site/src/content/docs/docs/usage/web-ui.md)；本文只说开发相关的事。

## 开发

```bash
cd web
pnpm install --frozen-lockfile
pnpm dev        # 开发服务器，/api 代理到 vite.config.ts 配置的目标（默认 localhost:8106）
```

开发前先起后端：

```bash
./tongstock serve     # 默认 127.0.0.1:8106
```

## 构建

```bash
pnpm build      # tsc -b + vite build
```

生产构建会先跑 TypeScript 编译，类型错误直接中断构建。构建完成后把 `web/dist` 复制到 `pkg/web/dist` 才能被 Go 嵌入——仓库根的 `make cli` 已经包含这一步。

## 接口契约

DTO 与操作表由 `api/openapi.json` 生成，**不要手改生成文件**：

```bash
pnpm api:generate   # 重新生成
pnpm api:check      # 校验没有漂移（CI 与 make check 会跑）
```

Go 侧的契约测试会双向比对实际 Gin 路由与 `api/openapi.json`：新增接口必须同时更新契约，否则测试失败。

## 质量门禁

```bash
pnpm lint         # ESLint（基线模式，只报新增问题）
pnpm typecheck    # tsc -b --noEmit
pnpm test         # vitest
```

`make check` 会把 `lint`、`typecheck`、`api:check`、`test` 和生产构建全部串起来。

## 两个容易踩的部署细节

1. **API 请求走域根绝对路径**：`src/api/client.ts` 里 `BASE = ''`，请求形如 `/api/quote`。子路径部署时 API 仍指向域根，需要反向代理把 `/api` 转发到后端。
2. **路由 basename 由构建变量决定**：`<BrowserRouter basename={import.meta.env.BASE_URL}>`，所以改 `VITE_BASE_PATH` 就能把整个 SPA 挂到子路径（例如 `/tongstock/app/`）下。

## 目录

```
web/src/
├── api/          # 接口客户端与生成的 DTO（generated.ts 由脚本产出）
├── components/   # 复用组件（图表、面板、看板等）
├── pages/        # 路由页面（Dashboard / Methods / Portfolio / stock / news ...）
├── services/     # 业务逻辑（SSE、缓存、格式化等）
├── hooks/        # 自定义 hooks
├── lib/ utils/   # 工具
└── types/        # 类型定义
```
