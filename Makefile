.PHONY: all web cli server clean

all: web cli server

web:
	cd web && pnpm build
	rm -rf pkg/web/dist
	cp -r web/dist pkg/web/dist

cli:
	go build -o tongstock-cli ./cmd/cli

server: web
	go build -o tongstock-server ./cmd/server

clean:
	rm -f tongstock-cli tongstock-server
	rm -rf pkg/web/dist
