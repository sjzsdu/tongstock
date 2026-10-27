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

## 官网与文档的托管（GitHub Pages）

官网 + 文档 + Web UI 部署在同一个 Pages 站点上，布局是：

| 路径 | 内容 | 构建来源 |
| --- | --- | --- |
| `/<repo>/` | 官网与文档 | `site/dist`（`SITE_BASE=/<repo>/`） |
| `/<repo>/app/` | 现有 Web UI | `web/dist`（`VITE_BASE_PATH=/<repo>/app/`） |
| `/<repo>/404.html` | 根 404 分流 | `site/scripts/gen-pages-404.mjs` 生成 |

部署位置**不写死在代码里**，全部来自构建期变量。GitHub Actions 里用仓库变量（Settings → Secrets and variables → Actions → Variables）覆盖，默认值见下表。

### 变量表

| 变量 | 默认值 | 作用 | 改哪里 |
| --- | --- | --- | --- |
| `SITE_URL` | `https://<owner>.github.io` | 站点 origin（canonical、og:image、sitemap 用） | Pages workflow 的 `env`，或 repo variable |
| `SITE_BASE` | `/<repo>/` | 部署子路径；同时决定 Astro 的 `base` 与应用的 `VITE_BASE_PATH` | 同上 |
| `APP_URL` | `<SITE_URL><SITE_BASE>app/` | 落地页主 CTA「进入控制台」的落点，可指向任意实例（含内网） | repo variable |
| `REPO_URL` | `https://github.com/sjzsdu/tongstock` | 仓库链接、Edit 链接、Release/契约/许可链接的基址 | repo variable |
| `RELEASES_URL` | `<REPO_URL>/releases` | 页脚 Release 链接 | 一般不用改 |
| `DOCKER_IMAGE` | `sjzsdu/tongstock` | Docker Hub 链接由它推导（`<DOCKER_IMAGE>`） | 一般不用改 |
| `API_CONTRACT_URL` | `<REPO_URL>/blob/master/api/openapi.json` | 页脚 API 契约链接 | 一般不用改 |
| `LICENSE_URL` | `<REPO_URL>#许可证` | 页脚许可链接 | 一般不用改 |
| `LOCAL_COMMAND` / `LOCAL_URL` | `./tongstock serve` / `http://localhost:8106/` | 落地页「本机启动」的复制命令与地址 | repo variable |
| `ASTRO_TELEMETRY_DISABLED` | `1` | 关闭 Astro 遥测（CI 与受限环境必需） | Pages workflow 的 `env` |

`site.ts` 只提供上表的默认值；页面与文档一律从它读取，没有任何硬编码 URL。

### 换部署位置怎么做

改变量、重跑 workflow，不改代码。本地可以这样验证：

```bash
cd site
SITE_URL=https://example.com SITE_BASE=/repo/ APP_URL=https://inner.local:8106/ pnpm build
grep -o 'href="https://inner.local:8106/"' dist/index.html | head -1
```

要指向内网实例，只设 `APP_URL` 一个变量即可——主 CTA、文档里的“进入 Web UI”引用都会跟着变。

### 防呆

`astro.config.mjs` 会拒绝自相矛盾的配置：`SITE_URL` 里带了路径（如 `https://host/repo`）却把 `SITE_BASE` 留空时，构建直接报错并给出两种正确写法，而不是悄悄生成一半对一半错的链接。

### 深链与 404

静态托管只有一个根 `404.html`，所以 `/<repo>/app/stock/000001` 这类刷新会落到它：根 404 里的脚本把应用内路径放进 `?__app_path=` 跳到应用根，应用启动时（`web/src/lib/appPath.ts`）用 `history.replaceState` 还原。旧的外链 `/<repo>/stock/000001` 会被同一脚本识别并一次跳转到还原入口——旧路由表从 `web/src/App.tsx` 生成，不是手写枚举。

本地验收（模拟 Pages 语义，任一不过即失败）：

```bash
cd site
node scripts/assemble-pages.mjs --site-dist=dist --web-dist=../web/dist \
  --out=../pages-dist --base=/tongstock/ --app-path=app
node scripts/check-pages.mjs --root=../pages-dist --base=/tongstock/
```
