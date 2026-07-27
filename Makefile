.PHONY: all web cli server menubar install clean run

# Default to ~/.local/bin for installation, which is the standard user binary directory
# Users can override with BINDIR environment variable if needed
BINDIR ?= $(HOME)/.local/bin
CLI_BIN := tongstock

all: cli

web:
	cd web && CI=true pnpm run build
	rm -rf pkg/web/dist
	cp -r web/dist pkg/web/dist

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
