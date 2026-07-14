#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."
GOCACHE="${GOCACHE:-/private/tmp/meshclaw-go-build}" go test ./...
go build -o "${MESHCLAW_BIN:-/Users/example/bin/meshclaw}" ./cmd/meshclaw
echo "installed ${MESHCLAW_BIN:-/Users/example/bin/meshclaw}"
