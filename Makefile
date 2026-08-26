.PHONY: all web cli server menubar install clean run check \
	fmt-check go-check go-arch-check go-migration-check web-check quality-check

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

server: web
	go build -o tongstock-server ./cmd/server

menubar:
	go build -o tongstock-menubar ./cmd/menubar

# quality-check: the slower end-to-end, data-aware unified quality gate
# (data quality + backtest golden + forward monitors). Distinct from `check`
# because it needs network + local DB state. Run before release.
quality-check: cli
	./$(CLI_BIN) quality check --block

run: cli
	./$(CLI_BIN) server

install: cli
	mkdir -p $(BINDIR)
	install -m 755 $(CLI_BIN) $(BINDIR)/$(CLI_BIN)

clean:
	rm -f $(CLI_BIN) tongstock-server tongstock-menubar
	rm -rf pkg/web/dist
