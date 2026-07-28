.PHONY: all web cli server menubar install clean run check fmt-check go-check web-check

# Default to ~/.local/bin for installation, which is the standard user binary directory
# Users can override with BINDIR environment variable if needed
BINDIR ?= $(HOME)/.local/bin
CLI_BIN := tongstock
GO ?= go
GOFMT ?= gofmt

all: cli

web:
	cd web && CI=true pnpm run build
	rm -rf pkg/web/dist
	cp -rf web/dist pkg/web/dist

check: fmt-check go-check web-check

fmt-check:
	sh scripts/gofmt-baseline.sh $(GOFMT)

go-check:
	$(GO) test ./...
	$(GO) test -race ./pkg/tdx/... ./pkg/server/... ./internal/app/... ./internal/serverapp

web-check:
	cd web && pnpm lint
	cd web && pnpm typecheck
	cd web && pnpm api:check
	cd web && pnpm test:ci

cli: web
	go build -o $(CLI_BIN) ./cmd/cli

server: web
	go build -o tongstock-server ./cmd/server

menubar:
	go build -o tongstock-menubar ./cmd/menubar

run: cli
	./$(CLI_BIN) server

install: cli
	mkdir -p $(BINDIR)
	install -m 755 $(CLI_BIN) $(BINDIR)/$(CLI_BIN)

clean:
	rm -f $(CLI_BIN) tongstock-server tongstock-menubar
	rm -rf pkg/web/dist
