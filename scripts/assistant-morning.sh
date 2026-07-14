#!/usr/bin/env bash
set -euo pipefail

MESHCLAW_BIN="${MESHCLAW_BIN:-$HOME/.local/bin/meshclaw}"
TARGET="${MESHCLAW_ASSISTANT_TARGET:-meshclaw-ops}"
LOCATION="${MESHCLAW_ASSISTANT_LOCATION:-Seoul}"
NEWS_LIMIT="${MESHCLAW_ASSISTANT_NEWS_LIMIT:-10}"

exec "$MESHCLAW_BIN" briefing morning \
  --location "$LOCATION" \
  --news-limit "$NEWS_LIMIT" \
  --target "$TARGET" \
  --execute
