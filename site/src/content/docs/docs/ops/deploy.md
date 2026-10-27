---
title: Docker 部署
description: 镜像构建与推送流程、运行时需要挂载什么、以及容器里的默认监听策略。
---

## 镜像

已发布到 Docker Hub：`sjzsdu/tongstock`。标签策略（由 `.github/workflows/docker-images.yml` 定义）：

| 触发 | 产出标签 |
| --- | --- |
| 推送 `master` | `latest` + `sha-<短SHA>` |
| 推送 `v*` 标签 | `latest` + `vX.Y.Z` |
| 手动触发 | 可自定义版本号 |

## 运行

```bash
docker run -d --name tongstock \
  -p 127.0.0.1:8106:8106 \
  -v "$HOME/.tongstock:/root/.tongstock" \
  --restart unless-stopped \
  sjzsdu/tongstock:latest
```

必须挂载的只有一个目录：`~/.tongstock`（配置 + SQLite 数据库）。容器内路径是 `/root/.tongstock`。

:::note[端口不要随手对外开放]
上面把宿主端绑定在 `127.0.0.1`。要对外提供服务，请先配令牌（见下），再用反向代理暴露——不要直接把 `8106` 摊到公网。
:::

## 通过环境变量注入密钥

容器里推荐用环境变量而不是把令牌写进镜像：

```bash
docker run -d --name tongstock \
  -p 8106:8106 \
  -e TONGSTOCK_SERVER_BIND_ADDRESS=0.0.0.0 \
  -e TONGSTOCK_SERVER_ACCESS_TOKEN="$(openssl rand -hex 32)" \
  -v "$HOME/.tongstock:/root/.tongstock" \
  sjzsdu/tongstock:latest
```

它覆盖 `config.yaml` 中的 `server.bind_address` 与 `server.access_token`，优先级高于配置文件。也支持 `TONGSTOCK_SERVER_PORT`。

注意：容器内必须监听 `0.0.0.0` 才能被端口映射访问；此时**没有令牌会直接拒绝启动**，这是刻意的。

## 自己构建

```bash
# 构建（附加 latest 与 git describe 得到的版本标签）
make docker-build

# 推到 Docker Hub
make docker-push
```

`Dockerfile` 里通过 `PICOCLAW_REPO` 与 `PICOCLAW_PIN` 两个 build-arg 拉取固定 commit 的相邻依赖（`go.mod` 里有 `replace ... => ../picoclaw`），所以构建过程不依赖你本地的目录结构。

## 数据备份

```bash
docker cp tongstock:/root/.tongstock/cache/tongstock.db ./tongstock-$(date +%F).db
```

数据库是单文件（业务数据 + 缓存 + 水位同库），备份与迁移都只需要它。

## 反向代理

把令牌、TLS 与域名一起处理的做法见[远程访问与令牌](/docs/ops/remote-access/)。
