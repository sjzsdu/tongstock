# syntax=docker/dockerfile:1

# 全局构建参数（可用 --build-arg 覆盖）
# GOPROXY 默认走国内代理；海外构建可用：
#   --build-arg GOPROXY=https://proxy.golang.org,direct
ARG GOPROXY=https://goproxy.cn,direct

# go.mod 通过 `replace github.com/sipeed/picoclaw => ../picoclaw` 引用本地兄弟
# 仓库，Docker 构建上下文里不存在该目录。这里用公开 fork 固定 commit 在镜像内
# 还原同一份代码（已实测 tongstock 全量 ./... 编译通过）。本地 picoclaw 有更新
# 并需要进镜像时，更新 PICOCLAW_PIN。
ARG PICOCLAW_REPO=https://github.com/sjzsdu/picoclaw.git
ARG PICOCLAW_PIN=1f8286e5e91187a1d978d0a1869d9ddb5a1d9c9c

# ---- Stage 1: 前端与官网构建 ----
# web/ 与 site/ 都用 pnpm（各自带 pnpm-lock.yaml），没有 package-lock.json。
# Node 24：Astro 7 的硬下界是 22.23.4，固定大版本 24 避免再被 patch 拖累。
FROM node:24-bookworm AS web-builder
RUN npm install -g pnpm@10
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# 自托管布局：应用挂在 /app/ 下（官网占根），产物里的资源前缀必须是 /app/。
RUN VITE_BASE_PATH=/app/ pnpm run build

WORKDIR /src/site
COPY site/package.json site/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY site/ ./
RUN ASTRO_TELEMETRY_DISABLED=1 pnpm run build

# ---- Stage 2: Go 构建（mattn/go-sqlite3 需要 CGO，不能用 CGO_ENABLED=0）----
FROM golang:1.25.13-bookworm AS go-builder
ARG GOPROXY
ARG PICOCLAW_REPO
ARG PICOCLAW_PIN
ENV GOPROXY=${GOPROXY} \
    CGO_ENABLED=1
# go.mod 的 replace ../picoclaw 相对 WORKDIR /src 解析，即 /picoclaw
RUN git init /picoclaw \
 && git -C /picoclaw remote add origin "${PICOCLAW_REPO}" \
 && git -C /picoclaw fetch --depth 1 origin "${PICOCLAW_PIN}" \
 && git -C /picoclaw checkout --detach FETCH_HEAD \
 && rm -rf /picoclaw/.git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 前端产物拷进 pkg/web/dist 与 pkg/site/dist（与 make cli 的行为一致，go:embed 需要）
COPY --from=web-builder /src/web/dist ./pkg/web/dist
COPY --from=web-builder /src/site/dist ./pkg/site/dist
RUN go build -o /out/tongstock ./cmd/cli

# ---- Stage 3: 运行时 ----
# debian (glibc) 而非 alpine (musl)：CGO 编译的 sqlite3 二进制不兼容 musl。
# curl 供部署侧 compose healthcheck 探活 /health 使用。
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/*
ENV TZ=Asia/Shanghai
COPY --from=go-builder /out/tongstock /usr/local/bin/tongstock
EXPOSE 8106
# 容器单进程模型：前台运行，由 docker/compose 管理生命周期；
# serve start --daemon 的 setsid + PID 文件模式仅适用于本机部署
ENTRYPOINT ["tongstock"]
CMD ["serve"]
