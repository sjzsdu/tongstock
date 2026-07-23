.PHONY: all web cli server menubar install clean run

BINDIR ?= $(if $(GOBIN),$(GOBIN),$(shell go env GOPATH)/bin)
CLI_BIN := tongstock
SERVER_BIN := tongstock-server
MENUBAR_BIN := tongstock-menubar

all: cli

web:
	cd web && CI=true pnpm run build
	rm -rf pkg/web/dist
	cp -r web/dist pkg/web/dist

cli: web
	go build -o $(CLI_BIN) ./cmd/cli

server: web
	go build -o $(SERVER_BIN) ./cmd/server

menubar:
	go build -o $(MENUBAR_BIN) ./cmd/menubar

run: cli
	./$(CLI_BIN) server

install: cli
	mkdir -p $(BINDIR)
	install -m 755 $(CLI_BIN) $(BINDIR)/$(CLI_BIN)

clean:
	rm -f $(CLI_BIN) $(SERVER_BIN) $(MENUBAR_BIN)
	rm -rf pkg/web/dist
