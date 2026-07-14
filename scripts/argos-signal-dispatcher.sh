#!/usr/bin/env bash
set -euo pipefail

MESHCLAW_BIN="${MESHCLAW_BIN:-/Users/argos/.local/bin/meshclaw}"
LOG_DIR="${MESHCLAW_LOG_DIR:-/Users/argos/.meshclaw/logs}"
INTERVAL_SECONDS="${MESHCLAW_SIGNAL_DISPATCH_INTERVAL_SECONDS:-2}"
TIMEOUT_SECONDS="${MESHCLAW_SIGNAL_DISPATCH_TIMEOUT_SECONDS:-8}"
MAX_MESSAGES="${MESHCLAW_SIGNAL_DISPATCH_MAX_MESSAGES:-20}"
MAX_AGE_SECONDS="${MESHCLAW_SIGNAL_DISPATCH_MAX_AGE_SECONDS:-600}"

mkdir -p "$LOG_DIR"

while true; do
  started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "[$started] signal dispatch tick timeout=${TIMEOUT_SECONDS} max=${MAX_MESSAGES} max_age=${MAX_AGE_SECONDS}"
  if ! "$MESHCLAW_BIN" messenger listen-all \
    --timeout "$TIMEOUT_SECONDS" \
    --max-messages "$MAX_MESSAGES" \
    --max-age "$MAX_AGE_SECONDS" \
    --execute \
    --json; then
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] signal dispatch failed"
  fi
  sleep "$INTERVAL_SECONDS"
done
