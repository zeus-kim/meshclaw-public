#!/usr/bin/env bash
set -euo pipefail

export PATH="/Users/example/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export MESHCLAW_SIGNAL_BOT_NUMBER="${MESHCLAW_SIGNAL_BOT_NUMBER:-+821086215273}"
export ARGOS_SIGNAL_ACCOUNT="${ARGOS_SIGNAL_ACCOUNT:-+821086215273}"

log_dir="/Users/example/.meshclaw/logs"
mkdir -p "$log_dir"

while true; do
  # Only registered interactive targets are read. Reports are sent out-of-band.
  /Users/example/bin/meshclaw messenger listen-all \
    --timeout 8 \
    --max-messages 5 \
    --max-age 60 \
    --execute \
    --json || true
  sleep 3
done
