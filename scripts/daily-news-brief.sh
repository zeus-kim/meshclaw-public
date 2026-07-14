#!/usr/bin/env bash
set -euo pipefail

MESHCLAW_BIN="${MESHCLAW_BIN:-$HOME/.local/bin/meshclaw}"
TARGET="${MESHCLAW_NEWS_TARGET:-meshclaw-ops}"
SINCE_HOURS="${MESHCLAW_NEWS_SINCE_HOURS:-24}"
LIMIT="${MESHCLAW_NEWS_LIMIT:-20}"

exec "$MESHCLAW_BIN" briefing rss \
  --since-hours "$SINCE_HOURS" \
  --limit "$LIMIT" \
  --target "$TARGET" \
  --execute
