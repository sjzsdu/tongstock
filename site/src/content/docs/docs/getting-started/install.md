---
title: 安装
description: Release 二进制、源码编译、Docker 三条安装路径，以及各自的依赖前提。
---

## 前提

源码构建需要：

- **Go 1.25+**（仓库 `go.mod` 声明的版本为准，当前是 `1.25.13`）
- **Node.js 22+**
- **pnpm 10**

只想跑二进制的话，只需要对应平台的可执行文件，不需要 Go 与 Node。

## 路径一：下载 Release 二进制

Release 产物命名为 `tongstock-<tag>-<goos>-<goarch>.tar.gz`（Windows 为 `.zip`），覆盖 linux / darwin / windows 的 amd64 与 arm64 组合。解包后目录名与包名一致，里面是 `tongstock` 可执行文件。

把解出来的二进制放进 `PATH` 即可：

```bash
tar -xzf tongstock-vX.Y.Z-darwin-arm64.tar.gz
install -m 755 tongstock-vX.Y.Z-darwin-arm64/tongstock ~/.local/bin/tongstock
tongstock --help
```

## 路径二：源码编译（推荐给要改代码的人）

```bash
git clone https://github.com/sjzsdu/tongstock.git
cd tongstock

# 先把 Web UI 装好依赖：CLI 会把 web/dist 嵌进二进制
cd web && pnpm install --frozen-lockfile && cd ..

# 构建二进制（make cli = 构建前端 + go build）
make cli

# 启动服务
./tongstock serve
```

`make cli` 会先执行 `pnpm build` 生成 `web/dist`，再复制到 `pkg/web/dist` 由 Go 侧 `//go:embed` 嵌进二进制——所以**跳过前端构建会导致编译失败或界面缺失**。

装到用户目录：

```bash
make install          # 默认 ~/.local/bin，可用 BINDIR=/path 覆盖
```

:::caution[go.mod 通过 replace 引用相邻目录]
仓库 `go.mod` 里有 `replace github.com/sipeed/picoclaw => ../picoclaw`，即要求 `picoclaw` 与 `tongstock` 是同级目录。CI 与 Dockerfile 会按固定 commit 拉取该依赖；本地首次 `go build` 前需要把它放到 `../picoclaw`，否则会报找不到模块。
:::

## 路径三：Docker

镜像已发布为 `sjzsdu/tongstock`，构建入口在仓库根的 `Dockerfile`：

```bash
docker run --rm -p 8106:8106 \
  -v "$HOME/.tongstock:/root/.tongstock" \
  sjzsdu/tongstock:latest
```

本地自己构建：

```bash
make docker-build     # docker build -t sjzsdu/tongstock:latest -t <tag>
make docker-push
```

从别的机器访问容器里的服务必须配 `access_token`，否则服务按设计拒绝启动——细节见[远程访问与令牌](/docs/ops/remote-access/)。

## 装完先做什么

1. `./tongstock serve`，浏览器打开 `http://127.0.0.1:8106/`。
2. 想用命令行验证一下：`./tongstock quote 000001`。
3. 想把能力接进 AI 助手：`npx skills add sjzsdu/tongstock`，见 [AI 助手集成](/docs/usage/skills/)。

配置项与目录结构见[首次运行与配置](/docs/getting-started/first-run/)。
