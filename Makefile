.PHONY: all web cli server install clean run

BINDIR ?= $(if $(GOBIN),$(GOBIN),$(shell go env GOPATH)/bin)
CLI_BIN := tongstock
SERVER_BIN := tongstock-server

all: web cli server

web:
	cd web && pnpm build
	rm -rf pkg/web/dist
	cp -r web/dist pkg/web/dist

cli:
	go build -o $(CLI_BIN) ./cmd/cli

server: web
	go build -o $(SERVER_BIN) ./cmd/server

run: server
	./$(SERVER_BIN)

install: server cli
	mkdir -p $(BINDIR)
	install -m 755 $(CLI_BIN) $(BINDIR)/$(CLI_BIN)
	install -m 755 $(SERVER_BIN) $(BINDIR)/$(SERVER_BIN)

clean:
	rm -f $(CLI_BIN) $(SERVER_BIN)
	rm -rf pkg/web/dist
