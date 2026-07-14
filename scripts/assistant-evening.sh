#!/usr/bin/env bash
set -euo pipefail

MESHCLAW_BIN="${MESHCLAW_BIN:-$HOME/.local/bin/meshclaw}"
TARGET="${MESHCLAW_ASSISTANT_TARGET:-meshclaw-ops}"
LOCATION="${MESHCLAW_ASSISTANT_LOCATION:-Seoul}"

exec "$MESHCLAW_BIN" briefing evening \
  --location "$LOCATION" \
  --target "$TARGET" \
  --execute
