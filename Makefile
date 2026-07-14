BIN ?= ./bin/meshclaw
GOCACHE ?= /tmp/meshclaw-go-build

.PHONY: build test install verify mcp-tools

build:
	mkdir -p ./bin
	go build -o $(BIN) ./cmd/meshclaw

test:
	GOCACHE=$(GOCACHE) go test ./...

install: test build

verify: build
	$(BIN) --help >/dev/null
	$(BIN) workflows inspect fleet-health-demo --json >/dev/null
	$(BIN) run fleet-health-demo --dry-run --json >/dev/null

mcp-tools: build
	printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | $(BIN) mcp
