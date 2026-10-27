.PHONY: all web cli server install clean run check \
	fmt-check go-check go-arch-check go-migration-check web-check quality-check \
	docker-build docker-push site site-check

# Default to ~/.local/bin for installation, which is the standard user binary directory
# Users can override with BINDIR environment variable if needed
BINDIR ?= $(HOME)/.local/bin
CLI_BIN := tongstock
GO ?= go
GOFMT ?= gofmt
NPM ?= npm

all: cli

web:
	cd web && $(NPM) run build
	rm -rf pkg/web/dist
	cp -rf web/dist pkg/web/dist

# Unified quality check. All targets in `check` are blocking — any failure
# returns non-zero so CI/release never ships regressions.
check: fmt-check go-check go-arch-check go-migration-check web-check web-build-check

fmt-check:
	sh scripts/gofmt-baseline.sh $(GOFMT)

# Go unit + integration tests. Note: race tests are scoped to the packages
# that actually exercise concurrency (server, storage adapters, serviceproc)
# so we keep the race step under ~3 min on developer machines.
go-check:
	$(GO) test ./...
	$(GO) test -race ./pkg/tdx/... ./pkg/server/... ./internal/app/... ./internal/serverapp ./internal/paradigms ./internal/backtest ./internal/quality

# Architecture static gates from tongstock-ai.1.5:
#   dependency direction, import cycles, no mock/random/hardcoded in
#   production, dead production callers, duplicate domain type owners.
go-arch-check:
	$(GO) run ./cmd/cli arch check --root .

# Storage migration idempotency and schema compatibility tests.
go-migration-check:
	$(GO) test ./pkg/storage/ -run Migration -v

web-check:
	cd web && $(NPM) run lint
	cd web && $(NPM) run typecheck
	cd web && $(NPM) run api:check
	cd web && $(NPM) run test:ci

# web-build-check: release build of the frontend. This catches type-only
# imports that pass --noEmit but fail at bundle time.
web-build-check:
	cd web && $(NPM) run build

cli: web
	go build -o $(CLI_BIN) ./cmd/cli

# quality-check: the slower end-to-end, data-aware unified quality gate
# (data quality + backtest golden + forward monitors). Distinct from `check`
# because it needs network + local DB state. Run before release.
quality-check: cli
	./$(CLI_BIN) quality check --block

run: cli
	./$(CLI_BIN) serve

install: cli
	mkdir -p $(BINDIR)
	install -m 755 $(CLI_BIN) $(BINDIR)/$(CLI_BIN)

clean:
	rm -f $(CLI_BIN) tongstock-server
	rm -rf pkg/web/dist

# Docker 镜像构建/推送（供部署仓库使用：compose/Traefik 拉取 sjzsdu/tongstock）
# latest 之外附加一个版本 tag：优先 git tag（如 v0.2.7），无 tag 时退化为 short
# hash；本地有未提交改动时 git describe 追加 -dirty 后缀。
DOCKER_IMAGE ?= sjzsdu/tongstock
DOCKER_TAG ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

docker-build:
	docker build -t $(DOCKER_IMAGE):latest -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-push: docker-build
	docker push $(DOCKER_IMAGE):latest
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

# 官网站点（site/）：完全隔离的一套工具链，不进入上面的 check 门禁。
# 先构建 CLI 二进制，因为文档里的命令与参数表由 `tongstock --help` 实跑生成，
# 端点表由 api/openapi.json 生成——两者都不允许手写。
# 产物在 site/dist/（已 gitignore），部署位置未定，本目标只保证“本地可构建”。
SITE_DIR := site
SITE_BIN := $(CURDIR)/$(CLI_BIN)

site: cli
	cd $(SITE_DIR) && pnpm install --frozen-lockfile
	cd $(SITE_DIR) && TONGSTOCK_BIN=$(SITE_BIN) ASTRO_TELEMETRY_DISABLED=1 pnpm gen
	cd $(SITE_DIR) && ASTRO_TELEMETRY_DISABLED=1 pnpm build
	cd $(SITE_DIR) && node scripts/check-links.mjs

# 只校验不写盘：生成物是否与仓库现状一致（需要已有的 CLI 二进制与 site/dist）。
site-check:
	cd $(SITE_DIR) && TONGSTOCK_BIN=$(SITE_BIN) pnpm gen:check
	cd $(SITE_DIR) && node scripts/check-links.mjs
