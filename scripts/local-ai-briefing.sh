#!/usr/bin/env bash
set -euo pipefail

MESHCLAW_BIN="${MESHCLAW_BIN:-$HOME/bin/meshclaw}"

exec "$MESHCLAW_BIN" briefing local-ai
