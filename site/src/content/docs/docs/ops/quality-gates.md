---
title: 质量门禁
description: make check 每一步在验什么，以及为什么还有一道需要网络的 quality-check。
---

仓库只有两个质量入口，本地与 CI 用的是同一套。

## `make check` —— 提交前的统一门禁

```bash
make check
```

依次执行六个目标，**任何一个失败都会让整条命令失败**（不存在“警告继续”）：

| 目标 | 在验什么 |
| --- | --- |
| `fmt-check` | gofmt 基线：新增/修改的代码必须格式化 |
| `go-check` | `go test ./...`，并在真正涉及并发的包上跑 `-race` |
| `go-arch-check` | 架构静态门禁（见下） |
| `go-migration-check` | SQLite 迁移的幂等性与 schema 兼容性 |
| `web-check` | 前端 lint、`tsc` 类型检查、`api:check` 契约校验、vitest |
| `web-build-check` | 生产构建，专门抓“类型能过但打包失败”的情况（例如仅类型导入） |

## 架构门禁在查什么

```bash
go run ./cmd/cli arch check --root .
```

对应 `tongstock arch` 命令，检查：

- 依赖方向（内层不依赖外层）
- 导入循环
- 生产代码里的死调用者
- 重复的领域类型定义
- 生产代码中的 mock / 随机 / 硬编码结果扫描

最后一条最值得说：**生产路径里出现 mock 或随机数会直接让门禁失败**，这是“不许在真数据上造假结论”的工程化保障。

## 契约门禁

HTTP 契约不是文档，是测试对象：

- Go 侧的契约测试**双向比对**实际 Gin 路由与 `api/openapi.json`：加了路由没写契约、或契约里有不存在的路由，都会失败；
- 前端 `api:check` 校验从契约生成的 DTO / 操作表没有漂移——手改了生成代码会被抓出来。

所以本站的[端点参考](/docs/api/endpoints/)可以直接从 `api/openapi.json` 生成而不担心失真。

## `make quality-check` —— 需要真实数据的门禁

```bash
make quality-check
```

这条不在 `make check` 里，因为它需要**网络与本地数据库状态**：

```bash
./tongstock quality check --block   # 数据质量 + 回测黄金集 + 范式阶段门 + AI 评测 + 前向监控 + 恢复就绪
./tongstock quality verify          # 快速版：数据质量 + 回测黄金集
./tongstock quality status          # 查看质量门配置与状态
./tongstock quality report [id]
```

发布前跑它，而不是每次提交都跑。

## 端到端验收

```bash
./tongstock acceptance verify
```

验证正式数据库的全链路血缘：从原始数据到方法、到选股、到持仓决策的每一条引用都要能落地。**缺失即失败**，不做“尽力而为”。

## 站点自身的校验

本站（`site/`）与仓库主门禁**隔离**，自成一套：

```bash
cd site
pnpm gen:check      # 端点表与 CLI 参考是否与仓库现状一致
pnpm check:links    # 站内死链 + 文档里引用的 /api 路径是否真的存在
pnpm build
```

`site/` 不参与 `make check`，避免官网工具链影响仓库主门禁；它在自己的 CI 步骤里按路径过滤运行。
