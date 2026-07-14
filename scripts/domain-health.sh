#!/usr/bin/env bash
set -euo pipefail

domain="${1:-argos.zeus.kim}"
expected_status="${MESHCLAW_DOMAIN_EXPECTED_STATUS:-200}"
expected_via="${MESHCLAW_DOMAIN_EXPECTED_VIA:-Caddy}"
expected_server="${MESHCLAW_DOMAIN_EXPECTED_SERVER:-nginx}"
timeout="${MESHCLAW_DOMAIN_TIMEOUT:-20}"
out="${MESHCLAW_DOMAIN_OUT:-}"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

status="$(
  curl -sS -L --max-redirs 2 --connect-timeout "$timeout" --max-time "$timeout" \
    -D "$tmp" -o /dev/null -w '%{http_code}' "https://${domain}/"
)"

headers="$(tr -d '\r' < "$tmp")"
ok=true
error=""

if [[ "$status" != "$expected_status" ]]; then
  ok=false
  error="expected HTTP $expected_status, got $status"
fi

if ! grep -qi "server: .*${expected_server}" "$tmp"; then
  ok=false
  error="${error:+$error; }missing server header containing ${expected_server}"
fi

if ! grep -qi "via: .*${expected_via}" "$tmp"; then
  ok=false
  error="${error:+$error; }missing via header containing ${expected_via}"
fi

if ! grep -qi "x-robots-tag: noindex, nofollow" "$tmp"; then
  ok=false
  error="${error:+$error; }missing noindex header"
fi

json="$(
  python3 - "$domain" "$status" "$ok" "$error" "$headers" <<'PY'
import json
import sys

domain, status, ok, error, headers = sys.argv[1:6]
print(json.dumps({
    "domain": domain,
    "url": f"https://{domain}/",
    "ok": ok == "true",
    "status": int(status) if status.isdigit() else status,
    "error": error,
    "headers": headers.splitlines(),
}, ensure_ascii=False, indent=2))
PY
)"

if [[ -n "$out" ]]; then
  mkdir -p "$(dirname "$out")"
  printf '%s\n' "$json" > "$out"
fi

printf '%s\n' "$json"

if [[ "$ok" != "true" ]]; then
  exit 1
fi
